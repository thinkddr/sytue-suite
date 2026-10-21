/**
 * /api/reports — denuncias (proxy BFF hacia el servicio Go; el access token no
 * toca el navegador).
 *   GET                                               → { reports: [...] } (cola de quien modera)
 *   POST { messageId, reason, note? }                 → denuncia un mensaje
 *   POST { channelId, subject, reason, note? }        → denuncia a una persona del canal
 *   POST { reportId, action }                         → resuelve una denuncia
 *
 * Sin lógica de negocio: quién denuncia y quién puede moderar lo decide el
 * servicio a partir del token. El 429 (tope de denuncias) se preserva.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, chatFail, REPORT_REASONS, type ReportAction } from '../../lib/chat.js';

export const prerender = false;

const ACTIONS: readonly string[] = ['dismissed', 'message_deleted', 'member_removed'];

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const reports = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listReports();
    return json({ reports });
  } catch (err) {
    return chatFail(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    messageId?: string;
    channelId?: string;
    subject?: string;
    reason?: string;
    note?: string;
    reportId?: string;
    action?: string;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    const reportId = payload.reportId?.trim();
    if (reportId) {
      if (!payload.action || !ACTIONS.includes(payload.action)) {
        return json({ error: 'invalid_action' }, 400);
      }
      await client.resolveReport(reportId, payload.action as ReportAction);
      return json({ ok: true });
    }
    const reason = payload.reason ?? '';
    if (!(REPORT_REASONS as readonly string[]).includes(reason)) {
      return json({ error: 'invalid_reason' }, 400);
    }
    const messageId = payload.messageId?.trim();
    const channelId = payload.channelId?.trim();
    const subject = payload.subject?.trim();
    if (!messageId && !(channelId && subject)) return json({ error: 'missing_target' }, 400);
    await client.report({
      ...(messageId ? { messageId } : { channelId: channelId!, subject: subject! }),
      reason,
      note: (payload.note ?? '').slice(0, 500),
    });
    return json({ ok: true });
  } catch (err) {
    return chatFail(err);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
