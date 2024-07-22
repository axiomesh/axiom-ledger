package adaptor

import (
	"crypto/ed25519"
	"fmt"
	"math"

	"github.com/bcds/go-hpc-dagbft/common/errors"
	"github.com/bcds/go-hpc-dagbft/common/types"
	"github.com/bcds/go-hpc-dagbft/common/utils/concurrency"
	"github.com/bcds/go-hpc-dagbft/protocol"
)

type ValidatorVerify struct {
	nodes         protocol.Validators
	totalPower    protocol.VotePower
	quorumPower   protocol.VotePower
	validityPower protocol.VotePower
}

func newValidatorVerify(validators []*protocol.Validator) *ValidatorVerify {
	totalPower := protocol.VotePower(0)
	for _, validator := range validators {
		totalPower += validator.VotePower
	}
	faultPower := (totalPower - 1) / 3
	validityPower := faultPower + 1
	quorumPower := protocol.VotePower(math.Ceil(float64(totalPower+faultPower+1) / float64(2)))

	return &ValidatorVerify{
		nodes:         validators,
		totalPower:    totalPower,
		quorumPower:   quorumPower,
		validityPower: validityPower,
	}
}

func id2index(id types.ValidatorID) uint32 {
	return id - 1
}

func (m *ValidatorVerify) GetValidators() []*protocol.Validator { return m.nodes }

func (m *ValidatorVerify) TotalPower() protocol.VotePower { return m.totalPower }

func (m *ValidatorVerify) QuorumPower() protocol.VotePower { return m.quorumPower }

func (m *ValidatorVerify) ValidityPower() protocol.VotePower { return m.validityPower }

func (m *ValidatorVerify) Verify(message []byte, signature protocol.Signature) error {
	index := id2index(signature.Signer)
	if index >= uint32(len(m.nodes)) {
		return fmt.Errorf("signer %d is not in validators", signature.Signer)
	}
	node := m.nodes[index]
	if signature.Signer != node.ValidatorId {
		return fmt.Errorf("invalid signer %d", signature.Signer)
	}
	if !ed25519.Verify(node.PubKey, message, signature.Signature) {
		return fmt.Errorf("invalid signature by signer %d", signature.Signer)
	}
	return nil
}

func (m *ValidatorVerify) QuorumVerify(message []byte, signatures protocol.MultiSignature) error {
	var weights protocol.VotePower
	verifies := make([]func() error, 0, len(signatures))
	votes := make(map[uint32]bool, len(signatures))
	signatures.Range(func(signature protocol.Signature) {
		verifies = append(verifies, func() error { return m.Verify(message, signature) })
		weights += m.nodes[id2index(signature.Signer)].VotePower
		votes[signature.Signer] = true
	})
	if len(votes) < len(signatures) {
		return errors.New("duplicate signer")
	}
	if weights < m.quorumPower {
		return fmt.Errorf("insufficient voting power %d for quorum %d", weights, m.quorumPower)
	}
	return concurrency.Parallel(verifies...)
}
