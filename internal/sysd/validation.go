package sysd

import (
	"fmt"
	"regexp"
)

var (
	serviceNamePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)
	interfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)
)

func validateServiceName(name string) error {
	if !serviceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid systemd service name %q", name)
	}
	return nil
}

// Runtime config, bootstrap relay, watchdog state, and watchdog units are
// intentionally process-wide singletons. Reject alternate main service names
// at mutating entry points instead of allowing two instances to overwrite or
// contend for the same files and loopback relay port.
func validateManagedServiceName(name string) error {
	if err := validateServiceName(name); err != nil {
		return err
	}
	if name != DefaultName {
		return fmt.Errorf("unsupported managed service %q: sboxkit manages only %q", name, DefaultName)
	}
	return nil
}

func validateInterfaceName(name string) error {
	if !interfaceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid network interface name %q", name)
	}
	return nil
}
