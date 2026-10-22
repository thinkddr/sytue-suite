/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — navegar DENTRO de una carpeta compartida.
 *
 *   GET /api/shared?id=<raíz>&subpath=<sub>
 *     → { permission: 'view'|'edit', isDir, more, items: DriveResource[] }
 *
 * Proxya el listado gated del servicio Go (verja de acceso por share). Las rutas de
 * los items son RELATIVAS a la raíz del share (el cliente las reenvía como
 * ?subpath=). El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import type { DriveResource } from '@sytue/api-clients';
import { driveFetch } from '../../lib/auth';
import { toResource, type RawNode } from '../../lib/nodes';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const id = params.get('id')?.trim() || '';
  if (!id) return json({ error: 'missing_id' }, 400);
  const subpath = params.get('subpath') ?? '';

  const qs = subpath ? `?subpath=${encodeURIComponent(subpath)}` : '';
  const res = await driveFetch(ctx, `/api/v1/shared/${encodeURIComponent(id)}/list${qs}`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'shared_list_failed' }, res.status >= 500 ? 502 : res.status);
  const body = (await res.json()) as {
    permission?: 'view' | 'edit';
    is_dir?: boolean;
    more?: boolean;
    items?: RawNode[];
  };
  const items: DriveResource[] = (body.items ?? []).map(toResource);
  return json({
    permission: body.permission === 'edit' ? 'edit' : 'view',
    isDir: body.is_dir === true,
    more: body.more === true,
    items,
  });
};
