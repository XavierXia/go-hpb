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
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"
	"github.com/hpb-project/ghpb/common"
	"github.com/hpb-project/ghpb/core/state"
	"github.com/hpb-project/ghpb/core/types"
	"github.com/hpb-project/ghpb/common/crypto"
	"github.com/hpb-project/ghpb/storage"
	"github.com/hpb-project/ghpb/core/event"
	"github.com/hpb-project/ghpb/common/constant"
	"github.com/hpb-project/ghpb/core"
	"math/rand"
)

// testTxPoolConfig is a transaction pool configuration without stateful disk
// sideeffects used during testing.
var testTxPoolConfig TxPoolConfig

func init() {
	testTxPoolConfig = DefaultTxPoolConfig
}

type testBlockChain struct {
	statedb       *state.StateDB
	gasLimit      *big.Int
	chainHeadFeed *event.Feed
}

func (bc *testBlockChain) CurrentBlock() *types.Block {
	return types.NewBlock(&types.Header{
		GasLimit: bc.gasLimit,
	}, nil, nil, nil)
}

func (bc *testBlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return bc.CurrentBlock()
}

func (bc *testBlockChain) StateAt(common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func transaction(nonce uint64, gaslimit *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	return pricedTransaction(nonce, gaslimit, big.NewInt(1), key)
}

func pricedTransaction(nonce uint64, gaslimit, gasprice *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	tx, _ := types.SignTx(types.NewTransaction(nonce, common.Address{}, big.NewInt(100), gaslimit, gasprice, nil), types.HomesteadSigner{}, key)
	return tx
}

func setupTxPool() (*TxPool, *ecdsa.PrivateKey) {
	db, _ := hpbdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	blockchain := &testBlockChain{statedb, big.NewInt(1000000), new(event.Feed)}

	key, _ := crypto.GenerateKey()
	pool := NewTxPool(testTxPoolConfig, params.TestnetChainConfig, blockchain)

	return pool, key
}

// validateTxPoolInternals checks various consistency invariants within the pool.
func validateTxPoolInternals(pool *TxPool) error {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	// Ensure the total transaction set is consistent with pending + queued
	pending, queued := pool.Stats()
	if total := len(pool.all); total != pending+queued {
		return fmt.Errorf("total transaction count %d != %d pending + %d queued", total, pending, queued)
	}

	// Ensure the next nonce to assign is the correct one
	for addr, txs := range pool.pending {
		// Find the last transaction
		var last uint64
		for nonce, _ := range txs.txs.items {
			if last < nonce {
				last = nonce
			}
		}
		if nonce := pool.pendingState.GetNonce(addr); nonce != last+1 {
			return fmt.Errorf("pending nonce mismatch: have %v, want %v", nonce, last+1)
		}
	}
	return nil
}

func deriveSender(tx *types.Transaction) (common.Address, error) {
	return types.Sender(types.HomesteadSigner{}, tx)
}

type testChain struct {
	*testBlockChain
	address common.Address
	trigger *bool
}

// testChain.State() is used multiple times to reset the pending state.
// when simulate is true it will create a state that indicates
// that tx0 and tx1 are included in the chain.
func (c *testChain) State() (*state.StateDB, error) {
	// delay "state change" by one. The tx pool fetches the
	// state multiple times and by delaying it a bit we simulate
	// a state change between those fetches.
	stdb := c.statedb
	if *c.trigger {
		db, _ := hpbdb.NewMemDatabase()
		c.statedb, _ = state.New(common.Hash{}, state.NewDatabase(db))
		// simulate that the new head block included tx0 and tx1
		c.statedb.SetNonce(c.address, 2)
		c.statedb.SetBalance(c.address, new(big.Int).SetUint64(params.Ether))
		*c.trigger = false
	}
	return stdb, nil
}

func TestAddTx(t *testing.T) {
	var (
		db, _      = hpbdb.NewMemDatabase()
		key, _     = crypto.GenerateKey()
		address    = crypto.PubkeyToAddress(key.PublicKey)
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(db))
		trigger    = false
	)

	// setup pool with 2 transaction in it
	statedb.SetBalance(address, new(big.Int).SetUint64(params.Ether))
	blockchain := &testChain{&testBlockChain{statedb, big.NewInt(1000000000), new(event.Feed)}, address, &trigger}

	tx0 := transaction(0, big.NewInt(100000), key)
	tx1 := transaction(1, big.NewInt(100000), key)

	pool := NewTxPool(testTxPoolConfig, params.TestnetChainConfig, blockchain)
	defer pool.Stop()

	nonce := pool.State().GetNonce(address)
	if nonce != 0 {
		t.Fatalf("Invalid nonce, want 0, got %d", nonce)
	}

	pool.AddTxs(types.Transactions{tx0, tx1})

	nonce = pool.State().GetNonce(address)
	if nonce != 2 {
		t.Fatalf("Invalid nonce, want 2, got %d", nonce)
	}
}

