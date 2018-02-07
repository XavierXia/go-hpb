// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package eth implements the Hpbereum protocol.
package hpb

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/hpb-project/go-hpb/accounts"
	"github.com/hpb-project/go-hpb/common"
	"github.com/hpb-project/go-hpb/common/hexutil"
	"github.com/hpb-project/go-hpb/consensus"
	"github.com/hpb-project/go-hpb/consensus/clique"
	"github.com/hpb-project/go-hpb/consensus/hpbhash"
	"github.com/hpb-project/go-hpb/core"
	"github.com/hpb-project/go-hpb/core/bloombits"
	"github.com/hpb-project/go-hpb/core/types"
	"github.com/hpb-project/go-hpb/core/vm"
	"github.com/hpb-project/go-hpb/hpb/downloader"
	"github.com/hpb-project/go-hpb/hpb/filters"
	"github.com/hpb-project/go-hpb/hpb/gasprice"
	"github.com/hpb-project/go-hpb/hpbdb"
	"github.com/hpb-project/go-hpb/event"
	"github.com/hpb-project/go-hpb/internal/hpbapi"
	"github.com/hpb-project/go-hpb/log"
	"github.com/hpb-project/go-hpb/miner"
	"github.com/hpb-project/go-hpb/node"
	"github.com/hpb-project/go-hpb/p2p"
	"github.com/hpb-project/go-hpb/params"
	"github.com/hpb-project/go-hpb/rlp"
	"github.com/hpb-project/go-hpb/rpc"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
}

// Hpbereum implements the Hpbereum full node service.
type Hpbereum struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan  chan bool    // Channel for shutting down the ethereum
	stopDbUpgrade func() error // stop chain db sequential key upgrade

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb hpbdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	ApiBackend *HpbApiBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	hpberbase common.Address

	networkId     uint64
	netRPCService *hpbapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and hpberbase)
}

func (s *Hpbereum) AddLesServer(ls LesServer) {
	s.lesServer = ls
}

