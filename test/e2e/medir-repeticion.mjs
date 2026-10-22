import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } });
const p = await c.newPage();
await p.goto('https://sytue.localhost/es/', { waitUntil: 'domcontentloaded', timeout: 45000 });
await p.waitForTimeout(2500);
const txt = await p.locator('body').innerText();
for (const re of [/soberan[ao]s?/gi, /europe[ao]s?/gi, /\bUE\b/g, /100 ?%/g]) {
  console.log(String(re).padEnd(18), (txt.match(re) || []).length);
}
// Y solo la primera pantalla (lo que se ve sin desplazar).
const hero = await p.locator('section').first().innerText();
console.log('--- en el hero:', ['soberan', 'europe', 'UE', '100'].map(w => `${w}=${(hero.match(new RegExp(w, 'gi'))||[]).length}`).join(' '));
await b.close();
