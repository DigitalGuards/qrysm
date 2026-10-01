package executionrequests_test

import (
	"bytes"
	"encoding/hex"
	"math"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/executionrequests"
)

// These constants are frozen research vectors, independently checked by the
// client's existing SSZ implementation here. They are synthetic public data.
const frozenKeyRoot = "d808c9d029dac6b8c80e02de706124b1183e60dfeed7899bc6d3a66bbff37986"
const frozenRecord = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f0807060504030201d808c9d029dac6b8c80e02de706124b1183e60dfeed7899bc6d3a66bbff37986"

func TestExecutionExitFrozenVector(t *testing.T) {
	publicKey := make([]byte, 2592)
	for i := range publicKey {
		publicKey[i] = byte(i % 251)
	}
	root, err := executionrequests.PublicKeyRoot(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(root[:]) != frozenKeyRoot {
		t.Fatalf("public-key SSZ root %x, want %s", root, frozenKeyRoot)
	}
	record, err := hex.DecodeString(frozenRecord)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := executionrequests.DecodeExits(record, 1)
	if err != nil {
		t.Fatal(err)
	}
	request := requests[0]
	if request.ValidatorIndex != 0x0102030405060708 || request.ValidatorPubkeyRoot != root {
		t.Fatalf("decoded request differs from frozen vector: %+v", request)
	}
	for i, b := range request.SourceAddress {
		if b != byte(i) {
			t.Fatalf("source byte %d = %x", i, b)
		}
	}
	if !bytes.Equal(request.Encode(), record) {
		t.Fatal("encoded record differs from frozen vector")
	}
	// Input storage must not remain aliased to decoded requests.
	record[0] ^= 0xff
	if request.SourceAddress[0] != 0 {
		t.Fatal("decoded source aliases input")
	}
}

func TestDecodeExecutionExitsBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int
		limit  uint64
		valid  bool
	}{
		{"empty", 0, 2, true},
		{"one", 104, 2, true},
		{"at limit", 208, 2, true},
		{"over limit", 312, 2, false},
		{"truncated", 103, 2, false},
		{"valid prefix malformed suffix", 209, 2, false},
		{"zero limit", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests, err := executionrequests.DecodeExits(make([]byte, tc.length), tc.limit)
			if (err == nil) != tc.valid {
				t.Fatalf("DecodeExits returned %v, valid=%v", err, tc.valid)
			}
			if err != nil && requests != nil {
				t.Fatal("malformed container returned a partial request list")
			}
		})
	}
	request := executionrequests.Exit{ValidatorIndex: math.MaxUint64}
	decoded, err := executionrequests.DecodeExits(request.Encode(), 1)
	if err != nil || decoded[0] != request {
		t.Fatalf("uint64 maximum index did not round trip: %v", err)
	}
}

func TestExecutionExitPublicKeyLength(t *testing.T) {
	for _, size := range []int{0, 48, 2591, 2593} {
		if _, err := executionrequests.PublicKeyRoot(make([]byte, size)); err == nil {
			t.Fatalf("accepted public key of length %d", size)
		}
	}
}