// New creates a new Hpbereum object (including the
// initialisation of the common Hpbereum object)
func New(ctx *node.ServiceContext, config *Config) (*Hpbereum, error) {
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run hpb.Hpbereum in light sync mode, use les.LightEthereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := CreateDB(ctx, config, "chaindata")
	if err != nil {
		return nil, err
	}
	stopDbUpgrade := upgradeDeduplicateData(chainDb)
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	hpb := &Hpbereum{
		config:         config,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, config, chainConfig, chainDb),
		shutdownChan:   make(chan bool),
		stopDbUpgrade:  stopDbUpgrade,
		networkId:      config.NetworkId,
		gasPrice:       config.GasPrice,
		hpberbase:      config.Hpberbase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	log.Info("Initialising Hpbereum protocol", "versions", ProtocolVersions, "network", config.NetworkId)

	if !config.SkipBcVersionCheck {
		bcVersion := core.GetBlockChainVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run geth upgradedb.\n", bcVersion, core.BlockChainVersion)
		}
		core.WriteBlockChainVersion(chainDb, core.BlockChainVersion)
	}

	vmConfig := vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
	hpb.blockchain, err = core.NewBlockChain(chainDb, hpb.chainConfig, hpb.engine, vmConfig)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		hpb.blockchain.SetHead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	hpb.bloomIndexer.Start(hpb.blockchain.CurrentHeader(), hpb.blockchain.SubscribeChainEvent)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	hpb.txPool = core.NewTxPool(config.TxPool, hpb.chainConfig, hpb.blockchain)

	if hpb.protocolManager, err = NewProtocolManager(hpb.chainConfig, config.SyncMode, config.NetworkId, hpb.eventMux, hpb.txPool, hpb.engine, hpb.blockchain, chainDb); err != nil {
		return nil, err
	}
	hpb.miner = miner.New(hpb, hpb.chainConfig, hpb.EventMux(), hpb.engine)
	hpb.miner.SetExtra(makeExtraData(config.ExtraData))

	hpb.ApiBackend = &HpbApiBackend{hpb, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	hpb.ApiBackend.gpo = gasprice.NewOracle(hpb.ApiBackend, gpoParams)

	return hpb, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"geth",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (hpbdb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*hpbdb.LDBDatabase); ok {
		db.Meter("hpb/db/chaindata/")
	}
	return db, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Hpbereum service
func CreateConsensusEngine(ctx *node.ServiceContext, config *Config, chainConfig *params.ChainConfig, db hpbdb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	if chainConfig.Clique != nil {
		return clique.New(chainConfig.Clique, db)
	}
	// Otherwise assume proof-of-work
	switch {
	case config.PowFake:
		log.Warn("Hpbhash used in fake mode")
		return hpbhash.NewFaker()
	case config.PowTest:
		log.Warn("Hpbhash used in test mode")
		return hpbhash.NewTester()
	case config.PowShared:
		log.Warn("Hpbhash used in shared mode")
		return hpbhash.NewShared()
	default:
		engine := hpbhash.New(ctx.ResolvePath(config.HpbhashCacheDir), config.HpbhashCachesInMem, config.HpbhashCachesOnDisk,
			config.HpbhashDatasetDir, config.HpbhashDatasetsInMem, config.HpbhashDatasetsOnDisk)
		engine.SetThreads(-1) // Disable CPU mining
		return engine
	}
}

// APIs returns the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Hpbereum) APIs() []rpc.API {
	apis := hpbapi.GetAPIs(s.ApiBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicHpbereumAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Hpbereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Hpbereum) Hpberbase() (eb common.Address, err error) {
	s.lock.RLock()
	hpberbase := s.hpberbase
	s.lock.RUnlock()

	if hpberbase != (common.Address{}) {
		return hpberbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			return accounts[0].Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("hpberbase address must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (self *Hpbereum) SetHpberbase(hpberbase common.Address) {
	self.lock.Lock()
	self.hpberbase = hpberbase
	self.lock.Unlock()

	self.miner.SetHpberbase(hpberbase)
}

func (s *Hpbereum) StartMining(local bool) error {
	eb, err := s.Hpberbase()
	if err != nil {
		log.Error("Cannot start mining without hpberbase", "err", err)
		return fmt.Errorf("hpberbase missing: %v", err)
	}
	if clique, ok := s.engine.(*clique.Clique); ok {
		wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("Hpberbase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		clique.Authorize(eb, wallet.SignHash)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so noone will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *Hpbereum) StopMining()         { s.miner.Stop() }
func (s *Hpbereum) IsMining() bool      { return s.miner.Mining() }
func (s *Hpbereum) Miner() *miner.Miner { return s.miner }

func (s *Hpbereum) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *Hpbereum) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *Hpbereum) TxPool() *core.TxPool               { return s.txPool }
func (s *Hpbereum) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Hpbereum) Engine() consensus.Engine           { return s.engine }
func (s *Hpbereum) ChainDb() hpbdb.Database            { return s.chainDb }
func (s *Hpbereum) IsListening() bool                  { return true } // Always listening
func (s *Hpbereum) EthVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *Hpbereum) NetVersion() uint64                 { return s.networkId }
func (s *Hpbereum) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Hpbereum) Protocols() []p2p.Protocol {
	if s.lesServer == nil {
		return s.protocolManager.SubProtocols
	}
	return append(s.protocolManager.SubProtocols, s.lesServer.Protocols()...)
}

// Start implements node.Service, starting all internal goroutines needed by the
// Hpbereum protocol implementation.
func (s *Hpbereum) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers()

	// Start the RPC service
	s.netRPCService = hpbapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if s.config.LightServ > 0 {
		maxPeers -= s.config.LightPeers
		if maxPeers < srvr.MaxPeers/2 {
			maxPeers = srvr.MaxPeers / 2
		}
	}
	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Hpbereum protocol.
func (s *Hpbereum) Stop() error {
	if s.stopDbUpgrade != nil {
		s.stopDbUpgrade()
	}
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.protocolManager.Stop()
	if s.lesServer != nil {
		s.lesServer.Stop()
	}
	s.txPool.Stop()
	s.miner.Stop()
	s.eventMux.Stop()

	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
