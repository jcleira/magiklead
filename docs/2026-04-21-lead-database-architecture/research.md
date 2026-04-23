# Lead Database Architecture — Research

**Date:** 2026-04-21
**Purpose:** Design a multi-source B2B lead database that is MagikLead's core IP. This database is the product's moat — every paid API we're evaluating is ultimately a wrapper over a proprietary DB built this way.

---

## Product Decisions (confirmed)

| # | Question | Answer |
|---|----------|--------|
| 1 | Multi-tenant scope | **Unified database** shared across all tenants. Every tenant benefits from data every other tenant generates. GDPR deferred. |
| 2 | Historical data | **Keep full employment history.** Tim Cook's job at IBM in 1998 is preserved forever. |
| 3 | GDPR deletion | Person requests deletion → **hard delete** + blocklist to prevent re-ingestion. User deletes account → **unlink tenant from leads only**, canonical data survives. |
| 4 | Identity coverage | **Any identifier** — email, phone, LinkedIn URL, Twitter handle, GitHub, personal website. Schema must be extensible. |
| 5 | Emails without source | Store the **person** but never invent/guess emails. Emails must come from a real source. |
| 6 | Conflict resolution | **Manual review UI** for conflict resolution (Wikidata says 1998, LinkedIn says 2000). |
| 7 | Query performance | **Sub-second** query response required for all customer searches. |
| 8 | Update frequency | Continuous — database is the masterpiece, updated daily. |

---

## Core Principles

1. **Canonical model, multi-source provenance.** One record per real-world entity (person/company), with evidence pointing back to every source that mentions them.

2. **Raw ingests preserved forever.** Every CSV, API response, scraped HTML file is stored in object storage (S3 or equivalent). We can reprocess from scratch at any time.

3. **Emails are never guessed.** They come from a source (EDGAR, a leaked DB, a scraped page, a verified SMTP response to a signup). Pattern-guessed emails have caused bounces — this is an anti-pattern.

4. **Sub-second queries are a hard requirement.** This shapes the architecture: denormalized search tables, proper indexing, possibly a dedicated search layer (Typesense or pg_trgm).

5. **Tenant data is separate from canonical data.** Canonical persons/orgs are shared; tenant lists, campaigns, imports, and relationships are scoped per tenant.

---

## Data Sources (ranked by quality + cost)

### Tier 1: Free Structured (start here)

| Source | Type | Records | Data Quality | Notes |
|--------|------|---------|--------------|-------|
| **SEC EDGAR** | API + Bulk | ~500K execs at US public companies | High | Real execs, titles, dates — from legal filings. Limited to public companies. |
| **Wikidata** | SPARQL | ~100K business people with employment data | High | Structured, includes dates, multiple companies |
| **OpenCorporates** | API (paid above free tier) | 200M+ companies, directors | Medium | Many shell companies, noisy |
| **CrunchBase Open Data** | CSV daily | ~1M execs, founders | High | Free daily dump of key data |
| **GitHub API** | API | 100M+ dev profiles | Medium | Developers only, self-reported |
| **Product Hunt** | API | Makers + companies | Medium | Tech startup founders |
| **AngelList** | API | Startup profiles | Medium | Tech startup execs |

### Tier 2: Free Unstructured (needs processing)

| Source | Type | Records | Notes |
|--------|------|---------|-------|
| **Common Crawl** | HTML dumps | 250B+ pages / crawl | Cheap bandwidth, need to extract team pages |
| **Company websites** | Targeted scraping | Depends on crawl | What our current engine does |
| **Press releases** | PR Newswire, BusinessWire | Millions | Execs quoted with titles |
| **Podcast metadata** | Apple Podcasts, Spotify | Guests = real people | Show notes contain names+titles |
| **Conference speaker pages** | Targeted scraping | Industry-specific | High-quality, niche |

### Tier 3: Kaggle Datasets

| Dataset | Size | Quality | Verdict |
|---------|------|---------|---------|
| leadsblue/b2b-contact-database-sample-dataset | Sample (~10K?) | Unknown provenance | **Evaluate, probably skip** |
| mayuringle8890/b2b-companies-table | Small | Unknown | **Skip** |
| wcukierski/enron-email-dataset | Historical only | Historical, not useful | **Skip** |

