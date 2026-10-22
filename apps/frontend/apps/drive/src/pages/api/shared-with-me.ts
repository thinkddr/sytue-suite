/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — «Compartido conmigo».
 *
 *   GET /api/shared-with-me   → { items: (DriveResource & { permission, sharedBy })[] }
 *
 * Lista los recursos (fichero/carpeta) que otros han compartido con el usuario vía
 * shares kind=user|group vigentes; el servicio Go resuelve la ruta absoluta y aplica
 * la verja de acceso. El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';
import { toResource, type RawNode } from '../../lib/nodes';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** nodeView + permiso + dueño, tal y como lo emite el servicio Go. */
type RawShared = RawNode & {
  permission?: 'view' | 'edit';
  shared_by?: string;
  edit_requested?: boolean;
};

export const GET: APIRoute = async (ctx) => {
  const res = await driveFetch(ctx, '/api/v1/shared-with-me');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'shared_failed' }, res.status >= 500 ? 502 : res.status);
  const body = (await res.json()) as { items?: RawShared[] };
  const items = (body.items ?? []).map((n) => ({
    ...toResource(n),
    permission: n.permission === 'edit' ? 'edit' : 'view',
    sharedBy: n.shared_by ?? '',
    editRequested: n.edit_requested === true,
  }));
  return json({ items });
};
