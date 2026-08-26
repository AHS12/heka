package osapp

// StartupRegistrar manages OS-level startup registration (SPEC-15 §3).
// Each platform provides its own implementation; tests inject fakes.
type StartupRegistrar interface {
	Enable(exePath string) error
	Disable() error
	Enabled() (bool, error)
}

// NewStartupRegistrar returns the platform's startup registrar.
var NewStartupRegistrar = func() StartupRegistrar {
	return newStartupRegistrar()
}
