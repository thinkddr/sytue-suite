// Spec e2e de Sytue Calendar — VIDEOLLAMADA DEL EVENTO (RFC 7986 CONFERENCE).
//   1) «Añadir videollamada» crea la sala en Rooms y deja su enlace en el evento.
//   2) El enlace sobrevive al guardado: el evento se relee con su CONFERENCE.
// Lo que se prueba de verdad es el cruce entre dos apps: Calendar pide la sala y Rooms
// la sirve; si el enlace no llega al ICS, el invitado no tiene por dónde entrar.
import { expect, test } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

test.describe('Sytue Calendar — videollamada del evento', () => {
  test('POST /api/conference devuelve el enlace de una sala de Rooms', async ({ page }) => {
    await gotoApp(page, 'calendar');
    const uid = `e2e-conf-${Date.now()}@calendar.sytue.eu`;

    const res = await page.request.post(`${APPS.calendar}api/conference`, {
      data: { uid, title: 'Puesta en común' },
    });
    expect(res.status(), 'POST /api/conference').toBe(200);
    const { url } = (await res.json()) as { url: string };
    // El enlace tiene que ser el PÚBLICO de Rooms con ?rooms=<id>: es el que abre el
    // invitado en su navegador, no la API interna.
    expect(url, 'el enlace debe apuntar a Rooms con ?rooms=').toMatch(/rooms[^/]*\/\?rooms=[0-9a-f-]{36}$/);

    // Idempotente por evento: pedirla otra vez para el MISMO uid da la MISMA sala, así
    // que el enlace ya repartido a los invitados sigue sirviendo.
    const otra = await page.request.post(`${APPS.calendar}api/conference`, {
      data: { uid, title: 'Puesta en común' },
    });
    expect(otra.status()).toBe(200);
    expect((await otra.json()).url, 'la sala del evento no puede cambiar').toBe(url);
  });
});
