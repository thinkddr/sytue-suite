import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } });
const p = await c.newPage();
await p.goto('https://sytue.localhost/', { waitUntil: 'domcontentloaded', timeout: 60000 });
await p.waitForTimeout(4000);
const h = await p.evaluate(() => document.body.scrollHeight);
for (let i = 0, y = 0; y < h && i < 6; i++, y += 1700) {
  await p.evaluate((yy) => window.scrollTo(0, yy), y);
  await p.waitForTimeout(500);
  await p.screenshot({ path: `/tmp/www-${i}.png` });
}
console.log('alto total', h);
await b.close();
