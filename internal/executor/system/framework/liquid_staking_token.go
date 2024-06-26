package framework

import (
	"fmt"
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/liquid_staking_token"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/liquid_staking_token_client"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	LiquidStakingTokenNextIDStorageKey           = "nextID"
	LiquidStakingTokenInfoStorageKey             = "info"
	LiquidStakingTokenOwnerStorageKey            = "owner"
	LiquidStakingTokenBalanceStorageKey          = "balance"
	LiquidStakingTokenApprovalStorageKey         = "tokenApproval"
	LiquidStakingTokenOperatorApprovalStorageKey = "operatorApproval"
)

var LiquidStakingTokenBuildConfig = &common.SystemContractBuildConfig[*LiquidStakingToken]{
	Name:    "framework_liquid_staking_token",
	Address: common.LiquidStakingTokenContractAddr,
	AbiStr:  liquid_staking_token_client.BindingContractMetaData.ABI,
	Constructor: func(systemContractBase common.SystemContractBase) *LiquidStakingToken {
		return &LiquidStakingToken{
			SystemContractBase: systemContractBase,
		}
	},
}

var _ liquid_staking_token.LiquidStakingToken = (*LiquidStakingToken)(nil)

type OperatorApprovalKey struct {
	Owner    ethcommon.Address
	Operator ethcommon.Address
}

type LiquidStakingToken struct {
	common.SystemContractBase

	// self-increment token id (start from 1)
	nextID *common.VMSlot[*big.Int]

	// tokenID -> token info
	infoMap *common.VMMap[*big.Int, liquid_staking_token.LiquidStakingTokenInfo]

	// tokenID -> owner
	ownerMap *common.VMMap[*big.Int, ethcommon.Address]

	// owner -> balance
	balanceMap *common.VMMap[ethcommon.Address, *big.Int]

	// tokenID -> approved operator
	tokenApprovalMap *common.VMMap[*big.Int, ethcommon.Address]

	// owner -> operator -> approved
	operatorApprovalMap *common.VMMap[OperatorApprovalKey, bool]
}

func (lst *LiquidStakingToken) GenesisInit(genesis *repo.GenesisConfig) error {
	return nil
}

func (lst *LiquidStakingToken) SetContext(context *common.VMContext) {
	lst.SystemContractBase.SetContext(context)

	lst.nextID = common.NewVMSlot[*big.Int](lst.StateAccount, LiquidStakingTokenNextIDStorageKey)
	lst.infoMap = common.NewVMMap[*big.Int, liquid_staking_token.LiquidStakingTokenInfo](lst.StateAccount, LiquidStakingTokenInfoStorageKey, func(key *big.Int) string {
		return key.String()
	})
	lst.ownerMap = common.NewVMMap[*big.Int, ethcommon.Address](lst.StateAccount, LiquidStakingTokenOwnerStorageKey, func(key *big.Int) string {
		return key.String()
	})
	lst.balanceMap = common.NewVMMap[ethcommon.Address, *big.Int](lst.StateAccount, LiquidStakingTokenBalanceStorageKey, func(key ethcommon.Address) string {
		return key.String()
	})
	lst.tokenApprovalMap = common.NewVMMap[*big.Int, ethcommon.Address](lst.StateAccount, LiquidStakingTokenApprovalStorageKey, func(key *big.Int) string {
		return key.String()
	})
	lst.operatorApprovalMap = common.NewVMMap[OperatorApprovalKey, bool](lst.StateAccount, LiquidStakingTokenOperatorApprovalStorageKey, func(key OperatorApprovalKey) string {
		return fmt.Sprintf("%s_%s", key.Owner, key.Operator)
	})
}

func (lst *LiquidStakingToken) updateInfo(tokenID *big.Int, info *liquid_staking_token.LiquidStakingTokenInfo) error {
	if err := lst.infoMap.Put(tokenID, *info); err != nil {
		return err
	}
	lst.EmitUpdateInfoEvent(tokenID, info.Principal, info.Unlocked, info.ActiveEpoch)
	return nil
}

