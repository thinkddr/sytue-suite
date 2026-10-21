/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue PDF — LO QUE CAMBIA el documento: anotar, rellenar, organizar,
 * proteger, censurar y firmar.
 *
 *   POST /api/pdf-aplicar  { path, op, …, destino: 'guardar' | 'copia' }
 *
 * DÓNDE VA EL RESULTADO, que es la decisión importante:
 *
 *   - `guardar` sobrescribe el fichero de Drive. Es lo correcto para anotar y
 *     rellenar —es el MISMO documento y Drive conserva la versión anterior en su
 *     historial, así que quien se arrepienta la recupera—.
 *   - `copia` deja un fichero nuevo al lado. Es lo obligado para lo que NO tiene
 *     vuelta atrás: censurar destruye contenido, aplanar quita los campos y
 *     firmar ata el fichero a un momento. Pisar el original ahí sería tirar algo
 *     que no se puede recuperar.
 *
 * Censurar y aplanar fuerzan `copia` aunque pidan otra cosa: la interfaz lo
 * avisa, y el servidor no se fía de que lo haya hecho.
 *
 * CSRF: POST con content-type application/json + cookie same-origin.
 */
import type { APIRoute } from 'astro';
import { DriveClient, HttpError, bearer, cuerpoDeError } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/** El tope del motor (`domain.MaxPDFBytes`), que vale por fichero Y para el
 *  cuerpo entero. Ver la explicación larga en `pdf.ts`. */
const MAX_BYTES = 100 * 1024 * 1024;

/** op → endpoint del motor y el `op` que espera dentro. */
const OPERACIONES: Record<string, { ruta: string; motor?: string; siempreCopia?: boolean }> = {
  anotar: { ruta: '/pdf/annotate', motor: 'poner' },
  'borrar-anotacion': { ruta: '/pdf/annotate', motor: 'borrar' },
  'aplanar-anotaciones': { ruta: '/pdf/annotate', motor: 'aplanar', siempreCopia: true },
  // Responder NO deja copia: un hilo de revisión se sigue sobre el mismo
  // documento, y una copia por réplica haría ilegible la conversación.
  responder: { ruta: '/pdf/annotate', motor: 'responder' },
  marcar: { ruta: '/pdf/annotate', motor: 'marcar' },
  'crear-campos': { ruta: '/pdf/form', motor: 'crear', siempreCopia: true },
  'importar-datos': { ruta: '/pdf/form', motor: 'importar-datos', siempreCopia: true },
  rellenar: { ruta: '/pdf/form', motor: 'rellenar' },
  'aplanar-formulario': { ruta: '/pdf/form', motor: 'aplanar', siempreCopia: true },
  proteger: { ruta: '/pdf/protect', motor: 'proteger', siempreCopia: true },
  desproteger: { ruta: '/pdf/protect', motor: 'desproteger', siempreCopia: true },
  sanear: { ruta: '/pdf/protect', motor: 'sanear', siempreCopia: true },
  metadatos: { ruta: '/pdf/protect', motor: 'metadatos' },
  redactar: { ruta: '/pdf/protect', motor: 'redactar', siempreCopia: true },
  firmar: { ruta: '/pdf/sign', motor: 'dibujada', siempreCopia: true },
  'firmar-certificado': { ruta: '/pdf/sign', motor: 'certificado', siempreCopia: true },
  paginas: { ruta: '/pdf/pages', siempreCopia: true },
  comprimir: { ruta: '/pdf/tools', motor: 'comprimir', siempreCopia: true },
  recortar: { ruta: '/pdf/tools', motor: 'recortar', siempreCopia: true },
  archivo: { ruta: '/pdf/tools', motor: 'archivo', siempreCopia: true },
  'editar-texto': { ruta: '/pdf/edit', motor: 'texto', siempreCopia: true },
  'reemplazar-texto': { ruta: '/pdf/edit', motor: 'buscar-reemplazar', siempreCopia: true },
  'editar-imagen': { ruta: '/pdf/edit', motor: 'imagen', siempreCopia: true },
  capas: { ruta: '/pdf/edit', motor: 'capas' },
  // Reescribe el árbol de marcadores entero: deja copia porque perder el índice
  // que traía un documento ajeno no se recupera de otra forma.
  indice: { ruta: '/pdf/edit', motor: 'indice', siempreCopia: true },
  'capa-de-texto': { ruta: '/pdf/ocr', motor: 'capa', siempreCopia: true },
  // Reconocer SIEMPRE deja copia: el original escaneado es la prueba, y una capa
  // de texto mal reconocida encima del único ejemplar no se deshace.
  'reconocer-texto': { ruta: '/pdf/ocr', motor: 'reconocer', siempreCopia: true },
};

