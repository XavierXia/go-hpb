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
	"github.com/hpb-project/go-hpb/common/log"
	hpbinter "github.com/hpb-project/go-hpb/interface"
	"github.com/hpb-project/go-hpb/storage"
	"github.com/rcrowley/go-metrics"
	"math/big"
	"sync"
	"sync/atomic"
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
	rttMinEstimate   = 2 * time.Second          // Minimum round-trip time to target for download requests
	rttMaxEstimate   = 20 * time.Second         // Maximum rount-trip time to target for download requests
	rttMinConfidence = 0.1                      // Worse confidence factor in our estimated RTT value
	ttlScaling       = 3                        // Constant scaling factor for RTT -> TTL conversion
	ttlLimit         = time.Minute              // Maximum TTL allowance to prevent reaching crazy timeouts

	qosTuningPeers   = 5    // Number of peers to tune based on (best peers)
	qosConfidenceCap = 10   // Number of peers above which not to modify RTT confidence
	qosTuningImpact  = 0.25 // Impact that a new tuning target has on the previous value

	maxQueuedHeaders  = 32 * 1024 // Maximum number of headers to queue for import (DOS protection)
	maxHeadersProcess = 2048      // Number of header download results to import at once into the chain
	maxResultsProcess = 2048      // Number of content download results to import at once into the chain

	fsHeaderCheckFrequency = 100        // Verification frequency of the downloaded headers during fast sync
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
	errNoPeers                 = errors.New("no peers to keep download active")
	errTimeout                 = errors.New("timeout")
	errEmptyHeaderSet          = errors.New("empty header set by peer")
	errPeersUnavailable        = errors.New("no peers available or all tried for download")
	errInvalidAncestor         = errors.New("retrieved ancestor is invalid")
	errInvalidChain            = errors.New("retrieved hash chain is invalid")
	errInvalidBlock            = errors.New("retrieved block is invalid")
	errInvalidBody             = errors.New("retrieved block body is invalid")
	errInvalidReceipt          = errors.New("retrieved receipt is invalid")
	errCancelBlockFetch        = errors.New("block download canceled (requested)")
	errCancelHeaderFetch       = errors.New("block header download canceled (requested)")
	errCancelBodyFetch         = errors.New("block body download canceled (requested)")
	errCancelReceiptFetch      = errors.New("receipt download canceled (requested)")
	errCancelStateFetch        = errors.New("state data download canceled (requested)")
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
	progress() hpbinter.SyncProgress
}

type Syncer struct {
	strategy   syncStrategy

	mode SyncMode       // Synchronisation mode defining the strategy used (per sync cycle)
	mux  *event.TypeMux // Event multiplexer to announce sync operation events

	sch   *scheduler   // Scheduler for selecting the hashes to syncer
	peers   *peerSet // Set of active peers from which download can proceed
	stateDB hpbdb.Database

	fsPivotLock  *types.Header // Pivot header on critical section entry (cannot change between retries)
	fsPivotFails uint32        // Number of subsequent fast sync failures in the critical section

	rttEstimate   uint64 // Round trip time to target for download requests
	rttConfidence uint64 // Confidence in the estimated RTT (unit: millionths to allow atomic ops)

	// Statistics
	syncStatsChainOrigin uint64 // Origin block number where syncing started at
	syncStatsChainHeight uint64 // Highest block number known when syncing started
	syncStatsState       stateSyncStats
	syncStatsLock        sync.RWMutex // Lock protecting the sync stats fields

	lightchain LightChain

	// Callbacks
	dropPeer peerDropFn // Drops a peer for misbehaving

	// Status
	synchroniseMock func(id string, hash common.Hash) error // Replacement for synchronise during testing
	synchronising   int32
	notified        int32

	// Channels
	headerCh      chan dataPack        // Channel receiving inbound block headers
	bodyCh        chan dataPack        // Channel receiving inbound block bodies
	receiptCh     chan dataPack        // Channel receiving inbound receipts
	bodyWakeCh    chan bool            // Channel to signal the block body fetcher of new tasks
	receiptWakeCh chan bool            // Channel to signal the receipt fetcher of new tasks
	headerProcCh  chan []*types.Header // Channel to feed the header processor new tasks

	// for stateFetcher
	stateSyncStart chan *stateSync
	trackStateReq  chan *stateReq
	stateCh        chan dataPack // Channel receiving inbound node state data

	// Cancellation and termination
	cancelPeer string        // Identifier of the peer currently being used as the master (cancel on drop)
	cancelCh   chan struct{} // Channel to cancel mid-flight syncs
	cancelLock sync.RWMutex  // Lock to protect the cancel channel and peer in delivers

	quitCh   chan struct{} // Quit channel to signal termination
	quitLock sync.RWMutex  // Lock to prevent double closes

	// Testing hooks
	syncInitHook     func(uint64, uint64)  // Method to call upon initiating a new sync run
	bodyFetchHook    func([]*types.Header) // Method to call upon starting a block body fetch
	receiptFetchHook func([]*types.Header) // Method to call upon starting a receipt fetch
	chainInsertHook  func([]*fetchResult)  // Method to call upon inserting a chain of blocks (possibly in multiple invocations)
}

