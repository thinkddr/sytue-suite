/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — las personas de la organización, para el selector de menciones
 * de los comentarios. Solo correo y nombre: es lo que se inserta (`@ana@empresa.es`) y
 * lo que se enseña. El `sub` no le hace falta a la página.
 *
 *   GET /api/people  →  Sytue ID GET /api/v1/directory (con la sesión de quien pide)
 *   GET /api/people?resolve=<correo>  →  la persona de la organización con ese correo, con su
 *       `sub` (identificador): lo necesitan los rangos editables de una hoja protegida, que se
 *       conceden por `sub`. Lo traduce Sytue ID, acotado a la organización de quien pregunta;
 *       404 si no existe (o es de otra organización: no se distinguen).
 */
import type { APIRoute } from 'astro';
import { agenda, resolverPersona } from '../../lib/directorio';
import { json } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const correo = new URL(ctx.request.url).searchParams.get('resolve')?.trim();
  if (correo) {
    const persona = correo.includes('@') ? await resolverPersona(ctx, correo) : null;
    if (!persona) return json({ error: 'unknown_person' }, 404);
    return json({
      sub: persona.userId,
      email: persona.email,
      ...(persona.name ? { name: persona.name } : {}),
    });
  }
  const personas = await agenda(ctx);
  return json({
    people: personas.map(({ email, name }) => ({ email, ...(name ? { name } : {}) })),
  });
};
