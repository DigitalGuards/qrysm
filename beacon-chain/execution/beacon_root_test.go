package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/rpc"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	pb "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
)

type beaconRootRPCRequest struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func beaconRootRPC(t *testing.T, unsupported bool) (*Service, <-chan beaconRootRPCRequest) {
	t.Helper()
	requests := make(chan beaconRootRPCRequest, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request beaconRootRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if unsupported {
			response["error"] = map[string]any{"code": -32601, "message": "method not found"}
		} else {
			switch request.Method {
			case NewPayloadMethodV2, NewPayloadWithBeaconRootMethodV1:
				response["result"] = fixtures()["ValidPayloadStatus"]
			case ForkchoiceUpdatedMethodV2, ForkchoiceUpdatedWithBeaconRootMethodV1:
				response["result"] = fixtures()["ForkchoiceUpdatedResponse"]
			default:
				response["result"] = fixtures()["ExecutionPayloadZondWithValue"]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := rpc.DialContext(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return &Service{rpcClient: client, chainStartData: &qrysmpb.ChainStartData{GenesisTime: 100}}, requests
}

func TestBeaconRootEngineActivationAndProvenance(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	activation := uint64(160)
	params.BeaconConfig().ExperimentalBeaconRootTime = &activation
	s, requests := beaconRootRPC(t, false)
	ctx := context.Background()
	root := common.Hash{0: 0x81, 31: 0x92}
	for _, timestamp := range []uint64{159, 160} {
		payload := fixtures()["ExecutionPayloadZond"].(*pb.ExecutionPayloadZond)
		payload.Timestamp = timestamp
		wrapped, err := blocks.WrappedExecutionPayloadZond(payload, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.NewPayload(ctx, wrapped, nil, &root); err != nil {
			t.Fatal(err)
		}
		request := <-requests
		want := NewPayloadMethodV2
		if timestamp == activation {
			want = NewPayloadWithBeaconRootMethodV1
		}
		if request.Method != want {
			t.Fatalf("method %s, want %s", request.Method, want)
		}
		if timestamp == activation {
			var got common.Hash
			if len(request.Params) != 2 {
				t.Fatal("missing parent root argument")
			}
			if err := json.Unmarshal(request.Params[1], &got); err != nil || got != root {
				t.Fatalf("wrong root %x: %v", got, err)
			}
			if _, err := s.NewPayload(ctx, wrapped, nil, nil); err == nil {
				t.Fatal("accepted missing parent root")
			}
		} else if len(request.Params) != 1 {
			t.Fatal("pre-activation argument changed")
		}
	}
	for _, timestamp := range []uint64{159, 160} {
		attrs, err := payloadattribute.New(&pb.PayloadAttributesV2{Timestamp: timestamp, PrevRandao: make([]byte, 32), SuggestedFeeRecipient: make([]byte, 64)})
		if err != nil {
			t.Fatal(err)
		}
		if timestamp == activation {
			if _, _, err := s.ForkchoiceUpdated(ctx, &pb.ForkchoiceState{}, attrs); err == nil {
				t.Fatal("accepted active attrs without beacon root")
			}
			attrs, err = payloadattribute.WithParentBeaconBlockRoot(attrs, root[:])
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := s.ForkchoiceUpdated(ctx, &pb.ForkchoiceState{}, attrs); err != nil {
			t.Fatal(err)
		}
		request := <-requests
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(request.Params[1], &wire); err != nil {
			t.Fatal(err)
		}
		if timestamp == activation {
			var got common.Hash
			if err := json.Unmarshal(wire["parentBeaconBlockRoot"], &got); err != nil || got != root {
				t.Fatalf("attrs root %x: %v", got, err)
			}
			if request.Method != ForkchoiceUpdatedWithBeaconRootMethodV1 || string(wire["withdrawals"]) != "[]" {
				t.Fatal("noncanonical root attributes")
			}
		} else if _, exists := wire["parentBeaconBlockRoot"]; exists || request.Method != ForkchoiceUpdatedMethodV2 {
			t.Fatal("pre-activation attrs changed")
		}
	}
	if _, _, err := s.ForkchoiceUpdated(ctx, &pb.ForkchoiceState{}, payloadattribute.EmptyWithVersion(version.Zond)); err != nil {
		t.Fatal(err)
	}
	if request := <-requests; request.Method != ForkchoiceUpdatedMethodV2 || string(request.Params[1]) != "null" {
		t.Fatal("head-only update changed")
	}
}

func TestBeaconRootGetPayloadActivationAndMissingCapability(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	activation := uint64(100) + params.BeaconConfig().SecondsPerSlot
	params.BeaconConfig().ExperimentalBeaconRootTime = &activation
	s, requests := beaconRootRPC(t, false)
	for _, slot := range []primitives.Slot{0, 1} {
		if _, overrideBuilder, err := s.GetPayload(context.Background(), [8]byte{1}, slot); err != nil {
			t.Fatal(err)
		} else if overrideBuilder != (slot == 1) {
			t.Fatal("builder override differs from root activation")
		}
		want := GetPayloadMethodV2
		if slot == 1 {
			want = GetPayloadWithBeaconRootMethodV1
		}
		if request := <-requests; request.Method != want {
			t.Fatalf("got %s, want %s", request.Method, want)
		}
	}
	missing, _ := beaconRootRPC(t, true)
	_, _, err := missing.GetPayload(context.Background(), [8]byte{1}, 1)
	if err == nil || !strings.Contains(err.Error(), "both clients must enable compatible transport") || !strings.Contains(err.Error(), GetPayloadWithBeaconRootMethodV1) {
		t.Fatalf("unclear unsupported transport error: %v", err)
	}
	s.chainStartData = nil
	if _, _, err := s.GetPayload(context.Background(), [8]byte{1}, 1); err == nil {
		t.Fatal("accepted uninitialized genesis time")
	}
}
