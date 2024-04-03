package precheck

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/axiomesh/axiom-ledger/internal/components"
	"github.com/ethereum/go-ethereum/params"
	"github.com/gammazero/workerpool"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var _ PreCheck = (*TxPreCheckMgr)(nil)

const (
	defaultTxPreCheckSize = 10000
	ErrTxEventType        = "invalid tx event type"
	ErrParseTxEventType   = "parse tx event type error"
)

var (
	concurrencyLimit = runtime.NumCPU()

	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrOversizedData = errors.New("oversized data")
)

type ValidTxs struct {
	Local            bool
	Txs              []*types.Transaction
	LocalCheckRespCh chan *common.TxResp
	LocalPoolRespCh  chan *common.TxResp
}

type TxPreCheckMgr struct {
	basicCheckCh chan *common.UncheckedTxEvent
	verifySignCh chan *common.UncheckedTxEvent
	verifyDataCh chan *common.UncheckedTxEvent
	validTxsCh   chan *ValidTxs
	logger       logrus.FieldLogger

	txpool txpool.TxPool[types.Transaction, *types.Transaction]

	BaseFee        *big.Int // current is 0
	getBalanceFn   func(address string) *big.Int
	getChainMetaFn func() *types.ChainMeta

	ctx       context.Context
	txMaxSize atomic.Uint64
}

func (tp *TxPreCheckMgr) UpdateEpochInfo(epoch *rbft.EpochInfo) {
	tp.txMaxSize.Store(epoch.MiscParams.TxMaxSize)
}

func (tp *TxPreCheckMgr) PostUncheckedTxEvent(ev *common.UncheckedTxEvent) {
	tp.basicCheckCh <- ev
}

func (tp *TxPreCheckMgr) pushValidTxs(ev *ValidTxs) {
	tp.validTxsCh <- ev
}

func NewTxPreCheckMgr(ctx context.Context, conf *common.Config) *TxPreCheckMgr {
	tp := &TxPreCheckMgr{
		basicCheckCh:   make(chan *common.UncheckedTxEvent, defaultTxPreCheckSize),
		verifySignCh:   make(chan *common.UncheckedTxEvent, defaultTxPreCheckSize),
		verifyDataCh:   make(chan *common.UncheckedTxEvent, defaultTxPreCheckSize),
		validTxsCh:     make(chan *ValidTxs, defaultTxPreCheckSize),
		logger:         conf.Logger,
		ctx:            ctx,
		BaseFee:        big.NewInt(0),
		getBalanceFn:   conf.GetAccountBalance,
		getChainMetaFn: conf.GetChainMetaFunc,
		txpool:         conf.TxPool,
	}

	if conf.GenesisEpochInfo.MiscParams.TxMaxSize == 0 {
		tp.txMaxSize.Store(repo.DefaultTxMaxSize)
	} else {
		tp.txMaxSize.Store(conf.GenesisEpochInfo.MiscParams.TxMaxSize)
	}

	return tp
}

func (tp *TxPreCheckMgr) Start() {
	go tp.dispatchTxEvent()
	go tp.dispatchVerifySignEvent()
	go tp.dispatchVerifyDataEvent()
	go tp.postValidTxs()
	tp.logger.Info("tx precheck manager started")
}

func (tp *TxPreCheckMgr) postValidTxs() {
	for {
		select {
		case <-tp.ctx.Done():
			return
		case txs := <-tp.validTxsCh:
			validTxCounter.Add(float64(len(txs.Txs)))
			if txs.Local {
				// notify consensus that it had prechecked, can broadcast to other nodes
				respLocalTx(responseType_precheck, txs.LocalCheckRespCh, nil)
				err := tp.txpool.AddLocalTx(txs.Txs[0])
				respLocalTx(responseType_txPool, txs.LocalPoolRespCh, err)
			} else {
				tp.txpool.AddRemoteTxs(txs.Txs)
			}
		}
	}
}

