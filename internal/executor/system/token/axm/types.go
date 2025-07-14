package axm

import (
	"fmt"
	"math/big"

	"github.com/axiomesh/axiom-kit/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type Config struct {
	Name            string
	Symbol          string
	Decimals        uint8
	TotalSupply     *big.Int
	InitialAccounts []*InitialAccount
}

type InitialAccount struct {
	Address *types.Address
	Balance *big.Int
}

var (
	ErrTotalSupply         = errors.New("total supply below zero")
	ErrValue               = errors.New("input Value below zero")
	ErrInsufficientBalance = errors.New("Value exceeds balance")
	ErrEmptyAccount        = errors.New("account is empty")
	ErrContractAccount     = errors.New("account is not Token IToken contract account")
)

const (
	TotalSupplyKey = "totalSupplyKey"
	DecimalsKey    = "decimalsKey"

	SymbolKey     = "symbolKey"
	NameKey       = "nameKey"
	AllowancesKey = "allowancesKey"

	nameMethod         = "name"
	symbolMethod       = "symbol"
	totalSupplyMethod  = "totalSupply"
	decimalsMethod     = "decimals"
	balanceOfMethod    = "balanceOf"
	transferMethod     = "transfer"
	approveMethod      = "approve"
	allowanceMethod    = "allowance"
	transferFromMethod = "transferFrom"
	mintMethod         = "mint"
	burnMethod         = "burn"
)

func (am *Manager) checkBeforeMint(account ethcommon.Address, value *big.Int) error {
	// todo: implement it
	return nil
}

func (am *Manager) checkBeforeBurn(account ethcommon.Address, value *big.Int) error {
	// Todo implement me
	return nil
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

func GenerateConfig(genesis *repo.GenesisConfig) (Config, error) {
	var err error
	initialAccounts := make([]*InitialAccount, len(genesis.Accounts))
	// calculate the total consume balance for accounts
	accBalanceSlice := lo.Map(genesis.Accounts, func(ac *repo.Account, index int) *big.Int {
		balance, ok := new(big.Int).SetString(ac.Balance, 10)
		if !ok || balance.Sign() < 0 {
			err = fmt.Errorf("invalid balance: %s", ac.Balance)
		}
		initialAccounts[index] = &InitialAccount{
			Address: types.NewAddressByStr(ac.Address),
			Balance: balance,
		}
		return balance
	})
	if err != nil {
		return Config{}, err
	}
	consumeBalance := big.NewInt(0)
	lo.ForEach(accBalanceSlice, func(balance *big.Int, _ int) {
		consumeBalance.Add(consumeBalance, balance)
	})

	totalSupply, _ := new(big.Int).SetString(genesis.NativeToken.TotalSupply, 10)
	// calculate totalSupply - (sum<each account balance>)
	contractBalance := new(big.Int).Set(totalSupply)
	if contractBalance.Cmp(consumeBalance) < 0 {
		return Config{}, ErrTotalSupply
	}
	tokenConfig := Config{
		Name:            genesis.NativeToken.Name,
		Symbol:          genesis.NativeToken.Symbol,
		Decimals:        genesis.NativeToken.Decimals,
		InitialAccounts: initialAccounts,
		TotalSupply:     contractBalance,
	}
	return tokenConfig, nil
}
