/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/describir  {"id": "<foto>"}
 *
 * Describe una foto con Sytue AI y devuelve `{ description, tags }`.
 *
 * VA EN EL BFF Y NO EN EL SERVICIO GO, aunque el servicio ya tenga los bytes.
 * Quien tiene la sesión de quien lo pide es esto, y la llamada de visión se
 * cobra a un presupuesto por persona y organización: un servicio que no sabe de
 * quién es la petición no puede gastar de nadie. Es además el patrón de la
 * casa: el asistente de Docs, de Mail y de Notepads también salen del BFF.
 *
 * Lo de traer la miniatura y llamar a `/assist` está en `lib/visionIA.ts`,
 * compartido con «preguntar sobre la foto»: aquí solo queda lo que cambia, que
 * es lo que se le pide al modelo.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/bff.js';
import { interpretarRespuesta } from '../../lib/respuestaIA.js';
import { json, miniaturaEnBase64, urlDeIA, verLaFoto } from '../../lib/visionIA.js';

export const prerender = false;

/**
 * Lo que se le pide al modelo.
 *
 * EN LA LENGUA DE LA SESIÓN, porque lo que devuelve acaba en el `alt` de la
 * foto y en su ficha: una descripción en inglés dentro de una cuenta en español
 * es tan inútil para un lector de pantalla como el nombre del fichero.
 *
 * Se pide JSON y se pide corto. Lo de «no inventes nombres de personas» no es
 * remilgo: un modelo que reconoce a alguien y se equivoca deja escrito en la
 * foto de otra persona que ahí sale fulano, y eso no se puede deshacer sin que
 * alguien lo lea antes.
 */
const INSTRUCCION = [
  'Describe what is visible in this photo for use as alt text.',
  'Answer ONLY with a JSON object: {"descripcion": string, "etiquetas": string[]}.',
  'The description must be one sentence, at most 200 characters, in the language of this request.',
  'Use at most 8 short lowercase tags naming objects, places or scenes that appear.',
  'Do not name or identify people. Do not guess who someone is.',
  'Describe only what you can see; do not speculate about the context.',
].join(' ');

export const POST: APIRoute = async (ctx) => {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return json({ error: 'unauthenticated' }, 401);

  const base = urlDeIA();
  if (!base) return json({ error: 'ai_not_configured' }, 503);

  const { id } = (await ctx.request.json().catch(() => ({}))) as { id?: string };
  if (!id) return json({ error: 'missing_id' }, 400);

  const foto = await miniaturaEnBase64(id, sesion.tokens.accessToken);
  if ('error' in foto) return foto.error;

  const visto = await verLaFoto({
    base,
    token: sesion.tokens.accessToken,
    locale: ctx.locals.locale,
    action: 'describir-foto',
    instruction: INSTRUCCION,
    context: '',
    // `medium` y no `none`: en visión, razonar menos se traga la mitad de lo
    // que hay en la foto. Lo dice el propio contrato del servicio.
    esfuerzo: 'medium',
    base64: foto.base64,
  });
  if ('error' in visto) return visto.error;

  const leido = interpretarRespuesta(visto.text);
  // Si el modelo no devolvió algo usable se dice, en vez de escribir en el
  // `alt` de la foto una frase que no describe nada.
  if (!leido) return json({ error: 'ai_unreadable' }, 502);

  return json({ description: leido.descripcion, tags: leido.etiquetas });
};

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);
