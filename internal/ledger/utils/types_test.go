package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/axiomesh/axiom-kit/types"
)

func TestCompositeKey(t *testing.T) {
	assert.Equal(t, CompositeKey(BlockHashKey, "0x112233"), []byte("block-hash-0x112233"))
	addr := types.NewAddressByStr("0x5f9f18f7c3a6e5e4c0b877fe3e688ab08840b997")
	assert.True(t, len(CompositeStorageKey(addr, []byte("1das2345fsdfdssd"))) > 0)
	assert.True(t, len(CompositeAccountKey(addr)) > 0)
	assert.True(t, len(CompositeCodeKey(addr, []byte("1das2345fsdfdssd"))) > 0)

	fmt.Printf("CompositeStorageKey(addr, []byte(\"1das2345fsdfdssd\"))=%v\n", CompositeStorageKey(addr, []byte("1das2345fsdfdssd")))
	fmt.Printf("CompositeAccountKey(addr)=%v\n", CompositeAccountKey(addr))

}
