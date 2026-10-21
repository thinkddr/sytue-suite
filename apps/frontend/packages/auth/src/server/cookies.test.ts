/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA COOKIE DE DISPOSITIVO SE REENVÍA CON EL NOMBRE QUE EL SERVICIO ESPERA.
 *
 * En HTTPS Sytue ID la emite como `__Host-sytue_device`; en dev HTTP, sin
 * prefijo. Del otro lado, `r.Cookie` busca el nombre EXACTO. La Consola leía
 * `sytue_device` a secas y montaba la cabecera con ese nombre, así que EN
 * PRODUCCIÓN reenviaba una cookie que el servicio no encontraba: nunca se
 * iluminaba «este dispositivo» y marcar de confianza respondía siempre «no se ha
 * podido identificar».
 *
 * No se ve en desarrollo —ahí el nombre sin prefijo es el bueno— ni deja rastro
 * en los logs: la lista carga bien y solo falta una marca. De ahí el test.
 */
import { describe, expect, test } from 'bun:test';
import { deviceCookieHeader } from './cookies.js';

describe('deviceCookieHeader', () => {
  test('en HTTPS reenvía el nombre con prefijo __Host-', () => {
    const get = (n: string) => (n === '__Host-sytue_device' ? 'v1' : undefined);
    expect(deviceCookieHeader(get)).toEqual({ cookie: '__Host-sytue_device=v1' });
  });

  test('en dev HTTP reenvía el nombre simple', () => {
    const get = (n: string) => (n === 'sytue_device' ? 'v2' : undefined);
    expect(deviceCookieHeader(get)).toEqual({ cookie: 'sytue_device=v2' });
  });

  test('sin cookie no manda cabecera: la lista sigue cargando', () => {
    expect(deviceCookieHeader(() => undefined)).toEqual({});
  });

  test('con las dos gana la de HTTPS, que es la que emitió este despliegue', () => {
    expect(deviceCookieHeader(() => 'x')).toEqual({ cookie: '__Host-sytue_device=x' });
  });
});
