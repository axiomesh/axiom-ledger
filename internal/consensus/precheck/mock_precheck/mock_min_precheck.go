package mock_precheck

import (
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck"
	"go.uber.org/mock/gomock"

	"github.com/axiomesh/axiom-kit/types"
)

type miniPreCheck struct {
	*MockPreCheck
	pool txpool.TxPool[types.Transaction, *types.Transaction]
}

func NewMockMinPreCheck(mockCtl *gomock.Controller, pool txpool.TxPool[types.Transaction, *types.Transaction]) precheck.PreCheck {
	mockPrecheck := &miniPreCheck{
		MockPreCheck: NewMockPreCheck(mockCtl),
		pool:         pool,
	}
	mockPrecheck.EXPECT().Start().AnyTimes()
	mockPrecheck.EXPECT().UpdateEpochInfo(gomock.Any()).AnyTimes()
	mockPrecheck.EXPECT().PostUncheckedTxEvent(gomock.Any()).Do(func(ev *common.UncheckedTxEvent) {
		switch ev.EventType {
		case common.LocalTxEvent:
			txWithResp, ok := ev.Event.(*common.TxWithResp)
			if !ok {
				panic("invalid event type")
			}

			tx := txWithResp.Tx

			err := mockPrecheck.pool.AddLocalTx(tx)
			if err != nil {
				txWithResp.PoolCh <- &common.TxResp{
					Status:   false,
					ErrorMsg: err.Error(),
				}
			} else {
				txWithResp.PoolCh <- &common.TxResp{
					Status: true,
				}
			}

			txWithResp.CheckCh <- &common.TxResp{
				Status: true,
			}

		case common.RemoteTxEvent:
			txSet, ok := ev.Event.([]*types.Transaction)
			if !ok {
				panic("invalid event type")
			}
			mockPrecheck.pool.AddRemoteTxs(txSet)
		}
	}).AnyTimes()
	return mockPrecheck
}
