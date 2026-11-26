/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/agrupar  {"fotos": [{id, takenAt, takenAtFromExif, location, description, tags}]}
 *   →  {"albumes": [{"titulo": "Verano en Cádiz", "ids": ["<foto>", …]}]}
 *
 * «Organizar en álbumes con IA»: con lo que Sytue AI ya vio en cada foto, UNA
 * llamada de texto que propone álbumes. NO crea nada: la galería enseña la
 * propuesta y quien mira decide qué álbumes se crean.
 *
 * VA EN EL BFF por lo mismo que describir: la llamada se cobra al presupuesto de
 * quien la pide, y quien tiene su sesión es esto. Las fotos llegan de la galería
 * de esa misma persona, que es quien ya las tiene en pantalla: pedirlas otra vez
 * al servicio, una a una, sería repetir doscientas peticiones para leer lo mismo.
 * Lo que traen va como CONTEXTO —dato, nunca orden— y la instrucción es fija.
 *
 * La instrucción, el contexto y el lector viven en `lib/agruparIA.ts`, los mismos
 * que los de las apps nativas.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/bff.js';
import {
  contextoAgrupar,
  instruccionAgrupar,
  leerAgrupacion,
  MIN_FOTOS_POR_ALBUM,
  TOPE_ORGANIZAR,
  type FotoParaAgrupar,
} from '../../lib/agruparIA.js';
import { json, pedirAsistente, urlDeIA } from '../../lib/visionIA.js';

export const prerender = false;

/**
 * Las fotos del cuerpo, con solo lo que se le cuenta al modelo y de los tipos que
 * se esperan: lo que no lo es se queda fuera en vez de llegar al contexto. `null`
 * si el cuerpo no es una lista de fotos con su id.
 */
function fotosDelCuerpo(cuerpo: unknown): FotoParaAgrupar[] | null {
  const fotos = (cuerpo as { fotos?: unknown } | null)?.fotos;
  if (!Array.isArray(fotos)) return null;
  const out: FotoParaAgrupar[] = [];
  for (const x of fotos) {
    const f = (x ?? {}) as Record<string, unknown>;
    const id = f['id'];
    if (typeof id !== 'string' || id === '' || id.length > 200) return null;
    const lugar = f['location'] as { lat?: unknown; lon?: unknown } | undefined;
    out.push({
      id,
      takenAt: typeof f['takenAt'] === 'string' ? f['takenAt'] : undefined,
      takenAtFromExif: f['takenAtFromExif'] === true,
      location:
        typeof lugar?.lat === 'number' && typeof lugar.lon === 'number'
          ? { lat: lugar.lat, lon: lugar.lon }
          : undefined,
      description: typeof f['description'] === 'string' ? f['description'] : undefined,
      tags: Array.isArray(f['tags'])
        ? f['tags'].filter((e): e is string => typeof e === 'string')
        : undefined,
    });
  }
  return out;
}

export const POST: APIRoute = async (ctx) => {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return json({ error: 'unauthenticated' }, 401);

  const base = urlDeIA();
  if (!base) return json({ error: 'ai_not_configured' }, 503);

  const fotos = fotosDelCuerpo(await ctx.request.json().catch(() => null));
  if (!fotos) return json({ error: 'bad_photos' }, 400);
  // El tope es de la pasada, y lo aplica también quien llama: más que esto no es
  // una pasada, es la biblioteca entera en una petición.
  if (fotos.length > TOPE_ORGANIZAR) return json({ error: 'too_many_photos' }, 400);

  const { contexto, ids } = contextoAgrupar(fotos);
  // Con menos de dos fotos con algo escrito no hay nada que agrupar, y no se gasta
  // una llamada para que lo diga el modelo.
  if (ids.length < MIN_FOTOS_POR_ALBUM) return json({ albumes: [] });

  const visto = await pedirAsistente({
    base,
    token: sesion.tokens.accessToken,
    locale: ctx.locals.locale,
    action: 'agrupar-fotos',
    kind: 'photos',
    instruction: instruccionAgrupar(ctx.locals.locale),
    context: contexto,
    // Sin razonar, como ampliar una búsqueda: en texto, `none` acierta igual por
    // una fracción del coste, y el razonamiento contaría contra el tope de salida.
    esfuerzo: 'none',
  });
  if ('error' in visto) return visto.error;

  // El modelo lee números cortos; de vuelta, cada uno es el id de su foto.
  const numeros = ids.map((_, i) => String(i + 1));
  const leido = leerAgrupacion(visto.text, numeros);
  if (!leido) return json({ error: 'ai_unreadable' }, 502);
  return json({
    albumes: leido.map((a) => ({ titulo: a.titulo, ids: a.ids.map((n) => ids[Number(n) - 1]) })),
  });
};

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);
