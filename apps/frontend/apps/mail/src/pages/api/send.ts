/**
 * POST /api/send — envia un correo. Sin CCO va por el SDK JMAP (que compone el
 * RFC 5322 y lo entrega al endpoint REST /api/v1/send del servicio Go: firma DKIM
 * + cola). CON CCO (bcc), como el `send` del SDK deriva el sobre solo de To+Cc, el
 * BFF compone el mensaje (cabeceras To/Cc, nunca Bcc) y lo entrega con el sobre
 * incluyendo a los Bcc como destinatarios ocultos. El remitente se fija al usuario
 * autenticado (no se confia en el cliente).
 *
 * Cuerpo JSON: { to, cc?, bcc?, subject, textBody?, htmlBody?, attachments? }.
 * Se toleran (e ignoran) identityId/draftMailboxId de clientes antiguos.
 */
import type { APIRoute } from 'astro';
import type { EmailAddress, OutgoingAttachment, OutgoingEmail } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';
import { mailPost } from '../../lib/mailrest.js';
import { direccionDeEnvio } from '../../lib/identidad.js';
import { buildRfc5322, envelopeRecipients } from '../../lib/rfc5322.js';
import { decodeAttachment, type AttachmentPayload } from '../../lib/attachments.js';

export const prerender = false;

/**
 * Traduce el rechazo del servicio a una respuesta que el navegador PUEDA EXPLICAR.
 *
 * Antes todo se aplanaba a `502 {error:'mail_error'}`: un 403 «no puedes enviar
 * como esa dirección» —que tiene arreglo y lo tiene quien está delante— llegaba
 * como un error de servidor genérico, y el compositor solo sabía decir «no se ha
 * podido enviar». El motivo existía en los tres saltos y se tiraba en el primero.
 *
 * Se conserva el estado cuando es 4xx (culpa del cliente, accionable) y se
 * reenvía el `code` del servicio, que es su clave estable de traducción
 * (platform/pkg/httpx: «Lo que el cliente traduce es Code»). El 5xx sigue saliendo
 * como 502: un fallo del servicio no se le explica a quien escribe el correo.
 */
async function fallo(res: Response): Promise<Response> {
  const cuerpo = (await res.json().catch(() => ({}))) as { code?: string; message?: string };
  const cliente = res.status >= 400 && res.status < 500;
  return json(
    {
      error: 'mail_error',
      status: res.status,
      ...(cuerpo.code ? { code: cuerpo.code } : {}),
      ...(cuerpo.message ? { message: cuerpo.message } : {}),
    },
    cliente ? res.status : 502,
  );
}

interface Payload {
  to?: EmailAddress[];
  cc?: EmailAddress[];
  bcc?: EmailAddress[];
  subject?: string;
  textBody?: string;
  htmlBody?: string;
  attachments?: AttachmentPayload[];
  identityId?: string;
  draftMailboxId?: string;
  /** Enviar-como: dirección «De» (primaria o alias verificado). El servicio valida. */
  from?: string;
  /** Programar (ISO 8601) o deshacer (segundos de gracia): difieren el envío. */
  sendAt?: string;
  undoSeconds?: number;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!Array.isArray(body.to) || body.to.length === 0)
    return json({ error: 'missing_recipients' }, 400);

  let attachments: OutgoingAttachment[];
  try {
    attachments = (body.attachments ?? []).map(decodeAttachment);
  } catch {
    return json({ error: 'invalid_attachment' }, 400);
  }

  // Enviar-como / enviar-en-nombre-de: la dirección «De» elegida (primaria, alias,
  // buzón compartido o buzón delegado). El servicio Go VERIFICA la autorización
  // (permiso send) y rechaza una identidad ajena; aquí solo se propaga. Si el From
  // no es la propia dirección, NO se adjunta el nombre del usuario autenticado
  // (sería el del asistente, no el del buzón/directivo); en delegación el servicio
  // sella además Sender = el remitente real.
  //
  // La dirección PROPIA es la del buzón, no la de la sesión de Sytue ID: quien
  // entra como ops@sytue.com puede tener el buzón en ops@corporate.sytue.eu, y
  // usar la de la sesión hacía que el servicio rechazara el envío con 403.
  const propia = await direccionDeEnvio(session.accessToken, session.user.id, session.user.email);
  const fromEmail = typeof body.from === 'string' && body.from.trim() ? body.from.trim() : propia;
  const isCustomFrom = fromEmail.toLowerCase() !== propia.toLowerCase();
  const from: EmailAddress = isCustomFrom
    ? { email: fromEmail }
    : { name: session.user.name, email: fromEmail };
  const subject = body.subject ?? '(Sin asunto)';
  const bcc = Array.isArray(body.bcc) ? body.bcc.filter((a) => a?.email) : [];
  // Diferir (programado/deshacer) exige el camino RAW: el SDK JMAP compartido no
  // acepta send_at/undo_seconds, así que el BFF compone el mensaje y lo entrega al
  // endpoint REST del servicio Go con esos campos. El CCO y el enviar-como (alias)
  // usan también este camino: el BFF controla las cabeceras From/To y el envelope.
  const deferred =
    Boolean(body.sendAt) || (typeof body.undoSeconds === 'number' && body.undoSeconds > 0);

  try {
    if (deferred || bcc.length > 0 || isCustomFrom) {
      const raw = buildRfc5322({
        from,
        to: body.to,
        cc: body.cc ?? [],
        subject,
        ...(body.htmlBody != null ? { html: body.htmlBody } : { text: body.textBody ?? '' }),
        attachments,
      });
      const res = await mailPost(session.accessToken, '/api/v1/send', {
        from: from.email,
        recipients: envelopeRecipients(body.to, body.cc, bcc),
        // Dentro del JSON si cabe en el MiB del servicio; si no, subido aparte.
        ...(await jmapClient(session.accessToken).rawPayload(raw)),
        ...(body.sendAt ? { send_at: body.sendAt } : {}),
        ...(body.undoSeconds ? { undo_seconds: body.undoSeconds } : {}),
      });
      if (!res.ok) return await fallo(res);
      const data = (await res.json().catch(() => ({}))) as {
        id?: string;
        release_at?: string;
        status?: string;
      };
      return json({ ok: true, id: data.id, releaseAt: data.release_at, status: data.status });
    }

    const email: OutgoingEmail = {
      // El from se ata a la identidad del usuario autenticado.
      from,
      to: body.to,
      subject,
      ...(body.cc ? { cc: body.cc } : {}),
      ...(body.textBody != null ? { textBody: body.textBody } : {}),
      ...(body.htmlBody != null ? { htmlBody: body.htmlBody } : {}),
      ...(attachments.length ? { attachments } : {}),
    };
    await jmapClient(session.accessToken).send(email);
    return json({ ok: true });
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
