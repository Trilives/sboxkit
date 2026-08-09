package converter

import "encoding/json"

// MarshalJSON keeps native sing-box passthrough truly lossless. The typed
// fields remain populated for callers/tests, while Raw retains top-level and
// nested sections unknown to sboxkit (services, endpoints, NTP, custom route
// fields, and future sing-box additions).
func (c Config) MarshalJSON() ([]byte, error) {
	if c.Raw != nil {
		return json.Marshal(c.Raw)
	}
	type plain Config
	return json.Marshal(plain(c))
}
