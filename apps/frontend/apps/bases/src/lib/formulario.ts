/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El formulario público de Bases: lo que la web hace SIN sesión en nombre de quien abre el
 * enlace de un formulario. La única credencial es el token del enlace y se comprueba
 * contra el servicio en cada petición (revocar corta al instante). Estas funciones son
 * del servidor del BFF: el navegador solo habla con las rutas `/bases/f/<token>/…`.
 */
import type { BasesField } from '@sytue/api-clients/bases';
import { DESTINO } from './servicio.js';

/** La forma de un token: 32 bytes en base64 sin relleno. Todo lo demás es un 404 sin llamar. */
export const RE_TOKEN = /^[A-Za-z0-9_-]{43}$/;

/**
 * Lo que cabe en un envío. El servicio corta en 96 KiB, pero este servidor LEE el cuerpo
 * antes de reenviarlo: sin su propio techo, cualquiera —la ruta no pide sesión— podría
 * hacerle cargar en memoria lo que quisiera con un token de la forma correcta.
 */
export const MAX_CUERPO_ENVIO = 96 * 1024;

export interface DefinicionFormulario {
  name: string;
  fields: BasesField[];
}

/** Las cabeceras que se reenvían al servicio: solo la IP de quien pregunta, para el límite. */
function cabeceras(request: Request): Record<string, string> {
  const h: Record<string, string> = {};
  const xff = request.headers.get('x-forwarded-for');
  if (xff) h['x-forwarded-for'] = xff;
  return h;
}

export async function leerFormulario(
  token: string,
  request: Request,
): Promise<DefinicionFormulario | null> {
  if (!RE_TOKEN.test(token)) return null;
  try {
    const res = await fetch(`${DESTINO}/_bases/v1/forms/${token}`, { headers: cabeceras(request) });
    return res.ok ? ((await res.json()) as DefinicionFormulario) : null;
  } catch {
    return null;
  }
}

/** Reenvía una petición pública al servicio y devuelve su respuesta sin cuerpo ajeno. */
export async function reenviar(
  token: string,
  ruta: '' | '/altcha',
  request: Request,
): Promise<Response> {
  if (!RE_TOKEN.test(token)) return new Response(null, { status: 404 });
  const post = request.method === 'POST';
  if (post) {
    // Se exige la longitud declarada (el navegador la manda siempre) y se rechaza ANTES
    // de leer: el servidor HTTP no entrega más bytes de los que anuncia la cabecera.
    const largo = Number(request.headers.get('content-length') ?? Number.NaN);
    if (!Number.isFinite(largo)) return new Response(null, { status: 411 });
    if (largo > MAX_CUERPO_ENVIO) return new Response(null, { status: 413 });
  }
  try {
    const res = await fetch(`${DESTINO}/_bases/v1/forms/${token}${ruta}`, {
      method: post ? 'POST' : 'GET',
      headers: { ...cabeceras(request), ...(post ? { 'content-type': 'application/json' } : {}) },
      ...(post ? { body: await request.text() } : {}),
    });
    return new Response(res.status === 204 ? null : res.body, {
      status: res.status,
      headers: {
        'content-type': res.headers.get('content-type') ?? 'application/json',
        'cache-control': 'no-store',
      },
    });
  } catch {
    return new Response(JSON.stringify({ error: { code: 'unavailable' } }), {
      status: 503,
      headers: { 'content-type': 'application/json' },
    });
  }
}
