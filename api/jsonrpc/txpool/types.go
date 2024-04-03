package txpool

import (
	"errors"

	rpctypes "github.com/axiomesh/axiom-ledger/api/jsonrpc/types"
)

var (
	ErrNotStarted = errors.New("txpool is not started")
)

type ContentResponse struct {
	Pending         map[string]map[string]*rpctypes.RPCTransaction `json:"pending"`
	Queued          map[string]map[string]*rpctypes.RPCTransaction `json:"queued"`
	TxCountLimit    uint64                                         `json:"txCountLimit"`
	TxCount         uint64                                         `json:"txCount"`
	ReadyTxCount    uint64                                         `json:"readyTxCount"`
	NotReadyTxCount uint64                                         `json:"notReadyTxCount"`
}

type SimpleContentResponse struct {
	SimpleAccountContent map[string]SimpleAccountContent `json:"simpleAccountContent"`
	TxCountLimit         uint64                          `json:"txCountLimit"`
	TxCount              uint64                          `json:"txCount"`
	ReadyTxCount         uint64                          `json:"readyTxCount"`
	NotReadyTxCount      uint64                          `json:"notReadyTxCount"`
}

type SimpleAccountContent struct {
	Pending      []TxByNonce `json:"pending"`
	Queued       []TxByNonce `json:"queued"`
	CommitNonce  uint64      `json:"commitNonce"`
	PendingNonce uint64      `json:"pendingNonce"`
	TxCount      uint64      `json:"txCount"`
}

type TxByNonce struct {
	Nonce  uint64
	TxHash string
}

type AccountContentResponse struct {
	Pending      map[string]*rpctypes.RPCTransaction `json:"pending"`
	Queued       map[string]*rpctypes.RPCTransaction `json:"queued"`
	CommitNonce  uint64                              `json:"commitNonce"`
	PendingNonce uint64                              `json:"pendingNonce"`
	TxCount      uint64                              `json:"txCount"`
}

type InspectResponse struct {
	Pending map[string]map[string]string `json:"pending"`
	Queued  map[string]map[string]string `json:"queued"`
}

type StatusResponse struct {
	Pending uint64 `json:"pending"`
	Queued  uint64 `json:"queued"`
	Total   uint64 `json:"total"`
}
