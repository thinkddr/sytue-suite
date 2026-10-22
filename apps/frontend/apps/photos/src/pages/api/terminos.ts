/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/terminos  {"q": "playa"}  →  {"terminos": ["orilla", "mar", ...]}
 *
 * Otras formas de decir lo que alguien escribió, para que la búsqueda encuentre
 * lo que Sytue AI describió con otras palabras: quien busca «playa» quiere la
 * foto descrita como «la orilla del mar al atardecer».
 *
 * POR QUÉ ESTO Y NO UN BUSCADOR SEMÁNTICO DE VERDAD. Buscar por significado
 * pide vectorizar lo escrito con el MISMO modelo que vectorizó las fotos, o sea
 * tener el codificador de texto de SigLIP 2 corriendo en algún sitio. Sería un
 * servicio de inferencia en Python, y la casa tiene una regla: «backend siempre
 * Go» (docs/business.md). Esto se acerca con lo que ya hay —el modelo de texto
 * de Sytue AI, que es barato y rápido— y sin servicio nuevo.
 *
 * ES UNA LLAMADA DE TEXTO, no de visión: cuesta una fracción de lo que cuesta
 * describir una foto, y por eso se puede pagar en una búsqueda.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from '../../lib/bff.js';
import { terminosDeRespuesta } from '../../lib/terminos.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const ia = (): string | undefined => env.SYTUE_AI_URL;

function endpoint(base: string): string {
  const limpia = base.replace(/\/$/, '');
  return limpia.endsWith('/assist') ? limpia : `${limpia}/api/v1/assist`;
}

/**
 * Lo que se le pide.
 *
 * EN LA LENGUA DE LA SESIÓN, porque lo que devuelve se compara contra
 * descripciones escritas en esa misma lengua: pedir sinónimos en inglés para
 * buscar en descripciones en español no encontraría nada.
 *
 * Palabras sueltas y en minúscula, porque van a un `ILIKE`: una frase entera
 * como término de búsqueda no casa con nada.
 */
const INSTRUCCION = [
  'The user is searching their photo library.',
  'Answer ONLY with a JSON array of at most 6 short lowercase words,',
  'in the same language as the query, that could appear in a description of',
  'the kind of photo they are looking for. Include close synonyms and',
  'closely related objects or scenes. No explanations, no duplicates,',
  'and do not repeat the query itself.',
].join(' ');

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return json({ error: 'unauthenticated' }, 401);

  const base = ia();
  if (!base) return json({ error: 'ai_not_configured' }, 503);

  const { q } = (await ctx.request.json().catch(() => ({}))) as { q?: string };
  const consulta = (q ?? '').trim();
  if (!consulta || consulta.length > 100) return json({ error: 'bad_query' }, 400);

  const res = await fetch(endpoint(base), {
    method: 'POST',
    headers: {
      authorization: `Bearer ${sesion.tokens.accessToken}`,
      'content-type': 'application/json',
      'accept-language': ctx.locals.locale,
    },
    body: JSON.stringify({
      action: 'terminos-busqueda',
      kind: 'search',
      instruction: INSTRUCCION,
      // La consulta va como CONTEXTO y no dentro de la instrucción: así lo que
      // escribió una persona son datos, no órdenes que el modelo pueda leer
      // como suyas.
      context: consulta,
      // Sin razonar: son sinónimos, no un problema. `none` acierta igual por
      // una fracción del coste, que es lo que permite pagarlo en una búsqueda.
      esfuerzo: 'none',
    }),
    signal: AbortSignal.timeout(20_000),
  }).catch(() => null);

  if (!res) return json({ error: 'ai_timeout' }, 504);
  if (!res.ok) return json({ error: 'ai_failed', status: res.status }, 502);

  const { text } = (await res.json().catch(() => ({}))) as { text?: string };
  return json({ terminos: terminosDeRespuesta(text ?? '') });
};

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);
