// e2e del portal Sytue One (one.sytue.localhost): saludo, cuadrícula de apps,
// accesos rápidos, consola limpia y destino de los enlaces.
// Solo lectura: no crea datos, cada test es independiente y re-ejecutable.
import { test, expect } from '@playwright/test';
import { gotoApp, expectNoConsoleErrors } from './helpers';

/** Apps que deben aparecer como tarjeta en la cuadrícula «Aplicaciones». */
const APPS_EN_CUADRICULA = [
  'Mail',
  'Drive',
  'Calendar',
  'Contacts',
  'Docs',
  'Rooms',
  'Cloud',
  'Console',
  'ID',
] as const;

/** Accesos rápidos cross-app del portal. */
const ACCESOS_RAPIDOS = ['Redactar correo', 'Nuevo documento', 'Subir a Drive', 'Crear evento'];

test.describe('Portal Sytue One', () => {
  test('muestra el saludo personalizado', async ({ page }) => {
    await gotoApp(page, 'one');
    // «Buenos días/Buenas tardes/Buenas noches, <nombre>» según la hora.
    // Regex tolerante con/sin tilde y exige un nombre tras la coma.
    await expect(
      page.getByRole('heading', { level: 1, name: /^Buen(os d[ií]as|as tardes|as noches), \S+/ }),
    ).toBeVisible();
  });

  test('la cuadrícula «Aplicaciones» lista las apps de la suite', async ({ page }) => {
    await gotoApp(page, 'one');
    const cuadricula = page.locator('section[aria-labelledby="apps-heading"]');
    await expect(cuadricula.getByRole('heading', { name: 'Aplicaciones' })).toBeVisible();
    for (const app of APPS_EN_CUADRICULA) {
      // aria-label de cada tarjeta: «Sytue <App> — <descripción>».
      await expect(
        cuadricula.getByRole('link', { name: new RegExp(`^Sytue ${app}\\b`) }),
        `tarjeta de Sytue ${app}`,
      ).toBeVisible();
    }
  });

  test('ofrece los accesos rápidos cross-app', async ({ page }) => {
    await gotoApp(page, 'one');
    const seccion = page.locator('section[aria-label="Accesos rápidos"]');
    await expect(seccion.getByRole('heading', { name: /Accesos r[aá]pidos/ })).toBeVisible();
    for (const acceso of ACCESOS_RAPIDOS) {
      await expect(seccion.getByRole('link', { name: acceso }), `acceso «${acceso}»`).toBeVisible();
    }
  });

  test('carga sin errores de consola reales', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page); // antes de navegar
    await gotoApp(page, 'one');
    // Comprobación de que la página rindió de verdad antes de dar la consola por limpia.
    await expect(page).toHaveTitle(/Sytue One/);
    await expect(page.getByRole('heading', { level: 1, name: /^Buen/ })).toBeVisible();
    sinErrores();
  });

  // Regresión (arreglado 2026-07-04): `suiteApex` (packages/config/src/tokens.ts) caía a
  // `sytue.eu` en SSR (sin `location`) y la hidratación de Preact NO re-aplica los href,
  // así que TODAS las tarjetas y accesos (que derivan de `appDomains`) enlazaban a
  // producción. Ahora el apex se deriva de PUBLIC_OIDC_ISSUER, horneado igual en SSR y
  // cliente. Este spec fija que ningún enlace se escape del apex del entorno (dev).
  test('los enlaces de tarjetas y accesos apuntan a *.sytue.localhost', async ({ page }) => {
    await gotoApp(page, 'one');
    const enlaces = page.locator(
      'section[aria-labelledby="apps-heading"] a, section[aria-label="Accesos rápidos"] a',
    );
    // Espera activa (sin sleeps) a que ambas secciones hayan rendido sus enlaces.
    await expect
      .poll(() => enlaces.count(), { message: 'enlaces de tarjetas y accesos rendidos' })
      .toBeGreaterThanOrEqual(APPS_EN_CUADRICULA.length + ACCESOS_RAPIDOS.length);
    const hrefs = await enlaces.evaluateAll((as) => as.map((a) => (a as HTMLAnchorElement).href));
    for (const href of hrefs) {
      expect(new URL(href).hostname, `enlace fuera del entorno dev: ${href}`).toMatch(
        /\.sytue\.localhost$/,
      );
    }
  });
});
