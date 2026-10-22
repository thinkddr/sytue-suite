// Spec e2e de Sytue Rooms en tiempo real: dos sesiones (contextos de navegador
// independientes, mismo admin) en el canal General. Un mensaje enviado en A
// aparece en B por WebSocket, SIN recargar, en <5 s; y una reacción se registra
// vía el BFF (-> servicio Go) y muestra su conteo.
//
// Mecanismo REAL (apps/chat/src/client/chatApi.ts + services/chat):
// la isla abre el WS con el ticket del BFF y envía un frame `subscribe
// {channel_id}` por canal; el servicio Go difunde a la sala del canal vía el
// outbox + LISTEN/NOTIFY (services/chat/internal/ws + service/events.go). El
// envío usa el BFF POST /api/send; la reacción, POST /api/reactions -> PUT/DELETE
// /messages/{id}/reactions/{emoji} en el servicio Go.
//
// Nota (mismo usuario en A y B): la reacción propia se ignora en el WS para no
// duplicar el conteo (subject === yo), así que su propagación en vivo se cubre
// con distintos usuarios, no aquí; este spec verifica el conteo en la pestaña que
// reacciona y el round-trip real contra el backend (respuesta 200 del BFF).
import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { STORAGE_STATE } from '../playwright.config';
import { APPS } from './helpers';

const CHAT = APPS.rooms; // https://chat.sytue.localhost/

/** Abre Chat y espera WS «En línea» + canal General activo (composer listo). */
async function abrirChatEnGeneral(page: Page): Promise<void> {
  await page.goto(CHAT, { waitUntil: 'domcontentloaded' });
  // WS abierto: el indicador de conexión muestra «En línea» (sr-only al estar
  // sano; el rol status sigue siendo visible por el punto de estado).
  await expect(
    page.getByRole('status').filter({ hasText: 'En línea' }),
    'el WS no llegó a «En línea»',
  ).toBeVisible({ timeout: 30_000 });
  // Canal General activo: cabecera + composer visibles. Implica canales cargados
  // y, por tanto, que la suscripción al canal ya se envió por el WS.
  await expect(page.getByRole('heading', { name: 'General', level: 1 })).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByLabel('Escribe un mensaje en General')).toBeVisible();
}

test.describe('Sytue Rooms en tiempo real', () => {
  test('entrega en vivo por WS entre dos pestañas + reacción con conteo', async ({
    page,
    browser,
  }) => {
    test.slow(); // dev-server en frío + dos contextos + round-trip WS.

    // Segundo contexto con el MISMO storageState del setup; las opciones del
    // config (ignoreHTTPSErrors, viewport) no se heredan en newContext().
    let contextoB: BrowserContext | null = null;
    try {
      contextoB = await browser.newContext({
        storageState: STORAGE_STATE,
        ignoreHTTPSErrors: true,
        viewport: { width: 1440, height: 900 },
      });
      const pageB = await contextoB.newPage();

      await abrirChatEnGeneral(page); // A
      await abrirChatEnGeneral(pageB); // B

      // 1) Entrega en vivo: A envía por el composer; B lo recibe por WS.
      const cuerpo = `e2e realtime ${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const envio = page.waitForResponse(
        (r) => r.url().includes('/api/send') && r.request().method() === 'POST' && r.ok(),
        { timeout: 15_000 },
      );
      const composerA = page.getByLabel('Escribe un mensaje en General');
      await composerA.fill(cuerpo);
      await composerA.press('Enter');
      await envio;

      // A lo muestra (optimista/confirmado)…
      await expect(page.getByText(cuerpo)).toBeVisible({ timeout: 5_000 });
      // …y B lo recibe EN VIVO por el WebSocket en <5 s (sin recargar la página).
      await expect(pageB.getByText(cuerpo), 'B no recibió el mensaje por WS en <5 s').toBeVisible({
        timeout: 5_000,
      });

      // 2) Reacción: A reacciona 👍 (BFF -> servicio Go) y ve el conteo.
      const fila = page.locator('div.group').filter({ hasText: cuerpo });
      const reaccion = page.waitForResponse(
        (r) => r.url().includes('/api/reactions') && r.request().method() === 'POST',
        { timeout: 15_000 },
      );
      await fila.hover(); // revela la barra de reacciones rápidas (pointer-events)
      await fila.getByRole('button', { name: 'Reaccionar con 👍' }).click();
      const resReac = await reaccion;
      expect(resReac.status(), 'el BFF /api/reactions debe responder 200').toBe(200);

      // El chip de reacción muestra emoji y conteo. Se afirma sobre el TEXTO VISIBLE,
      // no sobre el aria-label: ese nombra a quién reaccionó («Quitar tu reacción:
      // Admin Sytue») y no siempre contiene el emoji ni el número.
      await expect(
        fila.locator('button').filter({ hasText: '👍' }).filter({ hasText: '1' }),
        'no se muestra el conteo de la reacción',
      ).toBeVisible({ timeout: 5_000 });
    } finally {
      await contextoB?.close();
    }
  });
});
