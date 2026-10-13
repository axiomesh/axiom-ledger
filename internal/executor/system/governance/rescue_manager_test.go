package governance

import (
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/mock_ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

// nonSystemAddr is an arbitrary non-system address used as the
// rescue target in these tests. It MUST NOT match any entry in
// rescueForbiddenTargets.
const nonSystemAddr = "0x3A1092655b3a9436253c813bcF02E445E3340333"

func newRescueTestGov(t *testing.T, admins []*repo.Admin) (*Governance, *mock_ledger.MockStateLedger) {
	t.Helper()
	logger := logrus.New()
	gov := NewGov(&common.SystemContractConfig{
		Logger: logger,
	})

	mockCtl := gomock.NewController(t)
	stateLedger := mock_ledger.NewMockStateLedger(mockCtl)

	account := ledger.NewMockAccount(2, types.NewAddressByStr(common.GovernanceContractAddr))

	stateLedger.EXPECT().GetOrCreateAccount(gomock.Any()).Return(account).AnyTimes()
	stateLedger.EXPECT().AddLog(gomock.Any()).AnyTimes()
	stateLedger.EXPECT().SetBalance(gomock.Any(), gomock.Any()).AnyTimes()

	if len(admins) > 0 {
		err := InitCouncilMembers(stateLedger, admins)
		assert.Nil(t, err)
	}

	// NewGov already registers the RescueManager override (see
	// governance.go:178), so no extra Register call is needed.
	return gov, stateLedger
}

func setRescueContext(gov *Governance, stateLedger *mock_ledger.MockStateLedger, caller string, height uint64) {
	addr := types.NewAddressByStr(caller).ETHAddress()
	logs := make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		CurrentUser:   &addr,
		CurrentHeight: height,
		CurrentLogs:   &logs,
		StateLedger:   stateLedger,
	})
}

func makeRescueExtra(target string, slot uint64) []byte {
	// 32 bytes of zero — enough to satisfy the length check and is
	// a safe value to write to an arbitrary slot during tests.
	value := "0x0000000000000000000000000000000000000000000000000000000000000000"
	b, err := json.Marshal(RescueExtraArgs{
		Target: target,
		Slot:   slot,
		Value:  value,
	})
	if err != nil {
		panic(err)
	}
	return b
}

