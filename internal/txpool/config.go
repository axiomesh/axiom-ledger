package txpool

import (
	"time"

	commonpool "github.com/axiomesh/axiom-kit/txpool"
	"github.com/sirupsen/logrus"
)

// Config defines the txpool config items.
type Config struct {
	RepoRoot               string
	Logger                 logrus.FieldLogger
	ChainInfo              *commonpool.ChainInfo
	PoolSize               uint64
	ToleranceNonceGap      uint64
	ToleranceTime          time.Duration
	ToleranceRemoveTime    time.Duration
	CleanEmptyAccountTime  time.Duration
	RotateTxLocalsInterval time.Duration
	GetAccountNonce        GetAccountNonceFunc
	GetAccountBalance      GetAccountBalanceFunc
	EnableLocalsPersist    bool
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (c *Config) sanitize() {
	if c.PoolSize == 0 {
		c.PoolSize = DefaultPoolSize
	}
	if c.ToleranceTime == 0 {
		c.ToleranceTime = DefaultToleranceTime
	}
	if c.ToleranceRemoveTime == 0 {
		c.ToleranceRemoveTime = DefaultToleranceRemoveTime
	}
	if c.CleanEmptyAccountTime == 0 {
		c.CleanEmptyAccountTime = DefaultCleanEmptyAccountTime
	}
	if c.ToleranceNonceGap == 0 {
		c.ToleranceNonceGap = DefaultToleranceNonceGap
	}
	if c.RotateTxLocalsInterval == 0 {
		c.RotateTxLocalsInterval = DefaultRotateTxLocalsInterval
	}
}
