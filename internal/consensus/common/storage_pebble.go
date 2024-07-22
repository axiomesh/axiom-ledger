package common

import (
	"encoding/json"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	"github.com/bcds/go-hpc-dagbft/common/dbutils"
	"github.com/bcds/go-hpc-dagbft/protocol"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
	"github.com/gogo/protobuf/proto"
)

var _ layer.Storage = (*PebbleAdaptor)(nil)

type PebbleAdaptor struct {
	store kv.Storage
}

func OpenPebbleDb(repoRoot string) (*PebbleAdaptor, error) {
	storePath := repo.GetStoragePath(repoRoot, storagemgr.Consensus)
	store, err := storagemgr.OpenWithMetrics(storePath, storagemgr.Consensus)
	if err != nil {
		return nil, err
	}
	return &PebbleAdaptor{
		store: store,
	}, nil
}

func (p PebbleAdaptor) Write(values ...protocol.Entry) error {
	var (
		enc []byte
		err error
	)
	for _, one := range values {
		key := one.EntryKey()
		if mr, ok := one.(proto.Marshaler); ok {
			enc, err = mr.Marshal()
		} else {
			enc, err = json.Marshal(one)
		}
		if err != nil {
			return err
		}
		p.store.Put(key, enc)
	}
	return nil
}

func (p PebbleAdaptor) Delete(keys ...protocol.Entry) error {
	for _, one := range keys {
		key := one.EntryKey()
		p.store.Delete(key)
	}
	return nil
}

func (p PebbleAdaptor) Has(key protocol.Entry) (bool, error) {
	return p.store.Has(key.EntryKey()), nil
}

func (p PebbleAdaptor) Read(key protocol.Entry) (protocol.Entry, error) {
	enc := p.store.Get(key.EntryKey())
	// if the entry is not exist, return nil
	if enc == nil {
		return nil, nil
	}
	newEntry := key.New()

	return p.read(newEntry, enc)
}

func (p PebbleAdaptor) read(empty protocol.Entry, enc []byte) (protocol.Entry, error) {
	var err error
	if mr, ok := empty.(proto.Unmarshaler); ok {
		err = mr.Unmarshal(enc)
	} else {
		err = json.Unmarshal(enc, empty)
	}
	if err != nil {
		return nil, err
	}
	return empty, nil
}

func (p PebbleAdaptor) ReadRange(lower, higher protocol.Entry, reversed bool, iterator func(entry protocol.Entry) bool) error {
	var beginKey, endKey []byte

	beginKey = lower.EntryKey()
	if higher != nil {
		endKey = higher.EntryKey()
	} else {
		endKey = dbutils.BytesPrefixUpperBound(beginKey)
	}

	var it kv.Iterator
	it = p.store.Iterator(beginKey, endKey)
	if reversed {
		if !it.Last() {
			return nil
		}
	} else {
		if valid := it.Next(); !valid || !it.First() {
			return nil
		}
	}
	for has := true; has; {
		value := it.Value()
		entry, err := p.read(lower.New(), value)
		if err != nil {
			return err
		}
		if !iterator(entry) {
			break
		}
		if reversed {
			has = it.Prev()
		} else {
			has = it.Next()
		}
	}
	return nil
}
