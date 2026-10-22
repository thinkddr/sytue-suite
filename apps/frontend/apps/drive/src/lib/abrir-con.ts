/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * QUÉ APP DE LA SUITE ABRE CADA FICHERO.
 *
 * Pulsar un fichero abría SIEMPRE el visor de Drive, aunque la suite tuviera un
 * producto entero para ese formato: un PDF se veía en un diálogo sin anotar ni
 * firmar, teniendo Sytue CertiPDF; una foto se veía plana, teniendo Sytue Fotos
 * con su editor. La previsualización se queda para lo que NO sabe abrir nadie.
 *
 * El fichero no se copia a ningún sitio: las tres apps lo abren donde está, en
 * Drive, con la ACL y el historial de versiones de Drive. Por eso esto decide un
 * DESTINO y no una importación.
 */
// `appUrl` conoce host Y prefijo de cada producto (packages/config/src/tokens.ts).
import { appUrl } from '@sytue/config';
import { carpetaDe } from './arrastrar.js';

/** El producto que abre un fichero, o null si solo cabe previsualizarlo aquí. */
export type AppQueAbre = 'pdf' | 'photos' | 'docs' | 'sheets' | 'slides' | 'editor';

/** Extensiones que abren directamente en la suite ofimática nativa (Docus, Calcus, Diapos). */
// Las plantillas (.dotx .xltx .potx .ott .ots .otp) abren su producto como documento NUEVO.
// Las de iWork (.pages .numbers .key) se convierten al abrir y el original no se toca.
const DOCS_EXTS = new Set(['odt', 'docx', 'ott', 'dotx', 'pages']);
const SHEETS_EXTS = new Set(['ods', 'xlsx', 'ots', 'xltx', 'numbers']);
const SLIDES_EXTS = new Set(['odp', 'pptx', 'otp', 'potx', 'key']);

/**
 * Producto que abre este fichero.
 *
 * `conMiniatura` y `conEditor` los decide quien llama porque son dos preguntas
 * que esta función no puede responder sola: si el servicio de fotos sabe leer
 * ese formato —lo mismo que ya decide si hay miniatura— y si el editor que
 * despliega quien opera la suite declara esa extensión. Prometer una app que
 * luego rechaza el fichero es peor que no ofrecerla: parece que ha funcionado.
 */
export function appQueAbre(
  kind: 'image' | 'pdf' | 'text' | 'office' | null,
  opciones: { conMiniatura: boolean; conEditor: boolean; ext?: string },
): AppQueAbre | null {
  if (kind === 'pdf') return 'pdf';
  if (kind === 'image') return opciones.conMiniatura ? 'photos' : null;

  const ext = opciones.ext?.toLowerCase();
  // Un `.key` de texto es una clave PEM, no una presentación: se queda en el visor.
  if (ext && !(ext === 'key' && kind === 'text')) {
    if (DOCS_EXTS.has(ext)) return 'docs';
    if (SHEETS_EXTS.has(ext)) return 'sheets';
    if (SLIDES_EXTS.has(ext)) return 'slides';
  }

  // El texto, al editor SOLO si el editor desplegado declara esa extensión: un
  // .csv en la hoja de cálculo se puede leer y ordenar, y en el visor de Drive
  // es un <pre> con comas. Un .go o un .json no los declara nadie, así que se
  // quedan donde estaban.
  //
  // A Notepads NO va: no lo abriría, haría una copia suya, y quien pulsa un
  // fichero de su carpeta no está pidiendo un duplicado.
  if (kind === 'office' || kind === 'text') return opciones.conEditor ? 'editor' : null;
  return null;
}

/**
 * URL de la app que abre el fichero, con la vuelta a Drive puesta.
 *
 * Se vuelve a la carpeta DEL FICHERO y no a la lista que se estuviera mirando:
 * desde una búsqueda, lo que se mira no es una carpeta, y devolver allí con el
 * término ya olvidado deja a la persona en una lista vacía. `carpetaDe` es la
 * misma que decide destinos al arrastrar.
 *
 * La compone `appUrl` y no una variable de entorno: ella conoce el host Y EL
 * PREFIJO de cada producto. Escrita a mano faltaba el prefijo —el menú mandaba a
 * `docs.sytue.eu/pdf` cuando la página vive en `/docs/pdf`—, y el fallo no se
 * parece a un enlace roto: el host resuelve, el ingress enruta y lo que contesta
 * es un 404 que se lee como «CertiPDF está caído».
 *
 * Con `org` (ADR 0056) la URL lleva la organización como segmento de path y
 * aterriza canónica, sin el salto que de otro modo añade el middleware de la app
 * de destino. Sin org también funciona —es la forma que dejan los llamadores que
 * no la tienen a mano—.
 */
export function urlDeApertura(
  app: AppQueAbre,
  item: { path: string; id?: string; name?: string },
  org?: string | null,
): string {
  const volver = `volver=${encodeURIComponent(carpetaDe(item.path))}`;
  if (app === 'pdf') {
    return appUrl('pdf', `pdf?path=${encodeURIComponent(item.path)}&${volver}`, org);
  }
  if (app === 'photos') {
    return appUrl('photos', `abrir?file=${encodeURIComponent(item.id ?? '')}&${volver}`, org);
  }
  if (app === 'docs' || app === 'sheets' || app === 'slides') {
    const fileParam = item.id
      ? `file=${encodeURIComponent(item.id)}`
      : `path=${encodeURIComponent(item.path)}`;
    return appUrl(app, `open-from-drive?${fileParam}&${volver}`, org);
  }
  return '';
}

/**
 * ¿Se ofrece «Subir a Cast»? Solo con un fichero de vídeo (`video/*`) con
 * identificador: es lo mismo que exige el servicio de Cast, y ofrecerlo sobre
 * algo que luego rechaza sería un menú que promete y falla.
 */
export function subibleACast(item: {
  id?: string;
  isDirectory: boolean;
  contentType?: string;
}): boolean {
  return (
    !item.isDirectory &&
    Boolean(item.id) &&
    (item.contentType ?? '').toLowerCase().startsWith('video/')
  );
}

/**
 * «Subir a Cast» (decisión del dueño, 2026-09-28). NO es abrir en el sitio, como
 * lo de arriba: es una COPIA explícita. El vídeo pasa a vivir en Sytue Cast con
 * su transcodificación y el fichero de Drive se queda donde está; la pantalla de
 * Cast lo dice antes de confirmar. Va por id (lo que Cast pide a Drive con el
 * token de la persona) y sin vuelta: tras subir, lo natural es seguir en Cast.
 *
 * Con `org` (ADR 0056), como el resto de destinos.
 */
export function urlDeSubirACast(fileId: string, org?: string | null): string {
  return appUrl('cast', `upload?drive=${encodeURIComponent(fileId)}`, org);
}
