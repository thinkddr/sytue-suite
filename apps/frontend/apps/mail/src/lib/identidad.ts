/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * De qué dirección sale un correo que compone esta persona.
 *
 * PARECE OBVIO Y NO LO ES, y por no serlo el envío estuvo roto. La sesión de
 * Sytue ID trae el correo con el que alguien INICIA SESIÓN (ops@sytue.com); el
 * buzón que le sirve Sytue Mail puede estar en otra dirección
 * (ops@corporate.sytue.eu) — y en una organización con dominio propio eso no es
 * el caso raro, es el normal. El servicio rechaza con 403 cualquier `From` que no
 * sea la primaria del buzón o un alias verificado, así que el correo NO SALÍA: el
 * compositor decía «no se ha podido enviar» y nada más.
 *
 * La dirección buena la sabe el servicio y ya la publica en `/api/v1/aliases`
 * (`primary`). Se pregunta ahí y no se deduce del token ni se pide al navegador:
 * un `From` que elige el cliente es exactamente lo que el servicio prohíbe.
 */
import { mailGet } from './mailrest.js';

/** Cuánto se recuerda la dirección de un buzón. */
const TTL_MS = 5 * 60_000;

/**
 * Caché en memoria del proceso.
 *
 * Existe por el autoguardado del borrador, que compone un mensaje cada pocos
 * segundos: sin ella, cada pulsación de tecla acabaría en un viaje extra al
 * servicio. La dirección primaria de un buzón cambia como mucho una vez en la
 * vida de la cuenta, así que cinco minutos de desfase no rompen nada — y un
 * cambio real se nota al siguiente refresco, no al siguiente reinicio.
 */
const cache = new Map<string, { direccion: string; expira: number }>();

/**
 * Dirección desde la que envía `clave` (el identificador del usuario).
 *
 * `respaldo` es el correo de la sesión: se usa solo si el servicio no contesta.
 * Es peor que la primaria, pero deja el compositor funcionando para las cuentas
 * en las que ambas coinciden en vez de romper el envío para todas.
 */
export async function direccionDeEnvio(
  accessToken: string,
  clave: string,
  respaldo: string,
): Promise<string> {
  const guardada = cache.get(clave);
  if (guardada && guardada.expira > Date.now()) return guardada.direccion;
  try {
    const res = await mailGet(accessToken, '/api/v1/aliases');
    if (res.ok) {
      const body = (await res.json()) as { primary?: unknown };
      const primaria = typeof body.primary === 'string' ? body.primary.trim() : '';
      if (primaria) {
        cache.set(clave, { direccion: primaria, expira: Date.now() + TTL_MS });
        return primaria;
      }
    }
  } catch {
    // Sin servicio no hay dirección que recordar: se cae al respaldo y se vuelve
    // a intentar en la siguiente composición, no dentro de cinco minutos.
  }
  return respaldo;
}

/** Olvida lo recordado. Solo para los checks: no hay motivo de producto. */
export function olvidarDirecciones(): void {
  cache.clear();
}
