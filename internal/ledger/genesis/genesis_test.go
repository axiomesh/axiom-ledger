package genesis

import (
	"encoding/json"

	"testing"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/mock_ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestInitialize(t *testing.T) {
	mockCtl := gomock.NewController(t)
	chainLedger := mock_ledger.NewMockChainLedger(mockCtl)
	stateLedger := mock_ledger.NewMockStateLedger(mockCtl)
	mockLedger := &ledger.Ledger{
		ChainLedger: chainLedger,
		StateLedger: stateLedger,
	}

	account := ledger.NewMockAccount(1, types.NewAddressByStr(common.ZeroAddress))
	stateLedger.EXPECT().GetOrCreateAccount(gomock.Any()).Return(account).AnyTimes()
	stateLedger.EXPECT().PrepareBlock(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().SetBalance(gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().Finalise().AnyTimes()
	stateLedger.EXPECT().Commit().AnyTimes()
	chainLedger.EXPECT().PersistExecutionResult(gomock.Any(), gomock.Any()).AnyTimes()

	genesisConfig := repo.DefaultGenesisConfig(false)
	err := Initialize(genesisConfig, mockLedger)
	assert.Nil(t, err)
}

func TestGetGenesisConfig(t *testing.T) {
	mockCtl := gomock.NewController(t)
	chainLedger := mock_ledger.NewMockChainLedger(mockCtl)
	stateLedger := mock_ledger.NewMockStateLedger(mockCtl)
	mockLedger := &ledger.Ledger{
		ChainLedger: chainLedger,
		StateLedger: stateLedger,
	}
	// test get account is nil
	stateLedger.EXPECT().GetAccount(gomock.Any()).Return(nil).Times(1)
	accountGenesisConfig, err := GetGenesisConfig(mockLedger)
	assert.Nil(t, err)
	// test get state is nil
	account := ledger.NewMockAccount(1, types.NewAddressByStr(common.ZeroAddress))
	stateLedger.EXPECT().GetAccount(gomock.Any()).Return(account).AnyTimes()
	accountGenesisConfig, err = GetGenesisConfig(mockLedger)
	assert.Nil(t, err)
	// test get genesis config success
	genesisConfig := repo.DefaultGenesisConfig(false)
	genesisCfg, err := json.Marshal(genesisConfig)
	assert.Nil(t, err)
	account.SetState([]byte("genesis_cfg"), genesisCfg)
	accountGenesisConfig, err = GetGenesisConfig(mockLedger)
	assert.Nil(t, err)
	assert.NotNil(t, accountGenesisConfig.ChainID)

	// test get genesis config fail
	account.SetState([]byte("genesis_cfg"), []byte{})
	accountGenesisConfig, err = GetGenesisConfig(mockLedger)
	assert.NotNil(t, err)
}

func createMockRepo(t *testing.T) *repo.Repo {
	r, err := repo.Default(t.TempDir())
	require.Nil(t, err)
	return r
}