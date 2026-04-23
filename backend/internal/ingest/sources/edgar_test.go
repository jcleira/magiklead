package sources

import (
	"archive/zip"
	"bytes"
	"testing"
	"time"
)

func TestParseInsiderZip_ExecutiveYielded(t *testing.T) {
	zipData := buildFakeInsiderZip(t, [][]string{
		// SUBMISSION.tsv
		{"ACCESSION_NUMBER\tFILING_DATE\tDOCUMENT_TYPE\tISSUERCIK\tISSUERNAME\tISSUERTRADINGSYMBOL"},
		{"0000320193-26-000001\t2026-01-15\t4\t0000320193\tAPPLE INC\tAAPL"},
	}, [][]string{
		// REPORTINGOWNER.tsv
		{"ACCESSION_NUMBER\tRPTOWNERCIK\tRPTOWNERNAME\tIS_OFFICER\tIS_DIRECTOR\tOFFICER_TITLE"},
		{"0000320193-26-000001\t0001214156\tCook, Timothy D.\t1\t0\tCEO"},
	})

	records, err := parseInsiderZip(zipData, 0)
	if err != nil {
		t.Fatalf("parseInsiderZip: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	r := records[0]
	if r.ExternalID != "0000320193-26-000001:0001214156" {
		t.Errorf("ExternalID=%q", r.ExternalID)
	}
	assertField(t, r.Fields, "name", "Cook, Timothy D.")
	assertField(t, r.Fields, "title", "CEO")
	assertField(t, r.Fields, "company", "APPLE INC")
	assertField(t, r.Fields, "issuer_cik", "0000320193")
	assertField(t, r.Fields, "issuer_ticker", "AAPL")
	assertField(t, r.Fields, "person_cik", "0001214156")
	assertField(t, r.Fields, "filing_date", "2026-01-15")
	assertField(t, r.Fields, "document_type", "4")
}

func TestParseInsiderZip_SkipNonOfficers(t *testing.T) {
	zipData := buildFakeInsiderZip(t, [][]string{
		{"ACCESSION_NUMBER\tFILING_DATE\tDOCUMENT_TYPE\tISSUERCIK\tISSUERNAME\tISSUERTRADINGSYMBOL"},
		{"A-1\t2026-01-15\t4\t0001\tACME INC\tACME"},
		{"A-2\t2026-01-15\t4\t0001\tACME INC\tACME"},
	}, [][]string{
		{"ACCESSION_NUMBER\tRPTOWNERCIK\tRPTOWNERNAME\tIS_OFFICER\tIS_DIRECTOR\tOFFICER_TITLE"},
		// Director only (no title) — skip.
		{"A-1\t1001\tDoe, Jane\t0\t1\t"},
		// 10%-owner masquerading as officer=false — skip.
		{"A-2\t1002\tFund, Big\t0\t0\t"},
	})

	records, err := parseInsiderZip(zipData, 0)
	if err != nil {
		t.Fatalf("parseInsiderZip: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestParseInsiderZip_RespectsMaxRecords(t *testing.T) {
	subHeader := "ACCESSION_NUMBER\tFILING_DATE\tDOCUMENT_TYPE\tISSUERCIK\tISSUERNAME\tISSUERTRADINGSYMBOL"
	ownHeader := "ACCESSION_NUMBER\tRPTOWNERCIK\tRPTOWNERNAME\tIS_OFFICER\tOFFICER_TITLE"

	var subRows [][]string
	var ownRows [][]string
	subRows = append(subRows, []string{subHeader})
	ownRows = append(ownRows, []string{ownHeader})
	for i := 0; i < 10; i++ {
		acc := "A-" + string(rune('0'+i))
		subRows = append(subRows, []string{acc + "\t2026-01-01\t4\t0001\tACME INC\tACME"})
		ownRows = append(ownRows, []string{acc + "\t9" + string(rune('0'+i)) + "\tOfficer " + string(rune('A'+i)) + "\t1\tCEO"})
	}

	zipData := buildFakeInsiderZip(t, subRows, ownRows)

	records, err := parseInsiderZip(zipData, 3)
	if err != nil {
		t.Fatalf("parseInsiderZip: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("max=3, got %d", len(records))
	}
}

func TestQuartersSince(t *testing.T) {
	now := time.Date(2026, time.April, 21, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		since time.Time
		want  []quarter
	}{
		{
			name:  "zero since -> current quarter only",
			since: time.Time{},
			want:  []quarter{{Year: 2026, Quarter: 2}},
		},
		{
			name:  "two quarters ago",
			since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			want:  []quarter{{Year: 2026, Quarter: 1}, {Year: 2026, Quarter: 2}},
		},
		{
			name:  "crosses year boundary",
			since: time.Date(2025, time.October, 1, 0, 0, 0, 0, time.UTC),
			want: []quarter{
				{Year: 2025, Quarter: 4},
				{Year: 2026, Quarter: 1},
				{Year: 2026, Quarter: 2},
			},
		},
		{
			name:  "since in the future",
			since: time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quartersSince(tc.since, now)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("quarter[%d]: got %v want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func buildFakeInsiderZip(t *testing.T, submission, owners [][]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	write := func(name string, rows [][]string) {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		for _, row := range rows {
			for _, line := range row {
				if _, err := f.Write([]byte(line + "\n")); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
		}
	}
	write(submissionTSV, submission)
	write(reportingOwnerTSV, owners)
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func assertField(t *testing.T, fields map[string]any, key, want string) {
	t.Helper()
	got, _ := fields[key].(string)
	if got != want {
		t.Errorf("fields[%q]=%q, want %q", key, got, want)
	}
}
