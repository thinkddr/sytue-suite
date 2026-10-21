// e2e de Sytue Rooms: canal General auto-aprovisionado, envío/lectura de mensajes
// por el BFF y conexión del WebSocket de tiempo real.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp, APPS } from './helpers';

const BASE = APPS.rooms.replace(/\/$/, '');
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

interface Canal {
  id: string;
  name: string;
}
interface Mensaje {
  id: string;
  body: string;
  author: { id: string; name: string };
}

/** Canal General vía el BFF (page.request comparte la cookie de sesión del contexto). */
async function canalGeneral(page: import('@playwright/test').Page): Promise<Canal> {
  const res = await page.request.get(`${BASE}/api/channels`);
  expect(res.status(), 'el BFF /api/channels debe responder 200').toBe(200);
  const { channels } = (await res.json()) as { channels: Canal[] };
  const general = channels.find((c) => c.name === 'General');
  expect(general, 'falta el canal General auto-aprovisionado').toBeTruthy();
  return general!;
}

test.describe('Sytue Rooms — mensajería', () => {
  test('GET /api/channels responde 200 con el canal General', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page);
    // La isla ChatApp pide /api/channels al montar: capturamos esa respuesta real.
    const respuesta = page.waitForResponse(
      (r) => r.url().includes('/api/channels') && r.request().method() === 'GET',
    );
    await gotoApp(page, 'rooms');

    const res = await respuesta;
    expect(res.status(), 'el BFF /api/channels debe responder 200').toBe(200);
    const { channels } = (await res.json()) as { channels: Canal[] };
    expect(
      channels.map((c) => c.name),
      'falta el canal General auto-aprovisionado',
    ).toContain('General');
    sinErrores();
  });

  test('POST /api/send publica y GET /api/messages lo devuelve', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'rooms');
    const general = await canalGeneral(page);

    // Cuerpo único para no chocar con otras ejecuciones/tests (el sufijo
    // aleatorio cubre repeticiones paralelas en el mismo milisegundo).
    const cuerpo = `e2e chat.spec ${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const envio = await page.request.post(`${BASE}/api/send`, {
      data: { channelId: general.id, body: cuerpo },
    });
    expect(envio.status(), 'el BFF /api/send debe responder 200').toBe(200);
    const { message } = (await envio.json()) as { message: Mensaje };
    expect(message.body).toBe(cuerpo);
    // El autor debe ser un nombre legible, no el sub OIDC (UUID) de relleno.
    expect(message.author.name, 'author.name vacío').toBeTruthy();
    expect(message.author.name, 'author.name es un UUID (falta author_name)').not.toMatch(UUID_RE);

    // El histórico del canal devuelve el mensaje recién enviado.
    const lectura = await page.request.get(
      `${BASE}/api/messages?channelId=${encodeURIComponent(general.id)}`,
    );
    expect(lectura.status(), 'el BFF /api/messages debe responder 200').toBe(200);
    const { messages } = (await lectura.json()) as { messages: Mensaje[] };
    const recibido = messages.find((m) => m.body === cuerpo);
    expect(recibido, 'el mensaje enviado no aparece en /api/messages').toBeTruthy();
    expect(recibido!.author.name, 'author.name es un UUID en el histórico').not.toMatch(UUID_RE);
    sinErrores();
  });

  test('el WebSocket de tiempo real conecta sin errores', async ({ page }) => {
    // Colecciona solo fallos de conexión WebSocket (mensaje nativo de Chromium).
    const erroresWs: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error' && /WebSocket connection .* failed/i.test(msg.text())) {
        erroresWs.push(msg.text().slice(0, 300));
      }
    });
    // Registrar ANTES de navegar: la isla abre el WS nada más montar. El
    // predicado descarta el WebSocket del HMR de Astro/Vite (pathname '/').
    const wsCreado = page.waitForEvent('websocket', {
      predicate: (ws) => new URL(ws.url()).pathname.endsWith('/ws'),
      timeout: 30_000,
    });
    await gotoApp(page, 'rooms');

    const ws = await wsCreado;
    // Señal determinista de upgrade completado (sin sleeps): el indicador de
    // conexión de la isla solo muestra «En línea» cuando el WS dispara 'open'.
    // El filtro por texto lo distingue de otros role="status" (spinners, etc.).
    await expect(
      page.getByRole('status').filter({ hasText: 'En línea' }),
      'el indicador de conexión no llegó a «En línea»',
    ).toBeVisible({ timeout: 15_000 });
    expect(ws.isClosed(), 'el WebSocket se cerró nada más abrirse').toBe(false);
    expect(erroresWs, 'fallos de conexión WebSocket en consola').toEqual([]);
  });
});
