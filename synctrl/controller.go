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
	"github.com/hpb-project/ghpb/network/p2p"
	"time"
)

const (
	syncInterval        = 10 * time.Second // Time interval to syncs
)

type synctrl struct {
	sch     *scheduler
	syn     []*sync
	newPeer chan interface{}
}

func New(sh *scheduler) *synctrl {
	ctrl := &synctrl{
		sch    : sh,
		newPeer: make(chan interface{}),
	}

	return ctrl
}

func (this *synctrl) Start() error {
	defer this.Stop()

	intervalSync := time.NewTicker(syncInterval)
	defer intervalSync.Stop()
	for {
		select {
		case peer := <-this.newPeer:
			p, ok := peer.(*p2p.Peer) if ok {
				go this.syn.start(p)
			}
		case <-intervalSync.C:
			// Force a sync even if not enough peers are present
			go this.syn.start(pm.peers.BestPeer())
		}
	}
}

func (this *synctrl) Stop() {
	close(this.newPeer)
}