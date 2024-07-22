package common

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/bcds/go-hpc-dagbft/common/dbutils"
	"github.com/bcds/go-hpc-dagbft/protocol"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/rosedblabs/rosedb/v2"
)

type RosedbAdaptor struct {
	store *rosedb.DB
}

func (a *RosedbAdaptor) Write(values ...protocol.Entry) error {
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
		if err = a.store.Put(key, enc); err != nil {
			return err
		}
	}
	return nil
}

func (a *RosedbAdaptor) Delete(keys ...protocol.Entry) error {
	for _, one := range keys {
		key := one.EntryKey()
		if err := a.store.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func (a *RosedbAdaptor) Has(key protocol.Entry) (bool, error) {
	return a.store.Exist(key.EntryKey())
}

func (a *RosedbAdaptor) read(empty protocol.Entry, enc []byte) (protocol.Entry, error) {
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

func (a *RosedbAdaptor) Read(key protocol.Entry) (protocol.Entry, error) {
	enc, err := a.store.Get(key.EntryKey())
	if err != nil {
		return nil, err
	}

	// if the entry is not exist, return nil
	if enc == nil {
		return nil, nil
	}
	newEntry := key.New()

	return a.read(newEntry, enc)
}

func (a *RosedbAdaptor) ReadRange(lower, higher protocol.Entry, reversed bool, iterator func(entry protocol.Entry) bool) error {
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

func OpenRosedb(path string) (*RosedbAdaptor, error) {
	options := rosedb.DefaultOptions
	options.DirPath = path
	options.Sync = false
	options.BlockCache = 50 * 1024 * 1024
	options.SegmentSize = 50 * 1024 * 1024
	store, err := rosedb.Open(options)
	if err != nil {
		return nil, err
	}

	// merge log files
	if err := store.Merge(true); err != nil {
		return nil, err
	}
	go func() {
		tk := time.NewTicker(5 * time.Minute)
		defer tk.Stop()

		for {
			<-tk.C
			if err := store.Merge(true); err != nil {
				continue
			}
		}
	}()

	return &RosedbAdaptor{
		store: store,
	}, nil
}

func (a *RosedbAdaptor) StoreState(key string, value []byte) error {
	return a.store.Put([]byte(key), value)
}

func (a *RosedbAdaptor) DelState(key string) error {
	return a.store.Delete([]byte(key))
}

// ReadState retrieves the value associated with the given key from the DB, or <nil, nil> if not found
func (a *RosedbAdaptor) ReadState(key string) ([]byte, error) {
	b, err := a.store.Get([]byte(key))
	if err != nil {
		if errors.Is(err, rosedb.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

// ReadStateSet retrieves all key-value pairs where the key starts with prefix from the database with the given namespace
func (a *RosedbAdaptor) ReadStateSet(prefix string) (map[string][]byte, error) {
	prefixRaw := []byte(prefix)
	ret := make(map[string][]byte)
	a.store.Ascend(func(k []byte, v []byte) (bool, error) {
		if bytes.HasPrefix(k, prefixRaw) {
			ret[string(k)] = append([]byte(nil), v...)
		}
		return true, nil
	})
	return ret, nil
}
