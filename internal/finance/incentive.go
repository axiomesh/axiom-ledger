package finance

//
//import (
//	"math/big"
//
//	"github.com/axiomesh/axiom-kit/types"
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/token/axc"
//	"github.com/axiomesh/axiom-ledger/internal/ledger"
//	"github.com/axiomesh/axiom-ledger/pkg/loggers"
//	"github.com/axiomesh/axiom-ledger/pkg/repo"
//	ethcommon "github.com/ethereum/go-ethereum/common"
//	"github.com/sirupsen/logrus"
//)
//
//type Incentive struct {
//	repo                *repo.GenesisConfig
//	logger              logrus.FieldLogger
//	miningAddr          ethcommon.Address
//	userAcquisitionAddr ethcommon.Address
//
//	totalAmount *big.Int
//	// todo: change the remaining in memory to the receiver address in the future, in case restart vanish the data
//	remaining    *big.Int
//	blockNumHalf uint64
//	rules        []MiningRules
//
//	axc *axc.Manager
//}
//
//func NewIncentive(config *repo.GenesisConfig, nvm common.VirtualMachine) (*Incentive, error) {
//	logger := loggers.Logger(loggers.Finance)
//
//	systemContract := nvm.GetContractInstance(types.NewAddressByStr(common.AXCContractAddr))
//	manager, ok := systemContract.(*axc.Manager)
//	if !ok {
//		return nil, ErrAXCContractType
//	}
//
//	var miningAddr, userAcquisitionAddr ethcommon.Address
//	for _, entity := range config.Incentive.Distributions {
//		if entity.Name == "Mining" {
//			miningAddr = types.NewAddressByStr(entity.Addr).ETHAddress()
//		}
//		if entity.Name == "UserAcquisition" {
//			userAcquisitionAddr = types.NewAddressByStr(entity.Addr).ETHAddress()
//		}
//	}
//	if miningAddr.String() == common.ZeroAddress || userAcquisitionAddr.String() == common.ZeroAddress {
//		return nil, ErrNoIncentiveAddrFound
//	}
//	totalAmount, _ := new(big.Int).SetString(config.Incentive.Mining.TotalAmount, 10)
//	years := config.Incentive.Mining.BlockNumToNone / config.Incentive.Mining.BlockNumToHalf
//	rules := make([]MiningRules, years)
//	accumulativeRatio := float64(0)
//	startRatio := 0.5
//	for i := uint64(0); i < years; i++ {
//		rules[i] = MiningRules{
//			startBlock: i * config.Incentive.Mining.BlockNumToHalf,
//			endBlock:   (i + 1) * config.Incentive.Mining.BlockNumToHalf,
//			percentage: startRatio,
//		}
//		accumulativeRatio += startRatio
//		if i < years-2 {
//			startRatio /= 2
//		} else {
//			startRatio = 1 - accumulativeRatio
//		}
//	}
//
//	return &Incentive{
//		repo:                config,
//		logger:              logger,
//		miningAddr:          miningAddr,
//		userAcquisitionAddr: userAcquisitionAddr,
//
//		totalAmount:  totalAmount,
//		remaining:    totalAmount,
//		blockNumHalf: config.Incentive.Mining.BlockNumToHalf,
//		rules:        rules,
//		axc:          manager,
//	}, nil
//}
//
//func (in *Incentive) SetMiningRewards(receiver ethcommon.Address, ledger ledger.StateLedger, blockHeight uint64) error {
//	msgFrom := in.miningAddr
//	logs := make([]common.Log, 0)
//	in.axc.SetContext(&common.VMContext{
//		CurrentLogs:   &logs,
//		StateLedger:   ledger,
//		CurrentHeight: blockHeight,
//		CurrentUser:   &msgFrom,
//	})
//	value := in.calculateMiningRewards(blockHeight)
//	return in.axc.Transfer(receiver, value)
//}
//
//func (in *Incentive) calculateMiningRewards(currentBlock uint64) *big.Int {
//	index := currentBlock / in.blockNumHalf
//	if index >= uint64(len(in.rules)) {
//		return big.NewInt(0)
//	}
//	percentage := big.NewInt(int64(axc.Decimals * in.rules[index].percentage))
//	rewardsTotal := new(big.Int).Div(new(big.Int).Mul(in.totalAmount, percentage), big.NewInt(axc.Decimals))
//	return new(big.Int).Div(rewardsTotal, new(big.Int).SetUint64(in.blockNumHalf))
//}
//
//func (in *Incentive) Unlock(_ ethcommon.Address, _ *big.Int) {
//	// todo: implement it
//}
//
//func (in *Incentive) SetUserAcquisitionReward(ledger ledger.StateLedger, blockHeight uint64, block *types.Block) error {
//	if blockHeight > in.repo.Incentive.UserAcquisition.BlockToNone {
//		return nil
//	}
//	msgFrom := in.userAcquisitionAddr
//	logs := make([]common.Log, 0)
//	in.axc.SetContext(&common.VMContext{
//		CurrentLogs:   &logs,
//		StateLedger:   ledger,
//		CurrentHeight: blockHeight,
//		CurrentUser:   &msgFrom,
//	})
//	return in.setUserAcquisitionReward(block)
//}
//
//func (in *Incentive) setUserAcquisitionReward(block *types.Block) error {
//	records := make(map[ethcommon.Address]int)
//	for _, transaction := range block.Transactions {
//		if transaction.GetType() == types.IncentiveTxType {
//			source := transaction.GetInner().GetIncentiveAddress()
//			if _, ok := records[*source]; !ok {
//				records[*source] = 0
//			}
//			records[*source]++
//		}
//	}
//	totalReward, _ := new(big.Int).SetString(in.repo.Incentive.UserAcquisition.AvgBlockReward, 10)
//	for address, cnt := range records {
//		percentage := float64(cnt) / float64(len(block.Transactions))
//		value := new(big.Int).Div(new(big.Int).Mul(totalReward, big.NewInt(int64(percentage*axc.Decimals))), big.NewInt(axc.Decimals))
//		if err := in.axc.Transfer(address, value); err != nil {
//			return err
//		}
//	}
//	return nil
//}
