/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «FIRMAR EN EL DISPOSITIVO» con el Drive en la bóveda (ADR 0086).
 *
 * Con el Drive de la persona en la bóveda post-cuántica, el servidor ya no puede leer
 * sus PDF: CertiPDF no puede abrirlos por ruta ni firmarlos en el motor. Su PROPIA
 * firma se hace aquí: el PDF se baja cifrado y se descifra en memoria con la misma
 * puerta que Docs y Photos (`@sytue/offline/vault/drive`: la bóveda solo si existe,
 * el repositorio solo con su recibo, `importar: false`), se firma con el `.p12` de la
 * persona (`pades.ts`) y el firmado vuelve CIFRADO a la bóveda, junto al original:
 * un fichero NUEVO «nombre (firmado).pdf» sin pisar ninguno —firmar ata el fichero a
 * un momento y en el servidor también deja copia (`siempreCopia`)—. A ningún servidor
 * le llega en claro ni el PDF ni el certificado ni su contraseña.
 *
 * Pedir firma a otras personas u organizaciones NO va por aquí: el servicio tiene que
 * leer el PDF para mandarlo, y comprobar lo que vuelve contra él. Eso sigue
 * necesitando el Drive del servidor (`pdf.device.requests`).
 */
import type { PrivateDriveRepository } from '@sytue/offline/matrix';
import {
  collectPrivateDriveFile,
  consumerPrivateDriveFailure,
  freePrivateDriveName,
  privateDriveFolderEntries,
} from '@sytue/offline/vault/drive';
import { ErrorPkcs12, type Identidad } from './pkcs12';
import { DocumentoPdf, ErrorPdf, type MotivoPdf } from './pdfObjetos';
import { firmarPdfConDetalle, type FirmaVisible, type OpcionesFirmaLocal } from './pades';
import {
  ALTO_LIENZO,
  ANCHO_LIENZO,
  rectFirma,
  trazosAPagina,
  type PosicionFirma,
} from '../firmaPosicion';
import { ErrorSello } from './sello';

/** El tope de un PDF que se firma en memoria: el mismo del motor (`domain.MaxPDFBytes`). */
export const MAX_PDF_LOCAL = 100 * 1024 * 1024;
/** Cuántos PDF se enseñan, como la portada del servidor (`pdf-recientes.ts`). */
const MAXIMO = 48;
/** Cuántas páginas de recientes se recorren buscándolos. */
const PAGINAS = 10;

/** Lo que el firmado usa del repositorio de Drive de la bóveda. */
export type RepositorioFirma = Pick<
  PrivateDriveRepository,
  'recent' | 'file' | 'download' | 'children' | 'upload'
>;

/** Un PDF del Drive de la bóveda, tal y como se enseña. */
export interface PdfDeBoveda {
  id: string;
  nombre: string;
  /** La ruta dentro del Drive («/Contratos/acta.pdf»), para reconocerlo. */
  ruta: string;
  modificado: string | null;
}

const texto = (fila: Record<string, unknown>, clave: string): string => {
  const v = fila[clave];
  return typeof v === 'string' ? v : '';
};

const esPdf = (fila: Record<string, unknown>) =>
  /\.pdf$/i.test(texto(fila, 'name')) || texto(fila, 'content_type') === 'application/pdf';

/** Los PDF de la bóveda, del más reciente al más antiguo (el orden del repositorio). */
export async function pdfsDeBoveda(
  repositorio: Pick<RepositorioFirma, 'recent'>,
  signal?: AbortSignal,
): Promise<PdfDeBoveda[]> {
  const out: PdfDeBoveda[] = [];
  let cursor = '';
  for (let i = 0; i < PAGINAS && out.length < MAXIMO; i++) {
    const pagina = await repositorio.recent(cursor, signal);
    for (const { row, path } of pagina.items) {
      if (!esPdf(row) || out.length >= MAXIMO) continue;
      out.push({
        id: texto(row, 'id'),
        nombre: texto(row, 'name'),
        ruta: path,
        modificado: texto(row, 'updated_at') || null,
      });
    }
    cursor = pagina.next_cursor;
    if (!cursor) break;
  }
  return out;
}

