package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

func TestNormalizeTitles(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"trims whitespace", []string{"  VP Sales  "}, []string{"VP Sales"}},
		{"drops empties", []string{"", "  ", "Head of Sales"}, []string{"Head of Sales"}},
		{"all-empty becomes nil", []string{"", "  "}, nil},
		{"nil in nil out", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeTitles(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d]=%q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestToSearchResult(t *testing.T) {
	person := repository.SearchPersonsRow{
		PersonID:            pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		PersonCanonicalName: "Tim Cook",
		FirstName:           pgtype.Text{String: "Tim", Valid: true},
		LastName:            pgtype.Text{String: "Cook", Valid: true},
		Title:               pgtype.Text{String: "CEO", Valid: true},
		OrganizationID:      pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
		OrganizationName:    "Apple Inc",
		PrimaryDomain:       pgtype.Text{String: "apple.com", Valid: true},
		TitleScore:          0.92,
	}
	email := repository.ListBestEmailsForPersonsRow{
		PersonID:   person.PersonID,
		Email:      "tim@apple.com",
		VerifiedAt: pgtype.Timestamptz{Valid: true},
		IsCatchall: pgtype.Bool{Bool: false, Valid: true},
	}

	got := toSearchResult(person, email)

	if got.Name != "Tim Cook" {
		t.Errorf("Name=%q", got.Name)
	}
	if got.FirstName == nil || *got.FirstName != "Tim" {
		t.Errorf("FirstName=%v", got.FirstName)
	}
	if got.Email == nil || *got.Email != "tim@apple.com" {
		t.Errorf("Email=%v", got.Email)
	}
	if !got.EmailVerified {
		t.Errorf("EmailVerified should be true")
	}
	if got.EmailIsCatchall {
		t.Errorf("EmailIsCatchall should be false")
	}
	if got.TitleScore == nil || *got.TitleScore != 0.92 {
		t.Errorf("TitleScore=%v", got.TitleScore)
	}
}

func TestToSearchResult_NoEmail(t *testing.T) {
	person := repository.SearchPersonsRow{
		PersonID:            pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
		PersonCanonicalName: "Anonymous Founder",
		OrganizationID:      pgtype.UUID{Bytes: [16]byte{4}, Valid: true},
		OrganizationName:    "Stealth Co",
	}

	got := toSearchResult(person, repository.ListBestEmailsForPersonsRow{})

	if got.Email != nil {
		t.Errorf("Email should be nil, got %v", *got.Email)
	}
	if got.EmailVerified {
		t.Errorf("EmailVerified should default to false")
	}
	if got.FirstName != nil || got.LastName != nil || got.Title != nil || got.Domain != nil {
		t.Errorf("nullable fields should be nil")
	}
}

func TestSearch_RejectsUnsupportedFilters(t *testing.T) {
	cases := map[string]leadSearchRequest{
		"industries":   {Industries: []string{"SaaS"}},
		"company_size": {CompanySize: "100-500"},
		"locations":    {Locations: []string{"United States"}},
	}
	h := &LeadSearchHandler{}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			buf, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search", strings.NewReader(string(buf)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.Search(rr, req)

			if rr.Code != http.StatusNotImplemented {
				t.Errorf("status=%d want %d", rr.Code, http.StatusNotImplemented)
			}
			var resp map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp["code"] != "filter_unsupported" {
				t.Errorf("code=%q", resp["code"])
			}
		})
	}
}

func TestSearch_BadJSON(t *testing.T) {
	h := &LeadSearchHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search", strings.NewReader("not json"))
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}
