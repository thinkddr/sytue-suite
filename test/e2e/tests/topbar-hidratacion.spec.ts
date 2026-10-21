// La topbar se hidrata mal cuando el árbol del cliente no tiene los mismos
// hijos que el HTML del servidor: Preact empareja por POSICIÓN y reutiliza el
// elemento equivocado conservando sus atributos del SSR. Pasó con el aviso de
// «sin conexión», y se veía en producción de dos formas, las dos malas:
//
//   - Conectado: quedaba la caja del aviso vacía —borde y fondo, 22×32— entre
//     la campana y el avatar. Un cuadro gris que veía todo el mundo, release
//     tras release, y que no se encontraba leyendo el componente: el componente
//     estaba bien, lo que fallaba era `navigator.onLine` en el SSR (Node ≥21
//     define `navigator` SIN `onLine`, así que `!navigator.onLine` era `true` y
//     el servidor mandaba a todos el HTML de «sin conexión»).
//   - Sin conexión: el aviso se pintaba DENTRO del separador —`w-px h-6` y
//     `aria-hidden`—, o sea invisible y mudo justo cuando hay que leerlo.
//
// Por eso se mide en el DOM y no por captura: los dos fallos son elementos que
// están donde no deben, con el tamaño equivocado, y ninguna aserción de texto
// los habría visto.
import { expect, test } from '@playwright/test';
import { APPS } from './helpers';

/** Fuerza el estado de red ANTES de que cargue nada, que es cuando se hidrata. */
async function conRed(page: import('@playwright/test').Page, hayRed: boolean): Promise<void> {
  await page.addInitScript(`Object.defineProperty(navigator, 'onLine', {
    get: () => ${hayRed}, configurable: true });`);
}

const CAJAS = ['one', 'drive', 'mail'] as const;

for (const app of CAJAS) {
  test(`${app}: la topbar no deja cajas vacías al hidratar`, async ({ page }) => {
    await conRed(page, true);
    await page.goto(APPS[app], { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(3000);

    const vacias = await page.evaluate(() => {
      const acciones = document.querySelector('header')?.lastElementChild;
      return [...(acciones?.children ?? [])]
        .filter((e) => {
          const r = e.getBoundingClientRect();
          const cs = getComputedStyle(e);
          const pinta =
            cs.borderTopWidth !== '0px' || cs.backgroundColor !== 'rgba(0, 0, 0, 0)';
          // Un separador es legítimamente estrecho y sin contenido; lo que no
          // puede haber es una caja ANCHA, pintada y sin nada dentro.
          return pinta && r.width > 4 && !(e.textContent || '').trim() && !e.querySelector('svg');
        })
        .map((e) => e.outerHTML.slice(0, 120));
    });
    expect(vacias, 'caja pintada y vacía en la topbar').toEqual([]);
  });
}

test('el aviso de sin conexión se ve y se anuncia', async ({ page }) => {
  await conRed(page, false);
  await page.goto(APPS.drive, { waitUntil: 'domcontentloaded' });

  const aviso = page.locator('header [role="status"][aria-live]').filter({ hasText: /\S/ });
  await expect(aviso).toBeVisible();
  // Dentro del separador medía 1px de ancho: visible para Playwright, ilegible
  // para una persona. La caja real ronda los 80×32.
  const caja = await aviso.boundingBox();
  expect(caja?.width ?? 0).toBeGreaterThan(40);
  await expect(aviso).not.toHaveAttribute('aria-hidden', 'true');
});