// This test simulates a scenario where a new block is imported during a
// state reset and tests whether the pending state is in sync with the
// block head event that initiated the resetState().
func TestStateChangeDuringPoolReset(t *testing.T) {
	var (
		db, _      = hpbdb.NewMemDatabase()
		key, _     = crypto.GenerateKey()
		address    = crypto.PubkeyToAddress(key.PublicKey)
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(db))
		trigger    = false
	)

	// setup pool with 2 transaction in it
	statedb.SetBalance(address, new(big.Int).SetUint64(params.Ether))
	blockchain := &testChain{&testBlockChain{statedb, big.NewInt(1000000000), new(event.Feed)}, address, &trigger}

	tx0 := transaction(0, big.NewInt(100000), key)
	tx1 := transaction(1, big.NewInt(100000), key)

	pool := NewTxPool(testTxPoolConfig, params.TestnetChainConfig, blockchain)
	defer pool.Stop()

	nonce := pool.State().GetNonce(address)
	if nonce != 0 {
		t.Fatalf("Invalid nonce, want 0, got %d", nonce)
	}

	pool.AddTxs(types.Transactions{tx0, tx1})

	nonce = pool.State().GetNonce(address)
	if nonce != 2 {
		t.Fatalf("Invalid nonce, want 2, got %d", nonce)
	}

	// trigger state change in the background
	trigger = true

	pool.lockedReset(nil, nil)

	pendingTx, err := pool.Pending()
	if err != nil {
		t.Fatalf("Could not fetch pending transactions: %v", err)
	}

	for addr, txs := range pendingTx {
		t.Logf("%0x: %d\n", addr, len(txs))
	}

	nonce = pool.State().GetNonce(address)
	if nonce != 2 {
		t.Fatalf("Invalid nonce, want 2, got %d", nonce)
	}
}

func TestInvalidTransactions(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx := transaction(0, big.NewInt(5), key)
	from, _ := deriveSender(tx)

	pool.currentState.AddBalance(from, big.NewInt(1))
	if err := pool.AddTx(tx); err != ErrInsufficientFunds {
		t.Error("expected", ErrInsufficientFunds)
	}

	balance := new(big.Int).Add(tx.Value(), new(big.Int).Mul(tx.Gas(), tx.GasPrice()))
	pool.currentState.AddBalance(from, balance)
	if err := pool.AddTx(tx); err != ErrIntrinsicGas {
		t.Error("expected", ErrIntrinsicGas, "got", err)
	}

	pool.currentState.SetNonce(from, 1)
	pool.currentState.AddBalance(from, big.NewInt(0xffffffffffffff))
	tx = transaction(0, big.NewInt(100000), key)
	if err := pool.AddTx(tx); err != ErrNonceTooLow {
		t.Error("expected", ErrNonceTooLow)
	}

	tx = transaction(1, big.NewInt(100000), key)
	pool.gasPrice = big.NewInt(1000)
	if err := pool.AddTx(tx); err != ErrUnderpriced {
		t.Error("expected", ErrUnderpriced, "got", err)
	}
	tx = pricedTransaction(1, big.NewInt(100000),big.NewInt(1000), key)
	if err := pool.AddTx(tx); err != nil {
		t.Error("expected", nil, "got", err)
	}
}

