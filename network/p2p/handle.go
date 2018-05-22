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

package p2p

import (
	"go/types"
	"github.com/hpb-project/ghpb/common"
	"github.com/hpb-project/ghpb/synccontroller/downloader"
	"encoding/json"
	"time"
	"github.com/hpb-project/ghpb/common/rlp"
	"math/big"
	"sync/atomic"
	"github.com/hpb-project/ghpb/common/constant"
	"github.com/hpb-project/ghpb/network/p2p/discover"
)


// Official short name of the protocol used during capability negotiation.
var ProtocolName = "hpb"
// Supported versions of the hpb protocol (first is primary).
var ProtocolVersions = []uint{params.ProtocolV111}

// Number of implemented message corresponding to different protocol versions.
var ProtocolLengths = []uint64{17}

const ProtocolMaxMsgSize = 10 * 1024 * 1024 // Maximum cap on the size of a protocol message

// eth protocol message codes
const (
	StatusMsg          = 0x00
	NewBlockHashesMsg  = 0x01
	TxMsg              = 0x02
	GetBlockHeadersMsg = 0x03
	BlockHeadersMsg    = 0x04
	GetBlockBodiesMsg  = 0x05
	BlockBodiesMsg     = 0x06
	NewBlockMsg        = 0x07

	GetNodeDataMsg     = 0x0d
	NodeDataMsg        = 0x0e
	GetReceiptsMsg     = 0x0f
	ReceiptsMsg        = 0x10
)

type HandleHpb struct {

}

// handle is the callback invoked to manage the life cycle of an eth peer. When
// this function terminates, the peer is disconnected.
func (hd *HandleHpb) handle(p *Peer) error {
	if pm.peers.Len() >= pm.maxPeers {
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("Hpb peer connected", "name", p.Name())

	//TODO 从BC获取当前链的状态，区块链状态握手
	td, head, genesis := blockchain.Status()
	if err := p.Handshake(pm.networkId, td, head, genesis); err != nil {
		p.Log().Debug("Hpb handshake failed", "err", err)
		return err
	}


	if rw, ok := p.rw.(*meteredMsgReadWriter); ok {
		rw.Init(p.version)
	}


	// Register the peer locally
	if err := pm.peers.Register(p); err != nil {
		p.Log().Error("Hpb peer registration failed", "err", err)
		return err
	}
	defer pm.removePeer(p.id)

	// main loop. handle incoming messages.
	for {
		if err := pm.handleMsg(p); err != nil {
			p.Log().Debug("Hpb message handling failed", "err", err)
			return err
		}
	}
}

//处理每个Peer的协议消息
func (hd *HandleHpb) handleMsg(p *Peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	// Handle the message depending on its contents
	switch {
	case msg.Code == StatusMsg:
		// Status messages should never arrive after the handshake
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

	case msg.Code == GetBlockHeadersMsg:

	case msg.Code == BlockHeadersMsg:

	case msg.Code == GetBlockBodiesMsg:

	case msg.Code == BlockBodiesMsg:

	case msg.Code == GetNodeDataMsg:

	case msg.Code == NodeDataMsg:

	case msg.Code == GetReceiptsMsg:

	case msg.Code == ReceiptsMsg:

	case msg.Code == NewBlockHashesMsg:

	case msg.Code == NewBlockMsg:

	case msg.Code == TxMsg:


	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}