Kaggle B2B datasets are mostly **samples** of paid databases — tiny and legally questionable. Not a reliable source.

### Tier 4: Leaked / Breach Data (legally risky)

Background research (public knowledge):

- **Apollo.io 2018 breach:** 125M unique emails, 200M+ contact records exposed. Data has been traded on hacking forums for years.
- **LinkedIn 2021 breach:** 700M records scraped from public profiles.
- **Nov 2025 "lead-gen mega-leak":** 16TB, 4.3B professional records, reportedly scraped from LinkedIn + Apollo. Secured within 2 days after researchers notified.

**Legal/ethical stance:** Using breached data exposes MagikLead to serious legal risk:
- **GDPR violations** (€20M+ fines) — processing personal data without lawful basis
- **CFAA exposure** (US) — using data from a "computer obtained without authorization"
- **Reputational risk** — being the "built on leaked data" startup kills enterprise sales
- **Breach notification** — if we're found in possession, we owe notifications ourselves

**Recommendation: avoid leaked datasets entirely.** The marginal data value is not worth the legal exposure. Note they exist for context only — do not ingest.

### Tier 5: Paid Seed Data (optional later)

| Provider | Model | Cost |
|----------|-------|------|
| BookYourData | Pay-as-you-go CSV | $0.40/contact |
| Lead411 | Subscription | $49/mo (1K exports) |
| People Data Labs | Enterprise | $98-2500/mo |
| Apollo CSV export | Subscription | $49/mo |

Skip for MVP. Use only if free sources don't cover an ICP.

---

## Recommended Database Architecture

### Canonical entities

```
organizations          — one row per real-world company
organization_aliases   — "Google", "Alphabet Inc.", "google.com"
organization_identifiers — domain, LinkedIn URL, EIN, CIK, Crunchbase ID, ticker, etc.

persons                — one row per real-world human
person_aliases         — "Timothy Cook", "Tim Cook", "T. Cook"
person_identifiers     — LinkedIn URL, Twitter, GitHub, personal website

employments            — time-bound person ↔ organization relationships
  (person_id, org_id, title, start_date, end_date, is_current)

emails                 — verified emails only (never guessed)
  (email, person_id, verified_at, verification_method, bounce_count, is_catchall)

phones                 — verified phones
social_profiles        — Twitter, GitHub, LinkedIn (structured)

domains                — company email domains
  (domain, mx_records, is_catchall, last_verified_at)
```

### Source provenance

```
sources                — master list of data providers
  (name, type, api_endpoint, last_ingested_at)

raw_ingests            — S3 metadata for every blob we've ingested
  (source_id, file_url, ingested_at, checksum, size_bytes)

source_records         — parsed records from raw ingests, immutable
  (source_id, raw_ingest_id, external_id, fields_json, ingested_at)

evidence               — links canonical records to source records
  (canonical_table, canonical_id, source_record_id, confidence, conflict_flag, resolved_by_user_id)
```

### Tenant data

```
tenants                — existing
users                  — existing
tenant_leads           — per-tenant relationships with canonical persons
  (tenant_id, person_id, status, added_at, notes)  — replaces current 'leads' table
campaigns, campaign_leads — existing, mostly unchanged
```

### Operational

```
deletion_blocklist     — person identifiers we cannot re-ingest (GDPR)
conflict_queue         — conflicts awaiting manual review
merge_history          — audit of dedup decisions
search_cache           — materialized results for repeated queries
```

---

## Key Design Decisions

### 1. Entity Resolution (Deduplication)

When we ingest "Tim Cook, CEO, Apple" from SEC EDGAR and "Timothy D. Cook, CEO, Apple Inc." from Wikidata, we must merge them.

**Strategy:**
- **Exact match on strong identifiers** (LinkedIn URL, verified email, SEC CIK for executives) → auto-merge
- **Fuzzy match on name + company + title** → queue for review
- **New canonical record** if no match

Use PostgreSQL `pg_trgm` for fuzzy name matching. Store match confidence in the evidence table.

### 2. Sub-Second Query Performance

At 100M+ records, PostgreSQL alone with btree indexes won't hit sub-second for complex filters.

