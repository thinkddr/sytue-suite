/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «GUARDAR EN DRIVE» CON EL DRIVE EN LA BÓVEDA (ADR 0086).
 *
 * Con el Drive de la persona en la bóveda post-cuántica, el Drive del servidor está
 * congelado: ni el servicio de Docs ni el BFF pueden dejar ahí el fichero de un
 * documento (Drive contesta 409 y el BFF lo traduce a `drive_private_vault`). Solo
 * el navegador puede: exporta el documento (el contenido ya está en Docs, que no es
 * de extremo a extremo), lo CIFRA aquí y lo sube al Drive de la bóveda con el mismo
 * repositorio y la misma puerta que Drive web y Photos (`@sytue/offline/vault/drive`).
 * A ningún servidor le llega el fichero en claro; al de Docs, solo el ancla.
 *
 * Dónde va:
 *
 * - Si el documento ya está anclado («drive:<id>» de antes de pasar a la bóveda, que
 *   conserva su id, o «vault:<id>») y ese fichero sigue vivo en la bóveda, a ESE
 *   fichero, con su nombre y en su carpeta: una versión nueva, como hace el BFF con
 *   el Drive del servidor. El formato es el de su extensión; si no tiene ida y
 *   vuelta (.pdf, .txt), no se guarda (`formato`), igual que en el servidor.
 * - Si no (sin ancla, o su fichero ya no está), a un fichero NUEVO en «Sytue Docs»
 *   de la raíz —la carpeta del servidor (`drive.CarpetaDocumentos`)—, en ODF —el
 *   formato con que el servidor crea el hogar—, sin pisar ningún nombre. El dueño
 *   ancla entonces el documento a él («vault:<id>») si podía (`sePuedeAnclarEnBoveda`).
 *
 * Lo que NO se hace, y por qué: escribir el fichero «hogar» al crear el documento.
 * Exigiría abrir la bóveda (y quizá pedir volver a entrar) en cada creación, desde
 * todas las que hay (portada, plantillas, copias, Drive), y aun así el fichero no
 * seguiría al documento: tampoco lo hace el del servidor, que solo se reescribe al
 * guardar. El ancla se crea en el primer «Guardar en Drive», que es explícito.
 */
import { DriveClient, type DocKind } from '@sytue/api-clients';
import type { MatrixStorageScope } from '@sytue/config/matrix';
import { PrivateDriveError, type PrivateDriveRepository } from '@sytue/offline/matrix';
import type { PqVault, PqVaultOpenOptions } from '@sytue/offline/vault';
import {
  consumerPrivateDriveFailure,
  freePrivateDriveName,
  openConsumerPrivateDrive,
  privateDriveFolderEntries,
  privateDriveRootFolder,
  type ConsumerPrivateDrive,
  type PrivateDriveSource,
} from '@sytue/offline/vault/drive';
import { withBase } from '@sytue/ui';
import type { ExportFormat } from '../types';
import { exportFilename } from '../exportName';
import { anclaDeBoveda, leerAncla, sePuedeAnclarEnBoveda } from './anclaDrive';

/** El prefijo de las rutas del BFF de la ofimática para la bóveda de Drive. */
export const RUTA_DRIVE_CIFRADO = '/api/drive-cifrado';
/** La carpeta de los documentos: la misma que en el Drive del servidor. */
export const CARPETA_DOCUMENTOS = 'Sytue Docs';

/**
 * Extensión del fichero → familia de exportación de Docs (la misma tabla que
 * `pages/api/save-to-drive.ts`). Solo los formatos con ida y vuelta.
 */
export const FORMATO_POR_EXTENSION: Readonly<Record<string, ExportFormat>> = {
  docx: 'ooxml',
  xlsx: 'ooxml',
  pptx: 'ooxml',
  odt: 'odf',
  ods: 'odf',
  odp: 'odf',
  md: 'markdown',
  markdown: 'markdown',
  html: 'html',
  htm: 'html',
};

/**
 * Por qué no se pudo guardar en el Drive de la bóveda: `entrar`, volver a entrar
 * para desbloquearlo; `pendiente`, Drive aún no ha terminado de pasar a la bóveda;
 * `formato`, el fichero anclado no tiene ida y vuelta; `fallo`, cualquier otra cosa.
 */
export type MotivoDriveCifrado = 'entrar' | 'pendiente' | 'formato' | 'fallo';

/** El fichero anclado no se puede reescribir con su formato. */
export class FormatoSinVuelta extends Error {
  constructor() {
    super('unsupported_format');
    this.name = 'FormatoSinVuelta';
  }
}