func TestTransactionQueue(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx := transaction(0, big.NewInt(100), key)
	from, _ := deriveSender(tx)
	pool.currentState.AddBalance(from, big.NewInt(1000))
	pool.lockedReset(nil, nil)
	pool.enqueueTx(tx.Hash(), tx)

	pool.promoteExecutables([]common.Address{from})
	if len(pool.pending) != 1 {
		t.Error("expected valid txs to be 1 is", len(pool.pending))
	}

	tx = transaction(1, big.NewInt(100), key)
	from, _ = deriveSender(tx)
	pool.currentState.SetNonce(from, 2)
	pool.enqueueTx(tx.Hash(), tx)
	pool.promoteExecutables([]common.Address{from})
	if _, ok := pool.pending[from].txs.items[tx.Nonce()]; ok {
		t.Error("expected transaction to be in tx pool")
	}

	if len(pool.queue) > 0 {
		t.Error("expected transaction queue to be empty. is", len(pool.queue))
	}

	pool, key = setupTxPool()
	defer pool.Stop()

	tx1 := transaction(0, big.NewInt(100), key)
	tx2 := transaction(10, big.NewInt(100), key)
	tx3 := transaction(11, big.NewInt(100), key)
	from, _ = deriveSender(tx1)
	pool.currentState.AddBalance(from, big.NewInt(1000))
	pool.lockedReset(nil, nil)

	pool.enqueueTx(tx1.Hash(), tx1)
	pool.enqueueTx(tx2.Hash(), tx2)
	pool.enqueueTx(tx3.Hash(), tx3)

	pool.promoteExecutables([]common.Address{from})

	if len(pool.pending) != 1 {
		t.Error("expected tx pool to be 1, got", len(pool.pending))
	}
	if pool.queue[from].Len() != 2 {
		t.Error("expected len(queue) == 2, got", pool.queue[from].Len())
	}
}

func TestNegativeValue(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(-1), big.NewInt(100), big.NewInt(1), nil), types.HomesteadSigner{}, key)
	from, _ := deriveSender(tx)
	pool.currentState.AddBalance(from, big.NewInt(1))
	if err := pool.AddTx(tx); err != ErrNegativeValue {
		t.Error("expected", ErrNegativeValue, "got", err)
	}
}

func TestTransactionChainFork(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	resetState := func() {
		db, _ := hpbdb.NewMemDatabase()
		statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
		statedb.AddBalance(addr, big.NewInt(100000000000000))

		pool.chain = &testBlockChain{statedb, big.NewInt(1000000), new(event.Feed)}
		pool.lockedReset(nil, nil)
	}
	resetState()

	tx := transaction(0, big.NewInt(100000), key)
	if _, err := pool.add(tx); err != nil {
		t.Error("didn't expect error", err)
	}
	pool.removeTx(tx.Hash())

	// reset the pool's internal state
	resetState()
	if _, err := pool.add(tx); err != nil {
		t.Error("didn't expect error", err)
	}
}

func TestTransactionDoubleNonce(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	resetState := func() {
		db, _ := hpbdb.NewMemDatabase()
		statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
		statedb.AddBalance(addr, big.NewInt(100000000000000))

		pool.chain = &testBlockChain{statedb, big.NewInt(1000000), new(event.Feed)}
		pool.lockedReset(nil, nil)
	}
	resetState()

	signer := types.HomesteadSigner{}
	tx1, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(100), big.NewInt(100000), big.NewInt(1), nil), signer, key)
	tx2, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(100), big.NewInt(1000000), big.NewInt(2), nil), signer, key)
	tx3, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(100), big.NewInt(1000000), big.NewInt(1), nil), signer, key)

	// Add the first two transaction, ensure higher priced stays only
	if replace, err := pool.add(tx1); err != nil || replace {
		t.Errorf("first transaction insert failed (%v) or reported replacement (%v)", err, replace)
	}
	if replace, err := pool.add(tx2); err != nil || !replace {
		t.Errorf("second transaction insert failed (%v) or not reported replacement (%v)", err, replace)
	}
	pool.promoteExecutables([]common.Address{addr})
	if pool.pending[addr].Len() != 1 {
		t.Error("expected 1 pending transactions, got", pool.pending[addr].Len())
	}
	if tx := pool.pending[addr].txs.items[0]; tx.Hash() != tx2.Hash() {
		t.Errorf("transaction mismatch: have %x, want %x", tx.Hash(), tx2.Hash())
	}
	// Add the third transaction and ensure it's not saved (smaller price)
	pool.add(tx3)
	pool.promoteExecutables([]common.Address{addr})
	if pool.pending[addr].Len() != 1 {
		t.Error("expected 1 pending transactions, got", pool.pending[addr].Len())
	}
	if tx := pool.pending[addr].txs.items[0]; tx.Hash() != tx2.Hash() {
		t.Errorf("transaction mismatch: have %x, want %x", tx.Hash(), tx2.Hash())
	}
	// Ensure the total transaction count is correct
	if len(pool.all) != 1 {
		t.Error("expected 1 total transactions, got", len(pool.all))
	}
}