func (lst *LiquidStakingToken) GetInfo(tokenID *big.Int) (info liquid_staking_token.LiquidStakingTokenInfo, err error) {
	exist, info, err := lst.infoMap.Get(tokenID)
	if err != nil {
		return liquid_staking_token.LiquidStakingTokenInfo{}, err
	}
	if !exist {
		return liquid_staking_token.LiquidStakingTokenInfo{}, nil
	}
	return info, nil
}

func (lst *LiquidStakingToken) GetInfos(_tokenIds []*big.Int) ([]liquid_staking_token.LiquidStakingTokenInfo, error) {
	var infos []liquid_staking_token.LiquidStakingTokenInfo
	for _, tokenID := range _tokenIds {
		info, err := lst.infoMap.MustGet(tokenID)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (lst *LiquidStakingToken) lockedReward(info liquid_staking_token.LiquidStakingTokenInfo) (*big.Int, error) {
	stakingManager := StakingManagerBuildConfig.Build(lst.CrossCallSystemContractContext())
	currentEpoch, err := EpochManagerBuildConfig.Build(lst.CrossCallSystemContractContext()).CurrentEpoch()
	if err != nil {
		return nil, err
	}
	// add stake epoch: info.ActiveEpoch-1 == currentEpoch.Epoch
	// add stake next epoch: info.ActiveEpoch == currentEpoch.Epoch
	if info.ActiveEpoch-1 == currentEpoch.Epoch || info.ActiveEpoch == currentEpoch.Epoch {
		return big.NewInt(0), nil
	}

	stakingRate, err := stakingManager.GetPoolHistoryLiquidStakingTokenRate(info.PoolID, info.ActiveEpoch-1)
	if err != nil {
		return nil, err
	}
	lastRate, err := stakingManager.GetPoolHistoryLiquidStakingTokenRate(info.PoolID, currentEpoch.Epoch-1)
	if err != nil {
		return nil, err
	}
	principalAndReward := calculatePrincipalAndReward(stakingRate, lastRate, info.Principal)
	return new(big.Int).Sub(principalAndReward, info.Principal), nil
}

func (lst *LiquidStakingToken) GetLockedReward(tokenID *big.Int) (*big.Int, error) {
	exist, info, err := lst.infoMap.Get(tokenID)
	if err != nil {
		return nil, err
	}
	if !exist {
		return big.NewInt(0), nil
	}

	return lst.lockedReward(info)
}

func (lst *LiquidStakingToken) GetUnlockingCoin(tokenID *big.Int) (*big.Int, error) {
	exist, info, err := lst.infoMap.Get(tokenID)
	if err != nil {
		return nil, err
	}
	if !exist {
		return big.NewInt(0), nil
	}
	unlockingToken := big.NewInt(0)
	for _, record := range info.UnlockingRecords {
		if record.UnlockTimestamp > lst.Ctx.CurrentEVM.Context.Time {
			unlockingToken = unlockingToken.Add(unlockingToken, record.Amount)
		}
	}
	return unlockingToken, nil
}

func (lst *LiquidStakingToken) GetUnlockedCoin(tokenID *big.Int) (*big.Int, error) {
	exist, info, err := lst.infoMap.Get(tokenID)
	if err != nil {
		return nil, err
	}
	if !exist {
		return big.NewInt(0), nil
	}
	unlockedToken := info.Unlocked
	for _, record := range info.UnlockingRecords {
		if record.UnlockTimestamp <= lst.Ctx.CurrentEVM.Context.Time {
			unlockedToken = unlockedToken.Add(unlockedToken, record.Amount)
		}
	}
	return unlockedToken, nil
}

func (lst *LiquidStakingToken) GetLockedCoin(tokenID *big.Int) (*big.Int, error) {
	exist, info, err := lst.infoMap.Get(tokenID)
	if err != nil {
		return nil, err
	}
	if !exist {
		return big.NewInt(0), nil
	}

	return lst.getLockedCoin(info)
}

func (lst *LiquidStakingToken) getLockedCoin(info liquid_staking_token.LiquidStakingTokenInfo) (*big.Int, error) {
	currentEpoch, err := EpochManagerBuildConfig.Build(lst.CrossCallSystemContractContext()).CurrentEpoch()
	if err != nil {
		return nil, err
	}
	if info.ActiveEpoch > currentEpoch.Epoch {
		return big.NewInt(0), nil
	}
	if info.ActiveEpoch == currentEpoch.Epoch {
		return info.Principal, nil
	}

	lockedReward, err := lst.lockedReward(info)
	if err != nil {
		return nil, err
	}
	return lockedReward.Add(lockedReward, info.Principal), nil
}

func (lst *LiquidStakingToken) GetTotalCoin(tokenID *big.Int) (*big.Int, error) {
	exist, info, err := lst.infoMap.Get(tokenID)
	if err != nil {
		return nil, err
	}
	if !exist {
		return big.NewInt(0), nil
	}

	totalCoin := new(big.Int).Set(info.Principal)
	lockedReward, err := lst.lockedReward(info)
	if err != nil {
		return nil, err
	}
	totalCoin = totalCoin.Add(totalCoin, lockedReward)
	for _, record := range info.UnlockingRecords {
		totalCoin = totalCoin.Add(totalCoin, record.Amount)
	}
	return totalCoin, nil
}

func (lst *LiquidStakingToken) Name() string {
	return "liquid staking AXC"
}

func (lst *LiquidStakingToken) Symbol() string {
	return "lstAXC"
}

func (lst *LiquidStakingToken) BalanceOf(owner ethcommon.Address) (*big.Int, error) {
	if common.IsZeroAddress(owner) {
		return nil, errors.Errorf("invalid owner: %s", common.ZeroAddress)
	}

	exist, balance, err := lst.balanceMap.Get(owner)
	if err != nil {
		return nil, err
	}
	if !exist {
		return big.NewInt(0), nil
	}
	return balance, nil
}

func (lst *LiquidStakingToken) Mint(to ethcommon.Address, info *liquid_staking_token.LiquidStakingTokenInfo) (tokenID *big.Int, err error) {
	exist, nextID, err := lst.nextID.Get()
	if err != nil {
		return nil, err
	}
	if !exist {
		nextID = big.NewInt(1)
	}

	// set info
	if err = lst.infoMap.Put(nextID, *info); err != nil {
		return nil, err
	}

	// update nextID
	if err := lst.nextID.Put(big.NewInt(0).Add(nextID, big.NewInt(1))); err != nil {
		return nil, err
	}

	_, err = lst.updateOwnership(to, nextID, ethcommon.Address{})
	if err != nil {
		return nil, err
	}
	return nextID, nil
}

func (lst *LiquidStakingToken) Burn(tokenID *big.Int) error {
	exist, owner, err := lst.ownerMap.Get(tokenID)
	if err != nil {
		return err
	}
	if !exist || common.IsZeroAddress(owner) {
		return errors.Errorf("token not exist: %s", tokenID.String())
	}

	_, err = lst.updateOwnership(ethcommon.Address{}, tokenID, lst.Ctx.From)
	if err != nil {
		return err
	}

	return nil
}

func (lst *LiquidStakingToken) ownerOf(tokenID *big.Int) (ethcommon.Address, error) {
	exist, owner, err := lst.ownerMap.Get(tokenID)
	if err != nil {
		return ethcommon.Address{}, err
	}
	if !exist {
		return ethcommon.Address{}, nil
	}
	return owner, nil
}

func (lst *LiquidStakingToken) OwnerOf(tokenID *big.Int) (ethcommon.Address, error) {
	owner, err := lst.ownerOf(tokenID)
	if err != nil {
		return ethcommon.Address{}, err
	}
	if common.IsZeroAddress(owner) {
		return ethcommon.Address{}, errors.Errorf("token not exist: %s", tokenID.String())
	}
	return owner, nil
}

func (lst *LiquidStakingToken) SafeTransferFrom(from ethcommon.Address, to ethcommon.Address, tokenID *big.Int) error {
	return lst.SafeTransferFrom0(from, to, tokenID, nil)
}

func (lst *LiquidStakingToken) SafeTransferFrom0(from ethcommon.Address, to ethcommon.Address, tokenID *big.Int, data []byte) error {
	if err := lst.TransferFrom(from, to, tokenID); err != nil {
		return err
	}
	if lst.Ctx.StateLedger.GetCodeSize(types.NewAddress(to.Bytes())) != 0 {
		// TODO: support call to.onERC721Received(operator, from, tokenId, data)
	}

	return nil
}

func (lst *LiquidStakingToken) TransferFrom(from ethcommon.Address, to ethcommon.Address, tokenID *big.Int) error {
	if common.IsZeroAddress(to) {
		return errors.Errorf("invalid receiver: %s", common.ZeroAddress)
	}
	previousOwner, err := lst.updateOwnership(to, tokenID, lst.Ctx.From)
	if err != nil {
		return err
	}
	if previousOwner != from {
		return errors.Errorf("incorrect owner, from: %s, tokenId: %s, previousOwner: %s", from.String(), tokenID.String(), previousOwner.String())
	}
	return nil
}

func (lst *LiquidStakingToken) Approve(to ethcommon.Address, tokenID *big.Int) error {
	return lst.approve(to, tokenID, lst.Ctx.From, true)
}

func (lst *LiquidStakingToken) SetApprovalForAll(operator ethcommon.Address, approved bool) error {
	if common.IsZeroAddress(operator) {
		return errors.Errorf("invalid operator: %s", common.ZeroAddress)
	}

	if err := lst.operatorApprovalMap.Put(OperatorApprovalKey{
		Owner:    lst.Ctx.From,
		Operator: operator,
	}, approved); err != nil {
		return err
	}

	lst.EmitApprovalForAllEvent(lst.Ctx.From, operator, approved)
	return nil
}

func (lst *LiquidStakingToken) GetApproved(tokenID *big.Int) (operator ethcommon.Address, err error) {
	exist, approved, err := lst.tokenApprovalMap.Get(tokenID)
	if err != nil {
		return ethcommon.Address{}, err
	}
	if !exist {
		return ethcommon.Address{}, nil
	}
	return approved, nil
}

func (lst *LiquidStakingToken) IsApprovedForAll(owner ethcommon.Address, operator ethcommon.Address) (bool, error) {
	exist, approved, err := lst.operatorApprovalMap.Get(OperatorApprovalKey{
		Owner:    owner,
		Operator: operator,
	})
	if err != nil {
		return false, err
	}
	return exist && approved, nil
}

func (lst *LiquidStakingToken) EmitTransferEvent(from, to ethcommon.Address, tokenID *big.Int) {
	lst.EmitEvent(&liquid_staking_token.EventTransfer{
		From:    from,
		To:      to,
		TokenId: tokenID,
	})
}

func (lst *LiquidStakingToken) EmitApprovalEvent(owner, approved ethcommon.Address, tokenID *big.Int) {
	lst.EmitEvent(&liquid_staking_token.EventApproval{
		Owner:    owner,
		Approved: approved,
		TokenId:  tokenID,
	})
}

func (lst *LiquidStakingToken) EmitApprovalForAllEvent(owner, operator ethcommon.Address, approved bool) {
	lst.EmitEvent(&liquid_staking_token.EventApprovalForAll{
		Owner:    owner,
		Operator: operator,
		Approved: approved,
	})
}

func (lst *LiquidStakingToken) EmitUpdateInfoEvent(tokenID *big.Int, newPrincipal *big.Int, newUnlocked *big.Int, newActiveEpoch uint64) {
	lst.EmitEvent(&liquid_staking_token.EventUpdateInfo{
		TokenId:        tokenID,
		NewPrincipal:   newPrincipal,
		NewUnlocked:    newUnlocked,
		NewActiveEpoch: newActiveEpoch,
	})
}

func (lst *LiquidStakingToken) isAuthorized(owner ethcommon.Address, spender ethcommon.Address, tokenID *big.Int) (bool, error) {
	if common.IsZeroAddress(spender) {
		return false, nil
	}

	if owner == spender {
		return true, nil
	}

	isApprovedForAll, err := lst.IsApprovedForAll(owner, spender)
	if err != nil {
		return false, err
	}
	if isApprovedForAll {
		return true, nil
	}
	_, approved, err := lst.tokenApprovalMap.Get(tokenID)
	if err != nil {
		return false, err
	}
	if approved == spender {
		return true, nil
	}

	return false, nil
}

func (lst *LiquidStakingToken) checkAuthorized(owner ethcommon.Address, spender ethcommon.Address, tokenID *big.Int) error {
	authorized, err := lst.isAuthorized(owner, spender, tokenID)
	if err != nil {
		return err
	}
	if !authorized {
		if common.IsZeroAddress(owner) {
			return errors.Errorf("token not exist: %s", tokenID.String())
		}
		return errors.Errorf("insufficient approval, spender: %s, token: %s", spender.String(), tokenID.String())
	}

	return nil
}

func (lst *LiquidStakingToken) requireOwned(tokenID *big.Int) (ethcommon.Address, error) {
	owner, err := lst.ownerOf(tokenID)
	if err != nil {
		return ethcommon.Address{}, err
	}
	if common.IsZeroAddress(owner) {
		return ethcommon.Address{}, errors.Errorf("token not exist: %s", tokenID.String())
	}
	return owner, nil
}

func (lst *LiquidStakingToken) approve(to ethcommon.Address, tokenID *big.Int, auth ethcommon.Address, emitEvent bool) error {
	if emitEvent || !common.IsZeroAddress(auth) {
		owner, err := lst.requireOwned(tokenID)
		if err != nil {
			return err
		}

		if !common.IsZeroAddress(auth) && owner != auth {
			isApprovedForAll, err := lst.IsApprovedForAll(owner, auth)
			if err != nil {
				return err
			}
			if !isApprovedForAll {
				return errors.Errorf("invalid approval, auth: %s, token: %s", auth.String(), tokenID.String())
			}
		}

		if emitEvent {
			lst.EmitApprovalEvent(owner, to, tokenID)
		}
	}
	return lst.tokenApprovalMap.Put(tokenID, to)
}

func (lst *LiquidStakingToken) updateOwnership(to ethcommon.Address, tokenID *big.Int, auth ethcommon.Address) (previousOwner ethcommon.Address, err error) {
	from, err := lst.ownerOf(tokenID)
	if err != nil {
		return ethcommon.Address{}, err
	}

	if !common.IsZeroAddress(auth) {
		if err := lst.checkAuthorized(from, auth, tokenID); err != nil {
			return ethcommon.Address{}, err
		}
	}

	if !common.IsZeroAddress(from) {
		if err := lst.approve(ethcommon.Address{}, tokenID, ethcommon.Address{}, false); err != nil {
			return ethcommon.Address{}, err
		}
		// update balance
		exist, balance, err := lst.balanceMap.Get(from)
		if err != nil {
			return ethcommon.Address{}, err
		}
		if !exist {
			balance = big.NewInt(0)
		} else {
			balance = balance.Sub(balance, big.NewInt(1))
		}
		if err = lst.balanceMap.Put(from, balance); err != nil {
			return ethcommon.Address{}, err
		}
	}

	if !common.IsZeroAddress(to) {
		exist, balance, err := lst.balanceMap.Get(to)
		if err != nil {
			return ethcommon.Address{}, err
		}
		if !exist {
			balance = big.NewInt(0)
		}
		balance = balance.Add(balance, big.NewInt(1))
		if err = lst.balanceMap.Put(to, balance); err != nil {
			return ethcommon.Address{}, err
		}
	}

	if err = lst.ownerMap.Put(tokenID, to); err != nil {
		return ethcommon.Address{}, err
	}

	lst.EmitTransferEvent(from, to, tokenID)

	return from, nil
}

func (lst *LiquidStakingToken) mustGetInfo(tokenID *big.Int) (*liquid_staking_token.LiquidStakingTokenInfo, error) {
	info, err := lst.GetInfo(tokenID)
	if err != nil {
		return nil, err
	}
	if info.PoolID == 0 {
		return nil, errors.Errorf("token not exist: %s", tokenID.String())
	}
	return &info, nil
}

func updateLiquidStakingTokenUnlockingRecords(blockTimestamp uint64, info *liquid_staking_token.LiquidStakingTokenInfo) {
	var remainRecords []liquid_staking_token.UnlockingRecord
	for _, record := range info.UnlockingRecords {
		if record.UnlockTimestamp <= blockTimestamp {
			info.Unlocked = info.Unlocked.Add(info.Unlocked, record.Amount)
		} else {
			remainRecords = append(remainRecords, record)
		}
	}
	info.UnlockingRecords = remainRecords
}
