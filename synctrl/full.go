// Copyright 2018 The go-hpb Authors
// This file is part of the go-hpb.
//
// The go-hpb is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-hpb is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-hpb. If not, see <http://www.gnu.org/licenses/>.

package synctrl

import (
	"github.com/hpb-project/go-hpb/data"
	"sync/atomic"
)


type fullSyncSty struct {
	dataHead chan packet
	dataBody chan packet
}

func cFullsync() *fullSyncSty {
	syn := &fullSyncSty{
		dataHead: make(chan packet, 1),
		dataBody: make(chan packet, 1),
	}

	return syn
}

func (this *fullSyncSty) start(peer *p2p.Peer) {
	if peer == nil {
		return
	}
	currentBlock := data.InstanceBlockChain().CurrentBlock()
	td := data.InstanceBlockChain().GetTd(currentBlock.Hash(), currentBlock.NumberU64())

	pHead, pTd := peer.Head()

	if pTd.Cmp(td) <= 0 {
		return
	}
	err := this.Synchronise(peer.id, pHead, pTd)
	if err != nil {
		return
	}
	atomic.StoreUint32(&pm.acceptTxs, 1)
	if head := data.InstanceBlockChain().CurrentBlock(); head.NumberU64() > 0 {
		go this.BroadcastBlock(head, false)
	}
}

func (full *fullSyncSty) stop() {

}