**Approach:**
- **Primary store:** PostgreSQL (canonical tables, full history, audit)
- **Search layer:** Typesense or Meilisearch — denormalized documents with person+current_employment+emails+identifiers for fast filtered search
- **Sync:** Logical replication / CDC from Postgres → search index
- **Indexes:** GIN on JSONB fields, trgm on name/title/company columns

**Query pattern:**
```
Customer query "VP Sales at 100-500 SaaS companies in NYC"
  → Search layer (Typesense): 50-200ms
  → Returns canonical_person_ids
  → Postgres: fetch current employments + verified emails
  → Total: 200-500ms
```

### 3. Email Verification Pipeline

**Tiered email quality:**
1. **Source-verified:** email came from a structured source (SEC, CrunchBase) — mark trusted
2. **SMTP-verified (non-catchall):** catchall detection first, then RCPT TO — only trust if non-catchall
3. **Pattern-guessed:** NEVER store. Don't create phantom emails.
4. **Bounced:** move to `bounce_log`, don't purge

### 4. GDPR Compliance Design

**Right to erasure:**
- DELETE person from canonical tables
- DELETE from all indexes
- INSERT into `deletion_blocklist` (by email hash, LinkedIn URL, name+company)
- Keep `raw_ingests` for audit (legal basis: legitimate interest)
- On re-ingestion, check blocklist before inserting

**User account deletion:**
- DELETE `tenant_leads` rows for tenant
- Canonical `persons` untouched
- Tenant can't see the leads anymore, but they exist for other tenants

### 5. Source Retention

S3 bucket: `magiklead-raw-ingests`
Structure: `s3://magiklead-raw-ingests/<source>/<date>/<filename>`
Example: `s3://magiklead-raw-ingests/sec-edgar/2026-04-21/company-officers-Q1-2026.xml`

Retained forever. Allows reprocessing, debugging, compliance evidence, and schema changes.

---

## Architecture Decisions (resolved)

1. **Search layer:** **PostgreSQL with `pg_trgm` + GIN indexes.** No external search service for MVP. Re-evaluate when we cross 10M records or complex queries exceed 1s.
2. **Raw storage:** **S3** (or Hetzner-hosted MinIO). Bucket: `magiklead-raw-ingests`. Structure: `<source>/<YYYY-MM-DD>/<filename>`.
3. **Confidence scoring:** **0-100 numerical.** Stored as `int2` on evidence rows. Enables fine-grained tuning of auto-merge and ranking.
4. **Conflict UI:** **Internal admin panel only.** MagikLead operators resolve conflicts. Not exposed to tenants (avoids abuse + keeps data quality in our control).
5. **Auto-merge threshold:** **≥ 95% confidence auto-merges.** 70-95% queues for admin review. < 70% creates a new canonical record.

---

## Realistic Ingestion Roadmap (phases)

**Phase 1 (week 1):** Schema + infrastructure
- Postgres migrations for the new tables
- S3 bucket for raw ingests
- Ingestion runner framework (source → raw_ingests → source_records → canonical)

**Phase 2 (week 2):** SEC EDGAR
- Parse quarterly filings for exec officers
- Entity resolution within EDGAR first (same person appears in multiple filings)
- ~500K high-quality execs seeded

**Phase 3 (week 3):** Wikidata + CrunchBase Open Data
- SPARQL queries for business people
- Daily CrunchBase CSV dump processing
- Cross-source dedup with EDGAR

**Phase 4 (week 4):** OpenCorporates + GitHub
- Directors from OpenCorporates (lower quality, filter carefully)
- GitHub org members for tech-focused searches

**Phase 5 (week 5-6):** Common Crawl extraction
- Build pipeline to find team/about pages in Common Crawl
- Extract names + titles with Claude
- Massive potential volume (millions of records)

**Phase 6 (week 7+):** Search layer + UI
- Typesense integration
- Sub-second query endpoint
- Conflict resolution UI
- Paid seed data (BookYourData) for gap coverage

---

## What This Replaces

- Current `leads` table → becomes `persons` + `emails` + `employments`
- Current on-demand scraping → falls back option when canonical DB doesn't cover a query
- Current SMTP-guess pipeline → replaced by source-verified emails only
