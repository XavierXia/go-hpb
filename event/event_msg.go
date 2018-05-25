package event

import (
	"github.com/hpb-project/ghpb/core/types"
	"github.com/hpb-project/go-hpb/txpool"
)

type ChainHeadEvent struct {
	Message *types.Block
}

type TxPreEvent struct {
	Tx *txpool.Transaction
}