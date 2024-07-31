package genesis

import (
	"encoding/json"
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var (
	genesisConfigKey = []byte("genesis_cfg")
)

// Initialize initialize block
func Initialize(genesis *repo.GenesisConfig, lg *ledger.Ledger) error {
	dummyRootHash := ethcommon.Hash{}
	lg.StateLedger.PrepareBlock(types.NewHash(dummyRootHash[:]), genesis.EpochInfo.StartBlock)

	if err := initializeGenesisConfig(genesis, lg.StateLedger); err != nil {
		return err
	}

	err := system.New().GenesisInit(genesis, lg.StateLedger)
	if err != nil {
		return err
	}
	lg.StateLedger.Finalise()

	stateJournal, err := lg.StateLedger.Commit()
	if err != nil {
		return err
	}

	block := &types.Block{
		Header: &types.BlockHeader{
			Number:         genesis.EpochInfo.StartBlock,
			StateRoot:      stateJournal.RootHash,
			TxRoot:         &types.Hash{},
			ReceiptRoot:    &types.Hash{},
			ParentHash:     &types.Hash{},
			Timestamp:      genesis.Timestamp,
			Epoch:          genesis.EpochInfo.Epoch,
			Bloom:          new(types.Bloom),
			ProposerNodeID: 0,
			GasPrice:       0,
			GasUsed:        0,
			TotalGasFee:    big.NewInt(0),
			GasFeeReward:   big.NewInt(0),
		},
		Transactions: []*types.Transaction{},
	}
	blockData := &ledger.BlockData{
		Block: block,
	}
	block.Header.CalculateHash()
	lg.PersistBlockData(blockData)
	if err = lg.StateLedger.Archive(block.Header, stateJournal); err != nil {
		return err
	}

	return nil
}

func IsInitialized(lg *ledger.Ledger) bool {
	account := lg.StateLedger.GetAccount(types.NewAddressByStr(common.ZeroAddress))
	if account == nil {
		return false
	}
	exists, _ := account.GetState(genesisConfigKey)
	return exists
}

func initializeGenesisConfig(genesis *repo.GenesisConfig, lg ledger.StateLedger) error {
	account := lg.GetOrCreateAccount(types.NewAddressByStr(common.ZeroAddress))

	genesisCfg, err := json.Marshal(genesis)
	if err != nil {
		return err
	}
	account.SetState(genesisConfigKey, genesisCfg)
	return nil
}

// GetGenesisConfig retrieves the genesis configuration from the given ledger.
func GetGenesisConfig(lg ledger.StateLedger) (*repo.GenesisConfig, error) {
	account := lg.GetAccount(types.NewAddressByStr(common.ZeroAddress))
	if account == nil {
		return nil, nil
	}

	state, bytes := account.GetState(genesisConfigKey)
	if !state {
		return nil, nil
	}

	genesis := &repo.GenesisConfig{}
	err := json.Unmarshal(bytes, genesis)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
