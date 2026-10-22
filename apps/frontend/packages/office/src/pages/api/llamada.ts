/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de «Llamar desde el documento» (Sytue Docs → Sytue Rooms).
 *
 *   GET  /api/llamada?id=<doc>  →  { available, active, participants: [{ sub, name? }] }
 *   POST /api/llamada?id=<doc>  →  { url }   la ventana de la llamada en Rooms
 *
 * El GET sondea la sala del documento SIN crearla y dice quién está en ella. Si Rooms no
 * está desplegado o no contesta, `available: false` y la cabecera no pinta la función: no es
 * un error. El POST da la dirección de la ventana donde Rooms crea la sala (o entra en la
 * que ya hay) con el token de quien llama.
 *
 * A Rooms solo llegan el id y el título del documento, nunca su contenido ni la lista de
 * personas con acceso: quién puede entrar lo decide Docs, con el token de cada persona, en
 * cada petición (services/rooms/internal/media/service/documento.go).
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, docsBaseUrl, roomsBaseUrl, roomsPublicUrl } from '../../lib/auth';
import { esUuid, json, requireId } from '../../lib/docsProxy';

export const prerender = false;

/** Cuánto se espera a Rooms antes de darlo por no disponible. */
const ESPERA_MS = 2500;
/** Cota de lo que se devuelve: una sala no pasa de unas decenas. */
const MAX_PARTICIPANTES = 100;

/** Rooms no está (no resuelve, no contesta, 5xx): la función desaparece, sin error. */
const NO_DISPONIBLE = { available: false, active: false, participants: [] };

async function aRooms(path: string, token: string): Promise<Response | null> {
  try {
    return await fetch(`${roomsBaseUrl()}/api/v1${path}`, {
      headers: { authorization: `Bearer ${token}`, accept: 'application/json' },
      signal: AbortSignal.timeout(ESPERA_MS),
    });
  } catch {
    return null;
  }
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const id = requireId(ctx);
  if (!id || !esUuid(id)) return json({ error: 'missing_id' }, 400);
  const token = session.tokens.accessToken;

  const sala = await aRooms(
    `/rooms/by-source?kind=document&ref_id=${encodeURIComponent(id)}`,
    token,
  );
  if (sala === null || sala.status >= 500 || sala.status === 401) return json(NO_DISPONIBLE);
  // 404 = Rooms está pero no hay llamada (o Docs no deja ver el documento: no se distingue).
  if (sala.status === 404) return json({ available: true, active: false, participants: [] });
  if (!sala.ok) return json(NO_DISPONIBLE);
  let roomId = '';
  let creadaPor = '';
  try {
    const r = (await sala.json()) as { id?: unknown; created_by?: unknown };
    roomId = typeof r.id === 'string' ? r.id : '';
    creadaPor = typeof r.created_by === 'string' ? r.created_by : '';
  } catch {
    return json(NO_DISPONIBLE);
  }
  if (!esUuid(roomId)) return json(NO_DISPONIBLE);

  const quien = await aRooms(`/rooms/${encodeURIComponent(roomId)}/participants?present=1`, token);
  const participants: { sub: string; name?: string }[] = [];
  if (quien?.ok) {
    try {
      const b = (await quien.json()) as { participants?: { identity?: unknown; name?: unknown }[] };
      for (const p of b.participants ?? []) {
        if (typeof p.identity !== 'string' || p.identity === '') continue;
        const name = typeof p.name === 'string' ? p.name.trim() : '';
        participants.push({ sub: p.identity, ...(name ? { name } : {}) });
        if (participants.length >= MAX_PARTICIPANTES) break;
      }
    } catch {
      // Sin la lista, la llamada sigue activa: se enseña sin nombres.
    }
  }
  return json({ available: true, active: true, created_by: creadaPor, participants });
};

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const id = requireId(ctx);
  if (!id || !esUuid(id)) return json({ error: 'missing_id' }, 400);
  const token = session.tokens.accessToken;

  // El título sale de Docs, no del cliente: es lo único del documento que viaja, y pedirlo
  // con el token de quien llama comprueba de paso que puede verlo.
  let titulo = '';
  try {
    const r = await fetch(`${docsBaseUrl()}/api/v1/documents/${encodeURIComponent(id)}`, {
      headers: { authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(ESPERA_MS),
    });
    if (r.status === 403 || r.status === 404) return json({ error: 'forbidden' }, 403);
    if (!r.ok) return json({ error: 'upstream_error' }, 502);
    const b = (await r.json()) as { document?: { title?: unknown } };
    titulo =
      typeof b.document?.title === 'string' ? b.document.title.replace(/\s+/g, ' ').trim() : '';
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }
  // Rooms acota el título a 200; un documento sin título entra con un nombre legible.
  const t = Array.from(titulo || 'Documento')
    .slice(0, 200)
    .join('');
  const q = new URLSearchParams({ documento: id, titulo: t, mic: '1', cam: '0' });
  return json({ url: `${roomsPublicUrl()}/pip?${q.toString()}` });
};
