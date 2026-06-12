package pdl

import (
	"encoding/json"
	"testing"
)

// PDL's free tier obfuscates contact fields: work_email / emails come
// back as a boolean presence flag (true = "a value exists you can't
// see", false = none) instead of the value. The wire structs must
// tolerate either shape — a single boolean field must not 502 the whole
// search. This was confirmed live: a free-key search returned
// `cannot unmarshal bool into Go struct field pdlPerson.data.work_email`.
func TestDecodeFreeTierContact_DoesNotError(t *testing.T) {
	// Every field PDL's free tier obfuscates as a boolean among those we
	// parse: work_email, emails, AND location_name. A single one of these
	// as a bool must not fail the whole decode. Verified data-driven
	// against an actual free-tier record.
	var p pdlPerson
	body := `{"id":"x","full_name":"joseph brown","job_title":"eng","location_name":true,"work_email":true,"emails":true}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("free-tier PDL person must decode, got: %v", err)
	}
	if p.JobTitle != "eng" || p.FullName != "joseph brown" {
		t.Errorf("string fields corrupted: title=%q name=%q", p.JobTitle, p.FullName)
	}
	if p.LocationName.Value != "" {
		t.Errorf("gated location_name should yield empty Value, got %q", p.LocationName.Value)
	}
}

// Free tier: contact fields are booleans. presence = true, but there is
// no address to send to — so hasEmail is true and pickEmail is empty.
func TestDecodeFreeTier_PresentButNoAddress(t *testing.T) {
	var p pdlPerson
	body := `{"id":"x","full_name":"joseph brown","work_email":true,"emails":true,"mobile_phone":true}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasEmail(p) {
		t.Error("hasEmail = false, want true (free-tier presence flag is true)")
	}
	if got := pickEmail(p); got != "" {
		t.Errorf("pickEmail = %q, want empty (no address visible on free tier)", got)
	}
}

// Paid tier: work_email is the real string → hasEmail true, pickEmail
// returns the lowercased address.
func TestDecodePaidTier_RealAddress(t *testing.T) {
	var p pdlPerson
	body := `{"id":"x","full_name":"a b","work_email":"Joe@Acme.com","emails":[{"address":"j@acme.com","type":"work"}]}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasEmail(p) {
		t.Error("hasEmail = false, want true")
	}
	if got := pickEmail(p); got != "joe@acme.com" {
		t.Errorf("pickEmail = %q, want joe@acme.com", got)
	}
}

// emails array present but work_email absent → still emailable, picks
// the first array address.
func TestDecode_EmailsArrayOnly(t *testing.T) {
	var p pdlPerson
	body := `{"id":"x","full_name":"a b","emails":[{"address":"only@acme.com","type":"personal"}]}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasEmail(p) {
		t.Error("hasEmail = false, want true (emails array non-empty)")
	}
	if got := pickEmail(p); got != "only@acme.com" {
		t.Errorf("pickEmail = %q, want only@acme.com", got)
	}
}

// No email at all: free tier signals false. hasEmail false, pickEmail
// empty — these are correctly filtered out by with_email.
func TestDecode_NoEmail(t *testing.T) {
	for _, body := range []string{
		`{"id":"x","full_name":"a b","work_email":false,"emails":false}`,
		`{"id":"x","full_name":"a b"}`,
		`{"id":"x","full_name":"a b","work_email":null,"emails":null}`,
	} {
		var p pdlPerson
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		if hasEmail(p) {
			t.Errorf("hasEmail = true for %s, want false", body)
		}
		if got := pickEmail(p); got != "" {
			t.Errorf("pickEmail = %q for %s, want empty", got, body)
		}
	}
}
