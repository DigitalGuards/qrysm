package blocks_test

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/validators"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/executionrequests"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func executionExitState(t *testing.T, count int) state.BeaconState {
	t.Helper()
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	cfg := params.BeaconConfig()
	registry := make([]*qrysmpb.Validator, count)
	balances := make([]uint64, count)
	for i := range registry {
		publicKey := make([]byte, fieldparams.MLDSA87PubkeyLength)
		publicKey[0] = byte(i + 1)
		recipient := make([]byte, fieldparams.WithdrawalRecipientLength)
		recipient[0], recipient[len(recipient)-1] = 0x45, byte(i+1)
		registry[i] = &qrysmpb.Validator{
			PublicKey:           publicKey,
			WithdrawalRecipient: recipient,
			EffectiveBalance:    cfg.MaxEffectiveBalance,
			ActivationEpoch:     0,
			ExitEpoch:           cfg.FarFutureEpoch,
			WithdrawableEpoch:   cfg.FarFutureEpoch,
		}
		balances[i] = cfg.MaxEffectiveBalance
	}
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot:        primitives.Slot(cfg.ShardCommitteePeriod) * cfg.SlotsPerEpoch,
		Validators:  registry,
		Balances:    balances,
		RandaoMixes: make([][]byte, cfg.EpochsPerHistoricalVector),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func executionExitRequest(t *testing.T, s state.BeaconState, index primitives.ValidatorIndex) executionrequests.Exit {
	t.Helper()
	v, err := s.ValidatorAtIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	root, err := executionrequests.PublicKeyRoot(v.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	request := executionrequests.Exit{ValidatorIndex: index, ValidatorPubkeyRoot: root}
	copy(request.SourceAddress[:], v.WithdrawalRecipient)
	return request
}

func TestExecutionExitRequestsShareNativeChurn(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.MinPerEpochChurnLimit = 2
	s := executionExitState(t, 5)
	ctx := context.Background()
	// Conventional operations have already scheduled one native exit. This
	// models the required placement after the signed-exit and slashing paths.
	var err error
	s, firstEpoch, err := validators.InitiateValidatorExit(ctx, s, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := s.Validators()
	var records []byte
	for _, index := range []primitives.ValidatorIndex{1, 1, 2, 3} {
		records = append(records, executionExitRequest(t, s, index).Encode()...)
	}
	result, err := blocks.ProcessExecutionExitRequests(ctx, s, records, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []primitives.Epoch{firstEpoch, firstEpoch, firstEpoch + 1, firstEpoch + 1, cfg.FarFutureEpoch}
	for i, v := range result.Validators() {
		if v.ExitEpoch != want[i] {
			t.Fatalf("validator %d exit epoch %d, want %d", i, v.ExitEpoch, want[i])
		}
		if i < 4 && v.WithdrawableEpoch != v.ExitEpoch+cfg.MinValidatorWithdrawabilityDelay {
			t.Fatalf("validator %d withdrawal delay changed", i)
		}
		if !reflect.DeepEqual(v.WithdrawalRecipient, before[i].WithdrawalRecipient) || v.EffectiveBalance != before[i].EffectiveBalance {
			t.Fatalf("validator %d recipient or effective balance changed", i)
		}
	}
	if !reflect.DeepEqual(s.Validators(), before) || !reflect.DeepEqual(s.Balances(), result.Balances()) {
		t.Fatal("input state or balances changed")
	}
	// A second batch sees the same shared queue and consumes duplicates.
	next := append(executionExitRequest(t, result, 1).Encode(), executionExitRequest(t, result, 4).Encode()...)
	result, err = blocks.ProcessExecutionExitRequests(ctx, result, next, 2)
	if err != nil {
		t.Fatal(err)
	}
	v, err := result.ValidatorAtIndex(4)
	if err != nil || v.ExitEpoch != firstEpoch+2 {
		t.Fatalf("second batch failed to preserve churn: %v, validator=%+v", err, v)
	}
}

func TestExecutionExitRequestsConsumeIneligible(t *testing.T) {
	for _, scenario := range []string{"unknown index", "source first byte", "source last byte", "wrong key root", "not activated", "minimum period", "already exiting", "activation period overflow"} {
		t.Run(scenario, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			s := executionExitState(t, 2)
			request := executionExitRequest(t, s, 0)
			v, err := s.ValidatorAtIndex(0)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "unknown index":
				request.ValidatorIndex = math.MaxUint64
			case "source first byte":
				request.SourceAddress[0] ^= 0xff
			case "source last byte":
				request.SourceAddress[63] ^= 0xff
			case "wrong key root":
				request.ValidatorPubkeyRoot[31] ^= 0xff
			case "not activated":
				v.ActivationEpoch = params.BeaconConfig().ShardCommitteePeriod + 1
			case "minimum period":
				v.ActivationEpoch = 1
			case "already exiting":
				v.ExitEpoch = 100
			case "activation period overflow":
				v.ActivationEpoch = 1
				params.BeaconConfig().ShardCommitteePeriod = math.MaxUint64
			}
			if err := s.UpdateValidatorAtIndex(0, v); err != nil {
				t.Fatal(err)
			}
			before := s.Validators()
			result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, request.Encode(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, result.Validators()) {
				t.Fatal("ineligible request changed validators")
			}
		})
	}
}

func TestExecutionExitRequestsRejectMalformedAtomically(t *testing.T) {
	s := executionExitState(t, 2)
	record := executionExitRequest(t, s, 0).Encode()
	before := s.Validators()
	for _, malformed := range [][]byte{record[:103], append(append([]byte{}, record...), 1), append(append([]byte{}, record...), record...)} {
		result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, malformed, 1)
		if err == nil || result != nil {
			t.Fatalf("accepted malformed or excessive records: %v", err)
		}
		if !reflect.DeepEqual(before, s.Validators()) {
			t.Fatal("malformed batch changed input state")
		}
	}
}

func TestExecutionExitRequestsIneligiblePrefixDoesNotBlockEligibleRequest(t *testing.T) {
	s := executionExitState(t, 1)
	valid := executionExitRequest(t, s, 0)
	unknown := valid
	unknown.ValidatorIndex = math.MaxUint64
	wrongSource := valid
	wrongSource.SourceAddress[0] ^= 0xff
	wrongKey := valid
	wrongKey.ValidatorPubkeyRoot[0] ^= 0xff
	var records []byte
	for _, request := range []executionrequests.Exit{unknown, wrongSource, wrongKey, valid} {
		records = append(records, request.Encode()...)
	}
	result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, records, 4)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := result.ValidatorAtIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	want := helpers.ActivationExitEpoch(params.BeaconConfig().ShardCommitteePeriod)
	if validator.ExitEpoch != want {
		t.Fatalf("eligible request after ineligible prefix has exit epoch %d, want %d", validator.ExitEpoch, want)
	}
}

