/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — la biblioteca de plantillas de la organización.
 *
 *   GET    /api/org-templates?id=  →  GET    /org-templates/{id}            la ficha y sus versiones
 *   PUT    /api/org-templates?id=  →  PUT    /org-templates/{id}            marcar (nombre, descripción)
 *   DELETE /api/org-templates?id=  →  DELETE /org-templates/{id}            quitar la marca
 *   POST   /api/org-templates?id=  →  POST   /org-templates/{id}/versions   publicar una versión
 *
 * Quién puede qué lo decide el servicio (administrar la organización para marcar y
 * quitar; editar el documento para publicar): aquí solo se reenvía.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';

export const prerender = false;

const ruta = (id: string): string => `/org-templates/${encodeURIComponent(id)}`;

export const GET: APIRoute = (ctx) => {
  const id = requireId(ctx);
  return id ? proxyDocs(ctx, ruta(id)) : json({ error: 'missing_id' }, 400);
};

export const PUT: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, ruta(id), {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: (await ctx.request.text()) || '{}',
  });
};

export const DELETE: APIRoute = (ctx) => {
  const id = requireId(ctx);
  return id ? proxyDocs(ctx, ruta(id), { method: 'DELETE' }) : json({ error: 'missing_id' }, 400);
};

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `${ruta(id)}/versions`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: (await ctx.request.text()) || '{}',
  });
};
