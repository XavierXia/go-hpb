package txpool

import (
	"testing"
)

func TestNewTxPool(t *testing.T) {
	poolConfig := TxPoolConfig{

	}
	NewTxPool(poolConfig)
}
