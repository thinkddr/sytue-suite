/**
 * GET /api/descargas/<fichero> — lleva a UN instalador nativo.
 *
 * Es el único camino a los binarios: el bucket es privado y no hay URL pública
 * que compartir. Quien no tenga sesión de Sytue ID recibe un 401, y como el IdP
 * no emite token sin derecho a la suite, tener sesión ya implica suscripción viva
 * —propia, regalada o de empleado— (ver lib/descargas.ts).
 *
 * SE REDIRIGE A UNA URL FIRMADA, NO SE PROXEAN LOS BYTES. Un instalador son
 * cientos de megas: hacerlos pasar por este proceso ataría una goroutine de Bun y
 * un tramo de red por cada descarga, y tres personas a la vez bastarían para que
 * el portal dejara de responder al resto. Con la redirección, el fichero va del
 * almacenamiento al navegador y aquí solo se decide QUIÉN puede pedirlo.
 *
 * La URL vive cinco minutos: lo justo para empezar la descarga y poco para
 * reenviarla por ahí. Y no se cachea en ningún intermediario, porque es
 * exactamente lo contrario de contenido compartible.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth';
import { esInterno, firmarInstalador } from '../../../lib/descargas.js';

export const prerender = false;

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const fichero = String(ctx.params.fichero ?? '');
  const firmada = await firmarInstalador(fichero);
  if (!firmada) return json({ error: 'not_found' }, 404);

  // SI LA URL FIRMADA NO LA PUEDE ABRIR UN NAVEGADOR, SE SIRVE DESDE AQUÍ.
  //
  // La redirección es el camino bueno y sigue siendo el de siempre, pero exige
  // que el almacenamiento esté publicado. Con un S3 dentro del clúster —el caso
  // del piloto— el host firmado es un nombre de servicio de Kubernetes: el
  // navegador lo pide, no lo resuelve y la descarga muere con `ETIMEOUT`. Medido
  // el 2026-09-12.
  //
  // Antes que redirigir a donde no se llega, se hace de intermediario. Va en
  // STREAMING, así que no se bufferizan cientos de megas: lo que cuesta es una
  // conexión abierta mientras dura la descarga, y eso en una instalación con el
  // almacenamiento dentro es lo que hay. Publicar el S3 —definiendo
  // `downloadsPublicEndpoint`— devuelve el 302 y este camino deja de usarse.
  if (esInterno(firmada)) {
    const origen = await fetch(firmada);
    if (!origen.ok || !origen.body) return json({ error: 'not_found' }, 404);
    return new Response(origen.body, {
      status: 200,
      headers: {
        'content-type': origen.headers.get('content-type') ?? 'application/octet-stream',
        ...(origen.headers.get('content-length')
          ? { 'content-length': origen.headers.get('content-length') as string }
          : {}),
        'content-disposition': `attachment; filename="${fichero.replace(/"/g, '')}"`,
        'cache-control': 'private, no-store',
      },
    });
  }

  return new Response(null, {
    status: 302,
    headers: {
      location: firmada,
      // 302 y `no-store`: la URL caduca, así que un intermediario que la guardara
      // serviría un enlace muerto —o, peor, uno vivo a quien no le toca—.
      'cache-control': 'private, no-store',
    },
  });
};
