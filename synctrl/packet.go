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
	"github.com/hpb-project/go-hpb/blockchain/types"
)

const (
	Pk_Header          = 0x00
	Pk_Body            = 0x01
	Pk_Receipt         = 0x02
	Pk_State           = 0x03
)

type pkFactory struct {
}

func (this pkFactory) Create(tp int) packet {
	switch tp {
	case Pk_Header:
		return new(pk_header)
	case Pk_Body:
		return new(pk_body)
	case Pk_Receipt:
		return new(pk_receipt)
	case Pk_State:
		return new(pk_state)
	default:
		return nil
	}
}

type packet interface {
	PeerId() string
	Items() int
}

// headerPack is a batch of block headers returned by a peer.
type pk_header struct {
	peerId  string
	headers []*types.Header
}

func (p *pk_header) PeerId() string { return p.peerId }
func (p *pk_header) Items() int     { return len(p.headers) }

// bodyPack is a batch of block bodies returned by a peer.
type pk_body struct {
	peerId       string
	transactions [][]*types.Transaction
	uncles       [][]*types.Header
}

func (p *pk_body) PeerId() string { return p.peerId }
func (p *pk_body) Items() int {
	if len(p.transactions) <= len(p.uncles) {
		return len(p.transactions)
	}
	return len(p.uncles)
}

// receiptPack is a batch of receipts returned by a peer.
type pk_receipt struct {
	peerId   string
	receipts [][]*types.Receipt
}

func (p *pk_receipt) PeerId() string { return p.peerId }
func (p *pk_receipt) Items() int     { return len(p.receipts) }

// statePack is a batch of states returned by a peer.
type pk_state struct {
	peerId string
	states [][]byte
}

func (p *pk_state) PeerId() string { return p.peerId }
func (p *pk_state) Items() int     { return len(p.states) }