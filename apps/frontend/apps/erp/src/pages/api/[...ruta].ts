/**
 * BFF de SyERP: UN proxy para toda la API.
 *
 * El servicio expone una sola API genérica —`/api/v1/{modelo}` sirve a los
 * treinta y seis modelos— así que el BFF también es uno. Escribir un fichero de
 * proxy por recurso sería copiar catorce veces las mismas doce líneas y volver a
 * tocarlas cada vez que se añade un módulo.
 *
 * El navegador NUNCA ve el access token: habla con `/erp/api/*` usando la cookie
 * de sesión httpOnly, y el Bearer se pone aquí. La autorización la sigue
 * aplicando el servicio recurso a recurso.
 */
import type { APIContext, APIRoute } from 'astro';
import { apiUrl, getApiSession } from '../../lib/bff.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/**
 * Rutas admitidas. Lista blanca de FORMA, no de nombres: los modelos los decide
 * el catálogo del servicio, y enumerarlos aquí obligaría a tocar el frontend cada
 * vez que se añade un módulo — justo lo que este diseño evita.
 *
 * Lo que sí se corta: cualquier cosa que no sean segmentos simples. Sin esto, un
 * `..` en la ruta saldría de `/api/v1` y llegaría a otra superficie del servicio
 * con el token de quien navega.
 */
const RUTA_VALIDA = /^[a-z0-9][a-z0-9_-]*(\/[a-zA-Z0-9_-]+)*$/;

async function reenviar(ctx: APIContext): Promise<Response> {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const ruta = (ctx.params.ruta ?? '').replace(/^\/+|\/+$/g, '');
  if (!RUTA_VALIDA.test(ruta)) return json({ error: 'bad_path' }, 400);

  const destino = `${apiUrl()}/api/v1/${ruta}${ctx.url.search}`;
  const cabeceras: Record<string, string> = {
    authorization: `Bearer ${session.accessToken}`,
    // El servicio traduce el catálogo con esto: los rótulos de los veinticuatro
    // módulos llegan ya en el idioma de quien mira, y el frontend no guarda ni
    // una cadena de producto.
    'accept-language': ctx.locals.locale ?? 'en',
  };
  // La petición CONDICIONAL, tal cual viene del navegador. Sin reenviarla, el
  // servicio no puede contestar 304 y el catálogo —32 KB— vuelve a bajar entero
  // en cada navegación. El ETag existe justo para que eso no pase, y un proxy
  // que se lo come lo anula sin que nada falle.
  const siNoCoincide = ctx.request.headers.get('if-none-match');
  if (siNoCoincide) cabeceras['if-none-match'] = siNoCoincide;
  const metodo = ctx.request.method.toUpperCase();
  let cuerpo: ArrayBuffer | undefined;
  if (metodo !== 'GET' && metodo !== 'HEAD') {
    // En BYTES también a la ida: el Norma 43 de los bancos va en ISO-8859-1, y
    // leerlo como texto cambiaba cada «Ñ» por el carácter de reemplazo.
    cuerpo = await ctx.request.arrayBuffer();
    // El tipo del cuerpo lo dice quien lo manda: JSON casi siempre, `text/csv`
    // en la importación. Fijarlo aquí a JSON convertía el CSV en un 400.
    cabeceras['content-type'] = ctx.request.headers.get('content-type') ?? 'application/json';
  }

  try {
    // `body` va condicional y no como `undefined`: con
    // `exactOptionalPropertyTypes` un undefined explícito no es lo mismo que no
    // pasar la propiedad, y un GET con body no es una petición válida.
    const resp = await fetch(destino, {
      method: metodo,
      headers: cabeceras,
      ...(cuerpo === undefined ? {} : { body: cuerpo }),
    });

    // 304: sin cuerpo, y el navegador usa lo que ya tiene. Devolverlo con cuerpo
    // —o como 200— sería exactamente perder la optimización.
    const salida = new Headers({
      'content-type': resp.headers.get('content-type') ?? 'application/json; charset=utf-8',
    });
    // El validador y la política de caché los decide el SERVICIO, que es quien
    // sabe cuándo cambia cada recurso. El respaldo es `no-store`: son datos de
    // una persona y lo prudente por defecto es no guardarlos.
    const etag = resp.headers.get('etag');
    if (etag) salida.set('etag', etag);
    // `Content-Disposition` DECIDE SI ALGO SE DESCARGA. Sin reenviarlo, la
    // exportación a CSV llegaba al navegador sin él y se pintaba como texto en
    // la pestaña en vez de bajarse como fichero — la función entera parecía
    // rota sin que fallara nada.
    const adjunto = resp.headers.get('content-disposition');
    if (adjunto) salida.set('content-disposition', adjunto);
    salida.set('cache-control', resp.headers.get('cache-control') ?? 'no-store');
    if (resp.status === 304) return new Response(null, { status: 304, headers: salida });

    // En BYTES y no como texto: el Excel de Sytue AI es un ZIP, y decodificarlo
    // como UTF-8 lo corrompe sin que nada falle hasta que Excel no lo abre.
    const cuerpoResp = await resp.arrayBuffer();
    return new Response(cuerpoResp.byteLength > 0 ? cuerpoResp : null, {
      status: resp.status,
      headers: salida,
    });
  } catch {
    // El servicio no respondió: 502 y no 500. La interfaz distingue «se ha caído
    // el ERP» de «lo que has enviado no vale».
    return json({ error: 'upstream_unavailable' }, 502);
  }
}

export const GET: APIRoute = reenviar;
export const POST: APIRoute = reenviar;
export const PUT: APIRoute = reenviar;
export const DELETE: APIRoute = reenviar;