func NewSyncer(mode SyncMode, stateDb hpbdb.Database, mux *event.TypeMux, lightchain LightChain,
	dropPeer peerDropFn) *Syncer {
	if lightchain == nil {
		lightchain = core.InstanceBlockChain()
	}
	syn := &Syncer{
		mode:           mode,
		stateDB:        stateDb,
		mux:            mux,
		sch:            newScheduler(),
		peers:          newPeerSet(),
		rttEstimate:    uint64(rttMaxEstimate),
		rttConfidence:  uint64(1000000),
		lightchain:     lightchain,
		dropPeer:       dropPeer,
		headerCh:       make(chan dataPack, 1),
		bodyCh:         make(chan dataPack, 1),
		receiptCh:      make(chan dataPack, 1),
		bodyWakeCh:     make(chan bool, 1),
		receiptWakeCh:  make(chan bool, 1),
		headerProcCh:   make(chan []*types.Header, 1),
		quitCh:         make(chan struct{}),
		stateCh:        make(chan dataPack),
		stateSyncStart: make(chan *stateSync),
		trackStateReq:  make(chan *stateReq),
	}
	switch mode {
	case FullSync:
		syn.strategy = NewFullsync(lightchain)
	case FastSync:
		syn.strategy = NewFastsync()
	default:
		syn.strategy = nil
	}
	go syn.qosTuner()
	go syn.stateFetcher()
	return syn
}

// Synchronise tries to sync up our local block chain with a remote peer, both
// adding various sanity checks as well as wrapping it with various log entries.
func (this *Syncer) Synchronise(id string, head common.Hash, td *big.Int, mode SyncMode) error {
	err := this.synchronise(id, head, td, mode)
	switch err {
	case nil:
	case errBusy:

	case errTimeout, errBadPeer, errStallingPeer,
		errEmptyHeaderSet, errPeersUnavailable, errProVLowerBase,
		errInvalidAncestor, errInvalidChain:
		log.Warn("Synchronisation failed, dropping peer", "peer", id, "err", err)
		this.dropPeer(id)

	default:
		log.Warn("Synchronisation failed, retrying", "err", err)
	}
	return err
}

// Terminate interrupts the syn, canceling all pending operations.
// The downloader cannot be reused after calling Terminate.
func (this *Syncer) Terminate() {
	// Close the termination channel (make sure double close is allowed)
	this.quitLock.Lock()
	select {
	case <-this.quitCh:
	default:
		close(this.quitCh)
	}
	this.quitLock.Unlock()

	// Cancel any pending download requests
	this.Cancel()
}

// Synchronising returns whether the syn is currently retrieving blocks.
func (this *Syncer) Synchronising() bool {
	return atomic.LoadInt32(&this.synchronising) > 0
}

// RegisterPeer injects a new syn peer into the set of block source to be
// used for fetching hashes and blocks from.
func (this *Syncer) RegisterPeer(id string, version uint, peer Peer) error {

	logger := log.New("peer", id)
	logger.Trace("Registering sync peer")
	if err := this.peers.Register(newPeerConnection(id, version, peer, logger)); err != nil {
		logger.Error("Failed to register sync peer", "err", err)
		return err
	}
	this.qosReduceConfidence()

	return nil
}

// RegisterLightPeer injects a light client peer, wrapping it so it appears as a regular peer.
func (this *Syncer) RegisterLightPeer(id string, version uint, peer LightPeer) error {
	return this.RegisterPeer(id, version, &lightPeerWrapper{peer})
}

// UnregisterPeer remove a peer from the known list, preventing any action from
// the specified peer. An effort is also made to return any pending fetches into
// the queue.
func (this *Syncer) UnregisterPeer(id string) error {
	// Unregister the peer from the active peer set and revoke any fetch tasks
	logger := log.New("peer", id)
	logger.Trace("Unregistering sync peer")
	if err := this.peers.Unregister(id); err != nil {
		logger.Error("Failed to unregister sync peer", "err", err)
		return err
	}
	this.sch.Revoke(id)

	// If this peer was the master peer, abort sync immediately
	this.cancelLock.RLock()
	master := id == this.cancelPeer
	this.cancelLock.RUnlock()

	if master {
		this.Cancel()
	}
	return nil
}

