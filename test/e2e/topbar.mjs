import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ storageState: '.auth/admin.json', ignoreHTTPSErrors: true, viewport: { width: 1280, height: 800 } });
const p = await c.newPage();
await p.goto('https://drive.sytue.localhost/', { waitUntil: 'domcontentloaded' });
await p.waitForTimeout(7000);
console.log(await p.evaluate(() => {
  const head = document.querySelector('header') || document.body;
  // Todo lo que hay en la zona de acciones, de izquierda a derecha
  return [...head.querySelectorAll('*')]
    .filter((e) => { const r = e.getBoundingClientRect(); return r.width > 8 && r.height > 8 && r.right > window.innerWidth - 420; })
    .map((e) => {
      const r = e.getBoundingClientRect();
      const cs = getComputedStyle(e);
      return `${e.tagName.toLowerCase()}${e.className && typeof e.className === 'string' ? '.' + e.className.split(' ').slice(0,2).join('.') : ''} [${Math.round(r.left)},${Math.round(r.width)}x${Math.round(r.height)}] bg=${cs.backgroundColor} texto="${(e.textContent||'').trim().slice(0,12)}"`;
    }).slice(0, 12);
}));
await p.screenshot({ path: '/tmp/topbar.png', clip: { x: 900, y: 0, width: 380, height: 64 } });
await b.close();