/** «acta.pdf» → «acta (firmado).pdf». La palabra viene traducida. */
export function nombreFirmado(nombre: string, sufijo: string): string {
  const base = nombre.replace(/\.pdf$/i, '');
  return `${base} (${sufijo}).pdf`;
}

/**
 * La firma VISIBLE tal y como se elige en el diálogo: la página (desde 1), una de las
 * cuatro posiciones del estudio y, si se dibujó, la rúbrica en coordenadas del lienzo.
 */
export interface EleccionVisible {
  pagina: number;
  posicion: PosicionFirma;
  trazos?: [number, number][][] | undefined;
}

/**
 * Pasa la elección a coordenadas del PDF con el tamaño REAL de la página (el mismo
 * cálculo que el estudio, `rectFirma` y `trazosAPagina`). `ErrorPdf('pagina')` si la
 * página no existe.
 */
export async function colocarFirma(pdf: Uint8Array, e: EleccionVisible): Promise<FirmaVisible> {
  const paginas = await (await DocumentoPdf.abrir(pdf)).paginas();
  const p = paginas[e.pagina - 1];
  if (!p || !Number.isInteger(e.pagina)) throw new ErrorPdf('pagina', `página ${e.pagina}`);
  const rect = rectFirma(e.posicion, p.caja);
  const trazos = (e.trazos ?? []).filter((t) => t.length > 1);
  return {
    pagina: e.pagina,
    rect,
    trazos: trazos.length ? trazosAPagina(trazos, ANCHO_LIENZO, ALTO_LIENZO, rect) : undefined,
  };
}

export interface FirmadoEnBoveda {
  /** El nombre con el que quedó el PDF firmado. */
  nombre: string;
  id: string;
  /** La hora del sello de tiempo (B-T), o `null` si va sin él (B-B). */
  sello: Date | null;
  /** Se pidió sello y la autoridad no lo dio. */
  selloFallido: boolean;
}

/**
 * Firma en el dispositivo el PDF `id` de la bóveda y deja el firmado, cifrado, en su
 * misma carpeta con un nombre libre. Si algo falla antes de subir, no se escribe nada.
 */
export async function firmarEnBoveda(options: {
  repositorio: RepositorioFirma;
  id: string;
  identidad: Identidad;
  opciones?: OpcionesFirmaLocal;
  /** La firma visible elegida; sin ella, invisible. */
  visible?: EleccionVisible | undefined;
  /** La palabra del nombre del firmado, en el idioma de quien firma. */
  sufijo: string;
  signal?: AbortSignal | undefined;
}): Promise<FirmadoEnBoveda> {
  const { repositorio, id, signal } = options;
  const fila = await repositorio.file(id, signal);
  const carpeta = texto(fila, 'folder_id');
  if (!carpeta) throw new ErrorPdf('ilegible', 'sin carpeta');
  const original = await bajarPdf(repositorio, fila, signal);
  let firmado: Uint8Array<ArrayBuffer>;
  let sello: Date | null;
  let selloFallido: boolean;
  try {
    const visible = options.visible ? await colocarFirma(original, options.visible) : undefined;
    ({
      datos: firmado,
      sello,
      selloFallido,
    } = await firmarPdfConDetalle(original, options.identidad, {
      ...options.opciones,
      visible,
      signal,
    }));
  } finally {
    original.fill(0);
  }
  try {
    signal?.throwIfAborted();
    const { folders, files } = await privateDriveFolderEntries(repositorio, carpeta, signal);
    const nombre = freePrivateDriveName(
      nombreFirmado(texto(fila, 'name') || 'documento.pdf', options.sufijo),
      new Set([...folders, ...files].map((f) => texto(f, 'name'))),
    );
    const nueva = await repositorio.upload(
      carpeta,
      nombre,
      'application/pdf',
      new Blob([firmado], { type: 'application/pdf' }),
      signal,
    );
    return { nombre: texto(nueva, 'name') || nombre, id: texto(nueva, 'id'), sello, selloFallido };
  } finally {
    firmado.fill(0);
  }
}

