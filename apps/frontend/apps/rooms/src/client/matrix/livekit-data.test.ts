/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';

/** Run the real SDK Worker in an isolated runtime, as browsers do for call encryption. */
test('LiveKit encrypts data before transport, marks GCM and rejects modified ciphertext', async () => {
  const process = Bun.spawn(
    [
      Bun.which('bun') ?? globalThis.process.execPath,
      new URL('../../../../../../../test/e2e/lib/livekit-data-probe.mjs', import.meta.url).pathname,
    ],
    { stdout: 'pipe', stderr: 'pipe' },
  );
  const timeout = setTimeout(() => process.kill(), 15000);
  try {
    const [code, out, error] = await Promise.all([
      process.exited,
      new Response(process.stdout).text(),
      new Response(process.stderr).text(),
    ]);
    if (code !== 0) throw new Error(`LiveKit SDK probe failed (${code}): ${error}`);
    expect(JSON.parse(out)).toEqual({
      encrypted: true,
      gcm: true,
      recovered: true,
      tamperingRejected: true,
    });
  } finally {
    clearTimeout(timeout);
    process.kill();
  }
}, 20000);
