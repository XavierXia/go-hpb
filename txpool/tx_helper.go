package txpool

import (
	"github.com/hpb-project/ghpb/common"
	"github.com/hpb-project/ghpb/common/hexutil"
	"github.com/hpb-project/ghpb/core/types"
)

// SendTxArgs represents the arguments to submit a new transaction into the transaction pool.
type SendTxArgs struct {
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Big    `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Data     hexutil.Bytes   `json:"data"`
	Nonce    *hexutil.Uint64 `json:"nonce"`
}

//SubmitTx try to submit transaction from local RPC call into tx_pool and return transaction's hash.
func SubmitTx(sendTxArgs SendTxArgs) (common.Hash, error) {
	//1.build Transaction object and set default value for nil arguments in sendTxArgs.
	//2.sign Transaction using local private keystore.
	//3.call tx_pool's addTx() push tx into tx_pool.
	//4.return the transaction's hash.
	return common.Hash{}, nil
}

//SubmitRawTx try to decode rlp data and submit transaction from remote RPC call into tx_pool and return transaction's hash.
func SubmitRawTx(encodedTx hexutil.Bytes) (common.Hash, error) {
	//1.decode raw transaction data to Transaction object.
	//2.call tx_pool's addTx() push tx into tx_pool.
	//3.return the transaction's hash.
	return common.Hash{}, nil
}

//SignTx use local keystore sign transaction.
func SignTx(transaction *types.Transaction){

}