package txpool

import "github.com/axiomesh/axiom-ledger/internal/components/timer"

func (p *txPoolImpl[T, Constraint]) handleRemoveTimeout(timeoutEvent timer.TimeoutEvent) {
	p.timerMgr.StopTimer(timeoutEvent)
	var ev txPoolEvent
	switch timeoutEvent {
	case RemoveTx:
		ev = &removeTxsEvent{
			EventType: timeoutTxsEvent,
		}
	case CleanEmptyAccount:
		ev = &localEvent{
			EventType: gcAccountEvent,
		}
	case RotateTxLocals:
		ev = &localEvent{
			EventType: rotateTxLocalsEvent,
		}
	default:
		p.logger.Warning("unknown timeout event", timeoutEvent)
		return
	}
	p.postEvent(ev)
	err := p.timerMgr.RestartTimer(timeoutEvent)
	if err != nil {
		p.logger.Warning("failed to restart timer", err)
	}
}
