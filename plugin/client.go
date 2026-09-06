package plugin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"golang.zabbix.com/sdk/errs"
)

// broker identifies one distinct OPC UA connection - same endpoint (or
// endpoint list, for a redundant pair) and same security/auth settings.
// Metric calls that resolve to the same broker share one underlying
// session, the same way the built-in MQTT plugin shares one broker
// connection across every subscription on it.
//
// All fields are plain strings so broker stays comparable and can be used
// directly as a map key.
type broker struct {
	endpoint       string
	securityMode   string
	securityPolicy string
	certFile       string
	keyFile        string
	user           string
	password       string
}

type opcuaClient struct {
	mu        sync.Mutex
	b         broker
	client    *opcua.Client
	connected bool
}

// connPool is a small mutex-guarded map from broker identity to session,
// deliberately self-contained rather than built on golang.zabbix.com/sdk's
// zbxsync.SyncMap (used by the built-in MQTT plugin for the same purpose).
// zbxsync.SyncMap would work fine here too - this just avoids depending on
// its exact method surface for something this small.
type connPool struct {
	mu    sync.Mutex
	conns map[broker]*opcuaClient
}

func newConnPool() *connPool {
	return &connPool{conns: make(map[broker]*opcuaClient)}
}

func (p *connPool) get(b broker) *opcuaClient {
	p.mu.Lock()
	defer p.mu.Unlock()

	oc, ok := p.conns[b]
	if !ok {
		oc = &opcuaClient{b: b}
		p.conns[b] = oc
	}

	return oc
}

// closeAll disconnects every pooled session. Called from Plugin.Stop when
// the agent deactivates the plugin.
func (p *connPool) closeAll(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for b, oc := range p.conns {
		oc.close(ctx)
		delete(p.conns, b)
	}
}

func (oc *opcuaClient) close(ctx context.Context) {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if oc.client != nil {
		_ = oc.client.Close(ctx)
	}

	oc.client = nil
	oc.connected = false
}

// connect tries each configured endpoint in order and opens a session
// against the first one that answers. Endpoint is a single URL for a
// standalone server, or a semicolon-separated list for a redundant pair -
// see splitEndpoints. Every candidate is assumed to share the same
// security mode/policy and credentials; there's no per-endpoint override,
// on the assumption that a redundant pair is the same underlying system
// (e.g. two Experion nodes) rather than genuinely different servers.
//
// The overall timeout budget is split evenly across candidates, so one
// unreachable (as opposed to promptly-refused) primary can't consume the
// whole budget and starve the fallback from ever being tried. With the
// default single endpoint this has no effect - the one candidate gets the
// full timeout, same as before.
//
// Always starting from the top of the list, rather than remembering which
// candidate worked last time, means a recovered primary is preferred
// again on the very next reconnect without any separate failback logic.
//
// IMPORTANT - this has not been built or run against a live server: the
// sandbox this was written in has no network access to pull
// github.com/gopcua/opcua or compile against it. The call shapes below
// (GetEndpoints, SelectEndpoint, SecurityFromEndpoint, AuthUsername /
// AuthAnonymous, CertificateFile / PrivateKeyFile, ua.StatusOK,
// ua.TimestampsToReturnNeither) are taken from gopcua's own
// examples/subscribe.go, examples/read.go and examples/crypto - not from a
// successful local build against the current release. Run
// `go mod tidy && go build`, and check the installed gopcua version's
// godoc, before trusting this against a real server.
func (oc *opcuaClient) connect(ctx context.Context, timeout time.Duration) error {
	candidates := splitEndpoints(oc.b.endpoint)
	if len(candidates) == 0 {
		return errs.New("no endpoint configured")
	}

	perCandidate := timeout / time.Duration(len(candidates))
	if perCandidate <= 0 {
		perCandidate = timeout
	}

	var lastErr error

	for _, candidate := range candidates {
		candCtx, cancel := context.WithTimeout(ctx, perCandidate)
		c, err := oc.dial(candCtx, candidate)
		cancel()

		if err != nil {
			lastErr = errs.Wrapf(err, "endpoint %q", candidate)

			continue
		}

		oc.client = c
		oc.connected = true

		return nil
	}

	return errs.Wrap(lastErr, "failed to connect to any configured endpoint")
}

