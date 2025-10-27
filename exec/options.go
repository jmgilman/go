package exec

// config holds the configuration for command execution.
// It distinguishes between global settings (set at creation time) and local settings (set per-execution).
type config struct {
	// Global settings (set at creation time)
	globalEnv        map[string]string
	globalDir        string
	globalInheritEnv bool
	globalDisableColors bool
	globalPassthrough bool

	// Local settings (set per-execution, override global)
	localEnv        map[string]string
	localDir        string
	localInheritEnv *bool
	localDisableColors *bool
	localPassthrough *bool
}

// newConfig creates a new configuration with default values.
func newConfig() *config {
	return &config{
		globalEnv: make(map[string]string),
		localEnv:  make(map[string]string),
	}
}

// clone creates a deep copy of the configuration.
func (c *config) clone() *config {
	clone := &config{
		globalEnv:          make(map[string]string),
		globalDir:          c.globalDir,
		globalInheritEnv:   c.globalInheritEnv,
		globalDisableColors: c.globalDisableColors,
		globalPassthrough:  c.globalPassthrough,
		localEnv:           make(map[string]string),
		localDir:           c.localDir,
	}

	for k, v := range c.globalEnv {
		clone.globalEnv[k] = v
	}

	for k, v := range c.localEnv {
		clone.localEnv[k] = v
	}

	if c.localInheritEnv != nil {
		val := *c.localInheritEnv
		clone.localInheritEnv = &val
	}

	if c.localDisableColors != nil {
		val := *c.localDisableColors
		clone.localDisableColors = &val
	}

	if c.localPassthrough != nil {
		val := *c.localPassthrough
		clone.localPassthrough = &val
	}

	return clone
}

// effectiveEnv returns the effective environment variables, merging global and local settings.
// Local settings override global settings.
func (c *config) effectiveEnv() map[string]string {
	env := make(map[string]string)

	// Start with global environment
	for k, v := range c.globalEnv {
		env[k] = v
	}

	// Override with local environment
	for k, v := range c.localEnv {
		env[k] = v
	}

	// Apply disable colors if enabled
	if c.effectiveDisableColors() {
		env["NO_COLOR"] = "1"
		env["TERM"] = "dumb"
		env["CLICOLOR"] = "0"
		env["CLICOLOR_FORCE"] = "0"
		env["FORCE_COLOR"] = "0"
	}

	return env
}

// effectiveDir returns the effective working directory.
// Local setting overrides global setting.
func (c *config) effectiveDir() string {
	if c.localDir != "" {
		return c.localDir
	}
	return c.globalDir
}

// effectiveInheritEnv returns whether to inherit environment variables.
// Local setting overrides global setting.
func (c *config) effectiveInheritEnv() bool {
	if c.localInheritEnv != nil {
		return *c.localInheritEnv
	}
	return c.globalInheritEnv
}

// effectiveDisableColors returns whether to disable colors.
// Local setting overrides global setting.
func (c *config) effectiveDisableColors() bool {
	if c.localDisableColors != nil {
		return *c.localDisableColors
	}
	return c.globalDisableColors
}

// effectivePassthrough returns whether to enable passthrough.
// Local setting overrides global setting.
func (c *config) effectivePassthrough() bool {
	if c.localPassthrough != nil {
		return *c.localPassthrough
	}
	return c.globalPassthrough
}

// resetLocal resets all local settings.
// This should be called after each Run() to ensure local settings don't carry over.
func (c *config) resetLocal() {
	c.localEnv = make(map[string]string)
	c.localDir = ""
	c.localInheritEnv = nil
	c.localDisableColors = nil
	c.localPassthrough = nil
}
