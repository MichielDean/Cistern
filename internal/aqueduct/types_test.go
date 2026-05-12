package aqueduct

import (
	"testing"
)

func TestDeliveryModeConstants(t *testing.T) {
	if DeliveryModeDirect != "direct" {
		t.Errorf("DeliveryModeDirect = %q, want %q", DeliveryModeDirect, "direct")
	}
	if DeliveryModeFork != "fork" {
		t.Errorf("DeliveryModeFork = %q, want %q", DeliveryModeFork, "fork")
	}
}

func TestDeliveryModeZeroValue_IsDirect(t *testing.T) {
	var dm DeliveryMode
	if dm != "" {
		t.Errorf("zero value DeliveryMode = %q, want empty string", dm)
	}
	// The zero value should be treated as "direct" by validation, not here.
	// This test just confirms the zero value is the empty string.
}

func TestRepoConfig_ForkMode_YAML(t *testing.T) {
	cfg := RepoConfig{
		Name:           "external-repo",
		URL:            "https://github.com/contributor/fork.git",
		Cataractae:     2,
		Names:          []string{"alpha", "beta"},
		Prefix:         "ext",
		Aqueduct:       "feature",
		DeliveryMode:   DeliveryModeFork,
		UpstreamRemote: "https://github.com/upstream-org/repo.git",
	}
	if cfg.DeliveryMode != DeliveryModeFork {
		t.Errorf("DeliveryMode = %q, want %q", cfg.DeliveryMode, DeliveryModeFork)
	}
	if cfg.UpstreamRemote != "https://github.com/upstream-org/repo.git" {
		t.Errorf("UpstreamRemote = %q, want upstream URL", cfg.UpstreamRemote)
	}
}

func TestRepoConfig_DirectMode_ZeroValues(t *testing.T) {
	cfg := RepoConfig{
		Name:       "internal-repo",
		URL:        "https://github.com/org/repo.git",
		Cataractae: 1,
		Prefix:     "ir",
		Aqueduct:   "feature",
	}
	// Zero values — DeliveryMode is "" (defaults to "direct" via validation).
	if cfg.DeliveryMode != "" {
		t.Errorf("default DeliveryMode = %q, want empty string", cfg.DeliveryMode)
	}
	if cfg.UpstreamRemote != "" {
		t.Errorf("default UpstreamRemote = %q, want empty string", cfg.UpstreamRemote)
	}
}