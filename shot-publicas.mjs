import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } });
const p = await c.newPage();
for (const [nombre, url] of [['downloads', '/downloads'], ['producto', '/product/mail'], ['privacy', '/privacy']]) {
  try {
    await p.goto('https://sytue.localhost' + url, { waitUntil: 'domcontentloaded', timeout: 45000 });
    await p.waitForTimeout(2500);
    await p.screenshot({ path: `/tmp/pub-${nombre}.png` });
    console.log('OK', nombre, await p.title());
  } catch (e) { console.log('FALLO', nombre, String(e).split('\n')[0].slice(0, 60)); }
}
await b.close();
