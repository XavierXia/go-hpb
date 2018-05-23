// Copyright 2018 The go-hpb Authors
// This file is part of the go-hpb.
//
// The go-hpb is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-hpb is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-hpb. If not, see <http://www.gnu.org/licenses/>.

package synctrl

import (
	"math/big"
	"sync/atomic"
	"time"

	"github.com/hpb-project/go-hpb/blockchain"
	"github.com/hpb-project/go-hpb/common"
	"github.com/hpb-project/go-hpb/common/log"
)

type fullSyncSty struct {
	dataHead chan packet
	dataBody chan packet

	notified int32
	running  int32
	quitchan chan struct{}
}

func cFullsync() *fullSyncSty {
	syn := &fullSyncSty{
		dataHead: make(chan packet, 1),
		dataBody: make(chan packet, 1),
	}

	return syn
}

func (this *fullSyncSty) start(peer *peermanager.Peer) {
	if peer == nil {
		return
	}
	currentBlock := core.InstanceBlockChain().CurrentBlock()
	td := core.InstanceBlockChain().GetTd(currentBlock.Hash(), currentBlock.NumberU64())

	pHead, pTd := peer.Head()

	if pTd.Cmp(td) <= 0 {
		return
	}
	err := this.Synchronise(peer.id, pHead, pTd)
	if err != nil {
		return
	}
	atomic.StoreUint32(&pm.acceptTxs, 1)
	if head := core.InstanceBlockChain().CurrentBlock(); head.NumberU64() > 0 {
		go this.BroadcastBlock(head, false)
	}
}

func (this *fullSyncSty) Synchronise(id string, head common.Hash, td *big.Int) error {
	err := this.synchronise(id, head, td)
	switch err {
	case nil:
	case errBusy:

	case errTimeout, errBadPeer, errStallingPeer,
		errEmptyHeaderSet, errPeersUnavailable, errTooOld,
		errInvalidAncestor, errInvalidChain:
		log.Warn("Synchronisation failed, dropping peer", "peer", id, "err", err)
		this.dropPeer(id)

	default:
		log.Warn("Synchronisation failed, retrying", "err", err)
	}
	return err
}

func (this *fullSyncSty) synchronise(id string, hash common.Hash, td *big.Int) error {
	// Make sure only one goroutine is ever allowed past this point at once
	if !atomic.CompareAndSwapInt32(&this.running, 0, 1) {
		return errBusy
	}
	defer atomic.StoreInt32(&this.running, 0)

	// Post a user notification of the sync (only once per session)
	if atomic.CompareAndSwapInt32(&this.notified, 0, 1) {
		log.Info("Block synchronisation started")
	}

	// Create cancel channel for aborting mid-flight and mark the master peer
	this.quitchan = make(chan struct{})

	defer this.Cancel() // No matter what, we can't leave the cancel channel open

	p := peermanager.peers.Peer(id)
	if p == nil {
		return errUnknownPeer
	}

	return this.syncWithPeer(p, hash, td)
}

func (this *fullSyncSty) syncWithPeer(p *peermanager.peer, hash common.Hash, td *big.Int) (err error) {
	log.Debug("Synchronising with the network", "peer", p.id, "eth", p.version, "head", hash, "td", td, "mode", this.mode)
	defer func(start time.Time) {
		log.Debug("Synchronisation terminated", "elapsed", time.Since(start))
	}(time.Now())

	// Look up the sync boundaries: the common ancestor and the target block
	latest, err := this.fetchHeight(p)
	if err != nil {
		return err
	}
	height := latest.Number.Uint64()

	origin, err := this.findAncestor(p, height)
	if err != nil {
		return err
	}

	this.syncStatsLock.Lock()
	if this.syncStatsChainHeight <= origin || this.syncStatsChainOrigin > origin {
		this.syncStatsChainOrigin = origin
	}
	this.syncStatsChainHeight = height
	this.syncStatsLock.Unlock()

	// Initiate the sync using a concurrent header and content retrieval algorithm
	pivot := uint64(0)

	this.queue.Prepare(origin+1, this.mode, pivot, latest)

	fetchers := []func() error{
		func() error { return this.fetchHeaders(p, origin+1) }, // Headers are always retrieved
		func() error { return this.fetchBodies(origin + 1) },   // Bodies are retrieved during normal and fast sync
		func() error { return this.fetchReceipts(origin + 1) }, // Receipts are retrieved during fast sync
		func() error { return this.processHeaders(origin+1, td) },
		this.processFullSyncContent,
	}

	err = this.spawnSync(fetchers)
	return err
}

func (this *fullSyncSty) fetchHeaders(p *peermanager.peer, from uint64) error {
	log.Debug("Directing header downloads", "origin", from)
	defer log.Debug("Header download terminated")

	// Create a timeout timer, and the associated header fetcher
	request := time.Now()       // time of the last skeleton fetch request
	timeout := time.NewTimer(0) // timer to dump a non-responsive active peer
	<-timeout.C                 // timeout channel should be initially empty
	defer timeout.Stop()

	var ttl time.Duration
	getHeaders := func(from uint64) {
		request = time.Now()

		ttl = this.requestTTL()
		timeout.Reset(ttl)

		log.Trace("Fetching full headers", "count", MaxHeaderFetch, "from", from)
		go p.peer.RequestHeadersByNumber(from, MaxHeaderFetch, 0, false)
	}
	// Start pulling the header chain skeleton until all is done
	getHeaders(from)

	for {
		select {
		case <-this.cancelCh:
			return errCancelHeaderFetch

		case packet := <-this.headerCh:
			// Make sure the active peer is giving us the skeleton headers
			if packet.PeerId() != p.id {
				log.Debug("Received skeleton from incorrect peer", "peer", packet.PeerId())
				break
			}
			headerReqTimer.UpdateSince(request)
			timeout.Stop()

			// If the skeleton's finished, pull any remaining head headers directly from the origin
			if packet.Items() == 0 && skeleton {
				skeleton = false
				getHeaders(from)
				continue
			}
			// If no more headers are inbound, notify the content fetchers and return
			if packet.Items() == 0 {
				p.log.Debug("No more headers available")
				select {
				case this.headerProcCh <- nil:
					return nil
				case <-this.cancelCh:
					return errCancelHeaderFetch
				}
			}
			headers := packet.(*headerPack).headers

			// Insert all the new headers and fetch the next batch
			if len(headers) > 0 {
				p.log.Trace("Scheduling new headers", "count", len(headers), "from", from)
				select {
				case this.headerProcCh <- headers:
				case <-this.cancelCh:
					return errCancelHeaderFetch
				}
				from += uint64(len(headers))
			}
			getHeaders(from)

		case <-timeout.C:
			// Header retrieval timed out, consider the peer bad and drop
			log.Debug("Header request timed out", "elapsed", ttl)
			headerTimeoutMeter.Mark(1)
			this.dropPeer(p.id)

			// Finish the sync gracefully instead of dumping the gathered data though
			for _, ch := range []chan bool{this.bodyWakeCh, this.receiptWakeCh} {
				select {
				case ch <- false:
				case <-this.cancelCh:
				}
			}
			select {
			case this.headerProcCh <- nil:
			case <-this.cancelCh:
			}
			return errBadPeer
		}
	}
}

func (this *fullSyncSty) stop() {
	if nil != this.quitchan {
		close(this.quitchan)
	}
}