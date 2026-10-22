/**
 * GET /api/doc-preview?id=<documento> — VISTA PREVIA de un documento de Sytue
 * Docs incrustado en una nota.
 *
 * Devuelve el documento exportado a HTML, servido DESDE EL ORIGEN DE ESTA APP
 * para que la nota lo pinte en un `<iframe sandbox>`. Con `sandbox` vacío el
 * navegador le da un origen opaco y sin scripts: es la misma barrera que un
 * saneador, la pone el navegador, y además impide que el CSS del documento se
 * derrame sobre el cromo de la nota. Es el patrón que ya usa el visor WOPI de
 * Sytue Docs.
 *
 * Se hace aquí y no con un iframe a docs.sytue.eu porque toda la suite se sirve
 * con `frame-ancestors 'none'` —la verja contra el clickjacking— y relajarla para
 * TODO Docs, con su editor y su sesión dentro, sería pagar un agujero por una
 * previsualización. El middleware de esta app relaja `frame-ancestors` solo para
 * ESTA ruta, que no tiene sesión propia ni acciones.
 *
 * El export sale con la letra de Docs (lib/vista-doc.ts): sin ella, el navegador lo
 * pintaba en Times.
 *
 * La autorización la aplica Docs: el export se pide con el token del usuario, así
 * que un documento que no puede ver devuelve 403 y aquí no hay nada que enseñar.
 */
import type { APIRoute } from 'astro';
import { DocsClient, HttpError, bearer } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import figtree from '@fontsource-variable/figtree/files/figtree-latin-wght-normal.woff2?inline';
import { getApiSession } from '../../lib/session.js';
import { conLetraDeDocs } from '../../lib/vista-doc.js';

export const prerender = false;

const env = serverEnv(import.meta.env);

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response('', { status: 401 });

  const id = new URL(ctx.request.url).searchParams.get('id')?.trim();
  if (!id) return new Response('', { status: 400 });

  const client = new DocsClient({
    baseUrl: env.SYTUE_DOCS_API_URL ?? env.PUBLIC_SYTUE_DOCS_BASE_URL ?? 'https://docs.sytue.eu',
    auth: bearer(session.accessToken),
  });
  try {
    const blob = await client.createExport(id, 'html');
    const { bytes } = await client.downloadExport(id, blob.id);
    return new Response(conLetraDeDocs(new TextDecoder().decode(bytes), figtree), {
      status: 200,
      headers: {
        'content-type': 'text/html; charset=utf-8',
        // Es contenido de un usuario y cambia al editarse: nunca en caché.
        'cache-control': 'no-store',
      },
    });
  } catch (err) {
    const status = err instanceof HttpError ? err.status : 502;
    return new Response('', { status });
  }
};
