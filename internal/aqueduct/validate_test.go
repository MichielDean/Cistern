package aqueduct

import (
	"strings"
	"testing"
)

func validConfig() *AqueductConfig {
	return &AqueductConfig{
		Aqueducts: []Workflow{
			{Name: "default", Cataractae: []WorkflowCataractae{
				{Name: "implement", Type: CataractaeTypeAgent, OnPass: "done"},
			}},
		},
		Repos: []RepoConfig{
			{Name: "myrepo", Aqueduct: "default", Cataractae: 1, Prefix: "mr"},
		},
	}
}

func TestValidateAqueductConfig_ForkMode_MissingUpstreamRemote(t *testing.T) {
	cfg := validConfig()
	cfg.Repos[0].DeliveryMode = DeliveryModeFork
	// UpstreamRemote is empty — this must fail.

	err := ValidateAqueductConfig(cfg)
	if err == nil {
		t.Fatal("expected error for fork mode without upstream_remote, got nil")
	}
	if !strings.Contains(err.Error(), "upstream_remote is required") {
		t.Errorf("error = %q, want it to contain 'upstream_remote is required'", err)
	}
}

func TestValidateAqueductConfig_ForkMode_ValidUpstreamRemote(t *testing.T) {
	cfg := validConfig()
	cfg.Repos[0].DeliveryMode = DeliveryModeFork
	cfg.Repos[0].UpstreamRemote = "https://github.com/upstream-org/repo.git"

	if err := ValidateAqueductConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	// Validation should normalize the DeliveryMode on the original struct.
	if cfg.Repos[0].DeliveryMode != DeliveryModeFork {
		t.Errorf("DeliveryMode after validation = %q, want %q", cfg.Repos[0].DeliveryMode, DeliveryModeFork)
	}
}

func TestValidateAqueductConfig_DirectMode_UpstreamRemoteNotRequired(t *testing.T) {
	cfg := validConfig()
	cfg.Repos[0].DeliveryMode = DeliveryModeDirect
	// UpstreamRemote is empty — should pass for direct mode.

	if err := ValidateAqueductConfig(cfg); err != nil {
		t.Fatalf("expected valid config for direct mode without upstream_remote, got: %v", err)
	}
}

func TestValidateAqueductConfig_EmptyDeliveryMode_DefaultsToDirect(t *testing.T) {
	cfg := validConfig()
	// DeliveryMode is empty (zero value).

	if err := ValidateAqueductConfig(cfg); err != nil {
		t.Fatalf("expected valid config for empty delivery mode, got: %v", err)
	}
	if cfg.Repos[0].DeliveryMode != DeliveryModeDirect {
		t.Errorf("DeliveryMode after validation = %q, want %q", cfg.Repos[0].DeliveryMode, DeliveryModeDirect)
	}
}

func TestValidateAqueductConfig_DirectMode_UpstreamRemoteIgnored(t *testing.T) {
	cfg := validConfig()
	cfg.Repos[0].DeliveryMode = DeliveryModeDirect
	cfg.Repos[0].UpstreamRemote = "https://github.com/upstream-org/repo.git"
	// Direct mode with UpstreamRemote set — should not fail (upstream ignored).

	if err := ValidateAqueductConfig(cfg); err != nil {
		t.Fatalf("expected valid config for direct mode with upstream_remote set, got: %v", err)
	}
}