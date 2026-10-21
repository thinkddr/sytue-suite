/**
 * GET /api/notebooks/:id/export?format=…&view=1 — descarga o abre el cuaderno.
 *
 * A diferencia del resto del BFF, esta ruta NO devuelve JSON: reenvía el cuerpo
 * del servicio tal cual. El PDF es binario y un volcado puede pesar, así que se
 * pasa el `body` sin leerlo —en streaming— en vez de materializarlo aquí.
 */
import type { APIRoute } from 'astro';
import { NotesClient } from '@sytue/api-clients';
import { conCliente, fallo, json } from '../../../../lib/notes.js';

export const prerender = false;

type Formato = (typeof NotesClient.formatosExport)[number];

const FORMATOS = new Set<string>(NotesClient.formatosExport);

export const GET: APIRoute = async (ctx) => {
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);

  // Sin `?format=` sale el NATIVO, igual que en el servicio: quien no elige
  // quiere su cuaderno entero, y el HTML es el único que se lo lleva.
  //
  // El formato se valida AQUÍ además de en el servicio: así un `?format=` raro no
  // gasta un viaje, y sobre todo no acaba en la cabecera de respuesta de abajo.
  const formato = (ctx.url.searchParams.get('format') || 'html').toLowerCase();
  if (!FORMATOS.has(formato)) return json({ error: 'unsupported_format' }, 400);

  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    // `?note=` exporta una sola nota del cuaderno; el servicio valida el id y
    // contesta 404 si no es de este cuaderno.
    const nota = ctx.url.searchParams.get('note') ?? undefined;
    const r = await c.client.exportNotebook(
      id,
      formato as Formato,
      ctx.url.searchParams.get('view') === '1',
      nota,
    );
    // Se copian solo las cabeceras que describen el fichero, no las que traiga el
    // servicio: `Content-Disposition` lleva el nombre y `Content-Security-Policy`
    // es lo que hace seguro abrir un PDF en la pestaña. El resto —cookies del
    // upstream, cabeceras de infraestructura— no tiene por qué salir al navegador.
    const headers = new Headers({ 'cache-control': 'no-store' });
    for (const h of [
      'content-type',
      'content-disposition',
      'content-security-policy',
      'x-content-type-options',
    ]) {
      const v = r.headers.get(h);
      if (v) headers.set(h, v);
    }
    return new Response(r.body, { status: r.status, headers });
  } catch (err) {
    return fallo(err);
  }
};
