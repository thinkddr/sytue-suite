/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ciclo COMPLETO de una llamada nacida en un canal de Sytue Rooms:
 *
 *   canal → «Llamar» → sala de Meet → entrar (medios reales) → salir →
 *   el SFU avisa de que la sala terminó → Rooms publica la tarjeta en el canal.
 *
 * Es el único camino que ejercita de punta a punta la cadena Rooms→Meet→LiveKit→
 * Rooms, y cada uno de sus eslabones ha estado roto por separado sin que ningún
 * test unitario lo notara: el SFU no tenía configurado a quién avisar, sus avisos
 * se rechazaban por el formato de los enteros, y `meet` no recibía el secreto del
 * canal interno, así que abrir la sala de un canal devolvía 403.
 *
 * El navegador entra con cámara y micrófono FALSOS (flags de Chromium), no sin
 * dispositivos: así el participante publica pistas de verdad y LiveKit ve una
 * sesión real, que es lo que dispara los eventos del ciclo.
 */
import { expect, test, type Browser, type Page } from '@playwright/test';
import { STORAGE_STATE } from '../playwright.config';
import { APPS } from './helpers';

const ROOMS = 'https://rooms.sytue.localhost/'; // host canónico (chat.* y meet.* redirigen aquí)
const T = 60_000;

/** Contexto con medios sintéticos y permisos ya concedidos. */
async function contextoConMedios(browser: Browser) {
  return browser.newContext({
    ignoreHTTPSErrors: true,
    permissions: ['camera', 'microphone'],
    storageState: STORAGE_STATE,
  });
}

/**
 * Tarjetas de llamada del canal. NO son texto: son mensajes de sistema `type=call`.
 * La de inicio llega con el cuerpo vacío y la de FIN con la duración en segundos, que
 * es lo que publica la videollamada de Sytue Rooms al cerrarse la sala.
 */
async function tarjetasDeLlamada(
  page: Page,
  channelId: string,
): Promise<{ body: string; type?: string }[]> {
  const res = await page.request.get(`${ROOMS}api/messages?channelId=${channelId}`, { timeout: T });
  if (!res.ok()) return [];
  const { messages } = (await res.json()) as { messages?: { body?: string; type?: string }[] };
  return (messages ?? [])
    .filter((m) => m.type === 'call')
    .map((m) => ({ body: m.body ?? '', type: m.type }));
}

test.describe('Sytue Rooms — ciclo de una llamada de canal', () => {
  test('entrar y salir de la llamada de un canal deja su tarjeta en el canal', async ({
    page,
    browser,
  }) => {
    test.slow(); // conexión WebRTC real + cierre de sala por inactividad

    // El canal de siempre: el que crea el alta de la organización.
    const canales = await page.request.get(`${ROOMS}api/channels`, { timeout: T });
    expect(canales.status(), 'GET /api/channels').toBe(200);
    const { channels } = (await canales.json()) as { channels: { id: string; name: string }[] };
    const canal = channels.find((c) => /general/i.test(c.name)) ?? channels[0];
    expect(canal, 'hace falta al menos un canal').toBeTruthy();

    const antes = await tarjetasDeLlamada(page, canal.id);

    // 1) y 2) Entrar en la llamada del canal POR LA INTERFAZ, con cámara y micrófono
    // sintéticos. El deep-link `?call=<canal>` es el que compone el intent del Hub, así
    // que es el mismo camino que recorre quien pulsa «Llamar» desde otra app.
    const ctx = await contextoConMedios(browser);
    try {
      const vista = await ctx.newPage();
      await vista.goto(`${ROOMS}?call=${encodeURIComponent(canal.id)}`, { timeout: T });

      // La vista de sala solo se monta cuando el handshake con el SFU termina.
      await expect(vista.locator('[title="Tú"]')).toBeVisible({ timeout: 60_000 });

      // 3) Salir: al quedarse vacía, el SFU cierra la sala y avisa a Rooms.
      await vista.getByRole('button', { name: 'Salir' }).click();
    } finally {
      await ctx.close();
    }

    // 4) La tarjeta de FIN aparece en el canal de origen: mensaje de sistema
    //    `type=call` con la duración. El cierre lo decide el SFU cuando la sala se
    //    queda vacía, así que se espera a que llegue en vez de asumir el instante.
    await expect
      .poll(
        async () => {
          const t = await tarjetasDeLlamada(page, canal.id);
          return t.filter((m) => /^\d+$/.test(m.body)).length;
        },
        { timeout: 180_000, intervals: [3_000] },
      )
      .toBeGreaterThan(antes.filter((m) => /^\d+$/.test(m.body)).length);
  });
});
