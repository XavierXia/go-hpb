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

package txpool

import (
	"github.com/hpb-project/ghpb/core/types"
	"sync"
	"github.com/hpb-project/ghpb/common"
)

type TxPool struct {
	wg     sync.WaitGroup
	stopCh chan struct{}
}

//Create the transaction pool and start main process loop.
func (pool *TxPool) NewTxPool() {
	//1.Sanitize the input to ensure no vulnerable gas prices are set
	//2.Create the transaction pool with its initial settings
	//3.start main process loop
	pool.wg.Add(1)
	go pool.loop()
}

//Stop the transaction pool.
func (pool *TxPool) Stop() {
	//1.stop main process loop
	pool.stopCh <- struct{}{}
	//2.wait quit
	pool.wg.Wait()
}

//Main process loop.
func (pool *TxPool) loop() {

}

// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *TxPool) AddTx(transaction types.Transaction) (bool, error) {

	return true, nil
}

// Pending retrieves all currently processable transactions, groupped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) Pending() (map[common.Address]types.Transactions, error) {

	return nil, nil
}

// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPool) validateTx(tx *types.Transaction) error {

	return nil
}

// promoteTx adds a transaction to the pending (processable) list of transactions.
//
// Note, this method assumes the pool lock is held!
func (pool *TxPool) promoteTx(addr common.Address, hash common.Hash, tx *types.Transaction) {

}