interface Cuerpo {
  path?: unknown;
  op?: unknown;
  destino?: unknown;
  clave?: unknown;
  sufijo?: unknown;
  /** Carga útil de la operación, ya en la forma que espera el motor. */
  datos?: unknown;
  /** Campos sueltos del formulario (op de páginas, contraseñas, permisos). */
  campos?: unknown;
  /** Certificado y clave privada en PEM, para firmar. */
  certificado?: unknown;
  clavePrivada?: unknown;
  /** Certificado .p12/.pfx en base64 y su contraseña. Manda sobre los PEM. */
  pkcs12?: unknown;
  clavePkcs12?: unknown;
  /** Imagen de sustitución como dataURL (op `editar-imagen`). */
  imagen?: unknown;
  /** Datos de formulario en XFDF (op `importar-datos`). */
  xfdf?: unknown;
  /** Qué buscar y por qué cambiarlo (op `reemplazar-texto`). */
  busqueda?: unknown;
  /** Coincidencias marcadas; vacío = todas (op `reemplazar-texto`). */
  elegidas?: unknown;
  /**
   * Rutas de Drive de lo que se añade DETRÁS del actual, para `combinar`.
   * Acepta PDF e imágenes; las imágenes se convierten antes (ver abajo).
   *
   * El motor acepta varios ficheros en el mismo campo `file` desde que existe
   * (`CombinarPDF(docs...)`, y rechaza con «Combining needs at least two files»
   * si le llega uno), pero este puente mandaba SIEMPRE uno solo: la operación
   * estaba implementada de punta a punta y era inalcanzable desde la aplicación.
   */
  rutas?: unknown;
}

/** Tope de ficheros por combinación. */
const MAX_COMBINAR = 20;

