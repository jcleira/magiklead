// Port of /tmp/argos-walk/clerk-mint.sh (the AFK helper that drove
// #13's flow walk). Mints a fresh Clerk user via the Backend API,
// creates a session, requests a JWT against the magiklead-backend
// template, and returns everything a test needs to act as that
// user. Per-test invocation gives tenant isolation for free —
// every test gets its own Clerk identity → its own materialised
// tenant on first protected request.

export interface ClerkIdentity {
  userId: string;
  sessionId: string;
  email: string;
  jwt: string;
  // mintFreshJWT requests a new token for the same session — the
  // template lifetime is 60s, so long-running specs need to refresh.
  mintFreshJWT: () => Promise<string>;
}

const CLERK_BASE = 'https://api.clerk.com/v1';

async function clerkFetch(path: string, init: RequestInit = {}): Promise<unknown> {
  const secret = process.env.CLERK_SECRET_KEY;
  if (!secret) {
    throw new Error('CLERK_SECRET_KEY not set — see tests/e2e/.env.e2e.example');
  }
  const res = await fetch(`${CLERK_BASE}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${secret}`,
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
  });
  const body = await res.text();
  if (!res.ok) {
    throw new Error(`Clerk ${init.method ?? 'GET'} ${path} → ${res.status}: ${body}`);
  }
  return body ? JSON.parse(body) : null;
}

function randomPassword(): string {
  const bytes = new Uint8Array(20);
  crypto.getRandomValues(bytes);
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return `Argos-${hex}!`;
}

// mintIdentity creates a fresh user + session and returns the
// first JWT. The slug parameter shows up in the email local-part so
// failed runs are easy to attribute to the source spec.
export async function mintIdentity(slug: string): Promise<ClerkIdentity> {
  const email = `argos-e2e-${slug}-${Date.now()}@gmail.com`;
  const user = (await clerkFetch('/users', {
    method: 'POST',
    body: JSON.stringify({
      email_address: [email],
      password: randomPassword(),
      skip_password_checks: true,
    }),
  })) as { id: string };

  const session = (await clerkFetch('/sessions', {
    method: 'POST',
    body: JSON.stringify({ user_id: user.id }),
  })) as { id: string };

  const mintFreshJWT = async (): Promise<string> => {
    const tok = (await clerkFetch(
      `/sessions/${session.id}/tokens/magiklead-backend`,
      { method: 'POST' },
    )) as { jwt: string };
    return tok.jwt;
  };

  const jwt = await mintFreshJWT();
  return { userId: user.id, sessionId: session.id, email, jwt, mintFreshJWT };
}

// deleteIdentity is the per-test teardown. Best-effort — a 404 is
// fine (the test may have already DELETE'd its account via the api
// route, which cascades to Clerk).
export async function deleteIdentity(userId: string): Promise<void> {
  try {
    await clerkFetch(`/users/${userId}`, { method: 'DELETE' });
  } catch (err) {
    if (!(err instanceof Error) || !err.message.includes('404')) throw err;
  }
}