export function motivoDriveCifrado(error: unknown): MotivoDriveCifrado {
  if (error instanceof FormatoSinVuelta) return 'formato';
  const motivo = consumerPrivateDriveFailure(error);
  return motivo === 'signin' ? 'entrar' : motivo === 'pending' ? 'pendiente' : 'fallo';
}

/** Lo que guardar usa del repositorio de Drive de la bóveda. */
export type RepositorioDocs = Pick<
  PrivateDriveRepository,
  'root' | 'children' | 'createFolder' | 'upload' | 'file'
>;

const texto = (fila: Record<string, unknown>, clave: string): string => {
  const valor = fila[clave];
  return typeof valor === 'string' ? valor : '';
};

/** El fichero anclado, vivo en la bóveda, o `null` si no está (o está en la papelera). */
async function ficheroAnclado(
  repositorio: RepositorioDocs,
  ancla: string,
  signal?: AbortSignal,
): Promise<Record<string, unknown> | null> {
  const a = leerAncla(ancla);
  if (!a) return null;
  try {
    return await repositorio.file(a.id, signal);
  } catch (error) {
    if (error instanceof PrivateDriveError && error.reason === 'not_found') return null;
    throw error;
  }
}

export interface GuardadoCifrado {
  /** El nombre con el que quedó el fichero. */
  nombre: string;
  /** El ancla del fichero escrito («drive:» o «vault:» si era el anclado). */
  ancla: string;
  /** ¿Es un fichero nuevo (y no una versión nueva del anclado)? */
  nuevo: boolean;
}

/**
 * Escribe el documento en el Drive de la bóveda (ver arriba dónde). `exportar`
 * devuelve sus bytes en el formato pedido; se llama una sola vez.
 */
export async function guardarEnDriveCifrado(options: {
  repositorio: RepositorioDocs;
  ancla: string;
  titulo: string;
  kind: DocKind;
  exportar: (formato: ExportFormat) => Promise<Blob>;
  signal?: AbortSignal | undefined;
}): Promise<GuardadoCifrado> {
  const { repositorio, signal } = options;
  const anclado = await ficheroAnclado(repositorio, options.ancla, signal);
  if (anclado) {
    const nombre = texto(anclado, 'name');
    const carpeta = texto(anclado, 'folder_id');
    const formato = FORMATO_POR_EXTENSION[nombre.slice(nombre.lastIndexOf('.') + 1).toLowerCase()];
    if (!formato || !carpeta) throw new FormatoSinVuelta();
    const datos = await options.exportar(formato);
    // Mismo nombre en la misma carpeta: el repositorio lo sube como versión nueva
    // de ese fichero (la anterior queda en su historial), no como otro fichero.
    const fila = await repositorio.upload(
      carpeta,
      nombre,
      datos.type || texto(anclado, 'content_type') || 'application/octet-stream',
      datos,
      signal,
    );
    return { nombre: texto(fila, 'name') || nombre, ancla: options.ancla, nuevo: false };
  }
  const carpeta = await privateDriveRootFolder(repositorio, CARPETA_DOCUMENTOS, true, signal);
  if (!carpeta) throw new PrivateDriveError('not_found');
  const { folders, files } = await privateDriveFolderEntries(repositorio, carpeta, signal);
  const nombre = freePrivateDriveName(
    exportFilename(options.titulo, options.kind, 'odf'),
    new Set([...folders, ...files].map((f) => texto(f, 'name'))),
  );
  const datos = await options.exportar('odf');
  const fila = await repositorio.upload(
    carpeta,
    nombre,
    datos.type || 'application/octet-stream',
    datos,
    signal,
  );
  return {
    nombre: texto(fila, 'name') || nombre,
    ancla: anclaDeBoveda(texto(fila, 'id')),
    nuevo: true,
  };
}

/** El Drive de la bóveda abierto para guardar. `cerrar` lo suelta todo. */
export interface DriveCifradoAbierto {
  readonly repositorio: RepositorioDocs;
  readonly signal: AbortSignal;
  cerrar: () => Promise<void>;
}

export interface ResultadoGuardadoCifrado extends GuardadoCifrado {
  /** El ancla que el documento tiene ahora en el servicio (la nueva si se ancló). */
  anclaDelDocumento: string;
}

/**
 * Todo el «Guardar en Drive» cifrado: abre el Drive de la bóveda, escribe y lo
 * suelta; si el fichero es nuevo y quien guarda es el dueño, ancla el documento a
 * él. Que el ancla no se pueda poner (otra pestaña ancló antes, sin red) no deshace
 * lo guardado: el fichero ya está en la bóveda y el siguiente guardado lo intenta.
 */
