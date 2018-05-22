package txpool

import (
	"github.com/hpb-project/ghpb/core"
	"github.com/hpb-project/ghpb/storage"
	"github.com/hpb-project/ghpb/common/constant"
	"math/big"
	"github.com/hpb-project/ghpb/consensus/prometheus"
	"github.com/hpb-project/ghpb/core/vm"
	"github.com/hpb-project/ghpb/core/state"
	"github.com/hpb-project/ghpb/core/types"
	"path/filepath"
	"io/ioutil"
	"os"
	"github.com/hpb-project/go-hpb/account"
	"github.com/hpb-project/go-hpb/account/keystore"
)

func MockBlockChain() *core.BlockChain {
	db, _ := hpbdb.NewMemDatabase()
	gspec := &core.Genesis{
		Config:     params.TestnetChainConfig,
		Difficulty: big.NewInt(1),
	}
	gspec.MustCommit(db)
	engine := prometheus.New(params.TestnetChainConfig.Prometheus,db)
	blockchain, err := core.NewBlockChain(db, gspec.Config, engine, vm.Config{})
	if err != nil {
		panic(err)
	}
	blockchain.SetValidator(bproc{})
	return blockchain
}

type bproc struct{}

func (bproc) ValidateBody(*types.Block) error { return nil }
func (bproc) ValidateState(block, parent *types.Block, state *state.StateDB, receipts types.Receipts, usedGas *big.Int) error {
	return nil
}
func (bproc) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, *big.Int, error) {
	return nil, nil, new(big.Int), nil
}

var datadirDefaultKeyStore = "keystore"           // Path within the datadir to the keystore

func MockAccountManager(useLightweightKDF bool,keyStoreDir string,dataDir string) (*accounts.Manager, string, error) {
	scryptN := keystore.StandardScryptN
	scryptP := keystore.StandardScryptP
	if useLightweightKDF {
		scryptN = keystore.LightScryptN
		scryptP = keystore.LightScryptP
	}

	var (
		keydir    string
		ephemeral string
		err       error
	)
	switch {
	case filepath.IsAbs(keyStoreDir):
		keydir = keyStoreDir
	case dataDir != "":
		if keyStoreDir == "" {
			keydir = filepath.Join(dataDir, datadirDefaultKeyStore)
		} else {
			keydir, err = filepath.Abs(keyStoreDir)
		}
	case keyStoreDir != "":
		keydir, err = filepath.Abs(keyStoreDir)
	default:
		// There is no datadir.
		keydir, err = ioutil.TempDir("", "ghpb-keystore")
		ephemeral = keydir
	}
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(keydir, 0700); err != nil {
		return nil, "", err
	}
	// Assemble the account manager and supported backends
	backends := []accounts.Backend{
		keystore.NewKeyStore(keydir, scryptN, scryptP),
	}
	return accounts.NewManager(backends...), ephemeral, nil
}