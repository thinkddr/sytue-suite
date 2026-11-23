/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de CertiPDF — el SELLO DE TIEMPO de una firma hecha en el dispositivo
 * (ADR 0086, «Firmar en el dispositivo»).
 *
 *   GET  /api/pdf-sello                       →  GET  /pdf/timestamp   {disponible, autoridad}
 *   POST /api/pdf-sello  {resumen: base64}    →  POST /pdf/timestamp   {token: base64}
 *
 * La autoridad de sellado (su URL, credenciales y anclas) solo la conoce el servicio
 * de Docs, así que el navegador le pide el sello a través de aquí. Lo ÚNICO que
 * viaja es el SHA-256 del valor de la firma (el messageImprint que se sella): ni el
 * PDF, ni su resumen, ni el certificado. Por eso el cuerpo no se reenvía tal cual:
 * se valida que sean 32 bytes y se rehace con ese único campo.
 */
import type { APIRoute } from 'astro';
import { json, proxyDocs } from '../../lib/docsProxy';

export const prerender = false;

/** 32 bytes en base64 estándar: 43 caracteres y un `=`. */
const RESUMEN = /^[A-Za-z0-9+/]{43}=$/;

export const GET: APIRoute = (ctx) => proxyDocs(ctx, '/pdf/timestamp');

export const POST: APIRoute = async (ctx) => {
  let cuerpo: unknown;
  try {
    cuerpo = await ctx.request.json();
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const resumen = (cuerpo as { resumen?: unknown } | null)?.resumen;
  if (typeof resumen !== 'string' || !RESUMEN.test(resumen))
    return json({ error: 'invalid_body' }, 400);
  return proxyDocs(ctx, '/pdf/timestamp', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ resumen }),
  });
};
