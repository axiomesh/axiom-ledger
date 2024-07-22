package adaptor

import (
	"io"
	"sync"

	"github.com/bcds/go-hpc-dagbft/common/types"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
)

type Builder = func(epoch types.Epoch, name string) layer.Storage

type Factory struct {
	sync.Mutex
	epoch   types.Epoch
	dbs     map[string]layer.Storage
	builder Builder
}

func NewFactory(epoch types.Epoch, builder Builder) *Factory {
	return &Factory{
		epoch:   epoch,
		dbs:     make(map[string]layer.Storage),
		builder: builder,
	}
}

func (f *Factory) Close() error {
	for _, s := range f.dbs {
		if c, ok := s.(io.Closer); ok {
			if err := c.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *Factory) GetStorage(name string) layer.Storage {
	f.Lock()
	defer f.Unlock()
	db, ok := f.dbs[name]
	if !ok {
		db = f.builder(f.epoch, name)
		f.dbs[name] = db
	}
	return db
}
