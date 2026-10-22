/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /s/{token}/desbloquear — la contraseña de un enlace protegido, SIN sesión.
 *
 * La comprueba el servicio (`POST /api/v1/share/unlock`, con su límite de intentos). Si
 * acierta, la prueba que devuelve se guarda en una cookie httpOnly de esta dirección y se
 * vuelve a la vista; si no, se vuelve al formulario con el motivo. El servicio contesta lo
 * mismo a una contraseña errónea que a un enlace que no existe, y aquí no se añade nada.
 *
 * El token va en el cuerpo hacia el servicio, no en su ruta: la ruta se registra.
 */
import type { APIRoute } from 'astro';
import { docsBaseUrl } from '../../../lib/auth';
import { guardarPrueba, rutaPublica } from '../../../lib/enlaceClave';

export const prerender = false;

/** Cota de lo que se reenvía: la del servicio (128 caracteres) con margen de bytes. */
const MAX_BYTES = 1024;

export const POST: APIRoute = async (ctx) => {
  const token = ctx.params.token ?? '';
  const vista = rutaPublica(token);
  const volver = (motivo: string): Response => ctx.redirect(`${vista}?clave=${motivo}`, 303);

  let password = '';
  try {
    const v = (await ctx.request.formData()).get('password');
    password = typeof v === 'string' ? v : '';
  } catch {
    return volver('mal');
  }
  if (password === '' || password.length > MAX_BYTES) return volver('mal');

  let res: Response;
  try {
    const xff = ctx.request.headers.get('x-forwarded-for');
    res = await fetch(`${docsBaseUrl().replace(/\/$/, '')}/api/v1/share/unlock`, {
      method: 'POST',
      headers: { 'content-type': 'application/json', ...(xff ? { 'x-forwarded-for': xff } : {}) },
      body: JSON.stringify({ token, password }),
    });
  } catch {
    return volver('mal');
  }
  if (res.status === 429) return volver('espera');
  if (!res.ok) return volver('mal');
  const { proof } = (await res.json().catch(() => ({}))) as { proof?: unknown };
  if (typeof proof !== 'string' || proof === '') return volver('mal');
  guardarPrueba(ctx, token, proof);
  return ctx.redirect(vista, 303);
};
