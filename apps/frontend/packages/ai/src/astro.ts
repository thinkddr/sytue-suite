/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL PROXY DEL ASISTENTE, uno solo para las apps que solo proxyan.
 *
 * Recibe { action, instruction, context, kind } (lo compone el panel, puro y
 * testeado), añade la sesión httpOnly como Authorization: Bearer y REENVÍA a un
 * servicio de IA CONFIGURABLE por env (SYTUE_AI_URL). El BFF NO lleva lógica de
 * IA: solo proxya (memoria «backend siempre Go»). Devuelve la respuesta completa
 * del servicio ({ text }) o un error con mensaje claro (incl. «no configurado»).
 *
 * POR QUÉ AQUÍ. Estaba copiado en Calendar, Contactos, Mail y Notepads, idéntico
 * salvo el nombre del producto en el comentario: 145 líneas por app de validación
 * en la frontera de confianza, topes de tamaño y dos plazos de espera. Cuatro
 * copias de una validación de entrada es cuatro sitios donde arreglar el mismo
 * agujero, y ya se vio lo que pasa: la nota sobre la base del servicio se
 * corrigió en Docs y en Drive y no llegó a estas cuatro.
 *
 * NO lo usan Docs ni Drive, y es a propósito: ver el final del fichero.
 *
 * ponytail: respuesta completa (sin streaming) en el MVP; si el servicio expone
 * SSE/stream, reenviar el ReadableStream aquí y consumirlo incremental en el panel.
 */
import type { APIContext, APIRoute } from 'astro';

/** Holgura sobre el tope de contexto del cliente (12k chars): rechaza antes de leer. */
const MAX_BYTES = 256 * 1024;

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

/**
 * URL del endpoint de asistencia a partir de `SYTUE_AI_URL`.
 *
 * LA VARIABLE ES LA BASE DEL SERVICIO, no el endpoint: el servicio tiene dos
 * (`/assist` y `/transcribe`) y una variable con la ruta dentro no puede servir
 * a los dos. Componerla aquí es cosa de quien sabe a qué llama.
 *
 * Se tolera que ya venga con la ruta puesta: hay instalaciones configuradas con
 * la URL completa, y romperlas al actualizar sería cambiar un fallo por otro.
 */
export function endpointAsistencia(base: string): string {
  const limpia = base.replace(/\/+$/, '');
  return limpia.endsWith('/api/v1/assist') ? limpia : `${limpia}/api/v1/assist`;
}

interface AiProxyBody {
  action: string;
  instruction: string;
  context: string;
  kind: string;
  /** Modelo a usar: 'interactivo' (rápido, por defecto) o 'calidad' (acierta
   *  más, tarda del orden de quince segundos). Se omite si no viene. */
  modo?: 'interactivo' | 'calidad';
  /** Pisa el `reasoning_effort` del modo (none|low|medium|high). Lo decide
   *  quien llama porque depende de la TAREA, no del despliegue — ver
   *  `services/ai/internal/http/http.go`, campo `Esfuerzo`. Se omite si no
   *  viene: el servicio ya tiene un valor por defecto sensato. */
  esfuerzo?: 'none' | 'low' | 'medium' | 'high';
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
  // Misma lista cerrada que `modo`: un valor raro se traduce en «sin esfuerzo
  // pisado», que es el que decida el servicio por su cuenta.
  const esfuerzo =
    r.esfuerzo === 'none' ||
    r.esfuerzo === 'low' ||
    r.esfuerzo === 'medium' ||
    r.esfuerzo === 'high'
      ? r.esfuerzo
      : undefined;
  return {
    action: r.action,
    instruction: r.instruction,
    context: typeof r.context === 'string' ? r.context : '',
    kind: r.kind,
    ...(modo ? { modo } : {}),
    ...(esfuerzo ? { esfuerzo } : {}),
  };
}

/** Lo mínimo que el proxy necesita de la sesión: el token con el que firma. */
type LeerSesion = (ctx: APIContext) => Promise<{ accessToken: string } | null>;

/**
 * Los dos handlers de `POST /api/ai` para una app que solo proxya.
 *
 * `env` es el entorno EFECTIVO del servidor (`serverEnv(import.meta.env)`), y se
 * lee una vez al montar la ruta, igual que hacía cada copia. `SYTUE_AI_URL` no
 * tiene valor por defecto a propósito: ausente = función deshabilitada, con un
 * mensaje claro en vez de un 502 sin causa.
 */
export function crearProxyDeAsistente(
  env: Readonly<Record<string, string | undefined>>,
  sesion: LeerSesion,
): { GET: APIRoute; POST: APIRoute } {
  const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

  const POST: APIRoute = async (ctx) => {
    const session = await sesion(ctx);
    if (!session) return json({ error: 'unauthenticated' }, 401);

    const url = env.SYTUE_AI_URL;
    if (!url) return json({ error: 'ai_not_configured' }, 503);

    const len = Number(ctx.request.headers.get('content-length') ?? '0');
    if (Number.isFinite(len) && len > MAX_BYTES) return json({ error: 'bad_request' }, 413);

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

    let upstream: Response;
    try {
      upstream = await fetch(endpointAsistencia(url), {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          authorization: `Bearer ${session.accessToken}`,
        },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(body.modo === 'calidad' ? TIMEOUT_CALIDAD_MS : TIMEOUT_MS),
      });
    } catch (e) {
      // AbortSignal.timeout aborta con TimeoutError; el resto es red/servicio caído.
      const timedOut = e instanceof Error && e.name === 'TimeoutError';
      return json(
        { error: timedOut ? 'ai_timeout' : 'upstream_unreachable' },
        timedOut ? 504 : 502,
      );
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

  return { GET, POST };
}

/*
 * QUIÉN NO USA ESTO, Y POR QUÉ NO SE LE FORZÓ.
 *
 * - Sytue Docs manda ADEMÁS una página rasterizada en base64 para leer escaneos:
 *   tope distinto (1 MiB), validación de la imagen y de su tipo, y el tope de
 *   texto aplicado DESPUÉS de leer el cuerpo porque hasta entonces no se sabe si
 *   viene imagen. Meterlo aquí con un booleano `admitirImagen` sería una opción
 *   con un solo consumidor y un camino de validación que nadie más recorre.
 * - Sytue Drive no es un proxy: LEE el fichero con el token de la sesión, decide
 *   si es texto, lo recorta y compone él la instrucción. No comparte cuerpo con
 *   esto más allá de que acaba llamando al mismo servicio.
 */
