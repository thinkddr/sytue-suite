// Cadena de apilamiento del panel y de quien lo tapa.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
const S = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const b = await chromium.launch();
const c = await b.newContext({ storageState: S, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } });
const p = await c.newPage();
await p.goto('https://rooms.sytue.localhost/', { waitUntil: 'domcontentloaded', timeout: 60000 });
await p.waitForTimeout(7000);
await p.locator('button[aria-haspopup="menu"][aria-expanded]').first().click();
await p.waitForTimeout(600);
console.log(await p.evaluate(() => {
  const cadena = (el) => {
    const out = [];
    while (el && el !== document.documentElement) {
      const s = getComputedStyle(el);
      const crea = s.zIndex !== 'auto' && s.position !== 'static';
      if (crea || s.transform !== 'none' || s.filter !== 'none' || s.isolation === 'isolate' || +s.opacity < 1) {
        out.push(`${el.tagName.toLowerCase()}${el.id ? '#' + el.id : ''} pos=${s.position} z=${s.zIndex} tr=${s.transform !== 'none'} op=${s.opacity}`);
      }
      el = el.parentElement;
    }
    return out;
  };
  const btn = document.querySelector('button[aria-haspopup="menu"][aria-expanded="true"]');
  const panel = btn?.parentElement?.querySelector('div[style*="z-popover"]');
  const box = panel.getBoundingClientRect();
  const culpable = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
  const overflows = [];
  let el = panel;
  while (el && el !== document.documentElement) {
    const s2 = getComputedStyle(el);
    if (s2.overflow !== 'visible' || s2.overflowX !== 'visible' || s2.overflowY !== 'visible') {
      const r = el.getBoundingClientRect();
      overflows.push(`${el.tagName.toLowerCase()}${el.id ? '#' + el.id : ''} overflow=${s2.overflow}/${s2.overflowX}/${s2.overflowY} rect=${Math.round(r.width)}x${Math.round(r.height)}`);
    }
    el = el.parentElement;
  }
  const pr = panel.getBoundingClientRect();
  return ['PANEL rect: ' + JSON.stringify({x: Math.round(pr.x), y: Math.round(pr.y), w: Math.round(pr.width), h: Math.round(pr.height)}),
          '', 'ANCESTROS QUE RECORTAN:', ...(overflows.length ? overflows : ['(ninguno)']),
          '', 'CULPABLE: ' + culpable.tagName + '.' + String(culpable.className).slice(0, 40)].join('\n');
}));
await b.close();
