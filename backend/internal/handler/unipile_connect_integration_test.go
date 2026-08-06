//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// connectFixture is the minimal graph the connect-and-bind webhook (issue #3)
// acts on: a bare tenant with NO linkedin_account row yet. The webhook must
// create and bind the account from the signed metadata alone — proving the
// INSERT branch of the upsert, not the reconnect ON CONFLICT branch the status
// suite already covers.
//
// Run with:
//
//	devpods exec api go test -tags=integration ./internal/handler/...
type connectFixture struct {
	tenantID  uuid.UUID
	unipileID string
}

func newConnectFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) connectFixture {
	t.Helper()
	run := uuid.NewString()
	f := connectFixture{
		tenantID:  uuid.New(),
		unipileID: "acc_connect_" + run,
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, name) VALUES ($1, $2)`, f.tenantID, "Connect Test "+run); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE unipile_account_id = $1`, f.unipileID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
	})
	return f
}

// loadConnectWebhook reads the frozen connect fixture and substitutes the two
// inherently-dynamic fields: the Unipile account id and the signed metadata
// token (per-tenant and time-boxed, so it can never be frozen statically). The
// fixture's shape is Unipile's hosted-auth notify_url success callback —
// verified against developer.unipile.com/docs/hosted-auth: a flat
// {status,account_id,name} body where `name` echoes the value we set on the
// link (our signed tenant metadata). It is the only inbound payload carrying
// our tenant metadata, so it is the only one that connect-and-binds.
func loadConnectWebhook(t *testing.T, unipileID, meta string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/webhook_account_connected.json")
	if err != nil {
		t.Fatalf("read connect fixture: %v", err)
	}
	body := strings.ReplaceAll(string(raw), "__ACCOUNT_ID__", unipileID)
	body = strings.ReplaceAll(body, "__METADATA__", meta)
	return []byte(body)
}

// postConnect signs a fresh metadata token binding unipileID to tenantID,
// wraps it in the frozen connect body, and delivers it to the webhook exactly
// as Unipile's notify_url callback would — crucially WITHOUT the Unipile-Auth
// header. Unipile does not attach that header to the per-session hosted-auth
// callback; the header'd variant this test used to send masked a real 401 that
// blocked every live connect (found in the 2026-07 smoke). The bind therefore
// authenticates on the signed metadata alone. Returns the recorder.
func postConnect(t *testing.T, h *UnipileHandler, secret []byte, unipileID string, tenantID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	meta := signMeta(t, secret, tenantID, uuid.New(), time.Now().Add(10*time.Minute))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile",
		strings.NewReader(string(loadConnectWebhook(t, unipileID, meta))))
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)
	return rr
}

// TestUnipile_Webhook_ConnectAndBind_FreshInsert is the connect-and-bind
// tracer (issue #3, AC#1/#5) through every layer: a signed connect webhook for
// a tenant with no account yet is authenticated, parsed, its tenant recovered
// from the signed metadata, and a linkedin_accounts row is INSERTed bound to
// that tenant with status 'active' — zero manual SQL. Asserts via DB.
func TestUnipile_Webhook_ConnectAndBind_FreshInsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newConnectFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	if rr := postConnect(t, h, secret, f.unipileID, f.tenantID); rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	var tenantID pgtype.UUID
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, status FROM linkedin_accounts WHERE unipile_account_id = $1`,
		f.unipileID).Scan(&tenantID, &status); err != nil {
		t.Fatalf("scan bound account (no row inserted?): %v", err)
	}
	if !tenantID.Valid || uuid.UUID(tenantID.Bytes) != f.tenantID {
		t.Errorf("tenant_id=%x want %s (bound to the metadata tenant)", tenantID.Bytes, f.tenantID)
	}
	if status != "active" {
		t.Errorf("status=%q want active (connected)", status)
	}
}

// TestUnipile_Webhook_ConnectAndBind_SurfacesActive proves AC#4: after the
// bind, the account surfaces a clear connected/active status through the same
// read path the Settings page uses — GET /api/v1/linkedin/accounts
// (ListAccounts) — not just in the raw row. Drives the handler with the bound
// tenant in context and asserts the account is listed as 'active'.
func TestUnipile_Webhook_ConnectAndBind_SurfacesActive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newConnectFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	if rr := postConnect(t, h, secret, f.unipileID, f.tenantID); rr.Code != http.StatusOK {
		t.Fatalf("connect status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	lr := httptest.NewRecorder()
	h.ListAccounts(lr, withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/accounts", nil), f.tenantID))
	if lr.Code != http.StatusOK {
		t.Fatalf("ListAccounts status=%d want 200 body=%s", lr.Code, lr.Body.String())
	}

	var got []struct {
		UnipileAccountID string `json:"unipile_account_id"`
		Status           string `json:"status"`
	}
	if err := json.Unmarshal(lr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode accounts: %v body=%s", err, lr.Body.String())
	}
	found := false
	for _, a := range got {
		if a.UnipileAccountID == f.unipileID {
			found = true
			if a.Status != "active" {
				t.Errorf("surfaced status=%q want active", a.Status)
			}
		}
	}
	if !found {
		t.Errorf("bound account %s not surfaced by ListAccounts", f.unipileID)
	}
}

// TestUnipile_Webhook_StatusWithoutMetadata_DoesNotBind proves AC#2's negative:
// a payload without metadata that merely reports a status — the nested
// AccountStatus shape Unipile's account channel sends — is NOT treated as
// connect. Against a tenant with no row it is a no-op (nothing to update), so
// no account is created or bound. Asserts via DB (row count stays 0).
func TestUnipile_Webhook_StatusWithoutMetadata_DoesNotBind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newConnectFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	// Nested AccountStatus, CREATION_SUCCESS, but carrying NO metadata token.
	body := []byte(`{"AccountStatus":{"account_id":"` + f.unipileID + `","account_type":"LINKEDIN","message":"CREATION_SUCCESS"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id = $1`,
		f.unipileID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("row count=%d want 0 (a no-metadata status must not bind an account)", n)
	}
}

// TestUnipile_Webhook_NonConnectWithoutHeader_Rejected locks the boundary the
// connect exemption must not widen: ONLY account.connected (authenticated by
// its signed metadata) may pass without the Unipile-Auth header. Every other
// event comes from a registered webhook that does carry the header, so a
// header-less non-connect payload is a forgery and must 401 before any handler
// runs.
func TestUnipile_Webhook_NonConnectWithoutHeader_Rejected(t *testing.T) {
	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, stateSecret: secret}

	// A messaging event (registered webhook) delivered with NO Unipile-Auth header.
	body := []byte(`{"event":"message_received","account_id":"acc_x","chat_id":"c1","message_id":"m1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (a non-connect event without the header is a forgery)", rr.Code)
	}
}

// TestUnipile_Webhook_ConnectWithoutHeader_BadMetadata_DoesNotBind proves the
// connect exemption is safe: letting account.connected through without the
// header does not let a forger bind, because handleAccountConnected still
// verifies the metadata signature. A connect body carrying a garbage token
// binds nothing (and answers 200 as a harmless, logged no-op).
func TestUnipile_Webhook_ConnectWithoutHeader_BadMetadata_DoesNotBind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newConnectFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	// account.connected shape, real account id, but a forged metadata token and
	// no Unipile-Auth header.
	body := loadConnectWebhook(t, f.unipileID, "not-a-valid-signed-token")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (a bad-metadata connect is a logged no-op)", rr.Code)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id = $1`,
		f.unipileID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("row count=%d want 0 (a forged connect must not bind)", n)
	}
}
