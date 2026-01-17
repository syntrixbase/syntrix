package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	services "github.com/syntrixbase/syntrix/internal/services/config"
)

// mockServiceConfig implements ServiceConfig for testing ApplyServiceConfigs
type mockServiceConfig struct {
	defaultsApplied   bool
	envOverridesApply bool
	pathsResolved     bool
	validated         bool
	baseDir           string
	validateErr       error
	validatedMode     services.DeploymentMode
}

func (m *mockServiceConfig) ApplyDefaults() {
	m.defaultsApplied = true
}

func (m *mockServiceConfig) ApplyEnvOverrides() {
	m.envOverridesApply = true
}

func (m *mockServiceConfig) ResolvePaths(baseDir string) {
	m.pathsResolved = true
	m.baseDir = baseDir
}

func (m *mockServiceConfig) Validate(mode services.DeploymentMode) error {
	m.validated = true
	m.validatedMode = mode
	return m.validateErr
}

func TestApplyServiceConfigs_AllMethodsCalled(t *testing.T) {
	cfg1 := &mockServiceConfig{}
	cfg2 := &mockServiceConfig{}

	err := ApplyServiceConfigs("config", services.ModeDistributed, cfg1, cfg2)

	assert.NoError(t, err)
	assert.True(t, cfg1.defaultsApplied)
	assert.True(t, cfg1.envOverridesApply)
	assert.True(t, cfg1.pathsResolved)
	assert.True(t, cfg1.validated)
	assert.Equal(t, "config", cfg1.baseDir)
	assert.Equal(t, services.ModeDistributed, cfg1.validatedMode)

	assert.True(t, cfg2.defaultsApplied)
	assert.True(t, cfg2.envOverridesApply)
	assert.True(t, cfg2.pathsResolved)
	assert.True(t, cfg2.validated)
	assert.Equal(t, "config", cfg2.baseDir)
	assert.Equal(t, services.ModeDistributed, cfg2.validatedMode)
}

func TestApplyServiceConfigs_ValidationError(t *testing.T) {
	cfg1 := &mockServiceConfig{}
	cfg2 := &mockServiceConfig{validateErr: assert.AnError}

	err := ApplyServiceConfigs("config", services.ModeStandalone, cfg1, cfg2)

	assert.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestApplyServiceConfigs_EmptyList(t *testing.T) {
	err := ApplyServiceConfigs("config", services.ModeDistributed)
	assert.NoError(t, err)
}
