/**
 * Sytue Docs — /open-from-drive?file=<uuid>
 *
 * LO ENLAZAN DRIVE Y LAS APPS NATIVAS: pulsar un .odt/.docx, un .ods/.xlsx o un
 * .odp/.pptx en Drive viene aquí (`urlDeApertura` en drive/src/lib/abrir-con.ts), y el
 * Drive nativo cae aquí mismo cuando no hay app nativa que atienda el esquema
 * (`RutaDeApertura` en apps/nucleo/api/abrir.go).
 *
 * ABRIR UN FICHERO DE SYTUE DRIVE EN EL EDITOR. Era el destino de «Abrir con Sytue
 * Docs» del menú de un fichero en Drive y hace de una vez lo que la lista de
 * documentos hace en tres pasos al importar: leer el fichero, convertirlo, crear el
 * documento y sembrar el contenido. Termina redirigiendo a /edit?id=<id>.
 *
 * POR QUÉ AQUÍ Y NO EN DRIVE: quien sabe convertir un .docx y crear un documento es
 * Sytue Docs. Drive solo necesita saber a qué URL mandar a la persona, así que la
 * acción es un enlace normal y no una conversación entre dos BFF.
 *
 * POR QUÉ SOLO LLEGA EL ID: el `path` se resuelve aquí contra Drive (statById). Un
 * path en la URL sería una ruta elegida por quien pulsa el enlace y habría que
 * volver a comprobar de quién es; el id se resuelve con el token de la sesión, así
 * que si esa persona no puede ver el fichero, Drive contesta 404 y aquí no hay nada
 * que abrir.
 *
 * ABRIR DOS VECES NO DUPLICA: el documento queda anclado al fichero
 * (source_ref = «drive:<uuid>») y volver a pulsar lleva al MISMO documento. Sin eso,
 * cada apertura dejaría otra copia y las ediciones se repartirían entre todas.
 *
 * LO QUE ESTO NO HACE TODAVÍA: devolver los cambios al fichero de Drive. El
 * documento nace del fichero y luego vive por su cuenta; el fichero se queda como
 * estaba. Guardar de vuelta es la ola siguiente, y el vínculo se anota desde ya
 * porque es lo que la hará posible.
 */
import { DocsClient, DriveClient, bearer, type DocKind } from '@sytue/api-clients';
import type { APIRoute } from 'astro';
import { returnToCanonico } from '@sytue/config';
import { vieneDeOtroSitio } from '@sytue/auth/server';
import { bff, toBffContext, docsBaseUrl, driveBaseUrl } from '../lib/auth';
import { r } from '../lib/rutas.js';
import {
  guardarComentarios,
  rutaImportarComentarios,
  type Convertido,
} from '../lib/comentarios-importados';

export const prerender = false;

/** Clave del registro LWW donde vive el contenido, por tipo de documento
 *  (editor/types.ts: el mismo contrato que usa la importación de la lista). */
const CONTENT_KEY: Record<DocKind, string> = {
  document: 'doc:html',
  spreadsheet: 'doc:sheet',
  presentation: 'doc:slides',
};

/** Vuelta a la lista contando por qué no se pudo abrir (la lista lo traduce). */
const backToList = (reason: string): string => r(`/?error=${encodeURIComponent(reason)}`);

