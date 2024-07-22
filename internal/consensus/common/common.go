package common

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
)

const (
	EpochStatePrefix = "epoch_q_chkpt."
	EpochIndexKey    = "epoch_latest_idx"
)

const MaxChainSize = 1000

// todo: Unify the storage format of epoch store
func StoreEpochState(epochStore kv.Storage, key string, value []byte) error {
	epochStore.Put([]byte("epoch."+key), value)
	return nil
}

func PersistEpochChange(epochStore kv.Storage, epoch uint64, value []byte) error {
	key := fmt.Sprintf("%s%d", EpochStatePrefix, epoch)
	epochStore.Put([]byte("epoch."+key), value)

	// update latest epoch index
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, epoch)
	epochStore.Put([]byte("epoch."+EpochIndexKey), data)
	return nil
}

func GetEpochChange(epochStore kv.Storage, epoch uint64) ([]byte, error) {
	key := fmt.Sprintf("%s%d", EpochStatePrefix, epoch)
	b := epochStore.Get([]byte("epoch." + key))
	if b == nil {
		return nil, fmt.Errorf("epoch %d not found", epoch)
	}
	return b, nil
}

func ReadEpochState(epochStore kv.Storage, key string) ([]byte, error) {
	b := epochStore.Get([]byte("epoch." + key))
	if b == nil {
		return nil, errors.New("not found")
	}
	return b, nil
}

func NeedChangeEpoch(height uint64, epochInfo types.EpochInfo) bool {
	return height == (epochInfo.StartBlock + epochInfo.EpochPeriod - 1)
}

func CalFaulty(N uint64) uint64 {
	f := (N - 1) / 3
	return f
}

func CalQuorum(N uint64) uint64 {
	f := (N - 1) / 3
	return (N + f + 2) / 2
}
