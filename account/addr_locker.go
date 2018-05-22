package accounts

import (
	"sync"
	"github.com/hpb-project/ghpb/common"
)

var (
	mu    sync.Mutex
	locks map[common.Address]*sync.Mutex
)

// lock returns the lock of the given address.
func lock(address common.Address) *sync.Mutex {
	mu.Lock()
	defer mu.Unlock()
	if locks == nil {
		locks = make(map[common.Address]*sync.Mutex)
	}
	if _, ok := locks[address]; !ok {
		locks[address] = new(sync.Mutex)
	}
	return locks[address]
}

// LockAddr locks an account's mutex. This is used to prevent another tx getting the
// same nonce until the lock is released. The mutex prevents the (an identical nonce) from
// being read again during the time that the first transaction is being signed.
func LockAddr(address common.Address) {
	lock(address).Lock()
}

// UnlockAddr unlocks the mutex of the given account.
func UnlockAddr(address common.Address) {
	lock(address).Unlock()
}
