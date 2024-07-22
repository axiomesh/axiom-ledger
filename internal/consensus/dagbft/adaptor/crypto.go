package adaptor

import (
	"fmt"

	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/pkg/crypto"
	"github.com/bcds/go-hpc-dagbft/common/types/protos"
	"github.com/bcds/go-hpc-dagbft/protocol"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

var _ layer.Crypto = (*Crypto)(nil)

var _ protocol.ValidatorSigner = (*ValidatorSigner)(nil)

type Crypto struct {
	signer            *ValidatorSigner
	validatorVerifier *ValidatorVerify
	batchVerifier     *BatchVerifier
	logger            logrus.FieldLogger
}

func NewCrypto(priKey crypto.PrivateKey, conf *common.Config) (*Crypto, error) {
	logger := conf.Logger
	var validators []*protos.Validator

	var innerErr error
	validators = lo.Map(conf.ChainState.ValidatorSet, func(item chainstate.ValidatorInfo, index int) *protos.Validator {
		nodeInfo, err := conf.ChainState.GetNodeInfo(item.ID)
		if err != nil {
			innerErr = fmt.Errorf("failed to get node info: %w", err)
		}
		pubBytes, err := nodeInfo.ConsensusPubKey.Marshal()
		if err != nil {
			innerErr = fmt.Errorf("failed to marshal public key: %w", err)
		}
		return &protos.Validator{
			Hostname:    nodeInfo.Primary,
			PubKey:      pubBytes,
			ValidatorId: uint32(item.ID),
			VotePower:   uint64(item.ConsensusVotingPower),
			Workers:     nodeInfo.Workers,
		}
	})

	return &Crypto{
		signer:            newValidatorSigner(priKey, logger),
		validatorVerifier: newValidatorVerify(validators),
		batchVerifier:     newBatchVerifier(),
		logger:            logger,
	}, innerErr
}

type ValidatorSigner struct {
	priKey crypto.PrivateKey
	logger logrus.FieldLogger
}

func newValidatorSigner(priKey crypto.PrivateKey, logger logrus.FieldLogger) *ValidatorSigner {
	return &ValidatorSigner{
		priKey: priKey,
		logger: logger,
	}
}

func (v *ValidatorSigner) Sign(msg []byte) []byte {
	result, err := v.priKey.Sign(msg)
	if err != nil {
		v.logger.Error("failed to sign message", "err", err)
	}
	return result
}

type BatchVerifier struct {
}

func newBatchVerifier() *BatchVerifier {
	return &BatchVerifier{}
}

func (b *BatchVerifier) VerifyTransactions(batchTxs []protocol.Transaction) error {
	// TODO: refactor pre check to verify tx sigs here
	return nil
}

func (c *Crypto) GetValidatorSigner() protocol.ValidatorSigner {
	return c.signer
}

func (c *Crypto) GetValidatorVerifier() protocol.ValidatorVerifier {
	return c.validatorVerifier
}

func (c *Crypto) GetVerifierByValidators(validators protocol.Validators) (protocol.ValidatorVerifier, error) {
	return newValidatorVerify(validators), nil
}

func (c *Crypto) GetBatchVerifier() protocol.BatchVerifier {
	return c.batchVerifier
}
