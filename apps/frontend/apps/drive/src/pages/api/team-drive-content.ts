/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — CONTENIDO de una unidad de equipo. El servicio Go aísla el
 * contenido por unidad (RLS) y gatea por rol (viewer lee; editor+ escribe).
 *
 *   GET    /api/team-drive-content?id=&folder=&cursor=   → navegar (subcarpetas+ficheros+rol)
 *   GET    /api/team-drive-content?id=&file=<id>         → descargar un fichero (blob)
 *   POST   /api/team-drive-content?id=  { parent_id?, name }  → crear subcarpeta (editor+)
 *   PUT    /api/team-drive-content?id=&folder=&name=&type=    → subir fichero (editor+)
 *   DELETE /api/team-drive-content?id=&file=<id> | &folder=<id> → enviar a papelera (editor+)
 *
 * El cliente usa POST/PUT/DELETE con octet-stream o JSON para no disparar el
 * checkOrigin CSRF de Astro. El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Nodo de una unidad tal y como lo consume la isla (carpeta o fichero por id). */
interface TeamNode {
  id: string;
  name: string;
  isDirectory: boolean;
  size?: number;
  contentType?: string;
  modifiedAt?: string;
}

interface RawFolder {
  id: string;
  name: string;
  updated_at?: string;
}
interface RawFile {
  id: string;
  name: string;
  size_bytes?: number;
  content_type?: string;
  updated_at?: string;
}

function folderNode(f: RawFolder): TeamNode {
  return {
    id: f.id,
    name: f.name,
    isDirectory: true,
    ...(f.updated_at ? { modifiedAt: f.updated_at } : {}),
  };
}
function fileNode(f: RawFile): TeamNode {
  return {
    id: f.id,
    name: f.name,
    isDirectory: false,
    size: f.size_bytes ?? 0,
    ...(f.content_type ? { contentType: f.content_type } : {}),
    ...(f.updated_at ? { modifiedAt: f.updated_at } : {}),
  };
}

function query(ctx: Parameters<APIRoute>[0]): URLSearchParams {
  return new URL(ctx.request.url).searchParams;
}

export const GET: APIRoute = async (ctx) => {
  const params = query(ctx);
  const id = params.get('id')?.trim() || '';
  if (!id) return json({ error: 'missing_id' }, 400);
  const fileId = params.get('file')?.trim() || '';

  // Descarga de un fichero: transmite el blob del servicio (attachment, nosniff).
  if (fileId) {
    const res = await driveFetch(
      ctx,
      `/api/v1/team-drives/${encodeURIComponent(id)}/files/${encodeURIComponent(fileId)}/content`,
    );
    if (!res) return json({ error: 'unauthenticated' }, 401);
    if (!res.ok) return json({ error: 'download_failed' }, res.status >= 500 ? 502 : res.status);
    const headers = new Headers({
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
    });
    for (const h of ['content-type', 'content-length', 'content-disposition', 'etag']) {
      const v = res.headers.get(h);
      if (v) headers.set(h, v);
    }
    // VISTA PREVIA, con la misma política que /api/content y /api/shared-content.
    // Sin esto una unidad de equipo solo sabía DESCARGAR, y por eso pulsar un
    // fichero hacía ahí una cosa distinta que en el resto de Drive.
    if (params.get('disposition') === 'inline') {
      let type = headers.get('content-type') || 'application/octet-stream';
      // Todo texto baja como text/plain: servir text/html en el origen de la app
      // con cookies sería XSS almacenado.
      if (type.startsWith('text/') || type === 'application/json') {
        type = 'text/plain; charset=utf-8';
      }
      const seguro =
        type.startsWith('image/') || type === 'application/pdf' || type.startsWith('text/plain');
      headers.set('content-type', type);
      // Se cambia SOLO el tipo de disposición y se conserva el `filename` que ya
      // compuso el servicio: recomponerlo aquí obligaría a adivinar el nombre a
      // partir del identificador, que no lo lleva.
      const disp = headers.get('content-disposition') ?? 'attachment';
      headers.set(
        'content-disposition',
        seguro ? disp.replace(/^attachment/i, 'inline') : disp.replace(/^inline/i, 'attachment'),
      );
      if (seguro) headers.set('content-security-policy', 'sandbox');
    }
    return new Response(res.body, { headers });
  }

  // Navegar una carpeta de la unidad (raíz si ?folder= ausente).
  const qs = new URLSearchParams();
  const folder = params.get('folder')?.trim();
  if (folder) qs.set('folder', folder);
  const cursor = params.get('cursor')?.trim();
  if (cursor) qs.set('cursor', cursor);
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  const res = await driveFetch(
    ctx,
    `/api/v1/team-drives/${encodeURIComponent(id)}/children${suffix}`,
  );
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'list_failed' }, res.status >= 500 ? 502 : res.status);
  const body = (await res.json()) as {
    role?: string;
    can_write?: boolean;
    folders?: RawFolder[];
    files?: RawFile[];
    next_cursor?: string;
  };
  const items: TeamNode[] = [
    ...(body.folders ?? []).map(folderNode),
    ...(body.files ?? []).map(fileNode),
  ];
  return json({
    role: body.role ?? 'viewer',
    canWrite: body.can_write === true,
    items,
    ...(body.next_cursor ? { nextCursor: body.next_cursor } : {}),
  });
};

