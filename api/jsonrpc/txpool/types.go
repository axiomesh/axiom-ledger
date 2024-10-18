package txpool

import (
	"errors"

	rpctypes "github.com/axiomesh/axiom-ledger/api/jsonrpc/types"
)

var (
	ErrNotStarted = errors.New("txpool is not started")
)

type ContentResponse struct {
	Pending         map[string][]PoolTxContent `json:"pending"`
	Queued          map[string][]PoolTxContent `json:"queued"`
	TxCountLimit    uint64                     `json:"txCountLimit"`
	TxCount         uint64                     `json:"txCount"`
	ReadyTxCount    uint64                     `json:"readyTxCount"`
	NotReadyTxCount uint64                     `json:"notReadyTxCount"`
}

type PoolTxContent struct {
	Nonce       uint64                   `json:"nonce"`
	Transaction *rpctypes.RPCTransaction `json:"transaction"`
	Local       bool                     `json:"local"`
	LifeTime    int64                    `json:"lifeTime"`
	ArrivedTime int64                    `json:"arrivedTime"`
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
	Pending      []PoolTxContent `json:"pending"`
	Queued       []PoolTxContent `json:"queued"`
	CommitNonce  uint64          `json:"commitNonce"`
	PendingNonce uint64          `json:"pendingNonce"`
	TxCount      uint64          `json:"txCount"`
}

type InspectResponse struct {
	Pending map[string][]string `json:"pending"`
	Queued  map[string][]string `json:"queued"`
}

type StatusResponse struct {
	Pending uint64 `json:"pending"`
	Queued  uint64 `json:"queued"`
	Total   uint64 `json:"total"`
}
