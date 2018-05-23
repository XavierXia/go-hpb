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
	"github.com/hpb-project/go-hpb/blockchain"
	"github.com/hpb-project/go-hpb/network/p2p"
	"github.com/hpb-project/go-hpb/network/p2p/discover"
	"math/rand"
	"time"

	"github.com/hpb-project/go-hpb/common"
	"github.com/hpb-project/go-hpb/common/log"
)

const (
	syncInterval        = 10 * time.Second
	txChanCache         = 100000
	txsyncPackSize      = 100 * 1024
)

type synctrl struct {
	sch      *scheduler
	syn      []*sync
	newPeer  chan interface{}
	txChan   chan TxEvent
	txsyncCh chan *txsync
	quitSync chan struct{}
}

type txsync struct {
	p   *peermanager.peer
	txs []*types.Transaction
}

func NewSynCtrl(sh *scheduler) *synctrl {
	ctrl := &synctrl{
		sch    : sh,
		newPeer: make(chan interface{}),
	}
	ctrl.txChan = make(chan TxEvent, txChanSize)

	return ctrl
}

func (this *synctrl) Start() error {
	defer this.Stop()

	// broadcast
	go this.txTransLoop()
	go this.minedTransLoop()

	// start sync handlers
	go this.syncBlock()
	go this.txRelayLoop()
}

func (this *synctrl) txTransLoop() {
	for {
		select {
		case ch := <-this.txChan:
			this.BroadcastTx(ch.Tx.Hash(), ch.Tx)

			// Err() channel will be closed when unsubscribing.
		case <-this.txSub.Err():
			return
		}
	}
}

func (this *synctrl) minedTransLoop() {
	// automatically stops if unsubscribe
	for obj := range this.minedBlockSub.Chan() {
		switch ev := obj.Data.(type) {
		case core.NewMinedBlockEvent:
			this.BroadcastBlock(ev.Block, true)  // First propagate block to peers
			this.BroadcastBlock(ev.Block, false) // Only then announce to the rest
		}
	}
}

func (this *synctrl) txRelayLoop() {
	var (
		pending = make(map[discover.NodeID]*txsync)
		sending = false               // whether a send is active
		pack    = new(txsync)         // the pack that is being sent
		done    = make(chan error, 1) // result of the send
	)

	// send starts a sending a pack of transactions from the sync.
	send := func(s *txsync) {
		// Fill pack with transactions up to the target size.
		size := common.StorageSize(0)
		pack.p = s.p
		pack.txs = pack.txs[:0]
		for i := 0; i < len(s.txs) && size < txsyncPackSize; i++ {
			pack.txs = append(pack.txs, s.txs[i])
			size += s.txs[i].Size()
		}
		// Remove the transactions that will be sent.
		s.txs = s.txs[:copy(s.txs, s.txs[len(pack.txs):])]
		if len(s.txs) == 0 {
			delete(pending, s.p.ID())
		}
		// Send the pack in the background.
		s.p.Log().Trace("Sending batch of transactions", "count", len(pack.txs), "bytes", size)
		sending = true
		go func() { done <- pack.p.SendTransactions(pack.txs) }()
	}

	// pick chooses the next pending sync.
	pick := func() *txsync {
		if len(pending) == 0 {
			return nil
		}
		n := rand.Intn(len(pending)) + 1
		for _, s := range pending {
			if n--; n == 0 {
				return s
			}
		}
		return nil
	}

	for {
		select {
		case s := <-this.txsyncCh:
				send(s)
		case err := <-done:
			sending = false
			// Stop tracking peers that cause send failures.
			if err != nil {
				pack.p.Log().Debug("Transaction send failed", "err", err)
				delete(pending, pack.p.ID())
			}
			// Schedule the next send.
			if s := pick(); s != nil {
				send(s)
			}
		case <-this.quitSync:
			return
		}
	}
}

func (this *synctrl) syncBlock() {
	intervalSync := time.NewTicker(syncInterval)
	defer intervalSync.Stop()
	for {
		select {
		case peer := <-this.newPeer:
			p, ok := peer.(*p2p.Peer) if ok {
			go this.syn.start(p)
		}
		case <-intervalSync.C:
			// Force a sync even if not enough peers are present
			go this.syn.start(peermanager.Instance().BestPeer())
		}
	}
}

func (this *synctrl) Stop() {

	close(this.quitSync)
	log.Info("Hpb protocol stopped")
}