package common

import (
	"encoding/json"

	"github.com/axiomesh/axiom-kit/storage/minifile"
	"github.com/bcds/go-hpc-dagbft/common/dbutils"
	"github.com/bcds/go-hpc-dagbft/protocol"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
	"github.com/gogo/protobuf/proto"
)

var _ layer.Storage = (*MinifileAdaptor)(nil)

type MinifileAdaptor struct {
	store *minifile.MiniFile
}

func (a *MinifileAdaptor) Write(values ...protocol.Entry) error {
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
		if err = a.store.Put(string(key), enc); err != nil {
			return err
		}
	}
	return nil
}

func (a *MinifileAdaptor) Delete(keys ...protocol.Entry) error {
	for _, one := range keys {
		key := one.EntryKey()
		if err := a.store.Delete(string(key)); err != nil {
			return err
		}
	}
	return nil
}

func (a *MinifileAdaptor) Has(key protocol.Entry) (bool, error) {
	k := string(key.EntryKey())
	return a.store.Has(k)
}

func (a *MinifileAdaptor) Read(key protocol.Entry) (protocol.Entry, error) {
	k := string(key.EntryKey())

	v, err := a.store.Get(k)
	if err != nil || v == nil {
		return nil, err
	}

	return a.read(key.New(), v)
}

func (a *MinifileAdaptor) read(empty protocol.Entry, enc []byte) (protocol.Entry, error) {
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

func (a *MinifileAdaptor) ReadRange(lower, higher protocol.Entry, reversed bool, iterator func(entry protocol.Entry) bool) error {
	var beginKey, endKey []byte
	var innerErr error

	beginKey = lower.EntryKey()
	if higher != nil {
		endKey = higher.EntryKey()
	} else {
		endKey = dbutils.BytesPrefixUpperBound(beginKey)
	}

	if !reversed {
		a.store.AscendRange(beginKey, endKey, func(key, value []byte) (bool, error) {
			newEntry := lower.New()
			en, err := a.read(newEntry, value)
			if err != nil {
				innerErr = err
				return false, err
			}
			return iterator(en), nil
		})
	} else {
		a.store.DescendRange(endKey, beginKey, func(key, value []byte) (bool, error) {
			newEntry := lower.New()
			en, err := a.read(newEntry, value)
			if err != nil {
				innerErr = err
				return false, err
			}
			return iterator(en), nil
		})
	}
	return innerErr
}

func OpenMinifile(path string) (*MinifileAdaptor, error) {
	store, err := minifile.New(path)
	if err != nil {
		return nil, err
	}
	return &MinifileAdaptor{
		store: store,
	}, nil
}

func (a *MinifileAdaptor) StoreState(key string, value []byte) error {
	return a.store.Put(key, value)
}

func (a *MinifileAdaptor) DelState(key string) error {
	return a.store.Delete(key)
}

// ReadState retrieves the value associated with the given key from the DB, or <nil, nil> if not found
func (a *MinifileAdaptor) ReadState(key string) ([]byte, error) {
	b, err := a.store.Get(key)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ReadStateSet retrieves all key-value pairs where the key starts with prefix from the database with the given namespace
func (a *MinifileAdaptor) ReadStateSet(prefix string) (map[string][]byte, error) {
	ret, err := a.store.GetAll(prefix)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (a *MinifileAdaptor) Close() error {
	return a.store.Close()
}
