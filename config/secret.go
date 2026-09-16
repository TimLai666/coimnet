package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// secretPrefix is the only scheme a secret reference may name. A configuration
// points at an environment variable; it never points at a file, a URL or a
// literal, so there is exactly one place a credential can come from and the
// document can be published unchanged.
const secretPrefix = "env:"

// SecretRef is a reference to the environment variable that holds a credential.
// The value it points at is never read, stored or printed by this package: Load
// records the reference, Marshal writes the reference back, and resolving it is
// the job of whoever actually makes the call.
type SecretRef struct {
	Env string
}

// MarshalJSON writes the one form a secret ever takes on the wire.
func (s SecretRef) MarshalJSON() ([]byte, error) {
	if s.Env == "" {
		return nil, fmt.Errorf("config: a secret reference must name an environment variable")
	}
	return json.Marshal(struct {
		Ref string `json:"ref"`
	}{secretPrefix + s.Env})
}

// UnmarshalJSON accepts {"ref": "env:NAME"} and nothing else. Its messages
// never repeat the input, because a rejected document may well contain the
// literal credential the rule exists to keep out.
func (s *SecretRef) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf(`config: a secret must be declared as {"ref": "env:NAME"}, never as a literal value`)
	}
	var raw struct {
		Ref *string `json:"ref"`
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf(`config: a secret must be declared as {"ref": "env:NAME"}`)
	}
	if raw.Ref == nil {
		return fmt.Errorf(`config: a secret must be declared as {"ref": "env:NAME"}`)
	}
	name := strings.TrimPrefix(*raw.Ref, secretPrefix)
	if !strings.HasPrefix(*raw.Ref, secretPrefix) || name == "" {
		return fmt.Errorf(`config: a secret reference must be "env:NAME", naming the environment variable that holds the value`)
	}
	s.Env = name
	return nil
}
