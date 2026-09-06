package plugin

import (
	"golang.zabbix.com/sdk/conf"
	"golang.zabbix.com/sdk/errs"
	"golang.zabbix.com/sdk/plugin"
)

// session mirrors the connection-scoped metric parameters, so the same
// values can come from a metric key, a named session, or the Default
// block - matching the pattern used by the MQTT and example plugins.
type session struct {
	Endpoint       string `conf:"name=Endpoint,optional"`
	SecurityMode   string `conf:"name=SecurityMode,optional"`
	SecurityPolicy string `conf:"name=SecurityPolicy,optional"`
	CertFile       string `conf:"name=CertFile,optional"`
	KeyFile        string `conf:"name=KeyFile,optional"`
	User           string `conf:"name=User,optional"`
	Password       string `conf:"name=Password,optional"`
}

type options struct {
	System plugin.SystemOptions `conf:"optional"` //nolint:staticcheck // matches the field the example plugin uses for Plugins.OPCUA.System.Path.

	Timeout int `conf:"optional,range=1:30"`

	// Sessions stores pre-defined named sets of connection settings.
	Sessions map[string]session `conf:"optional"`

	// Default stores default connection parameter values from the
	// configuration file.
	Default session `conf:"optional"`
}

// Configure implements the plugin.Configurator interface.
func (p *Plugin) Configure(global *plugin.GlobalOptions, rawOptions any) {
	var o options

	err := conf.UnmarshalStrict(rawOptions, &o)
	if err != nil {
		p.Errf("cannot unmarshal configuration options: %s", err.Error())

		return
	}

	p.options = o

	if p.options.Timeout == 0 {
		p.options.Timeout = global.Timeout
	}
}

// Validate implements the plugin.Configurator interface.
func (*Plugin) Validate(rawOptions any) error {
	var o options

	err := conf.UnmarshalStrict(rawOptions, &o)
	if err != nil {
		return errs.Wrap(err, "failed to unmarshal configuration options")
	}

	for name, s := range o.Sessions {
		if err := validateSessionSecurity(s); err != nil {
			return errs.Wrapf(err, "session %s", name)
		}
	}

	if err := validateSessionSecurity(o.Default); err != nil {
		return errs.Wrap(err, "default session")
	}

	return nil
}

// validateSessionSecurity checks that a client certificate/key pair is
// present whenever a non-None SecurityPolicy is configured. It cannot
// check that the pair is actually valid for the policy chosen, or that the
// server will accept it - that only shows up at connect time.
func validateSessionSecurity(s session) error {
	if s.SecurityPolicy == "" || s.SecurityPolicy == "None" {
		return nil
	}

	if s.CertFile == "" || s.KeyFile == "" {
		return errs.New("CertFile and KeyFile are required when SecurityPolicy is not None")
	}

	return nil
}