export const GET: APIRoute = async (ctx) => {
  const url = new URL(ctx.request.url);

  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    // La petición puede llegar REESCRITA por dentro (entrada org-ful con la marca
    // `?__org=`, ADR 0056): el returnTo sale en la forma canónica, no en la
    // interna, o tras el login la barra se quedaría con la marca.
    const returnTo = returnToCanonico(url, bff.basePath);
    return ctx.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`);
  }

  // Abrir CREA un documento (FE-02): desde otra web no, aunque la cookie viaje.
  if (vieneDeOtroSitio(ctx.request)) return ctx.redirect(backToList('open_failed'));

  let fileId = url.searchParams.get('file');
  const pathParam = url.searchParams.get('path');
  const volverParam = url.searchParams.get('volver');
  const volverQuery = volverParam ? `&volver=${encodeURIComponent(volverParam)}` : '';

  if (!fileId && !pathParam) return ctx.redirect(backToList('missing_file'));

  const token = session.tokens.accessToken;
  const auth = bearer(token);
  // La org para los destinos (ADR 0056): `edit` es ruta en `sinOrg`, así que un
  // `/docs/edit?id=…` sin org NO recibe el salto del middleware y el editor
  // hidrata creyendo que `edit` es la organización. La del path si vino; si no,
  // la de la sesión.
  const org = ctx.locals.org ?? session.user.tenant;
  const drive = new DriveClient({ baseUrl: driveBaseUrl(), auth });

  let meta;
  try {
    if (fileId) {
      meta = await drive.statById(fileId);
    } else {
      meta = await drive.stat(pathParam!);
      fileId = meta.id ?? null;
    }
  } catch {
    return ctx.redirect(backToList('missing_file'));
  }

  if (!fileId || !meta) return ctx.redirect(backToList('missing_file'));
  if (meta.isDirectory) return ctx.redirect(backToList('not_a_file'));

  const docs = new DocsClient({ baseUrl: docsBaseUrl(), auth });
  const sourceRef = `drive:${fileId}`;

  try {
    // ¿Ya lo abrió antes? Entonces no hay que descargar ni convertir nada. Salvo que ese
    // documento no tenga NI UNA operación (`version` 0): es el hueco que deja una
    // apertura cuyo sembrado falló —lo convertido no cabía en una operación, antes de que
    // el servicio encogiera las imágenes al importar—, y abrirlo enseñaría una
    // presentación vacía cada vez. Ese se siembra ahora.
    // UNA PLANTILLA NO SE ANCLA (.dotx .xltx .potx .ott .ots .otp): abrirla es crear un
    // documento NUEVO a partir de ella, cada vez, y no hay «el documento de la plantilla»
    // al que volver. Se mira por la extensión; si la plantilla viene renombrada, lo
    // dice la conversión (`template`) y entonces se crea sin ancla.
    const esPlantilla = /\.(dotx|dotm|xltx|xltm|potx|potm|ott|ots|otp)$/i.test(meta.name);
    const abierto = esPlantilla ? null : await docs.bySource(sourceRef);
    if (abierto && abierto.version > 0) {
      return ctx.redirect(r(`/edit?id=${encodeURIComponent(abierto.id)}${volverQuery}`, org));
    }

    // Fichero → { kind, title, content }. El servicio acota tamaño y formato, y
    // reconoce el formato por la firma del contenido, no por la extensión.
    const bytes = await (await drive.openDownload(meta.path)).arrayBuffer();
    const form = new FormData();
    form.append('file', new Blob([bytes]), meta.name);
    // `comments=1`: los comentarios del fichero se guardan aquí mismo al crear el documento.
    // `locale`: el idioma de los avisos que el servicio deja dentro de lo convertido.
    const conv = await fetch(
      `${docsBaseUrl()}/api/v1/import?comments=1&locale=${encodeURIComponent(ctx.locals.locale)}`,
      {
        method: 'POST',
        headers: { authorization: `Bearer ${token}` },
        body: form,
      },
    );
    if (!conv.ok) {
      return ctx.redirect(backToList(conv.status === 413 ? 'too_large' : 'unsupported_format'));
    }
    const converted = (await conv.json()) as Convertido & {
      template?: boolean;
      truncated?: boolean;
    };
    // UN ARCHIVO RECORTADO TAMPOCO SE ANCLA. Si no cabe entero en un documento (.xlsx, .ods,
    // .docx o .pptx), el servicio lo abre con la parte inicial que cabe y un aviso delante
    // (`truncated`). Esa copia es un documento aparte: anclada, «Guardar en Drive»
    // sobrescribiría el fichero entero con la copia recortada.
    const nueva = esPlantilla || converted.template === true || converted.truncated === true;
    // El título de un documento nacido de una plantilla no lleva la extensión.
    const titulo = nueva ? meta.name.replace(/\.[^.]+$/, '') : meta.name;

    // El título es el del fichero, no el que adivine la conversión: es el nombre por
    // el que la persona lo conoce y el que verá al volver a Drive.
    const doc =
      abierto && !nueva
        ? { ...abierto, created: true }
        : await docs.create({
            kind: converted.kind,
            title: titulo,
            ...(nueva ? {} : { sourceRef }),
          });

    // Sembrar SOLO si el documento acaba de nacer (o es el hueco de arriba). Dos
    // pestañas abriendo el mismo fichero a la vez llegan aquí las dos; la segunda
    // recibe `created: false` y sembrar entonces pisaría el contenido con una segunda
    // op `set` idéntica.
    // Los comentarios se guardan ANTES de sembrar: el contenido lleva el ancla de los que
    // quedaron guardados y de ningún otro (un mark sin comentario no se puede quitar).
    let content = '';
    if (doc.created) {
      ({ content } = await guardarComentarios(converted, async (comments) => {
        const res = await fetch(`${docsBaseUrl()}/api/v1${rutaImportarComentarios(doc.id)}`, {
          method: 'POST',
          headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
          body: JSON.stringify({ comments }),
        });
        if (!res.ok) throw new Error('comments');
        return ((await res.json()) as { anchored?: string[] }).anchored ?? [];
      }));
    }
    if (doc.created && content) {
      const seed = await fetch(
        `${docsBaseUrl()}/api/v1/documents/${encodeURIComponent(doc.id)}/ops`,
        {
          method: 'POST',
          headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
          body: JSON.stringify({
            ops: [
              {
                kind: 'set',
                dot: { a: crypto.randomUUID(), c: 1 },
                key: CONTENT_KEY[converted.kind],
                value: content,
                lamport: 1,
              },
            ],
            // La contraseña de protección que traía el fichero (solo su hash).
            ...(converted.hashes ? { proteccion: { hashes: converted.hashes } } : {}),
          }),
        },
      );
      // Un documento vacío es peor que un error honesto: la persona creería que el
      // fichero no tenía nada dentro y empezaría a escribir sobre él.
      if (!seed.ok) return ctx.redirect(backToList('open_failed'));
    }
    return ctx.redirect(r(`/edit?id=${encodeURIComponent(doc.id)}${volverQuery}`, org));
  } catch {
    return ctx.redirect(backToList('open_failed'));
  }
};
