package data_syncer

import (
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
)

func (n *Node[T, Constraint]) handleTimeout(name timer.TimeoutEvent) {
	switch name {
	case syncStateRestart:
		n.logger.Info("restart sync state timer")
		if !n.statusMgr.InOne(InCommit, StateTransferring, InSyncState) {
			n.postEvent(n.genSyncStateEvent())
		}

		if err := n.timeMgr.RestartTimer(syncStateRestart); err != nil {
			n.logger.Error(err)
		}

	case fetchMissingTxsResp:
		n.logger.Info("handle fetchMissingTxsResp timeout")
		n.missingTxsInFetchingLock.Lock()
		request := n.missingBatchesInFetching
		if request == nil {
			n.missingTxsInFetchingLock.Unlock()
			n.logger.Debugf("no request found")
			return
		}
		request.retryCount++
		n.missingTxsInFetchingLock.Unlock()

		if request.retryCount > maxRetryCount {
			n.logger.Warningf("fetchMissingTxsResp retry count exceed max retry count: %d, request: %+v, start sync state", request.retryCount, request)
			n.statusMgr.Off(InCommit)
			n.missingTxsInFetchingLock.Lock()
			n.missingBatchesInFetching = nil
			n.missingTxsInFetchingLock.Unlock()
			n.postEvent(n.genSyncStateEvent())
			return
		}

		n.logger.Infof("retry fetchMissingTxsResp, retry count: %d, request: %+v", request.retryCount, request)
		if err := n.fetchMissingTxs(request.request, request.proposer); err != nil {
			n.logger.Error(err)
			return
		}

		if err := n.timeMgr.RestartTimer(fetchMissingTxsResp); err != nil {
			n.logger.Error(err)
		}

	case syncStateResp:
		if n.statusMgr.In(InEpochSyncing) {
			n.timeMgr.StopTimer(syncStateResp)
			n.logger.Infof("stop syncStateResp timer because current status is InEpochSyncing")
			return
		}
		n.logger.Info("handle syncStateResp timeout")
		if err := n.fetchSyncState(); err != nil {
			n.logger.Error(err)
			return
		}
		if err := n.timeMgr.RestartTimer(syncStateResp); err != nil {
			n.logger.Error(err)
		}
	case checkTxPool:
		n.logger.Info("handle checkTxPool timeout")
		n.timeMgr.StopTimer(checkTxPool)
		n.postEvent(&localEvent{EventType: eventType_checkTxPool})
	}
}
