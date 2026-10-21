/*
 * GET /api/logs?project=<slug>&q=<texto>&source=build|runtime&since=&until=&cursor=
 * GET /api/logs?project=<slug>&download=1[&q=…]  -> descarga el filtro como .log
 *
 * BFF de búsqueda de registros de EJECUCIÓN: reenvía la consulta al control-plane
 * (`GET /projects/{id}/logs`) con el access token de la sesión. Acepta texto, rango de
 * fechas y cursor keyset, que es lo que ofrece el control-plane desde que se construyó
 * la recolección con retención.
 *
 * Antes esta búsqueda iba al log de la última COMPILACIÓN: quien preguntaba por qué
 * falla su aplicación en producción leía el log de cuando se construyó. Degrada al seed
 * filtrado si no hay backend vivo. NUNCA expone tokens al cliente.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { loadLogs, streamLogExport } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const project = url.searchParams.get('project')?.trim();
  if (!project) return json({ error: 'missing_params' }, 400);
  const q = url.searchParams.get('q')?.trim() || undefined;

  // Descarga: el cuerpo del control-plane se reenvía TAL CUAL (es un fichero de
  // texto que puede pesar), sin materializarlo aquí. Sin backend vivo no se
  // inventa un fichero: 503, y la pantalla lo dice.
  if (url.searchParams.get('download') === '1') {
    const upstream = await streamLogExport(session, project, q);
    if (!upstream) return json({ error: 'unavailable' }, 503);
    return upstream;
  }
  // El rango y el cursor viajan tal cual: validarlos aquí duplicaría la regla del
  // control-plane, que ya responde 400 con el motivo si vienen mal.
  // `source=build` consulta el log de compilación; cualquier otro valor, el de
  // ejecución, que es lo que se busca al preguntar qué está pasando ahora.
  const opts = {
    ...(url.searchParams.get('source') === 'build' ? { source: 'build' as const } : {}),
    ...(url.searchParams.get('since') ? { since: url.searchParams.get('since')! } : {}),
    ...(url.searchParams.get('until') ? { until: url.searchParams.get('until')! } : {}),
    ...(url.searchParams.get('cursor') ? { cursor: url.searchParams.get('cursor')! } : {}),
  };

  const { live, data, nextCursor } = await loadLogs(session, project, q, opts);
  return json({ live, logs: data, ...(nextCursor ? { nextCursor } : {}) });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
