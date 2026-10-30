package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/axiomesh/axiom-kit/types"
)

// stripProtoField returns the message with one field removed.
//
// It mirrors the helper in cmd/chainfix. The wire format is protobuf, so
// the field number is the key shifted right by three and the low three bits
// of the key hold the wire type.
func stripProtoField(msg []byte, target uint64) []byte {
	out := make([]byte, 0, len(msg))
	i := 0
	for i < len(msg) {
		start := i
		key, n := decodeVarint(msg[i:])
		if n == 0 {
			return msg
		}
		i += n
		field, wire := key>>3, key&0x7
		switch wire {
		case 0:
			_, n := decodeVarint(msg[i:])
			if n == 0 {
				return msg
			}
			i += n
		case 2:
			l, n := decodeVarint(msg[i:])
			if n == 0 {
				return msg
			}
			i += n
			if i+int(l) > len(msg) {
				return msg
			}
			i += int(l)
		case 5:
			if i+4 > len(msg) {
				return msg
			}
			i += 4
		case 1:
			if i+8 > len(msg) {
				return msg
			}
			i += 8
		default:
			return msg
		}
		if field != target {
			out = append(out, msg[start:i]...)
		}
	}
	return out
}

func decodeVarint(b []byte) (uint64, int) {
	var v uint64
	var shift uint
	for i := 0; i < len(b); i++ {
		if i > 9 {
			return 0, 0
		}
		v |= uint64(b[i]&0x7f) << shift
		if b[i] < 0x80 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, 0
}

// TestBlockHashExcludesBloom pins down how the block hash is computed.
//
// axiom-kit's BlockHeader.CalculateHash rebuilds a fresh BlockHeader that
// copies every field except Bloom, and then hashes the marshalled result.
// The blockfile stores the original header, with the bloom still in it, so
// hashing the stored bytes directly does not reproduce the block hash.
//
// The recovery tool cmd/chainfix has to recompute block hashes from the
// blockfile to be able to rewrite the chain meta, so it strips the bloom
// field before hashing. This test makes sure that stays true: if axiom-kit
// ever starts including the bloom in the hash, this test fails and the
// recovery tool has to be updated.
func TestBlockHashExcludesBloom(t *testing.T) {
	var bloom types.Bloom
	for i := range bloom {
		bloom[i] = byte(i * 7)
	}

	h := &types.BlockHeader{
		Number:          18566580,
		GasPrice:        1000000000,
		Epoch:           300,
		Bloom:           &bloom,
		ProposerAccount: "0xC1064AD00be66834e79Ca11382e01BE71f30eE95",
		GasUsed:         133078,
		Extra:           []byte{1, 2, 3},
	}

	blob, err := h.Marshal()
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	// The bloom is protobuf field 9.
	const bloomField = 9

	stripped := stripProtoField(blob, bloomField)
	if len(stripped) == len(blob) {
		t.Fatal("stripping the bloom field did not remove anything")
	}

	got := sha256.Sum256(stripped)
	want := h.Hash()
	if want.String() != "0x"+hex.EncodeToString(got[:]) {
		t.Fatalf("block hash mismatch:\n  sha256(stripped marshal) = 0x%s\n  Header.Hash()          = %s",
			hex.EncodeToString(got[:]), want.String())
	}

	// Hashing the stored bytes as they are must NOT match, otherwise the
	// test would not be proving anything.
	if plain := sha256.Sum256(blob); plain == got {
		t.Fatal("the bloom field was not present in the marshalled header")
	}
}

// TestChainMetaEncoding documents the on-disk layout of the chain meta.
//
// The recovery tool rewrites the chain meta by hand, so it has to produce
// exactly the same bytes that types.ChainMeta.Marshal produces.
func TestChainMetaEncoding(t *testing.T) {
	m := &types.ChainMeta{
		Height:    18566580,
		GasPrice:  bigOneGwei(),
		BlockHash: types.NewHash(make([]byte, 32)),
	}

	blob, err := m.Marshal()
	if err != nil {
		t.Fatalf("marshal chain meta: %v", err)
	}

	// field 1: height, varint
	if blob[0] != 0x08 {
		t.Fatalf("expected field 1 (height) first, got tag 0x%02x", blob[0])
	}
	height, n := decodeVarint(blob[1:])
	if n == 0 {
		t.Fatal("cannot decode the height")
	}
	if height != 18566580 {
		t.Fatalf("height = %d, want 18566580", height)
	}

	// The gas price is stored as the big-endian bytes of the big.Int, not
	// as a varint. 1 gwei is 0x3b9aca00.
	want := []byte{0x1a, 0x04, 0x3b, 0x9a, 0xca, 0x00}
	if !containsBytes(blob, want) {
		t.Fatalf("the gas price is not encoded as big.Int.Bytes():\n  got  %x\n  want the sequence %x", blob, want)
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func bigOneGwei() *big.Int {
	return big.NewInt(1000000000)
}
