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
	"errors"
	"fmt"
	"github.com/hpb-project/go-hpb/blockchain"
	"github.com/hpb-project/go-hpb/blockchain/event"
	"github.com/hpb-project/go-hpb/blockchain/types"
	"github.com/hpb-project/go-hpb/common"
	"github.com/hpb-project/go-hpb/common/constant"
	hpbinter "github.com/hpb-project/go-hpb/interface"
	"github.com/hpb-project/go-hpb/storage"
	"math/big"
	"time"
)

var (
	MaxHashFetch    = 512 // Amount of hashes to be fetched per retrieval request
	MaxBlockFetch   = 128 // Amount of blocks to be fetched per retrieval request
	MaxHeaderFetch  = 192 // Amount of block headers to be fetched per retrieval request
	MaxSkeletonSize = 128 // Number of header fetches to need for a skeleton assembly
	MaxBodyFetch    = 128 // Amount of block bodies to be fetched per retrieval request
	MaxReceiptFetch = 256 // Amount of transaction receipts to allow fetching per request
	MaxStateFetch   = 384 // Amount of node state values to allow fetching per request

	MaxForkAncestry  = 3 * params.EpochDuration // Maximum chain reorganisation
	rttMinEstimate   = 2 * time.Second          // Minimum round-trip time to target for sync requests
	rttMaxEstimate   = 20 * time.Second         // Maximum rount-trip time to target for sync requests
	rttMinConfidence = 0.1                      // Worse confidence factor in our estimated RTT value
	ttlScaling       = 3                        // Constant scaling factor for RTT -> TTL conversion
	ttlLimit         = time.Minute              // Maximum TTL allowance to prevent reaching crazy timeouts

	qosTuningPeers   = 5    // Number of peers to tune based on (best peers)
	qosConfidenceCap = 10   // Number of peers above which not to modify RTT confidence
	qosTuningImpact  = 0.25 // Impact that a new tuning target has on the previous value

	maxQueuedHeaders  = 32 * 1024 // Maximum number of headers to queue for import (DOS protection)
	maxHeadersProcess = 2048      // Number of header sync results to import at once into the chain
	maxResultsProcess = 2048      // Number of content sync results to import at once into the chain

	fsHeaderCheckFrequency = 100        // Verification frequency of the sync headers during fast sync
	fsHeaderSafetyNet      = 2048       // Number of headers to discard in case a chain violation is detected
	fsHeaderForceVerify    = 24         // Number of headers to verify before and after the pivot to accept it
	fsPivotInterval        = 256        // Number of headers out of which to randomize the pivot point
	fsMinFullBlocks        = 64         // Number of blocks to retrieve fully even in fast sync
	fsCriticalTrials       = uint32(32) // Number of times to retry in the cricical section before bailing
)

var (
	errBusy                    = errors.New("busy")
	errUnknownPeer             = errors.New("peer is unknown or unhealthy")
	errBadPeer                 = errors.New("action from bad peer ignored")
	errStallingPeer            = errors.New("peer is stalling")
	errNoPeers                 = errors.New("no peers to keep sync active")
	errTimeout                 = errors.New("timeout")
	errEmptyHeaderSet          = errors.New("empty header set by peer")
	errPeersUnavailable        = errors.New("no peers available or all tried for sync")
	errInvalidAncestor         = errors.New("retrieved ancestor is invalid")
	errInvalidChain            = errors.New("retrieved hash chain is invalid")
	errInvalidBlock            = errors.New("retrieved block is invalid")
	errInvalidBody             = errors.New("retrieved block body is invalid")
	errInvalidReceipt          = errors.New("retrieved receipt is invalid")
	errCancelBlockFetch        = errors.New("block sync canceled (requested)")
	errCancelHeaderFetch       = errors.New("block header sync canceled (requested)")
	errCancelBodyFetch         = errors.New("block body sync canceled (requested)")
	errCancelReceiptFetch      = errors.New("receipt sync canceled (requested)")
	errCancelStateFetch        = errors.New("state data sync canceled (requested)")
	errCancelHeaderProcessing  = errors.New("header processing canceled (requested)")
	errCancelContentProcessing = errors.New("content processing canceled (requested)")
	errNoSyncActive            = errors.New("no sync active")
	errProVLowerBase           = errors.New(fmt.Sprintf("peer is lower than the current baseline version (need Minimum version >= %d)", params.ProtocolV111))
	errTooOld                  = errors.New("peer doesn't speak recent enough protocol version (need version >= 62)")
)

// LightChain encapsulates functions required to synchronise a light chain.
type LightChain interface {
	// HasHeader verifies a header's presence in the local chain.
	HasHeader(h common.Hash, number uint64) bool

	// GetHeaderByHash retrieves a header from the local chain.
	GetHeaderByHash(common.Hash) *types.Header

	// CurrentHeader retrieves the head header from the local chain.
	CurrentHeader() *types.Header

	// GetTdByHash returns the total difficulty of a local block.
	GetTdByHash(common.Hash) *big.Int

	// InsertHeaderChain inserts a batch of headers into the local chain.
	InsertHeaderChain([]*types.Header, int) (int, error)

	// Rollback removes a few recently added elements from the local chain.
	Rollback([]common.Hash)
}

