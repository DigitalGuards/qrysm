package blockchain

import (
	"context"
	"testing"
	stdtime "time"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	engineMock "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/util"
)

type beaconRootCaptureEngine struct {
	engineMock.EngineClient
	parent *common.Hash
}

func (e *beaconRootCaptureEngine) NewPayload(_ context.Context, _ interfaces.ExecutionData, _ []common.Hash, parent *common.Hash) ([]byte, error) {
	if parent != nil {
		value := *parent
		e.parent = &value
	}
	return nil, nil
}

func TestBeaconRootImportUsesEnclosingParent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	activation := uint64(0)
	params.BeaconConfig().ExperimentalBeaconRootTime = &activation
	service, tr := minimalTestService(t)
	engine := &beaconRootCaptureEngine{}
	service.cfg.ExecutionEngineCaller = engine
	protoBlock := util.NewBeaconBlockZond()
	root := common.Hash{0: 0x85, 31: 0x16}
	protoBlock.Block.ParentRoot = root[:]
	protoBlock.Block.Body.ExecutionPayload.ParentHash[0] = 1
	protoBlock.Block.Body.ExecutionPayload.Timestamp = 60
	block, err := consensusblocks.NewSignedBeaconBlock(protoBlock)
	if err != nil {
		t.Fatal(err)
	}
	preHeader, err := consensusblocks.WrappedExecutionPayloadHeaderZond(&enginev1.ExecutionPayloadHeaderZond{BlockHash: make([]byte, 32)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := service.notifyNewPayload(tr.ctx, preHeader, block)
	if err != nil || !valid {
		t.Fatalf("import: valid=%v err=%v", valid, err)
	}
	if engine.parent == nil || *engine.parent != root {
		t.Fatalf("enclosing parent lost: %v", engine.parent)
	}
}

func TestBeaconRootProactiveAttributesUseSelectedHead(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	activation := uint64(100)
	params.BeaconConfig().ExperimentalBeaconRootTime = &activation
	service, tr := minimalTestService(t, WithProposerIdsCache(cache.NewProposerPayloadIDsCache()))
	service.genesisTime = stdtime.Unix(100, 0)
	st, _ := util.DeterministicGenesisStateZond(t, 1)
	service.cfg.ProposerSlotIndexCache.SetProposerAndPayloadIDs(0, 0, [8]byte{}, [32]byte{})
	root := [32]byte{0: 0x91, 31: 0x82}
	ok, attrs, _ := service.getPayloadAttribute(tr.ctx, st, 0, root[:])
	if !ok {
		t.Fatal("attributes were not prepared")
	}
	bound, ok := attrs.(payloadattribute.BeaconRootAttributer)
	if !ok || bound.ParentBeaconBlockRoot() != root {
		t.Fatal("selected head root lost")
	}
	if ok, _, _ := service.getPayloadAttribute(tr.ctx, st, 0, root[:31]); ok {
		t.Fatal("accepted truncated selected head")
	}
}
