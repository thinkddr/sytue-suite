// Sytue ID (id.sytue.localhost): página /account (ficha + Sesiones activas),
// cierre de sesión/dispositivos y redirección de la raíz según haya sesión SSO.
//
// La página de cuenta es server-rendered (Go, sin JS) con CSP estricta:
// default-src 'none' + style-src por hash sha256. Si los hashes de la CSP se
// desincronizan de los <style> inline (regresión ya vista), el navegador
// bloquea TODO el CSS: lo detectamos comprobando que `.logo svg` computa los
// 24px que fija loginCSS y que `ul.sess` computa el padding 0 de sessionsCSS.
import { expect, test, type Page } from '@playwright/test';
import { completarMFASiHaceFalta } from './helpers';

const ID = 'https://id.sytue.localhost';
const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';
// Usuario member del seed (sin MFA, sin rol de consola): las pruebas de
// revocación usan SUS sesiones para no quemar la sesión compartida del admin
// (storageState) de la que dependen el resto de specs del run.
const MEMBER_EMAIL = process.env.SYTUE_E2E_MEMBER_EMAIL || 'member@sytue.localhost';
const MEMBER_PASS = process.env.SYTUE_E2E_MEMBER_PASS || 'sytue-dev-admin';

/**
 * Login directo en el IdP (raíz → /login/?return_to=/account → /account),
 * resistente al lockout suave por cuenta (token bucket 5/min por email,
 * server.go/login.go) que cuenta también los logins correctos: los intentos
 * legítimos del propio run e2e pueden agotar la ráfaga y el POST cae en la
 * página de fallo. Con credenciales correctas esa es la única causa posible,
 * así que reintentamos por encima del refill (1 token/12 s); la denegación no
 * consume tokens. Completa el step-up TOTP si el IdP lo pide (admin de dev).
 */
async function loginDirectoIdP(page: Page, email: string, pass: string): Promise<void> {
  await expect(async () => {
    await page.goto(`${ID}/`);
    // Solo rellenar si estamos en el formulario: un intento anterior pudo
    // crear la sesión con redirección lenta y este goto entra directo a
    // /account (mismo guardián que auth.setup.ts y helpers.loginEnIdP).
    if (/\/login\//.test(page.url())) {
      await page.fill('input[name=email]', email);
      await page.fill('input[name=password]', pass);
      await page.click('button[type=submit]');
      await completarMFASiHaceFalta(page);
    }
    await expect(page).toHaveURL(`${ID}/account`, { timeout: 10_000 });
    // Margen ampliado: el login del admin de dev suma un segundo factor (TOTP)
    // sobre el refill del lockout (13 s), así que caben menos reintentos.
  }).toPass({ intervals: [13_000], timeout: 60_000 });
}

test.describe('Sytue ID · con sesión (storageState de admin)', () => {
  test('/account muestra Nombre/Email/Organizaciones/MFA con la marca', async ({ page }) => {
    await page.goto(`${ID}/account`);
    await expect(page).toHaveTitle(/Mi cuenta · Sytue/);
    await expect(page.getByRole('heading', { name: 'Mi cuenta' })).toBeVisible();

    // Ficha de identidad: las cuatro filas pedidas, con el email del admin.
    const ficha = page.locator('dl.rows');
    await expect(ficha).toBeVisible();
    for (const campo of ['Nombre', 'Email', 'Organizaciones', 'Verificación en dos pasos']) {
      await expect(ficha).toContainText(campo);
    }
    await expect(ficha).toContainText(EMAIL);

    // Marca Sytue ID en el shell.
    const marca = page.locator('.brand');
    await expect(marca).toContainText('Sytue');
    await expect(marca).toContainText('ID');

    // Regresión CSP style-src: con el CSS aplicado, el logo computa 24px de
    // ancho (loginCSS: .brand .logo svg{width:24px}); si la CSP bloquea los
    // <style> inline, el SVG cae a su tamaño intrínseco y esto falla.
    await expect
      .poll(() => page.locator('.logo svg').evaluate((el) => getComputedStyle(el).width), {
        message: 'CSS bloqueado por CSP (style-src): .logo svg no computa 24px',
      })
      .toBe('24px');

    // Acciones de la cuenta: volver a la suite y cerrar sesión.
    await expect(page.getByRole('link', { name: 'Cerrar sesión' })).toHaveAttribute(
      'href',
      '/oauth2/logout',
    );
  });

  test('/account lista las Sesiones activas con la sesión actual marcada', async ({ page }) => {
    await page.goto(`${ID}/account`);
    await expect(page.getByRole('heading', { name: 'Sesiones activas' })).toBeVisible();

    // Exactamente UNA fila lleva el distintivo "Esta sesión" (la del propio
    // storageState); las demás, si las hay, llevan su botón "Cerrar".
    await expect(page.locator('.sess .pill', { hasText: 'Esta sesión' })).toHaveCount(1);
    // La fila actual muestra el método de autenticación y las fechas.
    // (El locator interno de `has` se evalúa RELATIVO a la fila: `.pill`, no `.sess .pill`.)
    const filaActual = page.locator('.s-row', {
      has: page.locator('.pill', { hasText: 'Esta sesión' }),
    });
    await expect(filaActual.locator('.s-meta')).toContainText('inicio');
    await expect(filaActual.locator('.s-meta')).toContainText('caduca');
    // La fila actual NO ofrece cerrarse a sí misma.
    await expect(filaActual.getByRole('button', { name: /Cerrar la sesión/ })).toHaveCount(0);

    // Regresión CSP del <style> nuevo (sessionsCSS): ul.sess computa padding 0
    // (el UA pone 40px por defecto); si su hash falta en la CSP, esto falla.
    await expect
      .poll(() => page.locator('ul.sess').evaluate((el) => getComputedStyle(el).paddingLeft), {
        message: 'CSS bloqueado por CSP (style-src): ul.sess no computa padding 0',
      })
      .toBe('0px');
  });
});