func TestExecutionExitRequestsRejectSchedulingOverflowAtomically(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.MinPerEpochChurnLimit = 1
	s := executionExitState(t, 3)
	v, err := s.ValidatorAtIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	// First request fits at max-16. Its successor would overflow withdrawability.
	v.ExitEpoch = primitives.Epoch(math.MaxUint64) - cfg.MinValidatorWithdrawabilityDelay - 1
	v.WithdrawableEpoch = primitives.Epoch(math.MaxUint64) - 1
	if err := s.UpdateValidatorAtIndex(0, v); err != nil {
		t.Fatal(err)
	}
	before := s.Validators()
	records := append(executionExitRequest(t, s, 1).Encode(), executionExitRequest(t, s, 2).Encode()...)
	result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, records, 2)
	if err == nil || result != nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("expected withdrawal overflow, got result=%v err=%v", result, err)
	}
	if !reflect.DeepEqual(before, s.Validators()) {
		t.Fatal("late overflow leaked an earlier exit")
	}
}

func TestExecutionExitRequestsRejectActivationOverflow(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	s := executionExitState(t, 1)
	params.BeaconConfig().MaxSeedLookahead = math.MaxUint64
	record := executionExitRequest(t, s, 0).Encode()
	result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, record, 1)
	if err == nil || result != nil || !strings.Contains(err.Error(), "activation epoch") {
		t.Fatalf("expected activation overflow, got result=%v err=%v", result, err)
	}
}

func TestExecutionExitRequestsRejectMalformedCanonicalKey(t *testing.T) {
	s := executionExitState(t, 1)
	record := executionExitRequest(t, s, 0).Encode()
	v, err := s.ValidatorAtIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	v.PublicKey = v.PublicKey[:len(v.PublicKey)-1]
	if err := s.UpdateValidatorAtIndex(0, v); err != nil {
		t.Fatal(err)
	}
	result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, record, 1)
	if err == nil || result != nil || !strings.Contains(err.Error(), "public key length") {
		t.Fatalf("expected canonical key rejection, got result=%v err=%v", result, err)
	}
}

func TestExecutionExitRequestsEmptyAndCancelled(t *testing.T) {
	s := executionExitState(t, 1)
	result, err := blocks.ProcessExecutionExitRequests(context.Background(), s, nil, 1)
	if err != nil || result != s {
		t.Fatalf("empty batch returned %v, %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = blocks.ProcessExecutionExitRequests(ctx, s, executionExitRequest(t, s, 0).Encode(), 1)
	if err != context.Canceled || result != nil {
		t.Fatalf("cancelled batch returned %v, %v", result, err)
	}
}
