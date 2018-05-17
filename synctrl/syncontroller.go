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
)

type synctrl struct {
	peers map[string]*syncPeer
	sch   *scheduler
}

func New(sh *scheduler) *synctrl {
	return &synctrl{
		sch : sh,
	}
}

// RegisterPeer add a peer for synchronise
func (this *synctrl) RegisterPeer(id string, peer *syncPeer)  {
	this.peers[id] = peer
}

// UnRegisterPeer remove a peer for synchronise
func (this *synctrl) UnRegisterPeer(id string)  {
	delete(this.peers, id)
}

func (this *synctrl) Start() {

}

