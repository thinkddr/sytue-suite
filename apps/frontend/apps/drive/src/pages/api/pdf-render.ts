/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/pdf-render — las primeras páginas de un PDF de Drive, dibujadas por
 * Sytue PDF, para la VISTA PREVIA de la lista de ficheros.
 *
 * POR QUÉ NO EL VISOR DEL NAVEGADOR, que es lo que había: un `<iframe>` con el
 * blob delega el dibujo en el visor del sistema, y eso da tres resultados
 * distintos en las seis plataformas —Chrome lo pinta, algunos WebView de Android
 * lo DESCARGAN en vez de mostrarlo, y en iOS aparece un lector propio con su
 * cromo encima del nuestro—. Ahí no hay nada que ajustar: no es nuestro.
 *
 * El motor de la casa devuelve SVG (ADR-0033), así que la vista previa es la
 * misma en todas partes, el texto se puede seleccionar y copiar, y el zoom no
 * pixela. Y ya estaba escrito: esto solo lo hace alcanzable desde Drive.
 *
 * Puente igual que /api/pdf-inspect y /api/pdf-pages: descarga el fichero con el
 * token de la sesión y lo pasa por `POST /api/v1/pdf/render` de Docs. El token no
 * llega al cliente y no se guarda nada — se dibuja y se devuelve.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, driveClientFor, driveFetch, docsBaseUrl } from '../../lib/auth';

export const prerender = false;

const MAX_BYTES = 64 * 1024 * 1024;

/**
 * Tope de páginas por vista previa.
 *
 * Es una VISTA PREVIA: quien quiera leer el documento entero tiene «Abrir en
 * Sytue PDF» al lado, que además trae la barra de herramientas. Dibujar las
 * trescientas páginas de un expediente para enseñar un diálogo sería pagar el
 * documento completo por mirar la portada.
 */
