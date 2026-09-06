package plugin

import (
	"context"
	"encoding/json"
	"time"

	"golang.zabbix.com/sdk/errs"
	"golang.zabbix.com/sdk/metric"
	"golang.zabbix.com/sdk/plugin"
	"golang.zabbix.com/sdk/plugin/container"
	"golang.zabbix.com/sdk/zbxerr"
)

const (
	// Name is the plugin name used in Plugins.OPCUA.* configuration keys
	// and passed to plugin.RegisterMetrics().
	Name = "OPCUA"

	keyGet = "opcua.get"
)

var (
	_ plugin.Configurator = (*Plugin)(nil)
	_ plugin.Exporter     = (*Plugin)(nil)
	_ plugin.Runner       = (*Plugin)(nil)
)

// Plugin implements the OPC UA loadable plugin for Zabbix agent 2.
//
// Unlike the built-in MQTT plugin, this only ever implements Exporter,
// Runner and Configurator: loadable plugins do not support Watcher, so
// there is no way to have the agent push subscription updates to the
// plugin the way MQTT does with broker messages. Every opcua.get call is a
// synchronous OPC UA Read service call, driven by the item's own polling
// interval in Zabbix - the plugin never subscribes to anything on the
// server.
type Plugin struct {
	plugin.Base

	options options
	conns   *connPool
}

// New creates a new Plugin instance and registers its metrics.
func New() (*Plugin, error) {
	p := &Plugin{conns: newConnPool()}

	if err := p.registerMetrics(); err != nil {
		return nil, errs.Wrap(err, "plugin failed to register metrics")
	}

	return p, nil
}

// Run creates the plugin handler and blocks until the agent terminates the
// plugin. Called from main(); not part of any SDK interface.
func (p *Plugin) Run() error {
	h, err := container.NewHandler(Name)
	if err != nil {
		return errs.Wrap(err, "failed to create new handler")
	}

	p.Logger = h

	if err := h.Execute(); err != nil {
		return errs.Wrap(err, "failed to execute plugin handler")
	}

	return nil
}

// Start implements plugin.Runner. Sessions are opened lazily on first use
// (see connPool.get / opcuaClient.connect), so there is nothing to warm up
// here - this only exists to satisfy the interface and leave a hook for
// later.
func (p *Plugin) Start() {
	p.Infof("started")
}

// Stop implements plugin.Runner. Closes every pooled OPC UA session so the
// plugin doesn't leak sessions on the server when the agent deactivates it.
func (p *Plugin) Stop() {
	p.conns.closeAll(context.Background())
	p.Infof("stopped")
}

// Export implements plugin.Exporter.
func (p *Plugin) Export(key string, rawParams []string, _ plugin.ContextProvider) (any, error) {
	if key != keyGet {
		return nil, errs.Wrapf(zbxerr.ErrorUnsupportedMetric, "unknown metric %q", key)
	}

	params, extraParams, hardcodedParams, err := metricGet.EvalParams(rawParams, p.options.Sessions)
	if err != nil {
		return nil, errs.Wrap(err, "failed to evaluate parameters")
	}

	err = metric.SetDefaults(params, hardcodedParams, p.options.Default)
	if err != nil {
		return nil, errs.Wrap(err, "failed to set default parameters")
	}

	if len(extraParams) == 0 {
		return nil, errs.New("at least one NodeID parameter is required")
	}

	if params[paramEndpointName] == "" {
		return nil, errs.New("Endpoint is required")
	}

	b := broker{
		endpoint:       params[paramEndpointName],
		securityMode:   params[paramSecurityModeName],
		securityPolicy: params[paramSecurityPolicyName],
		certFile:       params[paramCertFileName],
		keyFile:        params[paramKeyFileName],
		user:           params[paramUserName],
		password:       params[paramPasswordName],
	}

	oc := p.conns.get(b)

	timeout := time.Duration(p.options.Timeout) * time.Second

	// context.Background() rather than a context derived from the
	// ContextProvider argument: per the Zabbix loadable-plugin guidelines,
	// loadable plugins get the Exporter interface "except the
	// ContextProvider parameter" - it isn't fully wired up outside a
	// built-in plugin, so a self-managed timeout is used instead.
	values, err := oc.read(context.Background(), timeout, extraParams)
	if err != nil {
		// Warningf, not Debugf/Infof: this should be visible at the
		// agent's normal logging level, not only with debug logging
		// turned up - a read failure is something worth seeing by
		// default.
		p.Warningf("opcua.get against %s failed: %s", b.endpoint, err.Error())

		return nil, errs.Wrap(err, "failed to read node value(s)")
	}

	p.Debugf("opcua.get read %d node(s) from %s", len(extraParams), b.endpoint)

	if len(extraParams) == 1 {
		return values[extraParams[0]], nil
	}

	j, err := json.Marshal(values)
	if err != nil {
		return nil, errs.Wrap(err, "failed to marshal result")
	}

	return string(j), nil
}

//nolint:gochecknoglobals // registered once and read-only after init; mirrors the built-in MQTT plugin's package-level metrics var.
var metricGet = metric.New(
	"Reads one or more OPC UA node values. Returns the raw value for a "+
		"single NodeID, or a JSON object of NodeID to value for more than one.",
	metricParams,
	true,
)

func (p *Plugin) registerMetrics() error {
	metricSet := metric.MetricSet{keyGet: metricGet}

	if err := plugin.RegisterMetrics(p, Name, metricSet.List()...); err != nil {
		return errs.Wrap(err, "failed to register metrics")
	}

	return nil
}
