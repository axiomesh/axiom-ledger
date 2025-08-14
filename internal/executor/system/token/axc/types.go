package axc

import (
	"fmt"
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
)

type Config struct {
	Name        string
	Symbol      string
	Decimals    uint8
	TotalSupply *big.Int
	Receivers   []*Distribution
}

type Distribution struct {
	Name         string
	Addr         string
	TotalValue   *big.Int
	InitEmission *big.Int
	Locked       bool
}

var (
	ErrTotalSupply         = errors.New("total supply below zero")
	ErrValue               = errors.New("input Value below zero")
	ErrInsufficientBalance = errors.New("value exceeds balance")
	ErrEmptyAccount        = errors.New("account is empty")
	ErrNotEnoughAllowance  = errors.New("not enough allowance")
	ErrNotImplemented      = errors.New("method not implemented")
)

const (
	TotalSupplyKey = "axcTotalSupplyKey"
	DecimalsKey    = "axcDecimalsKey"

	SymbolKey     = "axcSymbolKey"
	NameKey       = "axcNameKey"
	AllowancesKey = "axcAllowancesKey"

	// BalancesKey is a map stores axc balance, mapping(address => uint256)
	BalancesKey = "axcBalances"
	// LockedBalanceKey is a map stores axc locked balance, mapping(address => uint256)
	LockedBalanceKey = "axcLockedBalance"

	nameMethod         = "name"
	symbolMethod       = "symbol"
	totalSupplyMethod  = "totalSupply"
	decimalsMethod     = "decimals"
	balanceOfMethod    = "balanceOf"
	transferMethod     = "transfer"
	approveMethod      = "approve"
	allowanceMethod    = "allowance"
	transferFromMethod = "transferFrom"
	unlockMethod       = "unlock"

	Decimals = 10000
)

var Event2Sig = map[string]string{
	approveMethod:  "Approval(address,address,uint256)",
	transferMethod: "Transfer(address,address,uint256)",
}

func checkValue(value *big.Int) error {
	if value.Sign() < 0 {
		return ErrValue
	}
	return nil
}

func getAllowancesKey(owner, spender ethcommon.Address) string {
	return fmt.Sprintf("%s-%s-%s", AllowancesKey, owner.String(), spender.String())
}

func getBalancesKey(owner ethcommon.Address) string {
	return fmt.Sprintf("%s-%s", BalancesKey, owner.String())
}

func getLockedBalanceKey(owner ethcommon.Address) string {
	return fmt.Sprintf("%s-%s", LockedBalanceKey, owner.String())
}

//func GenerateConfig(genesis *repo.GenesisConfig) (Config, error) {
//	totalSupply, _ := new(big.Int).SetString(genesis.Axc.TotalSupply, 10)
//	if totalSupply.Cmp(big.NewInt(0)) < 0 {
//		return Config{}, ErrTotalSupply
//	}
//	receivers := make([]*Distribution, 0)
//	lo.ForEach(genesis.Incentive.Distributions, func(entity *repo.Distribution, index int) {
//		percentage := big.NewInt(int64(entity.Percentage * Decimals))
//		totalValue := new(big.Int).Div(new(big.Int).Mul(percentage, totalSupply), big.NewInt(Decimals))
//		emission := big.NewInt(int64(entity.InitEmission * Decimals))
//		emissionValue := new(big.Int).Div(new(big.Int).Mul(emission, totalValue), big.NewInt(Decimals))
//		receivers = append(receivers, &Distribution{
//			Name:         entity.Name,
//			Addr:         entity.Addr,
//			TotalValue:   totalValue,
//			InitEmission: emissionValue,
//			Locked:       entity.Locked,
//		})
//	})
//	tokenConfig := Config{
//		Name:        genesis.Axc.Name,
//		Symbol:      genesis.Axc.Symbol,
//		Decimals:    genesis.Axc.Decimals,
//		Receivers:   receivers,
//		TotalSupply: totalSupply,
//	}
//	return tokenConfig, nil
//}
