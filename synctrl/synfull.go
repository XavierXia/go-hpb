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
	"math/big"
	"sync/atomic"
	"time"

	"github.com/hpb-project/go-hpb/blockchain"

	"github.com/hpb-project/go-hpb/common"
	"github.com/hpb-project/go-hpb/common/log"
)

type fullSync struct {
	dataHead chan packet
	dataBody chan packet

	notified int32
	running  int32
	quitchan chan struct{}
}

func NewFullsync(lightchain LightChain) *fullSync {
	return syn
}

func (this *fullSync) start(peer *peermanager.Peer) {

}

func (this *fullSync) stop() {
	if nil != this.quitchan {
		close(this.quitchan)
	}
}