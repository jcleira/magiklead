package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
)

// stub is an httptest fake of Unipile's /api/v1/webhooks surface: it serves a
// mutable registration list, records every request the CLI makes, and lets a
// test assert both the plan the CLI printed and the calls it actually issued.
// Tests drive the real *unipile.Module against it (the Doer seam), so a run
// exercises CLI dispatch → client → HTTP → output end to end.
type stub struct {
	*httptest.Server

	mu       sync.Mutex
	items    []map[string]any
	requests []recordedRequest
	nextID   int
}

type recordedRequest struct {
	method string
	path   string
	body   map[string]any
}

func newStub(t *testing.T, existing ...map[string]any) *stub {
	t.Helper()
	s := &stub{items: existing}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *stub) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, _ := io.ReadAll(r.Body)
	rec := recordedRequest{method: r.Method, path: r.URL.Path}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &rec.body)
	}
	s.requests = append(s.requests, rec)

	switch {
	case r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "WebhookList",
			"items":  s.items,
			"cursor": nil,
		})

	case r.Method == http.MethodPost:
		s.nextID++
		created := map[string]any{"object": "Webhook", "id": idFor(s.nextID), "enabled": true}
		for k, v := range rec.body {
			created[k] = v
		}
		s.items = append(s.items, created)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)

	case r.Method == http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/webhooks/")
		kept := s.items[:0]
		for _, it := range s.items {
			if it["id"] != id {
				kept = append(kept, it)
			}
		}
		s.items = kept
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "WebhookDeleted"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func idFor(n int) string { return "wh_created_" + string(rune('a'+n-1)) }

// mutating returns the POST/DELETE requests the CLI issued — the assertion
// --dry-run turns on.
func (s *stub) mutating() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recordedRequest
	for _, r := range s.requests {
		if r.method == http.MethodPost || r.method == http.MethodDelete {
			out = append(out, r)
		}
	}
	return out
}

// registration builds a stub-side webhook row as Unipile returns it.
func registration(id, source, name, requestURL string, events ...string) map[string]any {
	if events == nil {
		events = []string{}
	}
	return map[string]any{
		"object":      "Webhook",
		"id":          id,
		"source":      source,
		"name":        name,
		"request_url": requestURL,
		"enabled":     true,
		"events":      events,
		"headers": []map[string]string{
			{"key": "Unipile-Auth", "value": stubSecret},
		},
	}
}

// stubSecret stands in for UNIPILE_WEBHOOK_SECRET — the static Unipile-Auth
// value the CLI must stamp into every registration it creates.
const stubSecret = "stub-secret"

// exec runs the CLI end to end against the stub and returns its output.
func exec(t *testing.T, s *stub, args ...string) string {
	t.Helper()
	m := unipile.New("key", s.URL, nil, s.Client())
	opts, err := parseArgs(args)
	if err != nil {
		t.Fatalf("parseArgs(%v): %v", args, err)
	}
	opts.authToken = stubSecret
	var out bytes.Buffer
	if err := run(context.Background(), &out, m, opts); err != nil {
		t.Fatalf("run(%v): %v", args, err)
	}
	return out.String()
}

// TestListPrintsRegistrations is the tracer bullet: one `list` run proves the
// whole path — arg parsing, the Module's ListWebhooks over HTTP, and the
// printed report — against the real WebhookList envelope Unipile returns.
func TestListPrintsRegistrations(t *testing.T) {
	s := newStub(t,
		registration("fdAhDzUxQbO", "messaging", "magiklead-mvp-messaging",
			"https://tunnel.example.com/api/v1/webhooks/unipile", "message_received"),
		registration("lv7ahylIQ7i", "account_status", "magiklead-mvp-account_status",
			"https://tunnel.example.com/api/v1/webhooks/unipile"),
	)

	got := exec(t, s, "list")

	for _, want := range []string{
		"fdAhDzUxQbO", "messaging", "magiklead-mvp-messaging",
		"https://tunnel.example.com/api/v1/webhooks/unipile",
		"lv7ahylIQ7i", "account_status", "magiklead-mvp-account_status",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("list output missing %q\n--- output ---\n%s", want, got)
		}
	}
	if n := len(s.mutating()); n != 0 {
		t.Errorf("list issued %d mutating requests, want 0", n)
	}
}

