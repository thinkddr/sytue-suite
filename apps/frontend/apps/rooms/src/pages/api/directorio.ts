/**
 * GET /api/directorio — el directorio de la organización para la tarjeta de persona
 * (D-ROOM-04): por cada persona activa, su sub, su correo de aquí y su puesto.
 *   GET → Sytue ID GET /api/v1/directory?limit=500 → { people: [{ sub, email, title? }] }
 *
 * Proxy BFF: Sytue ID decide quién sale (solo cuentas activas de la organización y
 * solo sus buzones de aquí, nunca el correo personal de un invitado) y lo cachea por
 * organización. Aquí solo se recorta a lo que pinta la tarjeta, para no mandar al
 * navegador departamento y ubicación de toda la plantilla sin necesidad.
 *
 * Sytue ID pagina de 500 en 500 (`nextCursor`): se siguen las páginas hasta
 * MAX_PAGINAS, para que en una organización mediana nadie salga sin correo. Por
 * encima de ese tope habría que pedir por sub a Sytue ID.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';

export const prerender = false;

/** 10 × 500 personas: cubre la organización mediana sin convertir esto en un barrido. */
const MAX_PAGINAS = 10;

interface MiembroDelDirectorio {
  user_id: string;
  email?: string;
  title?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const base = (serverEnv(import.meta.env).SYTUE_ID_BASE_URL ?? 'https://id.sytue.eu').replace(
    /\/$/,
    '',
  );
  try {
    const people: { sub: string; email: string; title?: string }[] = [];
    let cursor = '';
    for (let pagina = 0; pagina < MAX_PAGINAS; pagina++) {
      const url = `${base}/api/v1/directory?limit=500${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`;
      const res = await fetch(url, {
        headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' },
      });
      if (!res.ok) return json({ error: 'id_error', status: res.status }, 502);
      const { items = [], nextCursor = '' } = (await res.json()) as {
        items?: MiembroDelDirectorio[];
        nextCursor?: string;
      };
      for (const m of items) {
        if (!m.user_id || !m.email) continue;
        people.push({ sub: m.user_id, email: m.email, ...(m.title ? { title: m.title } : {}) });
      }
      if (!nextCursor) break;
      cursor = nextCursor;
    }
    return json({ people });
  } catch {
    return json({ error: 'id_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    // Privada: es la plantilla de la organización, no se guarda en cachés compartidas.
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'private, no-store',
    },
  });
}
