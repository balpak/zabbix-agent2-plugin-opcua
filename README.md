# Zabbix agent2 OPC UA Loadable Plugin

A Go-based **loadable plugin for Zabbix agent2** that collects monitoring data from OPC UA servers using the [`gopcua/opcua`](https://github.com/gopcua/opcua) client library.

> **Project status:** development / validation. Review the **Known limitations** and **Verified functionality** sections before production use.

## Overview

Loadable plugins have been released with the launch of Zabbix 6.0 enabling monitoring of OT devices using industrial protocols. The most common ones such as Modbus and MQTT are available out-of-the-box, however ZBXNEXT-7113 created in Y2021 is only on the [9.0 LTS roadmap](https://www.zabbix.com/roadmap#v9_0_LTS) for the moment.


### Loadable plugins versus built-in plugins

Zabbix agent2 plugin is a small Go component responsible for metrics collection via one or more item keys. Zabbix ships a set of built-in ones (MQTT, Modbus, MySQL, Docker and so on) compiled directly into the`zabbix_agent2` binary.

There's no built-in OPC UA plugin and adding one would mean forking and recompiling agent2's own source tree. This is not something worth doing for a single organization's monitoring need.

The alternative is a loadable plugin: a completely separate, standalone Go program that agent2 spawns as a sub-process and talks to over a local Unix socket. Original agent2's source code and compiled binary remains the same. You build custom plugin, point `Plugins.<Name>.System.Path` at the binary and agent2 handles the rest - starting it, feeding it item key requests and stopping it. This is what this OPC UA plugin is.

### What a loadable plugin can and can't do

Loadable plugins only support three of Zabbix's five plugin interfaces - **Exporter**, **Runner**, and **Configurator**. **Watcher** and **Collector** are not available to them regardless of how the plugin is written.

That matters here because the built-in MQTT plugin (the obvious reference point for "a protocol plugin that talks to a broker") is built entirely on Watcher. It opens one persistent connection to the broker and lets messages arrive asynchronously pushed to the plugin whenever the broker sends one. A loadable plugin cannot do that.

So this plugin is a poll and not a subscription. Every `opcua.get` call is a synchronous OPC UA Read service call triggered by agent2 asking for a value on whatever interval the item is configured with. The plugin never subscribes to anything on the OPC UA server - it connects (or reuses a connection), issues a Read, returns the value and waits for the next request.

What carries over from the MQTT plugin's design, despite the interface difference, is connection reuse: one pooled session per distinct set of connection parameters (endpoint, security mode/policy, credentials) reused across repeated polls rather than reconnecting on every call.

## Repository layout

```text
.
├── build
│   ├── zabbix-agent2-plugin-opcua          # Compiled plugin binary
│   └── zabbix-agent2-plugin-opcua.sha256   # Checksum
├── .gitignore                              # Git ignore rules
├── go.mod                                  # Active Go module/dependency definition
├── go.sum                                  # Go dependency checksums
├── LICENSE                                 # AGPLv3 license text
├── main.go                                 # Plugin executable entry point
├── opcua.conf                              # Zabbix agent2 plugin configuration example
├── plugin
│   ├── client.go                           # OPC UA client/connection and collection logic
│   ├── config.go                           # Plugin configuration handling
│   ├── params.go                           # Item/plugin parameter definitions and parsing
│   └── plugin.go                           # Zabbix plugin implementation/registration
├── README.md                               # This file
├── runme.sh                                # Dependency refresh and build helper
└── template.go.mod                         # Baseline Go module file used by runme.sh
```

Above tree shows all files after successful compilation. Upon `git clone` the 'build' directory will be empty because plugin binary and checksum have been moved to [Releases](https://github.com/balpak/zabbix-agent2-plugin-opcua/releases) page.

## The metric key

```text
opcua.get[Endpoint,SecurityMode,SecurityPolicy,User,Password,<NodeID>...]
```

`Endpoint` is mandatory, no default. `SecurityMode` and `SecurityPolicy` default to `None`. `User`/`Password` default to empty (anonymous). One or more NodeIDs follow the OPC UA's string syntax (`ns=1;s=some.tag`). A single key returns the raw value for one NodeID or a JSON structure with each NodeID's value if more than one NodeID set in the key.

`CertFile` and `KeyFile` are not part of the key and can only be set via a named session or the `Default` block in `opcua.conf`. This mirrors the built-in MQTT plugin's `TLSCAFile`/`TLSCertFile`/`TLSKeyFile` which work the same way.

`Endpoint` also accepts a semicolon-separated list of endpoint URLs `opc.tcp://host1:4840/;opc.tcp://host2:4840/` for a redundant server pair. On every connect the plugin tries each in order and uses the first one that answers always starting from the top of the list, so a recovered primary is preferred again on the next reconnect without any separate failback step. All candidates in the list share the same `SecurityMode`/`SecurityPolicy`/`CertFile`/`KeyFile`/`User`/`Password`; there's no per-endpoint override.

## Requirements and tested environment

The project requires the following components to build and run:

- Zabbix agent2, version 6.0.0 or newer
- Go 1.25+ (see `template.go.mod` for the version this was written against)

Plugin version 0.1.0 has been tested on the development system with application versions listed below.

- Ubuntu Server 22.04.5 LTS, kernel 5.15.0-161-generic
- git 2.55.0
- go 1.25.09
- Zabbix SDK pinned by `runme.sh` to commit `f70f12fff03` (7.5, 2026-08-18)
- opcua v0.9.1 (latest as of 06 Sep 2026)
- Zabbix agent2 7.0.30

Then compiled binary was copied onto Zabbix Appliance machine (AlmaLinux 8.10) running server/agent2 version 7.4.14 and works without issues.

Finally, the same source code has been compiled with go 1.25.14 on Ubuntu Server 26.04.1 LTS, kernel 7.0.0-30-generic running server/agent2 version 8.0.0beta2 Revision 678399df737. No issues found as well.

## OPC UA interoperability tested

Data collection has been tested against industrial and demo OPC UA Servers versions listed below:

- Honeywell Experion R530.1
- ProSys 5.5.2-362
- UaExpert CPP 1.8.7.644
- Emerson TankMaster OPC UA Connector 2.0.0.3501

## Quick build

Clone the repository and run the supplied build helper:

```bash
$ git clone <YOUR-GITHUB-REPOSITORY-URL>
$ cd <REPOSITORY-DIRECTORY>
$ chmod +x runme.sh
$ ./runme.sh
```

For an end user building on the currently supported target the **`./runme.sh` is the intended build command**.

> [!WARNING]
> `go.mod` will be configured with the correct module name using provided `template.go.mod`. Do not re-run `go mod init` in this directory.

The script:

1. Removes the generated/current `go.mod`, `go.sum` and previous plugin binary/checksum if present.
2. Restores `go.mod` from `template.go.mod`.
3. Fetches the pinned Zabbix Go SDK revision.
4. Fetches the OPC UA Go dependency.
5. Runs `go mod tidy`.
6. Produces a statically linked Linux AMD64 executable named `zabbix-agent2-plugin-opcua` using `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.
7. Generates executable's SHA256 checksum and verifies it.

The example output shown below may vary depending on whether it is the first execution of the script or the recompilation of an existing binary file.

```bash
$ ./runme.sh
Building zabbix-agent2-plugin-opcua
Old 'go.mod' deleted
Old 'go.sum' deleted
Old zabbix-agent2-plugin-opcua binary deleted
Old zabbix-agent2-plugin-opcua checksum deleted
Original 'go.mod' restored from template

go: downloading golang.zabbix.com/sdk v1.2.2-0.20260818110545-f70f12fff031
go: added golang.zabbix.com/sdk v1.2.2-0.20260818110545-f70f12fff031

go: downloading github.com/gopcua/opcua v0.9.1
go: added github.com/gopcua/opcua v0.9.1

go: downloading github.com/stretchr/testify v1.10.0
go: downloading github.com/google/go-cmp v0.6.0
go: downloading github.com/Microsoft/go-winio v0.6.0
go: downloading github.com/davecgh/go-spew v1.1.1
go: downloading github.com/pmezard/go-difflib v1.0.0
go: downloading golang.org/x/sys v0.33.0
go: downloading golang.org/x/tools v0.33.0
go: downloading gopkg.in/yaml.v3 v3.0.1
go: downloading golang.org/x/sync v0.14.0
go: downloading golang.org/x/mod v0.24.0

build/zabbix-agent2-plugin-opcua: OK
```

Confirm binary file linked statically:

```bash
$ file build/zabbix-agent2-plugin-opcua
build/zabbix-agent2-plugin-opcua: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, BuildID[sha1]=9c64796f970b0360b538504cffa875871e6512e3, with debug_info, not stripped

$ go version -m build/zabbix-agent2-plugin-opcua
build/zabbix-agent2-plugin-opcua: go1.25.9
        path    golang.zabbix.com/plugin/opcua
        mod     golang.zabbix.com/plugin/opcua  (devel)
        dep     github.com/gopcua/opcua v0.9.1  h1:Qp40I5JmiiKXYIWmk7xECYNrXs5unohH24jKWnSRyIE=
        dep     golang.org/x/sys        v0.33.0 h1:q3i8TbbEz+JRD9ywIRlyRAQbM0qF7hu24q3teo2hbuw=
        dep     golang.zabbix.com/sdk   v1.2.2-0.20260818110545-f70f12fff031    h1:EiKix0Vakta/iydIZD6rAHFgTCgiYZ/jPOADVFkvGZQ=
        build   -buildmode=exe
        build   -compiler=gc
        build   CGO_ENABLED=0
        build   GOARCH=amd64
        build   GOOS=linux
        build   GOAMD64=v1
        build   vcs=git
        build   vcs.modified=true

```

## Zabbix agent2 configuration

Stop agent:

```bash
$ sudo systemctl stop zabbix-agent2.service
```

Copy binary to directory designated to store executable programs and scripts that you manually installed or compiled locally. According to [official documentation](https://www.zabbix.com/documentation/7.4/en/manual/installation/install#installing-zabbix-agent-2-loadable-plugins) *"The plugin executable may be placed anywhere as long as it is loadable by Zabbix agent 2"* so the actual location is your own choice.

```bash
$ sudo cp build/zabbix-agent2-plugin-opcua /usr/local/bin/
```

Copy the supplied configuration example to Zabbix plugins default directory:

```bash
sudo cp opcua.conf /etc/zabbix/zabbix_agent2.d/plugins.d/
```

Review `opcua.conf` plugin executable path. Leave endpoint, authentication/security and other settings commented out.

```ini
Plugins.OPCUA.System.Path=/usr/local/bin/zabbix-agent2-plugin-opcua
```

Default `/etc/zabbix/zabbix_agent2.conf` has plugin folder path already included.

```ini
Include=/etc/zabbix/zabbix_agent2.d/plugins.d/*.conf
```

Start agent and make sure no errors reported.

```bash
$ sudo systemctl start zabbix-agent2.service
$ sudo systemctl status zabbix-agent2.service
```

Follow [Debug logging](#debug-logging) and [Troubleshooting](#troubleshooting-checklist) if startup fails.


## Manual testing

Zabbix agent2 supports command-line testing of item keys. Unless you already know exact NodeID syntax for each item a free OPC UA Explorer application from UaExpert, ProSys or Matrikon can help you to find it out.

### Specify Endpoint explicitly

Below example polls 'sine wave' item from ProSys OPC UA Server running on Windows Server 2022 virtual machine with IP address 172.26.0.21 and built-in firewall temporary disabled to allow all incoming connections. 

```bash
$ sudo zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t 'opcua.get[opc.tcp://172.26.0.21:53530/OPCUA/SimulationServer,,,,,ns=3;i=1004]'
opcua.get[opc.tcp://172.26.0.21:53530/OPCUA/SimulationServer,,,,,ns=3;i=1004][s|1.732051]

$ sudo zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t 'opcua.get[opc.tcp://172.26.0.21:53530/OPCUA/SimulationServer,,,,,ns=3;i=1004]'
opcua.get[opc.tcp://172.26.0.21:53530/OPCUA/SimulationServer,,,,,ns=3;i=1004][s|1.902113]
```

> [!IMPORTANT]
> Running as 'root' is mandatory otherwise `zabbix_agent2` could not create socket under `/run/zabbix` and you'll get an error like below:

```bash
$ sudo journalctl -xeu zabbix-agent2.service

zabbix_agent2[1890439]: [OPCUA] failed to kill plugin /usr/local/bin/zabbix-agent2-plugin-opcua: Failed to kill plugin "/usr/local/bin/zabbix-agent2-plugin-opcua" process: os: process already finished.
zabbix_agent2[1890439]: zabbix_agent2 [1890439]: ERROR: Cannot register plugins: failed to register metrics of plugin "OPCUA": failed to start plugin: failed to create connection with plugin /usr/local/bin/zabbix-agent2-plugin-opcua: failed to get connection within the time limit 3000000000.
```

Request for multiple NodeIDs returns JSON structure:

```bash
$ sudo zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t 'opcua.get[opc.tcp://172.26.0.21:53530/OPCUA/SimulationServer,,,,,ns=3;i=1002,ns=3;i=1003,ns=3;i=1004]'
opcua.get[opc.tcp://172.26.0.21:53530/OPCUA/SimulationServer,,,,,ns=3;i=1002,ns=3;i=1003,ns=3;i=1004][s|{"ns=3;i=1002":"0.6038459","ns=3;i=1003":"1.2","ns=3;i=1004":"1.17557"}]

```

### Put Endpoint, Username and Password in configuration file

If your server supports authentication by username/password update `opcua.conf` as shown below. Restart agent2 service and test:

```bash
$ grep -vi '^#\|^$' /etc/zabbix/zabbix_agent2.d/plugins.d/opcua.conf
Plugins.OPCUA.System.Path=/usr/local/bin/zabbix-agent2-plugin-opcua
Plugins.OPCUA.Default.Endpoint=opc.tcp://172.26.0.71:4843
Plugins.OPCUA.Default.User=toast
Plugins.OPCUA.Default.Password=***********

viewer@l2-brlin-04:~$ sudo zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t 'opcua.get[,,,,,ns=4;s=TankCollection.SystemInfo.AliveCounter]'
opcua.get[,,,,,ns=4;s=TankCollection.SystemInfo.AliveCounter][s|14493]

viewer@l2-brlin-04:~$ sudo zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t 'opcua.get[,,,,,ns=4;s=TankCollection.SystemInfo.AliveCounter]'
opcua.get[,,,,,ns=4;s=TankCollection.SystemInfo.AliveCounter][s|14513]

```

Sample error if provided credentials are somehow incorrect:

```bash
$ sudo zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t 'opcua.get[,,,,,ns=4;s=TankCollection.SystemInfo.AliveCounter]'
opcua.get[,,,,,ns=4;s=TankCollection.SystemInfo.AliveCounter][m|ZBX_NOTSUPPORTED] [Failed to read node value(s): failed to connect to any configured endpoint: endpoint "opc.tcp://172.26.0.71:4843": failed to connect: User does not have permission to perform the requested operation. StatusBadUserAccessDenied (0x801F0000).]
```


## Debug logging

For troubleshooting you need to increase Zabbix agent2 logging level in `zabbix_agent2.conf`:

```ini
DebugLevel=5
```

Then restart the service:

```bash
$ sudo systemctl restart zabbix-agent2.service
```

Follow logs with:

```bash
$ sudo journalctl -xeu zabbix-agent2.service
```

`DebugLevel=5` is useful while diagnosing plugin startup, configuration, IPC, connection and item execution problems. Restore your normal logging level after troubleshooting because debug logging is pretty verbose.

## Verified functionality

Confirmed working against a real production server (`SecurityMode=None`/anonymous): plugin registration, config parsing, a live `opcua.get` read through `zabbix_agent2 -t` and sustained polling through the running agent2 daemon. Read latency against an already-pooled session has consistently measured 3-10ms in production logs.

Confirmed working with username/password credentials provided in configuration file.

The connection pool has been confirmed to work exactly as designed: production logs show many distinct hosts and items, all resolving to the same `Endpoint` and sharing a single pooled connection rather than one per item. Pooling is keyed on an *exact string match* of `Endpoint` (plus security/auth) - `opc.tcp://host:4840` and `opc.tcp://host:4840/` (a trailing slash) are two different broker entries with two separate connections, not the same one. Worth a consistent convention wherever `Endpoint` is set, since drift here silently doubles connections rather than erroring.

Multi-endpoint failover has been confirmed working against a live pair (one real server, one unreachable address standing in for a dead node) where the plugin correctly fell through to the working endpoint. It took ~1.5s to complete initial sequence, i.e connect → discover the primary is unreachable → try the next candidate. Every read on the same broker afterward while the session stays live was back to the normal 3-10ms.

Multi-NodeID reads have been confirmed working (3 NodeIDs in one key returned as JSON structure) against a live server.

Not yet verified:

- `SecurityMode=Sign`/`SignAndEncrypt`, and the `CertFile`/`KeyFile` connection logic in `client.go` - only `None` has been exercised against a live server so far.
- Error paths other than the endpoint-list failover above - a NodeID that doesn't exist, a genuine read timeout - haven't been deliberately triggered.

## Known limitations

- The provided build script produces a **Linux AMD64** binary only.
- OPC UA interoperability depends on the functionality exposed by this plugin and by the selected `gopcua/opcua` dependency; this project does not claim complete OPC UA specification coverage.
- Security policies, certificate handling, authentication methods, OPC UA data types, arrays, complex/structured values, subscriptions, events, methods, discovery, and failover should be considered unsupported/unverified unless specifically exercised and documented for a release.
- `gopcua/opcua@latest` in `runme.sh` makes dependency resolution non-reproducible over time unless the resolved dependency is subsequently pinned.
- SDK compatibility matters. A plugin built against a development SDK revision may not be compatible with every Zabbix agent2 release.
- Network latency, OPC UA server limits, server-side session/subscription limits and timeout configuration can materially affect collection performance.
- The repository currently has no documented automated test suite in the supplied tree; manual/integration testing against a real OPC UA target is therefore important.

## Plugin-side scaling characteristics

`client.go` holds one mutex per broker (distinct endpoint/endpoint-list + security/auth combination) for the full duration of a connect-and-read. Multiple `opcua.get` calls against the *same* broker queue behind that lock rather than running in parallel, even though agent2 itself can dispatch concurrent `Export()` calls to the plugin process.

At the observed ~5ms per steady-state read, this lock can sustain on the order of 200 reads/second before it becomes the binding constraint - comfortably ahead of light polling loads, but worth watching as the number of `opcua.get` calls against one server grows. The scenario that actually matters in practice is a burst: every overdue item becoming due at once after an agent2 restart. Because all of those items share one mutex, the burst clears in (queued items × ~5ms) total regardless of how many individual items or hosts are involved - bounded and self-healing, but any item that would have had to wait longer than its own configured item timeout to get its turn shows as one missed scan for that one restart, not an ongoing failure.

If the primary in a redundant endpoint list happens to be down at the moment of that restart, the very first connect for that broker also pays the ~1.5s failover cost measured above, before any queued read starts - a one-time deduction from whatever timeout budget the burst has to work with, not a per-tag cost.

The mutex itself is a deliberate simplification: gopcua's client is documented as request/response-correlated, and loosening the lock to cover only the connect step, rather than the whole read, is the natural next step if this becomes a real bottleneck.


## Troubleshooting checklist

If a check is unsupported or fails:

1. Confirm `zabbix-agent2-plugin-opcua` is executable and its configured path is correct.
2. Confirm the plugin configuration is included by `zabbix_agent2.conf`.
3. Restart agent2 after configuration changes.
4. Set `DebugLevel=5` temporarily and inspect the agent journal.
5. Verify the OPC UA endpoint is reachable from the agent host.
6. Validate the same endpoint/node with an independent OPC UA client such as UaExpert.
7. Check authentication, certificate trust, security policy, namespace and NodeId values.
8. Run the item locally with `zabbix_agent2 -c ... -t ...` before testing from the server.
9. Verify that the plugin SDK revision is compatible with the installed Zabbix agent2 version.


## Contributing

Issues and feature requests are welcome. When reporting a problem, include sanitized versions of:

- Zabbix agent2 version.
- Plugin commit/tag.
- Go version and architecture.
- OPC UA server product/version.
- Relevant plugin configuration (without secrets).
- Item key being tested.
- Debug log excerpt around the failure.

Authors would be pleased to receive success stories about testing this plugin in your own OT environment as a comment on this repository or in the Zabbix forum.

## License

This project is licensed under the **GNU Affero General Public License, version 3 (AGPLv3)**. See [`LICENSE`](LICENSE) for the full license text.

If you modify and redistribute the software, or operate a modified version in circumstances covered by the AGPL's network-interaction provisions, make sure you understand and comply with the corresponding source-code obligations. This README is not legal advice.

## References

- [Zabbix Developer Center — Agent 2 plugins](https://www.zabbix.com/documentation/7.4/en/devel/plugins)
- [Prosys OPC UA Simulation Server](https://prosysopc.com/products/opc-ua-simulation-server/)
- [UA Expert Server and client](https://www.unified-automation.com/downloads.html)
- [GNU AGPLv3](https://www.gnu.org/licenses/agpl-3.0.html)

## Disclaimer

This is an independent project unless explicitly stated otherwise. Zabbix is a trademark of Zabbix LLC. OPC UA is a technology of the OPC Foundation. Experion is a trademark owned by Honeywell International Inc. TankMaster is a trademark of Rosemount Tank Radar AB, subsidiary company operating under Emerson Electric Co.

Product and company names are used only to identify interoperability / test environments.