// postsBySource indexes the stub's recorded POST bodies by the source they
// registered, failing the test if anything other than a POST mutated.
func postsBySource(t *testing.T, s *stub) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, r := range s.mutating() {
		if r.method != http.MethodPost {
			t.Fatalf("unexpected %s %s during register", r.method, r.path)
		}
		source, _ := r.body["source"].(string)
		out[source] = r.body
	}
	return out
}

// events pulls the JSON events array off a recorded POST body.
func events(body map[string]any) []string {
	raw, _ := body["events"].([]any)
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		s, _ := e.(string)
		out = append(out, s)
	}
	return out
}

// TestRegisterCreatesThreeSourcesThenKeepsThem drives the acceptance
// criterion directly: the first run registers all three sources the rail
// needs with the right request_url, auth header and event selectors; the
// second run against the now-populated workspace creates nothing.
func TestRegisterCreatesThreeSourcesThenKeepsThem(t *testing.T) {
	s := newStub(t)
	const base = "https://magiklead-smoke.example.com"
	const prefix = "magiklead-smoke-"
	wantURL := base + "/api/v1/webhooks/unipile"

	first := exec(t, s, "register", "--base-url", base, "--name-prefix", prefix)

	posted := postsBySource(t, s)
	if len(posted) != 3 {
		t.Fatalf("first register created %d sources (%v), want 3", len(posted), posted)
	}
	for _, source := range []string{"account_status", "users", "messaging"} {
		body, ok := posted[source]
		if !ok {
			t.Fatalf("first register did not create source %q (created %v)", source, posted)
		}
		if got := body["request_url"]; got != wantURL {
			t.Errorf("%s request_url = %v, want %v", source, got, wantURL)
		}
		name, _ := body["name"].(string)
		if !strings.HasPrefix(name, prefix) {
			t.Errorf("%s name = %q, want the %q ownership marker", source, name, prefix)
		}
		hdrs, _ := body["headers"].([]any)
		if len(hdrs) != 1 {
			t.Fatalf("%s headers = %v, want exactly the Unipile-Auth header", source, body["headers"])
		}
		h, _ := hdrs[0].(map[string]any)
		if h["key"] != "Unipile-Auth" || h["value"] != stubSecret {
			t.Errorf("%s auth header = %v, want Unipile-Auth=%s", source, h, stubSecret)
		}
	}
	if got := events(posted["messaging"]); len(got) != 1 || got[0] != "message_received" {
		t.Errorf("messaging events = %v, want [message_received]", got)
	}
	if got := events(posted["users"]); len(got) != 1 || got[0] != "new_relation" {
		t.Errorf("users events = %v, want [new_relation]", got)
	}
	if !strings.Contains(first, "created=3") || !strings.Contains(first, "kept=0") {
		t.Errorf("first register report = %q, want created=3 kept=0", first)
	}

	second := exec(t, s, "register", "--base-url", base, "--name-prefix", prefix)

	if n := len(s.mutating()); n != 3 {
		t.Errorf("after the second register the stub saw %d mutating calls, want the original 3", n)
	}
	if !strings.Contains(second, "created=0") || !strings.Contains(second, "kept=3") {
		t.Errorf("second register report = %q, want created=0 kept=3", second)
	}
}

// deletedIDs returns the ids the CLI issued a DELETE for, failing the test if
// prune mutated anything by another method.
func deletedIDs(t *testing.T, s *stub) []string {
	t.Helper()
	var ids []string
	for _, r := range s.mutating() {
		if r.method != http.MethodDelete {
			t.Fatalf("unexpected %s %s during prune", r.method, r.path)
		}
		ids = append(ids, strings.TrimPrefix(r.path, "/api/v1/webhooks/"))
	}
	sort.Strings(ids)
	return ids
}

