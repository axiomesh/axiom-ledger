package adaptor

import (
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
)

func (a *RBFTAdaptor) GetCurrentEpochInfo() (*types.EpochInfo, error) {
	info := a.config.ChainState.GetCurrentEpochInfo()
	return &info, nil
}

func (a *RBFTAdaptor) GetEpochInfo(epoch uint64) (*types.EpochInfo, error) {
	return a.config.ChainState.GetEpochInfo(epoch)
}

func (a *RBFTAdaptor) StoreEpochState(key string, value []byte) error {
	return common.StoreEpochState(a.epochStore, key, value)
}

func (a *RBFTAdaptor) ReadEpochState(key string) ([]byte, error) {
	return common.ReadEpochState(a.epochStore, key)
}
