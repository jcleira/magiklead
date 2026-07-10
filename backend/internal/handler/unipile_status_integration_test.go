//go:build integration

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// statusFixture is the minimal graph the account-status + reconnect webhooks
// (issue #8) act on: a tenant with one connected linkedin_account. Both
// webhooks match the account by its Unipile id, so no campaign/lead graph is
// needed.
//
// Run with:
//
//	devpods exec api go test -tags=integration ./internal/handler/...
type statusFixture struct {
	tenantID  uuid.UUID
	accountID uuid.UUID
	unipileID string
}

func newStatusFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) statusFixture {
	t.Helper()
	run := uuid.NewString()
	f := statusFixture{
		tenantID:  uuid.New(),
		accountID: uuid.New(),
		unipileID: "acc_status_" + run,
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`, f.tenantID, "Status Test "+run)
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		f.accountID, f.tenantID, f.unipileID)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, f.accountID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
	})
	return f
}

// signedStatusWebhook builds an authenticated account-status request the
// real module parses end to end. providerStatus is Unipile's raw status
// string (e.g. ERROR, DISCONNECTED) carried in the AccountStatus block.
// Auth is the static Unipile-Auth header (== the webhook secret).
func signedStatusWebhook(t *testing.T, _ *unipile.Module, unipileAccountID, providerStatus string) *http.Request {
	t.Helper()
	body := []byte(`{"AccountStatus":{"account_id":"` + unipileAccountID + `","account_type":"LINKEDIN","message":"` + providerStatus + `"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	return req
}

// signMeta mints the signed tenant/user metadata token AuthURL embeds in
// the hosted-auth link and the account.connected webhook decodes. It lives
// here because the reconnect test (a connect-shaped webhook) is its only
// user now; #3's connect-and-bind slice folds it into its own fixture.
func signMeta(t *testing.T, secret []byte, tenantID, userID uuid.UUID, exp time.Time) string {
	t.Helper()
	s, err := jwt.Encode(jwt.Claims{TenantID: tenantID.String(), UserID: userID.String(), Exp: exp.Unix()}, secret)
	if err != nil {
		t.Fatalf("sign meta: %v", err)
	}
	return s
}

// TestUnipile_Webhook_AccountStatus_RestrictsAndIsIdempotent is the
// status-detection tracer (issue #8) through every layer: a signed
// account-status webhook carrying a restriction is verified, parsed, mapped
// to our enum, and persisted — the account flips to 'restricted' with the
// provider reason recorded in last_error. Delivering it twice leaves the same
// terminal state (the UPDATE is idempotent). Asserts via DB.
func TestUnipile_Webhook_AccountStatus_RestrictsAndIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newStatusFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.Webhook(rr, signedStatusWebhook(t, mod, f.unipileID, "ERROR"))
		if rr.Code != http.StatusOK {
			t.Fatalf("delivery %d status=%d want 200 body=%s", i, rr.Code, rr.Body.String())
		}
	}

	var status string
	var lastErr pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("scan account: %v", err)
	}
	if status != "restricted" {
		t.Errorf("status=%q want restricted", status)
	}
	if !lastErr.Valid || lastErr.String != "ERROR" {
		t.Errorf("last_error=%+v want the provider status 'ERROR' recorded", lastErr)
	}
}

// TestUnipile_Webhook_AccountStatus_Disconnects proves the disconnect arm of
// the same detection: a DISCONNECTED status webhook flips the account to
// 'disconnected' and records the reason in last_error so Settings can explain
// why and prompt a reconnect. Asserts via DB.
func TestUnipile_Webhook_AccountStatus_Disconnects(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newStatusFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	rr := httptest.NewRecorder()
	h.Webhook(rr, signedStatusWebhook(t, mod, f.unipileID, "DISCONNECTED"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	var status string
	var lastErr pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("scan account: %v", err)
	}
	if status != "disconnected" {
		t.Errorf("status=%q want disconnected", status)
	}
	if !lastErr.Valid || lastErr.String != "DISCONNECTED" {
		t.Errorf("last_error=%+v want the provider status 'DISCONNECTED' recorded", lastErr)
	}
}

// TestUnipile_Webhook_Reconnect_ClearsRestricted proves the reconnect path
// (issue #8 → the issue #1 flow): a signed account.connected webhook for an
// account currently restricted upserts it back to 'active' and clears the
// recorded last_error, so the send ticks resume it. Asserts via DB.
func TestUnipile_Webhook_Reconnect_ClearsRestricted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newStatusFixture(t, ctx, pool)

	// Start restricted with a recorded error — the state a send-error
	// escalation or a status webhook leaves behind.
	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET status = 'restricted', last_error = 'ERROR' WHERE id = $1`, f.accountID); err != nil {
		t.Fatalf("seed restricted: %v", err)
	}

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	// account.connected for the same Unipile id hits the upsert's ON CONFLICT
	// branch: status → active, last_error → NULL. The signed metadata carries
	// the (unchanged) tenant binding the webhook recovers.
	meta := signMeta(t, secret, f.tenantID, uuid.New(), time.Now().Add(10*time.Minute))
	body := []byte(`{"status":"CREATION_SUCCESS","account_id":"` + f.unipileID + `","name":"` + meta + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	var status string
	var lastErr pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("scan account: %v", err)
	}
	if status != "active" {
		t.Errorf("status=%q want active (reconnected)", status)
	}
	if lastErr.Valid {
		t.Errorf("last_error=%q want NULL after reconnect", lastErr.String)
	}
}

