package event

import (
	"github.com/hpb-project/ghpb/core/types"
)

type ChainHeadEvent struct {
	Message *types.Block
}

