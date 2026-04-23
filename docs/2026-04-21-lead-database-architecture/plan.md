# Lead Database — Implementation Plan

**Date:** 2026-04-21
**Depends on:** `research.md` in this folder
**Goal:** Build MagikLead's proprietary lead database — the core product moat.

---

## Executor Notes

Each task is self-contained and can be picked up independently. Phase order matters: you can't ingest (Phase 2+) until the schema exists (Phase 1).

---

## Phase 1: Foundation (schema + infrastructure)

### T01: Add schema for canonical + source tables

**Files to modify:**
- `backend/migrations/013_canonical_schema.up.sql` (new)
- `backend/migrations/013_canonical_schema.down.sql` (new)

**Tables to create:**
```
sources              (id, name, type, api_endpoint, last_ingested_at, created_at)
raw_ingests          (id, source_id, file_url, ingested_at, checksum, size_bytes, meta jsonb)
source_records       (id, source_id, raw_ingest_id, external_id, fields jsonb, ingested_at)
organizations        (id, canonical_name, primary_domain, created_at, updated_at)
organization_aliases (id, organization_id, alias, alias_type)
organization_identifiers (id, organization_id, identifier_type, identifier_value, is_primary)
persons              (id, canonical_name, first_name, last_name, created_at, updated_at)
person_aliases       (id, person_id, alias, alias_type)
person_identifiers   (id, person_id, identifier_type, identifier_value, is_primary)
employments          (id, person_id, organization_id, title, start_date, end_date, is_current)
emails               (id, email, person_id, verified_at, verification_method, bounce_count, is_catchall, created_at)
phones               (id, phone, person_id, verified_at, created_at)
social_profiles      (id, person_id, platform, handle, url, created_at)
domains              (id, domain, mx_records text[], is_catchall, last_verified_at)
evidence             (id, canonical_table, canonical_id, source_record_id, confidence int2, conflict_flag bool, resolved_by_user_id, created_at)
deletion_blocklist   (id, identifier_hash, identifier_type, reason, created_at)
conflict_queue       (id, canonical_table, canonical_id, new_source_record_id, status, created_at)
merge_history        (id, surviving_id, merged_id, canonical_table, merged_by_user_id, reason, created_at)
```

**Indexes (critical for sub-second queries):**
- `pg_trgm` index on `persons.canonical_name`, `persons.first_name`, `persons.last_name`
- `pg_trgm` index on `organizations.canonical_name`, `organizations.primary_domain`
- `pg_trgm` index on `employments.title`
- BTREE + unique on `emails.email`, `person_identifiers.identifier_value`
- GIN on `source_records.fields`, `raw_ingests.meta`
- Partial index on `employments(person_id) WHERE is_current = true`

**Migration plan for existing `leads` table:**
- Keep `leads` for backward compat during migration
- Add `person_id` foreign key to `leads` (nullable)
- Write backfill job later

### T02: S3/MinIO setup for raw ingests

**Files to modify:**
- `backend/internal/storage/s3.go` (new)
- `backend/.env.example` (add S3_BUCKET, S3_REGION, S3_ENDPOINT, etc.)
- `docker-compose.yml` (add MinIO service for local dev)

**Interface:**
```go
type RawStorage interface {
    Upload(ctx, sourceSlug, fileName string, data io.Reader) (url string, err error)
    Download(ctx, url string) (io.ReadCloser, error)
    Checksum(ctx, url string) (string, error)
}
```

MinIO for local dev, Hetzner Object Storage (S3-compatible) or AWS S3 for prod.

### T03: Ingestion runner framework

**Files to modify:**
- `backend/internal/ingest/runner.go` (new) — orchestrator
- `backend/internal/ingest/types.go` (new) — Source interface
- `backend/internal/ingest/resolver.go` (new) — entity resolution / dedup
- `backend/cmd/ingest/main.go` (new) — CLI entrypoint

