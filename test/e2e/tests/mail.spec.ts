// e2e de Sytue Mail: carpetas del sistema, envío por el BFF y sidebar sin recortes.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp, sidebarNav } from './helpers';

/** Sidebar de carpetas (único <nav> de la página; el label viene de @sytue/ui). */
function sidebar(page: import('@playwright/test').Page) {
  return sidebarNav(page);
}

test.describe('Sytue Mail', () => {
  test('carpetas del sistema visibles vía /api/mailboxes', async ({ page }) => {
    // Primer hit del run: en frío el dev-server compila la página + la ruta API
    // antes de responder (misma latencia observada en /api/send): triplica el
    // timeout del test y dale holgura a la espera de la respuesta.
    test.slow();
    const sinErrores = expectNoConsoleErrors(page);
    // La isla MailApp pide /api/mailboxes al montar: capturamos esa respuesta real.
    const respuesta = page.waitForResponse(
      (r) => r.url().includes('/api/mailboxes') && r.request().method() === 'GET',
      { timeout: 120_000 },
    );
    await gotoApp(page, 'mail');

    const res = await respuesta;
    expect(res.status(), 'el BFF /api/mailboxes debe responder 200').toBe(200);
    const { mailboxes } = (await res.json()) as { mailboxes: Array<{ role?: string }> };
    const roles = mailboxes.map((m) => m.role);
    for (const role of ['inbox', 'drafts', 'sent', 'junk', 'trash']) {
      expect(roles, `falta la carpeta de sistema con role=${role}`).toContain(role);
    }

    // Y sus etiquetas en español, visibles en la sidebar.
    for (const etiqueta of ['Recibidos', 'Borradores', 'Enviados', 'Spam', 'Papelera']) {
      await expect(sidebar(page).getByText(etiqueta, { exact: true })).toBeVisible({
        timeout: 15_000,
      });
    }
    sinErrores();
  });

  test('POST /api/send a admin@sytue.localhost responde ok', async ({ page }) => {
    // El primer hit a /api/send en el dev-server (compilación en frío de la ruta
    // + primer envío JMAP) puede superar los 45s del timeout base: triplica.
    test.slow();
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'mail');

    // Asunto único incluso entre workers paralelos (--repeat-each corre en
    // paralelo con workers=2, y Date.now() puede coincidir al milisegundo).
    const asunto = `e2e mail.spec ${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
    const resultado = await page.evaluate(async (subject) => {
      // fetch same-origin: viaja la cookie de sesión del BFF, sin tokens en el test.
      const res = await fetch('/api/send', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          to: [{ email: 'admin@sytue.localhost' }],
          subject,
          textBody: 'Correo de humo del spec e2e de Sytue Mail.',
        }),
      });
      return { status: res.status, text: await res.text() };
    }, asunto);

    expect(resultado.status, `cuerpo de la respuesta: ${resultado.text}`).toBe(200);
    expect(JSON.parse(resultado.text)).toEqual({ ok: true });
    sinErrores();
  });

  test('la sidebar no está recortada: «CARPETAS» completo', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'mail');

    // El DOM dice «Carpetas»; el uppercase (CARPETAS) lo aplica CSS (.sytue-overline).
    const titulo = sidebar(page).getByText('Carpetas', { exact: true });
    await expect(titulo).toBeVisible({ timeout: 15_000 });
    // Íntegro en pantalla: ratio 1 falla si algún ancestro con overflow lo recorta.
    await expect(titulo).toBeInViewport({ ratio: 1 });
    // Y el texto no desborda su propia caja (sidebar demasiado estrecha).
    const desborde = await titulo.evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(desborde, 'el texto del encabezado desborda su caja (recorte)').toBeLessThanOrEqual(1);
    sinErrores();
  });
});
