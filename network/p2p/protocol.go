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
	"fmt"
	"errors"

	"github.com/hpb-project/ghpb/network/p2p/discover"
	"github.com/hpb-project/ghpb/network/rpc"
	"math/big"
	"github.com/hpb-project/ghpb/common"
	"gopkg.in/fatih/set.v0"
)

// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	// Name should contain the official protocol name,
	// often a three-letter word.
	Name string

	// Version should contain the version number of the protocol.
	Version uint

	// Length should contain the number of message codes used
	// by the protocol.
	Length uint64

	// Run is called in a new groutine when the protocol has been
	// negotiated with a peer. It should read and write messages from
	// rw. The Payload for each message must be fully consumed.
	//
	// The peer connection is closed when Start returns. It should return
	// any protocol-level error (such as an I/O error) that is
	// encountered.
	Run func(peer *Peer, rw MsgReadWriter) error

	// NodeInfo is an optional helper method to retrieve protocol specific metadata
	// about the host node.
	NodeInfo func() interface{}

	// PeerInfo is an optional helper method to retrieve protocol specific metadata
	// about a certain peer in the network. If an info retrieval function is set,
	// but returns nil, it is assumed that the protocol handshake is still running.
	PeerInfo func(id discover.NodeID) interface{}
}

func (p Protocol) cap() Cap {
	return Cap{p.Name, p.Version}
}

// Cap is the structure of a peer capability.
type Cap struct {
	Name    string
	Version uint
}

func (cap Cap) RlpData() interface{} {
	return []interface{}{cap.Name, cap.Version}
}

func (cap Cap) String() string {
	return fmt.Sprintf("%s/%d", cap.Name, cap.Version)
}

type capsByNameAndVersion []Cap

func (cs capsByNameAndVersion) Len() int      { return len(cs) }
func (cs capsByNameAndVersion) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }
func (cs capsByNameAndVersion) Less(i, j int) bool {
	return cs[i].Name < cs[j].Name || (cs[i].Name == cs[j].Name && cs[i].Version < cs[j].Version)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////
//HPB 协议
type Hpb struct {
	SubProtocols []Protocol
	//lesServer       LesServer
}
const ProtoHpbMaxMsg       = 10 * 1024 * 1024
const ProtoHpbV100    uint = 100

// Official short name of the protocol used during capability negotiation.
var ProtoName = "hpb"

// hpb protocol version control
var ProtoVersions = []uint{ProtoHpbV100}

// Number of implemented message corresponding to different protocol versions.
var ProtoLengths = []uint64{17}



// HPB 支持的协议消息
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


type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIdMismatch
	ErrGenesisBlockMismatch
	ErrNoStatusMsg
	ErrExtraStatusMsg
	ErrSuspendedPeer
)

func (s *Hpb) Protocols() []Protocol {
	//if s.lesServer != nil {
	//	append(s.SubProtocols, s.lesServer.Protocols()...)
	//}
	return s.SubProtocols
}

func (s *Hpb) Start(srvr *Server) error {

	// Initiate a sub-protocol for every implemented version we can handle
	s.SubProtocols = make([]Protocol, 0, len(ProtoVersions))
	for i, version := range ProtoVersions {
		version := version
		s.SubProtocols = append(s.SubProtocols, Protocol{
			Name:    ProtoName,
			Version: version,
			Length:  ProtoLengths[i],
			Run: func(p *Peer, rw MsgReadWriter) error {
				id := p.ID()
				p.version = version
				p.id      = fmt.Sprintf("%x", id[:8])
				p.knownTxs    = set.New()
				p.knownBlocks = set.New()
				return s.handle(p)
			},
			NodeInfo: func() interface{} {
				//TODO: 增加NodeInfo接口
				return s.NodeInfo()
			},
			PeerInfo: func(id discover.NodeID) interface{} {
				//TODO: 增加PeerInfo接口
				return nil
			},
		})
	}
	if len(s.SubProtocols) == 0 {
		return errors.New("SubProtocols incompatible configuration")
	}


	//if s.lesServer != nil {
	//	s.lesServer.Start(srvr)
	//}
	return nil
}

func (s *Hpb) Stop() error {
	//if s.lesServer != nil {
	//	s.lesServer.Stop()
	//}
	return nil
}


func (s *Hpb) APIs() []rpc.API {
	return nil
}


// HpbNodeInfo represents a short summary of the Hpb sub-protocol metadata known
// about the host peer.
type HpbNodeInfo struct {
	Network    uint64      `json:"network"`    // Hpb network ID (1=Frontier, 2=Morden, Ropsten=3)
	Difficulty *big.Int    `json:"difficulty"` // Total difficulty of the host's blockchain
	Genesis    common.Hash `json:"genesis"`    // SHA3 hash of the host's genesis block
	Head       common.Hash `json:"head"`       // SHA3 hash of the host's best owned block
}

// NodeInfo retrieves some protocol metadata about the running host node.
func (s *Hpb) NodeInfo() *HpbNodeInfo {
	/*
	currentBlock := self.blockchain.CurrentBlock()
	return &HpbNodeInfo{
		Network:    self.networkId,
		Difficulty: self.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64()),
		Genesis:    self.blockchain.Genesis().Hash(),
		Head:       currentBlock.Hash(),
	}
	*/
	return  nil
}

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}


// handle is the callback invoked to manage the life cycle of an eth peer. When
// this function terminates, the peer is disconnected.
func (s *Hpb) handle(p *Peer) error {
	p.Log().Debug("Peer connected", "name", p.Name())

	// Execute the Hpb handshake
	//TODO: 调用blockchain接口，获取状态信息
	/*
	networkId,td, head, genesis := blockchain.Status()
	if err := p.Handshake(networkId, td, head, genesis); err != nil {
		p.Log().Debug("Handshake failed", "err", err)
		return err
	}
	*/

	/*
	//peer层性能统计
	if rw, ok := p.rw.(*meteredMsgReadWriter); ok {
		rw.Init(p.version)
	}
	*/

	/*
	//注册到PeerSet
	// Register the peer locally
	if err := pm.peers.Register(p); err != nil {
		p.Log().Error("Hpb peer registration failed", "err", err)
		return err
	}
	defer pm.removePeer(p.id)
	*/

	// main loop. handle incoming messages.
	for {
		if err := s.handleMsg(p); err != nil {
			p.Log().Debug("Message handling failed", "err", err)
			return err
		}
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (s *Hpb) handleMsg(p *Peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtoHpbMaxMsg {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtoHpbMaxMsg)
	}
	defer msg.Discard()

	// Handle the message depending on its contents
	switch {
	case msg.Code == StatusMsg:
		// Status messages should never arrive after the handshake
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

		// Block header query, collect the requested headers and reply
	case msg.Code == GetBlockHeadersMsg:
		return nil

	case msg.Code == BlockHeadersMsg:
		return nil

	case msg.Code == GetBlockBodiesMsg:
		return nil

	case msg.Code == BlockBodiesMsg:
		return nil

	case msg.Code == GetNodeDataMsg:
		return nil

	case msg.Code == NodeDataMsg:
		return nil

	case msg.Code == GetReceiptsMsg:
		return nil

	case msg.Code == ReceiptsMsg:
		return nil

	case msg.Code == NewBlockHashesMsg:
		return nil

	case msg.Code == NewBlockMsg:
		return nil

	case msg.Code == TxMsg:
		return nil

	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}