func TestMissingNonce(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	pool.currentState.AddBalance(addr, big.NewInt(100000000000000))
	tx := transaction(1, big.NewInt(100000), key)
	if _, err := pool.add(tx); err != nil {
		t.Error("didn't expect error", err)
	}
	if len(pool.pending) != 0 {
		t.Error("expected 0 pending transactions, got", len(pool.pending))
	}
	if pool.queue[addr].Len() != 1 {
		t.Error("expected 1 queued transaction, got", pool.queue[addr].Len())
	}
	if len(pool.all) != 1 {
		t.Error("expected 1 total transactions, got", len(pool.all))
	}
}

func TestTransactionNonceRecovery(t *testing.T) {
	const n = 10
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	pool.currentState.SetNonce(addr, n)
	pool.currentState.AddBalance(addr, big.NewInt(100000000000000))
	pool.lockedReset(nil, nil)

	tx := transaction(n, big.NewInt(100000), key)
	if err := pool.AddTx(tx); err != nil {
		t.Error(err)
	}
	// simulate some weird re-order of transactions and missing nonce(s)
	pool.currentState.SetNonce(addr, n-1)
	pool.lockedReset(nil, nil)
	if fn := pool.pendingState.GetNonce(addr); fn != n-1 {
		t.Errorf("expected nonce to be %d, got %d", n-1, fn)
	}
	if len(pool.pending) != 0 {
		t.Error("expected 0 pending transactions, got", len(pool.pending))
	}
	if pool.queue[addr].Len() != 1 {
		t.Error("expected 1 queued transaction, got", pool.queue[addr].Len())
	}
	if len(pool.all) != 1 {
		t.Error("expected 1 total transactions, got", len(pool.all))
	}
}

