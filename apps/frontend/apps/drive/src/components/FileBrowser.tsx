/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * FileBrowser — explorador de archivos de Sytue Drive. Habla SOLO con el BFF de
 * esta app (endpoints /api/files, /api/folders, /api/content, /api/shares) por
 * cookie de sesion httpOnly; el BFF consulta el servicio Go propio (drive.sytue.eu,
 * REST + WebDAV) via @sytue/api-clients → DriveClient. El token NUNCA llega al
 * cliente. Navegacion por carpetas con breadcrumb, vista lista/rejilla, subir
 * archivos, crear carpeta, descargar, vista previa inline (imagen/PDF/texto),
 * eliminar y enlaces de compartir publicos (crear/listar/revocar, con permiso
 * y caducidad). Implementacion clean-room: SIN motores de terceros. Diseno
 * Sytue, claro/oscuro.
 *
 * Es una isla: SOLO habla con el BFF de esta app (endpoints /api/*, cookie de
 * sesion httpOnly). El access token de Sytue ID NUNCA llega al cliente; el BFF
 * lo usa server-side contra el servicio Go de Sytue Drive.
 *
 * Idioma: lo publica <SytueShell> en el contexto de i18n a partir del locale que
 * el layout negocia en el SERVIDOR (`src/middleware.ts`). Aqui no se detecta nada
 * —eso desajustaria la hidratacion— y los tamaños y fechas se formatean con ese
 * locale (docs/CONVENTIONS.md §13.1).
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  Icon,
  SectionHeader,
  EmptyState,
  FieldHint,
  InlineHelp,
  Modal,
  Input,
  FieldGroup,
  Select,
  Dropdown,
  MenuItem,
  MenuSeparator,
  Skeleton,
  Spinner,
  VirtualList,
  usePrompt,
  useToast,
  useAhora,
  cx,
  cardTone,
  CardIcon,
  CARD_TONES,
  useTheme,
  BulkActionBar,
  withBase,
  Illustration,
  esAtajoDeApp,
  Checkbox,
  Radio,
} from '@sytue/ui';
import type { DriveResource, ShareLink as ShareLinkSdk, TrashItem } from '@sytue/api-clients';

/**
 * Una compartición tal y como la sirve el BFF: la del SDK más el correo (y el
 * nombre) de la persona destinataria, que el BFF resuelve contra Sytue ID. El
 * servicio solo guarda el identificador, y un identificador no le dice nada a
 * quien mira la lista para decidir si eso sigue teniendo sentido.
 */
type ShareLink = ShareLinkSdk & { email?: string; name?: string };
import { FranjaPeticion, openInApp, inNativeShell } from '@sytue/shell';
import { appUrl, driveFileUrl, driveFolderUrl, orgActual } from '@sytue/config';
import { useConnectivity } from '@sytue/offline/preact';
import { formatDate as formatDateI18n, intlTag, translate, type Locale } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { useT, type T } from '../i18n';
import { Glyph } from './glyphs';
import { wopiExtensions, extensionOf, openInEditor, openableInDocs } from '../lib/wopi';
import { idsDelRango, partirEnTeselas, posicionEnLista } from '../lib/teselas';
import { appQueAbre, subibleACast, urlDeApertura, urlDeSubirACast } from '../lib/abrir-con';
import { MIME_ARRASTRE, movibles, rutaDestino } from '../lib/arrastrar';
import { formatBytes } from '@sytue/ui';
import { VersionsDialog } from './VersionsDialog';
import { ResumenIA } from './ResumenIA.js';
import { PdfPageOrder, expandirPaginas, type PaginaIndice } from './PdfPageOrder';

/** `t` + locale de la isla, en una linea (el proveedor lo pone <SytueShell>). */
function useDriveT(): { t: T; locale: Locale } {
  const { locale } = useI18n();
  return { t: useT(locale), locale };
}

type ViewMode = 'list' | 'grid';

/** Error con codigo de estado HTTP del BFF, para distinguir 401 (sesion). */
class BffError extends Error {
  constructor(
    public readonly status: number,
    /** Motivo legible que devolvió el servicio, si lo mandó (p. ej. por qué no se
     *  puede compartir un archivo etiquetado). Vacío = usar el texto genérico. */
    public readonly reason = '',
  ) {
    super(`bff_${status}`);
  }
}

async function bffJson<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const res = await fetch(input, { credentials: 'include', ...init });
  if (!res.ok) throw new BffError(res.status, await bffReason(res));
  return (await res.json()) as T;
}

/** Lee el `message` del sobre de error del BFF sin romper si no viene JSON. */
async function bffReason(res: Response): Promise<string> {
  try {
    const body = (await res.clone().json()) as { message?: unknown };
    return typeof body.message === 'string' ? body.message : '';
  } catch {
    return '';
  }
}

/** Tamaño de página del listado (keyset en el servicio Go, que acota a ≤500). */
const PAGE_SIZE = 200;

/** Umbral para preferir la subida directa a S3 (presigned): por debajo, el PUT por
 *  el BFF (/api/content) es más simple y evita dos round-trips. La vía presigned
 *  descarga del app-server la subida de ficheros grandes (van del hilo a S3). */
const PRESIGN_MIN_BYTES = 8 * 1024 * 1024; // 8 MiB
/** Cuando la vía presigned resulta no operativa (endpoint ausente, S3 sin CORS o no
 *  alcanzable por el navegador…), se desactiva para el resto de la sesión: el
 *  fallback por BFF (PUT /api/content) sigue funcionando sin reintentos inútiles. */
let presignUnavailable = false;

interface PresignInfo {
  url: string;
  handle: string;
}

/** PUT directo del fichero a la URL prefirmada de S3, con progreso (XHR: fetch no
 *  expone el progreso de subida). Resuelve al terminar; rechaza en error de red/HTTP. */
function putToS3(url: string, file: File, onProgress?: (fraction: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('PUT', url, true);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total);
    };
    xhr.onload = () =>
      xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(`s3_${xhr.status}`));
    xhr.onerror = () => reject(new Error('s3_network'));
    xhr.send(file);
  });
}

/**
 * Subida directa a S3 (presigned) de un fichero grande: presign (permiso + cuota,
 * en el servicio) → PUT directo a S3 con progreso → finalize (verifica en S3 y
 * registra). Devuelve el recurso creado, o `null` si NO aplica/está disponible
 * (fichero pequeño, endpoint ausente, CORS…), para que el llamador caiga al PUT por
 * el BFF. Propaga el 507 (cuota): en ese caso el fallback también fallaría.
 */
async function presignedUpload(
  path: string,
  file: File,
  onProgress?: (fraction: number) => void,
): Promise<{ resource?: DriveResource } | null> {
  if (presignUnavailable || file.size < PRESIGN_MIN_BYTES) return null;
  let info: PresignInfo;
  try {
    info = await bffJson<PresignInfo>('/api/presign', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, size: file.size, contentType: file.type || '' }),
    });
  } catch (e) {
    if (e instanceof BffError && e.status === 507) throw e; // cuota: reintentar no ayuda
    presignUnavailable = true; // presign no operativo → desactiva la vía
    return null;
  }
  try {
    await putToS3(info.url, file, onProgress);
  } catch {
    presignUnavailable = true; // CORS/red a S3 → desactiva y cae al fallback
    return null;
  }
  try {
    return await bffJson<{ resource?: DriveResource }>('/api/finalize', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ handle: info.handle }),
    });
  } catch (e) {
    if (e instanceof BffError && e.status === 507) throw e; // cuota
    return null; // finalize transitorio: cae al fallback (re-sube por el BFF)
  }
}