const MAX_PAGINAS = 5;

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let path = '';
  let paginas = '';
  let share = '';
  let subpath = '';
  try {
    const cuerpo = (await ctx.request.json()) as {
      path?: unknown;
      paginas?: unknown;
      share?: unknown;
      subpath?: unknown;
    };
    path = typeof cuerpo.path === 'string' ? cuerpo.path.trim() : '';
    paginas = typeof cuerpo.paginas === 'string' ? cuerpo.paginas.trim() : '';
    share = typeof cuerpo.share === 'string' ? cuerpo.share.trim() : '';
    subpath = typeof cuerpo.subpath === 'string' ? cuerpo.subpath : '';
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  if (!path && !share) return json({ error: 'missing_path' }, 400);

  // El cliente puede pedir una ventana concreta al desplazarse, pero el tope lo
  // fija el servidor: `paginas` viene del navegador y «1-5000» sería una forma
  // educada de pedirnos que dibujemos un expediente entero.
  if (paginas !== '' && cuantasPide(paginas) > MAX_PAGINAS) {
    return json({ error: 'too_many_pages' }, 400);
  }

  // DOS PUERTAS, y la del compartido NO se puede sustituir por la otra: un
  // fichero de «Compartido conmigo» no está en la unidad de quien lo mira, así
  // que `client.download(path)` daría 404 aunque el share sea válido. Se usa la
  // misma ruta gated que `/api/shared-content`, con su verja por share, en vez de
  // aceptar una URL del cliente —eso sería pedirnos que descarguemos lo que nos
  // digan—.
  let original: Blob;
  if (share) {
    const qs = subpath ? `?subpath=${encodeURIComponent(subpath)}` : '';
    const res = await driveFetch(ctx, `/api/v1/shared/${encodeURIComponent(share)}/content${qs}`);
    if (!res) return json({ error: 'unauthenticated' }, 401);
    if (!res.ok) return json({ error: 'download_failed' }, res.status >= 500 ? 502 : res.status);
    original = await res.blob();
  } else {
    const client = await driveClientFor(ctx);
    if (!client) return json({ error: 'unauthenticated' }, 401);
    try {
      original = await client.download(path);
    } catch {
      return json({ error: 'download_failed' }, 404);
    }
  }
  if (original.size > MAX_BYTES) return json({ error: 'too_large' }, 413);

  const nombre = (share ? subpath : path).split('/').pop() || 'documento.pdf';

  /** Una pasada del motor sobre el mismo fichero, ya descargado. */
  const dibujar = async (lista: string): Promise<Response> => {
    const form = new FormData();
    form.append('file', original, nombre);
    form.append('paginas', lista);
    // Sin anotaciones: una vista previa enseña el DOCUMENTO. Los comentarios y
    // resaltados de quien lo esté revisando se ven al abrirlo, donde además se
    // pueden contestar.
    form.append('sin_anotaciones', '1');
    return fetch(`${docsBaseUrl()}/api/v1/pdf/render`, {
      method: 'POST',
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
      body: form,
    });
  };

  // LA PRIMERA PÁGINA SIEMPRE, Y SOLA, aunque se vayan a pedir cinco.
  //
  // El motor rechaza el lote entero si el rango se sale —«Page 1-5 does not
  // exist: the document has 2»— así que pedir `1-5` de entrada rompería la vista
  // previa de casi todos los PDF, que tienen menos de cinco páginas. La página 1
  // existe siempre, y su respuesta trae el `total` con el que ya se puede acotar
  // lo demás. Es una pasada de más solo en documentos de varias páginas, y sobre
  // un fichero que ya está en memoria: no se vuelve a bajar de Drive.
  let res: Response;
  try {
    res = await dibujar(paginas === '' ? '1' : paginas);
  } catch {
    return json({ error: 'docs_unreachable' }, 502);
  }
  if (!res.ok) {
    // Un PDF con contraseña es el caso corriente, no un error del sistema: se
    // distingue para que la interfaz pueda ofrecer «ábrelo en Sytue PDF, que
    // sabe pedirla» en vez de un «no se pudo cargar» sin salida.
    if (res.status === 401 || res.status === 403) return json({ error: 'encrypted' }, 200);
    return json({ error: 'render_failed' }, res.status >= 500 ? 502 : res.status);
  }
  const primera = (await res.json()) as { paginas?: Record<string, string>; total?: number };

  // Si el cliente pidió una ventana concreta, ya está servida.
  if (paginas !== '') return json(primera, 200);

  const total = typeof primera.total === 'number' ? primera.total : 1;
  const hasta = Math.min(MAX_PAGINAS, total);
  if (hasta < 2) return json(primera, 200);

  let resto: Response;
  try {
    resto = await dibujar(`2-${hasta}`);
  } catch {
    // El resto es un extra: con la primera página ya hay vista previa útil, así
    // que un fallo aquí no la tira abajo.
    return json(primera, 200);
  }
  if (!resto.ok) return json(primera, 200);
  const siguientes = (await resto.json()) as { paginas?: Record<string, string> };
  return json(
    // Sin `?? {}`: esparcir `undefined` es legal y no añade nada (el linter lo
    // marca como fallback inútil).
    { paginas: { ...primera.paginas, ...siguientes.paginas }, total },
    200,
  );
};

/** Cuántas páginas pide una lista tipo «1,3,5-8», sin resolverla del todo. */
function cuantasPide(lista: string): number {
  let n = 0;
  for (const trozo of lista.split(',')) {
    const [a, b] = trozo.split('-').map((s) => Number.parseInt(s.trim(), 10));
    // `a === undefined` además de isFinite: con noUncheckedIndexedAccess el
    // desestructurado es `number | undefined`, e isFinite no estrecha el tipo.
    if (a === undefined || !Number.isFinite(a)) continue;
    n += Number.isFinite(b) ? Math.max(0, (b as number) - a + 1) : 1;
  }
  return n;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
