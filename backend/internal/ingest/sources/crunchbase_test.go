package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/ingest"
)

func TestCrunchbaseParse(t *testing.T) {
	zipData := buildCrunchbaseZip(t,
		// people.csv
		[]string{"uuid,name,first_name,last_name,linkedin_url,twitter_url",
			"p-1,Tim Cook,Tim,Cook,https://www.linkedin.com/in/tim-cook-123,https://twitter.com/tim_cook",
			"p-2,Satya Nadella,Satya,Nadella,,"},
		// organizations.csv
		[]string{"uuid,name,domain,homepage_url",
			"o-1,Apple,apple.com,https://www.apple.com/",
			"o-2,Microsoft,,https://www.microsoft.com/"},
		// jobs.csv
		[]string{"uuid,person_uuid,org_uuid,title,started_on,is_current",
			"j-1,p-1,o-1,Chief Executive Officer,2011-08-24,true",
			"j-2,p-2,o-2,Chairman and CEO,2021-06-16,true",
			// Orphan — no matching org.
			"j-3,p-1,o-missing,Visiting Lecturer,2020-01-01,false"})

	src := &CrunchbaseSource{}
	records, err := src.Parse(context.Background(), ingest.RawBatch{Content: zipData})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records (orphan skipped), got %d", len(records))
	}

	byName := map[string]ingest.SourceRecord{}
	for _, r := range records {
		byName[r.Fields["name"].(string)] = r
	}

	tim, ok := byName["Tim Cook"]
	if !ok {
		t.Fatal("Tim Cook missing")
	}
	assertField(t, tim.Fields, "title", "Chief Executive Officer")
	assertField(t, tim.Fields, "company", "Apple")
	assertField(t, tim.Fields, "company_domain", "apple.com")
	assertField(t, tim.Fields, "linkedin_url", "https://www.linkedin.com/in/tim-cook-123")
	assertField(t, tim.Fields, "start_date", "2011-08-24")
	if tim.Fields["is_current"] != true {
		t.Errorf("is_current=%v", tim.Fields["is_current"])
	}

	satya, ok := byName["Satya Nadella"]
	if !ok {
		t.Fatal("Satya Nadella missing")
	}
	// Missing domain should fall back to homepage_url.
	assertField(t, satya.Fields, "company_domain", "microsoft.com")
	if _, present := satya.Fields["linkedin_url"]; present {
		t.Error("linkedin_url should be absent for Satya (empty in source)")
	}
}

func TestCrunchbaseParse_SkipsRowsWithoutName(t *testing.T) {
	zipData := buildCrunchbaseZip(t,
		[]string{"uuid,name,first_name,last_name", "p-1,,,,"},
		[]string{"uuid,name,domain", "o-1,Empty,empty.com"},
		[]string{"uuid,person_uuid,org_uuid,title", "j-1,p-1,o-1,Ghost"})

	src := &CrunchbaseSource{}
	records, err := src.Parse(context.Background(), ingest.RawBatch{Content: zipData})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records (person has no name), got %d", len(records))
	}
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func buildCrunchbaseZip(t *testing.T, peopleRows, orgsRows, jobsRows []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	write := func(name string, rows []string) {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		for _, row := range rows {
			if _, err := f.Write([]byte(row + "\n")); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}
	write("people.csv", peopleRows)
	write("organizations.csv", orgsRows)
	write("jobs.csv", jobsRows)
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
