/**
 * Las respuestas JSON de los BFF de Mail que traducen rechazos del servicio Go: las
 * plantillas, posponer y silenciar. Un rechazo que la pantalla sabe explicar (400,
 * 404, 409) conserva su estado y el `code` estable del sobre de error del servicio;
 * el resto es un 502. Sin eso, «ya tienes una que se llama así» y «no hay red»
 * acababan en el mismo aviso genérico.
 */

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** El rechazo del servicio, con su estado si se sabe explicar y su `code`. */
export async function rechazoDelServicio(res: Response): Promise<Response> {
  const sobre = (await res.json().catch(() => ({}))) as { error?: { code?: string } };
  const code = sobre.error?.code;
  const status = [400, 404, 409].includes(res.status) ? res.status : 502;
  return json({ error: 'mail_error', status: res.status, ...(code ? { code } : {}) }, status);
}