test.describe('Sytue ID · sin sesión', () => {
  // Contexto limpio: estos tests ejercitan el flujo anónimo y el logout con
  // una sesión propia, sin quemar la sesión compartida del storageState de
  // admin (revocarla rompería al resto de specs del run).
  test.use({ storageState: { cookies: [], origins: [] } });

  test('/ redirige al login (con retorno a /account) cuando no hay sesión', async ({ page }) => {
    await page.goto(`${ID}/`);
    await expect(page).toHaveURL(/\/login\/\?return_to=(\/|%2F)account/);
    await expect(page.getByRole('heading', { name: 'Iniciar sesión' })).toBeVisible();
    await expect(page.locator('input[name=email]')).toBeVisible();
  });

  test('Cerrar sesión lleva al logout y revoca la sesión', async ({ page }) => {
    // Login fresco: la sesión que se revoca es exclusiva de este test.
    await loginDirectoIdP(page, EMAIL, PASS);

    // El enlace del pie de la ficha lleva al end_session endpoint, que pinta la
    // pantalla de cierre: pide a CADA superficie que borre su cookie host-only
    // (solo su propio host puede) antes de llevar al sitio público.
    await page.getByRole('link', { name: 'Cerrar sesión' }).click();
    await expect(page.getByRole('heading', { name: 'Cerrando sesión' })).toBeVisible();
    // Una petición de barrido por superficie de la suite, la del portal incluida.
    await expect(page.locator('img[src*="/auth/logout?scope=local"]').first()).toBeAttached();

    // Sesión revocada de verdad: la raíz vuelve a exigir login.
    await page.goto(`${ID}/`);
    await expect(page).toHaveURL(/\/login\//);
  });
});

test.describe('Sytue ID · dispositivos (sesiones del usuario member)', () => {
  // Todas las sesiones que se crean y revocan aquí son del usuario member y
  // nacen dentro del test: no tocan el storageState compartido del admin. No
  // pulsamos "Cerrar las demás sesiones" (solo comprobamos que existe) porque
  // console.spec.ts mantiene una sesión member viva en paralelo (workers: 2)
  // y cerrarla la rompería; el cierre en lote lo cubre el test de integración
  // del store (TestSessionDevicesListAndRevoke).
  test.use({ storageState: { cookies: [], origins: [] } });

  // UA distintivo del "dispositivo" víctima: no matchea ningún token de
  // deviceES (Go), así que la fila lo muestra literal y es inconfundible.
  const UA_VICTIMA = 'SytueE2E-Dispositivo-Victima';

  test('cerrar un dispositivo concreto revoca SU sesión y conserva la actual', async ({
    browser,
  }) => {
    // Sesión A (víctima), con el User-Agent distintivo.
    const ctxA = await browser.newContext({ ignoreHTTPSErrors: true, userAgent: UA_VICTIMA });
    const pageA = await ctxA.newPage();
    // Sesión B (superviviente), desde la que se cierra la A.
    const ctxB = await browser.newContext({ ignoreHTTPSErrors: true });
    const pageB = await ctxB.newPage();
    try {
      await loginDirectoIdP(pageA, MEMBER_EMAIL, MEMBER_PASS);
      await loginDirectoIdP(pageB, MEMBER_EMAIL, MEMBER_PASS);

      // B ve la sección con su propia sesión marcada y la fila del dispositivo A.
      await expect(pageB.getByRole('heading', { name: 'Sesiones activas' })).toBeVisible();
      await expect(pageB.locator('.sess .pill', { hasText: 'Esta sesión' })).toHaveCount(1);
      await expect(
        pageB.getByRole('button', { name: 'Cerrar las demás sesiones' }),
      ).toBeVisible();

      const cerrarA = pageB.getByRole('button', { name: `Cerrar la sesión de ${UA_VICTIMA}` });
      const antes = await cerrarA.count();
      expect(antes).toBeGreaterThanOrEqual(1);
      // .first() = la más reciente (orden auth_time desc) = la A de ESTE test,
      // aunque un run abortado dejara víctimas huérfanas de ejecuciones previas.
      await cerrarA.first().click();

      // PRG: el POST redirige a /account con la lista ya refrescada, sin la fila A.
      await expect(pageB).toHaveURL(`${ID}/account`);
      await expect(cerrarA).toHaveCount(antes - 1);

      // La sesión A está revocada de verdad: su contexto vuelve al login.
      await pageA.goto(`${ID}/`);
      await expect(pageA).toHaveURL(/\/login\//);

      // Y la sesión B (actual) sobrevive.
      await pageB.goto(`${ID}/account`);
      await expect(pageB.getByRole('heading', { name: 'Mi cuenta' })).toBeVisible();
      await expect(pageB.locator('.sess .pill', { hasText: 'Esta sesión' })).toHaveCount(1);
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
  });
});
