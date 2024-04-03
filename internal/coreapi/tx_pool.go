package coreapi

import (
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/coreapi/api"
)

type TxPoolAPI CoreAPI

var _ api.TxPoolAPI = (*TxPoolAPI)(nil)

func (api *TxPoolAPI) GetTotalPendingTxCount() uint64 {
	if api.axiomLedger.Repo.StartArgs.ReadonlyMode {
		return 0
	}
	return api.axiomLedger.TxPool.GetTotalPendingTxCount()
}

func (api *TxPoolAPI) GetPendingTxCountByAccount(account string) uint64 {
	if api.axiomLedger.Repo.StartArgs.ReadonlyMode {
		return 0
	}
	return api.axiomLedger.TxPool.GetPendingTxCountByAccount(account)
}

func (api *TxPoolAPI) GetTransaction(hash *types.Hash) *types.Transaction {
	if api.axiomLedger.Repo.StartArgs.ReadonlyMode {
		return nil
	}
	return api.axiomLedger.TxPool.GetPendingTxByHash(hash.String())
}

func (api *TxPoolAPI) GetAccountMeta(account string, full bool) any {
	if api.axiomLedger.Repo.StartArgs.ReadonlyMode {
		return nil
	}

	return api.axiomLedger.TxPool.GetAccountMeta(account, full)
}

func (api *TxPoolAPI) GetMeta(full bool) any {
	if api.axiomLedger.Repo.StartArgs.ReadonlyMode {
		return nil
	}
	return api.axiomLedger.TxPool.GetMeta(full)
}

func (api *TxPoolAPI) GetChainInfo() any {
	if api.axiomLedger.Repo.StartArgs.ReadonlyMode {
		return nil
	}
	return api.axiomLedger.TxPool.GetChainInfo()
}

func (api *TxPoolAPI) IsStarted() bool {
	return api.axiomLedger.TxPool.IsStarted()
}