**Source interface:**
```go
type Source interface {
    Name() string
    Fetch(ctx, since time.Time) ([]RawBatch, error)   // returns chunks with checksum
    Parse(ctx, raw RawBatch) ([]SourceRecord, error)  // structured records
}
```

**Runner flow:**
```
1. source.Fetch → returns batches
2. For each batch:
   a. Upload to S3 → raw_ingests row
   b. source.Parse → []SourceRecord
   c. Insert into source_records
   d. For each record: call resolver.Resolve(record)
      → finds/creates canonical person + organization
      → creates evidence row
      → logs conflicts if any
```

**CLI:** `go run ./cmd/ingest/... sec-edgar --since 2026-01-01`

---

## Phase 2: First Real Source (SEC EDGAR)

### T04: SEC EDGAR source implementation

**Files to modify:**
- `backend/internal/ingest/sources/edgar.go` (new)

**What it does:**
- Fetches quarterly and annual filings with exec officers (10-K, DEF 14A)
- Parses XBRL/XML for officer names, titles, companies (CIK)
- Yields `SourceRecord` per officer

**Expected output:** ~500K exec-company-title tuples from US public companies.

### T05: Entity resolver — EDGAR pass

**Files to modify:**
- `backend/internal/ingest/resolver.go` (extend)

**Logic:**
1. Match organization by CIK (exact, 100% confidence → auto-merge)
2. Match person by name + company + title fuzzy (pg_trgm > 0.8 → auto-merge at ≥ 95%)
3. Create employment row
4. Emit evidence with confidence score

**No emails yet** — SEC filings don't include email addresses. Persons get stored, email field stays null.

---

## Phase 3: Second & Third Sources

### T06: Wikidata source (SPARQL)

**Files to modify:**
- `backend/internal/ingest/sources/wikidata.go` (new)

**Query pattern:**
```sparql
SELECT ?person ?personLabel ?company ?companyLabel ?title ?startDate WHERE {
  ?person wdt:P108 ?company.   # employer
  ?person wdt:P39 ?title.       # position held
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
LIMIT 100000
```

### T07: CrunchBase Open Data source

**Files to modify:**
- `backend/internal/ingest/sources/crunchbase.go` (new)

**What it does:**
- Downloads daily CSV dumps (they publish them free)
- Parses `people.csv` and `organizations.csv`
- Yields SourceRecords

### T08: Entity resolver — cross-source dedup

**Files to modify:**
- `backend/internal/ingest/resolver.go` (extend)

**Additions:**
- Match by LinkedIn URL across sources (exact = 100%)
- Match by (normalized name, company, overlapping dates) between sources
- Populate `person_aliases`, `organization_aliases` when names differ

---

## Phase 4: Common Crawl (optional, large-scale)

### T09: Common Crawl team-page extraction

**Files to modify:**
- `backend/internal/ingest/sources/commoncrawl.go` (new)
- `backend/internal/ingest/extractor.go` (new) — HTML → person list

**What it does:**
- Filters Common Crawl index to URLs like `*/about`, `*/team`, `*/leadership`
- Streams WARC files
- For each page: scrape HTML → Claude extraction → SourceRecord

**Caution:** Massive data volume. Start with a filtered subset (e.g., S&P 500 company domains).

---

## Phase 5: Email Verification Pipeline

### T10: Catch-all detection + SMTP verifier

**Files to modify:**
- `backend/internal/leads/smtp_verify.go` (new, replaces old smtp check)

**Logic:**
1. MX lookup → cache in `domains` table
2. Send random probe email (e.g., `xyz-random-9384@<domain>`)
3. If accepted → `is_catchall = true` → skip SMTP verification for this domain
4. If rejected → proceed with RCPT TO check for real address

### T11: Email ingestion job

**Files to modify:**
- `backend/cmd/verify-emails/main.go` (new)

**What it does:**
- For each person without email: nothing (we DO NOT guess)
- For each email from source: SMTP verify if not already done
- Update `emails.verified_at`, `verification_method`

---

## Phase 6: Search + Tenant Layer

### T12: Query API

**Files to modify:**
- `backend/internal/handler/leads_search.go` (new)

