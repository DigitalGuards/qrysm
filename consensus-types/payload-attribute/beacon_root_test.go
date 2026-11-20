package payloadattribute

import (
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"testing"
)

func TestBeaconRootAttributesCopyAndBounds(t *testing.T) {
	attrs, err := New(&enginev1.PayloadAttributesV2{Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 31, 33} {
		if _, err := WithParentBeaconBlockRoot(attrs, make([]byte, size)); err == nil {
			t.Fatalf("accepted root length %d", size)
		}
	}
	root := make([]byte, 32)
	root[0], root[31] = 0x80, 0x11
	wrapped, err := WithParentBeaconBlockRoot(attrs, root)
	if err != nil {
		t.Fatal(err)
	}
	root[0] = 0
	if got := wrapped.(BeaconRootAttributer).ParentBeaconBlockRoot(); got[0] != 0x80 || got[31] != 0x11 {
		t.Fatal("root aliases source bytes")
	}
}
