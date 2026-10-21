/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de los scripts de usuario (services/docs/internal/http/scripts.go, ADR 0069). Una
 * sola puerta con una tabla CERRADA de rutas (lib/scriptsRuta.ts); el servicio decide
 * quién puede qué y aplica cuotas, aprobación y auditoría.
 *
 *   GET    /api/scripts[?sid=]                      lista · un script con sus versiones
 *   GET    /api/scripts?op=quota|runs[&doc=]        cuota de la hora · registro de ejecuciones
 *   POST   /api/scripts                             crear
 *   POST   /api/scripts?sid=&op=version             versión nueva (borrador)
 *   POST   /api/scripts?sid=&v=&op=propose|review|run
 *   PUT    /api/scripts?sid=&op=scope               privado u organización
 *   PUT    /api/scripts?run=                        cerrar una ejecución con su resultado
 *   DELETE /api/scripts?sid=
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json } from '../../lib/docsProxy';
import { rutaScripts } from '../../lib/scriptsRuta';

export const prerender = false;

const MAX_CUERPO = 128 * 1024;

const reenviar: APIRoute = async (ctx) => {
  const ruta = rutaScripts(ctx.request.method, new URL(ctx.request.url).searchParams);
  if (!ruta) return json({ error: 'bad_request' }, 400);
  if (!ruta.cuerpo) return proxyDocs(ctx, ruta.path, { method: ruta.method });
  const texto = await ctx.request.text();
  if (texto.length > MAX_CUERPO) return json({ error: 'too_large' }, 413);
  return proxyDocs(ctx, ruta.path, {
    method: ruta.method,
    headers: { 'content-type': 'application/json' },
    body: texto || '{}',
  });
};

export const GET = reenviar;
export const POST = reenviar;
export const PUT = reenviar;
export const DELETE = reenviar;