// TestRescueManager_SoloShortCircuit covers the solo / single-admin
// path: with no council (or a single admin), the proposer's default
// pass-vote already satisfies SimpleStrategy, so Propose itself
// flips the proposal to Approved and triggers the forced write in
// the same transaction. The user does NOT need to send a separate
// Vote tx — the original single-tx rescue behaviour is preserved.
func TestRescueManager_SoloShortCircuit(t *testing.T) {
	gov, stateLedger := newRescueTestGov(t, nil)
	setRescueContext(gov, stateLedger, nonSystemAddr, 1)

	// Expect the forced write to fire during Propose — the solo
	// short-circuit goes straight from Propose to Execute without
	// waiting for a Vote tx.
	stateLedger.EXPECT().SetState(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

	extra := makeRescueExtra(nonSystemAddr, 12)
	err := gov.Propose(uint8(RescueState), "rescue", "rescue desc", 100, extra)
	assert.Nil(t, err)

	proposal, err := gov.Proposal(gov.GetLatestProposalID())
	assert.Nil(t, err)
	assert.Equal(t, RescueState, proposal.Type)
	assert.Equal(t, Approved, proposal.Status)
	assert.Equal(t, uint64(1), proposal.EffectiveBlockNumber)
	assert.Equal(t, uint64(1), proposal.CreatedBlockNumber)
}

// TestRescueManager_SoloShortCircuitSingleAdmin verifies the same
// short-circuit triggers with a single-member council. With
// TotalVotes=1 and PassVotes=[proposer], 1*2 > 1 = true, so the
// proposal is still auto-approved at Propose time.
func TestRescueManager_SoloShortCircuitSingleAdmin(t *testing.T) {
	gov, stateLedger := newRescueTestGov(t, []*repo.Admin{
		{Address: admin1, Weight: 1, Name: "111"},
	})
	setRescueContext(gov, stateLedger, admin1, 1)
	stateLedger.EXPECT().SetState(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

	extra := makeRescueExtra(nonSystemAddr, 12)
	err := gov.Propose(uint8(RescueState), "rescue", "rescue desc", 100, extra)
	assert.Nil(t, err)

	proposal, err := gov.Proposal(gov.GetLatestProposalID())
	assert.Nil(t, err)
	assert.Equal(t, Approved, proposal.Status)
}

// TestRescueManager_MultiAdminRequiresVote covers the multi-admin
// path: with a 4-member council, a single pass-vote is NOT enough
// (1*2 > 4 = false). The short-circuit does NOT fire, the
// proposal stays Voting, and the forced write only happens once a
// second council member votes pass.
func TestRescueManager_MultiAdminRequiresVote(t *testing.T) {
	gov, stateLedger := newRescueTestGov(t, []*repo.Admin{
		{Address: admin1, Weight: 1, Name: "111"},
		{Address: admin2, Weight: 1, Name: "222"},
		{Address: admin3, Weight: 1, Name: "333"},
		{Address: admin4, Weight: 1, Name: "444"},
	})

	// admin1 proposes — proposer's default pass-vote (1 of 4) is
	// not enough, so the short-circuit does NOT fire.
	setRescueContext(gov, stateLedger, admin1, 1)
	extra := makeRescueExtra(nonSystemAddr, 12)
	err := gov.Propose(uint8(RescueState), "rescue", "rescue desc", 100, extra)
	assert.Nil(t, err)

	proposal, err := gov.Proposal(gov.GetLatestProposalID())
	assert.Nil(t, err)
	assert.Equal(t, Voting, proposal.Status)
	assert.Equal(t, uint64(0), proposal.EffectiveBlockNumber)

	// admin2 votes pass — 2 of 4. 2*2 > 4 (false). Still Voting.
	setRescueContext(gov, stateLedger, admin2, 2)
	err = gov.Vote(gov.GetLatestProposalID(), uint8(Pass))
	assert.Nil(t, err)
	proposal, err = gov.Proposal(gov.GetLatestProposalID())
	assert.Nil(t, err)
	assert.Equal(t, Voting, proposal.Status)

	// admin3 votes pass — 3 of 4. 3*2 > 4 (true) → Approved →
	// Execute runs.
	stateLedger.EXPECT().SetState(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	setRescueContext(gov, stateLedger, admin3, 3)
	err = gov.Vote(gov.GetLatestProposalID(), uint8(Pass))
	assert.Nil(t, err)
	proposal, err = gov.Proposal(gov.GetLatestProposalID())
	assert.Nil(t, err)
	assert.Equal(t, Approved, proposal.Status)
	assert.Equal(t, uint64(3), proposal.EffectiveBlockNumber)
}

// TestRescueManager_MultiAdminRejectNoExecute verifies that a
// proposal can be rejected by majority, and the rejection path
// does NOT trigger the forced write.
func TestRescueManager_MultiAdminRejectNoExecute(t *testing.T) {
	gov, stateLedger := newRescueTestGov(t, []*repo.Admin{
		{Address: admin1, Weight: 1, Name: "111"},
		{Address: admin2, Weight: 1, Name: "222"},
		{Address: admin3, Weight: 1, Name: "333"},
		{Address: admin4, Weight: 1, Name: "444"},
	})

	setRescueContext(gov, stateLedger, admin1, 1)
	extra := makeRescueExtra(nonSystemAddr, 12)
	err := gov.Propose(uint8(RescueState), "rescue", "rescue desc", 100, extra)
	assert.Nil(t, err)

	// admin2 rejects — 0 pass, 1 reject. 1*2 >= 4? No. Still Voting.
	setRescueContext(gov, stateLedger, admin2, 2)
	err = gov.Vote(gov.GetLatestProposalID(), uint8(Reject))
	assert.Nil(t, err)

	// admin3 rejects — 0 pass, 2 reject. 2*2 >= 4? Yes → Rejected.
	// No SetState must be observed.
	setRescueContext(gov, stateLedger, admin3, 3)
	err = gov.Vote(gov.GetLatestProposalID(), uint8(Reject))
	assert.Nil(t, err)
	proposal, err := gov.Proposal(gov.GetLatestProposalID())
	assert.Nil(t, err)
	assert.Equal(t, Rejected, proposal.Status)
}

// TestRescueManager_VoteRejectsNonCouncilMember ensures that
// the standard council-membership check applies — a non-council
// address can propose in solo mode, but cannot vote in multi-admin
// mode.
func TestRescueManager_VoteRejectsNonCouncilMember(t *testing.T) {
	gov, stateLedger := newRescueTestGov(t, []*repo.Admin{
		{Address: admin1, Weight: 1, Name: "111"},
		{Address: admin2, Weight: 1, Name: "222"},
	})

	// admin1 proposes.
	setRescueContext(gov, stateLedger, admin1, 1)
	extra := makeRescueExtra(nonSystemAddr, 12)
	err := gov.Propose(uint8(RescueState), "rescue", "rescue desc", 100, extra)
	assert.Nil(t, err)

	// Some non-council address tries to vote.
	setRescueContext(gov, stateLedger, nonSystemAddr, 2)
	err = gov.Vote(gov.GetLatestProposalID(), uint8(Pass))
	assert.Equal(t, ErrNotFoundCouncilMember, err)
}