export const POST: APIRoute = async (ctx) => {
  const id = query(ctx).get('id')?.trim() || '';
  if (!id) return json({ error: 'missing_id' }, 400);
  let body: { parent_id?: unknown; name?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const name = typeof body.name === 'string' ? body.name.trim() : '';
  if (!name) return json({ error: 'missing_name' }, 400);
  const parent = typeof body.parent_id === 'string' ? body.parent_id.trim() : '';
  const res = await driveFetch(ctx, `/api/v1/team-drives/${encodeURIComponent(id)}/folders`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(parent ? { parent_id: parent, name } : { name }),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'folder_failed' }, res.status >= 500 ? 502 : res.status);
  return json({ resource: folderNode((await res.json()) as RawFolder) }, 201);
};

export const PUT: APIRoute = async (ctx) => {
  const params = query(ctx);
  const id = params.get('id')?.trim() || '';
  if (!id) return json({ error: 'missing_id' }, 400);
  const name = params.get('name')?.trim() || '';
  if (!name) return json({ error: 'missing_name' }, 400);
  const folder = params.get('folder')?.trim() || '';
  const type = params.get('type') || 'application/octet-stream';
  // El nombre viaja en ?name= (el servicio lo decodifica con Query().Get, como
  // shared-upload); X-File-Name NO se decodifica, así que evitaría acentos/espacios.
  const goQs = new URLSearchParams();
  if (folder) goQs.set('folder', folder);
  goQs.set('name', name);
  const data = await ctx.request.arrayBuffer();
  const res = await driveFetch(
    ctx,
    `/api/v1/team-drives/${encodeURIComponent(id)}/files?${goQs.toString()}`,
    { method: 'POST', headers: { 'content-type': type }, body: data },
  );
  if (!res) return json({ error: 'unauthenticated' }, 401);
  // Preserva 4xx del servicio y el 507 de cuota (la UI de subida lo distingue).
  if (!res.ok) {
    const status = (res.status >= 400 && res.status < 500) || res.status === 507 ? res.status : 502;
    return json({ error: 'upload_failed' }, status);
  }
  return json({ resource: fileNode((await res.json()) as RawFile) }, 201);
};

export const DELETE: APIRoute = async (ctx) => {
  const params = query(ctx);
  const id = params.get('id')?.trim() || '';
  const fileId = params.get('file')?.trim() || '';
  const folderId = params.get('folder')?.trim() || '';
  if (!id || (!fileId && !folderId)) return json({ error: 'missing_target' }, 400);
  const path = fileId
    ? `/api/v1/team-drives/${encodeURIComponent(id)}/files/${encodeURIComponent(fileId)}`
    : `/api/v1/team-drives/${encodeURIComponent(id)}/folders/${encodeURIComponent(folderId)}`;
  const res = await driveFetch(ctx, path, {
    method: 'DELETE',
    headers: { 'content-type': 'application/octet-stream' },
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'delete_failed' }, res.status >= 500 ? 502 : res.status);
  return json({ ok: true });
};