export async function guardarDocumentoEnBoveda(options: {
  ancla: string;
  esDueno: boolean;
  titulo: string;
  kind: DocKind;
  abrir: () => Promise<DriveCifradoAbierto>;
  exportar: (formato: ExportFormat) => Promise<Blob>;
  /** Cambia el ancla en el servicio si sigue siendo `antes`; `true` si quedó puesta. */
  anclar: (nueva: string, antes: string) => Promise<boolean>;
}): Promise<ResultadoGuardadoCifrado> {
  const drive = await options.abrir();
  let guardado: GuardadoCifrado;
  try {
    guardado = await guardarEnDriveCifrado({
      repositorio: drive.repositorio,
      ancla: options.ancla,
      titulo: options.titulo,
      kind: options.kind,
      exportar: options.exportar,
      signal: drive.signal,
    });
  } finally {
    await drive.cerrar().catch(() => {});
  }
  let anclaDelDocumento = options.ancla;
  if (
    guardado.nuevo &&
    options.esDueno &&
    guardado.ancla !== options.ancla &&
    sePuedeAnclarEnBoveda(options.ancla)
  ) {
    const puesta = await options.anclar(guardado.ancla, options.ancla).catch(() => false);
    if (puesta) anclaDelDocumento = guardado.ancla;
  }
  return { ...guardado, anclaDelDocumento };
}

/* --- Lo del navegador: el BFF de la ofimática y la apertura de la bóveda --- */

const baseDelBff = (): string => withBase(import.meta.env.BASE_URL ?? '/', RUTA_DRIVE_CIFRADO);

function fuenteDelNavegador(): PrivateDriveSource {
  return new DriveClient({ baseUrl: window.location.origin + baseDelBff() });
}

/**
 * ¿El Drive del servidor de esta persona ya no admite escrituras (está, o está
 * pasando, a la bóveda)? Solo lee el estado de la fuente, sin claves. Ante la duda
 * (sin organización, error de red) contesta que no: el editor sigue como siempre.
 */
export async function driveEnBoveda(
  actor: string,
  tenant: string | undefined,
  fuente: () => Pick<PrivateDriveSource, 'privateStorageState'> = fuenteDelNavegador,
  signal?: AbortSignal,
): Promise<boolean> {
  if (!tenant || !actor) return false;
  const scope: MatrixStorageScope = { tenant, subject: actor, product: 'drive' };
  try {
    return (await fuente().privateStorageState(scope, signal)).mode !== 'legacy';
  } catch {
    return false;
  }
}

/** Abre el Drive de la bóveda (solo si existe y tiene el recibo de la migración). */
export async function abrirDriveCifrado(options: {
  actor: string;
  tenant?: string | undefined;
  signal?: AbortSignal | undefined;
  /** Sustituibles en pruebas. */
  abrir?: (options: PqVaultOpenOptions) => Promise<PqVault>;
  fuente?: () => PrivateDriveSource;
  base?: string;
}): Promise<DriveCifradoAbierto> {
  const drive = await openConsumerPrivateDrive({
    actor: options.actor,
    tenant: options.tenant,
    basePath: options.base ?? baseDelBff(),
    source: options.fuente ?? fuenteDelNavegador,
    signal: options.signal,
    open: options.abrir,
  });
  return { repositorio: drive.repository, signal: drive.signal, cerrar: drive.close };
}

/**
 * Lo mismo que `abrirDriveCifrado`, con el repositorio ENTERO: CertiPDF lista los PDF
 * de la bóveda y los baja para firmarlos en el dispositivo (`pdf/dispositivo`).
 */
export function abrirRepositorioCifrado(options: {
  actor: string;
  tenant?: string | undefined;
  signal?: AbortSignal | undefined;
}): Promise<ConsumerPrivateDrive> {
  return openConsumerPrivateDrive({
    actor: options.actor,
    tenant: options.tenant,
    basePath: baseDelBff(),
    source: fuenteDelNavegador,
    signal: options.signal,
  });
}

/** Los bytes del documento en `formato`, por el mismo BFF que la descarga. */
export async function exportarDocumento(id: string, formato: ExportFormat): Promise<Blob> {
  const res = await fetch(`/api/export?id=${encodeURIComponent(id)}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ format: formato }),
  });
  if (!res.ok) throw new Error(`export ${res.status}`);
  return res.blob();
}

/** Pone el ancla de la bóveda en el servicio si sigue siendo `antes`. */
export async function anclarEnServicio(id: string, nueva: string, antes: string): Promise<boolean> {
  const res = await fetch('/api/vault-anchor', {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ id, source_ref: nueva, expect: antes }),
  });
  return res.ok;
}
