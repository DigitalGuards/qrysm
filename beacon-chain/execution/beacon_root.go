package execution

import (
	"errors"
	"fmt"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/go-qrl/rpc"
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

// experimentalEngineError adds a configuration hint only when the execution
// client rejected the experimental method itself: it lacks the method
// (-32601) or its activation time disagrees with this node's (-38005). Other
// errors, such as an unknown payload after an execution-client restart, keep
// their own message so they point at the real cause.
func experimentalEngineError(method string, err error) error {
	converted := handleRPCError(err)
	if method != NewPayloadWithBeaconRootMethodV1 && method != ForkchoiceUpdatedWithBeaconRootMethodV1 && method != GetPayloadWithBeaconRootMethodV1 {
		return converted
	}
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) && (rpcErr.ErrorCode() == -32601 || rpcErr.ErrorCode() == -38005) {
		return fmt.Errorf("experimental beacon-root Engine method %s failed; both clients must enable compatible transport: %w", method, converted)
	}
	return converted
}
