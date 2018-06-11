package core

import (
	"github.com/hpb-project/go-hpb/common"
	"github.com/hpb-project/go-hpb/common/crypto"
	"github.com/hpb-project/go-hpb/config"
	"github.com/hpb-project/go-hpb/consensus/solo"
	"github.com/hpb-project/go-hpb/storage"
	"github.com/hpb-project/go-hpb/types"
	"math/big"
	"testing"
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
			Config: config.TestnetChainConfig,
			Alloc:  GenesisAlloc{address: {Balance: funds}, deleteAddr: {Balance: new(big.Int)}},
		}
		genesis = gspec.MustCommit(db)
	)

	blockchain, _ := NewBlockChain(db, gspec.Config, solo.New())
	defer blockchain.Stop()
	var blockCount = 1
	code := common.Hex2Bytes("608060405234801561001057600080fd5b50610100806100206000396000f300608060405260043610603f576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806382ab890a146044575b600080fd5b348015604f57600080fd5b50606c6004803603810190808035906020019092919050505060b5565b604051808373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018281526020019250505060405180910390f35b60008082600080828254019250508190555033600054915091509150915600a165627a7a723058200ac6aed3b33cc2913fdb245e249637ca0fa4b3c9c3c9bd3f535e3f00f1a707b30029")
	blocks, _ := GenerateChain(gspec.Config, genesis, db, blockCount, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				//tx, _ := types.SignTx(types.NewContractCreation(block.TxNonce(address),new(big.Int), big.NewInt(100000),new(big.Int), code), signer, key)
				tx := types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), big.NewInt(21000), new(big.Int), nil)
				tx.SetFrom(address)
				return tx, nil
			}
			_ = func(signer types.Signer) (*types.Transaction, error) {
				tx, _ := types.SignTx(types.NewContractCreation(block.TxNonce(address), new(big.Int), big.NewInt(100000), new(big.Int), code), signer, key)
				//tx, _ := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), big.NewInt(21000), new(big.Int), nil), signer, key)
				tx.SetFrom(address)
				return tx, nil
			}
			//testContractAddr = crypto.CreateAddress(acc1Addr, nonce)
		)
		//var txSlice = make([]*types.Transaction,0,10000)
		for i = 0; i < 50000; i++ {
			tx, err = basicTx(types.NewBoeSigner(gspec.Config.ChainId))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		}
	})
	var start = time.Now()
	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	t.Logf("insert %d blocks cost %s \n", blockCount, common.PrettyDuration(time.Since(start)).String())
	block := blockchain.GetBlockByNumber(1)
	t.Logf("block 1 size : %s, tx count : %d \n", block.Size().String(), block.Transactions().Len())
}
