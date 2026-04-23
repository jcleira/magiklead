package sources

import (
	"context"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/ingest"
)

func TestWikidataParse(t *testing.T) {
	raw := []byte(`{
  "head": {"vars": ["person","personLabel","personCik","company","companyLabel","companyCik","companyWebsite","positionLabel"]},
  "results": {"bindings": [
    {
      "person":         {"type":"uri","value":"http://www.wikidata.org/entity/Q267"},
      "personLabel":    {"type":"literal","value":"Tim Cook"},
      "personCik":      {"type":"literal","value":"0001214156"},
      "company":        {"type":"uri","value":"http://www.wikidata.org/entity/Q312"},
      "companyLabel":   {"type":"literal","value":"Apple Inc."},
      "companyCik":     {"type":"literal","value":"0000320193"},
      "companyWebsite": {"type":"literal","value":"https://www.apple.com/"},
      "positionLabel":  {"type":"literal","value":"chief executive officer"}
    },
    {
      "person":       {"type":"uri","value":"http://www.wikidata.org/entity/Q9999999"},
      "personLabel":  {"type":"literal","value":"Q9999999"},
      "personCik":    {"type":"literal","value":"0999999999"}
    }
  ]}
}`)
	src := &WikidataSource{}
	records, err := src.Parse(context.Background(), ingest.RawBatch{Content: raw})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 usable record (Q-ID-only label skipped), got %d", len(records))
	}
	r := records[0]
	if r.ExternalID != "Q267" {
		t.Errorf("ExternalID=%q, want Q267", r.ExternalID)
	}
	assertField(t, r.Fields, "name", "Tim Cook")
	assertField(t, r.Fields, "wikidata_id", "Q267")
	assertField(t, r.Fields, "person_cik", "0001214156")
	assertField(t, r.Fields, "company", "Apple Inc.")
	assertField(t, r.Fields, "issuer_cik", "0000320193")
	assertField(t, r.Fields, "company_domain", "apple.com")
	assertField(t, r.Fields, "title", "chief executive officer")
}

func TestDomainFromURL(t *testing.T) {
	tests := map[string]string{
		"https://www.apple.com/":                     "apple.com",
		"https://apple.com":                          "apple.com",
		"http://example.co.uk/path?q=1":              "example.co.uk",
		"https://WWW.Alphabet.COM/":                  "alphabet.com",
		"":                                           "",
		"not a url":                                  "",
	}
	for in, want := range tests {
		if got := domainFromURL(in); got != want {
			t.Errorf("domainFromURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLooksLikeQID(t *testing.T) {
	for _, s := range []string{"Q267", "Q1", "q9999"} {
		if !looksLikeQID(s) {
			t.Errorf("looksLikeQID(%q)=false, want true", s)
		}
	}
	for _, s := range []string{"", "Q", "Q12a", "Tim Cook"} {
		if looksLikeQID(s) {
			t.Errorf("looksLikeQID(%q)=true, want false", s)
		}
	}
}
