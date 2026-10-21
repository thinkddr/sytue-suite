/* SPDX-License-Identifier: Apache-2.0 */
import { createRequire } from 'node:module';
import { readFile, writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
const app = createRequire(new URL('../../apps/frontend/apps/rooms/package.json', import.meta.url));
const browser = createRequire(
  process.env.SYTUE_PLAYWRIGHT_ROOT
    ? pathToFileURL(process.env.SYTUE_PLAYWRIGHT_ROOT + '/package.json')
    : new URL('./package.json', import.meta.url),
);
const { chromium } = browser('playwright');
if (!process.env.SYTUE_LIVEKIT_GO_VIDEO_FIXTURE)
  throw new Error('An artificial Go SDK VP8 fixture is required');
const fixture = JSON.parse(await readFile(process.env.SYTUE_LIVEKIT_GO_VIDEO_FIXTURE, 'utf8'));
const vendor = new URL('./', pathToFileURL(app.resolve('livekit-client')));
const files = {
  '/livekit-client.mjs': await readFile(new URL('livekit-client.esm.mjs', vendor)),
  '/livekit-worker.mjs': await readFile(new URL('livekit-client.e2ee.worker.mjs', vendor)),
  '/probe.mjs': await readFile(new URL('./lib/livekit-video-browser.mjs', import.meta.url)),
};
const server = Bun.serve({
  hostname: '127.0.0.1',
  port: 0,
  fetch(request) {
    const path = new URL(request.url).pathname;
    if (path === '/')
      return new Response(
        '<!doctype html><html lang="en"><title>Artificial encrypted media fixture</title><script type="module" src="/probe.mjs"></script></html>',
        { headers: { 'content-type': 'text/html' } },
      );
    return files[path]
      ? new Response(files[path], { headers: { 'content-type': 'text/javascript' } })
      : new Response(null, { status: 404 });
  },
});
const instance = await chromium.launch({
  headless: true,
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--enable-features=WebRtcEncodedTransform'],
});
try {
  const page = await instance.newPage();
  await page.goto(server.url.href);
  await page.waitForFunction(() => typeof window.runLiveKitVideoInterop === 'function');
  const result = await page.evaluate((input) => window.runLiveKitVideoInterop(input), fixture);
  const destination =
    process.env.SYTUE_LIVEKIT_JS_VIDEO_FIXTURE ??
    new URL('../fixtures/rooms-livekit-js-video.json', import.meta.url);
  await writeFile(destination, JSON.stringify(result, null, 2) + '\n');
  process.stdout.write(
    JSON.stringify({
      artificial: true,
      goToJsFrames: result.frames.length,
      jsFramesGenerated: result.frames.length,
      headerTamperingRejected: true,
      ivTamperingRejected: true,
    }) + '\n',
  );
} finally {
  await instance.close();
  await server.stop(true);
}
