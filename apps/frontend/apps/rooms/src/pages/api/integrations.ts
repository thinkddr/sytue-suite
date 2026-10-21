/**
 * /api/integrations — gestión de INTEGRACIONES de Sytue Rooms (Ola 15): webhooks
 * entrantes/salientes y bots. Proxy BFF hacia el servicio Go Sytue Chat (el access
 * token nunca toca el navegador).
 *
 *   GET  ?channelId=…                 → { incoming, outgoing, bots, bridges, whatsapp, teams }
 *   POST { action, … }               → crea/borra/rota (un endpoint, discriminado por
 *                                       `action`; el POST esquiva el CSRF de Astro,
 *                                       como pins/emoji/categorías).
 *
 * El token/URL en CLARO de un webhook/bot solo lo devuelve el backend al crear/rotar;
 * el modal lo muestra una vez con «copiar» y no se vuelve a poder consultar.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const channelId = ctx.url.searchParams.get('channelId')?.trim();
  try {
    const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
    const [incoming, outgoing, bots, bridges, whatsapp, teams] = await Promise.all([
      channelId ? client.listIncomingWebhooks(channelId) : Promise.resolve([]),
      client.listOutgoingWebhooks(),
      client.listBots(),
      channelId ? client.listBridges(channelId) : Promise.resolve([]),
      client.listWhatsApp(),
      client.listTeamNames(),
    ]);
    // El dominio del correo a canal NO sale de aquí: lo publica el servicio con el
    // secreto de cada entrante (create/rotate), que es el que sabe si está encendido.
    return json({ incoming, outgoing, bots, bridges, whatsapp, teams });
  } catch (err) {
    return fromError(err);
  }
};

interface Payload {
  action?: string;
  channelId?: string;
  id?: string;
  botId?: string;
  name?: string;
  icon?: string;
  callbackUrl?: string;
  triggerWords?: string[];
  dialect?: string;
}

/**
 * Dialectos que el servicio reconoce (`services/rooms/internal/compat`). Se valida AQUÍ
 * y no se reenvía a ciegas: el BFF es la frontera con el navegador, y dejar pasar una
 * cadena arbitraria a la columna `dialect` haría que `NormalizeOutgoing` no la reconociera
 * y el saliente se enviara en un formato que nadie espera.
 */
const DIALECTOS = new Set(['sytue', 'slack', 'teams']);

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let p: Payload;
  try {
    p = (await ctx.request.json()) as Payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    switch (p.action) {
      case 'create-incoming': {
        if (!p.channelId || !p.name?.trim()) return json({ error: 'missing_fields' }, 400);
        return json(
          await client.createIncomingWebhook(p.channelId, p.name.trim(), p.icon?.trim()),
          201,
        );
      }
      case 'rotate-incoming': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        return json(await client.rotateIncomingWebhook(p.id));
      }
      case 'delete-incoming': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        await client.deleteIncomingWebhook(p.id);
        return json({ ok: true });
      }
      case 'create-outgoing': {
        if (!p.callbackUrl?.trim()) return json({ error: 'missing_callback' }, 400);
        return json(
          await client.createOutgoingWebhook({
            callbackUrl: p.callbackUrl.trim(),
            ...(p.channelId ? { channelId: p.channelId } : {}),
            ...(p.triggerWords?.length ? { triggerWords: p.triggerWords } : {}),
            // Sin esto, TODO saliente salía en dialecto nativo por mucho que el
            // servicio supiera hablar Slack y Teams desde la migración 0017.
            ...(p.dialect && DIALECTOS.has(p.dialect)
              ? { dialect: p.dialect as 'sytue' | 'slack' | 'teams' }
              : {}),
          }),
          201,
        );
      }
      case 'delete-outgoing': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        await client.deleteOutgoingWebhook(p.id);
        return json({ ok: true });
      }
      case 'disconnect-whatsapp': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        await client.disconnectWhatsApp(p.id);
        return json({ ok: true });
      }
      case 'delete-bridge': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        await client.deleteBridge(p.id);
        return json({ ok: true });
      }
      case 'create-bot': {
        if (!p.name?.trim()) return json({ error: 'missing_name' }, 400);
        return json(await client.createBot(p.name.trim()), 201);
      }
      case 'delete-bot': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        await client.deleteBot(p.id);
        return json({ ok: true });
      }
      case 'add-bot-channel': {
        if (!p.botId || !p.channelId) return json({ error: 'missing_fields' }, 400);
        await client.addBotToChannel(p.botId, p.channelId);
        return json({ ok: true });
      }
      case 'remove-bot-channel': {
        if (!p.botId || !p.channelId) return json({ error: 'missing_fields' }, 400);
        await client.removeBotFromChannel(p.botId, p.channelId);
        return json({ ok: true });
      }
      default:
        return json({ error: 'unknown_action' }, 400);
    }
  } catch (err) {
    return fromError(err);
  }
};

/** Mapea un error del cliente Go a una respuesta: reenvía 4xx (p. ej. 403 sin permiso)
 *  y cae a 502 para el resto. El cliente lanza «… -> <status>». */
function fromError(err: unknown): Response {
  const msg = String(err);
  const m = msg.match(/-> (\d{3})$/);
  const status = m ? Number(m[1]) : 0;
  if (status >= 400 && status < 500) return json({ error: 'chat_error', status }, status);
  return json({ error: 'chat_error', message: msg }, 502);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
