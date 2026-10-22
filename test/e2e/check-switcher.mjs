// ¿El selector de apps queda POR ENCIMA de la UI en todas las apps de la suite?
// No se comprueba leyendo z-index —lo que manda es el contexto de apilamiento—,
// sino preguntándole al navegador qué elemento hay de verdad en cada punto del
// panel: si elementFromPoint no cae dentro del panel, algo lo está tapando.
// ponytail: script de verificación; no es parte de la suite.
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';

const STORAGE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const APPS = process.argv.slice(2);

const browser = await chromium.launch();
const ctx = await browser.newContext({
  storageState: STORAGE,
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});

let fallos = 0;
for (const app of APPS) {
  const page = await ctx.newPage();
  try {
    await page.goto(`https://${app}.sytue.localhost/`, {
      waitUntil: 'domcontentloaded',
      timeout: 60_000,
    });
    await page.waitForTimeout(6000);

    const trigger = page.locator('button[aria-haspopup="menu"][aria-expanded]').first();
    await trigger.waitFor({ timeout: 20_000 });
    await trigger.click();
    await page.waitForTimeout(600);

    const r = await page.evaluate(() => {
      const btn = document.querySelector('button[aria-haspopup="menu"][aria-expanded="true"]');
      const panel = btn?.parentElement?.querySelector('div[style*="z-popover"]');
      if (!panel) return { error: 'no se encontró el panel abierto' };
      const box = panel.getBoundingClientRect();
      // Rejilla de puntos dentro del panel: esquinas útiles y centro.
      const pts = [
        [box.left + 8, box.top + 8],
        [box.left + box.width / 2, box.top + box.height / 2],
        [box.right - 8, box.top + 8],
        [box.left + 8, box.bottom - 8],
        [box.right - 8, box.bottom - 8],
      ];
      const tapados = [];
      for (const [x, y] of pts) {
        const el = document.elementFromPoint(x, y);
        if (!el || !panel.contains(el)) {
          const culpable = el
            ? `${el.tagName.toLowerCase()}${el.className ? '.' + String(el.className).slice(0, 60) : ''}`
            : 'nada';
          tapados.push({ x: Math.round(x), y: Math.round(y), culpable });
        }
      }
      return { w: Math.round(box.width), h: Math.round(box.height), tapados };
    });

    if (r.error) {
      console.log(`✗ ${app.padEnd(8)} ${r.error}`);
      fallos++;
    } else if (r.tapados.length) {
      console.log(`✗ ${app.padEnd(8)} panel ${r.w}x${r.h} — TAPADO en ${r.tapados.length}/5 puntos`);
      for (const t of r.tapados) console.log(`      (${t.x},${t.y}) → ${t.culpable}`);
      fallos++;
    } else {
      console.log(`✓ ${app.padEnd(8)} panel ${r.w}x${r.h} por encima en los 5 puntos`);
    }
  } catch (e) {
    console.log(`✗ ${app.padEnd(8)} ${String(e).split('\n')[0].slice(0, 90)}`);
    fallos++;
  } finally {
    await page.close();
  }
}
console.log(fallos ? `\n${fallos} app(s) con problema` : '\nTodas por encima');
await browser.close();
