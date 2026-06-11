// DB access for e2e assertions. Dual-mode: when DATABASE_URL is set
// (CI / direct), queries go straight to Postgres via psql; otherwise
// they route through the devpod's postgres container (local dev, where
// Postgres has no exposed host port). The exported helpers stay
// synchronous so specs read top-to-bottom without await.
import { execFileSync } from 'node:child_process';

export interface PSQLOptions {
  // Override the implicit devpod name (local mode only); ignored when
  // DATABASE_URL is set.
  devpod?: string;
}

function runPsql(sql: string, opts: PSQLOptions = {}): string {
  const url = process.env.DATABASE_URL;
  if (url) {
    const out = execFileSync('psql', [url, '-tAc', sql], {
      encoding: 'utf8',
      maxBuffer: 32 * 1024 * 1024,
    });
    return out.trim();
  }
  const args = ['exec'];
  if (opts.devpod) args.push('--name', opts.devpod);
  args.push('postgres', 'psql', '-U', 'devpod', '-d', 'devpod', '-tAc', sql);
  const out = execFileSync('devpods', args, { encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 });
  return out.trim();
}

// queryRows runs SQL that yields rows; returns an array of arrays
// (one row per inner array, columns split on the psql -A default
// separator '|'). Suitable for everything but free-text columns
// containing pipes — those are rare in our schema.
export function queryRows(sql: string, opts: PSQLOptions = {}): string[][] {
  const raw = runPsql(sql, opts);
  if (raw === '') return [];
  return raw.split('\n').map((line) => line.split('|'));
}

// queryScalar runs SQL that yields a single value; returns the
// trimmed string (use parseInt/parseFloat at the call site if you
// need a number).
export function queryScalar(sql: string, opts: PSQLOptions = {}): string {
  const out = runPsql(sql, opts);
  return out.split('\n')[0].trim();
}

// queryJSON runs SQL that yields a single jsonb cell; returns the
// parsed object. Useful for `SELECT row_to_json(...)` snapshots.
export function queryJSON<T = unknown>(sql: string, opts: PSQLOptions = {}): T {
  const raw = runPsql(sql, opts);
  return JSON.parse(raw) as T;
}

// exec runs SQL for its side effects, ignoring the row count.
export function exec(sql: string, opts: PSQLOptions = {}): void {
  runPsql(sql, opts);
}
