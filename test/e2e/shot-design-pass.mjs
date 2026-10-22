// Capturas del pase de diseño: abre cada app con la sesión admin (storageState)
// y guarda un PNG del viewport 1440x900 en /tmp/design-pass/<app>.png.
// ponytail: script de un solo uso para verificación; no es parte de la suite.
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const STORAGE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));
const OUT = '/tmp/design-pass';
mkdirSync(OUT, { recursive: true });

const APPS = {
  portal: 'https://one.sytue.localhost/',
  mail: 'https://mail.sytue.localhost/',
  drive: 'https://drive.sytue.localhost/',
  calendar: 'https://calendar.sytue.localhost/',
  contacts: 'https://contacts.sytue.localhost/',
  docs: 'https://docus.sytue.localhost/',
  chat: 'https://chat.sytue.localhost/',
  'cloud-dashboard': 'https://cloud.sytue.localhost/',
};

const browser = await chromium.launch();
const ctx = await browser.newContext({
  storageState: STORAGE,
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
});

for (const [name, url] of Object.entries(APPS)) {
  const page = await ctx.newPage();
  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
    // La topbar (banner) es señal de que la shell hidrató; espera suave.
    await page.locator('banner, [role=banner]').first().waitFor({ timeout: 10_000 }).catch(() => {});
    await page.waitForTimeout(1500); // asentar islands (Vite/HMR)
    const redirigido = /\/login\//.test(page.url()) || new URL(page.url()).hostname === 'id.sytue.localhost';
    await page.screenshot({ path: `${OUT}/${name}.png` });
    console.log(`${redirigido ? 'LOGIN?' : 'OK'}  ${name}  ->  ${OUT}/${name}.png  (${page.url()})`);
  } catch (e) {
    console.log(`ERR  ${name}  ${e.message.slice(0, 120)}`);
  } finally {
    await page.close();
  }
}

await browser.close();