// Cancel cancels all of the operations and resets the scheduler. It returns true
// if the cancel operation was completed.
func (this *Syncer) Cancel() {
	// Close the current cancel channel
	this.cancelLock.Lock()
	if this.cancelCh != nil {
		select {
		case <-this.cancelCh:
			// Channel was already closed
		default:
			close(this.cancelCh)
		}
	}
	this.cancelLock.Unlock()
}

// DeliverHeaders injects a new batch of block headers received from a remote
// node into the syn schedule.
func (this *Syncer) DeliverHeaders(id string, headers []*types.Header) (err error) {
	return this.deliver(id, this.headerCh, &headerPack{id, headers}, headerInMeter, headerDropMeter)
}

// DeliverBodies injects a new batch of block bodies received from a remote node.
func (this *Syncer) DeliverBodies(id string, transactions [][]*types.Transaction, uncles [][]*types.Header) (err error) {
	return this.deliver(id, this.bodyCh, &bodyPack{id, transactions, uncles}, bodyInMeter, bodyDropMeter)
}

// DeliverReceipts injects a new batch of receipts received from a remote node.
func (this *Syncer) DeliverReceipts(id string, receipts [][]*types.Receipt) (err error) {
	return this.deliver(id, this.receiptCh, &receiptPack{id, receipts}, receiptInMeter, receiptDropMeter)
}

// DeliverNodeData injects a new batch of node state data received from a remote node.
func (this *Syncer) DeliverNodeData(id string, data [][]byte) (err error) {
	return this.deliver(id, this.stateCh, &statePack{id, data}, stateInMeter, stateDropMeter)
}

// deliver injects a new batch of data received from a remote node.
func (this *Syncer) deliver(id string, destCh chan dataPack, packet dataPack, inMeter, dropMeter metrics.Meter) (err error) {
	// Update the delivery metrics for both good and failed deliveries
	inMeter.Mark(int64(packet.Items()))
	defer func() {
		if err != nil {
			dropMeter.Mark(int64(packet.Items()))
		}
	}()
	// Deliver or abort if the sync is canceled while queuing
	this.cancelLock.RLock()
	cancel := this.cancelCh
	this.cancelLock.RUnlock()
	if cancel == nil {
		return errNoSyncActive
	}
	select {
	case destCh <- packet:
		return nil
	case <-cancel:
		return errNoSyncActive
	}
}

// stateFetcher manages the active state sync and accepts requests
// on its behalf.
func (this *Syncer) stateFetcher() {
	for {
		select {
		case s := <-this.stateSyncStart:
			for next := s; next != nil; {
				next = this.runStateSync(next)
			}
		case <-this.stateCh:
			// Ignore state responses while no sync is running.
		case <-this.quitCh:
			return
		}
	}
}

// syncState starts downloading state with the given root hash.
func (this *Syncer) syncState(root common.Hash) *stateSync {
	s := newStateSync(this, root)
	select {
	case this.stateSyncStart <- s:
	case <-this.quitCh:
		s.err = errCancelStateFetch
		close(s.done)
	}
	return s
}

