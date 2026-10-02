package validator

import (
	"context"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"testing"

	chainMock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	engineMock "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	pb "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/util"
)

type beaconRootProposalEngine struct {
	engineMock.EngineClient
	attributes  payloadattribute.Attributer
	requestedID [8]byte
}

func (e *beaconRootProposalEngine) ForkchoiceUpdated(_ context.Context, _ *pb.ForkchoiceState, attributes payloadattribute.Attributer) (*pb.PayloadIDBytes, []byte, error) {
	e.attributes = attributes
	return &pb.PayloadIDBytes{2}, nil, nil
}

func (e *beaconRootProposalEngine) GetPayload(ctx context.Context, id [8]byte, timestamp uint64) (interfaces.ExecutionData, bool, error) {
	e.requestedID = id
	return e.EngineClient.GetPayload(ctx, id, timestamp)
}

func (e *beaconRootProposalEngine) GetPayloadWithRequests(ctx context.Context, id [8]byte, timestamp uint64) (interfaces.ExecutionData, bool, []*qrysmpb.ExecutionExitRequest, error) {
	payload, overrideBuilder, err := e.GetPayload(ctx, id, timestamp)
	return payload, overrideBuilder, nil, err
}

func TestBeaconRootProposalParentAndCacheSeparation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	activation := uint64(0)
	params.BeaconConfig().ExperimentalBeaconRootTime = &activation
	s, _ := util.DeterministicGenesisStateZond(t, 1)
	header, err := blocks.WrappedExecutionPayloadHeaderZond(&pb.ExecutionPayloadHeaderZond{BlockNumber: 1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLatestExecutionPayloadHeader(header); err != nil {
		t.Fatal(err)
	}
	engine := &beaconRootProposalEngine{EngineClient: engineMock.EngineClient{ExecutionPayloadZond: &pb.ExecutionPayloadZond{}}}
	vs := &Server{
		ExecutionEngineCaller:  engine,
		HeadFetcher:            &chainMock.ChainService{State: s},
		FinalizationFetcher:    &chainMock.ChainService{},
		ForkchoiceFetcher:      &chainMock.ChainService{},
		BeaconDB:               dbTest.SetupDB(t),
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}
	oldRoot, newRoot := [32]byte{1}, [32]byte{2}
	vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(s.Slot(), 0, [8]byte{1}, oldRoot)
	protoBlock := util.NewBeaconBlockZond()
	protoBlock.Block.Slot = s.Slot()
	protoBlock.Block.ParentRoot = newRoot[:]
	block, err := blocks.NewSignedBeaconBlock(protoBlock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := vs.getLocalPayload(context.Background(), block.Block(), s); err != nil {
		t.Fatal(err)
	}
	bound, ok := engine.attributes.(payloadattribute.BeaconRootAttributer)
	if !ok || bound.ParentBeaconBlockRoot() != newRoot || engine.requestedID != [8]byte{2} {
		t.Fatal("proposal reused another beacon parent's payload")
	}
	engine.attributes = nil
	vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(s.Slot(), 0, [8]byte{3}, newRoot)
	if _, _, _, err := vs.getLocalPayload(context.Background(), block.Block(), s); err != nil {
		t.Fatal(err)
	}
	if engine.attributes != nil || engine.requestedID != [8]byte{3} {
		t.Fatal("matching beacon-parent cache entry was not used")
	}
}
