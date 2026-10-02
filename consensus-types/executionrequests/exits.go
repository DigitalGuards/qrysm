// Package executionrequests contains the exploratory full-exit request codec.
// It adapts the EIP-7002 withdrawal request (https://eips.ethereum.org/EIPS/eip-7002)
// to full exits identified by validator index and ML-DSA public-key root, carried
// as an EIP-7685 typed request (https://eips.ethereum.org/EIPS/eip-7685). Its
// format and bounds require a coordinated fork before consensus use.
package executionrequests

import (
	"encoding/binary"
	"fmt"

	ssz "github.com/prysmaticlabs/fastssz"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// ExitRecordSize is the candidate fixed SSZ record width. The outer request
// type byte belongs to the versioned transport and is excluded from this size.
const ExitRecordSize = fieldparams.WithdrawalRecipientLength + 8 + fieldparams.RootLength

// Exit binds the execution-authenticated caller to a canonical validator index
// and the SSZ root of its ML-DSA public key. It always requests a full exit.
type Exit struct {
	SourceAddress       [fieldparams.WithdrawalRecipientLength]byte
	ValidatorIndex      primitives.ValidatorIndex
	ValidatorPubkeyRoot [fieldparams.RootLength]byte
}

// DecodeExits validates the complete container before returning any requests.
// maxRequests must be the active fork's limit when this prototype is integrated.
// Empty containers are valid; empty typed groups must be rejected by transport.
func DecodeExits(records []byte, maxRequests uint64) ([]Exit, error) {
	if maxRequests == 0 {
		return nil, fmt.Errorf("execution exit request limit must be positive")
	}
	if len(records)%ExitRecordSize != 0 {
		return nil, fmt.Errorf("execution exit records length %d is not a multiple of %d", len(records), ExitRecordSize)
	}
	count := len(records) / ExitRecordSize
	if uint64(count) > maxRequests {
		return nil, fmt.Errorf("execution exit request count %d exceeds limit %d", count, maxRequests)
	}
	requests := make([]Exit, count)
	for i := range requests {
		record := records[i*ExitRecordSize : (i+1)*ExitRecordSize]
		copy(requests[i].SourceAddress[:], record[:fieldparams.WithdrawalRecipientLength])
		indexStart := fieldparams.WithdrawalRecipientLength
		requests[i].ValidatorIndex = primitives.ValidatorIndex(binary.LittleEndian.Uint64(record[indexStart : indexStart+8]))
		copy(requests[i].ValidatorPubkeyRoot[:], record[indexStart+8:])
	}
	return requests, nil
}

// Encode encodes the candidate fixed record, including every source byte.
func (r Exit) Encode() []byte {
	record := make([]byte, ExitRecordSize)
	copy(record, r.SourceAddress[:])
	indexStart := fieldparams.WithdrawalRecipientLength
	binary.LittleEndian.PutUint64(record[indexStart:indexStart+8], uint64(r.ValidatorIndex))
	copy(record[indexStart+8:], r.ValidatorPubkeyRoot[:])
	return record
}

// PublicKeyRoot uses the same fixed-byte-vector operation as generated
// Validator.HashTreeRootWith. It rejects lengths that could otherwise be padded
// or truncated by the read-only validator convenience accessor.
func PublicKeyRoot(publicKey []byte) ([fieldparams.RootLength]byte, error) {
	if len(publicKey) != fieldparams.MLDSA87PubkeyLength {
		return [fieldparams.RootLength]byte{}, fmt.Errorf("validator public key length %d, want %d", len(publicKey), fieldparams.MLDSA87PubkeyLength)
	}
	hasher := ssz.NewHasher()
	hasher.PutBytes(publicKey)
	return hasher.HashRoot()
}
