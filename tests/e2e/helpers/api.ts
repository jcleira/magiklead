// Thin fetch wrapper that prepends the API base URL and attaches the
// bearer JWT. Most specs use this rather than Playwright's request
// fixture because the spec semantics are simpler when one helper
// owns auth refresh.
import { mintIdentity, type ClerkIdentity } from './clerk.js';

const API_BASE = process.env.E2E_API_URL ?? 'http://api-mvp.magiklead.localhost';

export interface APIClient {
  identity: ClerkIdentity;
  get: (path: string) => Promise<Response>;
  post: (path: string, body?: unknown) => Promise<Response>;
  patch: (path: string, body?: unknown) => Promise<Response>;
  del: (path: string) => Promise<Response>;
}

export async function newClient(slug: string): Promise<APIClient> {
  const identity = await mintIdentity(slug);

  const authedFetch = async (method: string, path: string, body?: unknown): Promise<Response> => {
    return fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${identity.jwt}`,
        'Content-Type': 'application/json',
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  };

  return {
    identity,
    get: (path) => authedFetch('GET', path),
    post: (path, body) => authedFetch('POST', path, body),
    patch: (path, body) => authedFetch('PATCH', path, body),
    del: (path) => authedFetch('DELETE', path),
  };
}

// bootstrapTenant fires the lazy-bootstrap path so the test starts
// with a tenant materialised in the DB. Specs that need to read or
// write tenant-scoped tables should call this in beforeEach.
export async function bootstrapTenant(client: APIClient): Promise<void> {
  const res = await client.get('/api/v1/settings');
  if (!res.ok) {
    throw new Error(`bootstrap: GET /settings → ${res.status} ${await res.text()}`);
  }
}

// cleanupTenant is the canonical teardown: DELETE /account cascades
// every tenant-scoped row (campaigns, campaign_leads, email_events,
// tenant_leads, gmail_accounts, subscriptions, user_tenants, the
// user row itself if orphaned) AND calls Clerk's user-delete via
// the handler's clerk-client. Falls back to a bare Clerk delete if
// the api call fails (e.g. test errored before bootstrap).
import { deleteIdentity } from './clerk.js';
export async function cleanupTenant(client: APIClient): Promise<void> {
  try {
    client.identity.jwt = await client.identity.mintFreshJWT();
    const res = await client.del('/api/v1/account');
    if (res.status === 204) return;
  } catch {
    // Fall through to bare Clerk delete.
  }
  await deleteIdentity(client.identity.userId);
}
