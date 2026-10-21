import { serverEnv } from '@sytue/auth/server';
/**
 * Cliente REST mínimo del lado servidor para Sytue Drive. Sytue Chat NO tiene
 * almacén de ficheros propio: los adjuntos son ficheros de Sytue Drive y se
 * consumen a través de su API REST existente (services/drive) — que este módulo
 * NO modifica. El access token de la sesión (Bearer) solo vive en el servidor;
 * el navegador habla con los proxies BFF /api/upload y /api/attachment, así el
 * token nunca llega al cliente. Clean-room: fetch propio, sin dependencias.
 */
const isDev = import.meta.env.DEV === true || import.meta.env.MODE === 'development';

/** Base de la API de Sytue Drive (drive.sytue.eu; dev :8082). Solo servidor. */
export function driveBaseUrl(): string {
  const env = serverEnv(import.meta.env);
  // El Service del clúster primero: desde un pod, el nombre público no resuelve
  // y la llamada ni llega a ser HTTP (sale como 502). En desarrollo no hay
  // Service, así que ahí manda el localhost de siempre.
  const base = isDev
    ? (env.SYTUE_DRIVE_API_URL ?? 'http://localhost:8082')
    : (env.SYTUE_DRIVE_INTERNAL_URL ?? env.SYTUE_DRIVE_API_URL ?? 'https://drive.sytue.eu');
  return base.replace(/\/$/, '');
}

/** Error de una llamada a Drive, con el status HTTP para propagar 413/507 útiles. */
export class DriveError extends Error {
  constructor(
    readonly status: number,
    readonly op: string,
  ) {
    super(`drive ${op} -> ${status}`);
    this.name = 'DriveError';
  }
}

/** Fichero de Drive (subconjunto del schema File del openapi de Drive). */
export interface DriveFile {
  id: string;
  name: string;
  content_type: string;
  size_bytes: number;
}

/** Carpeta raíz del tenant del usuario (Drive la crea si no existe). Es el destino
 *  de los adjuntos de chat: reusa el almacén de Drive sin carpeta especial. */
export async function driveRootFolderId(token: string, signal?: AbortSignal): Promise<string> {
  const res = await fetch(`${driveBaseUrl()}/api/v1/folders/root`, {
    headers: { authorization: `Bearer ${token}`, accept: 'application/json' },
    ...(signal ? { signal } : {}),
  });
  if (!res.ok) throw new DriveError(res.status, 'root');
  const folder = (await res.json()) as { id: string };
  return folder.id;
}

/** Sube bytes a una carpeta de Drive; devuelve el fichero creado (id + metadatos).
 *  El nombre viaja URL-codificado en ?name= (Drive lo decodifica) y el Content-Type
 *  real en la cabecera; Drive conserva ambos para la previsualización posterior. */
/**
 * El cuerpo de una subida es el de la petición TAL CUAL, sin copiarlo al montón: un
 * adjunto puede ser de gigabytes. Un cuerpo en flujo exige `duplex: 'half'` —lo pide
 * el estándar y undici lo impone—, y esa opción todavía no está en los tipos de
 * `RequestInit`, de ahí el ensanchamiento local en vez de un `any`.
 */
export type UploadBody = ReadableStream<Uint8Array>;

function uploadInit(body: UploadBody): RequestInit {
  return { body, duplex: 'half' } as RequestInit;
}

export async function driveUpload(
  token: string,
  folderId: string,
  name: string,
  contentType: string,
  body: UploadBody,
  signal?: AbortSignal,
): Promise<DriveFile> {
  const url = `${driveBaseUrl()}/api/v1/folders/${encodeURIComponent(folderId)}/files?name=${encodeURIComponent(name)}`;
  const res = await fetch(url, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${token}`,
      'content-type': contentType || 'application/octet-stream',
      accept: 'application/json',
    },
    ...uploadInit(body),
    ...(signal ? { signal } : {}),
  });
  if (!res.ok) throw new DriveError(res.status, 'upload');
  return (await res.json()) as DriveFile;
}

/** Abre el contenido de un fichero de Drive (respuesta en streaming). El proxy
 *  /api/attachment reenvía el cuerpo al navegador con cabeceras de seguridad. */
export async function driveDownload(token: string, fileId: string): Promise<Response> {
  const res = await fetch(`${driveBaseUrl()}/api/v1/files/${encodeURIComponent(fileId)}/content`, {
    headers: { authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new DriveError(res.status, 'download');
  return res;
}

/**
 * Sube un adjunto AL CANAL: Drive lo guarda en el ámbito del canal (fuera de «Mi
 * unidad», que es un espacio compartido de la organización) y exige que el usuario
 * sea miembro del canal. Es el camino del compositor de Rooms; los ficheros que NO
 * son de un canal (la imagen de un emoji personalizado) siguen yendo a la carpeta
 * raíz con driveUpload.
 */
export async function driveChannelUpload(
  token: string,
  channelId: string,
  name: string,
  contentType: string,
  body: UploadBody,
): Promise<DriveFile> {
  const url = `${driveBaseUrl()}/api/v1/channels/${encodeURIComponent(channelId)}/files?name=${encodeURIComponent(name)}`;
  const res = await fetch(url, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${token}`,
      'content-type': contentType || 'application/octet-stream',
      accept: 'application/json',
    },
    ...uploadInit(body),
  });
  if (!res.ok) throw new DriveError(res.status, 'channel-upload');
  return (await res.json()) as DriveFile;
}

/**
 * Abre el contenido (o la MINIATURA, con thumb) de un adjunto de un canal. Drive
 * comprueba en cada petición que el usuario siga siendo miembro del canal, y sirve
 * tanto el fichero subido al canal como el que se adjuntó por referencia desde el
 * Drive del usuario. `ifNoneMatch` propaga la revalidación de la miniatura (304).
 */
export async function driveChannelDownload(
  token: string,
  channelId: string,
  fileId: string,
  opts?: { thumb?: boolean; ifNoneMatch?: string | null },
): Promise<Response> {
  const base = `${driveBaseUrl()}/api/v1/channels/${encodeURIComponent(channelId)}/files/${encodeURIComponent(fileId)}`;
  const headers: Record<string, string> = { authorization: `Bearer ${token}` };
  if (opts?.ifNoneMatch) headers['if-none-match'] = opts.ifNoneMatch;
  const res = await fetch(`${base}${opts?.thumb ? '/thumbnail' : '/content'}`, { headers });
  // 304 no es un error: es la revalidación correcta de una miniatura cacheada.
  if (!res.ok && res.status !== 304) throw new DriveError(res.status, 'download');
  return res;
}
