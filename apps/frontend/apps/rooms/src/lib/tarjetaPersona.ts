/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que sabe la tarjeta de persona de Rooms (D-ROOM-04): su hora local y su correo
 * del directorio de la organización. Funciones puras y una caché, sin DOM, para
 * probarlas con `bun test`.
 *
 * La HORA sale de la zona IANA que su dispositivo manda en cada latido de presencia
 * (Hub), no de una ficha: es la de dónde está ahora. Sin zona —desconectado, o el
 * Hub sin Dragonfly— la tarjeta no enseña hora antes que inventar una.
 *
 * El CORREO sale del directorio de Sytue ID (`/api/directorio`), que solo lista a la
 * gente activa de la propia organización y sus buzones de aquí: a alguien de fuera no
 * se le pone correo, y no es un fallo.
 */
import { formatDateTime, type Locale } from '@sytue/i18n';

/** «14:05» en la zona `tz` y el formato del idioma; null si la zona no es válida. */
export function horaLocal(
  locale: Locale,
  tz: string | undefined,
  ahora: Date = new Date(),
): string | null {
  if (!tz) return null;
  try {
    return formatDateTime(locale, ahora, { hour: '2-digit', minute: '2-digit', timeZone: tz });
  } catch {
    return null;
  }
}

/** Una fila del directorio, reducida a lo que pinta la tarjeta. */
export interface FichaDirectorio {
  sub: string;
  email: string;
  title?: string;
}

let pendiente: Promise<Map<string, FichaDirectorio>> | null = null;

/**
 * El directorio, pedido UNA vez por pestaña y solo al abrir la primera tarjeta: la
 * plantilla cambia una vez al mes y no compensa pedirla con cada clic. Si falla, se
 * olvida el intento para que la siguiente tarjeta lo vuelva a probar.
 */
export function directorio(
  pedir: () => Promise<Response> = () =>
    fetch('/api/directorio', { credentials: 'include', headers: { accept: 'application/json' } }),
): Promise<Map<string, FichaDirectorio>> {
  pendiente ??= pedir()
    .then(async (res) => {
      if (!res.ok) throw new Error(`directorio ${res.status}`);
      const { people = [] } = (await res.json()) as { people?: FichaDirectorio[] };
      return new Map(people.map((p) => [p.sub, p]));
    })
    .catch((err: unknown) => {
      pendiente = null;
      throw err;
    });
  return pendiente;
}

/** Solo para tests: vacía la caché del directorio. */
export function olvidarDirectorio(): void {
  pendiente = null;
}
