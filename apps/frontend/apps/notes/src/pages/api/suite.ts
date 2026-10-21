/**
 * GET /api/suite?kind=drive|docs|calendar|rooms|notes — LO QUE SE PUEDE INCRUSTAR EN UNA NOTA.
 *
 * Un único endpoint para los cinco selectores. Cinco ficheros con el mismo
 * esqueleto —sesión, cliente, mapear, `no-store`— divergían a la primera: es la
 * misma pregunta («enséñame lo que puedo enganchar aquí») contra cinco APIs.
 *
 * `notes` es la propia Notepads, y no es un caso raro: enlazar una nota con OTRA
 * es la referencia que hereda de BasKet Note Pads, y hasta ahora solo se podía
 * crear llegando desde otra app con `?link=`. Dentro de Notepads —justo donde se
 * escriben las notas— no había forma.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly: el access token NUNCA
 * llega al navegador y la autorización la aplica cada servicio (Drive solo lista
 * lo que este usuario puede ver, Rooms solo sus salas…). Lo que se guarda en la
 * nota es una REFERENCIA —id, nombre y enlace público—, jamás una copia de bytes:
 * si mañana se revoca el permiso sobre el fichero, la nota deja de abrirlo.
 */
import type { APIRoute } from 'astro';
import {
  CalendarClient,
  ChatClient,
  DocsClient,
  DriveClient,
  NotesClient,
  bearer,
} from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { calendarApiUrl, fallo, json } from '../../lib/notes.js';
import { deCalendar, deDocs, deNotes, deRooms, listarDrive } from '../../lib/elegibles.js';
import { contiene } from '@sytue/i18n';

export const prerender = false;

const env = serverEnv(import.meta.env);

/**
 * Cada servicio: la URL INTERNA para hablarle. El enlace público que se guarda en
 * la nota no sale de aquí sino de `appUrl` (`lib/elegibles.ts`), que lleva el
 * prefijo de cada app.
 */
const api = {
  // El Service del clúster primero: desde un pod, el nombre público no
  // resuelve y la llamada ni llega a ser HTTP (sale como 502).
  drive:
    env.SYTUE_DRIVE_INTERNAL_URL ??
    env.SYTUE_DRIVE_API_URL ??
    env.PUBLIC_SYTUE_DRIVE_BASE_URL ??
    'https://drive.sytue.eu',
  docs: env.SYTUE_DOCS_API_URL ?? env.PUBLIC_SYTUE_DOCS_BASE_URL ?? 'https://docs.sytue.eu',
  calendar: calendarApiUrl(),
  rooms: env.SYTUE_ROOMS_API_URL ?? env.PUBLIC_SYTUE_ROOMS_BASE_URL ?? 'https://rooms.sytue.eu',
  // Nosotros mismos, por la interna igual que los demás.
  notes: env.SYTUE_NOTES_BASE_URL ?? env.PUBLIC_SYTUE_NOTES_BASE_URL ?? 'https://notes.sytue.eu',
} as const;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const kind = params.get('kind');
  const q = params.get('q')?.trim() ?? '';
  const auth = bearer(session.accessToken);
  // La organización activa de la sesión: la que llevan los enlaces que se guardan
  // en la nota (ADR 0056). Puede venir vacía (alta sin org) y ahí appUrl compone
  // la forma sin org, que el middleware del destino canariza con un 302.
  const org = session.user.tenant;

  try {
    switch (kind) {
      case 'drive': {
        // Navegar por carpetas (el selector) o, con `q`, buscar por nombre en toda
        // la unidad (la herramienta de enlaces).
        const client = new DriveClient({ baseUrl: api.drive, auth });
        return json(
          await listarDrive(client, {
            path: params.get('path') || '/',
            cursor: params.get('cursor')?.trim() || undefined,
            q,
          }),
        );
      }

      case 'docs': {
        const client = new DocsClient({ baseUrl: api.docs, auth });
        const page = await client.list({ ...(q ? { q } : {}), limit: 30 });
        // La org de la sesión en el enlace (ADR 0056): sale canónico. Map a secas
        // no vale: pasaría el ÍNDICE como segundo argumento.
        return json({ items: page.documents.map((d) => deDocs(d, org)), nextCursor: null });
      }

      case 'calendar': {
        // Sin término no hay nada que buscar: el calendario no tiene «los
        // últimos eventos» y traer un rango arbitrario llenaría la lista de
        // cosas que nadie pidió.
        const client = new CalendarClient({ baseUrl: api.calendar, auth });
        const events = q ? await client.searchEvents(q, 25) : [];
        return json({ items: events.map((e) => deCalendar(e, org)), nextCursor: null });
      }

      case 'rooms': {
        const client = new ChatClient({ baseUrl: api.rooms, auth });
        const canales = await client.listChannels();
        const filtrados = q ? canales.filter((c) => contiene(c.name, q)) : canales;
        return json({ items: filtrados.map((c) => deRooms(c, org)), nextCursor: null });
      }

      case 'notes': {
        // Sin término no se lista nada, por lo mismo que en Calendar: «las
        // últimas notas» de todos los cuadernos no es una respuesta a nada.
        const client = new NotesClient({ baseUrl: api.notes, auth });
        const hits = q ? await client.search(q, 25) : [];
        return json({ items: hits.map((h) => deNotes(h, org)), nextCursor: null });
      }

      default:
        return json({ error: 'unknown_kind' }, 400);
    }
  } catch (err) {
    return fallo(err);
  }
};