/** Lo que el motor sabe convertir a página (`op=imagenes-a-pdf`). */
const IMAGENES = /\.(jpe?g|png|gif|webp|bmp|tiff?)$/i;

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let cuerpo: Cuerpo;
  try {
    cuerpo = (await ctx.request.json()) as Cuerpo;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const op = texto(cuerpo.op);
  const conf = OPERACIONES[op];
  if (!conf) return json({ error: 'bad_op' }, 400);
  const path = texto(cuerpo.path);
  if (!path) return json({ error: 'missing_path' }, 400);

  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
  let original: Blob;
  try {
    original = await client.download(path);
  } catch {
    return json({ error: 'download_failed' }, 404);
  }
  if (original.size > MAX_BYTES) return json({ error: 'too_large' }, 413);

  const form = new FormData();
  form.append('file', original, nombreDe(path));
  // Lo que llevamos en memoria. Va sumando porque el tope del motor es del CUERPO
  // ENTERO, no de cada fichero: ver el bucle de la combinación.
  let total = original.size;

  // LOS DEMÁS PDF DE UNA COMBINACIÓN, en el mismo campo `file` y en orden: el
  // motor los une tal como llegan, así que el orden que el usuario ve en la
  // lista es el que manda. El actual va primero por eso mismo.
  //
  // Se descargan aquí y no en el navegador porque el token de Drive no sale al
  // cliente —igual que el original—, y se acota el número: cada uno es una
  // descarga entera en memoria, y sin tope una petición podría pedir el Drive
  // completo.
  //
  // EL TAMAÑO SE MIRA SOBRE LA MARCHA, como en `pdf-escanear.ts`. Comprobarlo
  // solo fichero a fichero dejaba pasar veinte de 100 MiB —dos gigas en la
  // memoria del frontal— para que el motor los rechazara después por pasarse del
  // cuerpo: quien tenga sesión puede tirar el proceso pidiendo una combinación
  // que nunca iba a caber.
  if (Array.isArray(cuerpo.rutas) && cuerpo.rutas.length > 0) {
    const rutas = cuerpo.rutas.map(texto).filter((p) => p !== '' && p !== path);
    if (rutas.length > MAX_COMBINAR) return json({ error: 'too_many_files' }, 400);
    for (const extra of rutas) {
      let doc: Blob;
      try {
        doc = await client.download(extra);
      } catch {
        // Se nombra CUÁL falló: con cinco ficheros, «download_failed» a secas
        // obliga a probarlos de uno en uno para saber a cuál se le ha perdido
        // el permiso o lo han movido.
        return json({ error: 'download_failed', path: extra }, 404);
      }
      total += doc.size;
      if (total > MAX_BYTES) return json({ error: 'too_large', path: extra }, 413);

      // UNA IMAGEN TAMBIÉN VALE, y es la mitad del caso real: añadir la foto de
      // un recibo al final del informe. El motor ya sabe hacerlo
      // (`op=imagenes-a-pdf`, una página por imagen) pero es otro endpoint, así
      // que se convierte aquí y lo que se combina es el PDF resultante.
      //
      // Una llamada POR IMAGEN, no una con todas: el orden que escribe el
      // usuario mezcla PDF e imágenes, y agruparlas las sacaría de su sitio.
      // Son como mucho `MAX_COMBINAR` y el motor ya las procesa de una en una.
      if (IMAGENES.test(extra)) {
        const conv = new FormData();
        conv.append('file', doc, nombreDe(extra));
        conv.append('op', 'imagenes-a-pdf');
        let hecho: Response;
        try {
          hecho = await fetch(`${docsBaseUrl()}/api/v1/pdf/tools`, {
            method: 'POST',
            headers: { authorization: `Bearer ${session.tokens.accessToken}` },
            body: conv,
          });
        } catch {
          return json({ error: 'docs_unreachable' }, 502);
        }
        if (!hecho.ok) return json({ error: 'image_failed', path: extra }, 400);
        form.append('file', await hecho.blob(), `${nombreDe(extra)}.pdf`);
        continue;
      }

      form.append('file', doc, nombreDe(extra));
    }
  }

  if (conf.motor) form.append('op', conf.motor);
  if (texto(cuerpo.clave) !== '') form.append('clave', texto(cuerpo.clave));

  // `datos` es el JSON de la operación (anotaciones, valores, áreas, firma). El
  // nombre del campo lo fija el motor, así que se deduce de la operación en vez
  // de dejar que el cliente elija: un nombre libre desde el navegador sería un
  // parámetro más que validar.
  const campoDatos = CAMPO_DATOS[op];
  if (campoDatos && cuerpo.datos !== undefined) {
    form.append(campoDatos, JSON.stringify(cuerpo.datos));
  }
  if (cuerpo.campos && typeof cuerpo.campos === 'object') {
    for (const [k, v] of Object.entries(cuerpo.campos as Record<string, unknown>)) {
      if (typeof v === 'string' || typeof v === 'number') form.append(k, String(v));
    }
  }
  // La imagen de sustitución viaja como dataURL dentro del JSON —el resto del
  // cuerpo también es JSON— y se convierte aquí en el fichero que espera el
  // motor. Sin `imagen`, la operación QUITA la que hubiera, que es legítima: es
  // el mismo botón visto desde el otro lado.
  if (op === 'editar-imagen' && typeof cuerpo.imagen === 'string' && cuerpo.imagen !== '') {
    const coma = cuerpo.imagen.indexOf(',');
    const cabecera = coma > 0 ? cuerpo.imagen.slice(0, coma) : '';
    if (!cabecera.startsWith('data:image/') || !cabecera.includes(';base64')) {
      return json({ error: 'bad_image' }, 400);
    }
    let binario: Uint8Array;
    try {
      binario = Uint8Array.from(atob(cuerpo.imagen.slice(coma + 1)), (c) => c.charCodeAt(0));
    } catch {
      return json({ error: 'bad_image' }, 400);
    }
    const tipo = cabecera.slice('data:'.length, cabecera.indexOf(';'));
    form.append('imagen', new Blob([binario.buffer as ArrayBuffer], { type: tipo }), 'imagen');
  }
  // Los datos de formulario llegan como texto XFDF y se mandan como fichero: es
  // lo que espera el motor, y así el BFF sigue hablando JSON con la app.
  if (op === 'reemplazar-texto') {
    form.append('aplicar', '1');
    form.append('busqueda', JSON.stringify(cuerpo.busqueda ?? {}));
    if (Array.isArray(cuerpo.elegidas)) {
      form.append('elegidas', JSON.stringify(cuerpo.elegidas));
    }
  }
  if (op === 'importar-datos') {
    const xfdf = texto(cuerpo.xfdf);
    if (xfdf === '') return json({ error: 'missing_data' }, 400);
    form.append('datos', new Blob([xfdf]), 'datos.xfdf');
  }
  if (op === 'firmar-certificado') {
    // Un `.p12`/`.pfx` trae certificado y clave dentro y es como lo entregan la
    // FNMT y las autoridades de certificación, así que va primero: quien tiene
    // uno no debería tener que partirlo en dos PEM para poder firmar.
    const p12 = texto(cuerpo.pkcs12);
    if (p12) {
      form.append('pkcs12', new Blob([base64ABytes(p12)]), 'certificado.p12');
      form.append('clave_p12', texto(cuerpo.clavePkcs12));
    } else {
      const cert = texto(cuerpo.certificado);
      const clave = texto(cuerpo.clavePrivada);
      if (!cert || !clave) return json({ error: 'missing_certificate' }, 400);
      form.append('certificado', new Blob([cert]), 'certificado.pem');
      form.append('clave_priv', new Blob([clave]), 'clave.pem');
    }
  }
  if (op === 'firmar') {
    // La rúbrica escaneada. Viaja en base64 porque este cuerpo es JSON; el
    // servicio la recibe ya como fichero del formulario.
    const imagen = texto(cuerpo.imagen);
    if (imagen) form.append('imagen', new Blob([base64ABytes(imagen)]), 'firma.png');
  }

  let res: Response;
  try {
    res = await fetch(`${docsBaseUrl()}/api/v1${conf.ruta}`, {
      method: 'POST',
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
      body: form,
    });
  } catch {
    return json({ error: 'docs_unreachable' }, 502);
  }
  if (!res.ok) {
    const detalle = cuerpoDeError(await res.text().catch(() => ''));
    return json(
      { error: 'pdf_failed', code: detalle.code, message: detalle.message },
      res.status >= 500 ? 502 : res.status,
    );
  }
  const salida = await res.blob();

  // Comprimir devuelve su resumen en cabeceras: sin él la interfaz no puede
  // decir cuánto se ahorró, que es lo único que el usuario quería saber.
  const ahorro = res.headers.get('x-sytue-ahorro');
  const bytesDespues = res.headers.get('x-sytue-bytes-despues');

  // Reconocer devuelve el suyo por la misma vía: cuántas páginas se leyeron y
  // cuántas se saltaron por no tener imagen que reconocer. Sin esto, un
  // expediente de veinte páginas del que solo se reconocen doce se anuncia igual
  // que uno completo, y nadie revisa las ocho que faltan.
  const ocrPaginas = res.headers.get('x-sytue-ocr-paginas');
  const ocrSinImagen = res.headers.get('x-sytue-ocr-sin-imagen');
  // Cuántos campos entraron al importar: sin esto, un fichero de OTRO impreso
  // —que no casa con ningún campo— se anuncia igual que uno que sí encajó.
  const camposImportados = res.headers.get('x-sytue-campos-importados');
  // El documento lleva XMP y el cambio de propiedades puede no verse: un lector
  // moderno prefiere ese bloque, que no se toca.
  const metadatosXmp = res.headers.get('x-sytue-metadatos-xmp');
  const reemplazos = res.headers.get('x-sytue-reemplazos');
  // Qué pasó con la marca PDF/A: «perdido» si el cambio no cumplía la norma de
  // archivo y se quitó, «rebajado» si bajó de nivel «a» a «b» al censurar.
  const pdfa = res.headers.get('x-sytue-pdfa');

  const enCopia = conf.siempreCopia || texto(cuerpo.destino) === 'copia';
  try {
    const destino = enCopia ? await destinoLibre(client, path, op, cuerpo.sufijo) : path;
    const recurso = await client.upload(destino, salida, 'application/pdf');
    return json(
      {
        resource: recurso,
        path: destino,
        copia: enCopia,
        ...(ahorro !== null ? { ahorro: Number(ahorro), bytes: Number(bytesDespues) } : {}),
        ...(camposImportados !== null ? { camposImportados: Number(camposImportados) } : {}),
        ...(metadatosXmp !== null ? { metadatosXmp: true } : {}),
        ...(reemplazos !== null ? { reemplazos: Number(reemplazos) } : {}),
        ...(pdfa === 'perdido' || pdfa === 'rebajado' ? { pdfa } : {}),
        ...(ocrPaginas !== null
          ? { ocrPaginas: Number(ocrPaginas), ocrSinImagen: Number(ocrSinImagen ?? 0) }
          : {}),
      },
      enCopia ? 201 : 200,
    );
  } catch {
    return json({ error: 'upload_failed' }, 502);
  }
};