func (tp *TxPreCheckMgr) dispatchTxEvent() {
	wp := workerpool.New(concurrencyLimit)

	for {
		select {
		case <-tp.ctx.Done():
			wp.StopWait()
			return
		case ev := <-tp.basicCheckCh:
			wp.Submit(func() {
				switch ev.EventType {
				case common.LocalTxEvent:
					now := time.Now()
					txWithResp, ok := ev.Event.(*common.TxWithResp)
					if !ok {
						tp.logger.Errorf("%s:%s", ErrParseTxEventType, "receive invalid local TxEvent")
						return
					}
					if err := tp.basicCheckTx(txWithResp.Tx); err != nil {
						respLocalTx(responseType_precheck, txWithResp.CheckCh, wrapError(err))
						tp.logger.Warningf("basic check local tx err:%s", err)
						return
					}
					basicCheckDuration.WithLabelValues("local").Observe(time.Since(now).Seconds())
					tp.verifySignCh <- ev

				case common.RemoteTxEvent:
					now := time.Now()
					txSet, ok := ev.Event.([]*types.Transaction)
					if !ok {
						tp.logger.Errorf("%s:%s", ErrParseTxEventType, "receive invalid remote TxEvent")
						return
					}
					validSignTxs := make([]*types.Transaction, 0)
					for _, tx := range txSet {
						if err := tp.basicCheckTx(tx); err != nil {
							tp.logger.Warningf("basic check remote tx err:%s", err)
							continue
						}
						validSignTxs = append(validSignTxs, tx)
					}
					ev.Event = validSignTxs
					basicCheckDuration.WithLabelValues("remote").Observe(time.Since(now).Seconds())
					tp.verifySignCh <- ev
				default:
					tp.logger.Errorf(ErrTxEventType)
					return
				}
			})
		}
	}
}

func (tp *TxPreCheckMgr) dispatchVerifySignEvent() {
	wp := workerpool.New(concurrencyLimit)
	for {
		select {
		case <-tp.ctx.Done():
			wp.StopWait()
			return
		case ev := <-tp.verifySignCh:
			wp.Submit(func() {
				now := time.Now()
				switch ev.EventType {
				case common.LocalTxEvent:
					txWithResp, ok := ev.Event.(*common.TxWithResp)
					if !ok {
						tp.logger.Errorf("%s:%s", ErrParseTxEventType, "receive invalid local TxEvent")
						return
					}
					if err := tp.verifySignature(txWithResp.Tx); err != nil {
						respLocalTx(responseType_precheck, txWithResp.CheckCh, wrapError(err))
						tp.logger.Warningf("verify signature of local tx [txHash:%s] err: %s", txWithResp.Tx.GetHash().String(), err)
						return
					}
					verifySignatureDuration.WithLabelValues("local").Observe(time.Since(now).Seconds())
					tp.verifyDataCh <- ev

				case common.RemoteTxEvent:
					txSet, ok := ev.Event.([]*types.Transaction)
					if !ok {
						tp.logger.Errorf("%s:%s", ErrParseTxEventType, "receive invalid remote TxEvent")
						return
					}
					validSignTxs := make([]*types.Transaction, 0)
					for _, tx := range txSet {
						if err := tp.verifySignature(tx); err != nil {
							tp.logger.Warningf("verify signature remote tx err:%s", err)
							continue
						}
						validSignTxs = append(validSignTxs, tx)
					}
					ev.Event = validSignTxs
					verifySignatureDuration.WithLabelValues("remote").Observe(time.Since(now).Seconds())
					tp.verifyDataCh <- ev
				default:
					tp.logger.Errorf(ErrTxEventType)
					return
				}
			})
		}
	}
}

func (tp *TxPreCheckMgr) dispatchVerifyDataEvent() {
	wp := workerpool.New(concurrencyLimit)
	for {
		select {
		case <-tp.ctx.Done():
			wp.StopWait()
			return
		case ev := <-tp.verifyDataCh:
			wp.Submit(func() {
				var (
					validDataTxs     []*types.Transaction
					local            bool
					localCheckRespCh chan *common.TxResp
					localPoolRespCh  chan *common.TxResp
				)

				now := time.Now()
				switch ev.EventType {
				case common.LocalTxEvent:
					local = true
					txWithResp := ev.Event.(*common.TxWithResp)
					localCheckRespCh = txWithResp.CheckCh
					localPoolRespCh = txWithResp.PoolCh
					// check balance
					if err := components.VerifyInsufficientBalance[types.Transaction, *types.Transaction](txWithResp.Tx, tp.getChainMetaFn().GasPrice, tp.getBalanceFn); err != nil {
						respLocalTx(responseType_precheck, txWithResp.CheckCh, wrapError(err))
						return
					}
					validDataTxs = append(validDataTxs, txWithResp.Tx)
					verifyBalanceDuration.WithLabelValues("local").Observe(time.Since(now).Seconds())

				case common.RemoteTxEvent:
					txSet, ok := ev.Event.([]*types.Transaction)
					if !ok {
						tp.logger.Errorf("receive invalid remote TxEvent")
						return
					}
					for _, tx := range txSet {
						if err := components.VerifyInsufficientBalance[types.Transaction, *types.Transaction](tx, tp.getChainMetaFn().GasPrice, tp.getBalanceFn); err != nil {
							tp.logger.Warningf("verify remote tx balance failed: %v", err)
							continue
						}

						validDataTxs = append(validDataTxs, tx)
					}
					verifyBalanceDuration.WithLabelValues("remote").Observe(time.Since(now).Seconds())
				}

				validTxs := &ValidTxs{
					Local: local,
					Txs:   validDataTxs,
				}
				if local {
					validTxs.LocalCheckRespCh = localCheckRespCh
					validTxs.LocalPoolRespCh = localPoolRespCh
				}

				tp.pushValidTxs(validTxs)
			})
		}
	}
}