**Endpoint:**
```
POST /api/v1/leads/search
Body: {
  "titles": ["VP Sales", "Head of Sales"],
  "industries": ["SaaS"],
  "company_size": "100-500",
  "locations": ["United States"],
  "with_email": true,
  "limit": 50
}
```

**Implementation:**
- pg_trgm similarity on titles
- JOIN current employments + emails
- Only return persons with verified emails when `with_email=true`
- Response time target: < 500ms

### T13: Tenant leads table + migration

**Files to modify:**
- `backend/migrations/014_tenant_leads.up.sql` (new)

**New table:**
```
tenant_leads (
  tenant_id, person_id, status, added_at, notes,
  PRIMARY KEY (tenant_id, person_id)
)
```

**Backfill:**
- For each row in old `leads` table → resolve to canonical `person_id` → insert into `tenant_leads`
- Deprecate old `leads` table after backfill
- Keep `campaigns`, `campaign_leads` — but campaign_leads now references person_id via tenant_leads

---

## Phase 7: Admin Panel (conflict resolution)

### T14: Admin panel backend

**Files to modify:**
- `backend/internal/handler/admin.go` (new)
- `backend/internal/middleware/admin.go` (new — admin-only auth)

**Endpoints:**
```
GET  /api/v1/admin/conflicts?status=pending
POST /api/v1/admin/conflicts/:id/merge     (body: { keep_id, merge_into })
POST /api/v1/admin/conflicts/:id/reject
GET  /api/v1/admin/persons/:id              (shows all evidence + source records)
POST /api/v1/admin/persons/:id/delete       (GDPR-style hard delete)
```

### T15: Admin panel frontend

**Files to modify:**
- `frontend/src/app/(admin)/conflicts/page.tsx` (new)
- `frontend/src/app/(admin)/persons/[id]/page.tsx` (new)
- `frontend/src/app/(admin)/layout.tsx` (new — admin-only routing)

**UI:**
- Conflict queue list with filters
- Side-by-side diff view of conflicting records
- Merge button, reject button, create-new button
- Full person detail view with all evidence + raw sources

---

## Phase 8: GDPR Compliance

### T16: Deletion blocklist + right-to-erasure

**Files to modify:**
- `backend/internal/handler/privacy.go` (new)

**Endpoint:**
```
POST /api/v1/privacy/erasure
Body: { email, linkedin_url, name, company }
```

**Flow:**
1. Identify canonical person(s) matching the input
2. Hash identifiers (email, LinkedIn URL, name+company)
3. Insert hashes into `deletion_blocklist`
4. Delete person and all related rows (cascade)
5. Re-ingestion checks blocklist before creating new records

---

## Implementation Order (concrete next steps)

**This week:**
1. T01 — Schema migration
2. T02 — S3 setup (use MinIO in docker-compose for local)
3. T03 — Ingestion runner framework

**Next week:**
4. T04 — SEC EDGAR ingestion (first real data)
5. T05 — Entity resolver v1

**Week 3:**
6. T06, T07, T08 — Wikidata + CrunchBase + cross-source dedup

**Week 4:**
7. T10, T11 — Email verification pipeline
8. T12 — Query API
9. T13 — Tenant migration (replace old `leads` table)

**Week 5+:**
10. T14, T15 — Admin panel
11. T09 — Common Crawl (ambitious, lots of infra)
12. T16 — GDPR endpoints

---

## Success Metrics

- **Phase 1 done:** Ingestion runner can execute a toy source end-to-end and store evidence.
- **Phase 2 done:** 400K+ persons from SEC EDGAR with valid employments, dedup rate > 90%.
- **Phase 3 done:** 500K+ persons total, cross-source matches > 20% of new ingests.
- **Phase 5 done:** Email verification working, catch-all detection prevents the bounce problem we had.
- **Phase 6 done:** Customer search returns < 500ms for complex queries on 500K records.
- **Phase 7 done:** Admin can resolve a conflict in < 30 seconds.