// TestUnipile_Webhook_AccountStatus_RecoveryReactivates proves the recovery
// arm of status detection via the *status* path — distinct from the reconnect
// path above, which arrives as an account.connected webhook carrying metadata.
// A restricted account that receives an active-mapping status webhook flips
// back to 'active' and clears the recorded last_error, so the send ticks
// resume it (US 23). Driven with CREATION_SUCCESS: the one status value the #1
// spike actually captured, and the value a hosted-auth reconnect re-emits.
// Asserts via DB.
func TestUnipile_Webhook_AccountStatus_RecoveryReactivates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newStatusFixture(t, ctx, pool)

	// Start restricted with a recorded reason — the state a prior restriction
	// (send-error escalation or a status webhook) leaves behind.
	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET status = 'restricted', last_error = 'ERROR' WHERE id = $1`, f.accountID); err != nil {
		t.Fatalf("seed restricted: %v", err)
	}

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	rr := httptest.NewRecorder()
	h.Webhook(rr, signedStatusWebhook(t, mod, f.unipileID, "CREATION_SUCCESS"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	var status string
	var lastErr pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("scan account: %v", err)
	}
	if status != "active" {
		t.Errorf("status=%q want active (recovered)", status)
	}
	if lastErr.Valid {
		t.Errorf("last_error=%q want NULL after recovery", lastErr.String)
	}
}

// TestUnipile_Webhook_AccountStatus_UnknownStatusNoClobber proves the
// no-clobber contract (issue #4): a status webhook carrying a value outside
// the known vocabulary leaves the stored account state exactly as it was,
// rather than guessing a mapping. Seeded restricted with a recorded reason —
// the meaningful direction, since silently clearing a real restriction would
// wrongly resume sending. The webhook still answers 200 (so Unipile stops
// retrying) but touches no row. Asserts via DB.
func TestUnipile_Webhook_AccountStatus_UnknownStatusNoClobber(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newStatusFixture(t, ctx, pool)

	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET status = 'restricted', last_error = 'ERROR' WHERE id = $1`, f.accountID); err != nil {
		t.Fatalf("seed restricted: %v", err)
	}

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	rr := httptest.NewRecorder()
	h.Webhook(rr, signedStatusWebhook(t, mod, f.unipileID, "SOME_UNKNOWN_STATUS"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (unknown status still 200s so Unipile stops retrying) body=%s", rr.Code, rr.Body.String())
	}

	var status string
	var lastErr pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("scan account: %v", err)
	}
	if status != "restricted" {
		t.Errorf("status=%q want restricted (unknown status must not clobber)", status)
	}
	if !lastErr.Valid || lastErr.String != "ERROR" {
		t.Errorf("last_error=%+v want the original 'ERROR' preserved (no clobber)", lastErr)
	}
}

// TestUnipile_Webhook_AccountStatus_UnknownAccountNotCreated proves a status
// webhook never *creates* an account (creation is connect, issue #3): the
// status path is an UPDATE keyed on unipile_account_id, so a status for an id
// we've never bound updates zero rows and writes nothing. It still answers 200
// so Unipile stops retrying. Asserts no row exists for the unknown id via DB.
func TestUnipile_Webhook_AccountStatus_UnknownAccountNotCreated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)

	unknownID := "acc_never_bound_" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM linkedin_accounts WHERE unipile_account_id = $1`, unknownID)
	})

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	rr := httptest.NewRecorder()
	h.Webhook(rr, signedStatusWebhook(t, mod, unknownID, "ERROR"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM linkedin_accounts WHERE unipile_account_id = $1`, unknownID).Scan(&count); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if count != 0 {
		t.Errorf("account rows for unknown id = %d want 0 (a status must not create an account)", count)
	}
}
