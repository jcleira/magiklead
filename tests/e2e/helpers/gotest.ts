// Runs `go test` against the backend module for the flows (10, 11)
// whose AFK substitute is a Go integration suite. Dual-mode: in CI
// (DATABASE_URL set) it runs go directly on the runner against the CI
// Postgres/Redis; locally it routes through the devpod's api
// container. Keeps those flows host-portable.
import { execFileSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const backendDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'backend');

export function goTest(args: string[]): string {
  if (process.env.DATABASE_URL) {
    return execFileSync('go', ['test', ...args], {
      cwd: backendDir,
      encoding: 'utf8',
      maxBuffer: 64 * 1024 * 1024,
    });
  }
  return execFileSync('devpods', ['exec', 'api', 'go', 'test', ...args], {
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024,
  });
}
