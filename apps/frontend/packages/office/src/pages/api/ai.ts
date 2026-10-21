/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/ai  (application/json)
 *
 * Proxy del asistente de IA del editor. Recibe { action, instruction, context,
 * kind } (lo compone el panel con `buildAiRequest`, puro y testeado), añade la
 * sesión httpOnly como Authorization: Bearer y REENVÍA a un servicio de IA
 * CONFIGURABLE por env (SYTUE_AI_URL). El BFF NO lleva lógica de IA: solo proxya
 * (memoria «backend siempre Go»). El servicio real —un microservicio Go que
 * llama a Claude/Anthropic u otro modelo soberano— es FOLLOW-UP; aquí solo se
 * define el contrato y el reenvío. Devuelve la respuesta completa del servicio
 * ({ text }) o un error con mensaje claro (incl. «no configurado»).
 *
 * ponytail: respuesta completa (sin streaming) en el MVP; si el servicio expone
 * SSE/stream, reenviar el ReadableStream aquí y consumirlo incremental en el panel.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth';
import { serverEnv } from '@sytue/auth/server';

export const prerender = false;

const env = serverEnv(import.meta.env);

/** URL del servicio de IA (server-only; nunca llega al cliente). Sin default:
 *  ausente = función deshabilitada, con mensaje claro. */
const aiUrl = (): string | undefined => env.SYTUE_AI_URL;

/** Holgura sobre el tope de contexto del cliente (12k chars): rechaza antes de leer. */
const MAX_BYTES = 256 * 1024;

/** Tope con imagen. Una página A4 a 150 ppp en JPEG ronda los 300 KiB, y el
 *  base64 abulta un tercio más; 1 MiB deja sitio sin invitar a mandar el
 *  original a 600 ppp, que no lee mejor y cuesta el triple. */
const MAX_BYTES_VISION = 1024 * 1024;

/** Tope de espera del servicio de IA (inferencia puede tardar).
 *
 *  DOS TOPES PORQUE HAY DOS MODELOS, y con uno solo el modo calidad no
 *  funcionaría: el servicio le da 120 s (razona antes de responder, 14 s de
 *  mediana y 34 s en el peor caso medido), así que un tope único de 60 s aquí
 *  abortaría la petición a mitad y devolvería `ai_timeout` mientras el modelo
 *  sigue trabajando —y con el coste ya gastado—. Cada tope va por encima del
 *  plazo que aplica el servicio, para que el error legible lo dé él y no este
 *  proxy con un AbortError sin causa. */
const TIMEOUT_MS = 70_000;
const TIMEOUT_CALIDAD_MS = 130_000;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

/**
 * URL del endpoint de asistencia a partir de `SYTUE_AI_URL`.
 *
 * LA VARIABLE ES LA BASE DEL SERVICIO, no el endpoint. Lo era al revés cuando
 * solo existía `/assist`, y por eso el chart —que emite la base, como
 * `SYTUE_PHOTOS_API_URL`— dejaba al BFF haciendo POST a la RAÍZ del servicio:
 * 404 arriba, y el BFF lo convertía en un 502 que no decía nada. Medido el
 * 2026-09-10 en el log del pod: `method=POST path=/ status=404`.
 *
 * Base y no endpoint porque el servicio ya tiene DOS (`/assist` y
 * `/transcribe`), y una variable con la ruta dentro no puede servir a los dos.
 *
 * Se tolera que ya venga con la ruta puesta: hay instalaciones configuradas con
 * la URL completa y romperlas al actualizar sería cambiar un fallo por otro.
 */
