package etcd

import "time"

// Etcdtool configuration struct.
type Etcdtool struct {
	Peers            string        `json:"peers,omitempty" yaml:"peers,omitempty" toml:"peers,omitempty"`
	Cert             string        `json:"cert,omitempty" yaml:"cert,omitempty" toml:"cert,omitempty"`
	Key              string        `json:"key,omitempty" yaml:"key,omitempty" toml:"key,omitempty"`
	CA               string        `json:"ca,omitempty" yaml:"ca,omitempty" toml:"peers,omitempty"`
	User             string        `json:"user,omitempty" yaml:"user,omitempty" toml:"user,omitempty"`
	Timeout          time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty" toml:"timeout,omitempty"`
	CommandTimeout   time.Duration `json:"commandTimeout,omitempty" yaml:"commandTimeout,omitempty" toml:"commandTimeout,omitempty"`
	Routes           []Route       `json:"routes" yaml:"routes" toml:"routes"`
	PasswordFilePath string        `json:"-,omitempty" yaml:",omitempty" toml:",omitempty"`
	Root             string        `json:"root,omitempty" yaml:"root,omitempty" toml:"root,omitempty"`
}

// Route configuration struct.
type Route struct {
	Regexp string `json:"regexp" yaml:"regexp" toml:"regexp"`
	Schema string `json:"schema" yaml:"schema" toml:"schema"`
}
