package params

import "testing"

func TestExperimentalBeaconRootConfigRoundTrip(t *testing.T) {
	cfg := MainnetConfig().Copy()
	if cfg.ExperimentalBeaconRootsEnabled(^uint64(0)) {
		t.Fatal("default config enabled experimental roots")
	}
	activation := uint64(17)
	cfg.ExperimentalBeaconRootTime = &activation
	if cfg.ExperimentalBeaconRootsEnabled(16) || !cfg.ExperimentalBeaconRootsEnabled(17) {
		t.Fatal("incorrect activation boundary")
	}
	copy := cfg.Copy()
	*copy.ExperimentalBeaconRootTime = 9
	if *cfg.ExperimentalBeaconRootTime != 17 {
		t.Fatal("config copy aliases activation pointer")
	}
	roundTrip, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig())
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.ExperimentalBeaconRootTime == nil || *roundTrip.ExperimentalBeaconRootTime != 17 {
		t.Fatal("activation lost in YAML")
	}
}
