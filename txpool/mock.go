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
