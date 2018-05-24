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
	"fmt"
	"github.com/hpb-project/go-hpb/blockchain"
	"github.com/hpb-project/go-hpb/blockchain/event"
	"github.com/hpb-project/go-hpb/blockchain/types"
	"github.com/hpb-project/go-hpb/network/p2p"

	"sync"
	"sync/atomic"
	"time"

	"github.com/hpb-project/go-hpb/common/constant"
	"github.com/hpb-project/go-hpb/common/log"
	"github.com/hpb-project/go-hpb/consensus"
	"github.com/hpb-project/go-hpb/storage"
)

const (
	syncInterval        = 10 * time.Second
	txChanCache         = 100000
	txsyncPackSize      = 100 * 1024
)

// SyncMode represents the synchronisation mode of the downloader.
type SyncMode int

const (
	FullSync  SyncMode = iota // Synchronise the entire blockchain history from full blocks
	FastSync                  // Quickly download the headers, full sync only at the chain head
	LightSync                 // Download only the headers and terminate afterwards
)

func (mode SyncMode) IsValid() bool {
	return mode >= FullSync && mode <= LightSync
}

// String implements the stringer interface.
func (mode SyncMode) String() string {
	switch mode {
	case FullSync:
		return "full"
	case FastSync:
		return "fast"
	case LightSync:
		return "light"
	default:
		return "unknown"
	}
}

func (mode SyncMode) MarshalText() ([]byte, error) {
	switch mode {
	case FullSync:
		return []byte("full"), nil
	case FastSync:
		return []byte("fast"), nil
	case LightSync:
		return []byte("light"), nil
	default:
		return nil, fmt.Errorf("unknown sync mode %d", mode)
	}
}

func (mode *SyncMode) UnmarshalText(text []byte) error {
	switch string(text) {
	case "full":
		*mode = FullSync
	case "fast":
		*mode = FastSync
	case "light":
		*mode = LightSync
	default:
		return fmt.Errorf(`unknown sync mode %q, want "full", "fast" or "light"`, text)
	}
	return nil
}

type SynCtrl struct {
	fastSync  uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)
	acceptTxs uint32 // Flag whether we're considered synchronised (enables transaction processing)

	txpool      txPool //todo xinqyu's
	chaindb     hpbdb.Database
	chainconfig *params.ChainConfig
	maxPeers    int

	syncer      *syncer
	puller      *puller
	peers       *peerSet //todo

	SubProtocols []p2p.Protocol

	eventMux      *event.TypeMux
	txCh          chan core.TxPreEvent
	txSub         event.Subscription
	minedBlockSub *event.TypeMuxSubscription

	// channels for fetcher, syncer, txsyncLoop
	newPeerCh   chan *peer //todo
	txsyncCh    chan *txsync
	quitSync    chan struct{}
	noMorePeers chan struct{}

	// wait group is used for graceful shutdowns during downloading
	// and processing
	wg sync.WaitGroup
}

// NewSynCtrl returns a new block synchronization controller.
func NewSynCtrl(config *params.ChainConfig, mode SyncMode, networkId uint64, mux *event.TypeMux, txpool txPool,/*todo txpool*/
	engine consensus.Engine, chaindb hpbdb.Database) (*SynCtrl, error) {
	synctrl := &SynCtrl{
		eventMux:    mux,
		txpool:      txpool,
		chaindb:     chaindb,
		chainconfig: config,
		peers:       newPeerSet(),//todo
		newPeerCh:   make(chan *peer),//todo
		noMorePeers: make(chan struct{}),
		txsyncCh:    make(chan *txsync),
		quitSync:    make(chan struct{}),
	}

	if mode == FastSync && core.InstanceBlockChain().CurrentBlock().NumberU64() > 0 {
		log.Warn("Blockchain not empty, fast sync disabled")
		mode = FullSync
	}
	if mode == FastSync {
		synctrl.fastSync = uint32(1)
	}
	// Construct the different synchronisation mechanisms
	synctrl.syncer = NewSyncer(mode, chaindb, synctrl.eventMux, nil, synctrl.removePeer)//todo removePeer

	validator := func(header *types.Header) error {
		return engine.VerifyHeader(core.InstanceBlockChain(), header, true)
	}
	heighter := func() uint64 {
		return core.InstanceBlockChain().CurrentBlock().NumberU64()
	}
	inserter := func(blocks types.Blocks) (int, error) {
		// If fast sync is running, deny importing weird blocks
		if atomic.LoadUint32(&synctrl.fastSync) == 1 {
			log.Warn("Discarded bad propagated block", "number", blocks[0].Number(), "hash", blocks[0].Hash())
			return 0, nil
		}
		atomic.StoreUint32(&synctrl.acceptTxs, 1) // Mark initial sync done on any fetcher import
		return core.InstanceBlockChain().InsertChain(blocks)
	}
	synctrl.puller = NewPuller(core.InstanceBlockChain().GetBlockByHash, validator, synctrl.BroadcastBlock, heighter, inserter, synctrl.removePeer)//todo removerPeer

	return synctrl, nil
}

func (this *SynCtrl) Start() error {

}

func (this *SynCtrl) Stop() {

}

func (this *SynCtrl) removePeer(id string) {
	// Short circuit if the peer was already removed
	peer := this.peers.Peer(id)
	if peer == nil {
		return
	}
	log.Debug("Removing Hpb peer", "peer", id)

	// Unregister the peer from the downloader and Hpb peer set
	this.syncer.UnregisterPeer(id)
	if err := this.peers.Unregister(id); err != nil {
		log.Error("Peer removal failed", "peer", id, "err", err)
	}
	// Hard disconnect at the networking layer
	if peer != nil {
		peer.Peer.Disconnect(p2p.DiscUselessPeer)//todo qinghua's peer
	}
}