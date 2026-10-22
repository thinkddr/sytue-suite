// Login y SSO contra el IdP (id.sytue.localhost) — SIN storageState previo:
// cada test parte de un contexto limpio y hace su propio login si lo necesita.
// Cubre la regresión de la CSP: una form-action demasiado estricta en el IdP
// abortaba la cadena de redirecciones tras el POST del formulario de login.
//
// Nota de diseño: el IdP aplica lockout suave POR CUENTA en /login/submit
// (token bucket de 5 con recarga 5/min, clave "login:"+email —
// services/id/internal/http/{server.go,login.go}) y cada envío consume cupo,
// también los correctos; la cuenta admin se comparte además entre TODAS las
// ejecuciones e2e concurrentes (auth.setup.ts de cada suite). Por eso (b),
// (d) y (e) comparten UN único login resistente al lockout (loginEnIdP) y la
// prueba de contraseña inválida usa un email inexistente y único: gasta cupo
// en una clave desechable, nunca en la del admin.
import { expect, test, type Page } from '@playwright/test';
import { gotoApp, irAlFormularioDelIdP, loginEnIdP } from './helpers';

const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';

test.use({
  storageState: { cookies: [], origins: [] },
  // Margen para TODA navegación del spec (goto y waitForURL): las cadenas de
  // redirects OIDC sobre dev-servers fríos (Vite en bind-mount Docker) superan
  // a veces el default de 30 s; 90 s como en auth.setup.ts. Sin esto, los
  // page.goto de (a)/(c)/(e) y el de gotoApp en (d) abortarían a los 30 s
  // antes de que los waitForURL tuvieran ocasión de esperar.
  navigationTimeout: 90_000,
});

/** Entra por el portal, rellena el formulario del IdP y aterriza autenticado. */
async function login(page: Page): Promise<void> {
  await irAlFormularioDelIdP(page);
  await loginEnIdP(page, EMAIL, PASS);
  // Predicado por hostname: el redirect_uri lleva one.sytue.localhost en la
  // query de login y engañaría a un regex sobre la URL completa. Esperamos a
  // domcontentloaded: load tarda demasiado con los bundles del dev-server.
  await page.waitForURL((url) => url.hostname === 'one.sytue.localhost', {
    waitUntil: 'domcontentloaded',
  });
}

// (a) Sin sesión, el portal redirige al formulario del IdP con la marca.
test('sin sesión, el portal redirige al login del IdP con la marca', async ({ page }) => {
  await irAlFormularioDelIdP(page);
  await expect(page).toHaveTitle(/Sytue/);
  await expect(page.locator('input[name=email]')).toBeVisible();
});

// (b) + (d) + (e) Viaje completo con un solo login (ver nota de diseño arriba).
test('login válido: portal autenticado, SSO en mail y ficha en /account', async ({ page }) => {
  // Tres navegaciones OIDC sobre dev-servers fríos + posibles reintentos del
  // login por el lockout compartido: presupuesto propio en vez de test.slow().
  test.setTimeout(300_000);

  // (b) Credenciales válidas → aterriza autenticado en one.sytue.localhost.
  // La regresión de la CSP form-action rompía justo esta cadena de redirects.
  await login(page);
  expect(new URL(page.url()).hostname).toBe('one.sytue.localhost');
  expect(page.url()).not.toMatch(/\/login\//);

  // (d) SSO: mail no vuelve a pedir login (la cadena OIDC pasa sin formulario).
  await gotoApp(page, 'mail');
  await page.waitForURL((url) => url.hostname === 'mail.sytue.localhost', {
    waitUntil: 'domcontentloaded',
  });
  await expect(page.locator('input[name=password]')).toHaveCount(0);

  // (e) La raíz del IdP con sesión lleva a /account con los datos del usuario.
  await page.goto('https://id.sytue.localhost/', { waitUntil: 'domcontentloaded' });
  await page.waitForURL(/\/account/, { waitUntil: 'domcontentloaded' });
  await expect(page.locator('.row', { hasText: 'Nombre' }).locator('dd')).not.toBeEmpty();
  await expect(page.locator('.row', { hasText: 'Email' }).locator('dd')).toContainText(EMAIL);
});

// (c) Credenciales inválidas. Email inexistente y único a propósito: el IdP
// responde el MISMO error genérico (no enumera usuarios, login.go) y el cupo
// del lockout se consume en una clave desechable, no en la del admin.
test('contraseña inválida: permanece en el IdP y muestra el error', async ({ page }) => {
  await irAlFormularioDelIdP(page);
  await page.fill('input[name=email]', `e2e-invalido-${Date.now()}@sytue.localhost`);
  await page.fill('input[name=password]', 'contraseña-mala');
  await page.click('button[type=submit]');
  await expect(page.getByText('Email o contraseña incorrectos')).toBeVisible();
  expect(new URL(page.url()).hostname).toBe('id.sytue.localhost');
  // El IdP re-renderiza el propio formulario con el error encima (el formulario
  // ES el reintento in situ; sin callejón sin salida ni enlace aparte).
  await expect(page.locator('input[name=password]')).toBeVisible();
});
