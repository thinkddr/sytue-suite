// Vista móvil: lo que `mobile-audit.mjs` no mira.
//
// Aquella detecta desbordamiento horizontal —que la página no se salga de la
// pantalla— y con eso no se veía nada de lo que se reportó el 2026-09-07:
//
//   · campos que disparan el zoom de iOS y dejan la página ampliada,
//   · el menú lateral que se queda tapando lo que acabas de abrir.
//
// Los dos son fallos del SISTEMA (la hoja compartida y las dos shells), así que
// se prueban en TODAS las apps: un arreglo en el sitio común se puede deshacer
// desde una sola app con una clase, y esto es lo que lo pillaría.
//
// Lo que NO está aquí, dicho para que no se busque: el tercer fallo del mismo
// día —`/auth/logout` devolviendo 404— no se reproduce en dev. Cada app se sirve
// aquí en su propio subdominio Y EN LA RAÍZ, mientras que el fallo solo existe
// bajo el host de organización, donde viven todas juntas con prefijo. Lo cubren
// el test unitario de `@sytue/auth` (que fija que la ruta se prefija) y la sonda
// de `sonda-movil.mjs`, que sí mide contra producción.
import { expect, test } from '@playwright/test';
import { APPS, type AppKey, gotoApp } from './helpers';

/** iPhone SE: el más estrecho que se sigue usando, y donde todo aparece antes. */
const MOVIL = { width: 375, height: 667 };

/**
 * Campos visibles cuya letra medida es menor de 16 px.
 *
 * Se lee de `getComputedStyle`, que da el valor YA RESUELTO, y no de la hoja: es
 * la única forma de ver el fallo que motivó esto —`font-size: 1rem` mide 14 px
 * cuando la raíz del navegador está en 14— y el umbral de iOS son 16 px
 * absolutos, no «una vez el tamaño base».
 */
const CAMPOS_PEQUENOS = `(() => {
  const fuera = ['checkbox','radio','range','hidden','submit','button','file','image','color'];
  const malos = [];
  for (const el of document.querySelectorAll('input, select, textarea')) {
    const t = (el.getAttribute('type') || 'text').toLowerCase();
    if (fuera.includes(t)) continue;
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) continue;
    const px = parseFloat(getComputedStyle(el).fontSize);
    if (px < 16) malos.push(el.tagName.toLowerCase() + '[' + t + '] ' + px + 'px'
      + (el.getAttribute('name') ? ' name=' + el.getAttribute('name') : ''));
  }
  return malos;
})()`;

const TODAS = Object.keys(APPS) as AppKey[];

test.describe('vista móvil', () => {
  test.use({ viewport: MOVIL });

  for (const app of TODAS) {
    test(`${app}: ningún campo por debajo de 16px (zoom de iOS)`, async ({ page }) => {
      test.slow(); // el dev-server puede recompilar en frío
      await gotoApp(page, app);
      const pequenos = (await page.evaluate(CAMPOS_PEQUENOS)) as string[];
      // El mensaje nombra los campos: un «esperaba 0, hubo 3» obligaría a
      // reproducirlo a mano para saber cuáles.
      expect(pequenos, `campos que amplían la página en ${app}: ${pequenos.join(', ')}`).toEqual([]);
    });
  }

  for (const app of TODAS) {
    test(`${app}: el menú lateral se cierra al usarlo`, async ({ page }) => {
      test.slow();
      await gotoApp(page, app);

      // El botón que abre el menú se reconoce por CONTROLAR algo, no por su id:
      // lo genera `useId` y no contiene nada estable.
      const toggle = page.locator('button[aria-controls][aria-expanded]').first();
      test.skip((await toggle.count()) === 0, `${app} no monta menú lateral en móvil`);

      await toggle.click();
      const drawer = page.locator('aside.sytue-drawer-in');
      await expect(drawer).toBeVisible({ timeout: 5_000 });

      // Se activa el primer control del menú. Cuál da igual: lo que se fija es
      // que USAR el menú lo cierra, no qué hace ese control en concreto.
      const dentro = drawer.locator('a[href], button:not([data-mantiene-nav])').first();
      test.skip((await dentro.count()) === 0, `el menú de ${app} está vacío`);
      await dentro.click();

      await expect(
        drawer,
        `el menú de ${app} sigue abierto y tapa lo que se acaba de abrir`,
      ).toBeHidden({ timeout: 5_000 });
    });
  }

  // Los controles marcados NO cierran el menú: colapsar una rama ocurre DENTRO
  // del menú y no lo termina. Sin este test, «cerrar siempre» pasaría el de
  // arriba y rompería el árbol de cuadernos sin que nadie se enterara.
  test('notes: expandir una rama del árbol NO cierra el menú', async ({ page }) => {
    test.slow();
    await gotoApp(page, 'notes');
    const toggle = page.locator('button[aria-controls][aria-expanded]').first();
    test.skip((await toggle.count()) === 0, 'notes no monta menú lateral en móvil');
    await toggle.click();
    const drawer = page.locator('aside.sytue-drawer-in');
    await expect(drawer).toBeVisible({ timeout: 5_000 });

    const chevron = drawer.locator('[data-mantiene-nav]').first();
    test.skip((await chevron.count()) === 0, 'no hay ninguna rama con hijos que desplegar');
    await chevron.click();
    await expect(drawer, 'desplegar una rama cerró el menú').toBeVisible();
  });
});
