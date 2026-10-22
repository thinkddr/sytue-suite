// e2e de Sytue Rooms (Chat + Meet unificados): la videollamada embebida por canal.
// Verifica (1) el rebranding a Rooms sobre el host de Chat, (2) el BFF proxy PURO
// /api/call (from-source -> join contra el servicio Go Meet, con auth cross-service:
// el token de sesion de Chat lo acepta meet-api) y (3) la IDEMPOTENCIA de la sala por
// canal — from-source reutiliza la sala viva en lugar de duplicarla (fix en Go).
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test, type Page } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp, APPS } from './helpers';

const BASE = APPS.rooms.replace(/\/$/, ''); // Rooms se sirve en el host de Chat.

interface Canal {
  id: string;
  name: string;
}
interface Conexion {
  serverUrl: string;
  token: string;
  room: string;
  identity: string;
}
interface Llamada {
  connection: Conexion;
  roomTitle: string;
}

/** Canal General vía el BFF (page.request comparte la cookie de sesión del contexto). */
async function canalGeneral(page: Page): Promise<Canal> {
  const res = await page.request.get(`${BASE}/api/channels`);
  expect(res.status(), 'el BFF /api/channels debe responder 200').toBe(200);
  const { channels } = (await res.json()) as { channels: Canal[] };
  const general = channels.find((c) => c.name === 'General');
  expect(general, 'falta el canal General auto-aprovisionado').toBeTruthy();
  return general!;
}

/** Abre (o reutiliza) la sala del canal por el BFF y devuelve el nombre de sala. */
async function abrirSala(page: Page, canal: Canal): Promise<Conexion> {
  const res = await page.request.post(`${BASE}/api/call`, {
    data: { channelId: canal.id, title: canal.name },
  });
  expect(res.status(), 'el BFF /api/call debe responder 200').toBe(200);
  return ((await res.json()) as Llamada).connection;
}

test.describe('Sytue Rooms', () => {
  test('se presenta como Rooms y ofrece «Llamar» en el canal', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'rooms');
    // Rebranding Chat -> Rooms: título de pestaña y cabecera de la barra lateral.
    await expect(page).toHaveTitle(/Sytue Rooms/);
    await expect(
      page.getByText('Sytue Rooms', { exact: true }).first(),
      'la barra lateral debe rotularse «Sytue Rooms»',
    ).toBeVisible({ timeout: 20_000 });
    // La app auto-selecciona el primer canal (ChatApp.tsx setActiveId list[0]) ->
    // su cabecera ofrece «Llamar», la entrada a la videollamada embebida.
    await expect(
      page.getByRole('button', { name: /Llamar/ }).first(),
      'la cabecera de canal debe ofrecer «Llamar»',
    ).toBeVisible({ timeout: 20_000 });
    sinErrores();
  });

  test('POST /api/call abre la sala del canal (from-source -> join) y devuelve la conexión LiveKit', async ({
    page,
  }) => {
    await gotoApp(page, 'rooms');
    const general = await canalGeneral(page);
    const conn = await abrirSala(page, general);
    // El servicio Go mintea el JWT de LiveKit y expone la URL WSS del SFU: prueba el
    // BFF proxy + los dos hops Go + la auth cross-service (sesión de Chat en meet-api).
    expect(conn.token, 'falta el JWT de LiveKit minteado por Go').toBeTruthy();
    // URL del SFU: en dev es https://rooms-sfu.sytue.localhost (Traefik termina TLS y
    // el SDK de LiveKit la trata como wss); en prod es wss://. Aceptamos ambas.
    expect(conn.serverUrl, 'falta la URL del SFU').toMatch(/^(wss?|https?):\/\//);
    expect(conn.room, 'falta el nombre de sala').toBeTruthy();

    // El JWT no basta con que exista: tiene que CONCEDER la entrada a ESTA sala y no
    // estar caducado. Un token bien formado pero sin `roomJoin` deja al usuario fuera
    // con un error del SDK que no dice por qué.
    const claims = JSON.parse(
      Buffer.from(conn.token.split('.')[1]!, 'base64url').toString('utf8'),
    ) as { video?: { roomJoin?: boolean; room?: string }; sub?: string; exp?: number };
    expect(conn.token.split('.'), 'el token debe ser un JWT (3 segmentos)').toHaveLength(3);
    expect(claims.video?.roomJoin, 'el JWT no concede roomJoin').toBe(true);
    expect(claims.video?.room, 'el JWT apunta a otra sala').toBe(conn.room);
    expect(claims.sub).toBeTruthy();
    expect(claims.exp && claims.exp * 1000).toBeGreaterThan(Date.now());
  });

  test('la sala del canal es IDEMPOTENTE: dos llamadas reutilizan la misma sala', async ({ page }) => {
    await gotoApp(page, 'rooms');
    const general = await canalGeneral(page);
    // from-source es idempotente por (kind='chat', ref_id=canal) en el servicio Go:
    // la segunda llamada devuelve la sala YA existente, no una nueva (mismo `room`).
    const primera = await abrirSala(page, general);
    const segunda = await abrirSala(page, general);
    expect(
      segunda.room,
      'la 2ª llamada creó una sala distinta -> from-source NO es idempotente',
    ).toBe(primera.room);
  });
});
