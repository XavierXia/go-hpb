package txpool

import (
	"testing"
	"github.com/hpb-project/ghpb/common"
	"github.com/hpb-project/ghpb/common/hexutil"
	"math/big"
	"github.com/btcsuite/btcd/btcjson"
)

func TestSubmitTx(t *testing.T) {
	sendTxArgs := SendTxArgs{
		From:     common.Address{},
		To:       &common.Address{},
		Gas:      (*hexutil.Big)(big.NewInt(0)),
		GasPrice: (*hexutil.Big)(big.NewInt(0)),
		Value:    (*hexutil.Big)(big.NewInt(0)),
		Data:     hexutil.Bytes([]byte{}),
		Nonce:    (*hexutil.Uint64)(btcjson.Uint64(1)),
	}
	hash, err := SubmitTx(sendTxArgs)
	if err != nil {
		t.Error("error SubmitTx",)
	}
	t.Log(hash)
}

func TestSubmitRawTx(t *testing.T) {
}