const drive = {
  /**
   * Página de una carpeta: carpetas completas + primera tanda de ficheros ya
   * ordenados por el servicio (carpetas primero, luego nombre); con `cursor`
   * llegan los ficheros siguientes. `nextCursor` presente cuando hay más.
   */
  list: (path: string, cursor?: string) =>
    bffJson<{ items: DriveResource[]; nextCursor?: string }>(
      `/api/files?path=${encodeURIComponent(path)}&limit=${PAGE_SIZE}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`,
    ),
  createFolder: (path: string) =>
    bffJson<{ resource: DriveResource }>('/api/folders', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path }),
    }),
  rename: (path: string, name: string) =>
    // POST JSON (no PATCH): el CSRF de Astro solo admite POST con cuerpo tras el
    // proxy, y JSON no dispara su checkOrigin. El BFF delega en DriveClient.move().
    bffJson<{ ok: true }>('/api/files', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, name }),
    }),
  move: (path: string, toPath: string) =>
    bffJson<{ ok: true }>('/api/files', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, toPath }),
    }),
  // Operaciones en lote (multiselección): mover/papelera N en UNA operación. El
  // servicio Go las aplica con `id = ANY` (sin N round-trips), gateado por ítem.
  // targetId vacío = raíz «Mi unidad». POST JSON (no dispara el checkOrigin CSRF).
  batch: (
    op: 'move' | 'trash',
    sel: { fileIds: string[]; folderIds: string[] },
    targetId?: string,
  ) =>
    bffJson<{
      moved_files?: number;
      moved_folders?: number;
      trashed_files?: number;
      trashed_folders?: number;
    }>('/api/batch', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ op, ...sel, ...(targetId !== undefined ? { targetId } : {}) }),
    }),
  // Crear un documento de Sytue Docs sin salir de Drive. Devuelve la URL del editor
  // ya resuelta: el BFF crea con el token de sesión server-side, así que no hay rebote
  // de sesión visible al abrir el otro dominio.
  newDoc: (kind: 'document' | 'spreadsheet' | 'presentation') =>
    bffJson<{ id: string; kind: string; href: string }>('/api/new-doc', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ kind }),
    }),
  // Exportar una vista de Sytue Bases como CSV a la carpeta abierta.
  importBases: (path: string, token: string) =>
    bffJson<{ name: string }>('/api/import-bases', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, token }),
    }),
  // Etiquetas de sensibilidad. La taxonomía la define un administrador en Sytue Admin;
  // Drive solo la APLICA. Quitar va por POST sin labelId (el checkOrigin de Astro
  // rechaza DELETE tras el proxy; el servicio Go sí expone DELETE en su API).
  sensitivityLabels: () =>
    bffJson<{
      labels: { id: string; name: string; rank: number; block_external_share?: boolean }[];
    }>('/api/label'),
  setLabel: (fileId: string, labelId: string) =>
    bffJson<{ ok?: true }>('/api/label', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ fileId, ...(labelId ? { labelId } : {}) }),
    }),
  // URL del BFF para descargar una carpeta/selección como ZIP en streaming (para un
  // <a download>: navegación GET con la cookie de sesión, el token nunca toca el hilo).
  zipHref: (sel: { fileIds?: string[]; folderIds?: string[] }, name?: string): string => {
    const qs = new URLSearchParams();
    if (sel.fileIds?.length) qs.set('file_ids', sel.fileIds.join(','));
    if (sel.folderIds?.length) qs.set('folder_ids', sel.folderIds.join(','));
    if (name) qs.set('name', name);
    return withBase(import.meta.env.BASE_URL, `/api/zip?${qs.toString()}`);
  },
  remove: (path: string) =>
    // content-type octet-stream (no "de formulario"), igual que la subida: sin él,
    // Astro tras el proxy ve el DELETE sin content-type con Origin cruzado
    // (https del navegador ≠ http del server) y lo rechaza (403). DELETE no lleva cuerpo.
    bffJson<{ ok: true }>(`/api/files?path=${encodeURIComponent(path)}`, {
      method: 'DELETE',
      headers: { 'content-type': 'application/octet-stream' },
    }),
  upload: async (
    path: string,
    file: File,
    onProgress?: (fraction: number) => void,
  ): Promise<{ resource?: DriveResource }> => {
    // Vía preferida para ficheros grandes: subida directa a S3 (descarga el
    // app-server). Si no aplica/está disponible, cae al PUT por el BFF (probado).
    const direct = await presignedUpload(path, file, onProgress);
    if (direct) return direct;
    // Fallback: siempre octet-stream en el hilo (los content-type "de formulario"
    // disparan la protección CSRF de Astro tras el proxy). El tipo real va en ?type=.
    return bffJson<{ resource?: DriveResource }>(
      `/api/content?path=${encodeURIComponent(path)}&type=${encodeURIComponent(file.type || '')}`,
      {
        method: 'PUT',
        headers: { 'content-type': 'application/octet-stream' },
        body: file,
      },
    );
  },
  // Copia server-side: el servicio Go crea una fila NUEVA que referencia el MISMO
  // blob (sin duplicar bytes), «Copia de X» en la misma carpeta. POST JSON (no
  // dispara el checkOrigin CSRF de Astro tras el proxy), como /api/batch.
  copy: (item: DriveResource) =>
    bffJson<{ resource?: DriveResource }>('/api/copy', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id: item.id, kind: item.isDirectory ? 'folder' : 'file' }),
    }),
  createShare: (
    path: string,
    permission: 'view' | 'edit',
    opts: { expiresAt?: string | undefined; password?: string | undefined } = {},
  ) =>
    bffJson<{ share: ShareLink }>('/api/shares', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        path,
        permission,
        ...(opts.expiresAt ? { expiresAt: opts.expiresAt } : {}),
        ...(opts.password ? { password: opts.password } : {}),
      }),
    }).then((r) => r.share),
  /**
   * Comparte con una PERSONA por su correo. El BFF lo traduce al identificador
   * contra Sytue ID: si lo eligiera el navegador, conceder permiso a quien se
   * quisiera sería escribir un identificador en la petición.
   */
  shareWithPerson: (path: string, email: string, permission: 'view' | 'edit') =>
    bffJson<{ share: ShareLink }>('/api/shares', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, email, permission }),
    }).then((r) => r.share),
  /** Comparte con un GRUPO de Sytue ID. Su nombre ya es el identificador (ver
   *  pages/api/shares.ts): a diferencia de una persona, no hay correo que traducir. */
  shareWithGroup: (path: string, group: string, permission: 'view' | 'edit') =>
    bffJson<{ share: ShareLink }>('/api/shares', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, group, permission }),
    }).then((r) => r.share),
  /** Los grupos propios de quien ha iniciado sesión, para el selector de compartir. */
  myGroups: () => bffJson<{ groups: string[] }>('/api/groups').then((r) => r.groups),
  listShares: (path: string) =>
    bffJson<{ shares: ShareLink[] }>(`/api/shares?path=${encodeURIComponent(path)}`).then(
      (r) => r.shares,
    ),
  revokeShare: (id: string) =>
    // DELETE con octet-stream + query (sin cuerpo), igual que remove(): evita el
    // 403 del checkOrigin de Astro tras el proxy.
    bffJson<{ ok: true }>(`/api/shares?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: { 'content-type': 'application/octet-stream' },
    }),
  listTrash: () => bffJson<{ items: TrashItem[] }>('/api/trash').then((r) => r.items),
  restore: (id: string, kind: 'file' | 'folder') =>
    // JSON no dispara el checkOrigin CSRF de Astro (solo los content-type «de
    // formulario»), igual que POST /api/folders.
    bffJson<{ ok: true }>('/api/trash', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id, kind }),
    }),
  purge: (id: string, kind: 'file' | 'folder') =>
    // DELETE con octet-stream + query (sin cuerpo), igual que remove(): evita el
    // 403 del checkOrigin de Astro tras el proxy.
    bffJson<{ ok: true }>(`/api/trash?id=${encodeURIComponent(id)}&kind=${kind}`, {
      method: 'DELETE',
      headers: { 'content-type': 'application/octet-stream' },
    }),
  // Recientes: ficheros del tenant por reciente-primero (con su ruta resuelta).
  // El cursor es opaco: viene del `nextCursor` de la respuesta anterior (keyset).
  recent: (cursor?: string) =>
    bffJson<{ items: DriveResource[]; nextCursor?: string }>(
      cursor ? `/api/recent?cursor=${encodeURIComponent(cursor)}` : '/api/recent',
    ),
  // Favoritos del usuario (carpetas y ficheros, con su ruta resuelta).
  favorites: () => bffJson<{ items: DriveResource[] }>('/api/favorites').then((r) => r.items),
  setFavorite: (item: DriveResource, favorite: boolean) =>
    // POST JSON (no dispara el checkOrigin CSRF de Astro), como /api/folders.
    bffJson<{ ok: true }>('/api/favorites', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id: item.id, kind: item.isDirectory ? 'folder' : 'file', favorite }),
    }),
  quota: () =>
    bffJson<{ usedBytes: number; limitBytes: number; availableBytes: number }>('/api/quota'),
  // Compartido conmigo: raíces compartidas (fichero/carpeta) con permiso + dueño.
  sharedWithMe: () => bffJson<{ items: SharedRoot[] }>('/api/shared-with-me').then((r) => r.items),
  // Navegar DENTRO de una carpeta compartida (rutas relativas a la raíz del share).
  sharedList: (id: string, subpath: string) =>
    bffJson<{
      permission: SharedPermission;
      isDir: boolean;
      more: boolean;
      items: DriveResource[];
    }>(`/api/shared?id=${encodeURIComponent(id)}&subpath=${encodeURIComponent(subpath)}`),
  // Subir a una carpeta compartida (solo editor; el servicio deniega a viewers).
  sharedUpload: (id: string, subpath: string, file: File) =>
    bffJson<{ resource: DriveResource }>(
      `/api/shared-upload?id=${encodeURIComponent(id)}&subpath=${encodeURIComponent(subpath)}&name=${encodeURIComponent(file.name)}&type=${encodeURIComponent(file.type || '')}`,
      { method: 'PUT', headers: { 'content-type': 'application/octet-stream' }, body: file },
    ),
};

/** Permiso de un share dirigido al usuario (vocabulario de la UI de compartición). */
type SharedPermission = 'view' | 'edit';

/** Raíz de «Compartido conmigo»: un recurso con su permiso y quién lo compartió. */
type SharedRoot = DriveResource & {
  permission: SharedPermission;
  sharedBy: string;
  /** En lectura, ya ha pedido editarlo (`FranjaPeticion`). */
  editRequested?: boolean;
};

/** URL del BFF para el contenido de un fichero DENTRO de un recurso compartido. */
function sharedContentHref(rootId: string, subpath: string, inline = false): string {
  const q = `id=${encodeURIComponent(rootId)}&subpath=${encodeURIComponent(subpath)}`;
  return withBase(
    import.meta.env.BASE_URL,
    `/api/shared-content?${q}${inline ? '&disposition=inline' : ''}&offline=1`,
  );
}

/**
 * Tamaño legible. El SEPARADOR DECIMAL lo pone el idioma («1.5 MB» en inglés,
 * «1,5 MB» en español), por eso se delega en `formatBytes` (que pasa por
 * `formatNumber`) en vez de un `toFixed` fijo. Las unidades (B, KB, MB…) son
 * símbolos y NO se traducen.
 */
function formatSize(locale: Locale, bytes?: number): string {
  return bytes === undefined ? '—' : formatBytes(locale, bytes);
}

/** Fecha corta de modificación, en el formato del idioma activo. */
function formatDate(locale: Locale, iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return formatDateI18n(locale, d, { day: '2-digit', month: 'short', year: 'numeric' });
}

/** Extension en minusculas (sin punto); '' si el nombre no tiene extension. */
function fileExt(name: string): string {
  const i = name.lastIndexOf('.');
  return i > 0 ? name.slice(i + 1).toLowerCase() : '';
}

/** Extensiones que el navegador pinta nativamente en <img>. */
const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'avif']);
/** Extensiones de texto plano legibles en un <pre>. */
const TEXT_EXTS = new Set([
  'txt',
  'md',
  'csv',
  'json',
  'log',
  'xml',
  'yaml',
  'yml',
  'ini',
  'toml',
  'js',
  'ts',
  'css',
  'html',
  'htm',
  'sh',
  'go',
  'py',
  'sql',
]);

/**
 * Visor inline que corresponde a un recurso ('image' | 'pdf' | 'text'), o null
 * si solo cabe descargarlo. Decide por content-type real (lo guarda el servicio
 * al subir) con la extension como respaldo.
 */
export function previewKind(item: DriveResource): 'image' | 'pdf' | 'text' | 'office' | null {
  if (item.isDirectory) return null;
  const ct = item.contentType ?? '';
  const ext = fileExt(item.name);
  if (ct.startsWith('image/') || IMAGE_EXTS.has(ext)) return 'image';
  if (ct === 'application/pdf' || ext === 'pdf') return 'pdf';
  if (ct.startsWith('text/') || ct === 'application/json' || TEXT_EXTS.has(ext)) return 'text';
  // Lo último, aposta: `openableInDocs` incluye también pdf, csv o md, que ya
  // tienen visor propio unas líneas más arriba. Aquí solo quedan los formatos
  // que hay que convertir para ver algo: docx, xlsx, pptx y sus equivalentes ODF.
  if (openableInDocs(item.name)) return 'office';
  return null;
}

/**
 * Nombre del marco donde se pinta la previsualización ofimática.
 *
 * Existe porque el editor exige el token por POST de formulario, y un formulario
 * solo sabe entregar su respuesta a una ventana CON NOMBRE. Es constante y no
 * generado: solo hay un diálogo de previsualización abierto a la vez.
 */
const MARCO_WOPI = 'sytue-wopi-preview';

/**
 * Abre el fichero en la app de la suite que le corresponda. Devuelve `true` si
 * se ha ido a otro sitio, y entonces no hay visor que enseñar.
 *
 * Va aquí, a nivel de módulo, porque hay DOS listas que abren ficheros —el
 * explorador de carpetas y la de Recientes/Favoritos— y cada una tenía su propio
 * `handleOpen`. Escrito dos veces, una de las dos se queda atrás: es exactamente
 * lo que pasaba con el visor, que en Recientes ni miraba si el fichero podía
 * abrirse en el editor.
 *
 * EN LA MISMA PESTAÑA y no en otra: es abrir un fichero, no lanzar una
 * aplicación aparte, y así Atrás del navegador devuelve a la carpeta igual que
 * el botón de volver que lleva la app de destino.
 */
async function abrirEnLaSuite(
  item: DriveResource,
  kind: ReturnType<typeof previewKind>,
): Promise<boolean> {
  // `wopiExtensions()` se memoriza por carga de página: esperarlo aquí cuesta
  // una petición la primera vez y nada las demás.
  const app = appQueAbre(kind, {
    conMiniatura: thumbable(item),
    conEditor: item.id ? (await wopiExtensions()).has(extensionOf(item.name)) : false,
    ext: extensionOf(item.name),
  });
  if (app === 'pdf' || app === 'photos' || app === 'docs' || app === 'sheets' || app === 'slides') {
    // La org de la URL ACTUAL (ADR 0056): quien abre un fichero lo hace desde su
    // carpeta `/drive/<org>/…`, y con ella el destino aterriza canónico, sin el
    // 302 que de otro modo añade el middleware de la app de destino.
    window.location.assign(urlDeApertura(app, item, orgActual(import.meta.env.BASE_URL)));
    return true;
  }
  // Si el editor fallara —el token no se acuña, no contesta— se cae al visor de
  // siempre en vez de dejar la pantalla como estaba sin decir nada.
  if (app === 'editor' && item.id) return openInEditor(item.id, 'edit', '_self');
  return false;
}

/** Content-types ráster con miniatura server-side (el backend usa la stdlib de Go). */
const THUMB_CTS = new Set(['image/jpeg', 'image/png', 'image/gif']);
/** Respaldo por extensión cuando el fichero no trae content-type. */
const THUMB_EXTS = new Set(['jpg', 'jpeg', 'png', 'gif']);

/**
 * ¿Tiene el fichero miniatura server-side? Solo ráster JPEG/PNG/GIF (lo que el
 * servicio Go genera con la stdlib). WebP/AVIF/SVG caen al icono (el backend no los
 * miniaturiza); un desajuste residual lo cubre el onError del <img>. Necesita id (la
 * miniatura se pide por id, no por ruta).
 */
function thumbable(item: DriveResource): boolean {
  if (item.isDirectory || !item.id) return false;
  const ct = item.contentType ?? '';
  if (ct) return THUMB_CTS.has(ct);
  return THUMB_EXTS.has(fileExt(item.name));
}

/**
 * Color del distintivo de tipo (solo texto/glyph; el tile de fondo es neutro).
 * Usa roles -text calibrados a AA; el color acompaña siempre a las letras de la
 * extension, nunca es el unico portador del significado.
 */
const KIND_TINT: Record<string, string> = {
  pdf: 'text-danger-text',
  doc: 'text-primary-text',
  docx: 'text-primary-text',
  rtf: 'text-primary-text',
  odt: 'text-primary-text',
  xls: 'text-success-text',
  xlsx: 'text-success-text',
  csv: 'text-success-text',
  ods: 'text-success-text',
  ppt: 'text-warning-text',
  pptx: 'text-warning-text',
  odp: 'text-warning-text',
  key: 'text-warning-text',
  png: 'text-violet-text',
  jpg: 'text-violet-text',
  jpeg: 'text-violet-text',
  gif: 'text-violet-text',
  webp: 'text-violet-text',
  svg: 'text-violet-text',
  avif: 'text-violet-text',
  heic: 'text-violet-text',
  mp4: 'text-violet-text',
  mov: 'text-violet-text',
  webm: 'text-violet-text',
  mp3: 'text-violet-text',
  wav: 'text-violet-text',
  zip: 'text-warning-text',
  rar: 'text-warning-text',
  '7z': 'text-warning-text',
  tar: 'text-warning-text',
  gz: 'text-warning-text',
};

/** Segmentos de la ruta actual para el breadcrumb. */
function segments(path: string): { name: string; path: string }[] {
  const parts = path.split('/').filter(Boolean);
  const acc: { name: string; path: string }[] = [];
  let cur = '';
  for (const part of parts) {
    cur += `/${part}`;
    acc.push({ name: decodeURIComponent(part), path: cur });
  }
  return acc;
}

/** Subidas simultáneas máximas (paralelo ACOTADO): no saturar red ni el servicio. */
const UPLOAD_CONCURRENCY = 3;

/** Un fichero a subir con su subruta relativa ('' = carpeta actual; 'a/b' = drop de carpeta). */
interface UploadEntry {
  file: File;
  dir: string;
}

function readEntriesAsync(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
  return new Promise((resolve, reject) => reader.readEntries(resolve, reject));
}

function fileOfEntry(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject));
}

/**
 * Recorre una FileSystemEntry (webkitGetAsEntry) a una lista plana de {file, dir}.
 * Un fichero suelto queda con dir=''; el contenido de una carpeta arrastrada lleva
 * su subruta relativa (p.ej. 'proyecto/sub'). readEntries devuelve por tandas: se
 * llama hasta vaciar.
 */
async function walkEntry(
  entry: FileSystemEntry,
  parentDir: string,
  out: UploadEntry[],
): Promise<void> {
  if (entry.isFile) {
    out.push({ file: await fileOfEntry(entry as FileSystemFileEntry), dir: parentDir });
    return;
  }
  if (entry.isDirectory) {
    const childDir = parentDir ? `${parentDir}/${entry.name}` : entry.name;
    const reader = (entry as FileSystemDirectoryEntry).createReader();
    for (;;) {
      const batch = await readEntriesAsync(reader);
      if (batch.length === 0) break;
      for (const child of batch) await walkEntry(child, childDir, out);
    }
  }
}

/**
 * Extrae los ficheros de un arrastre. Con la API de entradas (webkitGetAsEntry)
 * conserva la estructura de carpetas; si no está, cae a la lista plana de ficheros.
 * Las entradas raíz se capturan de forma SÍNCRONA (la lista solo es válida durante
 * el evento drop) antes de cualquier lectura asíncrona.
 */
async function collectDropEntries(dt: DataTransfer): Promise<UploadEntry[]> {
  const roots: FileSystemEntry[] = [];
  for (let i = 0; i < dt.items.length; i++) {
    const entry = dt.items[i]?.webkitGetAsEntry?.();
    if (entry) roots.push(entry);
  }
  if (roots.length === 0) return Array.from(dt.files).map((file) => ({ file, dir: '' }));
  const out: UploadEntry[] = [];
  for (const entry of roots) await walkEntry(entry, '', out);
  return out;
}

/**
 * urlParam — lee un parámetro de la URL en el primer render.
 *
 * Hasta ahora la carpeta abierta era estado en memoria y nada más: no se podía copiar el
 * enlace de una carpeta, ni marcarla, ni —lo que trajo esto— aterrizar aquí desde un
 * `sytue://` de la app de escritorio. `?path=` es la carpeta, `?preview=` el fichero que
 * se abre al llegar.
 */
function urlParam(name: string): string {
  if (typeof window === 'undefined') return '';
  return new URLSearchParams(window.location.search).get(name) ?? '';
}

/**
 * resourceHref — la URL pública de un recurso: la que se copia y la que viaja al deep
 * link. Bajo el prefijo de la app: la raíz pelada con `?path=` solo llegaba de rebote
 * (Traefik manda al pod la raíz exacta) y no es la página de Drive. La app nativa lo
 * lee con o sin el prefijo (`DestinoDeEnlace`).
 */
function resourceHref(item: DriveResource): string {
  const u = new URL(`${appUrl('drive')}/`);
  if (item.isDirectory) {
    if (item.path !== '/') u.searchParams.set('path', item.path);
    return u.toString();
  }
  const parent = item.path.slice(0, item.path.lastIndexOf('/')) || '/';
  if (parent !== '/') u.searchParams.set('path', parent);
  u.searchParams.set('preview', item.path);
  return u.toString();
}

export function FileBrowser() {
  const toast = useToast();
  const offline = useConnectivity() === 'offline';
  // Solo `t`: tamaños y fechas los formatean las filas (FileRow/FileGrid), que
  // leen el locale por su cuenta del mismo contexto.
  const { t } = useDriveT();

  const [path, setPath] = useState(() => urlParam('path') || '/');
  // La barra de vista, para el aviso de que queda contenido a la derecha en móvil.
  // Se lee en el primer render porque el efecto que sincroniza la URL borra `preview`
  // enseguida (es de un solo uso, no debe quedarse pegado a la sesión).
  const wantPreview = useRef(urlParam('preview'));
  // Búsqueda dentro de Drive. El servicio la sabía hacer desde el principio (y el Hub la
  // federa en la topbar), pero el explorador no tenía caja: encontrar un fichero obligaba
  // a navegar carpeta por carpeta. Con término, la lista pasa a ser la de resultados.
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<DriveResource[] | null>(null);
  // La búsqueda no llegó a responder. Se distingue de «no hay nada que coincida»
  // porque la salida es la contraria: ahí se reintenta, aquí se cambia el término.
  const [searchFailed, setSearchFailed] = useState(false);
  const [items, setItems] = useState<DriveResource[] | null>(null);
  // Cursor keyset de la página siguiente (null = no hay más). Se resetea al
  // recargar la carpeta; `loadingMore` solo afecta al pie/botón «cargar más».
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  // 'session' (401: la sesión caducó, la salida es volver a entrar) o 'network'
  // (cualquier otro fallo, reintentable). El copy y la acción se derivan al pintar.
  const [error, setError] = useState<'session' | 'network' | null>(null);
  const [view, setView] = useState<ViewMode>('list');
  const [busy, setBusy] = useState(false);

  // Estado propio de la subida: el spinner de «Subir» solo debe girar al subir,
  // no durante otras mutaciones (que sí deshabilitan las acciones vía `busy`).
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<{
    current: number;
    total: number;
    name: string;
    // Progreso 0..1 del fichero EN CURSO (subida directa a S3); afina la barra sin
    // alterar el conteo de ficheros completados. Ausente/0 en la vía por BFF.
    fraction?: number;
  } | null>(null);
  // Ídem para «Crear»: solo gira mientras se crea la carpeta.
  const [creating, setCreating] = useState(false);

  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [folderName, setFolderName] = useState('');
  // Compartir: recurso cuyo modal de enlaces esta abierto (crear/listar/revocar).
  const [shareItem, setShareItem] = useState<DriveResource | null>(null);
  // Vista previa inline (imagen/PDF/texto): recurso abierto en el visor.
  const [previewItem, setPreviewItem] = useState<DriveResource | null>(null);
  // Historial de versiones: fichero cuyo panel de versiones está abierto.
  const [versionsItem, setVersionsItem] = useState<DriveResource | null>(null);
  const [organizePdfItem, setOrganizePdfItem] = useState<DriveResource | null>(null);
  // Clasificación: el fichero en curso y la taxonomía de la organización. La taxonomía
  // se pide al ABRIR el diálogo, no al montar el explorador: la mayoría de sesiones no
  // clasifica nada y sería una petición por carga para un dato que casi nadie usa.
  const [labelItem, setLabelItem] = useState<DriveResource | null>(null);
  // El fichero que se está resumiendo. Null = sin diálogo abierto.
  const [resumenItem, setResumenItem] = useState<DriveResource | null>(null);
  const [labelTaxonomy, setLabelTaxonomy] = useState<
    { id: string; name: string; rank: number; block_external_share?: boolean }[] | null
  >(null);
  // Renombrar: recurso pendiente + nombre editable (precargado con el actual).
  const [pendingRename, setPendingRename] = useState<DriveResource | null>(null);
  const [renameName, setRenameName] = useState('');
  const [renaming, setRenaming] = useState(false);
  // Mover a…: recurso pendiente (el picker de carpetas vive en MoveDialog).
  const [pendingMove, setPendingMove] = useState<DriveResource | null>(null);
  // Ids de los favoritos del usuario: pinta la etiqueta correcta en el menú
  // (Añadir/Quitar) sin un fetch por fila. Acotado (decenas), sin N+1.
  const [favIds, setFavIds] = useState<ReadonlySet<string>>(() => new Set());

  // Multiselección (por id): conjunto seleccionado + ancla para rangos con
  // shift-click. Se vacía al cambiar de carpeta o recargar (ver load()).
  const [selected, setSelected] = useState<ReadonlySet<string>>(() => new Set());
  const anchorRef = useRef<number>(-1);
  // Diálogo «Mover selección a…» y confirmación de «Enviar selección a la papelera».
  const [batchMoveOpen, setBatchMoveOpen] = useState(false);
  const [batchTrashOpen, setBatchTrashOpen] = useState(false);

  const fileInput = useRef<HTMLInputElement>(null);

  // Arrastrar-soltar: `dragging` pinta el overlay; `dragDepth` cuenta enter/leave
  // (los hijos disparan leave al entrar en ellos) para no parpadear al arrastrar.
  const [dragging, setDragging] = useState(false);
  const dragDepth = useRef(0);

  // Si la red cae con un diálogo de escritura abierto, ciérralo de inmediato.
  // Las vistas previas y el historial permanecen: son capacidades de lectura.
  useEffect(() => {
    if (!offline) return;
    setDragging(false);
    setNewFolderOpen(false);
    setShareItem(null);
    setLabelItem(null);
    setPendingRename(null);
    setPendingMove(null);
    setBatchMoveOpen(false);
    setBatchTrashOpen(false);
    setSelected(new Set());
  }, [offline]);

  // Invalida respuestas en vuelo al cambiar de carpeta o recargar: una página
  // del listado anterior no debe mezclarse con el actual.
  const epoch = useRef(0);
  // Guarda síncrona contra la ráfaga de eventos de scroll (el estado
  // `loadingMore` llega un render tarde y dispararía peticiones duplicadas).
  const fetchingMore = useRef(false);

  const load = useCallback(async (target: string) => {
    const e = ++epoch.current;
    setItems(null);
    setNextCursor(null);
    setError(null);
    setSelected(new Set());
    anchorRef.current = -1;
    try {
      // El orden llega del servicio (carpetas primero, luego ficheros por
      // nombre, keyset): reordenar en cliente rompería la continuidad al
      // encadenar páginas.
      const page = await drive.list(target);
      if (epoch.current !== e) return;
      setItems(page.items);
      setNextCursor(page.nextCursor ?? null);
    } catch (err) {
      if (epoch.current !== e) return;
      setError(err instanceof BffError && err.status === 401 ? 'session' : 'network');
    }
  }, []);

  /** Página siguiente (scroll en lista, «Cargar más» en rejilla): añade al final. */
  const loadMore = useCallback(async () => {
    if (!nextCursor || fetchingMore.current) return;
    fetchingMore.current = true;
    const e = epoch.current;
    setLoadingMore(true);
    try {
      const page = await drive.list(path, nextCursor);
      if (epoch.current !== e) return;
      setItems((prev) => [...(prev ?? []), ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      // No rompe lo ya cargado: aviso y reintento al volver a hacer scroll/clic.
      if (epoch.current === e) toast.notify({ tone: 'danger', title: t('files.loadMoreFailed') });
    } finally {
      fetchingMore.current = false;
      setLoadingMore(false);
    }
  }, [path, nextCursor, toast, t]);

  useEffect(() => {
    void load(path);
  }, [path, load]);

  // Carga el conjunto de favoritos una vez; el explorador funciona sin él.
  useEffect(() => {
    let alive = true;
    drive
      .favorites()
      .then((favs) => {
        if (alive) setFavIds(new Set(favs.map((f) => f.id).filter((id): id is string => !!id)));
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const goTo = (p: string) => setPath(p || '/');

  const handleOpen = async (item: DriveResource) => {
    if (item.isDirectory) {
      goTo(item.path);
      return;
    }
    const kind = previewKind(item);
    // SI LA SUITE TIENE UN PRODUCTO PARA ESTE FORMATO, SE ABRE ALLÍ. El visor de
    // Drive enseñaba el PDF sin poder anotarlo ni firmarlo y la foto sin poder
    // editarla, teniendo CertiPDF y Fotos al lado. El fichero no se copia: se
    // abre donde está, con la ACL y las versiones de Drive.
    if (await abrirEnLaSuite(item, kind)) return;

    // ponytail: textos >1 MB directos a descarga; un <pre> enorme congela la pestaña.
    // La ofimática sin `id` tampoco se puede previsualizar: el token WOPI se acuña
    // contra el identificador del fichero, no contra su ruta.
    if (
      !kind ||
      (kind === 'text' && (item.size ?? 0) > 1_000_000) ||
      (kind === 'office' && !item.id)
    ) {
      onDownload(item);
      return;
    }

    setPreviewItem(item);
  };

  // Con término escrito, la lista que se pinta es la de resultados y no la de la carpeta.
  // `results === null` mientras aún no ha vuelto la primera respuesta, que es lo que hace
  // que el esqueleto aparezca igual que al cambiar de carpeta.
  const searchMode = query.trim() !== '';
  const shown = searchMode ? results : items;

  // Invalida búsquedas en vuelo: con el rebote es raro, pero una respuesta lenta de
  // «inf» llegando después de la de «informe» pintaría los resultados del término
  // que ya no está escrito.
  const searchEpoch = useRef(0);

  /** Busca UN término. Un fallo se cuenta como fallo, no como «no hay nada». */
  const runSearch = useCallback(async (q: string) => {
    const e = ++searchEpoch.current;
    try {
      const res = await fetch(`/api/search?q=${encodeURIComponent(q)}`, {
        credentials: 'include',
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new BffError(res.status);
      const body = (await res.json()) as { items?: DriveResource[] };
      if (searchEpoch.current !== e) return;
      setResults(body.items ?? []);
      setSearchFailed(false);
    } catch {
      if (searchEpoch.current !== e) return;
      setResults([]);
      setSearchFailed(true);
    }
  }, []);

  // Búsqueda con rebote de 300 ms: una petición por pulsación castigaría al servicio y
  // haría parpadear la lista. El explorador nunca se rompe por esto: la carpeta sigue
  // navegable detrás.
  useEffect(() => {
    // La selección pertenece a la lista que se estaba mirando: al entrar o salir
    // de la búsqueda esa lista cambia entera, y arrastrar lo marcado dejaría la
    // barra de acciones hablando de ficheros que ya no están delante.
    setSelected(new Set());
    anchorRef.current = -1;
    const q = query.trim();
    if (!q) {
      searchEpoch.current++; // descarta la respuesta en vuelo al vaciar la caja
      setResults(null);
      setSearchFailed(false);
      return;
    }
    const id = window.setTimeout(() => void runSearch(q), 300);
    return () => window.clearTimeout(id);
  }, [query, runSearch]);

  // La URL sigue a la carpeta abierta. `replace` y no `push`: Atrás debe sacar de Drive
  // como ha hecho siempre, no recorrer el árbol hacia atrás. Con esto la barra de
  // direcciones es copiable y sirve de destino a un deep link.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const u = new URL(window.location.href);
    if (path === '/') u.searchParams.delete('path');
    else u.searchParams.set('path', path);
    u.searchParams.delete('preview');
    window.history.replaceState(null, '', u.toString());
  }, [path]);

  // Un deep link puede apuntar a un fichero concreto (?preview=): al llegar su carpeta se
  // abre el visor. Una sola vez; a partir de ahí navegar es cosa de quien mira.
  useEffect(() => {
    const want = wantPreview.current;
    if (!want || !items) return;
    wantPreview.current = '';
    const it = items.find((i) => i.path === want && !i.isDirectory);
    // SOLO SE ABRE LO QUE LA SUITE SABE ABRIR. `handleOpen` descarga lo que no
    // puede previsualizar, y un enlace del Hub que dispara una descarga sola —sin
    // que nadie haya pulsado «descargar»— no es lo que espera quien viene de la
    // actividad reciente. Para esos, el destino es la carpeta con el fichero a la
    // vista, que es lo que hace esta pantalla sin hacer nada más.
    if (it && previewKind(it)) void handleOpen(it);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- de un solo uso, al llegar los ítems
  }, [items]);

  const onUploadClick = () => {
    if (!offline) fileInput.current?.click();
  };

  // «u» sube y «n» crea una carpeta: los atajos que pulsa el menú nativo de la
  // app de escritorio («Archivos → Subir… ⌘U / Nueva carpeta ⌘N»).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!esAtajoDeApp(e) || offline) return;
      if (e.key === 'u') {
        e.preventDefault();
        fileInput.current?.click();
      } else if (e.key === 'n') {
        e.preventDefault();
        setNewFolderOpen(true);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [offline]);

  // Subida COMPARTIDA por el botón «Subir» y el arrastrar-soltar. Reusa el endpoint
  // de subida existente (drive.upload → PUT /api/content). En PARALELO ACOTADO
  // (UPLOAD_CONCURRENCY) con progreso e informe de errores/cuota; para drops de
  // carpeta, crea antes las subcarpetas necesarias (de someras a profundas).
  const uploadEntries = useCallback(
    async (entries: UploadEntry[]) => {
      if (offline || entries.length === 0 || busy) return;
      setBusy(true);
      setUploading(true);
      const total = entries.length;
      let done = 0;
      setUploadProgress({ current: 0, total, name: entries[0]!.file.name });
      const base = path.replace(/\/$/, '');

      // 1) Subcarpetas del drop (de un fichero suelto: ninguna). Únicas + ancestros,
      // ordenadas por profundidad; se toleran las que ya existan (409).
      const dirs = new Set<string>();
      for (const e of entries) {
        if (!e.dir) continue;
        let acc = '';
        for (const seg of e.dir.split('/')) {
          acc = acc ? `${acc}/${seg}` : seg;
          dirs.add(acc);
        }
      }
      for (const dir of [...dirs].sort((a, b) => a.split('/').length - b.split('/').length)) {
        try {
          await drive.createFolder(`${base}/${dir}`);
        } catch (e) {
          // 409 = la carpeta ya existe (drop repetido): no es un fallo. Otros errores
          // los evidenciará la subida de sus ficheros.
          if (!(e instanceof BffError && e.status === 409)) {
            /* continúa: la subida reportará lo que no entre */
          }
        }
      }

      // 2) Subida en paralelo acotado: un pool de UPLOAD_CONCURRENCY «workers» que
      // consumen la cola. Cada fallo se anota (con marca de cuota) sin abortar el resto.
      const failures: { name: string; quota: boolean }[] = [];
      let cursor = 0;
      const worker = async () => {
        while (cursor < entries.length) {
          const { file, dir } = entries[cursor++]!;
          const dest = `${base}/${dir ? `${dir}/` : ''}${file.name}`;
          try {
            await drive.upload(dest, file, (fraction) =>
              // Progreso intra-fichero de la subida directa a S3: afina la barra; el
              // texto sigue contando ficheros completados (`done`).
              setUploadProgress({ current: done, total, name: file.name, fraction }),
            );
          } catch (err) {
            failures.push({
              name: file.name,
              quota: err instanceof BffError && err.status === 507,
            });
          } finally {
            done += 1;
            setUploadProgress({ current: done, total, name: file.name });
          }
        }
      };
      await Promise.all(
        Array.from({ length: Math.min(UPLOAD_CONCURRENCY, entries.length) }, () => worker()),
      );

      // 3) Resumen sobrio + recarga.
      if (failures.length === 0) {
        toast.notify({ tone: 'success', title: t('files.upload.done', { count: total }) });
      } else {
        const ok = total - failures.length;
        toast.notify({
          tone: 'danger',
          title: failures.some((f) => f.quota)
            ? t('files.upload.quota', { uploaded: ok, count: total })
            : t('files.upload.partial', {
                uploaded: ok,
                count: total,
                failed: failures.length,
              }),
        });
      }
      setBusy(false);
      setUploading(false);
      setUploadProgress(null);
      await load(path);
    },
    [offline, busy, path, toast, load, t],
  );

  const onFilesSelected = async (e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = '';
    await uploadEntries(files.map((file) => ({ file, dir: '' })));
  };

  // --- Arrastrar-soltar (solo activa con ficheros del SO; ignora drags ajenos) ---

  const hasFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files');

  const onDragEnter = (e: DragEvent) => {
    if (offline || !hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current += 1;
    if (!busy) setDragging(true);
  };
  const onDragOver = (e: DragEvent) => {
    if (offline || !hasFiles(e)) return;
    e.preventDefault(); // habilita el drop
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
  };
  const onDragLeave = (e: DragEvent) => {
    if (!hasFiles(e)) return;
    dragDepth.current -= 1;
    if (dragDepth.current <= 0) {
      dragDepth.current = 0;
      setDragging(false);
    }
  };
  const onDrop = async (e: DragEvent) => {
    if (offline || !hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current = 0;
    setDragging(false);
    if (busy || !e.dataTransfer) return;
    const entries = await collectDropEntries(e.dataTransfer);
    await uploadEntries(entries);
  };

  // «Hacer una copia» server-side (referencia el mismo blob; sin duplicar bytes).
  const onCopy = async (item: DriveResource) => {
    if (offline || !item.id || busy) return;
    setBusy(true);
    try {
      await drive.copy(item);
      toast.notify({ tone: 'success', title: t('files.copy.done', { name: item.name }) });
      await load(path);
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title:
          e instanceof BffError && e.status === 507
            ? t('files.copy.noSpace')
            : t('files.copy.failed'),
      });
    } finally {
      setBusy(false);
    }
  };

  const onCreateFolder = async () => {
    const name = folderName.trim();
    // La guarda de `busy` evita el doble envío (Enter repetido mientras crea).
    if (offline || !name || busy) return;
    setBusy(true);
    setCreating(true);
    try {
      await drive.createFolder(`${path.replace(/\/$/, '')}/${name}`);
      toast.notify({ tone: 'success', title: t('files.newFolder.created', { name }) });
      setNewFolderOpen(false);
      setFolderName('');
      await load(path);
    } catch {
      toast.notify({ tone: 'danger', title: t('files.newFolder.failed') });
    } finally {
      setBusy(false);
      setCreating(false);
    }
  };

  /**
   * Empieza a arrastrar. Si el elemento forma parte de la selección, viaja la
   * selección ENTERA: arrastrar uno de doce marcados y mover solo ese es lo
   * contrario de lo que acaba de pedir quien los marcó.
   */
  function onDragItem(e: DragEvent, item: DriveResource): void {
    if (!e.dataTransfer) return;
    const enSeleccion = !!(item.id && selected.has(item.id));
    const rutas =
      enSeleccion && selectionCount > 0 ? selectedItems.map((i) => i.path) : [item.path];
    e.dataTransfer.setData(MIME_ARRASTRE, JSON.stringify(rutas));
    e.dataTransfer.effectAllowed = 'move';
  }

  /** Suelta sobre una carpeta: mueve lo que de verdad cambia de sitio. */
  async function onDropOnFolder(e: DragEvent, destino: string): Promise<void> {
    let rutas: string[] = [];
    try {
      rutas = JSON.parse(e.dataTransfer?.getData(MIME_ARRASTRE) || '[]') as string[];
    } catch {
      return; // arrastre de otra cosa: se ignora sin ruido
    }
    const aMover = movibles(rutas, destino);
    if (aMover.length === 0) return;
    setBusy(true);
    let movidos = 0;
    try {
      for (const origen of aMover) {
        // En serie y no en paralelo: mover cien ficheros a la vez son cien
        // escrituras simultáneas sobre el mismo prefijo, y el primer 429 dejaría
        // la mitad movida sin que nadie sepa cuál.
        await drive.move(origen, rutaDestino(origen, destino));
        movidos++;
      }
      toast.notify({ tone: 'success', title: t('files.move.done', { count: movidos }) });
    } catch {
      // Se dice CUÁNTOS iban: «ha fallado» a secas deja sin saber si hay que
      // repetir la operación entera o solo lo que quedaba.
      toast.notify({ tone: 'danger', title: t('files.move.failed', { count: movidos }) });
    } finally {
      setBusy(false);
      clearSelection();
      void load(path);
    }
  }

  const onRename = async () => {
    const item = pendingRename;
    const name = renameName.trim();
    // La guarda de `busy` evita el doble envío (Enter repetido mientras renombra).
    if (offline || !item || !name || busy) return;
    if (name === item.name) {
      setPendingRename(null);
      return;
    }
    setBusy(true);
    setRenaming(true);
    try {
      await drive.rename(item.path, name);
      toast.notify({ tone: 'success', title: t('files.rename.done', { name }) });
      setPendingRename(null);
      await load(path);
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title:
          e instanceof BffError && e.status === 409
            ? t('files.rename.conflict', { name })
            : t('files.rename.failed'),
      });
    } finally {
      setBusy(false);
      setRenaming(false);
    }
  };

  /** Devuelve a su carpeta lo que se acaba de mandar a la papelera. */
  const onUndoDelete = async (item: DriveResource, id: string) => {
    try {
      await drive.restore(id, item.isDirectory ? 'folder' : 'file');
      toast.notify({ tone: 'success', title: t('trash.restored', { name: item.name }) });
      await load(path);
    } catch {
      toast.notify({ tone: 'danger', title: t('trash.restoreFailed') });
    }
  };

  /**
   * Manda un elemento a la papelera. NO pregunta: esto es reversible —el elemento
   * sigue en la papelera con su cuenta atrás— y un diálogo por cada archivo que se
   * tira es una interrupción que no compra nada. Lo que sí compra es poder
   * DESHACERLO desde el mismo aviso, sin ir a buscarlo a la papelera. El diálogo se
   * queda donde de verdad no hay vuelta atrás: la purga.
   */
  const onDelete = async (item: DriveResource) => {
    if (offline || busy) return;
    setBusy(true);
    try {
      await drive.remove(item.path);
      const id = item.id;
      toast.notify({
        tone: 'success',
        title: t('files.delete.done', { name: item.name }),
        // 10 s y no los 5 de serie: deshacer hay que leerlo, decidirlo y alcanzarlo,
        // y un botón que se va antes de eso es como no tenerlo.
        duration: 10_000,
        // Sin id no hay a qué llamar para restaurar (la papelera trabaja por id, no
        // por ruta): se avisa igual, y la papelera sigue siendo el camino.
        ...(id
          ? {
              action: { label: t('files.delete.undo'), onClick: () => void onUndoDelete(item, id) },
            }
          : {}),
      });
      await load(path);
    } catch {
      toast.notify({ tone: 'danger', title: t('files.delete.failed') });
    } finally {
      setBusy(false);
    }
  };

  const onDownload = (item: DriveResource) => {
    // Una carpeta se descarga como ZIP en streaming (BFF /api/zip → servicio Go);
    // un fichero, como blob directo (BFF /api/content). Ambos por <a download> con
    // la cookie de sesión: el token nunca toca el cliente.
    const a = document.createElement('a');
    if (item.isDirectory) {
      if (!item.id) return;
      a.href = drive.zipHref({ folderIds: [item.id] }, item.name);
    } else {
      a.href = `/api/content?path=${encodeURIComponent(item.path)}&offline=1`;
      a.download = item.name;
    }
    a.rel = 'noopener';
    a.click();
  };

  // Compartir abre el gestor de enlaces del recurso (ShareDialog crea/lista/revoca).
  const onShare = (item: DriveResource) => {
    if (!offline) setShareItem(item);
  };

  // Marca/desmarca favorito con actualización optimista (revierte si falla).
  const onToggleFavorite = useCallback(
    async (item: DriveResource) => {
      if (offline || !item.id) return;
      const id = item.id;
      const next = !favIds.has(id);
      setFavIds((prev) => {
        const s = new Set(prev);
        if (next) s.add(id);
        else s.delete(id);
        return s;
      });
      try {
        await drive.setFavorite(item, next);
      } catch {
        setFavIds((prev) => {
          const s = new Set(prev);
          if (next) s.delete(id);
          else s.add(id);
          return s;
        });
        toast.notify({ tone: 'danger', title: t('files.favorite.failed') });
      }
    },
    [offline, favIds, toast, t],
  );

  // --- Multiselección -----------------------------------------------------

  const clearSelection = useCallback(() => {
    setSelected(new Set());
    anchorRef.current = -1;
  }, []);

  // Alterna un elemento; con shift + ancla previa selecciona el rango [ancla, i].
  // El índice es el de la lista QUE SE VE (`shown`), que en búsqueda no es la de
  // la carpeta: con `items` el rango marcaba los vecinos de otra lista.
  // FORMA «MARCO», vista de lista: las carpetas salen arriba como una franja de teselas
  // de color pleno y lo demás sigue en la lista (decisión del 2026-10-05). En «Carril»,
  // en la cuadrícula y en una búsqueda —cuyos resultados vienen de todo el árbol— no se
  // parte nada. La forma se lee del eje compartido, no de un ajuste de Drive.
  const { forma } = useTheme();
  const franja = forma === 'marco' && view === 'list' && !searchMode;
  const partes = useMemo(() => partirEnTeselas(shown, franja), [shown, franja]);

  const toggleSelect = useCallback(
    (item: DriveResource, index: number, shift: boolean) => {
      if (!item.id) return;
      const id = item.id;
      setSelected((prev) => {
        const next = new Set(prev);
        if (shift && anchorRef.current >= 0) {
          // El tramo es el que se VE: con la franja de teselas, carpetas y lista son
          // dos piezas y una sola serie de posiciones (lib/teselas).
          for (const rid of idsDelRango(partes.enOrden, anchorRef.current, index)) next.add(rid);
        } else {
          if (next.has(id)) next.delete(id);
          else next.add(id);
          anchorRef.current = index;
        }
        return next;
      });
    },
    [partes.enOrden],
  );

  // Selecciona todo lo cargado (o lo deselecciona si ya está todo marcado).
  const selectableIds = useMemo(
    () => (shown ?? []).map((it) => it.id).filter((id): id is string => !!id),
    [shown],
  );
  const allSelected = selectableIds.length > 0 && selectableIds.every((id) => selected.has(id));
  const toggleSelectAll = useCallback(() => {
    setSelected(() => (allSelected ? new Set() : new Set(selectableIds)));
    anchorRef.current = -1;
  }, [allSelected, selectableIds]);

  // Selección resuelta a los elementos VISIBLES (por si el conjunto arrastra ids ya
  // no listados) y partida en ficheros/carpetas para los endpoints de lote. Se
  // resuelve contra `shown`: marcar un resultado de búsqueda no dejaba ni rastro
  // —la barra de acciones no llegaba a aparecer— porque solo se miraba la carpeta.
  const selectedItems = useMemo(
    () => (shown ?? []).filter((it) => it.id && selected.has(it.id)),
    [shown, selected],
  );
  const selectedFileIds = useMemo(
    () => selectedItems.filter((it) => !it.isDirectory).map((it) => it.id!),
    [selectedItems],
  );
  const selectedFolderIds = useMemo(
    () => selectedItems.filter((it) => it.isDirectory).map((it) => it.id!),
    [selectedItems],
  );
  const selectedFolderPaths = useMemo(
    () => new Set(selectedItems.filter((it) => it.isDirectory).map((it) => it.path)),
    [selectedItems],
  );
  const selectionCount = selectedFileIds.length + selectedFolderIds.length;

  // PDF seleccionados, en el orden en que se ven en la lista — que es el orden
  // en que se combinarán. `selected` es un Set y no conserva el orden de clic,
  // así que se toma el de la lista, que es el que el usuario tiene delante.
  // ponytail: sin reordenar antes de combinar; llegará con el visor propio, que
  // es donde tiene sentido ver las páginas para decidir.
  const selectedPdfs = useMemo(
    () => selectedItems.filter((it) => previewKind(it) === 'pdf'),
    [selectedItems],
  );
  const puedeCombinar = selectedPdfs.length >= 2 && selectedPdfs.length === selectionCount;

  // Abre el diálogo de clasificación y trae la taxonomía si aún no está. Un fallo al
  // leerla deja la lista VACÍA, no un error: el diálogo dirá que la organización todavía
  // no ha definido clasificaciones, que es lo que le sirve a quien lo abre.
  const openLabel = (item: DriveResource): void => {
    if (offline) return;
    setLabelItem(item);
    if (labelTaxonomy !== null) return;
    void drive
      .sensitivityLabels()
      .then((r) => setLabelTaxonomy(r.labels ?? []))
      .catch(() => setLabelTaxonomy([]));
  };

  const applyLabel = async (labelId: string): Promise<void> => {
    const item = labelItem;
    if (offline || !item?.id || busy) return;
    setBusy(true);
    try {
      await drive.setLabel(item.id, labelId);
      const name = labelTaxonomy?.find((l) => l.id === labelId)?.name ?? '';
      toast.notify({
        tone: 'success',
        title: labelId ? t('label.applied', { name }) : t('label.cleared'),
      });
      setLabelItem(null);
      await load(path);
    } catch {
      toast.notify({ tone: 'danger', title: t('label.failed') });
    } finally {
      setBusy(false);
    }
  };

  // Crea el documento y abre el editor en una pestaña nueva: quien estaba navegando una
  // carpeta no pierde dónde estaba. `noopener` porque es otro origen.
  const crearDoc = async (kind: 'document' | 'spreadsheet' | 'presentation'): Promise<void> => {
    if (offline || busy) return;
    setBusy(true);
    try {
      const r = await drive.newDoc(kind);
      window.open(r.href, '_blank', 'noopener');
      // Dónde quedó, porque no es donde el usuario estaba: un documento de Docs no tiene
      // carpeta. Sin este aviso, «Nuevo → Documento» parece no haber hecho nada al volver.
      toast.notify({
        tone: 'success',
        title: t('files.doc.created'),
        description: t('files.doc.createdWhere'),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('files.doc.failed') });
    } finally {
      setBusy(false);
    }
  };

  // «Nuevo → Vista de Sytue Bases»: pide el token de una conexión concedida a Sytue Drive
  // y deja el CSV en esta carpeta. El token no se guarda: refrescar es volver a exportar.
  const { preguntar: preguntarBases, dialogo: dialogoBases } = usePrompt();
  const importarDeBases = async (): Promise<void> => {
    if (offline || busy) return;
    const token = (
      await preguntarBases({
        titulo: t('files.new.bases'),
        mensaje: t('files.bases.prompt'),
        confirmLabel: t('files.bases.confirm'),
        secreto: true,
      })
    )?.trim();
    if (!token) return;
    setBusy(true);
    try {
      const r = await drive.importBases(path, token);
      toast.notify({ tone: 'success', title: t('files.bases.done', { name: r.name }) });
      await load(path);
    } catch (e) {
      const invalida = e instanceof BffError && e.status === 422;
      const sinEspacio = e instanceof BffError && e.status === 507;
      toast.notify({
        tone: 'danger',
        title: t(
          invalida
            ? 'files.bases.invalid'
            : sinEspacio
              ? 'files.bases.quota'
              : 'files.bases.failed',
        ),
      });
    } finally {
      setBusy(false);
    }
  };

  async function onCombinarPdfs(): Promise<void> {
    if (!puedeCombinar || busy) return;
    try {
      const res = await fetch('/api/pdf-pages', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'application/json' },
        body: JSON.stringify({
          op: 'combinar',
          sufijo: t('pdf.name.combinar'),
          paths: selectedPdfs.map((it) => it.path),
        }),
      });
      const cuerpo = (await res.json().catch(() => ({}))) as {
        resource?: DriveResource;
        message?: string;
      };
      if (!res.ok) {
        toast.notify({ tone: 'danger', title: cuerpo.message ?? t('pdf.failed') });
        return;
      }
      toast.notify({
        tone: 'success',
        title: t('pdf.done', { name: cuerpo.resource?.name ?? '' }),
      });
      clearSelection();
      void load(path);
    } catch {
      toast.notify({ tone: 'danger', title: t('pdf.failed') });
    }
  }

  const onBatchDownload = () => {
    if (selectionCount === 0) return;
    // Nombre de descarga: el de la carpeta si es lo único seleccionado; si no, genérico.
    const only =
      selectedItems.length === 1 && selectedItems[0]?.isDirectory
        ? selectedItems[0].name
        : undefined;
    const a = document.createElement('a');
    a.href = drive.zipHref({ fileIds: selectedFileIds, folderIds: selectedFolderIds }, only);
    a.rel = 'noopener';
    a.click();
  };

  const onBatchTrash = async () => {
    if (offline || selectionCount === 0 || busy) return;
    setBusy(true);
    try {
      await drive.batch('trash', { fileIds: selectedFileIds, folderIds: selectedFolderIds });
      toast.notify({
        tone: 'success',
        title: t('files.batch.trashDone', { count: selectionCount }),
      });
      setBatchTrashOpen(false);
      clearSelection();
      await load(path);
    } catch {
      toast.notify({ tone: 'danger', title: t('files.batch.trashFailed') });
    } finally {
      setBusy(false);
    }
  };

  const crumbs = segments(path);

  // La cuadrícula de recursos: la vista de cuadrícula entera y, en la forma «Marco», la
  // franja de carpetas sobre la lista. Una sola pieza con las mismas acciones.
  const rejilla = (elementos: DriveResource[], enFranja = false) => (
    <FileGrid
      items={elementos}
      franja={enFranja}
      onOpen={(it) => void handleOpen(it)}
      onDownload={onDownload}
      folderDownload
      showLocation={searchMode}
      onShare={onShare}
      onRename={
        offline
          ? undefined
          : (it) => {
              setPendingRename(it);
              setRenameName(it.name);
            }
      }
      onMove={offline ? undefined : setPendingMove}
      onCopy={offline ? undefined : (it) => void onCopy(it)}
      onDelete={offline ? undefined : (it) => void onDelete(it)}
      onVersions={setVersionsItem}
      onOrganizePdf={setOrganizePdfItem}
      onLabel={offline ? undefined : openLabel}
      onResumen={offline ? undefined : setResumenItem}
      favoriteIds={favIds}
      onToggleFavorite={offline ? undefined : (it) => void onToggleFavorite(it)}
      selectedIds={offline ? undefined : selected}
      selectionActive={!offline && selectionCount > 0}
      onToggleSelect={offline ? undefined : toggleSelect}
      onDragItem={offline ? undefined : onDragItem}
      onDropOnFolder={offline ? undefined : (e, d) => void onDropOnFolder(e, d)}
    />
  );

  // En la vista lista con contenido el scroll vive DENTRO del VirtualList
  // (que exige altura acotada); en el resto de estados scrollea el contenedor.
  const listActive = view === 'list' && !error && items !== null && items.length > 0;

  return (
    <div class="flex h-full flex-col">
      {/* Barra de vista: cabecera sólida con el acento del producto (a juego con
          la topbar de la shell). Desde 768 px es una fila que envuelve.
          POR DEBAJO, DOS FILAS en rejilla: migas y selector de vista arriba; buscador,
          «Nuevo» y «Subir» abajo. Fue una sola fila que se desplazaba de lado, y a
          390 px «Subir» y el selector de vista quedaban fuera del borde derecho sin
          nada que dijera que estaban ahí. El grupo de acciones es `contents` en móvil:
          sus hijos se colocan en la rejilla de aquí, con el buscador en la columna que
          crece y los dos botones a su ancho, sea cual sea el idioma. */}
      <div
        class={cx(
          'bg-surface text-text border-border flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5 sm:px-6',
          'max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto_auto] max-md:gap-x-2',
        )}
      >
        {/* El rótulo de la carpeta ya está en el breadcrumb, pero ahí es un BOTÓN
            de navegación: la vista se quedaba sin encabezado y quien navega por
            encabezados no encontraba el título de la página. */}
        <h1 class="sr-only">{crumbs.at(-1)?.name ?? t('nav.myDrive')}</h1>
        {/* grow (basis auto), no flex-1 (basis 0): con basis 0 el breadcrumb se
            encoge a cero y nunca se produce el salto de línea en móvil. */}
        <nav
          aria-label={t('files.breadcrumb')}
          class="flex min-w-0 grow items-center gap-0.5 text-base max-md:col-span-2"
        >
          <button
            type="button"
            onClick={() => goTo('/')}
            aria-current={path === '/' ? 'page' : undefined}
            class={cx(
              'hover:bg-surface-sunken active:bg-surface-sunken sytue-focus-ring rounded-control px-2 py-1 whitespace-nowrap transition-colors duration-150',
              path === '/'
                ? 'text-text font-semibold'
                : 'text-text-muted hover:text-text font-medium',
            )}
          >
            {t('nav.myDrive')}
          </button>
          {crumbs.map((c) => (
            <span key={c.path} class="flex min-w-0 items-center gap-0.5">
              <Icon name="chevronRight" size={14} class="text-text/50 shrink-0" />
              <button
                type="button"
                onClick={() => goTo(c.path)}
                aria-current={c.path === path ? 'page' : undefined}
                title={c.name}
                class={cx(
                  'hover:bg-surface-sunken active:bg-surface-sunken sytue-focus-ring rounded-control truncate px-2 py-1 transition-colors duration-150',
                  c.path === path
                    ? 'text-text font-semibold'
                    : 'text-text-muted hover:text-text font-medium',
                )}
              >
                {c.name}
              </button>
            </span>
          ))}
        </nav>

        {/* En móvil no pinta caja (`contents`): sus hijos son celdas de la rejilla de
            arriba. Desde 768 px, el grupo de siempre, a la derecha. */}
        <div class="ml-auto flex flex-wrap items-center justify-end gap-2 max-md:contents">
          {/* Buscar en Drive. El input va en el idioma del header (velo translúcido),
              no con la variante clara de Input, que no lee sobre el cromo grafito.
              La LUPA no es adorno: es lo que identifica la caja como buscador antes de
              leer el placeholder. La exige el DS (design.md:639) y la lleva el buscador
              de la shell, que vive en la misma topbar unos píxeles más arriba — sin
              ella, dos buscadores contiguos se veían distintos.
              En móvil ocupa la columna que crece de la segunda fila: lo que dejen
              «Nuevo» y «Subir», que miden lo que su rótulo. */}
          <span class="relative inline-flex items-center max-md:min-w-0">
            <Icon
              name="search"
              size={16}
              class="text-text-muted pointer-events-none absolute left-2.5"
            />
            <input
              type="search"
              value={query}
              onInput={(e) => setQuery(e.currentTarget.value)}
              placeholder={t('files.search')}
              aria-label={t('files.searchAria')}
              class="text-text placeholder:text-text-muted hover:bg-surface-sunken focus:bg-surface-sunken border-border-strong sytue-focus-ring h-8 w-full rounded-md border bg-transparent pr-2.5 pl-8 text-sm transition-colors duration-150 md:w-56"
            />
          </span>
          {/* Controles sobre la cabecera sólida: las variantes de Button asumen
              superficie clara y no leen sobre el cromo grafito; aquí van en el idioma
              del header —hundido (bg-surface-sunken) y hairline de tinta—,
              como en calendar. */}
          {/* «Nuevo» agrupa crear carpeta y los tres tipos de Sytue Docs. Antes solo
              había «Nueva carpeta»: para empezar un documento había que salir a Docs,
              aunque el servicio sabe crearlo desde el principio. */}
          <Dropdown
            label={t('files.new')}
            align="end"
            trigger={({ onClick, ...aria }) => (
              <button
                type="button"
                onClick={onClick}
                disabled={busy || offline}
                class="text-text hover:bg-surface-sunken active:bg-surface-sunken border-border-strong sytue-focus-ring rounded-control inline-flex h-8 items-center gap-1.5 border px-3 text-sm font-semibold transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50"
                {...aria}
              >
                {t('files.new')}
                <Icon name="chevronDown" size={14} />
              </button>
            )}
          >
            <MenuItem
              icon={<Glyph name="folder" size={16} />}
              onClick={() => setNewFolderOpen(true)}
            >
              {t('files.new.folder')}
            </MenuItem>
            <MenuSeparator />
            <MenuItem
              icon={<Glyph name="document" size={16} />}
              onClick={() => void crearDoc('document')}
            >
              {t('files.new.document')}
            </MenuItem>
            <MenuItem
              icon={<Glyph name="spreadsheet" size={16} />}
              onClick={() => void crearDoc('spreadsheet')}
            >
              {t('files.new.spreadsheet')}
            </MenuItem>
            <MenuItem
              icon={<Glyph name="presentation" size={16} />}
              onClick={() => void crearDoc('presentation')}
            >
              {t('files.new.presentation')}
            </MenuItem>
            <MenuSeparator />
            <MenuItem
              icon={<Glyph name="spreadsheet" size={16} />}
              onClick={() => void importarDeBases()}
            >
              {t('files.new.bases')}
            </MenuItem>
          </Dropdown>
          <Button
            variant="primary"
            size="sm"
            onClick={onUploadClick}
            disabled={busy || offline}
            loading={uploading}
            leadingIcon={<Glyph name="upload" size={16} />}
          >
            {t('files.upload')}
          </Button>
          <input
            ref={fileInput}
            type="file"
            multiple
            class="hidden"
            onChange={(e) => void onFilesSelected(e)}
            aria-hidden="true"
            tabIndex={-1}
          />
          {/* Selector de vista, también en el idioma del header: los tokens de
              superficie clara (text-muted sobre el cromo grafito) quedaban en
              2,2:1. Activo = latón sobre cromo (5,1:1), el acento de la casa. */}
          {/* En móvil sube a la primera fila, a la derecha de las migas. */}
          <div class="divide-border border-border-strong flex items-center divide-x overflow-hidden rounded-md border max-md:col-start-3 max-md:row-start-1 max-md:justify-self-end md:ml-1">
            <button
              type="button"
              aria-label={t('files.viewList')}
              title={t('files.viewList')}
              aria-pressed={view === 'list'}
              onClick={() => setView('list')}
              class={cx(
                'sytue-focus-ring flex h-8 w-8 items-center justify-center transition-colors duration-150',
                view === 'list'
                  ? 'bg-surface-sunken text-primary-text'
                  : 'text-text-muted hover:bg-surface-sunken hover:text-text',
              )}
            >
              <Icon name="menu" size={16} />
            </button>
            <button
              type="button"
              aria-label={t('files.viewGrid')}
              title={t('files.viewGrid')}
              aria-pressed={view === 'grid'}
              onClick={() => setView('grid')}
              class={cx(
                'sytue-focus-ring flex h-8 w-8 items-center justify-center transition-colors duration-150',
                view === 'grid'
                  ? 'bg-surface-sunken text-primary-text'
                  : 'text-text-muted hover:bg-surface-sunken hover:text-text',
              )}
            >
              <Icon name="grid" size={16} />
            </button>
          </div>
        </div>
      </div>

      {/* Barra de acciones en lote (contextual: solo con selección). Sobria, sobre
          superficie tenue; anuncia el conteo por aria-live. */}
      {!offline && selectionCount > 0 ? (
        <BulkActionBar
          count={selectionCount}
          countLabel={t('files.selection.count', { count: selectionCount })}
          label={t('files.selection.region')}
          onClear={clearSelection}
          class="px-4 sm:px-6"
        >
          <div class="ml-auto flex flex-wrap items-center gap-2">
            {/* Combinar: solo si TODO lo seleccionado son PDF y hay dos o más.
                Con una carpeta o un .docx en la selección la acción no tendría
                sentido, y ofrecerla para fallar después es peor que ocultarla. */}
            {puedeCombinar ? (
              <Button
                variant="secondary"
                size="sm"
                leadingIcon={<Glyph name="copy" size={16} />}
                disabled={busy}
                title={t('pdf.combineOrder')}
                onClick={() => void onCombinarPdfs()}
              >
                {t('pdf.combine')}
              </Button>
            ) : null}
            <Button
              variant="secondary"
              size="sm"
              leadingIcon={<Glyph name="folderMove" size={16} />}
              disabled={busy}
              onClick={() => setBatchMoveOpen(true)}
            >
              {t('files.selection.move')}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              leadingIcon={<Glyph name="download" size={16} />}
              disabled={busy}
              onClick={onBatchDownload}
            >
              {t('files.selection.download')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              leadingIcon={<Glyph name="trash" size={16} />}
              disabled={busy}
              onClick={() => setBatchTrashOpen(true)}
            >
              {t('files.selection.trash')}
            </Button>
          </div>
        </BulkActionBar>
      ) : null}

      {uploadProgress ? (
        <div
          class="border-border bg-bg-subtle border-b px-4 py-2 sm:px-6"
          role="status"
          aria-live="polite"
          aria-label={t('files.uploadProgress.aria', {
            current: uploadProgress.current,
            total: uploadProgress.total,
          })}
        >
          <div class="text-text flex items-center gap-3 text-sm">
            <Spinner size={16} />
            <span class="min-w-0 truncate">
              {t('files.uploadProgress.text', {
                current: uploadProgress.current,
                total: uploadProgress.total,
                name: uploadProgress.name,
              })}
            </span>
          </div>
          <div class="bg-border mt-1.5 h-1 overflow-hidden rounded-full" aria-hidden="true">
            <div
              class="bg-primary h-full transition-[width] duration-150"
              style={{
                width: `${((uploadProgress.current + (uploadProgress.fraction ?? 0)) / uploadProgress.total) * 100}%`,
              }}
            />
          </div>
        </div>
      ) : null}

      {/* Contenido. Zona de arrastrar-soltar: todo el lienzo (también con la carpeta
          vacía) acepta ficheros del SO; el botón «Subir» sigue disponible (el drop no
          es el único camino). Los handlers ignoran drags que no traen ficheros. */}
      <div
        class={cx('relative min-h-0 flex-1', listActive ? 'flex flex-col' : 'overflow-y-auto')}
        onDragEnter={onDragEnter}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={(e) => void onDrop(e)}
      >
        {/* Overlay sobrio al arrastrar (decorativo: el progreso lo anuncia el aria-live
            de la barra de subida). pointer-events-none: el drop llega al contenedor. */}
        {dragging ? (
          <div
            class="border-primary bg-bg/90 pointer-events-none absolute inset-2 z-20 grid place-items-center rounded-xl border-2 border-dashed"
            aria-hidden="true"
          >
            <div class="text-text flex flex-col items-center gap-2">
              {/* El icono va SUELTO, sin tesela de fondo, por lo mismo que lo explica
                  EmptyState: tres rellenos apilados son la firma del estado vacío
                  genérico. Y sin `backdrop-blur`, que a 1 px no se ve y costaba una
                  capa de compositor sobre el listado entero mientras se arrastra. */}
              <span class="text-primary-text grid h-12 w-12 place-items-center">
                <Glyph name="upload" size={24} />
              </span>
              <span class="text-base font-semibold">{t('files.dropHere')}</span>
            </div>
          </div>
        ) : null}
        {shown === null && !error ? (
          view === 'grid' ? (
            <FileGridSkeleton />
          ) : (
            <FileListSkeleton />
          )
        ) : error && !searchMode ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(error === 'session' ? 'files.error.sessionTitle' : 'files.error.loadTitle')}
              description={t(
                error === 'session' ? 'files.error.sessionBody' : 'files.error.network',
              )}
              // Icono inline (no <img>): las ilustraciones SVG usan var(--sytue-*),
              // que no se resuelve dentro de un documento <img> externo.
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                error === 'session' ? (
                  // Recargar basta: el SSR sin sesión redirige a /auth/login.
                  <Button size="sm" onClick={() => location.reload()}>
                    {t('files.error.sessionCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load(path)}>
                    {t('files.retry')}
                  </Button>
                )
              }
            />
          </div>
        ) : shown && shown.length === 0 ? (
          <div class="grid h-full place-items-center p-6">
            {searchMode ? (
              searchFailed ? (
                <EmptyState
                  title={t('files.search.errorTitle')}
                  description={t('files.error.network')}
                  illustration={<Icon name="warning" size={40} class="text-danger-text" />}
                  action={
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => void runSearch(query.trim())}
                    >
                      {t('files.retry')}
                    </Button>
                  }
                />
              ) : (
                <EmptyState
                  title={t('files.search.emptyTitle', { query: query.trim() })}
                  description={t('files.search.emptyBody')}
                  illustration={<Illustration name="emptySearch" />}
                />
              )
            ) : (
              <EmptyState
                title={t('files.empty.title')}
                description={t('files.empty.body')}
                illustration={<Illustration name="emptyDrive" />}
                action={
                  <Button
                    size="sm"
                    leadingIcon={<Glyph name="upload" size={16} />}
                    onClick={onUploadClick}
                    disabled={offline}
                  >
                    {t('files.empty.cta')}
                  </Button>
                }
              />
            )}
          </div>
        ) : view === 'list' ? (
          <>
            {partes.carpetas.length > 0 ? (
              // Con muchas carpetas la franja desplaza por dentro: la lista conserva
              // siempre más de la mitad del alto.
              <div class="max-h-[45%] shrink-0 overflow-y-auto px-6 pt-4 pb-1">
                {rejilla(partes.carpetas, true)}
              </div>
            ) : null}
            <FileList
              items={partes.resto}
              // Los resultados de búsqueda no paginan: el servicio acota a 200 y la lista
              // completa llega de una vez.
              hasMore={!searchMode && nextCursor !== null}
              loadingMore={loadingMore}
              onLoadMore={() => void loadMore()}
              onOpen={(it) => void handleOpen(it)}
              onDownload={onDownload}
              folderDownload
              showLocation={searchMode}
              onShare={onShare}
              onRename={
                offline
                  ? undefined
                  : (it) => {
                      setPendingRename(it);
                      setRenameName(it.name);
                    }
              }
              onMove={offline ? undefined : setPendingMove}
              onCopy={offline ? undefined : (it) => void onCopy(it)}
              onDelete={offline ? undefined : (it) => void onDelete(it)}
              onVersions={setVersionsItem}
              onOrganizePdf={setOrganizePdfItem}
              onLabel={offline ? undefined : openLabel}
              onResumen={offline ? undefined : setResumenItem}
              favoriteIds={favIds}
              onToggleFavorite={offline ? undefined : (it) => void onToggleFavorite(it)}
              selectedIds={offline ? undefined : selected}
              selectionActive={!offline && selectionCount > 0}
              // La lista empieza DESPUÉS de la franja en la serie de posiciones.
              onToggleSelect={
                offline
                  ? undefined
                  : (it, i, shift) => toggleSelect(it, posicionEnLista(partes, i), shift)
              }
              allSelected={allSelected}
              someSelected={selectionCount > 0 && !allSelected}
              onToggleAll={toggleSelectAll}
              onDragItem={offline ? undefined : onDragItem}
              onDropOnFolder={offline ? undefined : (e, d) => void onDropOnFolder(e, d)}
            />
          </>
        ) : (
          <div class="px-6 py-4">
            {rejilla(shown!)}
            {/* La rejilla no se virtualiza (columnas fluidas por breakpoint):
                se pagina con un «Cargar más» explícito. La lista sí virtualiza
                y encadena páginas al hacer scroll. */}
            {!searchMode && nextCursor !== null ? (
              <div class="flex justify-center pt-5">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={loadingMore}
                  onClick={() => void loadMore()}
                >
                  {t('files.loadMore')}
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </div>

      {/* Modal nueva carpeta */}
      <Modal
        open={newFolderOpen}
        onClose={() => setNewFolderOpen(false)}
        title={t('files.newFolder.title')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setNewFolderOpen(false)}>
              {t('files.cancel')}
            </Button>
            <Button
              onClick={() => void onCreateFolder()}
              loading={creating}
              disabled={!folderName.trim()}
            >
              {t('files.create')}
            </Button>
          </>
        }
      >
        <FieldGroup label={t('files.newFolder.label')} htmlFor="folder-name">
          <Input
            id="folder-name"
            value={folderName}
            placeholder={t('files.newFolder.placeholder')}
            onInput={(e) => setFolderName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onCreateFolder();
            }}
          />
        </FieldGroup>
      </Modal>

      {/* Modal renombrar (mismo patrón que Nueva carpeta: Input + Enter envía) */}
      <Modal
        open={pendingRename !== null}
        onClose={() => setPendingRename(null)}
        title={t(
          pendingRename?.isDirectory ? 'files.rename.folderTitle' : 'files.rename.fileTitle',
        )}
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingRename(null)}>
              {t('files.cancel')}
            </Button>
            <Button
              onClick={() => void onRename()}
              loading={renaming}
              disabled={!renameName.trim()}
            >
              {t('files.rename.cta')}
            </Button>
          </>
        }
      >
        <FieldGroup label={t('files.rename.label')} htmlFor="rename-name">
          <Input
            id="rename-name"
            value={renameName}
            placeholder={pendingRename?.name}
            onInput={(e) => setRenameName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onRename();
            }}
          />
        </FieldGroup>
      </Modal>

      {/* Diálogo «Mover a…»: picker que navega el árbol de carpetas del BFF.
          key: al cambiar de recurso el picker nace limpio (destino = raíz). */}
      {pendingMove ? (
        <MoveDialog
          key={pendingMove.path}
          item={pendingMove}
          onClose={() => setPendingMove(null)}
          onMoved={() => {
            setPendingMove(null);
            void load(path);
          }}
        />
      ) : null}

      {/* Mover la SELECCIÓN a otra carpeta (mismo picker por árbol; excluye las
          carpetas seleccionadas como destino). */}
      {dialogoBases}

      {batchMoveOpen ? (
        <BatchMoveDialog
          fileIds={selectedFileIds}
          folderIds={selectedFolderIds}
          excludePaths={selectedFolderPaths}
          currentPath={path}
          count={selectionCount}
          onClose={() => setBatchMoveOpen(false)}
          onMoved={() => {
            setBatchMoveOpen(false);
            clearSelection();
            void load(path);
          }}
        />
      ) : null}

      {/* Confirmación de «Enviar selección a la papelera» (acción reversible desde
          la papelera, pero masiva: se confirma). */}
      <Modal
        open={batchTrashOpen}
        onClose={() => setBatchTrashOpen(false)}
        title={t('files.batch.trashTitle')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setBatchTrashOpen(false)} disabled={busy}>
              {t('files.cancel')}
            </Button>
            <Button variant="danger" loading={busy} onClick={() => void onBatchTrash()}>
              {t('files.batch.trashCta')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base">
          {t('files.batch.trashBody', { count: selectionCount })}
        </p>
      </Modal>

      {/* Modal de compartir: crear enlace (permiso + caducidad) y gestionar los
          vivos (listar/revocar). key: al cambiar de recurso nace limpio. */}
      {shareItem ? (
        <ShareDialog key={shareItem.path} item={shareItem} onClose={() => setShareItem(null)} />
      ) : null}

      {/* Visor inline (imagen/PDF/texto) con «Descargar» dentro. */}
      {previewItem ? (
        <PreviewDialog
          key={previewItem.path}
          item={previewItem}
          onClose={() => setPreviewItem(null)}
          onDownload={onDownload}
        />
      ) : null}

      {/* Clasificación de sensibilidad. Cada opción dice lo que IMPLICA —una etiqueta
          que bloquea el enlace público no es lo mismo que una informativa— porque el
          efecto no se deduce del nombre y quien clasifica tiene que saberlo ANTES. */}
      {resumenItem?.path ? (
        <ResumenIA
          t={t}
          path={resumenItem.path}
          nombre={resumenItem.name}
          onClose={() => setResumenItem(null)}
        />
      ) : null}
      {labelItem ? (
        <Modal open onClose={() => setLabelItem(null)} title={t('label.title')} size="sm">
          <p class="text-text-muted mb-3 truncate text-sm">{labelItem.name}</p>
          {labelTaxonomy === null ? (
            <div class="flex justify-center py-6">
              <Spinner size={20} />
            </div>
          ) : labelTaxonomy.length === 0 ? (
            <EmptyState
              title={t('label.emptyTitle')}
              description={t('label.emptyBody')}
              illustration={<Icon name="lock" size={28} />}
            />
          ) : (
            <div class="flex flex-col gap-1">
              {labelTaxonomy.map((l) => (
                <button
                  key={l.id}
                  type="button"
                  role="menuitemradio"
                  aria-checked={labelItem.label?.name === l.name}
                  disabled={busy}
                  onClick={() => void applyLabel(l.id)}
                  class="hover:bg-bg-subtle focus-visible:bg-bg-subtle flex w-full items-start gap-2.5 rounded-md px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none disabled:opacity-60"
                >
                  <span class="mt-0.5 w-4 shrink-0">
                    {labelItem.label?.name === l.name ? (
                      <Icon name="check" size={14} class="text-primary-text" />
                    ) : null}
                  </span>
                  <span class="min-w-0">
                    <span class="text-text block truncate text-sm font-medium">{l.name}</span>
                    <span class="text-text-muted block text-xs">
                      {t(l.block_external_share === true ? 'label.blocks' : 'label.informational')}
                    </span>
                  </span>
                </button>
              ))}
              {labelItem.label ? (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void applyLabel('')}
                  class="text-danger-text hover:bg-bg-subtle focus-visible:bg-bg-subtle rounded-control mt-1 px-3 py-2 text-left text-sm font-medium transition-colors duration-150 focus-visible:outline-none disabled:opacity-60 pointer-coarse:min-h-[max(44px,2.75rem)]"
                >
                  {t('label.remove')}
                </button>
              ) : null}
            </div>
          )}
        </Modal>
      ) : null}

      {/* Historial de versiones: lista, descarga y restauración no destructiva.
          key: al cambiar de fichero el panel nace limpio. Al restaurar, el
          explorador recarga la carpeta (el contenido actual cambió). */}
      {versionsItem ? (
        <VersionsDialog
          key={versionsItem.id ?? versionsItem.path}
          item={versionsItem}
          onClose={() => setVersionsItem(null)}
          onRestored={() => {
            setVersionsItem(null);
            void load(path);
          }}
        />
      ) : null}

      {/* Organizar páginas de un PDF. Al crear el fichero nuevo se recarga la
          carpeta: el resultado se guarda AHÍ, y no verlo aparecer haría dudar de
          si la operación funcionó. */}
      {organizePdfItem ? (
        <OrganizePdfDialog
          key={organizePdfItem.id ?? organizePdfItem.path}
          item={organizePdfItem}
          onClose={() => setOrganizePdfItem(null)}
          onCreated={() => void load(path)}
        />
      ) : null}
    </div>
  );
}

interface RowActions {
  items: DriveResource[];
  // Mover arrastrando. Ausentes = sin gesto (offline, papelera, resultados de
  // búsqueda: allí mover no tendría a dónde).
  onDragItem?: ((e: DragEvent, item: DriveResource) => void) | undefined;
  onDropOnFolder?: ((e: DragEvent, destino: string) => void) | undefined;
  onOpen: (item: DriveResource) => void;
  onDownload: (item: DriveResource) => void;
  onShare?: ((item: DriveResource) => void) | undefined;
  onRename?: ((item: DriveResource) => void) | undefined;
  onMove?: ((item: DriveResource) => void) | undefined;
  // Copiar (opcional): «Recientes»/«Favoritos» no la ofrecen (viven en «Mi unidad»).
  onCopy?: ((item: DriveResource) => void) | undefined;
  onDelete?: ((item: DriveResource) => void) | undefined;
  // Historial de versiones (opcional): solo ficheros con id. `| undefined` explícito
  // por exactOptionalPropertyTypes al reenviar el prop por los componentes intermedios.
  onVersions?: ((item: DriveResource) => void) | undefined;
  onOrganizePdf?: ((item: DriveResource) => void) | undefined;
  onLabel?: ((item: DriveResource) => void) | undefined;
  /** Abrir el resumen del fichero; sin él, el menú no lo ofrece. */
  onResumen?: ((item: DriveResource) => void) | undefined;
  // Favoritos (opcional): estado por-id + toggle. Ausente = sin acción de estrella.
  // `| undefined` explícito: exactOptionalPropertyTypes exige aceptar el paso de
  // `undefined` al reenviar el prop por los componentes intermedios.
  favoriteIds?: ReadonlySet<string> | undefined;
  onToggleFavorite?: ((item: DriveResource) => void) | undefined;
  // Descargar carpetas como ZIP (solo «Mi unidad»; en «Recientes»/«Favoritos» las
  // carpetas no se descargan). Ausente = sin «Descargar» para carpetas.
  folderDownload?: boolean | undefined;
  // Enseñar en qué carpeta vive cada elemento. Se enciende en los resultados de
  // búsqueda, que cruzan todo el árbol: sin esto, dos «informe.pdf» de carpetas
  // distintas son la misma fila y hay que abrirlos para saber cuál es cuál.
  showLocation?: boolean | undefined;
  // Multiselección (opcional): estado por-id + toggle con índice (para rangos con
  // shift) + si hay selección activa (para revelar las casillas). Ausente = sin casillas.
  selectedIds?: ReadonlySet<string> | undefined;
  selectionActive?: boolean | undefined;
  onToggleSelect?: ((item: DriveResource, index: number, shift: boolean) => void) | undefined;
}

function FileIcon({ resource, size = 36 }: { resource: DriveResource; size?: number }) {
  // Tile cuadrado por tipo (estilo Drive/Dropbox), todo por roles de tema —nunca
  // HEX crudos—: carpeta con tinte de marca; archivo con distintivo de extension
  // (o glyph de lineas si no tiene) sobre tile neutro. El tamaño escala el glyph.
  const box = { width: `${size}px`, height: `${size}px` };
  if (resource.isDirectory) {
    return (
      <span
        class="bg-selected text-primary-text grid shrink-0 place-items-center rounded-md"
        style={box}
        aria-hidden="true"
      >
        <Glyph name="folder" size={Math.round(size * 0.5)} />
      </span>
    );
  }
  const ext = fileExt(resource.name);
  return (
    <span
      class="bg-bg-subtle grid shrink-0 place-items-center rounded-md"
      style={box}
      aria-hidden="true"
    >
      {ext ? (
        <span
          class={cx(
            'leading-none font-bold tracking-wide uppercase',
            KIND_TINT[ext] ?? 'text-text-muted',
          )}
          style={{ fontSize: `${Math.max(9, Math.round(size * 0.26))}px` }}
        >
          {ext.slice(0, 4)}
        </span>
      ) : (
        <Glyph name="file" size={Math.round(size * 0.5)} class="text-text-subtle" />
      )}
    </span>
  );
}

/**
 * Vista previa del tile de rejilla: miniatura server-side (encuadrada, object-cover,
 * sin distorsión) para imágenes ráster; si no hay miniatura, no es imagen, o la
 * carga falla, cae al icono de tipo. Aspecto fijo 4/3 en AMBOS casos para que la
 * rejilla no se descuadre al mezclar miniaturas e iconos. La miniatura la sirve el
 * BFF (/api/thumbnail → servicio Go) con la cookie de sesión; el token nunca toca el
 * hilo. lazy: solo se piden las visibles.
 */
function GridPreview({ item, t }: { item: DriveResource; t: T }) {
  const [failed, setFailed] = useState(false);
  if (item.id && thumbable(item) && !failed) {
    return (
      <div class="bg-surface-sunken aspect-[4/3] w-full overflow-hidden">
        <img
          src={withBase(
            import.meta.env.BASE_URL,
            `/api/thumbnail?id=${encodeURIComponent(item.id)}`,
          )}
          alt={t('files.thumbnailAlt', { name: item.name })}
          loading="lazy"
          decoding="async"
          class="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      </div>
    );
  }
  return (
    <div class="bg-surface-sunken grid aspect-[4/3] w-full place-items-center">
      <FileIcon resource={item} size={48} />
    </div>
  );
}

function ActionsMenu({
  item,
  onDownload,
  onShare,
  onRename,
  onMove,
  onCopy,
  onDelete,
  onVersions,
  onOrganizePdf,
  onLabel,
  onResumen,
  favoriteIds,
  onToggleFavorite,
  folderDownload,
}: {
  item: DriveResource;
  onDownload: (item: DriveResource) => void;
  onShare?: ((item: DriveResource) => void) | undefined;
  // Renombrar/Mover/Eliminar son opcionales: «Recientes»/«Favoritos» reutilizan
  // este menú sin esas acciones (viven en «Mi unidad», con sus modales).
  // `| undefined` explícito por exactOptionalPropertyTypes (paso de props).
  onRename?: ((item: DriveResource) => void) | undefined;
  onMove?: ((item: DriveResource) => void) | undefined;
  onCopy?: ((item: DriveResource) => void) | undefined;
  onDelete?: ((item: DriveResource) => void) | undefined;
  onVersions?: ((item: DriveResource) => void) | undefined;
  onOrganizePdf?: ((item: DriveResource) => void) | undefined;
  onLabel?: ((item: DriveResource) => void) | undefined;
  /** Abrir el resumen del fichero; sin él, el menú no lo ofrece. */
  onResumen?: ((item: DriveResource) => void) | undefined;
  favoriteIds?: ReadonlySet<string> | undefined;
  onToggleFavorite?: ((item: DriveResource) => void) | undefined;
  folderDownload?: boolean | undefined;
}) {
  const toast = useToast();
  const { t } = useDriveT();
  const offline = useConnectivity() === 'offline';
  const [sharing, setSharing] = useState(false);

  // «Abrir en el editor» (WOPI): solo si el editor configurado declara saber abrir ESTA
  // extensión. Ofrecerlo siempre llevaría a una pestaña en blanco, que es peor que no
  // ofrecerlo: parece que ha funcionado.
  const [editable, setEditable] = useState(false);
  useEffect(() => {
    if (item.isDirectory || !item.id) return;
    const ext = extensionOf(item.name);
    if (!ext) return;
    let vivo = true;
    void wopiExtensions().then((exts) => {
      if (vivo) setEditable(exts.has(ext));
    });
    return () => {
      vivo = false;
    };
  }, [item.id, item.name, item.isDirectory]);

  async function abrirEnEditor(): Promise<void> {
    if (!item.id) return;
    if (await openInEditor(item.id)) return;
    toast.notify({ tone: 'danger', title: t('action.editorFailed') });
  }

  // «Abrir en la aplicación»: salta a la app instalada mostrando ESTE fichero, igual que
  // Office Online. Si el sistema no se lleva el foco se OFRECE la descarga, no se afirma
  // que falte: ningún navegador dice si una app está instalada —sería huella digital—,
  // así que la detección es un indicio.
  async function abrirEnApp(): Promise<void> {
    if (await openInApp(resourceHref(item))) return;
    toast.notify({
      tone: 'info',
      title: t('action.desktopTitle'),
      description: t('action.desktopBody'),
      action: {
        label: t('action.desktopCta'),
        onClick: () => window.open('https://sytue.eu/descargas', '_blank', 'noopener'),
      },
    });
  }

  // «Adjuntar a un correo» (intent attach-drive-to-mail): el Hub compone el
  // deep-link al compositor de Sytue Mail con este fichero ya adjunto POR
  // REFERENCIA —viaja el enlace canónico, no los bytes—, así que adjuntar algo de
  // 2 GB cuesta lo mismo que adjuntar un PDF y no choca con el límite del correo.
  //
  // Solo ficheros: una carpeta no se adjunta, y ofrecerlo sobre una carpeta sería
  // una acción que falla siempre.
  async function attachToMail(): Promise<void> {
    if (!item.id || sharing) return;
    setSharing(true);
    try {
      const res = await fetch('/api/intent', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'application/json' },
        body: JSON.stringify({
          type: 'attach-drive-to-mail',
          ids: [item.id],
          params: { subject: item.name },
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const result = (await res.json()) as { url?: string };
      if (!result.url) throw new Error('sin deep-link');
      window.location.assign(result.url);
    } catch {
      toast.notify({ tone: 'danger', title: t('action.mailFailed') });
    } finally {
      setSharing(false);
    }
  }

  // «Compartir a Rooms» (intent share-to-chat): el Hub compone el deep-link a
  // Sytue Rooms para compartir este recurso en un canal/DM. Se pasa el enlace
  // canónico del recurso ({origen}/files/{id}, la misma convención que la búsqueda
  // federada) en `text`; Rooms lo publica al elegir el destino. La autorización la
  // aplica Chat (no se puede publicar en un canal donde no estás): aquí solo se
  // compone y navega.
  async function shareToRooms(): Promise<void> {
    if (!item.id || sharing) return;
    setSharing(true);
    try {
      // Con el prefijo: `drive.sytue.eu/files/…` es el 404 de Traefik. Una carpeta va
      // por su ruta, como en la app nativa (`enlaceDe`): `/files/{id}` es de ficheros.
      const link = item.isDirectory ? driveFolderUrl(item.path) : driveFileUrl(item.id);
      const res = await fetch('/api/intent', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'application/json' },
        body: JSON.stringify({
          type: 'share-to-chat',
          params: { source: 'drive', id: item.id, text: `${item.name}\n${link}` },
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const result = (await res.json()) as { url?: string };
      if (!result.url) throw new Error('sin deep-link');
      // Navega a Sytue Rooms, que abre el selector de canal y publica el recurso.
      window.location.assign(result.url);
    } catch {
      toast.notify({ tone: 'danger', title: t('action.roomsFailed') });
      setSharing(false);
    }
  }

  // Contenedor presentacional: solo detiene la propagacion para que abrir el
  // menu no active la navegacion de la fila. El control real es el <button>
  // interno del Dropdown, que si es accesible por teclado.
  return (
    <span
      role="presentation"
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
    >
      <Dropdown
        label={t('action.menu', { name: item.name })}
        trigger={({ onClick, ...aria }) => (
          <button
            type="button"
            onClick={onClick}
            aria-label={t('action.menu', { name: item.name })}
            title={t('action.more')}
            class="text-text-muted hover:bg-bg-subtle hover:text-text active:bg-surface-sunken sytue-focus-ring rounded-control flex h-8 w-8 items-center justify-center transition-colors duration-150"
            {...aria}
          >
            <Glyph name="more" size={16} />
          </button>
        )}
      >
        {!item.isDirectory ? (
          <MenuItem icon={<Glyph name="download" size={16} />} onClick={() => onDownload(item)}>
            {t('action.download')}
          </MenuItem>
        ) : folderDownload && item.id && !offline ? (
          <MenuItem icon={<Glyph name="download" size={16} />} onClick={() => onDownload(item)}>
            {t('action.downloadZip')}
          </MenuItem>
        ) : null}
        {!offline && onRename ? (
          <MenuItem icon={<Glyph name="pencil" size={16} />} onClick={() => onRename(item)}>
            {t('action.rename')}
          </MenuItem>
        ) : null}
        {!offline && onMove ? (
          <MenuItem icon={<Glyph name="folderMove" size={16} />} onClick={() => onMove(item)}>
            {t('action.move')}
          </MenuItem>
        ) : null}
        {!offline && onCopy ? (
          <MenuItem icon={<Glyph name="copy" size={16} />} onClick={() => onCopy(item)}>
            {t('action.copy')}
          </MenuItem>
        ) : null}
        {!offline && onShare ? (
          <MenuItem icon={<Glyph name="link" size={16} />} onClick={() => onShare(item)}>
            {t('action.shareLink')}
          </MenuItem>
        ) : null}
        {/* Dentro de la app nativa sobra: ya estás en ella. */}
        {!inNativeShell() ? (
          <MenuItem icon={<Icon name="device" size={16} />} onClick={() => void abrirEnApp()}>
            {t('action.openInApp')}
          </MenuItem>
        ) : null}
        {/* Drive habla WOPI como host desde la migración 0016, pero no había desde dónde
            usarlo: el protocolo estaba entero y sin puerta. */}
        {!offline && editable ? (
          <MenuItem icon={<Glyph name="pencil" size={16} />} onClick={() => void abrirEnEditor()}>
            {t('action.openInEditor')}
          </MenuItem>
        ) : null}
        {!offline && !item.isDirectory && previewKind(item) === 'office' ? (
          <MenuItem
            icon={<Glyph name="pencil" size={16} />}
            onClick={() => void abrirEnLaSuite(item, 'office')}
          >
            {extensionOf(item.name) === 'odp' || extensionOf(item.name) === 'pptx'
              ? 'Abrir en Diapos'
              : extensionOf(item.name) === 'ods' ||
                  extensionOf(item.name) === 'xlsx' ||
                  extensionOf(item.name) === 'csv'
                ? 'Abrir en Calcus'
                : 'Abrir en Docus'}
          </MenuItem>
        ) : null}
        {!item.isDirectory && item.id && onVersions ? (
          <MenuItem icon={<Glyph name="clock" size={16} />} onClick={() => onVersions(item)}>
            {t('action.versions')}
          </MenuItem>
        ) : null}
        {/* Organizar páginas: SOLO en PDF. Ofrecerlo en cualquier fichero llevaría
            a un error del servicio tras rellenar el formulario, que es peor que
            no ofrecerlo — parece que se podía y no se podía. */}
        {!offline && onOrganizePdf && previewKind(item) === 'pdf' ? (
          <MenuItem icon={<Glyph name="copy" size={16} />} onClick={() => onOrganizePdf(item)}>
            {t('pdf.organize')}
          </MenuItem>
        ) : null}
        {/* Abrir en Sytue PDF. Va PRIMERO de las de PDF y con el verbo del
            producto, no «previsualizar»: ahí se ve, se anota, se rellena y se
            firma, que es casi todo lo que se hace con un PDF. Organizar páginas
            se queda porque es lo único que también tiene sentido sin abrirlo.
            El enlace lo compone el servidor (/api/abrir-pdf): la URL pública de
            Docs es configuración del despliegue y no tiene por qué bajar al
            navegador. */}
        {!offline && !item.isDirectory && previewKind(item) === 'pdf' ? (
          <MenuItem
            icon={<Glyph name="pencil" size={16} />}
            onClick={() =>
              window.open(
                withBase(
                  import.meta.env.BASE_URL,
                  `/api/abrir-pdf?path=${encodeURIComponent(item.path)}`,
                ),
                '_blank',
                'noopener',
              )
            }
          >
            {t('pdf.openInPdf')}
          </MenuItem>
        ) : null}
        {/* Clasificar. El servicio sabe hacerlo desde el principio (PUT/DELETE
            /files/{id}/label) y la fila YA pinta la etiqueta y bloquea con ella el
            enlace público — pero no había forma de ponerla desde ningún sitio. */}
        {!offline && !item.isDirectory && item.id && onLabel ? (
          <MenuItem icon={<Glyph name="lock" size={16} />} onClick={() => onLabel(item)}>
            {item.label ? t('action.classified', { name: item.label.name }) : t('action.classify')}
          </MenuItem>
        ) : null}
        {/* Resumir SOLO donde hay texto que leer: `previewKind` ya sabe qué
            ficheros lo son —el mismo criterio que decide si hay visor— y
            ofrecerlo sobre un .xlsx sería un menú que promete y luego falla.
            El BFF lo vuelve a comprobar: esto es la interfaz, no la frontera. */}
        {!offline && onResumen && previewKind(item) === 'text' ? (
          <MenuItem icon={<Icon name="ai" size={16} />} onClick={() => onResumen(item)}>
            {t('ai.summarize')}
          </MenuItem>
        ) : null}
        {/* Abrir en otro producto de la suite. Van juntas y seguidas porque son la
            misma idea —llevarse este fichero a otro sitio— y separarlas obligaría
            a buscar cada una en un tramo distinto del menú. */}
        {!offline && item.id && !item.isDirectory ? (
          <MenuItem
            icon={<Glyph name="mail" size={16} />}
            disabled={sharing}
            onClick={() => void attachToMail()}
          >
            {t('action.attachToMail')}
          </MenuItem>
        ) : null}
        {!offline && item.id ? (
          <MenuItem
            icon={<Glyph name="users" size={16} />}
            disabled={sharing}
            onClick={() => void shareToRooms()}
          >
            {t('action.shareToRooms')}
          </MenuItem>
        ) : null}
        {/* «Subir a Cast»: una COPIA en Sytue Cast (el fichero se queda aquí), solo
            para vídeos. La pantalla de Cast enseña el fichero y pide confirmar. */}
        {!offline && item.id && subibleACast(item) ? (
          <MenuItem
            icon={<Icon name="video" size={16} />}
            onClick={() =>
              window.location.assign(
                urlDeSubirACast(item.id ?? '', orgActual(import.meta.env.BASE_URL)),
              )
            }
          >
            {t('action.uploadToCast')}
          </MenuItem>
        ) : null}
        {!offline && item.id && onToggleFavorite ? (
          <MenuItem icon={<Glyph name="star" size={16} />} onClick={() => onToggleFavorite(item)}>
            {t(favoriteIds?.has(item.id) ? 'action.removeFavorite' : 'action.addFavorite')}
          </MenuItem>
        ) : null}
        {!offline && onDelete ? (
          <MenuItem
            danger
            icon={<Glyph name="trash" size={16} class="text-danger-text" />}
            onClick={() => onDelete(item)}
          >
            {t('action.delete')}
          </MenuItem>
        ) : null}
      </Dropdown>
    </span>
  );
}

/**
 * MoveDialog — picker de carpeta destino para «Mover a…». Navega el árbol con
 * el mismo GET /api/files?path= del explorador (solo muestra carpetas), con
 * breadcrumb para subir. Excluye la propia carpeta que se mueve (no puede ser
 * su propio destino) y deshabilita «Mover aquí» en la carpeta actual (no-op).
 */
function MoveDialog({
  item,
  onClose,
  onMoved,
}: {
  item: DriveResource;
  onClose: () => void;
  onMoved: () => void;
}) {
  const toast = useToast();
  const { t, locale } = useDriveT();
  const [dest, setDest] = useState('/');
  const [folders, setFolders] = useState<DriveResource[] | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [moving, setMoving] = useState(false);

  const loadDest = useCallback(
    async (target: string) => {
      setFolders(null);
      setLoadError(false);
      try {
        // Las carpetas van completas en la primera página (solo los ficheros
        // se paginan), así que el picker no necesita seguir nextCursor.
        const res = await drive.list(target);
        setFolders(
          res.items
            .filter((it) => it.isDirectory && it.path !== item.path)
            // Se ordena con el idioma del usuario, no con uno fijo: la ñ, los
            // acentos y la ö no se colocan igual en las cinco lenguas.
            .sort((a, b) => a.name.localeCompare(b.name, intlTag(locale))),
        );
      } catch {
        setLoadError(true);
      }
    },
    [item.path, locale],
  );

  useEffect(() => {
    void loadDest(dest);
  }, [dest, loadDest]);

  // Carpeta actual del recurso: mover ahí es un no-op, se deshabilita.
  const currentParent = '/' + item.path.split('/').filter(Boolean).slice(0, -1).join('/');
  const noop = dest === currentParent;

  const onConfirm = async () => {
    if (noop || moving) return;
    setMoving(true);
    try {
      await drive.move(item.path, dest);
      toast.notify({ tone: 'success', title: t('move.done', { name: item.name }) });
      onMoved();
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title:
          e instanceof BffError && e.status === 409
            ? t('move.conflict', { name: item.name })
            : t('move.failed'),
      });
      setMoving(false);
    }
  };

  const crumbs = segments(dest);

  return (
    <Modal
      open
      onClose={onClose}
      title={t('move.title', { name: item.name })}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={moving}>
            {t('files.cancel')}
          </Button>
          <Button
            onClick={() => void onConfirm()}
            loading={moving}
            disabled={noop || folders === null}
          >
            {t('move.here')}
          </Button>
        </>
      }
    >
      {/* Breadcrumb del destino: mismo idioma que el explorador, sobre superficie clara. */}
      <nav
        aria-label={t('move.destinationNav')}
        class="mb-2 flex flex-wrap items-center gap-0.5 text-base"
      >
        <button
          type="button"
          onClick={() => setDest('/')}
          aria-current={dest === '/' ? 'location' : undefined}
          class={cx(
            'hover:bg-bg-subtle active:bg-surface-sunken sytue-focus-ring rounded-control px-2 py-1 whitespace-nowrap transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]',
            dest === '/'
              ? 'text-text font-semibold'
              : 'text-text-muted hover:text-text font-medium',
          )}
        >
          {t('nav.myDrive')}
        </button>
        {crumbs.map((c) => (
          <span key={c.path} class="flex min-w-0 items-center gap-0.5">
            <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
            <button
              type="button"
              onClick={() => setDest(c.path)}
              aria-current={c.path === dest ? 'location' : undefined}
              title={c.name}
              class={cx(
                'hover:bg-bg-subtle active:bg-surface-sunken sytue-focus-ring rounded-control truncate px-2 py-1 transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]',
                c.path === dest
                  ? 'text-text font-semibold'
                  : 'text-text-muted hover:text-text font-medium',
              )}
            >
              {c.name}
            </button>
          </span>
        ))}
      </nav>

      {/* Lista de subcarpetas: altura fija para que navegar no salte el modal. */}
      <div class="border-border h-56 overflow-y-auto rounded-md border">
        {folders === null && !loadError ? (
          <div class="grid h-full place-items-center" aria-busy="true">
            <Spinner size={20} />
            <span class="sr-only" role="status">
              {t('move.loadingFolders')}
            </span>
          </div>
        ) : loadError ? (
          <div class="grid h-full place-items-center p-4 text-center">
            <div>
              <p class="text-text-muted text-base">{t('move.loadFailed')}</p>
              <Button
                variant="secondary"
                size="sm"
                class="mt-2"
                onClick={() => void loadDest(dest)}
              >
                {t('files.retry')}
              </Button>
            </div>
          </div>
        ) : folders && folders.length === 0 ? (
          <div class="grid h-full place-items-center p-4">
            <p class="text-text-muted text-base">{t('move.noSubfolders')}</p>
          </div>
        ) : (
          <ul>
            {folders!.map((f) => (
              <li key={f.path}>
                <button
                  type="button"
                  onClick={() => setDest(f.path)}
                  class="border-border/60 hover:bg-bg-subtle focus-visible:bg-bg-subtle flex w-full items-center gap-2 border-b px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none pointer-coarse:min-h-[max(44px,2.75rem)]"
                >
                  <Glyph name="folder" size={16} class="text-primary-text shrink-0" />
                  <span class="text-text min-w-0 flex-1 truncate font-medium" title={f.name}>
                    {f.name}
                  </span>
                  <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p class="text-text-subtle mt-2 text-sm">
        {noop
          ? t('move.noop')
          : t('move.destination', { path: dest === '/' ? t('nav.myDrive') : dest })}
      </p>
    </Modal>
  );
}

/**
 * OrganizePdfDialog — operaciones de página sobre un PDF (Sytue PDF, ola P4):
 * quedarse con unas páginas, quitarlas o girarlas.
 *
 * Se elige por LO QUE HACE y no por el nombre técnico de la operación —«quedarme
 * solo con estas páginas» en vez de «extraer»—, como en ShareDialog. Y el
 * subtítulo dice desde el principio lo único que de verdad inquieta al pulsar un
 * botón así: que el original NO se toca y que lo que sale es un fichero nuevo.
 *
 * La lista de páginas se escribe a mano («1,3,5-8»). Es el escalón mínimo: lo
 * cómodo sería elegirlas sobre miniaturas, y eso llega cuando el visor propio
 * exista —hoy los PDF se pintan con el visor del navegador dentro de un iframe
 * aislado, que no deja sacar miniaturas—.
 * ponytail: sin miniaturas por eso; cuando haya visor propio, esto se sustituye.
 */
/**
 * Las operaciones que ofrece el diálogo, en el orden en que se muestran.
 *
 * `reordenar` no existe en el servicio: es `extraer` con TODAS las páginas, y se
 * envía como tal. Está aquí porque es como se piensa el problema —«quiero mover
 * la página 7 al final»—, y esconderlo detrás de «quedarme solo con estas
 * páginas» era dejar la función construida pero sin puerta.
 */
const OPS_PDF = [
  'extraer',
  'reordenar',
  'borrar',
  'rotar',
  'marca-agua',
  'numerar',
  'bates',
  'encabezado',
  'pie',
] as const;
type OpPdf = (typeof OPS_PDF)[number];

/** Las que SUPERPONEN texto sobre todas las páginas en vez de tocar la lista. */
const CAPAS_PDF = new Set<OpPdf>(['marca-agua', 'numerar', 'bates', 'encabezado', 'pie']);

/** Ejemplo en el campo de texto: enseñar el formato ahorra explicarlo. */
const PLACEHOLDER_CAPA: Partial<Record<OpPdf, string>> = {
  'marca-agua': 'BORRADOR',
  numerar: '{pagina} / {total}',
  bates: 'ACME-',
  encabezado: 'Confidencial',
  pie: 'Confidencial',
};

function OrganizePdfDialog({
  item,
  onClose,
  onCreated,
}: {
  item: DriveResource;
  onClose: () => void;
  onCreated: () => void;
}) {
  const toast = useToast();
  const { t } = useDriveT();
  const [op, setOp] = useState<OpPdf>('extraer');
  const [paginas, setPaginas] = useState('');
  const [grados, setGrados] = useState<'90' | '180' | '270'>('90');
  const [textoCapa, setTextoCapa] = useState('');
  const [desde, setDesde] = useState('1');
  const [digitos, setDigitos] = useState('6');
  const [trabajando, setTrabajando] = useState(false);
  // Índice del documento: null mientras se pide, [] si no se pudo leer.
  const [indice, setIndice] = useState<PaginaIndice[] | null>(null);
  const [indiceFallo, setIndiceFallo] = useState(false);

  const esCapa = CAPAS_PDF.has(op);
  // Numerar y Bates traen formato por defecto, así que el texto es opcional; la
  // marca de agua y las cabeceras no tienen nada que escribir sin él.
  const faltaTexto = esCapa && op !== 'numerar' && op !== 'bates' && textoCapa.trim() === '';

  const total = indice?.length ?? 0;
  /**
   * Las páginas elegidas, expandidas y en el orden en que van a salir: es lo que
   * se arrastra para reordenar y lo que cuenta la vista previa.
   *
   * `reordenar` parte del documento ENTERO —reordenar es cambiar el orden de lo
   * que hay, no elegir un subconjunto—. Se DERIVA en vez de rellenar el campo al
   * pulsar la operación: así da igual que el índice todavía no hubiera llegado.
   */
  const orden = useMemo(() => {
    const escrito = expandirPaginas(paginas, total);
    if (op === 'reordenar' && escrito.length === 0) return indice?.map((p) => p.numero) ?? [];
    return escrito;
  }, [paginas, total, op, indice]);
  const faltaPaginas = !esCapa && paginas.trim() === '' && orden.length === 0;

  // El índice se pide UNA vez al abrir, no cada vez que cambia la operación:
  // describe el documento, no lo que se va a hacer con él.
  useEffect(() => {
    let vivo = true;
    void (async () => {
      try {
        const res = await fetch('/api/pdf-inspect', {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json', accept: 'application/json' },
          // Siempre por `path`: este diálogo se abre desde la carpeta propia
          // (`organizePdfItem`), nunca desde un enlace compartido, así que aquí
          // no hay `share`/`subpath` que ofrecer.
          body: JSON.stringify({ path: item.path }),
        });
        if (!res.ok) throw new Error(String(res.status));
        const cuerpo = (await res.json()) as { paginas?: PaginaIndice[] };
        if (vivo) setIndice(cuerpo.paginas ?? []);
      } catch {
        // Que no se pueda listar no impide trabajar: queda el campo de texto.
        if (vivo) {
          setIndice([]);
          setIndiceFallo(true);
        }
      }
    })();
    return () => {
      vivo = false;
    };
  }, [item.path]);

  /** Alterna una página en la lista escrita, conservando el orden de pulsación. */
  function alternarPagina(n: number): void {
    const partes = paginas
      .split(',')
      .map((x) => x.trim())
      .filter((x) => x !== '');
    const i = partes.indexOf(String(n));
    if (i >= 0) partes.splice(i, 1);
    else partes.push(String(n));
    setPaginas(partes.join(','));
  }

  /** ¿Está esa página en la lista escrita? Entiende también los rangos. */
  function elegida(n: number): boolean {
    return paginas.split(',').some((trozo) => {
      const t = trozo.trim();
      if (t === String(n)) return true;
      const m = /^(\d+)-(\d+)$/.exec(t);
      if (!m) return false;
      const a = Number(m[1]);
      const b = Number(m[2]);
      return n >= Math.min(a, b) && n <= Math.max(a, b);
    });
  }

  /**
   * Una línea con el resultado ANTES de crearlo: cuántas páginas va a tener el
   * fichero nuevo y sobre cuántas se ha decidido.
   *
   * Solo con el índice a mano: sin saber cuántas páginas tiene el documento no
   * se puede decir «de las 12», y media cuenta es peor que ninguna. Con el
   * índice truncado (500) el total tampoco es el real, así que tampoco se dice.
   */
  function vistaPrevia(): string | null {
    if (total === 0 || total >= 500) return null;
    if (esCapa) return t('pdf.preview.capa', { count: total });
    // Las repetidas cuentan una vez para decidir: borrar dos veces la página 3
    // borra una página, y girarla dos veces la gira una vez.
    const elegidas = new Set(orden).size;
    if (elegidas === 0) return null;
    if (op === 'reordenar') return t('pdf.preview.reordenar', { count: orden.length });
    if (op === 'extraer') return t('pdf.preview.extraer', { count: orden.length, total });
    if (op === 'borrar') {
      return t('pdf.preview.borrar', { count: elegidas, resto: total - elegidas, total });
    }
    return t('pdf.preview.rotar', { count: elegidas, total });
  }

  async function confirmar(): Promise<void> {
    if (faltaTexto || faltaPaginas || trabajando) return;
    setTrabajando(true);
    try {
      const res = await fetch('/api/pdf-pages', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'application/json' },
        body: JSON.stringify({
          path: item.path,
          // `reordenar` no existe en el servicio: es `extraer` con todas las
          // páginas, y el motor conserva el orden que se le da.
          op: op === 'reordenar' ? 'extraer' : op,
          // El nombre del fichero nuevo lleva una palabra en el idioma de quien
          // lo pide; el BFF la sanea antes de que toque una ruta.
          sufijo: t(`pdf.name.${op}`),
          ...(esCapa
            ? {
                texto: textoCapa.trim(),
                ...(op === 'bates' ? { desde: Number(desde), digitos: Number(digitos) } : {}),
              }
            : { paginas: paginas.trim() || orden.join(',') }),
          ...(op === 'rotar' ? { grados: Number(grados) } : {}),
        }),
      });
      const cuerpo = (await res.json().catch(() => ({}))) as {
        resource?: DriveResource;
        message?: string;
      };
      if (!res.ok) {
        // El servicio devuelve motivos accionables (PDF con contraseña, páginas
        // fuera de rango). Se enseña el suyo si lo hay, en vez de un genérico
        // que obliga a adivinar qué ha pasado.
        toast.notify({ tone: 'danger', title: cuerpo.message ?? t('pdf.failed') });
        setTrabajando(false);
        return;
      }
      toast.notify({
        tone: 'success',
        title: t('pdf.done', { name: cuerpo.resource?.name ?? item.name }),
        // Dónde ha quedado y qué NO ha pasado. Sin esto, «Creado …» junto a una
        // lista que se recarga deja la duda de si el original sigue entero, que
        // es justo lo que frena a pulsar el botón la primera vez.
        description: t('pdf.doneWhere', { name: item.name }),
      });
      onCreated();
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('pdf.failed') });
      setTrabajando(false);
    }
  }

  const previa = vistaPrevia();

  return (
    <Modal
      open
      onClose={onClose}
      title={t('pdf.title')}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={trabajando}>
            {t('files.cancel')}
          </Button>
          <Button
            onClick={() => void confirmar()}
            loading={trabajando}
            disabled={faltaTexto || faltaPaginas}
          >
            {trabajando ? t('pdf.working') : t('pdf.submit')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('pdf.subtitle', { name: item.name })}</p>

        <fieldset class="flex flex-col gap-2">
          <legend class="text-text mb-1 text-sm font-medium">{t('pdf.op')}</legend>
          {OPS_PDF.map((valor) => (
            <label
              key={valor}
              class="border-border hover:bg-bg-subtle has-checked:border-primary has-checked:bg-primary/5 flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2 transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]"
            >
              <Radio
                name="pdf-op"
                value={valor}
                checked={op === valor}
                onChange={() => setOp(valor)}
                class="accent-primary"
              />
              <span class="text-text text-sm">{t(`pdf.op.${valor}`)}</span>
            </label>
          ))}
        </fieldset>

        {!esCapa && indice === null ? (
          <p class="text-text-muted text-sm">{t('pdf.pickLoading')}</p>
        ) : null}

        {/* Elegir la página reconociéndola: número, tamaño y las primeras
            palabras. No es una miniatura —rasterizar un PDF sin dependencias es
            la pieza más cara del proyecto— pero resuelve el problema de verdad,
            que es no tener que recordar qué había en la página 7.
            ponytail: se le añadirá la miniatura cuando exista el visor propio;
            la lista no cambia de forma. */}
        {!esCapa && indice !== null && indice.length > 0 ? (
          <fieldset>
            <legend class="text-text mb-1 text-sm font-medium">{t('pdf.pick')}</legend>
            {/* «Todas» no es un adorno: reordenar un documento de veinte páginas
                empieza por elegirlas las veinte, y hacerlo a mano son veinte
                pulsaciones antes de poder mover la primera. */}
            <div class="mb-2 flex items-center gap-1">
              <p class="text-text-muted min-w-0 flex-1 text-xs">{t('pdf.pickHint')}</p>
              <Button
                variant="ghost"
                size="xs"
                onClick={() => setPaginas(indice.map((p) => p.numero).join(','))}
              >
                {t('pdf.pickAll')}
              </Button>
              {/* Vaciar lo escrito. En «reordenar» eso devuelve el documento a
                  su orden original, que es el deshacer que hace falta ahí. */}
              <Button
                variant="ghost"
                size="xs"
                disabled={paginas.trim() === ''}
                onClick={() => setPaginas('')}
              >
                {t('pdf.pickNone')}
              </Button>
            </div>
            <div class="border-border max-h-56 overflow-y-auto rounded-md border">
              <ul>
                {indice.map((pg) => {
                  const sel = elegida(pg.numero);
                  return (
                    <li key={pg.numero}>
                      <button
                        type="button"
                        aria-pressed={sel}
                        onClick={() => alternarPagina(pg.numero)}
                        class={cx(
                          'sytue-focus-ring hover:bg-bg-subtle flex w-full items-baseline gap-3 border-b px-3 py-2 text-left transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]',
                          'border-border/60 last:border-b-0',
                          sel ? 'bg-primary/10' : '',
                        )}
                      >
                        <span class="text-text shrink-0 text-sm font-medium tabular-nums">
                          {t('pdf.pageN', { n: pg.numero })}
                        </span>
                        <span class="text-text-subtle min-w-0 flex-1 truncate text-xs">
                          {pg.extracto === '' ? t('pdf.pageNoText') : pg.extracto}
                        </span>
                        {sel ? (
                          <Icon name="check" size={14} class="text-primary-text shrink-0" />
                        ) : null}
                      </button>
                    </li>
                  );
                })}
              </ul>
            </div>
            {indice.length >= 500 ? (
              <p class="text-text-muted mt-1 text-xs">{t('pdf.pickTruncated', { n: 500 })}</p>
            ) : null}
          </fieldset>
        ) : null}

        {!esCapa && indiceFallo ? (
          <p class="text-text-muted text-xs">{t('pdf.pickFailed')}</p>
        ) : null}

        {!esCapa ? (
          <FieldGroup
            label={indice !== null && indice.length > 0 ? t('pdf.pickManual') : t('pdf.pages')}
            htmlFor="pdf-paginas"
            help={t('pdf.pagesHint')}
          >
            <Input
              id="pdf-paginas"
              value={paginas}
              placeholder="1,3,5-8"
              inputMode="numeric"
              onInput={(e) => setPaginas((e.target as HTMLInputElement).value)}
            />
          </FieldGroup>
        ) : (
          <FieldGroup
            label={t('pdf.text')}
            htmlFor="pdf-texto"
            help={t(
              op === 'marca-agua'
                ? 'pdf.textHintWatermark'
                : op === 'bates'
                  ? 'pdf.textHintBates'
                  : 'pdf.textHintHeader',
            )}
          >
            <Input
              id="pdf-texto"
              value={textoCapa}
              placeholder={PLACEHOLDER_CAPA[op] ?? ''}
              onInput={(e) => setTextoCapa((e.target as HTMLInputElement).value)}
            />
          </FieldGroup>
        )}

        {/* El orden solo se ofrece donde SIGNIFICA algo: `extraer` y `reordenar`
            construyen el documento en el orden de la lista, mientras que borrar
            y girar trabajan sobre un conjunto —girar la 3 y luego la 1 gira las
            mismas dos páginas—. Una lista arrastrable ahí sería un mando que no
            hace nada. */}
        {(op === 'extraer' || op === 'reordenar') && indice !== null && orden.length >= 2 ? (
          <PdfPageOrder
            orden={orden}
            indice={indice}
            t={t}
            onChange={(siguiente) => setPaginas(siguiente.join(','))}
          />
        ) : null}

        {op === 'bates' ? (
          <div class="flex gap-3">
            <FieldGroup label={t('pdf.from')} htmlFor="pdf-desde" class="flex-1">
              <Input
                id="pdf-desde"
                value={desde}
                inputMode="numeric"
                onInput={(e) => setDesde((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
            <FieldGroup label={t('pdf.digits')} htmlFor="pdf-digitos" class="flex-1">
              <Input
                id="pdf-digitos"
                value={digitos}
                inputMode="numeric"
                onInput={(e) => setDigitos((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
          </div>
        ) : null}

        {op === 'rotar' ? (
          <FieldGroup label={t('pdf.degrees')} htmlFor="pdf-grados">
            <Select
              id="pdf-grados"
              value={grados}
              onChange={(e) => setGrados((e.target as HTMLSelectElement).value as typeof grados)}
            >
              <option value="90">{t('pdf.degrees.90')}</option>
              <option value="180">{t('pdf.degrees.180')}</option>
              <option value="270">{t('pdf.degrees.270')}</option>
            </Select>
          </FieldGroup>
        ) : null}

        {/* Qué va a salir, en una línea y justo encima del botón: la cuenta que
            el usuario estaba haciendo de cabeza mientras marcaba páginas. Y
            debajo, lo que hace que pulsar no dé miedo — fichero nuevo, y ni
            siquiera pisa el resultado de la vez anterior. */}
        {previa !== null ? (
          <InlineHelp tone="neutral" title={previa}>
            {t('pdf.preview.safe')}
          </InlineHelp>
        ) : null}
      </div>
    </Modal>
  );
}

/**
 * ShareDialog — gestor de enlaces publicos de un recurso. El nivel de acceso se
 * elige por LO QUE PERMITE HACER (ChoiceCard), no por su nombre tecnico; la
 * caducidad y la contrasena se cuentan en llano mientras se rellenan. Lista los
 * enlaces vivos y los revoca. El servicio solo emite la URL (token) al CREAR el
 * enlace; los listados no la exponen, por eso solo el recien creado es copiable.
 *
 * Si una ETIQUETA DE SENSIBILIDAD prohibe compartir fuera, el servicio devuelve
 * 403 con el motivo: se presenta como un bloque explicativo permanente (por que
 * esta bloqueado y que si se puede hacer), no como un error seco de un segundo.
 */
function ShareDialog({ item, onClose }: { item: DriveResource; onClose: () => void }) {
  const toast = useToast();
  const ahora = useAhora();
  const { t, locale } = useDriveT();
  const [shares, setShares] = useState<ShareLink[] | null>(null);
  const [listError, setListError] = useState(false);
  const [permission, setPermission] = useState<'view' | 'edit'>('view');
  // Fecha de caducidad (YYYY-MM-DD del <input type="date">); '' = sin caducidad.
  const [expires, setExpires] = useState('');
  // Contraseña opcional del enlace ('' = sin contraseña). El servicio la hashea.
  const [password, setPassword] = useState('');
  const [createdUrl, setCreatedUrl] = useState<string | null>(null);
  // Permiso y contraseña del enlace recién creado, para el panel «listo para
  // enviar» (el listado de abajo ya cuenta lo suyo con lo que devuelve el servicio).
  const [createdWith, setCreatedWith] = useState<{
    permission: 'view' | 'edit';
    hasPassword: boolean;
  } | null>(null);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<string | null>(null);
  // Motivo por el que el servicio bloqueó la compartición (etiqueta de
  // sensibilidad). Se conserva en pantalla: no es un fallo transitorio.
  const [blockedReason, setBlockedReason] = useState<string | null>(null);
  // Compartir con una persona de la organización. Va ANTES del enlace público
  // porque es lo que se busca el 90 % de las veces: un enlace con token es la
  // excepción —para quien está fuera—, no lo normal.
  const [persona, setPersona] = useState('');
  const [personaPermiso, setPersonaPermiso] = useState<'view' | 'edit'>('view');
  const [invitando, setInvitando] = useState(false);
  const [personaError, setPersonaError] = useState<string | null>(null);
  // Compartir con un GRUPO propio. La lista se pide perezosamente (al abrir el
  // diálogo, no antes): abrir el explorador no debería pagar una llamada extra
  // por archivo visible, solo por el que se comparte de verdad.
  const [grupos, setGrupos] = useState<string[] | null>(null);
  const [grupo, setGrupo] = useState('');
  const [grupoPermiso, setGrupoPermiso] = useState<'view' | 'edit'>('view');
  const [invitandoGrupo, setInvitandoGrupo] = useState(false);

  const loadShares = useCallback(async () => {
    setShares(null);
    setListError(false);
    try {
      setShares(await drive.listShares(item.path));
    } catch {
      setListError(true);
    }
  }, [item.path]);

  useEffect(() => {
    void loadShares();
  }, [loadShares]);

  useEffect(() => {
    let vivo = true;
    void drive
      .myGroups()
      .then((g) => vivo && setGrupos(g))
      .catch(() => vivo && setGrupos([]));
    return () => {
      vivo = false;
    };
  }, []);

  const onCreate = async () => {
    if (creating) return;
    setCreating(true);
    try {
      // Caducidad = fin del dia local elegido, en RFC3339 (lo exige el servicio).
      const expiresAt = expires ? new Date(`${expires}T23:59:59`).toISOString() : undefined;
      const pw = password.trim() || undefined;
      const share = await drive.createShare(item.path, permission, { expiresAt, password: pw });
      setCreatedUrl(share.url);
      setCreatedWith({ permission, hasPassword: Boolean(pw) });
      setBlockedReason(null);
      setPassword('');
      void loadShares();
    } catch (e) {
      // Un 403 con motivo es, hoy, la etiqueta de sensibilidad del archivo: se
      // queda en pantalla explicando POR QUÉ y qué sí se puede hacer (el toast
      // desaparece y el usuario se queda sin saber qué hacer).
      const reason = e instanceof BffError && e.status === 403 ? e.reason : '';
      if (reason) setBlockedReason(reason);
      else toast.notify({ tone: 'danger', title: t('share.createFailed') });
    } finally {
      setCreating(false);
    }
  };

  const onShareWithPerson = async () => {
    const email = persona.trim();
    if (!email || invitando) return;
    setInvitando(true);
    setPersonaError(null);
    try {
      await drive.shareWithPerson(item.path, email, personaPermiso);
      toast.notify({ tone: 'success', title: t('share.person.done', { email }) });
      setPersona('');
      void loadShares();
    } catch (e) {
      // 404 es «esa dirección no es de nadie de aquí», que es información y no un
      // fallo: se queda escrita al lado del campo en vez de irse con un toast.
      if (e instanceof BffError && e.status === 404) setPersonaError(t('share.person.unknown'));
      else if (e instanceof BffError && e.status === 403 && e.reason) setBlockedReason(e.reason);
      else toast.notify({ tone: 'danger', title: t('share.person.failed') });
    } finally {
      setInvitando(false);
    }
  };

  const onShareWithGroup = async () => {
    if (!grupo || invitandoGrupo) return;
    setInvitandoGrupo(true);
    try {
      await drive.shareWithGroup(item.path, grupo, grupoPermiso);
      toast.notify({ tone: 'success', title: t('share.group.done', { group: grupo }) });
      setGrupo('');
      void loadShares();
    } catch (e) {
      if (e instanceof BffError && e.status === 403 && e.reason) setBlockedReason(e.reason);
      else toast.notify({ tone: 'danger', title: t('share.group.failed') });
    } finally {
      setInvitandoGrupo(false);
    }
  };

  const onRevoke = async (share: ShareLink) => {
    setRevoking(share.id);
    try {
      await drive.revokeShare(share.id);
      toast.notify({ tone: 'success', title: t('share.revoked') });
      setShares((prev) => (prev ? prev.filter((s) => s.id !== share.id) : prev));
    } catch {
      toast.notify({ tone: 'danger', title: t('share.revokeFailed') });
    } finally {
      setRevoking(null);
    }
  };

  // Carpeta y archivo llevan FRASE PROPIA en cada idioma: componerla pegando el
  // sujeto («la carpeta») a la plantilla solo funciona en español.
  const dir = item.isDirectory;
  // La etiqueta de sensibilidad viaja con el recurso: si prohíbe compartir fuera, se
  // dice AL ABRIR el diálogo (antes se descubría al fallar el intento con un 403).
  const labelBlocks = item.label?.blocksExternalShare === true;

  return (
    <Modal
      open
      onClose={onClose}
      title={t('share.title', { name: item.name })}
      size="lg"
      footer={<Button onClick={onClose}>{t('share.done')}</Button>}
    >
      {/* Bloqueado por etiqueta de sensibilidad: POR QUÉ y qué sí puede hacer. Se
          sabe de antemano por la etiqueta del recurso; si el bloqueo aparece al
          intentarlo (403), el motivo lo redacta el servicio. */}
      {blockedReason || labelBlocks ? (
        <InlineHelp
          tone="danger"
          class="mb-4"
          title={t(dir ? 'share.blockedFolder' : 'share.blockedFile')}
        >
          <p>{blockedReason || t('share.blockedByLabel', { name: item.label?.name ?? '' })}</p>
          <p class="mt-1.5">{t('share.blockedAdmin')}</p>
        </InlineHelp>
      ) : null}

      {/* Compartir con una persona de la organización. Va PRIMERO porque es lo
          normal: el enlace con token es para quien está fuera de la casa. El
          servicio acepta comparticiones `kind=user` desde la migración 0001 y esta
          pantalla llevaba desde entonces sabiendo hacer solo enlaces. */}
      <fieldset class="min-w-0">
        <legend class="text-text mb-2 text-base font-semibold">{t('share.person.legend')}</legend>
        <div class="flex flex-wrap items-start gap-2">
          <FieldGroup
            label={t('share.person.email')}
            htmlFor="share-person"
            class="min-w-56 flex-1"
          >
            <Input
              id="share-person"
              type="email"
              inputMode="email"
              autocomplete="off"
              placeholder={t('share.person.placeholder')}
              value={persona}
              aria-describedby="share-person-hint"
              onInput={(e) => {
                setPersona(e.currentTarget.value);
                if (personaError) setPersonaError(null);
              }}
              onKeyDown={(e: KeyboardEvent) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void onShareWithPerson();
                }
              }}
            />
            <FieldHint id="share-person-hint" tone={personaError ? 'danger' : 'neutral'}>
              {personaError ?? t('share.person.hint')}
            </FieldHint>
          </FieldGroup>
          <FieldGroup
            label={t('share.person.access')}
            htmlFor="share-person-perm"
            class="w-44 max-md:w-full"
          >
            <Select
              id="share-person-perm"
              value={personaPermiso}
              onChange={(e) =>
                setPersonaPermiso(e.currentTarget.value === 'edit' ? 'edit' : 'view')
              }
            >
              <option value="view">{t('share.view.title')}</option>
              <option value="edit">{t('share.edit.title')}</option>
            </Select>
          </FieldGroup>
          <Button
            class="mt-6 max-md:mt-0 max-md:w-full"
            loading={invitando}
            disabled={!persona.trim()}
            onClick={() => void onShareWithPerson()}
          >
            {t('share.person.add')}
          </Button>
        </div>
      </fieldset>

      {/* Compartir con un GRUPO propio: solo se ofrecen los grupos a los que
          pertenece quien comparte (ver pages/api/groups.ts). El servicio acepta
          `kind=group` desde la migración 0001; esta pantalla no lo ofrecía. */}
      {grupos === null || grupos.length > 0 ? (
        <fieldset class="mt-4 min-w-0">
          <legend class="text-text mb-2 text-base font-semibold">{t('share.group.legend')}</legend>
          <div class="flex flex-wrap items-start gap-2">
            <FieldGroup
              label={t('share.group.select')}
              htmlFor="share-group"
              class="min-w-56 flex-1"
            >
              <Select
                id="share-group"
                value={grupo}
                disabled={grupos === null}
                onChange={(e) => setGrupo(e.currentTarget.value)}
              >
                <option value="">
                  {grupos === null ? t('share.group.loading') : t('share.group.placeholder')}
                </option>
                {(grupos ?? []).map((g) => (
                  <option key={g} value={g}>
                    {g}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <FieldGroup
              label={t('share.person.access')}
              htmlFor="share-group-perm"
              class="w-44 max-md:w-full"
            >
              <Select
                id="share-group-perm"
                value={grupoPermiso}
                onChange={(e) =>
                  setGrupoPermiso(e.currentTarget.value === 'edit' ? 'edit' : 'view')
                }
              >
                <option value="view">{t('share.view.title')}</option>
                <option value="edit">{t('share.edit.title')}</option>
              </Select>
            </FieldGroup>
            <Button
              class="mt-6 max-md:mt-0 max-md:w-full"
              loading={invitandoGrupo}
              disabled={!grupo}
              onClick={() => void onShareWithGroup()}
            >
              {t('share.group.add')}
            </Button>
          </div>
        </fieldset>
      ) : null}

      {/* Crear enlace: nivel de acceso por lo que PERMITE HACER. */}
      <fieldset class="mt-6 min-w-0">
        <legend class="text-text mb-2 text-base font-semibold">{t('share.legend')}</legend>
        <div class="grid gap-2 sm:grid-cols-2">
          <ChoiceCard
            name="share-permission"
            value="view"
            icon="globe"
            title={t('share.view.title')}
            description={t(dir ? 'share.view.folder' : 'share.view.file')}
            selected={permission === 'view'}
            onSelect={() => setPermission('view')}
          />
          <ChoiceCard
            name="share-permission"
            value="edit"
            icon="plus"
            title={t('share.edit.title')}
            description={t(dir ? 'share.edit.folder' : 'share.edit.file')}
            selected={permission === 'edit'}
            onSelect={() => setPermission('edit')}
          />
        </div>
      </fieldset>

      <div class="mt-4 flex flex-wrap items-start gap-3">
        <FieldGroup label={t('share.until')} htmlFor="share-expires" class="w-56">
          <Input
            id="share-expires"
            type="date"
            min={ahora.toISOString().slice(0, 10)}
            value={expires}
            aria-describedby="share-expires-hint"
            onInput={(e) => setExpires(e.currentTarget.value)}
          />
          <FieldHint id="share-expires-hint" tone={expires ? 'success' : 'neutral'}>
            {expires ? expiryPhrase(t, locale, expires) : t('share.noExpiry')}
          </FieldHint>
        </FieldGroup>
        <FieldGroup label={t('share.password')} htmlFor="share-password" class="w-56">
          <Input
            id="share-password"
            type="password"
            autocomplete="new-password"
            placeholder={t('share.passwordPlaceholder')}
            value={password}
            aria-describedby="share-password-hint"
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
          <FieldHint id="share-password-hint" tone={password.trim() ? 'success' : 'neutral'}>
            {t(password.trim() ? 'share.passwordSet' : 'share.passwordNone')}
          </FieldHint>
        </FieldGroup>
      </div>

      <div class="mt-3 flex flex-wrap items-center gap-2">
        <Button loading={creating} disabled={labelBlocks} onClick={() => void onCreate()}>
          {t('share.create')}
        </Button>
        {labelBlocks ? <span class="sytue-meta">{t('share.labelBlocksHint')}</span> : null}
      </div>

      {createdUrl && createdWith ? (
        <div class="border-success bg-success-bg mt-4 rounded-md border p-3">
          <p class="text-success-text text-sm font-semibold">{t('share.readyTitle')}</p>
          {/* En el teléfono, el enlace a todo el ancho y «Copiar» debajo: al lado le
              quedaban 229 px para 394 de enlace. */}
          <div class="mt-2 flex items-center gap-2 max-md:flex-col max-md:items-stretch">
            {/* Al enfocar se selecciona todo: facilita el copiado manual (⌘C). */}
            <Input
              readonly
              value={createdUrl}
              aria-label={t('share.linkAria')}
              class="flex-1"
              onFocus={(e) => e.currentTarget.select()}
            />
            <Button
              variant="secondary"
              onClick={() => {
                void navigator.clipboard?.writeText(createdUrl);
                toast.notify({ tone: 'success', title: t('share.copied') });
              }}
            >
              {t('share.copy')}
            </Button>
          </div>
          {/* Tres FRASES completas seguidas, no una frase cosida a trozos: cada
              idioma ordena sus oraciones como quiera. */}
          <p class="text-text-muted mt-2 text-sm leading-relaxed">
            {t(createdWith.permission === 'edit' ? 'share.summaryEdit' : 'share.summaryView')}{' '}
            {expires ? expiryPhrase(t, locale, expires) : t('share.summaryNoExpiry')}{' '}
            {t(createdWith.hasPassword ? 'share.summaryPassword' : 'share.summaryNoPassword')}
          </p>
          <p class="sytue-meta mt-1.5">{t('share.copyNow')}</p>
        </div>
      ) : null}

      {/* Enlaces vivos del recurso. */}
      <h3 class="sytue-overline text-text-subtle mt-5 mb-2 font-semibold">
        {t('share.liveHeading')}
      </h3>
      {shares === null && !listError ? (
        <div class="border-border grid h-16 place-items-center rounded-md border" aria-busy="true">
          <Spinner size={18} />
          <span class="sr-only" role="status">
            {t('share.loadingLinks')}
          </span>
        </div>
      ) : listError ? (
        <div class="border-border bg-bg-subtle flex items-center justify-between gap-2 rounded-md border px-3 py-2">
          <p class="text-text-muted text-sm">{t('share.loadFailed')}</p>
          <Button variant="secondary" size="sm" onClick={() => void loadShares()}>
            {t('files.retry')}
          </Button>
        </div>
      ) : shares && shares.length === 0 ? (
        <p class="text-text-muted text-sm">{t(dir ? 'share.emptyFolder' : 'share.emptyFile')}</p>
      ) : (
        <ul class="divide-border/60 border-border divide-y rounded-md border">
          {shares!.map((s) => (
            <li key={s.id} class="flex items-center gap-3 px-3 py-2">
              <Glyph
                name={s.kind === 'link' ? 'link' : 'user'}
                size={16}
                class="text-text-muted shrink-0"
              />
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-1.5">
                  {/* De una compartición con una persona, lo primero es CON QUIÉN.
                      De un enlace, QUÉ PERMITE: no hay a quién nombrar. */}
                  <span class="text-text truncate font-medium">
                    {s.kind === 'link'
                      ? t(s.permission === 'edit' ? 'share.permEdit' : 'share.permView')
                      : (s.name ?? s.email ?? t('share.person.someone'))}
                  </span>
                  {s.hasPassword ? (
                    <Glyph name="lock" size={13} class="text-text-subtle shrink-0" />
                  ) : null}
                </span>
                <span class="sytue-meta block">
                  {s.kind === 'link' ? (
                    <>
                      {s.expiresAt
                        ? expiryPhrase(t, locale, s.expiresAt)
                        : t('share.noExpiryShort')}{' '}
                      · {t(s.hasPassword ? 'share.asksPassword' : 'share.anyoneWithLink')}
                    </>
                  ) : (
                    <>
                      {t(s.permission === 'edit' ? 'share.permEdit' : 'share.permView')}
                      {s.name && s.email ? ` · ${s.email}` : ''}
                    </>
                  )}
                </span>
              </span>
              <Button
                variant="ghost"
                size="sm"
                class="text-danger-text"
                loading={revoking === s.id}
                disabled={revoking !== null && revoking !== s.id}
                onClick={() => void onRevoke(s)}
              >
                {t('share.revoke')}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {shares && shares.length > 0 ? <p class="sytue-meta mt-2">{t('share.revokeNote')}</p> : null}
    </Modal>
  );
}

/**
 * Fecha de un valor ISO; un 'YYYY-MM-DD' (el del <input type="date">) se entiende
 * como el FINAL de ese día LOCAL, que es cuando el servicio hace vencer el enlace.
 */
function parseWhen(iso: string): Date | null {
  const d = new Date(iso.length === 10 ? `${iso}T23:59:59` : iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** Días LOCALES que faltan hasta `d` (0 = hoy, negativo = ya pasó). */
function daysUntil(d: Date): number {
  const startOf = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  return Math.round((startOf(d) - startOf(new Date())) / 86_400_000);
}

/**
 * Caducidad contada en llano («Caduca mañana», «Caduca en 6 días»), no en fechas
 * que hay que restar mentalmente. Se compara por DÍA local: el servicio fija el
 * vencimiento al final del día elegido.
 */
function expiryPhrase(t: T, locale: Locale, iso: string): string {
  const d = parseWhen(iso);
  if (!d) return t('expiry.dated');
  const days = daysUntil(d);
  if (days < 0) return t('expiry.past');
  if (days === 0) return t('expiry.today');
  if (days === 1) return t('expiry.tomorrow');
  // El plural lo decide `Intl.PluralRules`, no un `days === 1`.
  return t('expiry.days', { count: days, date: formatDate(locale, d.toISOString()) });
}

/** ¿Le quedan 3 días o menos? (para pintar la cuenta atrás como advertencia). */
function urgentPurge(item: TrashItem): boolean {
  const d = item.purgesAt ? parseWhen(item.purgesAt) : null;
  return d !== null && daysUntil(d) <= 3;
}

/**
 * Cuenta atrás de la papelera en llano. `purgesAt` lo calcula el SDK con la fecha de
 * borrado del elemento y la retención que publica el servicio; sin `purgesAt` pero
 * CON fecha de borrado, la auto-purga está desactivada y el elemento se queda hasta
 * que alguien lo elimine. Sin ninguna de las dos no se afirma nada.
 */
function purgePhrase(t: T, locale: Locale, item: TrashItem): string {
  if (!item.deletedAt) return '';
  if (!item.purgesAt) return t('purge.never');
  const d = parseWhen(item.purgesAt);
  if (!d) return '';
  const days = daysUntil(d);
  if (days <= 0) return t('purge.today');
  if (days === 1) return t('purge.tomorrow');
  return t('purge.days', { count: days, date: formatDate(locale, d.toISOString()) });
}

/**
 * PreviewDialog — visor inline de un archivo: <img> para imagenes, <iframe>
 * (visor PDF nativo del navegador, sandbox sin scripts) para PDF y <pre> para
 * texto plano. El blob llega del BFF con ?disposition=inline y el content-type
 * real del backend. Sin librerias de terceros; «Descargar» siempre disponible.
 */
/**
 * Las primeras páginas de un PDF, dibujadas por Sytue PDF.
 *
 * El SVG se inserta con `dangerouslySetInnerHTML` y eso pide una justificación:
 * NO viene del fichero, viene de nuestro motor, que reescribe el flujo de
 * contenido del PDF a elementos de dibujo (ADR-0033). No copia marcado del
 * documento ni ejecuta nada suyo, así que no hay HTML de terceros que colar. La
 * alternativa —un `<img src="data:...">`— perdería justo lo que hace útil esto:
 * el texto dejaría de poder seleccionarse.
 */
function PdfPreview({
  paginas,
  cifrado,
  nombre,
  path,
  t,
}: {
  paginas: string[] | null;
  cifrado: boolean;
  nombre: string;
  path: string;
  t: T;
}) {
  if (cifrado) {
    return (
      <div class="grid h-40 place-content-center justify-items-center gap-3 text-center">
        <p class="text-text-muted text-base">{t('preview.pdf.encrypted')}</p>
        {/*
          Sin `path` no hay enlace: en «Compartido conmigo» el fichero no está en
          la unidad de quien mira, así que `/api/abrir-pdf` con esa ruta daría un
          404. Un enlace que promete y no cumple es peor que no ofrecerlo; queda
          el botón de descargar del pie, que ahí sí funciona.
        */}
        {path !== '' && (
          <a
            href={withBase(
              import.meta.env.BASE_URL,
              `/api/abrir-pdf?path=${encodeURIComponent(path)}`,
            )}
            target="_blank"
            rel="noopener"
            // `py-1.5 px-2`: 26 px de diana (WCAG 2.5.8 pide 24) sin cambiar el
            // aspecto. Va SUELTO, no dentro de un párrafo, así que no le vale la
            // excepción de enlace en línea: es la única salida de esta pantalla y
            // con el dedo tiene que poder pulsarse.
            class="text-primary-text sytue-focus-ring inline-block rounded-sm px-2 py-1.5 text-sm font-medium underline underline-offset-2"
          >
            {t('preview.pdf.open')}
          </a>
        )}
      </div>
    );
  }
  if (paginas === null) {
    return (
      <div class="grid h-40 place-items-center" aria-busy="true">
        <Spinner size={20} />
        <span class="sr-only" role="status">
          {t('preview.loading')}
        </span>
      </div>
    );
  }
  return (
    <div class="flex max-h-[65dvh] flex-col gap-3 overflow-auto">
      {paginas.map((svg, i) => (
        <div
          key={i}
          // `[&>svg]:w-full`: el SVG trae el tamaño en puntos del PDF y sin esto
          // se pinta a tamaño de papel, que en un móvil se sale del diálogo.
          class="border-border bg-surface rounded-md border [&>svg]:h-auto [&>svg]:w-full"
          role="img"
          aria-label={t('preview.pdf.page', { n: i + 1, name: nombre })}
          dangerouslySetInnerHTML={{ __html: svg }}
        />
      ))}
      {paginas.length > 0 && path !== '' && (
        // Un documento largo se corta en la quinta página, y decirlo evita que
        // alguien crea que su expediente tiene cinco hojas.
        <p class="text-text-subtle text-center text-xs">
          {t('preview.pdf.more')}{' '}
          <a
            href={withBase(
              import.meta.env.BASE_URL,
              `/api/abrir-pdf?path=${encodeURIComponent(path)}`,
            )}
            target="_blank"
            rel="noopener"
            class="text-primary-text sytue-focus-ring rounded-sm font-medium underline underline-offset-2"
          >
            {t('preview.pdf.open')}
          </a>
        </p>
      )}
    </div>
  );
}

export function PreviewDialog({
  item,
  onClose,
  onDownload,
  inlineSrc,
  share,
  subpath,
  drive,
  file,
}: {
  item: DriveResource;
  onClose: () => void;
  onDownload: (item: DriveResource) => void;
  // Origen del contenido inline. Ausente = «Mi unidad» (por ruta); presente =
  // «Compartido conmigo» (por share, endpoint gated). `| undefined` explícito por
  // exactOptionalPropertyTypes.
  inlineSrc?: string | undefined;
  // Raíz del recurso compartido y ruta dentro. Solo en «Compartido conmigo», y
  // solo sirven para pedir el dibujo del PDF: el fichero no está en la unidad de
  // quien lo mira, así que su `path` no lo encuentra.
  share?: string | undefined;
  subpath?: string | undefined;
  // Unidad de equipo: su contenido está aislado por unidad (RLS) y tampoco vive
  // en la unidad personal de quien mira.
  drive?: string | undefined;
  file?: string | undefined;
}) {
  const { t } = useDriveT();
  const kind = previewKind(item);
  const src =
    inlineSrc ?? `/api/content?path=${encodeURIComponent(item.path)}&disposition=inline&offline=1`;
  const [text, setText] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  // Páginas dibujadas por Sytue PDF, en SVG. `null` mientras se piden; `[]` si el
  // documento está cifrado, que no es un fallo sino un caso con salida propia.
  const [paginas, setPaginas] = useState<string[] | null>(null);
  const [cifrado, setCifrado] = useState(false);

  // PDF: lo dibuja NUESTRO motor, no el visor del navegador.
  //
  // El `<iframe>` con el blob delegaba en el visor del sistema, y eso da tres
  // resultados distintos en las seis plataformas: algunos WebView de Android lo
  // DESCARGAN en vez de mostrarlo y iOS abre su lector con su cromo encima del
  // nuestro. Con SVG (ADR-0033) la vista es la misma en todas partes, el texto se
  // selecciona y el zoom no pixela. `sharedSrc` marca «Compartido conmigo», que
  // va por otro endpoint y todavía no tiene puente de render: ahí se conserva el
  // visor del navegador en vez de dejar la vista previa sin nada.
  useEffect(() => {
    if (kind !== 'pdf' || (!share && !drive && inlineSrc)) return;
    let alive = true;
    fetch('/api/pdf-render', {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      // Cada superficie entra por su puerta: el fichero de una unidad de equipo
      // o de un compartido no está en la unidad personal de quien lo mira.
      body: JSON.stringify(
        drive && file ? { drive, file } : share ? { share, subpath } : { path: item.path },
      ),
    })
      .then((r) => (r.ok ? r.json() : Promise.reject(new BffError(r.status))))
      .then((d: { paginas?: Record<string, string>; error?: string }) => {
        if (!alive) return;
        if (d.error === 'encrypted') {
          setCifrado(true);
          setPaginas([]);
          return;
        }
        // El motor devuelve `{ "1": "<svg…>", "2": … }`: se ordena por número,
        // que como clave de objeto es texto y «10» iría antes que «2».
        const svgs = Object.entries(d.paginas ?? {})
          .sort((a, b) => Number(a[0]) - Number(b[0]))
          .map(([, svg]) => svg);
        setPaginas(svgs);
      })
      .catch(() => {
        if (alive) setFailed(true);
      });
    return () => {
      alive = false;
    };
  }, [kind, item.path, inlineSrc, share, subpath, drive, file]);

  // Ofimática: la pinta Sytue Docs DENTRO de este diálogo, por WOPI. El fichero se
  // queda donde está —no se convierte a un documento nuevo, que era lo que hacía
  // antes el doble clic y dejaba una copia por cada vez que mirabas qué había
  // dentro—. El POST del formulario entra en el marco de abajo, así que hay que
  // esperar a que exista: por eso va en un efecto y no en el manejador del clic.
  useEffect(() => {
    if (kind !== 'office' || !item.id) return;
    let alive = true;
    void openInEditor(item.id, 'view', MARCO_WOPI).then((ok) => {
      if (alive && !ok) setFailed(true);
    });
    return () => {
      alive = false;
    };
  }, [kind, item.id]);

  // Solo el texto se trae por fetch (va a un <pre>); imagen e iframe cargan solos.
  useEffect(() => {
    if (kind !== 'text') return;
    let alive = true;
    fetch(src, { credentials: 'include' })
      .then((r) => (r.ok ? r.text() : Promise.reject(new BffError(r.status))))
      // `body` y no `t`: dentro de este componente `t` es la traducción, y
      // ensombrecerla aquí es como se cuela una pantalla sin traducir.
      .then((body) => {
        if (alive) setText(body);
      })
      .catch(() => {
        if (alive) setFailed(true);
      });
    return () => {
      alive = false;
    };
  }, [kind, src]);

  return (
    <Modal
      open
      onClose={onClose}
      title={item.name}
      size="lg"
      footer={
        <>
          <Button
            variant="secondary"
            leadingIcon={<Glyph name="download" size={16} />}
            onClick={() => onDownload(item)}
          >
            {t('action.download')}
          </Button>
          {/* Secundario: el latón es para la acción principal, y cerrar no lo es. */}
          <Button variant="secondary" onClick={onClose}>
            {t('preview.close')}
          </Button>
        </>
      }
    >
      {failed ? (
        <div class="grid h-40 place-items-center">
          <p class="text-text-muted text-base">{t('preview.failed')}</p>
        </div>
      ) : kind === 'image' ? (
        <img
          src={src}
          alt={item.name}
          class="mx-auto max-h-[65dvh] max-w-full rounded-md object-contain"
          onError={() => setFailed(true)}
        />
      ) : kind === 'pdf' && inlineSrc && !share && !drive ? (
        // «Compartido conmigo» va por otro endpoint y aún no tiene puente de
        // render, así que aquí sigue el visor del navegador. Sandbox vacío (sin
        // scripts ni mismo origen): si un UA no lo soporta, queda «Descargar».
        <iframe
          src={src}
          title={item.name}
          sandbox=""
          class="border-border bg-surface-sunken h-[65dvh] w-full rounded-md border"
        />
      ) : kind === 'pdf' ? (
        <PdfPreview
          paginas={paginas}
          cifrado={cifrado}
          nombre={item.name}
          path={share || drive ? '' : item.path}
          t={t}
        />
      ) : kind === 'office' ? (
        // El editor es OTRO origen y necesita sus scripts para pintar, así que aquí
        // el sandbox no puede ir vacío. Lo que sí se le quita es poder navegar esta
        // pestaña: sin `allow-top-navigation`, el marco no puede sacar a nadie de
        // su carpeta. `allow-popups` es para su propio «abrir en una pestaña nueva».
        <iframe
          name={MARCO_WOPI}
          title={item.name}
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads"
          class="border-border bg-surface-sunken h-[65dvh] w-full rounded-md border"
        />
      ) : text === null ? (
        <div class="grid h-40 place-items-center" aria-busy="true">
          <Spinner size={20} />
          <span class="sr-only" role="status">
            {t('preview.loading')}
          </span>
        </div>
      ) : (
        <pre class="border-border bg-surface-sunken text-text max-h-[65dvh] overflow-auto rounded-md border p-4 font-mono text-sm break-words whitespace-pre-wrap">
          {text}
        </pre>
      )}
    </Modal>
  );
}

/**
 * FileList — vista lista VIRTUALIZADA (VirtualList de @sytue/ui): con decenas
 * de miles de ficheros solo se pintan las filas visibles + overscan. Cabecera
 * de columnas fija fuera del área de scroll (mismo efecto que la antigua
 * cabecera pegajosa) y filas flex con la misma retícula. Al acercarse al final
 * se encadena la página siguiente (keyset del servicio Go).
 */
function FileList({
  items,
  hasMore,
  loadingMore,
  onLoadMore,
  onOpen,
  onDownload,
  folderDownload,
  showLocation,
  onShare,
  onRename,
  onMove,
  onCopy,
  onDelete,
  onVersions,
  onOrganizePdf,
  onLabel,
  onResumen,
  favoriteIds,
  onToggleFavorite,
  selectedIds,
  selectionActive,
  onToggleSelect,
  onDragItem,
  onDropOnFolder,
  allSelected,
  someSelected,
  onToggleAll,
}: RowActions & {
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
  allSelected?: boolean | undefined;
  someSelected?: boolean | undefined;
  onToggleAll?: (() => void) | undefined;
}) {
  const { t } = useDriveT();
  // Clave estable por fila (la ruta): la caché de medición del virtualizador
  // sobrevive a inserciones/borrados sin saltos de scroll.
  const getKey = useMemo(() => (i: number) => items[i]?.path ?? i, [items]);
  return (
    <>
      {/* bg-bg (no bg-surface): se funde con el lienzo en claro y oscuro. Los
          rótulos de columna son decorativos (aria-hidden); la casilla «seleccionar
          todo» es un control real (fuera del aria-hidden). */}
      <div class="sytue-overline bg-bg text-text-subtle mx-3 flex shrink-0 items-center gap-3 px-3 py-2.5 font-semibold shadow-[inset_0_-1px_0_var(--sytue-border)]">
        {onToggleAll ? (
          <SelectAllCheckbox
            checked={!!allSelected}
            indeterminate={!!someSelected}
            onToggle={onToggleAll}
            t={t}
          />
        ) : null}
        <span aria-hidden="true" class="min-w-0 flex-1 truncate">
          {t('files.col.name')}
        </span>
        {/* w-24 y no w-20: «Größe»/«Modificado» son más largos que «Tamaño». */}
        <span aria-hidden="true" class="hidden w-24 truncate text-right sm:block">
          {t('files.col.size')}
        </span>
        <span aria-hidden="true" class="hidden w-28 truncate md:block">
          {t('files.col.modified')}
        </span>
        <span aria-hidden="true" class="w-8 shrink-0" />
      </div>
      <VirtualList
        items={items}
        estimateSize={53} /* py-2 (16) + icono 36 + hairline 1; la real se mide */
        getItemKey={getKey}
        aria-label={t('files.list.aria')}
        class="min-h-0 flex-1 px-3 pb-6 text-base"
        onScroll={(e) => {
          const el = e.currentTarget;
          if (hasMore && el.scrollTop + el.clientHeight >= el.scrollHeight - 600) onLoadMore();
        }}
        renderItem={(item, index) => (
          <FileRow
            item={item}
            index={index}
            onOpen={onOpen}
            onDownload={onDownload}
            folderDownload={folderDownload}
            showLocation={showLocation}
            onShare={onShare}
            onRename={onRename}
            onMove={onMove}
            onCopy={onCopy}
            onDelete={onDelete}
            onVersions={onVersions}
            onOrganizePdf={onOrganizePdf}
            onLabel={onLabel}
            onResumen={onResumen}
            favoriteIds={favoriteIds}
            onToggleFavorite={onToggleFavorite}
            selectedIds={selectedIds}
            selectionActive={selectionActive}
            onToggleSelect={onToggleSelect}
            onDragItem={onDragItem}
            onDropOnFolder={onDropOnFolder}
          />
        )}
      />
      {loadingMore ? (
        <div
          role="status"
          class="border-border/60 bg-bg sytue-meta flex shrink-0 items-center justify-center gap-2 border-t py-1.5"
        >
          <Spinner size={14} />
          {t('files.loadingMore')}
        </div>
      ) : null}
    </>
  );
}

/** Fila de la vista lista (una por recurso); la pinta y mide VirtualList. */
function FileRow({
  item,
  index,
  onOpen,
  onDownload,
  folderDownload,
  showLocation,
  onShare,
  onRename,
  onMove,
  onCopy,
  onDelete,
  onVersions,
  onOrganizePdf,
  onLabel,
  onResumen,
  favoriteIds,
  onToggleFavorite,
  selectedIds,
  selectionActive,
  onToggleSelect,
  onDragItem,
  onDropOnFolder,
}: { item: DriveResource; index: number } & Omit<RowActions, 'items'>) {
  const { t, locale } = useDriveT();
  const checked = !!(item.id && selectedIds?.has(item.id));
  const openLabel = t(item.isDirectory ? 'files.openFolder' : 'files.open', { name: item.name });
  // `encima` solo se pinta cuando lo que se arrastra PUEDE soltarse aquí: un
  // resalte que aparece también donde el movimiento va a ser rechazado enseña a
  // desconfiar del resalte.
  const [encima, setEncima] = useState(false);
  const puedeRecibir = item.isDirectory && !!onDropOnFolder;
  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={openLabel}
      draggable={!!onDragItem}
      onDragStart={(e: DragEvent) => onDragItem?.(e, item)}
      onDragOver={
        puedeRecibir
          ? (e: DragEvent) => {
              if (!e.dataTransfer?.types.includes(MIME_ARRASTRE)) return;
              e.preventDefault();
              e.dataTransfer.dropEffect = 'move';
              setEncima(true);
            }
          : undefined
      }
      onDragLeave={puedeRecibir ? () => setEncima(false) : undefined}
      onDrop={
        puedeRecibir
          ? (e: DragEvent) => {
              setEncima(false);
              if (!e.dataTransfer?.types.includes(MIME_ARRASTRE)) return;
              e.preventDefault();
              e.stopPropagation(); // que no lo recoja el drop de subida del contenedor
              onDropOnFolder?.(e, item.path);
            }
          : undefined
      }
      class={cx(
        'group border-border/60 hover:bg-bg-subtle focus-within:bg-bg-subtle sytue-focus-ring flex cursor-pointer items-center gap-3 border-b px-3 py-2 transition-colors duration-150',
        checked && 'bg-selected hover:bg-selected',
        encima && 'bg-selected ring-primary ring-2 ring-inset',
      )}
      onClick={() => onOpen(item)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onOpen(item);
        }
      }}
    >
      {onToggleSelect && item.id ? (
        <SelectCheckbox
          item={item}
          index={index}
          checked={checked}
          active={!!selectionActive}
          onToggle={onToggleSelect}
          t={t}
        />
      ) : null}
      <FileIcon resource={item} size={36} />
      <span class="min-w-0 flex-1">
        {/* La fila entera es el control «Abrir» (mismo patrón que la rejilla); el nombre solo muestra el tooltip. */}
        <span title={openLabel} class="text-text block max-w-full truncate text-left font-medium">
          {item.name}
        </span>
        {/* En una búsqueda, DÓNDE vive. Los resultados cruzan todo el árbol: sin la
            carpeta, dos ficheros con el mismo nombre son la misma fila. */}
        {showLocation ? (
          <span class="sytue-meta mt-0.5 flex min-w-0 items-center gap-1">
            <Glyph name="folder" size={12} class="shrink-0" />
            <span class="truncate">{locationOf(t, item.path)}</span>
          </span>
        ) : null}
        {/* En pantallas estrechas (columnas ocultas) el tamaño/fecha van bajo el nombre. */}
        <span class="sytue-meta mt-0.5 block truncate tabular-nums sm:hidden">
          {item.isDirectory ? t('files.folder') : formatSize(locale, item.size)}
          {item.modifiedAt ? ` · ${formatDate(locale, item.modifiedAt)}` : ''}
        </span>
      </span>
      <span class="text-text-muted hidden w-24 shrink-0 text-right whitespace-nowrap tabular-nums sm:block">
        {item.isDirectory ? '—' : formatSize(locale, item.size)}
      </span>
      <span class="text-text-muted hidden w-28 shrink-0 whitespace-nowrap tabular-nums md:block">
        {formatDate(locale, item.modifiedAt)}
      </span>
      {/* En puntero grueso NO hay hover: el menú se quedaba invisible pero pulsable,
          así que en el móvil no había forma de descargar, renombrar, compartir ni
          borrar nada, y sobraba una diana fantasma al final de la fila. Mismo
          patrón que la casilla de selección y que DocList de Sytue Docs. */}
      <span class="w-8 shrink-0 opacity-0 transition-opacity duration-150 group-hover:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100">
        <ActionsMenu
          item={item}
          onDownload={onDownload}
          folderDownload={folderDownload}
          onShare={onShare}
          onRename={onRename}
          onMove={onMove}
          onCopy={onCopy}
          onDelete={onDelete}
          onVersions={onVersions}
          onOrganizePdf={onOrganizePdf}
          onLabel={onLabel}
          onResumen={onResumen}
          favoriteIds={favoriteIds}
          onToggleFavorite={onToggleFavorite}
        />
      </span>
    </div>
  );
}

/** Placeholder de carga con la geometría de la rejilla (evita salto de layout). */
function FileGridSkeleton() {
  const { t } = useDriveT();
  return (
    <div class="px-6 py-4" aria-busy="true">
      <span class="sr-only" role="status">
        {t('files.loading')}
      </span>
      <ul class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
        {Array.from({ length: 10 }).map((_, i) => (
          <li key={i} class="border-border bg-surface overflow-hidden rounded-lg border">
            <div class="bg-surface-sunken grid aspect-[4/3] w-full place-items-center">
              <Skeleton width="48px" height="48px" />
            </div>
            <div class="border-border border-t px-3 py-2">
              <Skeleton width={`${52 + ((i * 13) % 36)}%`} height="0.85rem" />
              <Skeleton width="40%" height="0.6rem" class="mt-1.5" />
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Placeholder de carga con la geometria de la lista (evita salto de layout). */
function FileListSkeleton() {
  const { t } = useDriveT();
  return (
    <div class="px-3" aria-busy="true">
      <span class="sr-only" role="status">
        {t('files.loading')}
      </span>
      {/* Réplica de la fila de cabecera (Nombre / Tamaño / Modificado). */}
      <div class="border-border flex items-center border-b px-3 py-2.5">
        <Skeleton width="56px" height="0.6rem" />
        <span class="ml-auto hidden sm:block">
          <Skeleton width="48px" height="0.6rem" />
        </span>
        <span class="ml-8 hidden w-24 md:block">
          <Skeleton width="72px" height="0.6rem" />
        </span>
      </div>
      <ul class="flex flex-col">
        {Array.from({ length: 9 }).map((_, i) => (
          <li key={i} class="flex items-center gap-3 px-3 py-2">
            <Skeleton width="36px" height="36px" />
            <Skeleton width={`${38 + ((i * 11) % 42)}%`} height="0.85rem" />
            <span class="ml-auto hidden sm:block">
              <Skeleton width="44px" height="0.7rem" />
            </span>
            <span class="ml-8 hidden w-24 md:block">
              <Skeleton width="76px" height="0.7rem" />
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function FileGrid({
  items,
  onOpen,
  onDownload,
  folderDownload,
  showLocation,
  onShare,
  onRename,
  onMove,
  onCopy,
  onDelete,
  onVersions,
  onOrganizePdf,
  onLabel,
  onResumen,
  favoriteIds,
  onToggleFavorite,
  selectedIds,
  selectionActive,
  onToggleSelect,
  onDragItem,
  onDropOnFolder,
  franja = false,
}: RowActions & {
  /** La franja de carpetas sobre la lista: teselas bajas, sin vista previa. */
  franja?: boolean;
}) {
  const { t, locale } = useDriveT();
  // Ruta de la carpeta sobre la que se está soltando. Una sola: el puntero está
  // encima de un tile o de ninguno, así que no hace falta un estado por tarjeta.
  const [encima, setEncima] = useState<string | null>(null);
  return (
    <ul
      class={cx(
        'grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5',
        franja ? 'gap-2.5' : 'gap-4',
      )}
    >
      {items.map((item, index) => {
        const checked = !!(item.id && selectedIds?.has(item.id));
        const puedeRecibir = item.isDirectory && !!onDropOnFolder;
        // Una carpeta es una tarjeta de recurso: en la forma «Marco», tesela de color
        // pleno; en «Carril» la clase y las variables no pintan nada.
        const tono = item.isDirectory ? cardTone(CARD_TONES[index % CARD_TONES.length]!) : null;
        return (
          <li key={item.path}>
            <div
              role="button"
              tabIndex={0}
              aria-label={t(item.isDirectory ? 'files.openFolder' : 'files.open', {
                name: item.name,
              })}
              draggable={!!onDragItem}
              onDragStart={(e: DragEvent) => onDragItem?.(e, item)}
              onDragOver={
                puedeRecibir
                  ? (e: DragEvent) => {
                      if (!e.dataTransfer?.types.includes(MIME_ARRASTRE)) return;
                      e.preventDefault();
                      e.dataTransfer.dropEffect = 'move';
                      setEncima(item.path);
                    }
                  : undefined
              }
              onDragLeave={puedeRecibir ? () => setEncima(null) : undefined}
              onDrop={
                puedeRecibir
                  ? (e: DragEvent) => {
                      setEncima(null);
                      if (!e.dataTransfer?.types.includes(MIME_ARRASTRE)) return;
                      e.preventDefault();
                      e.stopPropagation(); // que no lo recoja el drop de subida del contenedor
                      onDropOnFolder?.(e, item.path);
                    }
                  : undefined
              }
              onClick={() => onOpen(item)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onOpen(item);
                }
              }}
              style={tono?.style}
              class={cx(
                'group border-border bg-surface hover:border-border-strong sytue-focus-ring relative flex cursor-pointer flex-col overflow-hidden rounded-lg border transition duration-150 hover:shadow-[var(--shadow-sm)]',
                tono?.class,
                checked && 'border-primary ring-primary ring-1',
                encima === item.path && 'border-primary ring-primary ring-2',
              )}
            >
              {onToggleSelect && item.id ? (
                <div
                  class={cx(
                    // En la franja la esquina izquierda es del icono: la casilla va a
                    // la derecha, junto al menú.
                    'absolute top-1.5 z-10 transition-opacity duration-150',
                    franja ? 'right-10' : 'left-1.5',
                    checked || selectionActive
                      ? 'opacity-100'
                      : // pointer-coarse: en el móvil no hay hover, y sin esto la
                        // casilla quedaba invisible pero pulsable — no se podía
                        // seleccionar nada y sobraba una diana fantasma en la esquina.
                        'opacity-0 group-hover:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100',
                  )}
                >
                  <SelectCheckbox
                    item={item}
                    index={index}
                    checked={checked}
                    active={!!selectionActive}
                    onToggle={onToggleSelect}
                    bare
                    t={t}
                  />
                </div>
              ) : null}
              <div class="absolute top-1.5 right-1.5 z-10 opacity-0 transition-opacity duration-150 group-hover:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100">
                <ActionsMenu
                  item={item}
                  onDownload={onDownload}
                  folderDownload={folderDownload}
                  onShare={onShare}
                  onRename={onRename}
                  onMove={onMove}
                  onCopy={onCopy}
                  onDelete={onDelete}
                  onVersions={onVersions}
                  onOrganizePdf={onOrganizePdf}
                  onLabel={onLabel}
                  onResumen={onResumen}
                  favoriteIds={favoriteIds}
                  onToggleFavorite={onToggleFavorite}
                />
              </div>
              {/* Vista previa: miniatura de imagen (encuadrada) o icono de tipo. En la
                  franja, solo el icono de la carpeta: es una tesela, no una ficha. */}
              {franja ? (
                <CardIcon tone={CARD_TONES[index % CARD_TONES.length]!} class="mt-3 ml-3">
                  <Glyph name="folder" size={18} />
                </CardIcon>
              ) : (
                <GridPreview item={item} t={t} />
              )}
              {/* Pie: nombre + meta (tamaño/fecha). */}
              <div class={cx('min-w-0 px-3 py-2', !franja && 'border-border border-t')}>
                <p class="text-text truncate text-base font-medium" title={item.name}>
                  {item.name}
                </p>
                <p class="sytue-meta mt-0.5 truncate tabular-nums">
                  {item.isDirectory ? t('files.folder') : formatSize(locale, item.size)}
                  {!item.isDirectory && item.modifiedAt
                    ? ` · ${formatDate(locale, item.modifiedAt)}`
                    : ''}
                  {/* En búsqueda, la carpeta donde vive: los resultados vienen de
                      todo el árbol y el nombre solo no basta para reconocerlo. */}
                  {showLocation ? ` · ${locationOf(t, item.path)}` : ''}
                </p>
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

/** DriveResource mínimo para reutilizar FileIcon con un elemento de papelera. */
function trashAsResource(it: TrashItem): DriveResource {
  return {
    path: it.id,
    name: it.name,
    isDirectory: it.isDirectory,
    ...(it.size !== undefined ? { size: it.size } : {}),
  };
}

/**
 * TrashView — vista «Papelera» de Sytue Drive. Lista los elementos borrados
 * (recuperables) y permite «Restaurar» (vuelven a su carpeta) o «Eliminar
 * definitivamente» (purga: borra el blob, irreversible). Habla SOLO con el BFF
 * (/api/trash) por cookie de sesión httpOnly. La renderiza Shell cuando la
 * sección activa es «Papelera».
 */
export function TrashView() {
  const toast = useToast();
  const { t, locale } = useDriveT();
  const offline = useConnectivity() === 'offline';
  const [items, setItems] = useState<TrashItem[] | null>(null);
  const [error, setError] = useState<'session' | 'network' | null>(null);
  const [busy, setBusy] = useState(false);
  // Confirmación previa a la purga (acción irreversible): elemento pendiente.
  const [pendingPurge, setPendingPurge] = useState<TrashItem | null>(null);

  useEffect(() => {
    if (offline) setPendingPurge(null);
  }, [offline]);

  const load = useCallback(async () => {
    setItems(null);
    setError(null);
    try {
      setItems(await drive.listTrash());
    } catch (e) {
      setError(e instanceof BffError && e.status === 401 ? 'session' : 'network');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const onRestore = async (it: TrashItem) => {
    if (offline) return;
    setBusy(true);
    try {
      await drive.restore(it.id, it.isDirectory ? 'folder' : 'file');
      toast.notify({ tone: 'success', title: t('trash.restored', { name: it.name }) });
      await load();
    } catch (e) {
      // 409: ya existe un recurso con ese nombre en la carpeta de destino.
      toast.notify({
        tone: 'danger',
        title:
          e instanceof BffError && e.status === 409
            ? t('trash.restoreConflict', { name: it.name })
            : t('trash.restoreFailed'),
      });
    } finally {
      setBusy(false);
    }
  };

  const onPurge = async (it: TrashItem) => {
    if (offline) return;
    setBusy(true);
    try {
      await drive.purge(it.id, it.isDirectory ? 'folder' : 'file');
      toast.notify({ tone: 'success', title: t('trash.purged', { name: it.name }) });
      await load();
    } catch (e) {
      // El servicio puede negarse con un motivo (hoy: retención legal). Decirlo es
      // la diferencia entre «reintenta en vano» y «esto no depende de ti».
      const reason = e instanceof BffError ? e.reason : '';
      toast.notify({
        tone: 'danger',
        title: reason ? t('trash.purgeDenied', { name: it.name }) : t('trash.purgeFailed'),
        ...(reason ? { description: reason } : {}),
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="flex h-full flex-col">
      {/* Cabecera de la papelera: sólida con el acento del producto (a juego con
          la topbar de la shell). Sin subir/crear: no aplican aquí. */}
      <SectionHeader
        level={1}
        icon={<Glyph name="trash" size={18} />}
        title={t('trash.title')}
        subtitle={t('trash.subtitle')}
      />

      <div class="min-h-0 flex-1 overflow-y-auto">
        {items === null && !error ? (
          <FileListSkeleton />
        ) : error ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(error === 'session' ? 'files.error.sessionTitle' : 'trash.errorTitle')}
              description={t(
                error === 'session' ? 'files.error.sessionBody' : 'files.error.network',
              )}
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                error === 'session' ? (
                  <Button size="sm" onClick={() => location.reload()}>
                    {t('files.error.sessionCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load()}>
                    {t('files.retry')}
                  </Button>
                )
              }
            />
          </div>
        ) : items && items.length === 0 ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t('trash.emptyTitle')}
              description={t('trash.emptyBody')}
              illustration={<Illustration name="emptyDrive" />}
            />
          </div>
        ) : (
          <ul class="px-3 pb-6">
            {/* Lo único que Drive puede AFIRMAR hoy sobre la permanencia: mientras
                estén aquí siguen ocupando almacenamiento (el servicio conserva los
                blobs y la cuota hasta la purga). */}
            <li class="px-3 pt-3 pb-1">
              {/* Dos frases independientes del catálogo (la segunda solo si hay
                  cuenta atrás): cada una se traduce entera, sin componerlas. */}
              <InlineHelp tone="neutral">
                {t('trash.help')}
                {items!.some((i) => i.purgesAt) ? ` ${t('trash.helpCountdown')}` : ''}
              </InlineHelp>
            </li>
            {items!.map((item) => {
              const purge = purgePhrase(t, locale, item);
              return (
                <li
                  key={item.id}
                  class="border-border/60 flex items-center gap-3 border-b px-3 py-2"
                >
                  <FileIcon resource={trashAsResource(item)} size={36} />
                  <span class="min-w-0 flex-1">
                    <span class="text-text block truncate font-medium" title={item.name}>
                      {item.name}
                    </span>
                    <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                      {item.isDirectory ? t('files.folder') : formatSize(locale, item.size)}
                      {/* Cuenta atrás real: el worker de auto-purga lo borrará ese día.
                        En rojo cuando quedan 3 días o menos: a partir de ahí, no
                        restaurarlo ES perderlo. */}
                      {purge ? (
                        <>
                          {' · '}
                          <span class={urgentPurge(item) ? 'text-danger-text' : ''}>{purge}</span>
                        </>
                      ) : null}
                    </span>
                  </span>
                  <span class="flex shrink-0 items-center gap-2">
                    <Button
                      variant="secondary"
                      size="sm"
                      disabled={busy || offline}
                      onClick={() => void onRestore(item)}
                    >
                      {t('trash.restore')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={busy || offline}
                      class="text-danger-text"
                      onClick={() => setPendingPurge(item)}
                    >
                      {t('trash.purge')}
                    </Button>
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {/* Confirmación de purga (irreversible: borra el contenido de verdad). */}
      <Modal
        open={pendingPurge !== null}
        onClose={() => setPendingPurge(null)}
        title={t('trash.purgeTitle')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingPurge(null)} disabled={busy}>
              {t('files.cancel')}
            </Button>
            <Button
              variant="danger"
              loading={busy}
              onClick={() => {
                if (!pendingPurge) return;
                void onPurge(pendingPurge).then(() => setPendingPurge(null));
              }}
            >
              {t('trash.purge')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base leading-relaxed">
          {t(pendingPurge?.isDirectory ? 'trash.purgeBodyFolder' : 'trash.purgeBodyFile', {
            name: pendingPurge?.name ?? '',
          })}
        </p>
      </Modal>
    </div>
  );
}

/** Carpeta contenedora de una ruta, como etiqueta de ubicación (Mi unidad / nombre). */
function locationOf(t: T, path: string): string {
  const parts = path.split('/').filter(Boolean);
  if (parts.length <= 1) return t('nav.myDrive');
  return decodeURIComponent(parts[parts.length - 2]!);
}

/** Glifo y CLAVES de catálogo de cada sección plana; el texto lo resuelve `t`. */
const SECTION_META = {
  recent: {
    glyph: 'clock',
    title: 'recent.title',
    subtitle: 'recent.subtitle',
    errorTitle: 'recent.errorTitle',
    emptyTitle: 'recent.emptyTitle',
    emptyDescription: 'recent.emptyBody',
  },
  favorites: {
    glyph: 'star',
    title: 'favorites.title',
    subtitle: 'favorites.subtitle',
    errorTitle: 'favorites.errorTitle',
    emptyTitle: 'favorites.emptyTitle',
    emptyDescription: 'favorites.emptyBody',
  },
} as const;

/**
 * ResourceListView — vista plana de «Recientes» o «Favoritos» de Sytue Drive.
 * Lista los recursos (con su ruta ya resuelta por el servicio) y permite abrir/
 * previsualizar/descargar, compartir y marcar/desmarcar favorito. Habla SOLO con
 * el BFF (/api/recent, /api/favorites) por cookie de sesión httpOnly. Las
 * acciones de gestión (renombrar/mover/eliminar) viven en «Mi unidad».
 * ponytail: abrir una carpeta favorita aún no salta a «Mi unidad» en su ruta
 * (haría falta elevar la navegación de sección); se listan y se gestionan sus
 * favoritos. «Recientes» pagina por keyset (cursor opaco del backend) con scroll
 * infinito; «Favoritos» llega entero en una tanda y no lleva sentinel.
 */
export function ResourceListView({ mode }: { mode: 'recent' | 'favorites' }) {
  const toast = useToast();
  const { t, locale } = useDriveT();
  const offline = useConnectivity() === 'offline';
  const meta = SECTION_META[mode];
  const [items, setItems] = useState<DriveResource[] | null>(null);
  const [error, setError] = useState<'session' | 'network' | null>(null);
  const [favIds, setFavIds] = useState<ReadonlySet<string>>(() => new Set());
  const [previewItem, setPreviewItem] = useState<DriveResource | null>(null);
  const [shareItem, setShareItem] = useState<DriveResource | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    if (offline) setShareItem(null);
  }, [offline]);

  const load = useCallback(async () => {
    setItems(null);
    setError(null);
    setNextCursor(null);
    try {
      let list: DriveResource[];
      if (mode === 'recent') {
        const page = await drive.recent();
        list = page.items;
        setNextCursor(page.nextCursor ?? null);
      } else {
        list = await drive.favorites();
      }
      setItems(list);
      // En «Favoritos» todo lo listado es favorito; en «Recientes» se pide el
      // conjunto aparte para pintar la estrella correcta (acotado, sin N+1).
      const idsFrom =
        mode === 'favorites' ? list : await drive.favorites().catch(() => [] as DriveResource[]);
      setFavIds(new Set(idsFrom.map((i) => i.id).filter((id): id is string => !!id)));
    } catch (e) {
      setError(e instanceof BffError && e.status === 401 ? 'session' : 'network');
    }
  }, [mode]);

  useEffect(() => {
    void load();
  }, [load]);

  // Siguiente tanda de «Recientes». Un fallo aquí NO tumba lo ya pintado: se
  // avisa y se conserva el cursor para poder reintentar con el botón.
  const loadMore = useCallback(async () => {
    if (loadingMore || !nextCursor) return;
    setLoadingMore(true);
    try {
      const page = await drive.recent(nextCursor);
      setItems((prev) => [...(prev ?? []), ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      toast.notify({ tone: 'danger', title: t('files.loadMoreFailed') });
    } finally {
      setLoadingMore(false);
    }
  }, [loadingMore, nextCursor, t, toast]);

  const scrollRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLLIElement>(null);
  useEffect(() => {
    const root = scrollRef.current;
    const target = sentinelRef.current;
    if (!root || !target || !nextCursor || loadingMore) return;
    const obs = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) void loadMore();
      },
      { root, rootMargin: '200px' },
    );
    obs.observe(target);
    return () => obs.disconnect();
  }, [nextCursor, loadingMore, loadMore]);

  const onDownload = (item: DriveResource) => {
    const a = document.createElement('a');
    a.href = `/api/content?path=${encodeURIComponent(item.path)}&offline=1`;
    a.download = item.name;
    a.rel = 'noopener';
    a.click();
  };

  const handleOpen = async (item: DriveResource) => {
    const kind = previewKind(item);
    // La MISMA puerta que el explorador: desde Recientes o Favoritos se abre un
    // fichero igual que desde su carpeta, y que dependiera de por dónde se llegó
    // sería un comportamiento que nadie puede explicar.
    if (await abrirEnLaSuite(item, kind)) return;
    // ponytail: textos >1 MB directos a descarga (un <pre> enorme congela la pestaña).
    if (!kind || (kind === 'text' && (item.size ?? 0) > 1_000_000)) {
      onDownload(item);
      return;
    }
    setPreviewItem(item);
  };

  const onToggleFavorite = async (item: DriveResource) => {
    if (offline || !item.id) return;
    const id = item.id;
    const next = !favIds.has(id);
    setFavIds((prev) => {
      const s = new Set(prev);
      if (next) s.add(id);
      else s.delete(id);
      return s;
    });
    // En «Favoritos», desmarcar retira la fila de la lista al instante.
    if (mode === 'favorites' && !next) {
      setItems((prev) => (prev ? prev.filter((i) => i.id !== id) : prev));
    }
    try {
      await drive.setFavorite(item, next);
    } catch {
      setFavIds((prev) => {
        const s = new Set(prev);
        if (next) s.delete(id);
        else s.add(id);
        return s;
      });
      if (mode === 'favorites' && !next) void load();
      toast.notify({ tone: 'danger', title: t('files.favorite.failed') });
    }
  };

  return (
    <div class="flex h-full flex-col">
      <SectionHeader
        level={1}
        icon={<Glyph name={meta.glyph} size={18} />}
        title={t(meta.title)}
        subtitle={t(meta.subtitle)}
      />

      <div ref={scrollRef} class="min-h-0 flex-1 overflow-y-auto">
        {items === null && !error ? (
          <FileListSkeleton />
        ) : error ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(error === 'session' ? 'files.error.sessionTitle' : meta.errorTitle)}
              description={t(
                error === 'session' ? 'files.error.sessionBody' : 'files.error.network',
              )}
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                error === 'session' ? (
                  <Button size="sm" onClick={() => location.reload()}>
                    {t('files.error.sessionCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load()}>
                    {t('files.retry')}
                  </Button>
                )
              }
            />
          </div>
        ) : items && items.length === 0 ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(meta.emptyTitle)}
              description={t(meta.emptyDescription)}
              illustration={<Illustration name="emptyDrive" />}
            />
          </div>
        ) : (
          <ul class="px-3 pb-6">
            {items!.map((item) => {
              const starred = item.id ? favIds.has(item.id) : false;
              return (
                <li
                  key={`${item.path}·${item.id ?? ''}`}
                  class="group border-border/60 hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 transition-colors duration-150"
                >
                  <FileIcon resource={item} size={36} />
                  <span class="min-w-0 flex-1">
                    {item.isDirectory ? (
                      <span class="text-text block truncate font-medium" title={item.name}>
                        {item.name}
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={() => void handleOpen(item)}
                        title={t('files.open', { name: item.name })}
                        class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
                      >
                        {item.name}
                      </button>
                    )}
                    <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                      {item.isDirectory ? t('files.folder') : formatSize(locale, item.size)}
                      {item.modifiedAt ? ` · ${formatDate(locale, item.modifiedAt)}` : ''}
                      {` · ${locationOf(t, item.path)}`}
                    </span>
                  </span>
                  <button
                    type="button"
                    onClick={() => void onToggleFavorite(item)}
                    disabled={offline}
                    aria-pressed={starred}
                    aria-label={t(starred ? 'files.star.remove' : 'files.star.add', {
                      name: item.name,
                    })}
                    title={t(starred ? 'action.removeFavorite' : 'action.addFavorite')}
                    class="hover:bg-bg-subtle sytue-focus-ring rounded-control flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50"
                  >
                    {/* Favorito = estado seleccionado → latón (el acento de acción
                        de la casa), no `warning`: el amarillo semántico significa
                        aviso, y aquí no hay nada que avisar. */}
                    <Glyph
                      name="star"
                      size={16}
                      class={starred ? 'text-primary-text' : 'text-text-subtle'}
                    />
                  </button>
                  <span class="shrink-0">
                    <ActionsMenu item={item} onDownload={onDownload} onShare={setShareItem} />
                  </span>
                </li>
              );
            })}
            {/* Sentinel de scroll infinito con botón de respaldo: el observador
                no dispara con teclado ni con `prefers-reduced-motion` en algunos
                navegadores, así que la acción tiene que existir de verdad. */}
            {nextCursor ? (
              <li ref={sentinelRef} class="flex items-center justify-center px-4 py-4">
                {loadingMore ? (
                  <span class="text-text-subtle flex items-center gap-2 text-sm">
                    <Spinner size={16} label={t('files.loadingMore')} />
                    <span aria-hidden="true">{t('files.loadingMore')}</span>
                  </span>
                ) : (
                  <Button variant="ghost" size="sm" onClick={() => void loadMore()}>
                    {t('files.loadMore')}
                  </Button>
                )}
              </li>
            ) : null}
          </ul>
        )}
      </div>

      {previewItem ? (
        <PreviewDialog
          key={previewItem.path}
          item={previewItem}
          onClose={() => setPreviewItem(null)}
          onDownload={onDownload}
        />
      ) : null}
      {shareItem ? (
        <ShareDialog key={shareItem.path} item={shareItem} onClose={() => setShareItem(null)} />
      ) : null}
    </div>
  );
}

/** Distintivo sobrio del permiso de un share (editor/viewer). */
function PermBadge({ permission, t }: { permission: SharedPermission; t: T }) {
  const editor = permission === 'edit';
  return (
    <span
      class={cx(
        'inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium',
        editor ? 'border-primary/40 text-primary-text' : 'border-border text-text-muted',
      )}
      title={t(editor ? 'shared.perm.editTitle' : 'shared.perm.viewTitle')}
    >
      {editor ? <Glyph name="pencil" size={12} /> : null}
      {t(editor ? 'shared.perm.edit' : 'shared.perm.view')}
    </span>
  );
}

/**
 * SharedWithMeView — vista «Compartido conmigo» de Sytue Drive. Lista los recursos
 * que otros han compartido contigo (por tu usuario o tus grupos), con el dueño y el
 * permiso, y permite NAVEGAR dentro de una carpeta compartida (respetando el
 * permiso), previsualizar/descargar ficheros y —solo si eres editor— subir. Habla
 * SOLO con el BFF (/api/shared-with-me, /api/shared, /api/shared-content,
 * /api/shared-upload) por cookie de sesión httpOnly; la verja de acceso la aplica
 * el servicio Go (sin share vigente → 404). Reusa la presentación de
 * ResourceListView (FileIcon/meta/EmptyState/PreviewDialog). La renderiza Shell
 * cuando la sección activa es «Compartido conmigo».
 */
export function SharedWithMeView() {
  const toast = useToast();
  const { t, locale } = useDriveT();
  const offline = useConnectivity() === 'offline';
  const [roots, setRoots] = useState<SharedRoot[] | null>(null);
  const [rootsError, setRootsError] = useState<'session' | 'network' | null>(null);
  // Carpeta raíz abierta (null = listado plano de raíces compartidas).
  const [current, setCurrent] = useState<{
    rootId: string;
    rootName: string;
    permission: SharedPermission;
  } | null>(null);
  const [subpath, setSubpath] = useState('');
  const [folder, setFolder] = useState<{
    permission: SharedPermission;
    items: DriveResource[];
  } | null>(null);
  const [folderError, setFolderError] = useState<'denied' | 'network' | null>(null);
  const [preview, setPreview] = useState<{
    item: DriveResource;
    src: string;
    /** Raíz del recurso compartido y ruta dentro, para poder DIBUJAR el PDF
     *  con nuestro motor: un fichero compartido no está en la unidad de quien
     *  lo mira, así que no vale su `path`. */
    share: string;
    subpath: string;
  } | null>(null);
  const [uploading, setUploading] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const loadRoots = useCallback(async () => {
    setRoots(null);
    setRootsError(null);
    try {
      setRoots(await drive.sharedWithMe());
    } catch (e) {
      setRootsError(e instanceof BffError && e.status === 401 ? 'session' : 'network');
    }
  }, []);

  useEffect(() => {
    void loadRoots();
  }, [loadRoots]);

  const loadFolder = useCallback(async (rootId: string, sp: string) => {
    setFolder(null);
    setFolderError(null);
    try {
      const res = await drive.sharedList(rootId, sp);
      setFolder({ permission: res.permission, items: res.items });
    } catch (e) {
      // 403/404: el share se revocó o caducó mientras navegabas (deny-by-default).
      setFolderError(
        e instanceof BffError && (e.status === 403 || e.status === 404) ? 'denied' : 'network',
      );
    }
  }, []);

  useEffect(() => {
    if (current) void loadFolder(current.rootId, subpath);
  }, [current, subpath, loadFolder]);

  const downloadShared = (rootId: string, sp: string, name: string) => {
    const a = document.createElement('a');
    a.href = sharedContentHref(rootId, sp, false);
    a.download = name;
    a.rel = 'noopener';
    a.click();
  };

  const openFile = (item: DriveResource, rootId: string, sp: string) => {
    const kind = previewKind(item);
    // ponytail: textos >1 MB directos a descarga (un <pre> enorme congela la pestaña).
    if (!kind || (kind === 'text' && (item.size ?? 0) > 1_000_000)) {
      downloadShared(rootId, sp, item.name);
      return;
    }
    setPreview({ item, src: sharedContentHref(rootId, sp, true), share: rootId, subpath: sp });
  };

  // Llegada desde el aviso de «compartido contigo» (`?shared=<id>`,
  // `hubnotify.EnlaceCompartido`): en cuanto están las raíces, se abre esa. Una vez,
  // y el parámetro se quita para que volver o refrescar no lo reabra.
  const pedido = useRef(
    typeof window === 'undefined'
      ? ''
      : (new URLSearchParams(window.location.search).get('shared') ?? ''),
  );
  useEffect(() => {
    if (!pedido.current || !roots) return;
    const r = roots.find((x) => x.id === pedido.current);
    pedido.current = '';
    const url = new URL(window.location.href);
    url.searchParams.delete('shared');
    window.history.replaceState(null, '', url.toString());
    if (r) openRoot(r);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roots]);

  const openRoot = (r: SharedRoot) => {
    if (!r.id) return;
    if (r.isDirectory) {
      setCurrent({ rootId: r.id, rootName: r.name, permission: r.permission });
      setSubpath('');
      return;
    }
    openFile(r, r.id, '');
  };

  const openItem = (item: DriveResource) => {
    if (!current) return;
    if (item.isDirectory) {
      setSubpath(item.path);
      return;
    }
    openFile(item, current.rootId, item.path);
  };

  const onUpload = async (e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    if (offline || files.length === 0 || !current) return;
    setUploading(true);
    try {
      for (const file of files) await drive.sharedUpload(current.rootId, subpath, file);
      // Plural por `Intl.PluralRules` (el catálogo trae las formas), no por `length === 1`.
      toast.notify({ tone: 'success', title: t('files.upload.done', { count: files.length }) });
      await loadFolder(current.rootId, subpath);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title:
          err instanceof BffError && err.status === 403
            ? t('shared.uploadDenied')
            : t('shared.uploadFailed'),
      });
    } finally {
      setUploading(false);
      input.value = '';
    }
  };

  // Migas de la subruta dentro de la carpeta compartida (relativas a su raíz).
  const crumbs = subpath ? subpath.split('/').filter(Boolean) : [];
  const permission = folder?.permission ?? current?.permission ?? 'view';
  const canEdit = permission === 'edit' && !offline;

  return (
    <div class="flex h-full flex-col">
      <SectionHeader
        level={1}
        icon={<Glyph name="users" size={18} />}
        title={t('shared.title')}
        subtitle={t('shared.subtitle')}
      />

      {/* Barra de navegación cuando estás DENTRO de una carpeta compartida. */}
      {current ? (
        <div class="border-border bg-bg-subtle flex flex-wrap items-center gap-x-2 gap-y-1.5 border-b px-4 py-2 sm:px-6">
          <button
            type="button"
            onClick={() => {
              setCurrent(null);
              setSubpath('');
              setFolder(null);
            }}
            class="text-text-muted hover:bg-bg hover:text-text sytue-focus-ring rounded-control inline-flex items-center gap-1 px-2 py-1 text-sm font-medium transition-colors duration-150"
          >
            <Icon name="chevronLeft" size={14} />
            {t('shared.title')}
          </button>
          <nav
            aria-label={t('files.breadcrumb')}
            class="flex min-w-0 flex-wrap items-center gap-0.5 text-sm"
          >
            <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
            <button
              type="button"
              onClick={() => setSubpath('')}
              aria-current={subpath === '' ? 'page' : undefined}
              title={current.rootName}
              class={cx(
                'hover:bg-bg sytue-focus-ring rounded-control max-w-[40vw] truncate px-2 py-1 transition-colors duration-150',
                subpath === ''
                  ? 'text-text font-semibold'
                  : 'text-text-muted hover:text-text font-medium',
              )}
            >
              {current.rootName}
            </button>
            {crumbs.map((name, i) => {
              const target = crumbs.slice(0, i + 1).join('/');
              const last = i === crumbs.length - 1;
              return (
                <span key={target} class="flex min-w-0 items-center gap-0.5">
                  <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
                  <button
                    type="button"
                    onClick={() => setSubpath(target)}
                    aria-current={last ? 'page' : undefined}
                    title={name}
                    class={cx(
                      'hover:bg-bg sytue-focus-ring rounded-control max-w-[40vw] truncate px-2 py-1 transition-colors duration-150',
                      last
                        ? 'text-text font-semibold'
                        : 'text-text-muted hover:text-text font-medium',
                    )}
                  >
                    {name}
                  </button>
                </span>
              );
            })}
          </nav>
          <div class="ml-auto flex shrink-0 items-center gap-2">
            <PermBadge permission={permission} t={t} />
            {canEdit ? (
              <>
                <button
                  type="button"
                  onClick={() => fileInput.current?.click()}
                  disabled={uploading}
                  aria-busy={uploading ? 'true' : undefined}
                  class="border-border text-text hover:bg-bg active:bg-surface-sunken sytue-focus-ring rounded-control inline-flex h-8 items-center gap-2 border px-3 text-sm font-semibold transition-colors duration-150 disabled:pointer-events-none disabled:opacity-60"
                >
                  {uploading ? <Spinner size={16} /> : <Glyph name="upload" size={16} />}
                  {t('shared.uploadHere')}
                </button>
                <input
                  ref={fileInput}
                  type="file"
                  multiple
                  class="hidden"
                  onChange={(e) => void onUpload(e)}
                  aria-hidden="true"
                  tabIndex={-1}
                />
              </>
            ) : null}
          </div>
        </div>
      ) : null}
      {/* Dentro de algo compartido en lectura: la franja con «Pedir acceso de edición»
          a quien lo compartió. Al conceder, el aviso del Hub la quita y deja subir. */}
      {current && permission === 'view' ? (
        <FranjaPeticion
          endpoint={`/api/edit-requests?id=${encodeURIComponent(current.rootId)}`}
          soloLectura={!offline}
          puedeConceder={false}
          pedida={roots?.find((r) => r.id === current.rootId)?.editRequested ?? false}
          mensaje={translate(locale, 'readOnly.sharedWithYou', { name: current.rootName })}
          locale={locale}
          concedido="drive.access.granted"
          recurso={current.rootId}
          alConceder={() => {
            setCurrent((c) => (c ? { ...c, permission: 'edit' } : c));
            setFolder((f) => (f ? { ...f, permission: 'edit' } : f));
            void loadRoots();
          }}
        />
      ) : null}

      <div class="min-h-0 flex-1 overflow-y-auto">
        {current ? (
          renderFolder()
        ) : roots === null && !rootsError ? (
          <FileListSkeleton />
        ) : rootsError ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(rootsError === 'session' ? 'files.error.sessionTitle' : 'shared.errorTitle')}
              description={t(
                rootsError === 'session' ? 'files.error.sessionBody' : 'files.error.network',
              )}
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                rootsError === 'session' ? (
                  <Button size="sm" onClick={() => location.reload()}>
                    {t('files.error.sessionCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void loadRoots()}>
                    {t('files.retry')}
                  </Button>
                )
              }
            />
          </div>
        ) : roots && roots.length === 0 ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t('shared.emptyTitle')}
              description={t('shared.emptyBody')}
              illustration={<Illustration name="emptyDrive" />}
            />
          </div>
        ) : (
          <ul class="px-3 pb-6">
            {roots!.map((r) => (
              <li
                key={`${r.id ?? r.path}`}
                class="group border-border/60 hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 transition-colors duration-150"
              >
                <FileIcon resource={r} size={36} />
                <span class="min-w-0 flex-1">
                  <button
                    type="button"
                    onClick={() => openRoot(r)}
                    title={t(r.isDirectory ? 'files.openFolder' : 'files.open', { name: r.name })}
                    class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
                  >
                    {r.name}
                  </button>
                  <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                    {r.isDirectory ? t('files.folder') : formatSize(locale, r.size)}
                    {` · ${locationOf(t, r.path)}`}
                    {r.sharedBy ? ` · ${t('shared.sharedBy', { name: r.sharedBy })}` : ''}
                  </span>
                </span>
                <PermBadge permission={r.permission} t={t} />
                {!r.isDirectory && r.id ? (
                  <button
                    type="button"
                    onClick={() => downloadShared(r.id!, '', r.name)}
                    aria-label={t('action.downloadName', { name: r.name })}
                    title={t('action.download')}
                    class="text-text-muted hover:bg-bg-subtle hover:text-text sytue-focus-ring rounded-control flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150"
                  >
                    <Glyph name="download" size={16} />
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      {preview ? (
        <PreviewDialog
          key={preview.src}
          item={preview.item}
          inlineSrc={preview.src}
          share={preview.share}
          subpath={preview.subpath}
          onClose={() => setPreview(null)}
          onDownload={(it) => {
            if (current) downloadShared(current.rootId, it.path, it.name);
            else if (it.id) downloadShared(it.id, '', it.name);
          }}
        />
      ) : null}
    </div>
  );

  // Listado DENTRO de una carpeta compartida (estados carga/vacío/error/denegado).
  function renderFolder() {
    if (folder === null && !folderError) return <FileListSkeleton />;
    if (folderError) {
      return (
        <div class="grid h-full place-items-center p-6">
          <EmptyState
            title={t(folderError === 'denied' ? 'shared.deniedTitle' : 'shared.folderErrorTitle')}
            description={t(folderError === 'denied' ? 'shared.deniedBody' : 'files.error.network')}
            illustration={<Icon name="warning" size={40} class="text-danger-text" />}
            action={
              folderError === 'denied' ? (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    setCurrent(null);
                    setSubpath('');
                    void loadRoots();
                  }}
                >
                  {t('shared.deniedCta')}
                </Button>
              ) : (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => current && void loadFolder(current.rootId, subpath)}
                >
                  {t('files.retry')}
                </Button>
              )
            }
          />
        </div>
      );
    }
    if (folder && folder.items.length === 0) {
      return (
        <div class="grid h-full place-items-center p-6">
          <EmptyState
            title={t('team.folder.emptyTitle')}
            description={t(canEdit ? 'shared.emptyFolderEdit' : 'shared.emptyFolderView')}
            illustration={<Glyph name="folder" size={40} />}
          />
        </div>
      );
    }
    return (
      <ul class="px-3 pb-6">
        {folder!.items.map((item) => (
          <li
            key={item.path}
            class="group border-border/60 hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 transition-colors duration-150"
          >
            <FileIcon resource={item} size={36} />
            <span class="min-w-0 flex-1">
              <button
                type="button"
                onClick={() => openItem(item)}
                title={t(item.isDirectory ? 'files.openFolder' : 'files.open', {
                  name: item.name,
                })}
                class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
              >
                {item.name}
              </button>
              <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                {item.isDirectory ? t('files.folder') : formatSize(locale, item.size)}
                {item.modifiedAt ? ` · ${formatDate(locale, item.modifiedAt)}` : ''}
              </span>
            </span>
            {!item.isDirectory && current ? (
              <button
                type="button"
                onClick={() => downloadShared(current.rootId, item.path, item.name)}
                aria-label={t('action.downloadName', { name: item.name })}
                title={t('action.download')}
                class="text-text-muted hover:bg-bg-subtle hover:text-text sytue-focus-ring rounded-control flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150"
              >
                <Glyph name="download" size={16} />
              </button>
            ) : null}
          </li>
        ))}
      </ul>
    );
  }
}

/**
 * SelectCheckbox — casilla de selección de una fila/tarjeta. Control NATIVO
 * (accesible por teclado), con el estilo del sistema (tokens). Detiene la
 * propagación para no abrir el recurso al marcar; lee shiftKey del clic para la
 * selección por rango. En lista se revela al pasar el cursor o con selección activa;
 * en rejilla el contenedor gestiona la visibilidad (bare).
 */
function SelectCheckbox({
  item,
  index,
  checked,
  active,
  onToggle,
  bare,
  t,
}: {
  item: DriveResource;
  index: number;
  checked: boolean;
  active: boolean;
  onToggle: (item: DriveResource, index: number, shift: boolean) => void;
  bare?: boolean;
  t: T;
}) {
  const input = (
    <Checkbox
      checked={checked}
      aria-label={t('files.select', { name: item.name })}
      onClick={(e) => {
        e.stopPropagation();
        onToggle(item, index, e.shiftKey);
      }}
      onChange={() => {}}
      // 16 px con ratón, 24 con el dedo: la diana medía 14x14 en táctil, por debajo
      // del suelo de 24 que documenta IconButton. Se agranda el PROPIO input y no
      // el <span> de fuera porque un checkbox nativo ignora el relleno del envoltorio
      // —el `<span role="presentation">` de abajo ni siquiera es un <label>, así que
      // su relleno no activa nada y encima se traga el clic de la fila—. Mismo patrón
      // que MessageList de Sytue Mail. La fila no cambia de alto: el icono son 36 px.
      class="border-border-strong accent-primary h-4 w-4 shrink-0 cursor-pointer rounded-sm pointer-coarse:h-6 pointer-coarse:w-6"
    />
  );
  if (bare) return input;
  return (
    <span
      role="presentation"
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
      class={cx(
        'flex shrink-0 items-center transition-opacity duration-150',
        checked || active
          ? 'opacity-100'
          : // En puntero grueso NO hay hover: la casilla se quedaba invisible pero
            // pulsable, así que en el móvil no se podía seleccionar un archivo y
            // había una diana fantasma sobre el icono de la fila. Mismo patrón que
            // DocList de Sytue Docs.
            'opacity-0 group-hover:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100',
      )}
    >
      {input}
    </span>
  );
}

/**
 * SelectAllCheckbox — casilla «seleccionar todo» de la cabecera de la lista, con
 * estado intermedio (algunos, no todos) vía la propiedad DOM `indeterminate`.
 */
function SelectAllCheckbox({
  checked,
  indeterminate,
  onToggle,
  t,
}: {
  checked: boolean;
  indeterminate: boolean;
  onToggle: () => void;
  t: T;
}) {
  return (
    <Checkbox
      indeterminate={indeterminate && !checked}
      checked={checked}
      aria-label={t('files.selectAll')}
      onClick={onToggle}
      onChange={() => {}}
      class="border-border-strong accent-primary h-4 w-4 shrink-0 cursor-pointer rounded-sm pointer-coarse:h-6 pointer-coarse:w-6"
    />
  );
}

/**
 * BatchMoveDialog — picker de carpeta destino para «Mover la selección». Navega el
 * árbol con el mismo GET /api/files del explorador (solo carpetas), llevando un
 * RASTRO {nombre, ruta, id} para conocer el ID del destino sin resolver rutas (la
 * raíz usa id vacío, que el servicio Go resuelve a «Mi unidad»). Excluye como
 * destino las carpetas seleccionadas (no se pueden mover dentro de sí mismas). El
 * movimiento va en UNA operación de lote (drive.batch).
 */
function BatchMoveDialog({
  fileIds,
  folderIds,
  excludePaths,
  currentPath,
  count,
  onClose,
  onMoved,
}: {
  fileIds: string[];
  folderIds: string[];
  excludePaths: ReadonlySet<string>;
  currentPath: string;
  count: number;
  onClose: () => void;
  onMoved: () => void;
}) {
  const toast = useToast();
  const { t, locale } = useDriveT();
  // El idioma no cambia mientras el diálogo vive (lo negocia el servidor), así que
  // el rótulo de la raíz se resuelve una vez, al inicializar el rastro.
  const [trail, setTrail] = useState<{ name: string; path: string; id: string }[]>(() => [
    { name: t('nav.myDrive'), path: '/', id: '' },
  ]);
  const [folders, setFolders] = useState<DriveResource[] | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [moving, setMoving] = useState(false);

  const dest = trail[trail.length - 1]!;

  const loadDest = useCallback(
    async (target: string) => {
      setFolders(null);
      setLoadError(false);
      try {
        const res = await drive.list(target);
        setFolders(
          res.items
            .filter((it) => it.isDirectory && it.id && !excludePaths.has(it.path))
            .sort((a, b) => a.name.localeCompare(b.name, intlTag(locale))),
        );
      } catch {
        setLoadError(true);
      }
    },
    [excludePaths, locale],
  );

  useEffect(() => {
    void loadDest(dest.path);
  }, [dest.path, loadDest]);

  const noop = dest.path === currentPath; // la selección ya vive aquí

  const onConfirm = async () => {
    if (moving) return;
    setMoving(true);
    try {
      await drive.batch('move', { fileIds, folderIds }, dest.id);
      toast.notify({ tone: 'success', title: t('files.batch.moveDone', { count }) });
      onMoved();
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title:
          e instanceof BffError && e.status === 409
            ? t('files.batch.moveConflict')
            : t('files.batch.moveFailed'),
      });
      setMoving(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={t('files.batch.moveTitle', { count })}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={moving}>
            {t('files.cancel')}
          </Button>
          <Button
            onClick={() => void onConfirm()}
            loading={moving}
            disabled={noop || folders === null}
          >
            {t('move.here')}
          </Button>
        </>
      }
    >
      <nav
        aria-label={t('move.destinationNav')}
        class="mb-2 flex flex-wrap items-center gap-0.5 text-base"
      >
        {/* `crumb` y no `t`: ese nombre es el de la traducción y ensombrecerlo aquí
            dejaría el diálogo sin traducir sin que el compilador dijera nada. */}
        {trail.map((crumb, i) => {
          const last = i === trail.length - 1;
          return (
            <span key={crumb.path} class="flex min-w-0 items-center gap-0.5">
              {i > 0 ? (
                <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
              ) : null}
              <button
                type="button"
                onClick={() => setTrail((prev) => prev.slice(0, i + 1))}
                aria-current={last ? 'location' : undefined}
                title={crumb.name}
                class={cx(
                  'hover:bg-bg-subtle active:bg-surface-sunken sytue-focus-ring rounded-control truncate px-2 py-1 transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]',
                  last ? 'text-text font-semibold' : 'text-text-muted hover:text-text font-medium',
                )}
              >
                {crumb.name}
              </button>
            </span>
          );
        })}
      </nav>

      <div class="border-border h-56 overflow-y-auto rounded-md border">
        {folders === null && !loadError ? (
          <div class="grid h-full place-items-center" aria-busy="true">
            <Spinner size={20} />
            <span class="sr-only" role="status">
              {t('move.loadingFolders')}
            </span>
          </div>
        ) : loadError ? (
          <div class="grid h-full place-items-center p-4 text-center">
            <div>
              <p class="text-text-muted text-base">{t('move.loadFailed')}</p>
              <Button
                variant="secondary"
                size="sm"
                class="mt-2"
                onClick={() => void loadDest(dest.path)}
              >
                {t('files.retry')}
              </Button>
            </div>
          </div>
        ) : folders && folders.length === 0 ? (
          <div class="grid h-full place-items-center p-4">
            <p class="text-text-muted text-base">{t('move.noSubfolders')}</p>
          </div>
        ) : (
          <ul>
            {folders!.map((f) => (
              <li key={f.path}>
                <button
                  type="button"
                  onClick={() =>
                    setTrail((prev) => [...prev, { name: f.name, path: f.path, id: f.id! }])
                  }
                  class="border-border/60 hover:bg-bg-subtle focus-visible:bg-bg-subtle flex w-full items-center gap-2 border-b px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none pointer-coarse:min-h-[max(44px,2.75rem)]"
                >
                  <Glyph name="folder" size={16} class="text-primary-text shrink-0" />
                  <span class="text-text min-w-0 flex-1 truncate font-medium" title={f.name}>
                    {f.name}
                  </span>
                  <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p class="text-text-subtle mt-2 text-sm">
        {noop
          ? t('files.batch.here')
          : t('move.destination', { path: dest.path === '/' ? t('nav.myDrive') : dest.path })}
      </p>
    </Modal>
  );
}
