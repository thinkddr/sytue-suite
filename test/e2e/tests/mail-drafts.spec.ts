// e2e de Sytue Mail — Borradores (end-to-end): redacta con asunto único, guarda el
// borrador, lo verifica en Borradores, lo reabre (rehidrata asunto/cuerpo) y lo
// envía; comprueba que sale de Borradores y se entrega en Recibidos (loopback).
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
//
// ESTADO: test.fixme — REQUIERE REBUILD del contenedor `mail`. La feature YA está
// implementada de punta a punta (Go + SDK + BFF + UI), pero el binario Go en
// ejecución es anterior a los nuevos endpoints REST de borradores:
//   • Go (NUEVO): POST /api/v1/drafts, PUT /api/v1/drafts/{id}, DELETE
//     /api/v1/drafts/{id} en services/mail/internal/jmap (handlers.go/jmap.go).
//     Componen el sobre desde el raw RFC 5322 del cliente y lo materializan en la
//     carpeta Drafts (\Draft) con store.InsertMessage, sin encolar el envío.
//     `go build ./services/mail/...` y `go test ./services/mail/...` en verde.
//   • SDK @sytue/api-clients: saveDraft / updateDraft / deleteDraft / readDraft
//     (+ parseDraftContent). BFF: apps/mail/src/pages/api/drafts.ts. UI:
//     ComposeModal («Guardar borrador» + autoguardado al cerrar con contenido) y
//     MailApp (reabrir un borrador en el compositor; al enviarlo se descarta).
// El front/BFF/SDK ya están vivos (se consumen desde fuente con HMR): un POST al
// BFF /api/drafts propaga correctamente hasta el Go y hoy responde 502 porque el
// contenedor `mail` devuelve 404 (endpoint aún no compilado).
//
// TRAS EL REBUILD de `mail`: cambiar `test.fixme` → `test` y pasa sin más (los
// selectores de abajo son los reales de la UF ya mergeada).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp, sidebarNav } from './helpers';

/** Sidebar de carpetas (único <nav> de la página; el label viene de @sytue/ui). */
function sidebar(page: import('@playwright/test').Page) {
  return sidebarNav(page);
}

test.describe('Sytue Mail · Borradores', () => {
  test('guardar → verificar en Borradores → reabrir → enviar → entregado', async ({ page }) => {
    // Primer hit en frío: el dev-server compila páginas + rutas API (compose,
    // drafts, send) y hay un salto de entrega loopback de por medio. Amplía margen.
    test.slow();
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'mail');

    // Asunto único por ejecución (incluso con --repeat-each en paralelo): Date.now
    // puede coincidir al milisegundo entre workers, de ahí el sufijo aleatorio.
    const asunto = `e2e borrador ${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
    const cuerpo = 'Borrador de humo del spec e2e de Sytue Mail.';

    // --- Redactar → rellenar → Guardar borrador ---
    await sidebar(page).getByRole('button', { name: 'Redactar' }).click();
    const modal = page.getByRole('dialog');
    await expect(modal.locator('#compose-subject')).toBeVisible({ timeout: 20_000 });
    await modal.locator('#compose-to').fill('admin@sytue.localhost');
    await modal.locator('#compose-subject').fill(asunto);
    await modal.locator('#compose-body').fill(cuerpo);
    await modal.getByRole('button', { name: 'Guardar borrador' }).click();
    // El modal se cierra al guardar; el campo desaparece del DOM.
    await expect(page.locator('#compose-subject')).toBeHidden({ timeout: 15_000 });

    // --- Aparece en la carpeta Borradores (el asunto vive en la fila) ---
    await sidebar(page).getByText('Borradores', { exact: true }).click();
    const filaBorrador = page.getByText(asunto);
    await expect(filaBorrador).toBeVisible({ timeout: 15_000 });

    // --- Reabrir el borrador: rehidrata asunto y cuerpo para editarlo o enviarlo ---
    await filaBorrador.click();
    await expect(modal.locator('#compose-subject')).toHaveValue(asunto, { timeout: 15_000 });
    // El cuerpo es RICH-TEXT (contenteditable), no un <textarea>: toHaveValue
    // falla con «Not an input element».
    await expect(modal.locator('#compose-body')).toHaveText(cuerpo);

    // --- Enviar el borrador: se descarta de Borradores y se entrega ---
    const enviado = page.waitForResponse(
      (r) => r.url().includes('/api/send') && r.request().method() === 'POST',
      { timeout: 120_000 },
    );
    await modal.getByRole('button', { name: 'Enviar' }).click();
    const resSend = await enviado;
    expect(resSend.status(), 'el BFF /api/send debe responder 200').toBe(200);
    await expect(page.getByRole('dialog')).toBeHidden({ timeout: 15_000 });

    // --- Se entrega en Recibidos (loopback ~pocos segundos): recarga y aparece ---
    const filaEntregada = page.getByText(asunto).first();
    await expect(async () => {
      await gotoApp(page, 'mail'); // recarga: la lista de Recibidos no hace polling
      await expect(filaEntregada).toBeVisible({ timeout: 8_000 });
    }).toPass({ intervals: [2_000, 3_000, 3_000, 5_000], timeout: 90_000 });

    // --- Y ya NO está en Borradores (el envío lo descartó) ---
    await sidebar(page).getByText('Borradores', { exact: true }).click();
    await expect(page.getByText(asunto)).toHaveCount(0, { timeout: 15_000 });

    sinErrores();
  });
});
