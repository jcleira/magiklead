// The Next dev server refuses a cross-origin dev request from a host that
// allowedDevOrigins does not list. A devpod answers at `*.magiklead.localhost`
// and at `*.magiklead.<suffix>` (devpods sets DEVPOD_SUFFIX), so the list
// holds both. Run: `pnpm test` (node --test; Node strips the types).
import assert from "node:assert/strict";
import { test } from "node:test";

let run = 0;

// load reads next.config.ts again with DEVPOD_SUFFIX set to suffix
// (undefined: not set); a new query string skips the module cache.
async function load(suffix) {
  const old = process.env.DEVPOD_SUFFIX;
  if (suffix === undefined) delete process.env.DEVPOD_SUFFIX;
  else process.env.DEVPOD_SUFFIX = suffix;
  try {
    const mod = await import(`./next.config.ts?run=${run++}`);
    return mod.default.allowedDevOrigins;
  } finally {
    if (old === undefined) delete process.env.DEVPOD_SUFFIX;
    else process.env.DEVPOD_SUFFIX = old;
  }
}

test("allowedDevOrigins has the suffix name of the devpod", async () => {
  assert.deepEqual(await load("z13.internal"), [
    "mvp.magiklead.localhost",
    "*.magiklead.localhost",
    "*.magiklead.z13.internal",
  ]);
});

test("allowedDevOrigins has only the .localhost names with no suffix", async () => {
  const want = ["mvp.magiklead.localhost", "*.magiklead.localhost"];
  assert.deepEqual(await load(undefined), want);
  assert.deepEqual(await load("localhost"), want);
  assert.deepEqual(await load(" "), want);
});
