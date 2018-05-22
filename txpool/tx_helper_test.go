package txpool

import (
	"testing"
	"github.com/hpb-project/ghpb/common/hexutil"
	"math/big"
	"github.com/btcsuite/btcd/btcjson"
	"fmt"
	"github.com/hpb-project/go-hpb/account/keystore"
)

func TestSubmitTx(t *testing.T) {
	am , _ , _ := MockAccountManager(false,"","")
	ks := am.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	account, err := ks.NewAccount("ABC")
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}
	fmt.Printf("Address From: {%x}\n", account.Address)
	ks.Unlock(account,"ABC")
	accountTo, err := ks.NewAccount("ABC")
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}
	fmt.Printf("Address To: {%x}\n", accountTo.Address)
	ks.Unlock(accountTo,"ABC")

	poolConfig := TxPoolConfig{

	}
	NewTxPool(poolConfig)

	sendTxArgs := SendTxArgs{
		From:     account.Address,
		To:       &accountTo.Address,
		Gas:      (*hexutil.Big)(big.NewInt(0)),
		GasPrice: (*hexutil.Big)(big.NewInt(0)),
		Value:    (*hexutil.Big)(big.NewInt(0)),
		Data:     hexutil.Bytes([]byte{}),
		Nonce:    (*hexutil.Uint64)(btcjson.Uint64(1)),
	}
	hash, err := SubmitTx(sendTxArgs)
	if err != nil {
		t.Error("error SubmitTx",err)
	}
	t.Log(hash)
}

func TestSubmitRawTx(t *testing.T) {
}