function endpointAsistencia(base: string): string {
  const limpia = base.replace(/\/+$/, '');
  return limpia.endsWith('/api/v1/assist') ? limpia : `${limpia}/api/v1/assist`;
}

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = aiUrl();
  if (!url) return json({ error: 'ai_not_configured' }, 503);

  const len = Number(ctx.request.headers.get('content-length') ?? '0');
  // El tope depende de si viene imagen, y eso no se sabe hasta leer el cuerpo.
  // Se usa el mayor para decidir si merece la pena leerlo, y el ajustado
  // después: así una petición de texto de 900 KiB sigue rechazándose.
  if (Number.isFinite(len) && len > MAX_BYTES_VISION) return json({ error: 'bad_request' }, 413);

  let payload: unknown;
  try {
    payload = await ctx.request.json();
  } catch {
    return json({ error: 'bad_request' }, 400);
  }

  // Validación en la frontera de confianza: el cuerpo llega del navegador. Solo se
  // reenvían los campos esperados (nada de pasar el objeto crudo del cliente).
  const body = pickRequest(payload);
  if (body === null) return json({ error: 'bad_request' }, 400);
  if (body.imagen === undefined && Number.isFinite(len) && len > MAX_BYTES) {
    return json({ error: 'bad_request' }, 413);
  }

  let upstream: Response;
  try {
    upstream = await fetch(endpointAsistencia(url), {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: `Bearer ${session.tokens.accessToken}`,
      },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(body.modo === 'calidad' ? TIMEOUT_CALIDAD_MS : TIMEOUT_MS),
    });
  } catch (e) {
    // AbortSignal.timeout aborta con TimeoutError; el resto es red/servicio caído.
    const timedOut = e instanceof Error && e.name === 'TimeoutError';
    return json({ error: timedOut ? 'ai_timeout' : 'upstream_unreachable' }, timedOut ? 504 : 502);
  }

  // El servicio responde 200 { text }: se reenvía verbatim. Cualquier error del
  // servicio se normaliza a `ai_failed` (el panel ya tiene el mensaje legible).
  if (!upstream.ok) return json({ error: 'ai_failed' }, 502);
  const text = await upstream.text();
  return new Response(text, {
    status: 200,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};

interface AiProxyBody {
  action: string;
  instruction: string;
  context: string;
  kind: string;
  /** Modelo a usar: 'interactivo' (rápido, por defecto) o 'calidad' (acierta
   *  más, tarda del orden de quince segundos). Se omite si no viene. */
  modo?: 'interactivo' | 'calidad';
  /** Página rasterizada en base64, para leer un escaneo. Sin ella la petición
   *  es solo de texto, que es el caso normal. */
  imagen?: string;
  /** Sale como `imagen_tipo`: el cable de la casa es snake y el TypeScript camel,
   *  con el BFF traduciendo. Lo que ENTRA del navegador sigue siendo camel. */
  imagen_tipo?: string;
}

/** Extrae solo los campos esperados; null si faltan o no son del tipo correcto. */
function pickRequest(raw: unknown): AiProxyBody | null {
  if (raw === null || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  if (
    typeof r.action !== 'string' ||
    typeof r.instruction !== 'string' ||
    typeof r.kind !== 'string' ||
    (r.context !== undefined && typeof r.context !== 'string')
  ) {
    return null;
  }
  // instruction vacía = nada que pedir (el panel ya lo evita, pero defendemos aquí).
  if (r.instruction.trim() === '') return null;
  // El modo se valida como lista cerrada, no se reenvía tal cual: este cuerpo
  // llega del navegador y el servicio rechaza lo que no conozca con un 400 que
  // el panel mostraría como fallo del asistente. Un valor raro aquí se traduce
  // en «sin modo», que es el rápido.
  const modo = r.modo === 'calidad' || r.modo === 'interactivo' ? r.modo : undefined;
  // La imagen se acepta como base64 y se valida la FORMA, no el contenido: aquí
  // no se decodifica —sería pagar el coste dos veces, porque el servicio lo hace
  // igual— pero sí se rechaza lo que no puede ser base64, que es lo que evita
  // reenviar basura y cobrar una llamada de visión por ella.
  const imagen =
    typeof r.imagen === 'string' && r.imagen !== '' && /^[A-Za-z0-9+/=]+$/.test(r.imagen)
      ? r.imagen
      : undefined;
  // Lista cerrada de tipos: lo que el modelo sabe mirar y nosotros sabemos
  // producir. Uno inventado sale con un 400 legible en vez del 502 del servicio.
  const tipo =
    imagen !== undefined && ['image/png', 'image/jpeg', 'image/webp'].includes(String(r.imagenTipo))
      ? String(r.imagenTipo)
      : undefined;
  return {
    action: r.action,
    instruction: r.instruction,
    context: typeof r.context === 'string' ? r.context : '',
    kind: r.kind,
    ...(modo ? { modo } : {}),
    ...(imagen !== undefined ? { imagen, imagen_tipo: tipo ?? 'image/png' } : {}),
  };
}
