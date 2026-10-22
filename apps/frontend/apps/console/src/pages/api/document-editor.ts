/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de la consola — con qué editor externo se abren los documentos de la organización.
 *
 *   GET  /api/document-editor                        → el editor conectado, o null
 *   POST /api/document-editor { check, discoveryUrl } → prueba una dirección sin guardar
 *   POST /api/document-editor { discoveryUrl, label } → conecta o reconfigura
 *   POST /api/document-editor { remove: true }        → desconecta
 *
 * Era una variable de entorno de Sytue Drive: conectar el Collabora de una empresa
 * obligaba a editar el despliegue y todas las organizaciones de una instalación
 * compartían editor. Ahora es un ajuste de la organización, y esta es su puerta.
 *
 * Las tres mutaciones van por POST con cuerpo JSON: el checkOrigin de Astro rechaza PUT y
 * DELETE tras el proxy (mismo patrón que el resto de la consola).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../server/bff.js';
import { driveBaseUrl } from '../../lib/auth.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Sesión con rol de administración: conectar un editor lo decide quien administra. */
async function guard(ctx: Parameters<APIRoute>[0]) {
  return requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
}

export const GET: APIRoute = async (ctx) => {
  const g = await guard(ctx);
  if (g instanceof Response) return json({ editor: null, host_enabled: false }, 403);
  const res = await fetch(`${driveBaseUrl().replace(/\/$/, '')}/api/v1/wopi/editor`, {
    headers: {
      authorization: `Bearer ${g.session.tokens.accessToken}`,
      accept: 'application/json',
    },
  });
  // Sin backend vivo la pantalla dice «ninguno conectado», que es lo que verá quien
  // abra la consola en un entorno sin Drive. `host_enabled` viaja tal cual: dice si
  // ESTE despliegue puede acuñar tokens WOPI, y sin él la consola cantaba «editor
  // conectado» en instalaciones donde la acción no iba a aparecer nunca.
  if (!res.ok) return json({ editor: null, host_enabled: false });
  return json(await res.json());
};

export const POST: APIRoute = async (ctx) => {
  const g = await guard(ctx);
  if (g instanceof Response) return json({ error: 'forbidden' }, 403);

  let p: { check?: unknown; remove?: unknown; discoveryUrl?: unknown; label?: unknown };
  try {
    p = (await ctx.request.json()) as typeof p;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const base = driveBaseUrl().replace(/\/$/, '');
  const auth = {
    authorization: `Bearer ${g.session.tokens.accessToken}`,
    'content-type': 'application/json',
    accept: 'application/json',
  };

  if (p.remove === true) {
    const res = await fetch(`${base}/api/v1/wopi/editor`, { method: 'DELETE', headers: auth });
    return res.ok ? json({ ok: true }) : json({ error: 'delete_failed' }, res.status);
  }

  const discoveryUrl = typeof p.discoveryUrl === 'string' ? p.discoveryUrl.trim() : '';
  if (!discoveryUrl) return json({ error: 'missing_url' }, 400);

  // Comprobar no guarda nada: se puede probar una dirección antes de decidir.
  if (p.check === true) {
    const res = await fetch(`${base}/api/v1/wopi/editor/check`, {
      method: 'POST',
      headers: auth,
      body: JSON.stringify({ discovery_url: discoveryUrl }),
    });
    if (!res.ok) return json({ ok: false, reason: `HTTP ${res.status}` });
    return json(await res.json());
  }

  const res = await fetch(`${base}/api/v1/wopi/editor`, {
    method: 'PUT',
    headers: auth,
    body: JSON.stringify({
      discovery_url: discoveryUrl,
      label: typeof p.label === 'string' ? p.label : '',
    }),
  });
  if (!res.ok) return json({ error: 'save_failed' }, res.status);
  return json(await res.json());
};