/** Nombre del campo multipart que lleva el JSON de cada operación. */
const CAMPO_DATOS: Record<string, string> = {
  anotar: 'anotaciones',
  'crear-campos': 'campos',
  metadatos: 'metadatos',
  rellenar: 'valores',
  redactar: 'areas',
  firmar: 'firma',
  'firmar-certificado': 'firma',
  'editar-texto': 'cambios',
  'capa-de-texto': 'textos',
  indice: 'marcadores',
};

/**
 * Ruta del resultado: junto al original, con una palabra que dice qué se hizo y
 * —si ya existe— un número, como cualquier gestor de archivos.
 *
 * Solo un 404 confirma que un nombre está libre. Cualquier otro fallo —red,
 * permiso— NO lo confirma, y darlo por libre sobreescribiría un fichero ajeno,
 * así que ahí se cae al sello de tiempo, que no choca con nada.
 */
async function destinoLibre(
  client: DriveClient,
  path: string,
  op: string,
  sufijo: unknown,
): Promise<string> {
  const partes = path.split('/').filter(Boolean);
  const nombre = partes.pop() ?? 'documento.pdf';
  const base = nombre.replace(/\.pdf$/i, '');
  const etiqueta = limpiarSufijo(sufijo) || op;
  const ruta = (cola: string): string =>
    '/' + [...partes, `${base} (${etiqueta}${cola}).pdf`].join('/');

  for (let n = 1; n <= 10; n++) {
    const candidato = ruta(n === 1 ? '' : ` ${n}`);
    try {
      await client.stat(candidato);
    } catch (e) {
      if (e instanceof HttpError && e.status === 404) return candidato;
      break;
    }
  }
  const sello = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-');
  return ruta(` ${sello}`);
}

/**
 * La palabra del nombre la escribe el CLIENTE (va traducida a su idioma), así
 * que se acota a letras, números, espacios y guiones: ni «/», ni «..», ni saltos
 * de línea pueden llegar a formar parte de una ruta.
 */
function limpiarSufijo(v: unknown): string {
  if (typeof v !== 'string') return '';
  return v
    .normalize('NFC')
    .replace(/[^\p{L}\p{N} -]/gu, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 24)
    .trim();
}

/**
 * base64 → bytes. Un `.p12` y un PNG son binarios y este cuerpo es JSON, así que
 * el navegador los manda codificados; aquí vuelven a ser lo que eran antes de
 * entrar en el formulario que va al servicio.
 */
function base64ABytes(b64: string): ArrayBuffer {
  const limpio = b64.includes(',') ? b64.slice(b64.indexOf(',') + 1) : b64;
  const bin = atob(limpio);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
}

function texto(v: unknown): string {
  return typeof v === 'string' ? v.trim() : '';
}

function nombreDe(path: string): string {
  const partes = path.split('/').filter(Boolean);
  return partes[partes.length - 1] ?? 'documento.pdf';
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
