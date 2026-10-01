package blocks

import (
	"bytes"
	"context"
	"fmt"

	"github.com/theQRL/qrysm/beacon-chain/core/validators"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/executionrequests"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/time/slots"
)

// ProcessExecutionExitRequests is an exploratory handler, intentionally uncalled
// by the canonical block transition. Integration requires fork-gated beacon
// containers and versioned Engine transport that authenticate these exact bytes
// against execution-derived requests. A future transition must invoke this after
// conventional operations so all exit paths share the resulting native churn.
//
// records excludes the outer request type byte. maxRequests must come from the
// coordinated fork configuration, never a proposer-supplied field. Structurally
// valid but ineligible requests are consumed with no change. Structural errors
// and state/scheduling errors reject the batch. Changes are made on a state copy;
// callers must adopt the returned state only on success.
func ProcessExecutionExitRequests(ctx context.Context, beaconState state.BeaconState, records []byte, maxRequests uint64) (state.BeaconState, error) {
	requests, err := executionrequests.DecodeExits(records, maxRequests)
	if err != nil {
		return nil, err
	}
	if len(requests) == 0 {
		return beaconState, nil
	}
	if beaconState == nil || beaconState.IsNil() {
		return nil, fmt.Errorf("nil beacon state for execution exit requests")
	}
	working := beaconState.Copy()
	currentEpoch := slots.ToEpoch(working.Slot())
	cfg := params.BeaconConfig()
	var maxExitEpoch primitives.Epoch
	var churn uint64
	err = working.ReadFromEveryValidator(func(_ int, validator state.ReadOnlyValidator) error {
		epoch := validator.ExitEpoch()
		if epoch == cfg.FarFutureEpoch {
			return nil
		}
		if epoch > maxExitEpoch {
			maxExitEpoch, churn = epoch, 1
		} else if epoch == maxExitEpoch {
			churn++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read execution exit queue: %w", err)
	}
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if uint64(request.ValidatorIndex) >= uint64(working.NumValidators()) {
			continue
		}
		validator, err := working.ValidatorAtIndex(request.ValidatorIndex)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(request.SourceAddress[:], validator.WithdrawalRecipient) {
			continue
		}
		keyRoot, err := executionrequests.PublicKeyRoot(validator.PublicKey)
		if err != nil {
			return nil, err
		}
		if request.ValidatorPubkeyRoot != keyRoot {
			continue
		}
		if validator.ExitEpoch != cfg.FarFutureEpoch || validator.ActivationEpoch > currentEpoch || currentEpoch >= validator.ExitEpoch {
			continue
		}
		// Subtraction avoids wrapping activation_epoch + minimum_active_period.
		if currentEpoch-validator.ActivationEpoch < cfg.ShardCommitteePeriod {
			continue
		}
		// Native scheduling computes this sum without overflow checks. Validate
		// it before delegating; its churn increment and withdrawal sum are checked.
		activationExitEpoch, err := currentEpoch.SafeAdd(1)
		if err == nil {
			_, err = activationExitEpoch.SafeAddEpoch(cfg.MaxSeedLookahead)
		}
		if err != nil {
			return nil, fmt.Errorf("execution exit activation epoch: %w", err)
		}
		var exitEpoch primitives.Epoch
		working, exitEpoch, err = validators.InitiateValidatorExit(ctx, working, request.ValidatorIndex, maxExitEpoch, churn)
		if err != nil {
			return nil, fmt.Errorf("schedule execution exit %d: %w", request.ValidatorIndex, err)
		}
		if exitEpoch >= cfg.FarFutureEpoch {
			return nil, fmt.Errorf("scheduled execution exit reaches far-future sentinel")
		}
		if exitEpoch > maxExitEpoch {
			maxExitEpoch, churn = exitEpoch, 1
		} else if exitEpoch == maxExitEpoch {
			churn++
		}
	}
	return working, nil
}