// Tests that if an account runs out of funds, any pending and queued transactions
// are dropped.
func TestTransactionDropping(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	pool.currentState.AddBalance(account, big.NewInt(1000))

	// Add some pending and some queued transactions
	var (
		tx0  = transaction(0, big.NewInt(100), key)
		tx1  = transaction(1, big.NewInt(200), key)
		tx2  = transaction(2, big.NewInt(300), key)
		tx10 = transaction(10, big.NewInt(100), key)
		tx11 = transaction(11, big.NewInt(200), key)
		tx12 = transaction(12, big.NewInt(300), key)
	)
	pool.promoteTx(account, tx0.Hash(), tx0)
	pool.promoteTx(account, tx1.Hash(), tx1)
	pool.promoteTx(account, tx2.Hash(), tx2)
	pool.enqueueTx(tx10.Hash(), tx10)
	pool.enqueueTx(tx11.Hash(), tx11)
	pool.enqueueTx(tx12.Hash(), tx12)

	// Check that pre and post validations leave the pool as is
	if pool.pending[account].Len() != 3 {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), 3)
	}
	if pool.queue[account].Len() != 3 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 3)
	}
	if len(pool.all) != 6 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 6)
	}
	pool.lockedReset(nil, nil)
	if pool.pending[account].Len() != 3 {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), 3)
	}
	if pool.queue[account].Len() != 3 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 3)
	}
	if len(pool.all) != 6 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 6)
	}
	// Reduce the balance of the account, and check that invalidated transactions are dropped
	pool.currentState.AddBalance(account, big.NewInt(-650))
	pool.lockedReset(nil, nil)

	if _, ok := pool.pending[account].txs.items[tx0.Nonce()]; !ok {
		t.Errorf("funded pending transaction missing: %v", tx0)
	}
	if _, ok := pool.pending[account].txs.items[tx1.Nonce()]; !ok {
		t.Errorf("funded pending transaction missing: %v", tx0)
	}
	if _, ok := pool.pending[account].txs.items[tx2.Nonce()]; ok {
		t.Errorf("out-of-fund pending transaction present: %v", tx1)
	}
	if _, ok := pool.queue[account].txs.items[tx10.Nonce()]; !ok {
		t.Errorf("funded queued transaction missing: %v", tx10)
	}
	if _, ok := pool.queue[account].txs.items[tx11.Nonce()]; !ok {
		t.Errorf("funded queued transaction missing: %v", tx10)
	}
	if _, ok := pool.queue[account].txs.items[tx12.Nonce()]; ok {
		t.Errorf("out-of-fund queued transaction present: %v", tx11)
	}
	if len(pool.all) != 4 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 4)
	}
	// Reduce the block gas limit, check that invalidated transactions are dropped
	pool.chain.(*testBlockChain).gasLimit = big.NewInt(100)
	pool.lockedReset(nil, nil)

	if _, ok := pool.pending[account].txs.items[tx0.Nonce()]; !ok {
		t.Errorf("funded pending transaction missing: %v", tx0)
	}
	if _, ok := pool.pending[account].txs.items[tx1.Nonce()]; ok {
		t.Errorf("over-gased pending transaction present: %v", tx1)
	}
	if _, ok := pool.queue[account].txs.items[tx10.Nonce()]; !ok {
		t.Errorf("funded queued transaction missing: %v", tx10)
	}
	if _, ok := pool.queue[account].txs.items[tx11.Nonce()]; ok {
		t.Errorf("over-gased queued transaction present: %v", tx11)
	}
	if len(pool.all) != 2 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 2)
	}
}

// Tests that if a transaction is dropped from the current pending pool (e.g. out
// of fund), all consecutive (still valid, but not executable) transactions are
// postponed back into the future queue to prevent broadcasting them.
func TestTransactionPostponing(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	pool.currentState.AddBalance(account, big.NewInt(1000))

	// Add a batch consecutive pending transactions for validation
	txns := []*types.Transaction{}
	for i := 0; i < 100; i++ {
		var tx *types.Transaction
		if i%2 == 0 {
			tx = transaction(uint64(i), big.NewInt(100), key)
		} else {
			tx = transaction(uint64(i), big.NewInt(500), key)
		}
		pool.promoteTx(account, tx.Hash(), tx)
		txns = append(txns, tx)
	}
	// Check that pre and post validations leave the pool as is
	if pool.pending[account].Len() != len(txns) {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), len(txns))
	}
	if len(pool.queue) != 0 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 0)
	}
	if len(pool.all) != len(txns) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), len(txns))
	}
	pool.lockedReset(nil, nil)
	if pool.pending[account].Len() != len(txns) {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), len(txns))
	}
	if len(pool.queue) != 0 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 0)
	}
	if len(pool.all) != len(txns) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), len(txns))
	}
	// Reduce the balance of the account, and check that transactions are reorganised
	pool.currentState.AddBalance(account, big.NewInt(-750))
	pool.lockedReset(nil, nil)

	if _, ok := pool.pending[account].txs.items[txns[0].Nonce()]; !ok {
		t.Errorf("tx %d: valid and funded transaction missing from pending pool: %v", 0, txns[0])
	}
	if _, ok := pool.queue[account].txs.items[txns[0].Nonce()]; ok {
		t.Errorf("tx %d: valid and funded transaction present in future queue: %v", 0, txns[0])
	}
	for i, tx := range txns[1:] {
		if i%2 == 1 {
			if _, ok := pool.pending[account].txs.items[tx.Nonce()]; ok {
				t.Errorf("tx %d: valid but future transaction present in pending pool: %v", i+1, tx)
			}
			if _, ok := pool.queue[account].txs.items[tx.Nonce()]; !ok {
				t.Errorf("tx %d: valid but future transaction missing from future queue: %v", i+1, tx)
			}
		} else {
			if _, ok := pool.pending[account].txs.items[tx.Nonce()]; ok {
				t.Errorf("tx %d: out-of-fund transaction present in pending pool: %v", i+1, tx)
			}
			if _, ok := pool.queue[account].txs.items[tx.Nonce()]; ok {
				t.Errorf("tx %d: out-of-fund transaction present in future queue: %v", i+1, tx)
			}
		}
	}
	if len(pool.all) != len(txns)/2 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), len(txns)/2)
	}
}