// mixedWorkspace is a workspace holding two of our registrations — one live,
// one left behind by a dead tunnel — plus a foreign one prune must never
// touch. The Unipile workspace is shared, so "delete every webhook" would be
// destructive to somebody else.
func mixedWorkspace(t *testing.T) *stub {
	t.Helper()
	return newStub(t,
		registration("ours_stale", "messaging", "magiklead-smoke-messaging",
			"https://dead-tunnel.example.com/api/v1/webhooks/unipile", "message_received"),
		registration("ours_live", "account_status", "magiklead-smoke-account_status",
			liveBase+"/api/v1/webhooks/unipile"),
		registration("foreign", "messaging", "someone-elses-integration",
			"https://other-tenant.example.com/hook", "message_received"),
	)
}

const liveBase = "https://magiklead-smoke.example.com"

// TestPruneDeletesOnlyOurRegistrations is the ownership-marker criterion:
// a bare prune clears every registration carrying our name prefix and leaves
// the foreign one alone.
func TestPruneDeletesOnlyOurRegistrations(t *testing.T) {
	s := mixedWorkspace(t)

	out := exec(t, s, "prune", "--name-prefix", "magiklead-smoke-")

	got := deletedIDs(t, s)
	want := []string{"ours_live", "ours_stale"}
	if !slices.Equal(got, want) {
		t.Fatalf("prune deleted %v, want %v (the foreign registration must survive)", got, want)
	}
	if !strings.Contains(out, "deleted=2") {
		t.Errorf("prune report = %q, want deleted=2", out)
	}
	if strings.Contains(out, "someone-elses-integration") && strings.Contains(out, "deleted ") {
		t.Errorf("prune report names the foreign registration as deleted:\n%s", out)
	}
}

// TestPruneWithBaseURLKeepsTheLiveRegistration covers the operational shape:
// after the tunnel hostname changes, prune --base-url <new> sweeps our stale
// registrations and keeps the ones already pointing at the new origin.
func TestPruneWithBaseURLKeepsTheLiveRegistration(t *testing.T) {
	s := mixedWorkspace(t)

	out := exec(t, s, "prune", "--base-url", liveBase, "--name-prefix", "magiklead-smoke-")

	got := deletedIDs(t, s)
	want := []string{"ours_stale"}
	if !slices.Equal(got, want) {
		t.Fatalf("prune --base-url deleted %v, want %v", got, want)
	}
	if !strings.Contains(out, "deleted=1") || !strings.Contains(out, "kept=1") {
		t.Errorf("prune report = %q, want deleted=1 kept=1", out)
	}
}

// TestDryRunRegisterPrintsPlanAndMutatesNothing is the safety criterion: the
// plan is fully printed — every source, with the request_url it would be
// created against — while not one POST reaches Unipile.
func TestDryRunRegisterPrintsPlanAndMutatesNothing(t *testing.T) {
	s := newStub(t)

	out := exec(t, s, "register", "--base-url", liveBase, "--name-prefix", "magiklead-smoke-", "--dry-run")

	if n := len(s.mutating()); n != 0 {
		t.Fatalf("dry-run register issued %d mutating requests, want 0: %v", n, s.mutating())
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	for _, want := range []string{
		"account_status", "users", "messaging",
		liveBase + "/api/v1/webhooks/unipile",
		"magiklead-smoke-messaging",
		"would-create=3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run plan missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestDryRunPrunePrintsPlanAndMutatesNothing is the same guarantee on the
// destructive side — the one where getting it wrong costs a live webhook.
func TestDryRunPrunePrintsPlanAndMutatesNothing(t *testing.T) {
	s := mixedWorkspace(t)

	out := exec(t, s, "prune", "--name-prefix", "magiklead-smoke-", "--dry-run")

	if n := len(s.mutating()); n != 0 {
		t.Fatalf("dry-run prune issued %d mutating requests, want 0: %v", n, s.mutating())
	}
	for _, want := range []string{
		"dry-run",
		"magiklead-smoke-messaging",
		"magiklead-smoke-account_status",
		"would-delete=2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run plan missing %q\n--- output ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "someone-elses-integration") {
		t.Errorf("dry-run prune plan includes the foreign registration:\n%s", out)
	}
}
