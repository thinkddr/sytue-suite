import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ storageState: '.auth/admin.json', ignoreHTTPSErrors: true, viewport: { width: 1280, height: 800 } });
const p = await c.newPage();
await p.goto('https://docus.sytue.localhost/', { waitUntil: 'domcontentloaded' });
await p.waitForTimeout(6000);
// Abrir el modal de compartir de un documento cualquiera
await p.getByText('gente', { exact: true }).first().click().catch(() => {});
await p.waitForTimeout(8000);
await p.locator('button:has-text("Share"), button:has-text("Compartir")').first().click().catch(() => {});
await p.waitForTimeout(2500);
console.log(await p.evaluate(() => {
  const btn = [...document.querySelectorAll('button[aria-label]')].find((b) => /close|cerrar|schließen/i.test(b.getAttribute('aria-label') || ''));
  if (!btn) return '(sin botón de cerrar)';
  return { aria: btn.getAttribute('aria-label'), tieneSVG: !!btn.querySelector('svg'), html: btn.innerHTML.slice(0, 70) };
}));
await p.screenshot({ path: '/tmp/modal.png' });
await b.close();
