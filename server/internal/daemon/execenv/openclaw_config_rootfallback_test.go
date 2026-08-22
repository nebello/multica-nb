package execenv

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Stub locale invece di quello condiviso in openclaw_config_test.go: il
// ricambio per-chiave interroga la CLI in parallelo, e quello stub accumula le
// chiamate senza lock. Un file nostro evita sia la corsa sia una modifica a un
// test di upstream, che sarebbe un conflitto a ogni rebase.
type concurrentOpenclawStub struct {
	mu        sync.Mutex
	calls     []string
	responses map[string]openclawResponse
}

func (s *concurrentOpenclawStub) exec(_ context.Context, _ string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	s.mu.Lock()
	s.calls = append(s.calls, key)
	s.mu.Unlock()
	if resp, ok := s.responses[key]; ok {
		return resp.stdout, resp.err
	}
	return "", errors.New("Config path not found: " + key)
}

func (s *concurrentOpenclawStub) install(t *testing.T) *concurrentOpenclawStub {
	t.Helper()
	prev := openclawExec
	openclawExec = s.exec
	t.Cleanup(func() { openclawExec = prev })
	return s
}

const rootPathRequiredErr = `openclaw config get --json: exit status 1 (stderr: Missing required argument "path".)`

func TestResolvedFullConfigFallsBackWhenRootNeedsAPath(t *testing.T) {
	(&concurrentOpenclawStub{responses: map[string]openclawResponse{
		"config get --json": {err: errors.New(rootPathRequiredErr)},
		"config schema": {stdout: `{"properties":{
			"$schema":{},"agents":{},"gateway":{},"mcp":{},"telemetry":{}
		}}`},
		"config get agents --json":  {stdout: `{"list":[{"id":"deepseek"}]}`},
		"config get gateway --json": {stdout: `{"port":19001}`},
		"config get mcp --json":     {stdout: `{"servers":{"utente":{"command":"x"}},"sessionIdleTtlMs":900}`},
		// telemetry non risponde: cade nel default dello stub, cioe' "Config
		// path not found", che va saltato senza far fallire il giro.
	}}).install(t)

	cfg, err := openclawResolvedFullConfig("/test/stub/openclaw", time.Second)
	if err != nil {
		t.Fatalf("il ricambio doveva riuscire, invece: %v", err)
	}
	if _, ok := cfg["telemetry"]; ok {
		t.Fatalf("una chiave non impostata non deve finire nello snapshot: %v", cfg)
	}
	if _, ok := cfg["$schema"]; ok {
		t.Fatalf("$schema e' metadato dello schema, non un percorso di config: %v", cfg)
	}
	for _, want := range []string{"agents", "gateway", "mcp"} {
		if _, ok := cfg[want]; !ok {
			t.Fatalf("manca la chiave %q nello snapshot ricostruito: %v", want, cfg)
		}
	}

	// La ragione per cui tutto questo esiste: lo snapshot deve restare
	// spogliabile di mcp.servers tenendo il resto di mcp.
	stripUserMcpServers(cfg)
	mcp, _ := cfg["mcp"].(map[string]any)
	if _, leaked := mcp["servers"]; leaked {
		t.Fatalf("i server MCP dell'utente sono rimasti nello snapshot: %v", mcp)
	}
	if mcp["sessionIdleTtlMs"] == nil {
		t.Fatalf("il resto della sezione mcp doveva sopravvivere: %v", mcp)
	}
	// Serializzabile: e' quello che il chiamante scrive su disco.
	if _, err := json.Marshal(cfg); err != nil {
		t.Fatalf("snapshot non serializzabile: %v", err)
	}
}

func TestResolvedFullConfigKeepsSingleCallWhenRootWorks(t *testing.T) {
	stub := (&concurrentOpenclawStub{responses: map[string]openclawResponse{
		"config get --json": {stdout: `{"gateway":{"port":19001}}`},
	}}).install(t)

	cfg, err := openclawResolvedFullConfig("/test/stub/openclaw", time.Second)
	if err != nil {
		t.Fatalf("la lettura dalla radice doveva riuscire: %v", err)
	}
	if _, ok := cfg["gateway"]; !ok {
		t.Fatalf("config inattesa: %v", cfg)
	}
	stub.mu.Lock()
	n := len(stub.calls)
	stub.mu.Unlock()
	if n != 1 {
		// Su una CLI che regge ancora la radice il ricambio non deve
		// scattare: sarebbero decine di invocazioni per niente.
		t.Fatalf("attesa una sola invocazione, ne sono state fatte %d: %v", n, stub.calls)
	}
}

func TestResolvedFullConfigSurfacesOtherFailures(t *testing.T) {
	(&concurrentOpenclawStub{responses: map[string]openclawResponse{
		"config get --json": {err: errors.New("exit status 127 (stderr: openclaw: command not found)")},
	}}).install(t)

	if _, err := openclawResolvedFullConfig("/test/stub/openclaw", time.Second); err == nil {
		t.Fatal("un guasto diverso dal path obbligatorio deve emergere, non attivare il ricambio")
	}
}
