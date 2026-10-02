package executionrequests_test

import (
	"bytes"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/executionrequests"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// The body's SSZ records must equal the execution client's drain output, so
// the Engine groups hash to the payload's requests commitment.
func TestProtoGroupsMatchExecutionRecords(t *testing.T) {
	exit := executionrequests.Exit{ValidatorIndex: 0x0102030405060708}
	for i := range exit.SourceAddress {
		exit.SourceAddress[i] = byte(i + 1)
	}
	exit.ValidatorPubkeyRoot[31] = 0xaa
	request := &qrysmpb.ExecutionExitRequest{
		SourceAddress:       exit.SourceAddress[:],
		ValidatorIndex:      exit.ValidatorIndex,
		ValidatorPubkeyRoot: exit.ValidatorPubkeyRoot[:],
	}
	groups, err := executionrequests.Groups([]*qrysmpb.ExecutionExitRequest{request, request})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{executionrequests.ExitRequestType}, append(exit.Encode(), exit.Encode()...)...)
	if len(groups) != 1 || !bytes.Equal(groups[0], want) {
		t.Fatalf("group %x, want %x", groups, want)
	}
	decoded, err := executionrequests.FromGroups(groups)
	if err != nil || len(decoded) != 2 || !proto.Equal(decoded[0], request) {
		t.Fatalf("round trip %v, %v", decoded, err)
	}
	if empty, err := executionrequests.Groups(nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty groups %v, %v", empty, err)
	}
	for _, bad := range [][][]byte{{{executionrequests.ExitRequestType}}, {{2, 0}}, {want, want}, {append(want, append(exit.Encode(), 0)...)}} {
		if _, err := executionrequests.FromGroups(bad); err == nil {
			t.Fatalf("accepted malformed groups %x", bad)
		}
	}
}
