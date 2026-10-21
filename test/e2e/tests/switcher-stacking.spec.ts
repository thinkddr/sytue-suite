// El selector de apps de la suite tiene que quedar POR ENCIMA de la interfaz de
// cualquier producto. Ha fallado dos veces por la misma causa —Sytue DevX y
// Sytue Rooms— y las dos con el mismo patrón: una barra lateral con el z-index
// de modal (1060) aplicado también en escritorio, por encima de la topbar
// (1040). Y la topbar CREA contexto de apilamiento, así que el panel, aunque
// declara 1070, compite como 1040 frente al resto de la página: subirle el
// z-index no arregla nada.
//
// No se comprueba leyendo z-index, que es justo lo que engaña aquí, sino
// preguntándole al navegador qué elemento hay realmente en cada punto del panel.
import { expect, test } from '@playwright/test';
import { APPS, gotoApp, type AppKey } from './helpers';

/**
 * Apps que montan la Sytue Shell (y por tanto el selector). Falta `calendar`:
 * su contenedor de dev no levanta ahora mismo por dependencias que le faltan al
 * entorno local, no por el apilamiento. Añadirla en cuanto vuelva a servir.
 */
const CON_SHELL: AppKey[] = [
  'one',
  'mail',
  'drive',
  'docs',
  'contacts',
  'rooms',
  'cloud',
  'console',
];

for (const app of CON_SHELL) {
  test(`el selector de apps queda por encima de la UI de ${app}`, async ({ page }) => {
    // Islas frías + OIDC: el presupuesto base se queda corto en el arranque.
    test.slow();
    await gotoApp(page, app);

    const trigger = page.locator('button[aria-haspopup="menu"][aria-expanded]').first();
    await expect(trigger, `${app} debe montar el selector de apps`).toBeVisible({
      timeout: 45_000,
    });
    await trigger.click();

    const tapados = await page.evaluate(() => {
      const btn = document.querySelector('button[aria-haspopup="menu"][aria-expanded="true"]');
      const panel = btn?.parentElement?.querySelector<HTMLElement>('div[style*="z-popover"]');
      if (!panel) return ['el panel no llegó a abrirse'];
      const b = panel.getBoundingClientRect();
      // Las cuatro esquinas útiles y el centro: si el panel asoma por debajo de
      // algo, casi siempre lo hace por un borde, no por el medio.
      const puntos: Array<[number, number]> = [
        [b.left + 8, b.top + 8],
        [b.left + b.width / 2, b.top + b.height / 2],
        [b.right - 8, b.top + 8],
        [b.left + 8, b.bottom - 8],
        [b.right - 8, b.bottom - 8],
      ];
      return puntos
        .filter(([x, y]) => {
          const el = document.elementFromPoint(x, y);
          return !el || !panel.contains(el);
        })
        .map(([x, y]) => {
          const el = document.elementFromPoint(x, y);
          const quien = el ? `${el.tagName.toLowerCase()}.${String(el.className).slice(0, 50)}` : '—';
          return `(${Math.round(x)},${Math.round(y)}) lo tapa ${quien}`;
        });
    });

    expect(tapados, `algo se pinta por encima del selector en ${APPS[app]}`).toEqual([]);
  });
}