// BlockChain encapsulates functions required to sync a (full or fast) blockchain.
type BlockChain interface {
	LightChain

	// HasBlockAndState verifies block and associated states' presence in the local chain.
	HasBlockAndState(common.Hash) bool

	// GetBlockByHash retrieves a block from the local chain.
	GetBlockByHash(common.Hash) *types.Block

	// CurrentBlock retrieves the head block from the local chain.
	CurrentBlock() *types.Block

	// CurrentFastBlock retrieves the head fast block from the local chain.
	CurrentFastBlock() *types.Block

	// FastSyncCommitHead directly commits the head block to a certain entity.
	FastSyncCommitHead(common.Hash) error

	// InsertChain inserts a batch of blocks into the local chain.
	InsertChain(types.Blocks) (int, error)

	// InsertReceiptChain inserts a batch of receipts into the local chain.
	InsertReceiptChain(types.Blocks, []types.Receipts) (int, error)
}

type syncStrategy interface {
	deliverHeaders(id string, headers []*types.Header) (err error)
	deliverBodies(id string, transactions [][]*types.Transaction, uncles [][]*types.Header) (err error)
	deliverReceipts(id string, receipts [][]*types.Receipt) (err error)
	deliverNodeData(id string, data [][]byte) (err error)

	start(id string, head common.Hash, td *big.Int, mode SyncMode) error
	cancel()
	terminate()
	progress() hpbinter.SyncProgress
	syning() bool

	registerPeer(id string, version uint, peer Peer) error
	registerLightPeer(id string, version uint, peer LightPeer) error
	unregisterPeer(id string) error
}
type Syncer struct {
	mode       SyncMode       // Synchronisation mode defining the strategy used (per sync cycle)
	strategy   syncStrategy
}

func NewSyncer(mode SyncMode, stateDb hpbdb.Database, mux *event.TypeMux, lightchain LightChain,
	dropPeer peerDropFn) *Syncer {
	if lightchain == nil {
		lightchain = core.InstanceBlockChain()
	}
	syn := &Syncer{
		mode:           mode,
	}
	switch mode {
	case FullSync:
		syn.strategy = newFullsync(stateDb, mux, lightchain, dropPeer)
	case FastSync:
		syn.strategy = newFastsync(stateDb, mux, lightchain, dropPeer)
	case LightSync:
		//syn.strategy = newLightsync(syn)
	default:
		syn.strategy = nil
	}
	return syn
}

// Start tries to sync up our local block chain with a remote peer, both
// adding various sanity checks as well as wrapping it with various log entries.
func (this *Syncer) Start(id string, head common.Hash, td *big.Int, mode SyncMode) error {
	return this.strategy.start(id, head, td, mode)
}

// Terminate interrupts the syn, canceling all pending operations.
// The syncer cannot be reused after calling Terminate.
func (this *Syncer) Terminate() {
	this.strategy.terminate()
}

// Synchronising returns whether the syn is currently retrieving blocks.
func (this *Syncer) Synchronising() bool {
	return this.strategy.syning()
}

// RegisterPeer injects a new syn peer into the set of block source to be
// used for fetching hashes and blocks from.
func (this *Syncer) RegisterPeer(id string, version uint, peer Peer) error {
	return this.strategy.registerPeer(id, version, peer)
}

// RegisterLightPeer injects a light client peer, wrapping it so it appears as a regular peer.
func (this *Syncer) RegisterLightPeer(id string, version uint, peer LightPeer) error {
	return this.RegisterPeer(id, version, &lightPeerWrapper{peer})
}

// UnregisterPeer remove a peer from the known list, preventing any action from
// the specified peer. An effort is also made to return any pending fetches into
// the queue.
func (this *Syncer) UnregisterPeer(id string) error {
	return this.strategy.unregisterPeer(id)
}

// Cancel cancels all of the operations and resets the scheduler. It returns true
// if the cancel operation was completed.
func (this *Syncer) Cancel() {
	this.strategy.cancel()
}

// DeliverHeaders injects a new batch of block headers received from a remote
// node into the syn schedule.
func (this *Syncer) DeliverHeaders(id string, headers []*types.Header) (err error) {
	return this.strategy.deliverHeaders(id, headers)
}

// DeliverBodies injects a new batch of block bodies received from a remote node.
func (this *Syncer) DeliverBodies(id string, transactions [][]*types.Transaction, uncles [][]*types.Header) (err error) {
	return this.strategy.deliverBodies(id, transactions, uncles)
}

// DeliverReceipts injects a new batch of receipts received from a remote node.
func (this *Syncer) DeliverReceipts(id string, receipts [][]*types.Receipt) (err error) {
	return this.strategy.deliverReceipts(id, receipts)
}

// DeliverNodeData injects a new batch of node state data received from a remote node.
func (this *Syncer) DeliverNodeData(id string, data [][]byte) (err error) {
	return this.strategy.deliverNodeData(id, data)
}