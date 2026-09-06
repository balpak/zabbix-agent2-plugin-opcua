package plugin

import "golang.zabbix.com/sdk/metric"

const (
	paramEndpointName       = "Endpoint"
	paramSecurityModeName   = "SecurityMode"
	paramSecurityPolicyName = "SecurityPolicy"
	paramCertFileName       = "CertFile"
	paramKeyFileName        = "KeyFile"
	paramUserName           = "User"
	paramPasswordName       = "Password"
)

// metricParams follows the built-in MQTT plugin's ordering convention:
// the key-fillable NewConnParam entries first (these occupy positions in
// the opcua.get[...] key, in this order), then the NewSessionOnlyParam
// entries (CertFile, KeyFile) last - those can only come from a named
// session or the Default block, never typed directly into a key, the same
// way MQTT's TLSCAFile/TLSCertFile/TLSKeyFile work.
//
// IMPORTANT: only the first param in this list may call .WithSession() -
// that's what marks a param's value as eligible to be a session name
// instead of a literal value, and the SDK panics at init time
// ("session must be placed first") if more than one param has it, or if
// it's not on the first one. The built-in MQTT plugin only puts it on its
// first param (URL); every other ConnParam, including User and Password,
// is a plain metric.NewConnParam with no .WithSession() call. An earlier
// version of this file put .WithSession() on five different params
// (Endpoint, SecurityMode, SecurityPolicy, User, Password) - that was
// wrong and is what caused the panic.
//
//nolint:gochecknoglobals // global constants, mirrors the pattern used in the built-in MQTT plugin and example plugin.
var (
	metricParams = []*metric.Param{
		paramEndpoint, paramSecurityMode, paramSecurityPolicy, paramUser, paramPassword,
		paramCertFile, paramKeyFile,
	}

	// paramEndpoint is mandatory and has no default, the same way the
	// MQTT plugin's Topic parameter has no default - there's no
	// meaningful universal endpoint to fall back to. It's the only param
	// here that calls .WithSession() - see the note above.
	paramEndpoint = metric.NewConnParam(
		paramEndpointName,
		"OPC UA server endpoint URL, e.g. opc.tcp://10.0.0.5:4840. May also be a session name, "+
			"or a semicolon-separated list of endpoint URLs (e.g. a redundant server pair) tried "+
			"in order on every connect.",
	).WithSession()

	paramSecurityMode = metric.NewConnParam(
		paramSecurityModeName, "Secure channel security mode: None, Sign or SignAndEncrypt.",
	).WithDefault("None")

	// paramSecurityPolicy takes the short policy name gopcua expects
	// (e.g. Basic256Sha256), not the full spec URI - see README.md.
	paramSecurityPolicy = metric.NewConnParam(
		paramSecurityPolicyName, "Secure channel security policy, e.g. Basic256Sha256. Use None to disable.",
	).WithDefault("None")

	paramCertFile = metric.NewSessionOnlyParam(
		paramCertFileName, "Path to the client certificate file. Required if SecurityPolicy is not None.",
	).WithDefault("")

	paramKeyFile = metric.NewSessionOnlyParam(
		paramKeyFileName, "Path to the client private key file. Required if SecurityPolicy is not None.",
	).WithDefault("")

	paramUser = metric.NewConnParam(
		paramUserName, "Username for OPC UA session authentication. Leave empty for anonymous authentication.",
	).WithDefault("")

	paramPassword = metric.NewConnParam(
		paramPasswordName, "Password for OPC UA session authentication.",
	).WithDefault("")
)
