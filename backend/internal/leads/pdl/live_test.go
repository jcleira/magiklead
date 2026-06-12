//go:build integration

package pdl_test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/leads/pdl"
)

// TestLive_PDLSearch hits the real PDL API when PDL_API_KEY is set
// and is skipped otherwise. Same skip-pattern as
// storage/s3_roundtrip_test.go. Costs real PDL credits per run; the
// stdout log line gives the operator a paper trail.
//
// Run with:
//   PDL_API_KEY=… devpods exec api go test -tags=integration -v \
//     -run TestLive_PDLSearch ./internal/leads/pdl/...
func TestLive_PDLSearch(t *testing.T) {
	apiKey := os.Getenv("PDL_API_KEY")
	if apiKey == "" {
		t.Skip("PDL_API_KEY not set; skipping live PDL integration test")
	}
	pool := withPool(t)

	m := pdl.New(pool, apiKey, nil)
	results, err := m.Search(context.Background(), pdl.Filters{
		Titles:     []string{"vp marketing"},
		Industries: []string{"software"},
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("live Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("live Search returned 0 results — possible coverage gap; widen filters and retry")
	}

	// Operator-readable cost log so credits-consumed shows up in the
	// CI artefact / terminal scroll-back rather than only inside
	// PDL's dashboard. One Search call ≈ N credits (one per result
	// at the time of writing — confirm with PDL pricing).
	log.Printf("PDL live test: %d persons returned (≈ %d credits consumed)",
		len(results), len(results))
	for i, p := range results {
		log.Printf("  [%d] %s @ %s (email=%q verified=%v)",
			i, p.Name, p.OrganizationName, p.Email, p.EmailVerified)
	}
}
