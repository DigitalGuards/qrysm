package payloadattribute

import "fmt"

// BeaconRootAttributer carries the selected beacon parent outside the unchanged
// Zond execution-payload SSZ, like the EIP-4788 parentBeaconBlockRoot payload
// attribute. Engine transport uses this only after activation.
type BeaconRootAttributer interface {
	Attributer
	ParentBeaconBlockRoot() [32]byte
}

type beaconRootAttributes struct {
	Attributer
	root [32]byte
}

// WithParentBeaconBlockRoot binds attributes to the selected proposal parent.
// Exact width is required so malformed input cannot be padded or truncated.
func WithParentBeaconBlockRoot(attributes Attributer, root []byte) (Attributer, error) {
	if attributes == nil || len(root) != 32 {
		return nil, fmt.Errorf("parent beacon root requires attributes and exactly 32 bytes")
	}
	result := &beaconRootAttributes{Attributer: attributes}
	copy(result.root[:], root)
	return result, nil
}

func (a *beaconRootAttributes) ParentBeaconBlockRoot() [32]byte { return a.root }