// runStateSync runs a state synchronisation until it completes or another root
// hash is requested to be switched over to.
func (this *Syncer) runStateSync(s *stateSync) *stateSync {
	var (
		active   = make(map[string]*stateReq) // Currently in-flight requests
		finished []*stateReq                  // Completed or failed requests
		timeout  = make(chan *stateReq)       // Timed out active requests
	)
	defer func() {
		// Cancel active request timers on exit. Also set peers to idle so they're
		// available for the next sync.
		for _, req := range active {
			req.timer.Stop()
			req.peer.SetNodeDataIdle(len(req.items))
		}
	}()
	// Run the state sync.
	go s.run()
	defer s.Cancel()

	// Listen for peer departure events to cancel assigned tasks
	peerDrop := make(chan *peerConnection, 1024)
	peerSub := s.syn.peers.SubscribePeerDrops(peerDrop)
	defer peerSub.Unsubscribe()

	for {
		// Enable sending of the first buffered element if there is one.
		var (
			deliverReq   *stateReq
			deliverReqCh chan *stateReq
		)
		if len(finished) > 0 {
			deliverReq = finished[0]
			deliverReqCh = s.deliver
		}

		select {
		// The stateSync lifecycle:
		case next := <-this.stateSyncStart:
			return next

		case <-s.done:
			return nil

			// Send the next finished request to the current sync:
		case deliverReqCh <- deliverReq:
			finished = append(finished[:0], finished[1:]...)

			// Handle incoming state packs:
		case pack := <-this.stateCh:
			// Discard any data not requested (or previsouly timed out)
			req := active[pack.PeerId()]
			if req == nil {
				log.Debug("Unrequested node data", "peer", pack.PeerId(), "len", pack.Items())
				continue
			}
			// Finalize the request and queue up for processing
			req.timer.Stop()
			req.response = pack.(*statePack).states

			finished = append(finished, req)
			delete(active, pack.PeerId())

			// Handle dropped peer connections:
		case p := <-peerDrop:
			// Skip if no request is currently pending
			req := active[p.id]
			if req == nil {
				continue
			}
			// Finalize the request and queue up for processing
			req.timer.Stop()
			req.dropped = true

			finished = append(finished, req)
			delete(active, p.id)

			// Handle timed-out requests:
		case req := <-timeout:
			// If the peer is already requesting something else, ignore the stale timeout.
			// This can happen when the timeout and the delivery happens simultaneously,
			// causing both pathways to trigger.
			if active[req.peer.id] != req {
				continue
			}
			// Move the timed out data back into the download queue
			finished = append(finished, req)
			delete(active, req.peer.id)

			// Track outgoing state requests:
		case req := <-this.trackStateReq:
			// If an active request already exists for this peer, we have a problem. In
			// theory the trie node schedule must never assign two requests to the same
			// peer. In practive however, a peer might receive a request, disconnect and
			// immediately reconnect before the previous times out. In this case the first
			// request is never honored, alas we must not silently overwrite it, as that
			// causes valid requests to go missing and sync to get stuck.
			if old := active[req.peer.id]; old != nil {
				log.Warn("Busy peer assigned new state fetch", "peer", old.peer.id)

				// Make sure the previous one doesn't get siletly lost
				old.timer.Stop()
				old.dropped = true

				finished = append(finished, old)
			}
			// Start a timer to notify the sync loop if the peer stalled.
			req.timer = time.AfterFunc(req.timeout, func() {
				select {
				case timeout <- req:
				case <-s.done:
					// Prevent leaking of timer goroutines in the unlikely case where a
					// timer is fired just before exiting runStateSync.
				}
			})
			active[req.peer.id] = req
		}
	}
}

// qosTuner is the quality of service tuning loop that occasionally gathers the
// peer latency statistics and updates the estimated request round trip time.
func (this *Syncer) qosTuner() {
	for {
		// Retrieve the current median RTT and integrate into the previoust target RTT
		rtt := time.Duration(float64(1-qosTuningImpact)*float64(atomic.LoadUint64(&this.rttEstimate)) + qosTuningImpact*float64(this.peers.medianRTT()))
		atomic.StoreUint64(&this.rttEstimate, uint64(rtt))

		// A new RTT cycle passed, increase our confidence in the estimated RTT
		conf := atomic.LoadUint64(&this.rttConfidence)
		conf = conf + (1000000-conf)/2
		atomic.StoreUint64(&this.rttConfidence, conf)

		// Log the new QoS values and sleep until the next RTT
		log.Debug("Recalculated downloader QoS values", "rtt", rtt, "confidence", float64(conf)/1000000.0, "ttl", this.requestTTL())
		select {
		case <-this.quitCh:
			return
		case <-time.After(rtt):
		}
	}
}

// qosReduceConfidence is meant to be called when a new peer joins the downloader's
// peer set, needing to reduce the confidence we have in out QoS estimates.
func (this *Syncer) qosReduceConfidence() {
	// If we have a single peer, confidence is always 1
	peers := uint64(this.peers.Len())
	if peers == 0 {
		// Ensure peer connectivity races don't catch us off guard
		return
	}
	if peers == 1 {
		atomic.StoreUint64(&this.rttConfidence, 1000000)
		return
	}
	// If we have a ton of peers, don't drop confidence)
	if peers >= uint64(qosConfidenceCap) {
		return
	}
	// Otherwise drop the confidence factor
	conf := atomic.LoadUint64(&this.rttConfidence) * (peers - 1) / peers
	if float64(conf)/1000000 < rttMinConfidence {
		conf = uint64(rttMinConfidence * 1000000)
	}
	atomic.StoreUint64(&this.rttConfidence, conf)

	rtt := time.Duration(atomic.LoadUint64(&this.rttEstimate))
	log.Debug("Relaxed downloader QoS values", "rtt", rtt, "confidence", float64(conf)/1000000.0, "ttl", this.requestTTL())
}

// requestTTL returns the current timeout allowance for a single sync request
// to finish under.
func (this *Syncer) requestTTL() time.Duration {
	var (
		rtt  = time.Duration(atomic.LoadUint64(&this.rttEstimate))
		conf = float64(atomic.LoadUint64(&this.rttConfidence)) / 1000000.0
	)
	ttl := time.Duration(ttlScaling) * time.Duration(float64(rtt)/conf)
	if ttl > ttlLimit {
		ttl = ttlLimit
	}
	return ttl
}