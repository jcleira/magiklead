# Lead Discovery Engine — Technical Design

**Date:** 2026-04-13
**Status:** Approved, ready for implementation

---

## The Problem

The engine must work for **any product, any ICP, any industry**. Whether a user sells AI photo tools to content creators, CRM software to sales teams, industrial equipment to manufacturing plants, or supplements to gym owners — the engine must find the right people who would actually buy that specific product.

The current engine can't do this. It has no real people search — only company domain lookups that return random employees.

---

## Architecture: 4-Step Pipeline

```
Step 1: AI Company Targeting (Claude Haiku)
  │  Input: product description + ICP
  │  Output: 8-10 company domains
  ▼
Step 2: People Search (LinkedIn API — RapidAPI rockapis)
  │  Input: titles + companies from Step 1
  │  Output: real people with name, title, company, LinkedIn URL
  ▼
Step 3: Email Enrichment (Hunter.io email-finder)
  │  Input: first_name + last_name + company domain
  │  Output: verified email + confidence score
  ▼
Step 4: Store + Cache (PostgreSQL)
     Only leads WITH verified email get linked to campaign
```

---

## Step 1: AI Company Targeting

**API:** Claude Haiku
**Cost:** ~$0.002 per call

Claude receives the user's product description and ICP, then identifies 8-10 specific companies whose employees would be buyers. Not platforms. Not tools. Companies where the target persona works.

---

## Step 2: People Search — LinkedIn API

**API:** Real-Time LinkedIn Scraper by rockapis
**Host:** `linkedin-data-api.p.rapidapi.com`
**Endpoint:** `GET /search-people`
**Plan:** Pro ($175/mo) — 50,000 credits/month, 120 req/min

### Request

```
GET /search-people?keywords=VP+Sales&start=0
X-RapidAPI-Key: <key>
X-RapidAPI-Host: linkedin-data-api.p.rapidapi.com
```

Can also filter by company, location, industry, etc.

### Response

```json
{
  "success": true,
  "data": [
    {
      "id": "abc123",
      "urn": "urn:li:member:12345",
      "url": "https://www.linkedin.com/in/john-smith",
      "full_name": "John Smith",
      "title": "VP Sales",
      "location": "San Francisco, CA",
      "company": "Acme Corp",
      "is_premium": true,
      "avatar": "https://..."
    }
  ],
  "total": 2450,
  "has_more": true
}
```

### Strategy

For each company from Step 1, search LinkedIn for people matching the ICP titles at that company. This gives us targeted, real people — not random employees.

With 50,000 credits/month at $175, that's ~$0.0035/search. Supporting thousands of discovery runs per month.

---

## Step 3: Email Enrichment — Hunter.io

**API:** Hunter.io email-finder
**Endpoint:** `GET /v2/email-finder`
**Cost:** 1 request per person

### Request

```
GET /v2/email-finder?domain=acme.com&first_name=John&last_name=Smith&api_key=KEY
```

### Response

```json
{
  "data": {
    "email": "john.smith@acme.com",
    "score": 91,
    "position": "VP Sales",
    "linkedin": "https://linkedin.com/in/john-smith"
  }
}
```

**Keep if:** score >= 70. Skip otherwise.

---

## Step 4: Store + Cache

- Upsert each lead into `leads` table (dedupe by `linkedin_url`)
- Only link leads WITH email to the campaign
- Cache the full search by query hash in `lead_searches` (TTL: 30 days)
- Leads without email stored for future enrichment but not in campaign

---

## Cost Per Discovery Run

~8 companies × ~10 people each = ~80 LinkedIn searches + ~80 Hunter lookups

| Step | API | Calls | Cost |
|------|-----|-------|------|
| Company targeting | Claude Haiku | 1 | $0.002 |
| People search | LinkedIn API | ~80 | $0.28 |
| Email finding | Hunter.io | ~80 | ~$0.80 |
| **Total per run** | | | **~$1.10** |
| **Per lead with email** | | | **~$0.03** |

### Monthly at scale

| Tier | LinkedIn API | Hunter.io | Claude | Total |
|------|-------------|-----------|--------|-------|
| Current | $175/mo (50k) | $49/mo (500) | ~$10 | $234/mo |
| Scale | $300/mo (100k) | $149/mo (2.5k) | ~$50 | $499/mo |

---

## Files to Create/Modify

| File | Action |
|------|--------|
| `backend/internal/leads/companies.go` | **NEW** — AI company targeting via Claude |
| `backend/internal/leads/linkedin_search.go` | **NEW** — LinkedIn people search via RapidAPI |
| `backend/internal/leads/hunter.go` | **NEW** — Hunter.io email-finder |
| `backend/internal/leads/pipeline.go` | **REWRITE** — orchestrate 4 steps |
| `backend/internal/leads/linkedin.go` | **DELETE** — replaced |
| `backend/internal/leads/email.go` | **DELETE** — replaced |
| `backend/internal/worker/discover.go` | **UPDATE** — pass business context |
| `backend/.env` | **UPDATE** — RAPIDAPI_KEY already exists |

---

## Verification

1. Clean data (redis + postgres)
2. Run onboarding for any product URL
3. Click "Discover Leads"
4. Worker logs show:
   - "AI selected 8 companies: [buzzfeed.com, ...]"
   - "LinkedIn: found 7 people at BuzzFeed matching 'content creator'"
   - "Hunter: john.doe@buzzfeed.com (confidence: 91%)"
   - "Discovered 35 leads, 22 with verified emails, linked to campaign"
5. UI shows real names, real titles, real companies, real emails
