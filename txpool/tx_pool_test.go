package txpool

import (
	"testing"
	"math/big"
)

func TestNewTxPool(t *testing.T) {
	poolConfig := TxPoolConfig{

	}
	NewTxPool(poolConfig,big.NewInt(1))
}
