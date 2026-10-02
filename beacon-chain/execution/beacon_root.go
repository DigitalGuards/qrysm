package execution

import (
	"fmt"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	pb "github.com/theQRL/qrysm/proto/engine/v1"
)

// This JSON-only transport leaves protobuf and SSZ payload layouts unchanged.
type beaconRootPayloadAttributes struct {
	Timestamp             hexutil.Uint64   `json:"timestamp"`
	PrevRandao            hexutil.Bytes    `json:"prevRandao"`
	SuggestedFeeRecipient hexutil.BytesQ   `json:"suggestedFeeRecipient"`
	Withdrawals           []*pb.Withdrawal `json:"withdrawals"`
	ParentBeaconBlockRoot common.Hash      `json:"parentBeaconBlockRoot"`
}

func experimentalEngineError(method string, err error) error {
	converted := handleRPCError(err)
	if method == NewPayloadWithBeaconRootMethodV1 || method == ForkchoiceUpdatedWithBeaconRootMethodV1 || method == GetPayloadWithBeaconRootMethodV1 {
		return fmt.Errorf("experimental beacon-root Engine method %s failed; both clients must enable compatible transport: %w", method, converted)
	}
	return converted
}
