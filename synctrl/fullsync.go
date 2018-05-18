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
//"crypto/rand"
//"errors"
//"fmt"
//"math"
//"math/big"
//"sync"
//"sync/atomic"
//"time"
//
//hpbinter "github.com/hpb-project/go-hpb/interface"
//"github.com/hpb-project/go-hpb/common"
//"github.com/hpb-project/go-hpb/data/types"
//"github.com/hpb-project/go-hpb/data/storage"
//"github.com/hpb-project/go-hpb/event"
//"github.com/hpb-project/go-hpb/common/log"
//"github.com/hpb-project/go-hpb/common/constant"
//"github.com/rcrowley/go-metrics"
)


type fullSyncSty struct {
	peerId   string
	dataHead chan packet
	dataBody chan packet
}

func cFullsync(id string) *fullSyncSty {
	syn := &fullSyncSty{
		dataHead: make(chan packet, 1),
		dataBody: make(chan packet, 1),
		peerId  : id,
	}

	return syn
}

func (full *fullSyncSty) start() error {

	err := errTimeout
	return err
}

func (full *fullSyncSty) stop() {

}