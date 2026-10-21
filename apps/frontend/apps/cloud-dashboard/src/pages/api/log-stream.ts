/*
 * NOMBRE DEL FICHERO: esta ruta estuvo en `api/logs/stream.ts` y `.gitignore` la
 * descartaba en silencio — la regla genérica `logs/` (pensada para directorios de
 * log en ejecución) se traga CUALQUIER carpeta con ese nombre, también de código.
 * Se dejó plana a propósito: forzar el add habría colado este fichero y perdido el
 * siguiente que alguien creara ahí.
 *
 * GET /api/log-stream?project=<slug>&deployment=<uuid>
 *
 * Tubería de los logs de RUNTIME en vivo (streaming).
 *
 * EL PROBLEMA: el control-plane exige `Authorization: Bearer` y ni EventSource ni
 * WebSocket permiten poner cabeceras desde el navegador (que además no habla
 * directo con el control-plane). SOLUCIÓN: el navegador pide este endpoint de
 * MISMO ORIGEN — autenticado solo, con la cookie httpOnly de sesión — y el
 * servidor abre la conexión real al control-plane añadiendo el Bearer. El cuerpo
 * se reenvía tal cual, sin bufferizar. El access token nunca sale del servidor.
 *
 * El cliente lo consume con `fetch` + lector de ReadableStream, NO con
 * EventSource: EventSource no deja leer el status de la respuesta (un 403 por
 * falta de rol llegaría como un `onerror` mudo, indistinguible de «no hay
 * backend») y encima reconecta solo en bucle. Con fetch, un fallo es un status
 * HTTP normal con su código real y la UI puede decir la verdad.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { openRuntimeLogStream } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const project = url.searchParams.get('project')?.trim();
  const deployment = url.searchParams.get('deployment')?.trim();
  if (!project || !deployment) return json({ error: 'missing_params' }, 400);

  const open = await openRuntimeLogStream(session, project, deployment, ctx.request.signal);
  if (!open.ok) return json({ error: open.code }, open.status);

  return new Response(open.body, {
    headers: {
      'content-type': 'text/event-stream; charset=utf-8',
      'cache-control': 'no-store',
      connection: 'keep-alive',
      // Sin esto un proxy con buffer (nginx/ingress) retiene el stream y los
      // «logs en vivo» llegarían a ráfagas de varios KiB.
      'x-accel-buffering': 'no',
    },
  });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