// splitEndpoints turns a possibly semicolon-separated Endpoint value into
// an ordered list of candidate URLs, trimming whitespace and dropping
// empty entries (so a trailing ";" or accidental double separator doesn't
// produce a blank candidate).
func splitEndpoints(raw string) []string {
	parts := strings.Split(raw, ";")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// dial performs the discover-select-connect sequence against one
// candidate endpoint URL. Factored out of connect() so that function can
// just be "try each candidate until one of these succeeds".
func (oc *opcuaClient) dial(ctx context.Context, endpoint string) (*opcua.Client, error) {
	endpoints, err := opcua.GetEndpoints(ctx, endpoint)
	if err != nil {
		return nil, errs.Wrap(err, "failed to fetch endpoints")
	}

	mode := ua.MessageSecurityModeFromString(oc.b.securityMode)

	ep, err := opcua.SelectEndpoint(endpoints, oc.b.securityPolicy, mode)
	if err != nil {
		return nil, errs.Wrap(err, "failed to select a matching endpoint")
	}

	// Dial the endpoint the item was configured with, not whatever host
	// the server advertised in its own endpoint description - servers
	// behind NAT or a reverse proxy often advertise an address the agent
	// can't actually reach.
	ep.EndpointURL = endpoint

	tokenType := ua.UserTokenTypeAnonymous
	if oc.b.user != "" {
		tokenType = ua.UserTokenTypeUserName
	}

	opts := []opcua.Option{
		opcua.SecurityFromEndpoint(ep, tokenType),
	}

	if oc.b.certFile != "" && oc.b.keyFile != "" {
		opts = append(opts, opcua.CertificateFile(oc.b.certFile), opcua.PrivateKeyFile(oc.b.keyFile))
	}

	if oc.b.user != "" {
		opts = append(opts, opcua.AuthUsername(oc.b.user, oc.b.password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}

	c, err := opcua.NewClient(ep.EndpointURL, opts...)
	if err != nil {
		return nil, errs.Wrap(err, "failed to create client")
	}

	if err := c.Connect(ctx); err != nil {
		return nil, errs.Wrap(err, "failed to connect")
	}

	return c, nil
}

// read performs a single Read service call for the given node IDs,
// connecting (or reconnecting) the underlying session first if needed.
//
// The whole session is locked for the duration of the call rather than
// relying on gopcua's own concurrency handling. That's a deliberate
// simplification, not a verified constraint: it trades some throughput on
// a shared session (concurrent opcua.get calls against the same broker
// queue up) for a connect/reconnect path that's simple to reason about.
// gopcua's *Client is documented as request/response-correlated, so
// finer-grained concurrency is probably possible - worth revisiting if a
// heavily-polled OPC UA server ever shows queuing here.
func (oc *opcuaClient) read(ctx context.Context, timeout time.Duration, nodeIDs []string) (map[string]string, error) {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if !oc.connected {
		if err := oc.connect(ctx, timeout); err != nil {
			return nil, err
		}
	}

	toRead := make([]*ua.ReadValueID, len(nodeIDs))

	for i, raw := range nodeIDs {
		id, err := ua.ParseNodeID(raw)
		if err != nil {
			return nil, errs.Wrapf(err, "invalid NodeID %q", raw)
		}

		toRead[i] = &ua.ReadValueID{NodeID: id}
	}

	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := oc.client.Read(readCtx, &ua.ReadRequest{
		NodesToRead:        toRead,
		TimestampsToReturn: ua.TimestampsToReturnNeither,
	})
	if err != nil {
		// Any transport-level failure invalidates the session - the next
		// call reconnects rather than repeatedly hitting a dead one.
		oc.connected = false

		return nil, errs.Wrap(err, "read request failed")
	}

	if len(resp.Results) != len(nodeIDs) {
		oc.connected = false

		return nil, errs.New("server returned an unexpected number of results")
	}

	out := make(map[string]string, len(nodeIDs))

	for i, dv := range resp.Results {
		if dv.Status != ua.StatusOK {
			return nil, errs.Errorf("node %q: status %s", nodeIDs[i], dv.Status)
		}

		if dv.Value == nil {
			return nil, errs.Errorf("node %q: server returned no value", nodeIDs[i])
		}

		out[nodeIDs[i]] = fmt.Sprintf("%v", dv.Value.Value())
	}

	return out, nil
}