func (tp *TxPreCheckMgr) verifySignature(tx *types.Transaction) error {
	if err := tx.VerifySignature(); err != nil {
		return errTxSign
	}

	// check to address
	if tx.GetTo() != nil {
		if tx.GetFrom().String() == tx.GetTo().String() {
			err := errTo
			tp.logger.Errorf(err.Error())
			return err
		}
	}
	return nil
}

func (tp *TxPreCheckMgr) basicCheckTx(tx *types.Transaction) error {
	// 1. reject transactions over defined size to prevent DOS attacks
	if uint64(tx.Size()) > tp.txMaxSize.Load() {
		return ErrOversizedData
	}

	gasPrice := tp.getChainMetaFn().GasPrice
	// ignore gas price if it's 0 or nil
	if tx.GetGasPrice() != nil {
		if tx.GetGasPrice().Uint64() != 0 && tx.GetGasPrice().Cmp(gasPrice) < 0 {
			return fmt.Errorf("%w:[hash:%s, nonce:%d] expect min gasPrice: %v, get price %v",
				errGasPriceTooLow, tx.GetHash().String(), tx.GetNonce(), gasPrice, tx.GetGasPrice())
		}
	}

	// 2. check the gas parameters's format are valid
	if tx.GetType() == types.DynamicFeeTxType {
		if tx.GetGasFeeCap().BitLen() > 0 || tx.GetGasTipCap().BitLen() > 0 {
			if l := tx.GetGasFeeCap().BitLen(); l > 256 {
				return fmt.Errorf("%w: [hash:%s, nonce:%d], maxFeePerGas bit length: %d", errFeeCapVeryHigh,
					tx.GetHash().String(), tx.GetNonce(), l)
			}
			if l := tx.GetGasTipCap().BitLen(); l > 256 {
				return fmt.Errorf("%w: [hash:%s, nonce:%d], maxPriorityFeePerGas bit length: %d", errTipVeryHigh,
					tx.GetHash().String(), tx.GetNonce(), l)
			}

			if tx.GetGasFeeCap().Cmp(tx.GetGasTipCap()) < 0 {
				return fmt.Errorf("%w: [hash:%s, nonce:%d], maxPriorityFeePerGas: %s, maxFeePerGas: %s", errTipAboveFeeCap,
					tx.GetHash().String(), tx.GetNonce(), tx.GetGasTipCap(), tx.GetGasFeeCap())
			}

			// This will panic if baseFee is nil, but basefee presence is verified
			// as part of header validation.
			// TODO: modify tp.BaseFee synchronously if baseFee changed
			if tx.GetGasFeeCap().Cmp(tp.BaseFee) < 0 {
				return fmt.Errorf("%w: [hash:%s, nonce:%d], maxFeePerGas: %s baseFee: %s", errFeeCapTooLow,
					tx.GetHash().String(), tx.GetNonce(), tx.GetGasFeeCap(), tp.BaseFee)
			}
		}
	}

	var isContractCreation bool
	if tx.GetTo() == nil {
		isContractCreation = true
	}
	// 5. if deployed a contract, Check whether the init code size has been exceeded.
	if isContractCreation && len(tx.GetPayload()) > params.MaxInitCodeSize {
		return fmt.Errorf("%w: [hash:%s, nonce:%d], code size %v limit %v", errMaxInitCodeSizeExceeded,
			tx.GetHash().String(), tx.GetNonce(), len(tx.GetPayload()), params.MaxInitCodeSize)
	}

	return nil
}

func respLocalTx(typ int, ch chan *common.TxResp, err error) {
	defer func() {
		if typ == responseType_precheck {
			reason, valid := convertErrorType(err)
			if valid {
				validTxCounter.Inc()
			} else {
				rejectTxCounter.WithLabelValues(reason).Inc()
			}
		}
	}()
	resp := &common.TxResp{
		Status: true,
	}

	if err != nil {
		resp.Status = false
		resp.ErrorMsg = err.Error()
	}

	ch <- resp
}
