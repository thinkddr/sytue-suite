// Feature END-TO-END: DESBLOQUEAR Sytue Admin (admin.sytue.localhost).
//
// El admin de dev (admin@sytue.localhost) es admin CON TOTP (seed de id). Al
// hacer login, el IdP exige el segundo factor (step-up MFA, Role.RequiresMFA);
// lo cumplimos calculando el código TOTP del secreto de dev (helpers.codigoTOTP,
// vía loginEnIdP). Con la sesión elevada (amr=mfa), Console YA NO devuelve 403
// sino el panel de administración.
//
// SIN storageState de admin: contexto limpio para ejercer el login con MFA de
// principio a fin, sin reutilizar la sesión compartida del harness.
//
// FIXME(requiere rebuild id): este contrato depende del nuevo seed de Sytue ID
// (admin + MFA + secreto TOTP en services/id/cmd/id/seed.go). Hasta que el
// contenedor `id` se reconstruya y se re-ejecute el seed (make dev-seed), el
// usuario de dev sigue siendo `member` sin TOTP y Console responde 403. Tras el
// rebuild + reseed, cambia `test.fixme` por `test` y pasa en verde.
import { expect, test } from '@playwright/test';
import { APPS, irAlFormularioDelIdP, loginEnIdP } from './helpers';

const EMAIL = process.env.SYTUE_E2E_EMAIL || 'admin@sytue.localhost';
const PASS = process.env.SYTUE_E2E_PASS || 'sytue-dev-admin';

test.use({
  storageState: { cookies: [], origins: [] },
  navigationTimeout: 90_000, // cadenas OIDC sobre dev-servers fríos (como login.spec.ts)
});

test('console: admin con step-up MFA (TOTP) ve el panel — ya no 403', async ({ page }) => {
  // Login por el portal + reintentos del lockout compartido + segundo factor +
  // primera compilación de Console en el dev-server: presupuesto amplio.
  test.setTimeout(300_000);

  // 1) Login por el portal. loginEnIdP intercala el segundo factor (TOTP de dev)
  //    porque el admin es un rol elevado con MFA, y aterriza autenticado.
  await irAlFormularioDelIdP(page);
  await loginEnIdP(page, EMAIL, PASS);
  await page.waitForURL((url) => url.hostname === 'one.sytue.localhost', {
    waitUntil: 'domcontentloaded',
  });

  // 2) Con la sesión elevada (amr incluye mfa), la cadena OIDC de Console supera
  //    el step-up y sirve el panel: el status final ya NO es 403.
  const resp = await page.goto(APPS.console, { waitUntil: 'domcontentloaded' });
  expect(resp, 'la navegación a Console debe producir una respuesta').not.toBeNull();
  expect(new URL(page.url()).hostname, 'no debe rebotar a login').toBe('admin.sytue.localhost');
  expect(resp!.status(), 'Console ya no debe denegar por rol/MFA (era 403)').not.toBe(403);
  expect(resp!.status(), 'status final de Console').toBeLessThan(400);

  // 3) Se muestra el panel de administración, no la página de denegación.
  await expect(page.locator('body')).not.toContainText('No autorizado para este recurso.');
  await expect(page).toHaveTitle(/Sytue Admin/);
  await expect(page.locator('body')).toContainText('Administración');
});
