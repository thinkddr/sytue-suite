import type { APIRoute } from 'astro';
import { DriveClient, bearer, type DriveResource } from '@sytue/api-clients';
import { bff, toBffContext, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/** La carpeta donde CertiPDF guarda lo que abre o crea (ver pdf-abrir.ts). */
const CARPETA = '/PDF';
const MAXIMO = 48;

export interface PdfReciente {
  name: string;
  path: string;
  modifiedAt: string | null;
  size: number | null;
}

/**
 * Los PDF recientes de la persona: lo que hay en /PDF de su Drive, del más
 * reciente al más antiguo. Es la lista de la portada de CertiPDF; los PDF no
 * son documentos de Docs (viven en Drive con su ACL y sus versiones), así que
 * no salen de `/api/documents`.
 */
export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
  // Sin carpeta todavía no hay PDF: es el estado de quien no ha abierto ninguno,
  // no un fallo. Lo que falle DESPUÉS de saber que existe sí es un fallo.
  const existe = await client.stat(CARPETA).then(
    () => true,
    () => false,
  );
  if (!existe) return json({ items: [] }, 200);
  try {
    const entradas: DriveResource[] = await client.list(CARPETA);
    const items: PdfReciente[] = entradas
      .filter((e) => !e.isDirectory && /\.pdf$/i.test(e.name))
      .sort((a, b) => (b.modifiedAt ?? '').localeCompare(a.modifiedAt ?? ''))
      .slice(0, MAXIMO)
      .map((e) => ({
        name: e.name,
        path: e.path,
        modifiedAt: e.modifiedAt ?? null,
        size: e.size ?? null,
      }));
    return json({ items }, 200);
  } catch {
    return json({ error: 'drive_unavailable' }, 502);
  }
};

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