/**
 * Por qué no se pudo firmar, para decirlo con su frase: el certificado (`contrasena`,
 * `certificado`), el PDF (`cifrado`, `bloqueado`, `ilegible`), la bóveda (`entrar`,
 * `pendiente`) o cualquier otra cosa (`fallo`).
 */
/** Baja y descifra en memoria el PDF de la fila `fila` (con el tope de siempre). */
export async function bajarPdf(
  repositorio: Pick<RepositorioFirma, 'download'>,
  fila: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<Uint8Array<ArrayBuffer>> {
  const blob = await collectPrivateDriveFile({
    size: fila.size_bytes,
    maximum: MAX_PDF_LOCAL,
    type: 'application/pdf',
    run: (sink) => repositorio.download(texto(fila, 'id'), sink, signal),
  });
  return new Uint8Array(await blob.arrayBuffer());
}

/** Un PDF de la bóveda abierto en memoria para el estudio: solo verlo y firmarlo. */
export interface PdfAbierto extends PdfDeBoveda {
  /** Los bytes en claro: NO salen del navegador; quien los usa los borra al terminar. */
  datos: Uint8Array<ArrayBuffer>;
}

/** «/Contratos/acta.pdf» → ['Contratos', 'acta.pdf'], sin vacíos ni «.»/«..». */
export function trozosDeRuta(ruta: string): string[] | null {
  const trozos = ruta.split('/').filter((t) => t !== '');
  if (trozos.length === 0 || trozos.some((t) => t === '.' || t === '..')) return null;
  return trozos;
}

/**
 * Busca por su RUTA (la de `/pdf?path=`) un PDF del Drive de la bóveda, bajando
 * carpeta a carpeta desde la raíz, y lo descifra en memoria. `null` si no está (o no
 * es un PDF). Nada de esto toca el servidor en claro: se leen registros cifrados.
 */
export async function abrirPdfDeBoveda(
  repositorio: Pick<RepositorioFirma, 'download' | 'children'> &
    Pick<PrivateDriveRepository, 'root'>,
  ruta: string,
  signal?: AbortSignal,
): Promise<PdfAbierto | null> {
  const trozos = trozosDeRuta(ruta);
  if (!trozos) return null;
  let carpeta = texto(await repositorio.root(signal), 'id');
  for (const nombre of trozos.slice(0, -1)) {
    const { folders } = await privateDriveFolderEntries(repositorio, carpeta, signal);
    const siguiente = folders.find((f) => texto(f, 'name') === nombre);
    if (!siguiente) return null;
    carpeta = texto(siguiente, 'id');
  }
  const { files } = await privateDriveFolderEntries(repositorio, carpeta, signal);
  const fila = files.find((f) => texto(f, 'name') === trozos[trozos.length - 1]);
  if (!fila || !esPdf(fila)) return null;
  const datos = await bajarPdf(repositorio, fila, signal);
  return {
    id: texto(fila, 'id'),
    nombre: texto(fila, 'name'),
    ruta: `/${trozos.join('/')}`,
    modificado: texto(fila, 'updated_at') || null,
    datos,
  };
}

export type MotivoFirmaLocal =
  'contrasena' | 'certificado' | MotivoPdf | 'entrar' | 'pendiente' | 'fallo';

export function motivoFirmaLocal(error: unknown): MotivoFirmaLocal {
  if (error instanceof ErrorSello) return 'fallo';
  if (error instanceof ErrorPkcs12)
    return error.motivo === 'contrasena' ? 'contrasena' : 'certificado';
  if (error instanceof ErrorPdf) return error.motivo;
  const boveda = consumerPrivateDriveFailure(error);
  return boveda === 'signin' ? 'entrar' : boveda === 'pending' ? 'pendiente' : 'fallo';
}
