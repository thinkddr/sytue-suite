import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const p = await c.newPage();
await p.goto('https://sytue.localhost/', { waitUntil: 'domcontentloaded', timeout: 60000 });
await p.waitForTimeout(4000);
await p.screenshot({ path: '/tmp/sweep-www.png' });
console.log('OK www', p.url());
await b.close();
