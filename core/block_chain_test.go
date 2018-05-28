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
)

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

	blockchain, _ := NewBlockChain(db, gspec.Config,solo.New())
	defer blockchain.Stop()

	blocks, _ := GenerateChain(gspec.Config, genesis, db, 4, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				tx,_:= types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), big.NewInt(21000), new(big.Int), nil), signer, key)
				tx.SetFrom(address)
				return tx,nil
			}
		)
		tx, err = basicTx(types.NewEIP155Signer(gspec.Config.ChainId))
		if err != nil {
			t.Fatal(err)
		}
		block.AddTx(tx)
	})

	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	block := blockchain.GetBlockByNumber(1)
	for _,tx := range block.Transactions() {
		t.Log(tx.String())
	}
}
