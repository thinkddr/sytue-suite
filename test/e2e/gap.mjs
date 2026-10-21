import { chromium } from 'playwright';
const b = await chromium.launch();
const c = await b.newContext({ storageState: '.auth/admin.json', ignoreHTTPSErrors: true, viewport: { width: 1280, height: 800 } });
const p = await c.newPage();
await p.goto('https://drive.sytue.localhost/', { waitUntil: 'domcontentloaded' });
await p.waitForTimeout(8000);
console.log(await p.evaluate(() => {
  const campana = [...document.querySelectorAll('button')].find((b) => /notif|benachricht/i.test(b.getAttribute('aria-label') || ''));
  const avatar = [...document.querySelectorAll('button')].find((b) => /cuenta|account|konto/i.test(b.getAttribute('aria-label') || ''));
  if (!campana || !avatar) return { campana: !!campana, avatar: !!avatar };
  const a = campana.getBoundingClientRect(), z = avatar.getBoundingClientRect();
  const medio = { x: (a.right + z.left) / 2, y: (a.top + a.bottom) / 2 };
  const pila = document.elementsFromPoint(medio.x, medio.y).slice(0, 3).map((e) => {
    const r = e.getBoundingClientRect(); const cs = getComputedStyle(e);
    return `${e.tagName.toLowerCase()}.${String(e.className).split(' ')[0]} ${Math.round(r.width)}x${Math.round(r.height)} bg=${cs.backgroundColor} radio=${cs.borderRadius}`;
  });
  return { campanaDerecha: Math.round(a.right), avatarIzq: Math.round(z.left), huecoPx: Math.round(z.left - a.right), enElMedio: pila };
}));
await p.screenshot({ path: '/tmp/gap.png', clip: { x: 1050, y: 0, width: 230, height: 60 } });
await b.close();
