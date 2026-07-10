package unipile_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
)

// resolveBody is the frozen §6 resolve-response shape (spike-captures.md): a
// bare UserProfile object. The member id is provider_id (ACoAA…); member_urn
// here is numeric and must NOT be used.
const resolveBody = `{
  "object": "UserProfile",
  "provider": "LINKEDIN",
  "provider_id": "ACoAABExample00042",
  "public_identifier": "ada-lovelace",
  "member_urn": "119375570",
  "first_name": "Ada",
  "last_name": "Lovelace",
  "headline": "Analytical Engine enthusiast"
}`

func TestResolveMemberID_ReturnsProviderID(t *testing.T) {
	s := newStub(t, 200, resolveBody)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	got, err := m.ResolveMemberID(context.Background(),
		"https://www.linkedin.com/in/ada-lovelace/", "acc_1")
	if err != nil {
		t.Fatalf("ResolveMemberID: %v", err)
	}
	if got != "ACoAABExample00042" {
		t.Fatalf("member id = %q, want provider_id ACoAABExample00042 (not member_urn)", got)
	}
	if s.method != "GET" {
		t.Errorf("method = %q, want GET", s.method)
	}
	if s.path != "/api/v1/users/ada-lovelace" {
		t.Errorf("path = %q, want /api/v1/users/ada-lovelace", s.path)
	}
	if !strings.Contains(s.rawQuery, "account_id=acc_1") {
		t.Errorf("query = %q, want account_id=acc_1", s.rawQuery)
	}
}

// The op takes a full profile URL and addresses Unipile by the public
// identifier (the /in/<slug> segment), tolerating trailing slashes, query
// strings, and the www/scheme variations real linkedin_url values carry.
func TestResolveMemberID_ExtractsPublicIdentifier(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://www.linkedin.com/in/ada-lovelace/", "/api/v1/users/ada-lovelace"},
		{"https://www.linkedin.com/in/ada-lovelace", "/api/v1/users/ada-lovelace"},
		{"https://linkedin.com/in/john-doe-123/?utm=x", "/api/v1/users/john-doe-123"},
		{"http://www.linkedin.com/in/grace-hopper", "/api/v1/users/grace-hopper"},
	}
	for _, tc := range cases {
		s := newStub(t, 200, resolveBody)
		m := unipile.New("key", s.URL, nil, s.Client())
		m.SetBaseURL(s.URL)
		if _, err := m.ResolveMemberID(context.Background(), tc.url, "acc_1"); err != nil {
			t.Fatalf("ResolveMemberID(%q): %v", tc.url, err)
		}
		if s.path != tc.want {
			t.Errorf("url %q → path %q, want %q", tc.url, s.path, tc.want)
		}
	}
}

func TestResolveMemberID_ClassifiesErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantErr   error
		permanent bool
	}{
		{"not found is permanent", 404, `{"type":"errors/no_user","title":"not found"}`, unipile.ErrNotFound, true},
		{"rate limited is transient", 429, `{"title":"slow down"}`, unipile.ErrRateLimited, false},
		{"upstream 5xx is transient", 502, `bad gateway`, unipile.ErrUpstream, false},
		{"empty provider_id is permanent", 200, `{"object":"UserProfile","provider_id":""}`, unipile.ErrNotFound, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, tc.status, tc.body)
			m := unipile.New("key", s.URL, nil, s.Client())
			m.SetBaseURL(s.URL)

			_, err := m.ResolveMemberID(context.Background(),
				"https://www.linkedin.com/in/ada-lovelace/", "acc_1")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErr)
			}
			if got := unipile.IsPermanentResolveError(err); got != tc.permanent {
				t.Errorf("IsPermanentResolveError(%v) = %v, want %v", err, got, tc.permanent)
			}
		})
	}
}

// A blank or malformed profile URL yields no public identifier — a permanent
// failure, since there is nothing to resolve and never will be.
func TestResolveMemberID_UnparseableURLIsPermanent(t *testing.T) {
	s := newStub(t, 200, resolveBody)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	_, err := m.ResolveMemberID(context.Background(), "not-a-linkedin-url", "acc_1")
	if !unipile.IsPermanentResolveError(err) {
		t.Fatalf("err = %v, want permanent (ErrNotFound)", err)
	}
}
