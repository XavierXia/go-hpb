package core

import (
	"testing"
	"math/big"
	"github.com/hpb-project/ghpb/storage"
	"github.com/hpb-project/ghpb/common/crypto"
	"github.com/hpb-project/ghpb/common"
	"github.com/hpb-project/ghpb/common/constant"
	"github.com/hpb-project/go-hpb/types"
	"github.com/hpb-project/go-hpb/consensus/solo"
	"time"
)

//go test -v -test.run TestApplyTx -cpuprofile ApplyTx_30000TPS.out
func TestApplyTx(t *testing.T) {

	// Configure and generate a sample block chain
	var (
		db, _      = hpbdb.NewMemDatabase()
		key, _     = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address    = crypto.PubkeyToAddress(key.PublicKey)
		funds      = big.NewInt(1000000000)
		deleteAddr = common.Address{1}
		gspec      = &Genesis{
			Config: params.TestnetChainConfig,
			Alloc:  GenesisAlloc{address: {Balance: funds}, deleteAddr: {Balance: new(big.Int)}},
		}
		genesis = gspec.MustCommit(db)
	)

	blockchain, _ := NewBlockChain(db, gspec.Config, solo.New())
	defer blockchain.Stop()
	var blockCount = 1
	code    := common.Hex2Bytes("60606040525b7f24ec1d3ff24c2f6ff210738839dbc339cd45a5294d85c79361016243157aae7b60405180905060405180910390a15b600a8060416000396000f360606040526008565b00")
	blocks, _ := GenerateChain(gspec.Config, genesis, db, blockCount, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				tx, _ := types.SignTx(types.NewContractCreation(block.TxNonce(address),new(big.Int), big.NewInt(100000),new(big.Int), code), signer, key)
				//tx, _ := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), big.NewInt(21000), new(big.Int), nil), signer, key)
				tx.SetFrom(address)
				return tx, nil
			}
		)
		//var txSlice = make([]*types.Transaction,0,10000)
		for i = 0; i < 1; i++ {
			tx, err = basicTx(types.NewBoeSigner(gspec.Config.ChainId))
			if err != nil {
				t.Fatal(err)
			}
			//txSlice = append(txSlice,tx)
			block.AddTx(tx)
		}
	})
	var start = time.Now()
	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	t.Logf("insert %d blocks cost %s \n", blockCount, common.PrettyDuration(time.Since(start)).String())
	block := blockchain.GetBlockByNumber(1)
	t.Logf("block 1 size : %s, tx count : %d \n",block.Size().String(),block.Transactions().Len())
}
