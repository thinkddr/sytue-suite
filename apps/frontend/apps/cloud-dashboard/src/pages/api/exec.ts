/*
 * POST /api/exec  { project, deployment, command } -> { output, truncated, timedOut }
 *
 * Terminal en el contenedor del despliegue (WebSocket del control-plane).
 *
 * EL PROBLEMA: igual que el SSE de logs, el WebSocket del navegador no puede
 * enviar `Authorization` y el control-plane no negocia subprotocolo (así que el
 * truco de meter el token en `Sec-WebSocket-Protocol` tampoco vale: el navegador
 * corta si el servidor no lo confirma). Y el adaptador de Astro no ve las
 * peticiones `Upgrade` — un endpoint SSR no puede hacer de proxy WebSocket.
 * SOLUCIÓN: el WS lo abre el SERVIDOR (con el Bearer de la sesión) y el navegador
 * habla con esta ruta por POST de mismo origen (cookie httpOnly). Un comando por
 * petición, sin estado en el BFF: funciona con N réplicas y sin sesiones pegajosas.
 *
 * La acción viaja por POST (el checkOrigin CSRF de Astro rechaza otros verbos) y
 * el rol lo impone el control-plane (admin/owner): aquí se propagan status y
 * código REALES para que la UI diga la verdad. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { execInDeployment } from '../../lib/cloud.js';

export const prerender = false;

/** Tope de comando: una línea de shell razonable, no un script pegado por accidente. */
const MAX_COMMAND = 4096;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { project?: unknown; deployment?: unknown; command?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const project = typeof body.project === 'string' ? body.project.trim() : '';
  const deployment = typeof body.deployment === 'string' ? body.deployment.trim() : '';
  const command = typeof body.command === 'string' ? body.command.trim() : '';
  if (!project || !deployment || !command) return json({ error: 'missing_params' }, 400);
  if (command.length > MAX_COMMAND) return json({ error: 'command_too_long' }, 413);
  // Un salto de línea colaría comandos extra tras el `exit` que cierra la sesión:
  // se rechaza en el borde en vez de intentar escapar nada.
  if (/[\r\n]/.test(command)) return json({ error: 'multiline_not_supported' }, 400);

  const res = await execInDeployment(session, project, deployment, command);
  if (!res.ok) return json({ error: res.code }, res.status);
  return json({ output: res.output, truncated: res.truncated, timedOut: res.timedOut });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
