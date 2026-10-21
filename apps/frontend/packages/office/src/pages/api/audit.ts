/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — auditoría de operaciones (solo el propietario).
 *
 *   GET /api/audit?id=&before_seq=&limit=  →  GET /documents/{id}/audit?before_seq=&limit=
 *
 * Quién cambió qué y cuándo, agrupado por autor, tipo, clave y minuto, y para los parches de
 * hoja y presentación DÓNDE (hoja/celda, diapositiva/caja). Nunca el valor escrito.
 *
 * A la respuesta se le añade `names` ({sub: nombre}), resuelto en el directorio de la
 * organización de quien pregunta: el servicio guarda el `sub`, que no se puede leer.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';
import { resolverPersonas } from '../../lib/directorio';

export const prerender = false;

/** Cuántas personas distintas se resuelven por página (el directorio va una a una). */
const MAX_PERSONAS = 50;

export const GET: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const params = new URL(ctx.request.url).searchParams;
  // Frontera de confianza: before_seq y limit, numéricos y acotados (§4.1).
  const before = Number(params.get('before_seq') ?? '0');
  const limit = Number(params.get('limit') ?? '500');
  if (!Number.isFinite(before) || !Number.isFinite(limit))
    return json({ error: 'bad_request' }, 400);
  const res = await proxyDocs(
    ctx,
    `/documents/${encodeURIComponent(id)}/audit?before_seq=${Math.max(0, Math.floor(before))}&limit=${Math.min(5000, Math.max(1, Math.floor(limit)))}`,
  );
  if (!res.ok) return res;
  try {
    const body = (await res.json()) as { entries?: { author_sub?: unknown }[] };
    const subs = [
      ...new Set(
        (body.entries ?? []).map((e) => e.author_sub).filter((s) => typeof s === 'string'),
      ),
    ].slice(0, MAX_PERSONAS);
    const personas = await resolverPersonas(ctx, subs);
    const names: Record<string, string> = {};
    for (const [sub, p] of personas) {
      const nombre = p.name || p.email;
      if (nombre) names[sub] = nombre;
    }
    return json({ ...body, names });
  } catch {
    // Cuerpo ilegible del servicio: no se inventa una lista vacía como si fuera la verdad.
    return json({ error: 'upstream_invalid' }, 502);
  }
};