// Tests that if the transaction count belonging to a single account goes above
// some threshold, the higher transactions are dropped to prevent DOS attacks.
func TestTransactionQueueAccountLimiting(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	pool.currentState.AddBalance(account, big.NewInt(1000000))

	// Keep queuing up transactions and make sure all above a limit are dropped
	for i := uint64(1); i <= testTxPoolConfig.AccountQueue+5; i++ {
		if err := pool.AddTx(transaction(i, big.NewInt(100000), key)); err != nil {
			t.Fatalf("tx %d: failed to add transaction: %v", i, err)
		}
		if len(pool.pending) != 0 {
			t.Errorf("tx %d: pending pool size mismatch: have %d, want %d", i, len(pool.pending), 0)
		}
		if i <= testTxPoolConfig.AccountQueue {
			if pool.queue[account].Len() != int(i) {
				t.Errorf("tx %d: queue size mismatch: have %d, want %d", i, pool.queue[account].Len(), i)
			}
		} else {
			if pool.queue[account].Len() != int(testTxPoolConfig.AccountQueue) {
				t.Errorf("tx %d: queue limit mismatch: have %d, want %d", i, pool.queue[account].Len(), testTxPoolConfig.AccountQueue)
			}
		}
	}
	if len(pool.all) != int(testTxPoolConfig.AccountQueue) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), testTxPoolConfig.AccountQueue)
	}
}

// Tests that if the transaction count belonging to multiple accounts go above
// some threshold, the higher transactions are dropped to prevent DOS attacks.
//
// This logic should not hold for local transactions, unless the local tracking
// mechanism is disabled.
func TestTransactionQueueGlobalLimiting(t *testing.T) {
	testTransactionQueueGlobalLimiting(t, false)
}
func TestTransactionQueueGlobalLimitingNoLocals(t *testing.T) {
	testTransactionQueueGlobalLimiting(t, true)
}

func testTransactionQueueGlobalLimiting(t *testing.T, nolocals bool) {
	// Create the pool to test the limit enforcement with
	db, _ := hpbdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	blockchain := &testBlockChain{statedb, big.NewInt(1000000), new(event.Feed)}

	config := testTxPoolConfig
	config.GlobalQueue = config.AccountQueue*3 - 1 // reduce the queue limits to shorten test time (-1 to make it non divisible)

	pool := NewTxPool(config, params.TestnetChainConfig, blockchain)
	defer pool.Stop()

	// Create a number of test accounts and fund them (last one will be the local)
	keys := make([]*ecdsa.PrivateKey, 5)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
		pool.currentState.AddBalance(crypto.PubkeyToAddress(keys[i].PublicKey), big.NewInt(1000000))
	}

	// Generate and queue a batch of transactions
	nonces := make(map[common.Address]uint64)

	txs := make(types.Transactions, 0, 3*config.GlobalQueue)
	for len(txs) < cap(txs) {
		key := keys[rand.Intn(len(keys)-1)] // skip adding transactions with the local account
		addr := crypto.PubkeyToAddress(key.PublicKey)

		txs = append(txs, transaction(nonces[addr]+1, big.NewInt(100000), key))
		nonces[addr]++
	}
	// Import the batch and verify that limits have been enforced
	pool.AddTxs(txs)

	queued := 0
	for addr, list := range pool.queue {
		if list.Len() > int(config.AccountQueue) {
			t.Errorf("addr %x: queued accounts overflown allowance: %d > %d", addr, list.Len(), config.AccountQueue)
		}
		queued += list.Len()
	}
	if queued > int(config.GlobalQueue) {
		t.Fatalf("total transactions overflow allowance: %d > %d", queued, config.GlobalQueue)
	}
}
