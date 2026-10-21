/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — COMPARTIR un documento: permisos por persona y enlaces públicos.
 *
 * El modelo estaba entero en el servicio Go —principales (persona, grupo, organización,
 * enlace), cuatro roles, enlaces con caducidad y su resolución sin sesión— y no había
 * forma de usarlo: ni endpoint aquí ni botón en la interfaz. Un documento colaborativo
 * que no se puede compartir es medio producto.
 *
 *   GET    /api/sharing?id=                    →  GET    /documents/{id}/permissions
 *                                              +  GET    /documents/{id}/shares
 *   PUT    /api/sharing?id=                    →  PUT    /documents/{id}/permissions
 *   POST   /api/sharing?id=                    →  POST   /documents/{id}/shares
 *   DELETE /api/sharing?id=&principal=…        →  DELETE /documents/{id}/permissions
 *   DELETE /api/sharing?id=&linkId=…           →  DELETE /documents/{id}/shares/{linkId}
 *   PATCH  /api/sharing?id=&linkId=…           →  PUT    /documents/{id}/shares/{linkId}/password
 *
 * Las dos lecturas van juntas en un GET porque el diálogo las enseña a la vez: pedirlas
 * por separado solo añadiría un estado intermedio en el que la mitad está y la otra no.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json, esUuid } from '../../lib/docsProxy';
import { resolverPersona, resolverPersonas } from '../../lib/directorio';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const doc = encodeURIComponent(id);
  const [permRes, shareRes] = await Promise.all([
    proxyDocs(ctx, `/documents/${doc}/permissions`),
    proxyDocs(ctx, `/documents/${doc}/shares`),
  ]);
  // Solo quien es propietario ve esto; si el servicio dice que no, se propaga tal cual
  // en vez de enseñar un diálogo vacío que parecería «no hay nadie con acceso».
  if (!permRes.ok) return permRes;
  const { permissions: crudos = [] } = (await permRes.json()) as {
    permissions?: { principal_kind?: string; principal_id?: string }[];
  };
  // Con quién está compartido, por su correo y su nombre: el permiso se guarda contra
  // el `sub`, que no le dice nada a nadie.
  const personas = await resolverPersonas(
    ctx,
    crudos.filter((p) => p.principal_kind === 'user').map((p) => p.principal_id ?? ''),
  );
  const permissions = crudos.map((p) => {
    const persona = p.principal_kind === 'user' ? personas.get(p.principal_id ?? '') : undefined;
    return persona
      ? {
          ...p,
          // Quien entró desde otra organización (ADR 0052) no trae correo: su nombre.
          ...(persona.email ? { principal_email: persona.email } : {}),
          ...(persona.name ? { principal_name: persona.name } : {}),
        }
      : p;
  });
  const { shares = [] } = shareRes.ok
    ? ((await shareRes.json()) as { shares?: unknown[] })
    : { shares: [] };
  return json({ permissions, shares });
};

export const PUT: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  let body: Record<string, unknown>;
  try {
    body = (await ctx.request.json()) as Record<string, unknown>;
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  // Compartir con una PERSONA se escribe con su dirección, y por dentro se guarda su
  // `sub`. La traducción la hace Sytue ID y no el navegador: un `sub` elegido por el
  // cliente sería conceder a quien se quisiera. Hasta el 2026-09-25 la dirección iba
  // tal cual como `principal_id`, el acceso se resuelve contra el `sub` y la concesión
  // no abría nada — ni avisaba a nadie.
  if (body.principal_kind === 'user') {
    const escrito = typeof body.principal_id === 'string' ? body.principal_id.trim() : '';
    const persona = escrito.includes('@') ? await resolverPersona(ctx, escrito) : null;
    if (!persona) return json({ error: 'unknown_person' }, 404);
    body.principal_id = persona.userId;
  }
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/permissions`, {
    method: 'PUT',
    body: JSON.stringify(body),
  });
};

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/shares`, {
    method: 'POST',
    body: await ctx.request.text(),
  });
};

/**
 * Poner, cambiar o quitar (`password: ''`) la contraseña de un enlace. Del cuerpo del
 * navegador solo viaja la contraseña; lo que vuelve es un 204: ni ella ni su hash regresan.
 */
export const PATCH: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const linkId = new URL(ctx.request.url).searchParams.get('linkId');
  if (!linkId || !esUuid(linkId)) return json({ error: 'bad_request' }, 400);
  const { password } = (await ctx.request.json().catch(() => ({}))) as { password?: unknown };
  if (typeof password !== 'string') return json({ error: 'bad_request' }, 400);
  return proxyDocs(
    ctx,
    `/documents/${encodeURIComponent(id)}/shares/${encodeURIComponent(linkId)}/password`,
    {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ password }),
    },
  );
};

export const DELETE: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const doc = encodeURIComponent(id);
  const params = new URL(ctx.request.url).searchParams;

  // Un enlace se revoca por su identificador; un permiso, por el principal al que se le
  // quita. Son dos rutas distintas del servicio y se distinguen por el parámetro.
  const linkId = params.get('linkId');
  if (linkId) {
    if (!esUuid(linkId)) return json({ error: 'bad_request' }, 400);
    return proxyDocs(ctx, `/documents/${doc}/shares/${encodeURIComponent(linkId)}`, {
      method: 'DELETE',
    });
  }
  const kind = params.get('principalKind');
  const principal = params.get('principalId');
  if (!kind) return json({ error: 'missing_principal' }, 400);
  const qs = new URLSearchParams({ principal_kind: kind, principal_id: principal ?? '' });
  return proxyDocs(ctx, `/documents/${doc}/permissions?${qs.toString()}`, { method: 'DELETE' });
};
