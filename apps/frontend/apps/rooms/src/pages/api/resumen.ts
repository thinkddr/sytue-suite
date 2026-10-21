/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/resumen  { channelId }  →  { texto }
 *
 * «Ponme al día»: qué se ha hablado en este canal mientras no estabas.
 *
 * QUÉ RESUELVE. Volver de dos días fuera a un canal con trescientos mensajes es
 * leerlos todos o no leer ninguno, y lo segundo es lo que hace la gente. Un
 * resumen de cinco líneas con lo que se decidió y lo que queda pendiente es la
 * diferencia entre enterarse y rendirse.
 *
 * LOS MENSAJES NO SALEN DEL SERVIDOR. El BFF los lee con el token de la sesión,
 * compone el contexto y se lo pasa al servicio de IA; el navegador solo recibe
 * el resumen. Y el servicio de IA es el de la casa, en la UE: no hay ningún
 * tercero leyendo las conversaciones de la organización.
 *
 * MODO INTERACTIVO Y `esfuerzo: none`, medido: en texto acierta igual que los
 * modos que razonan y cuesta la octava parte. Resumir no es deducir.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

const env = serverEnv(import.meta.env);

/** Cuántos mensajes se miran. Cien es una tarde de canal activo, y el contexto
 *  del modelo lo aguanta sin recortar por la mitad una frase. */
const CUANTOS = 100;

/** Tope del contexto que se manda. Un canal con volcados de registros llena
 *  cualquier ventana, así que se corta por caracteres y no por mensajes. */
const MAX_CHARS = 12_000;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** URL de asistencia. La variable es la BASE del servicio, no el endpoint. */
function urlDeAsistencia(): string | undefined {
  const base = env.SYTUE_AI_URL;
  if (!base) return undefined;
  const limpia = base.replace(/\/$/, '');
  return limpia.endsWith('/api/v1/assist') ? limpia : `${limpia}/api/v1/assist`;
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const destino = urlDeAsistencia();
  if (!destino) return json({ error: 'ai_not_configured' }, 503);

  const cuerpo = (await ctx.request.json().catch(() => null)) as { channelId?: unknown } | null;
  const canal = typeof cuerpo?.channelId === 'string' ? cuerpo.channelId.trim() : '';
  if (!canal) return json({ error: 'missing_channel' }, 400);

  let contexto: string;
  try {
    const mensajes = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listMessages(canal, { limit: CUANTOS });
    if (mensajes.length === 0) return json({ error: 'sin_mensajes' }, 422);
    // Quién dijo qué, en orden. El nombre va delante porque un resumen sin
    // autores no sirve para saber a quién preguntar.
    contexto = mensajes
      .map((m) => `${m.author.name || m.author.id}: ${m.body}`)
      .join('\n')
      .slice(-MAX_CHARS);
  } catch {
    return json({ error: 'chat_error' }, 502);
  }

  let res: Response;
  try {
    res = await fetch(destino, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
      },
      body: JSON.stringify({
        action: 'resumir',
        instruction:
          'Resume esta conversación de trabajo en cinco líneas como mucho. Di qué se ha ' +
          'decidido, qué queda pendiente y de quién se espera algo. En el idioma de la ' +
          'conversación. Sin preámbulo.',
        context: contexto,
        kind: 'chat',
        modo: 'interactivo',
        esfuerzo: 'none',
      }),
      signal: AbortSignal.timeout(70_000),
    });
  } catch {
    return json({ error: 'ai_unreachable' }, 502);
  }
  if (!res.ok)
    return json({ error: 'ai_failed', status: res.status }, res.status === 503 ? 503 : 502);
  const { text } = (await res.json()) as { text?: string };
  if (!text) return json({ error: 'ai_empty' }, 502);
  return json({ texto: text });
};
