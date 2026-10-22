/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Drive — SDK al servicio Go propio (drive.sytue.eu, dev :8082).
 * API REST propia (JSON) para navegar/subir/descargar/compartir + endpoint
 * WebDAV (RFC 4918) para clientes de sincronizacion. Estado en PostgreSQL,
 * blobs en Object Storage S3. NO es Nextcloud: implementacion clean-room sobre
 * estandares abiertos. Autorizacion Bearer (JWT de Sytue ID).
 *
 * REST base:    /api/v1 — API POR ID/árbol (/folders/{id}/children, /files/{id});
 *               este SDK ofrece una fachada POR RUTA (list('/docs')) y resuelve
 *               ruta→ID caminando el árbol desde /folders/root.
 * WebDAV base:  /dav
 */
import { HttpClient, HttpError, type AuthProvider } from '../core/http.js';

export interface DriveOptions {
  /** URL base, p.ej. https://drive.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/**
 * Etiqueta de sensibilidad de un fichero (snapshot del servicio). Solo lo que la
 * UI necesita para decidir ANTES de intentar algo: cómo se llama y si prohíbe
 * compartir fuera de la organización.
 */
export interface SensitivityLabel {
  name: string;
  /** true: el servicio rechaza (403) crear un enlace público para este fichero. */
  blocksExternalShare: boolean;
}

export interface DriveResource {
  /** Ruta absoluta dentro del Drive del usuario/tenant (p.ej. /docs/plan.pdf). */
  path: string;
  name: string;
  isDirectory: boolean;
  size?: number;
  contentType?: string;
  modifiedAt?: string;
  etag?: string;
  /** Identificador estable del recurso (UUID). */
  id?: string;
  /** Etiqueta de sensibilidad, si el fichero lleva una (las carpetas nunca). */
  label?: SensitivityLabel;
}

export interface ShareLink {
  id: string;
  url: string;
  path: string;
  hasPassword: boolean;
  expiresAt?: string;
  /** Qué permite el enlace: 'view' (mirar/descargar) o 'edit' (además escribir). */
  permission: 'view' | 'edit';
  /**
   * Con quién se comparte: 'link' es el enlace público con token, 'user' una
   * persona concreta y 'group' un grupo de Sytue ID. El servicio lo acepta desde
   * la migración 0001; el nombre `ShareLink` se queda por compatibilidad, aunque
   * ya no todo lo que devuelve es un enlace.
   */
  kind: 'link' | 'user' | 'group';
  /** Identificador del destinatario (`sub` o slug del grupo). Vacío en 'link'. */
  subject: string;
}

export interface CreateShareInput {
  path: string;
  password?: string;
  expiresAt?: string;
  /** 'view' (solo lectura) | 'edit'. */
  permission?: 'view' | 'edit';
  /**
   * Compartir con una PERSONA (o un grupo) en vez de crear un enlace público.
   *
   * Es el `sub` de Sytue ID, no un correo: una dirección cambia, se reasigna y
   * tiene alias, y un permiso guardado contra ella dejaría de significar lo
   * mismo. Quien llama resuelve el correo con `POST /api/v1/directory:resolve`
   * del servicio de identidad.
   */
  subject?: string;
  /** Qué es `subject`. Por defecto 'user'. */
  subjectKind?: 'user' | 'group';
}

/** Elemento en la papelera (borrado recuperable). */
export interface TrashItem {
  /** Identificador estable (UUID) para restaurar/purgar. */
  id: string;
  name: string;
  isDirectory: boolean;
  size?: number;
  /** Cuándo se envió a la papelera (ISO-8601). */
  deletedAt?: string;
  /**
   * Cuándo lo borrará DEFINITIVAMENTE el worker de auto-purga (ISO-8601):
   * deletedAt + retención del servicio. Ausente si la auto-purga está desactivada
   * (se conserva hasta que alguien lo elimine a mano).
   */
  purgesAt?: string;
}

/** Carpeta tal y como la emite el servicio Go. */
interface RawFolder {
  id: string;
  parent_id?: string;
  name: string;
  is_root: boolean;
  created_at: string;
  updated_at: string;
  /** Solo en los listados de papelera: cuándo se borró. */
  deleted_at?: string;
}

/** Etiqueta de sensibilidad tal y como la emite el servicio Go. */
interface RawLabel {
  id: string;
  name: string;
  rank: number;
  block_external_share: boolean;
}

/** Fichero tal y como lo emite el servicio Go. */
interface RawFile {
  id: string;
  folder_id: string;
  name: string;
  size_bytes: number;
  content_type: string;
  checksum?: string;
  version: number;
  created_at: string;
  updated_at: string;
  label?: RawLabel;
  /** Solo en los listados de papelera: cuándo se borró. */
  deleted_at?: string;
  /**
   * Ruta absoluta del fichero. SOLO la trae la respuesta de pedirlo por id:
   * el servicio la resuelve aparte (sabe de qué carpeta cuelga) y la añade
   * aditivamente. Los listados de carpeta y de papelera NO la llevan.
   */
  path?: string;
}

/** Compartición tal y como la emite el servicio Go. */
interface RawShare {
  id: string;
  file_id?: string;
  folder_id?: string;
  kind: 'link' | 'user' | 'group';
  /** Destinatario: `sub` del usuario o slug del grupo; vacío en 'link'. */
  subject?: string;
  token?: string;
  /** Nivel concedido: 'read' | 'write' (vocabulario del servicio). */
  permission?: 'read' | 'write';
  /** true si el enlace está protegido por contraseña (el hash nunca viaja). */
  has_password?: boolean;
  expires_at?: string;
}

/** Hijos de una carpeta (respuesta de /folders/{id}/children). */
interface RawChildren {
  folders: RawFolder[];
  files: RawFile[];
  /** Presente cuando quedan más ficheros (paginación keyset del servicio Go). */
  next_cursor?: string;
}

function joinPath(parent: string, name: string): string {
  return parent === '/' ? `/${name}` : `${parent}/${name}`;
}

function segments(path: string): string[] {
  return path.split('/').filter(Boolean).map(decodeURIComponent);
}

/**
 * La vista plana de un nodo («nodeView» del servicio Go: fichero o carpeta con su
 * RUTA ya resuelta, snake_case). La devuelven los listados que cruzan el árbol
 * —búsqueda, recientes, favoritos, compartidos—, que no se pueden navegar por
 * carpeta y por eso traen la ruta en la respuesta.
 */
export interface RawNode {
  id: string;
  name: string;
  is_directory: boolean;
  path: string;
  size_bytes?: number;
  content_type?: string;
  updated_at?: string;
  /**
   * Snapshot de etiqueta de sensibilidad, con la MISMA forma que en el listado
   * principal (/folders/{id}/children). Ausente si el fichero no lleva ninguna.
   */
  label?: { name: string; block_external_share?: boolean };
}

/** Un nodeView como DriveResource: mismo criterio que file/folderResource. */
export function nodeResource(n: RawNode): DriveResource {
  return {
    path: n.path,
    name: n.name,
    isDirectory: n.is_directory,
    id: n.id,
    ...(n.is_directory ? {} : { size: n.size_bytes ?? 0 }),
    ...(n.content_type ? { contentType: n.content_type } : {}),
    ...(n.updated_at ? { modifiedAt: n.updated_at } : {}),
    // Mismo mapeo que DriveClient.fileResource: la etiqueta llega a la UI y el
    // diálogo de compartir avisa antes de intentarlo.
    ...(n.label
      ? {
          label: { name: n.label.name, blocksExternalShare: n.label.block_external_share === true },
        }
      : {}),
  };
}

export class DriveClient {
  private readonly http: HttpClient;
  private readonly base: string;
  /** Raiz WebDAV (RFC 4918) para clientes de sync. */
  readonly davRoot: string;
  /** Cache ruta→id de carpeta (vida = la de la instancia; en BFF, una petición). */
  private readonly folderIds = new Map<string, string>();

  constructor(options: DriveOptions) {
    this.base = options.baseUrl.replace(/\/$/, '');
    this.http = new HttpClient({
      baseUrl: `${this.base}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
    this.davRoot = `${this.base}/dav`;
  }

  // --- Resolución ruta→ID (el backend indexa por ID/árbol) -------------------

  /** ID de la carpeta raíz (el backend la auto-aprovisiona en /folders/root). */
  private async rootId(): Promise<string> {
    const cached = this.folderIds.get('/');
    if (cached) return cached;
    const root = await this.http.json<RawFolder>({ path: '/folders/root' });
    this.folderIds.set('/', root.id);
    return root.id;
  }

  private children(folderId: string, cursor?: string): Promise<RawChildren> {
    return this.http.json<RawChildren>({
      path: `/folders/${encodeURIComponent(folderId)}/children`,
      query: { cursor: cursor || undefined },
    });
  }

  /**
   * Resuelve la carpeta de una ruta caminando el árbol desde la raíz.
   * ponytail: O(profundidad) peticiones por resolución; si el perfil lo pide,
   * el paso siguiente es un endpoint /resolve?path= en el servicio Go.
   */
  private async resolveFolderId(path: string): Promise<string> {
    const cached = this.folderIds.get(path || '/');
    if (cached) return cached;
    let id = await this.rootId();
    let current = '/';
    for (const seg of segments(path)) {
      const kids = await this.children(id);
      const next = kids.folders.find((f) => f.name === seg);
      if (!next) {
        throw new HttpError(404, 'Not Found', path, `La carpeta ${seg} no existe.`);
      }
      id = next.id;
      current = joinPath(current, seg);
      this.folderIds.set(current, id);
    }
    return id;
  }

  /** Resuelve una entrada (carpeta o fichero) por su ruta. */
  private async resolveEntry(
    path: string,
  ): Promise<{ kind: 'folder'; folder: RawFolder } | { kind: 'file'; file: RawFile }> {
    const segs = segments(path);
    if (segs.length === 0) {
      const root = await this.http.json<RawFolder>({ path: '/folders/root' });
      return { kind: 'folder', folder: root };
    }
    const name = segs[segs.length - 1]!;
    const parentId = await this.resolveFolderId('/' + segs.slice(0, -1).join('/'));
    // Las carpetas van completas en la primera página; los ficheros llegan en
    // páginas keyset (≤500) → hay que seguir next_cursor hasta encontrar el nombre.
    // ponytail: O(n/500) peticiones en el peor caso; si el perfil lo pide, el
    // paso siguiente es un endpoint /resolve?path= (o actuar por id) en el Go.
    let cursor: string | undefined;
    do {
      const kids = await this.children(parentId, cursor);
      const folder = kids.folders.find((f) => f.name === name);
      if (folder) return { kind: 'folder', folder };
      const file = kids.files.find((f) => f.name === name);
      if (file) return { kind: 'file', file };
      cursor = kids.next_cursor;
    } while (cursor);
    throw new HttpError(404, 'Not Found', path, 'El recurso no existe.');
  }

  private folderResource(parentPath: string, f: RawFolder): DriveResource {
    return {
      path: f.is_root ? '/' : joinPath(parentPath, f.name),
      name: f.name,
      isDirectory: true,
      modifiedAt: f.updated_at,
      id: f.id,
    };
  }

  private fileResource(parentPath: string, f: RawFile): DriveResource {
    return {
      path: joinPath(parentPath, f.name),
      name: f.name,
      isDirectory: false,
      size: f.size_bytes,
      contentType: f.content_type,
      modifiedAt: f.updated_at,
      etag: f.checksum || String(f.version),
      id: f.id,
      ...(f.label
        ? {
            label: {
              name: f.label.name,
              blocksExternalShare: f.label.block_external_share === true,
            },
          }
        : {}),
    };
  }

  // --- API pública por ruta ---------------------------------------------------

  /** Lista el contenido de una carpeta (primera página del servicio, ≤500 ficheros). */
  async list(path = '/'): Promise<DriveResource[]> {
    return (await this.listPage(path)).items;
  }

  /**
   * Lista una carpeta con paginación keyset (ListFilesPage en el servicio Go):
   * las subcarpetas van completas en la primera página; los ficheros llegan ya
   * ordenados por (lower(name), id) en páginas de `limit` (el servicio acota a
   * ≤500). `nextCursor` presente cuando hay más; es un token opaco que se pasa
   * tal cual para pedir la página siguiente.
   */
  async listPage(
    path = '/',
    opts: { cursor?: string | undefined; limit?: number | undefined } = {},
  ): Promise<{ items: DriveResource[]; nextCursor?: string }> {
    const id = await this.resolveFolderId(path);
    const r = await this.http.json<RawChildren>({
      path: `/folders/${encodeURIComponent(id)}/children`,
      query: { cursor: opts.cursor || undefined, limit: opts.limit },
    });
    const base = path === '' ? '/' : path;
    return {
      items: [
        ...(r.folders ?? []).map((f) => this.folderResource(base, f)),
        ...(r.files ?? []).map((f) => this.fileResource(base, f)),
      ],
      ...(r.next_cursor ? { nextCursor: r.next_cursor } : {}),
    };
  }

  /**
   * Busca por nombre en TODA la unidad (`GET /search?q=`): ficheros y carpetas,
   * cada uno con su ruta resuelta por el servicio, que es lo único que sitúa un
   * resultado cuando la búsqueda cruza el árbol.
   */
  async search(q: string, opts: { limit?: number | undefined } = {}): Promise<DriveResource[]> {
    const r = await this.http.json<{ files?: RawNode[] }>({
      path: '/search',
      query: { q, limit: opts.limit },
    });
    return (r.files ?? []).map(nodeResource);
  }

  /**
   * Metadatos de un fichero por su ID, con su RUTA REAL.
   *
   * `stat()` resuelve por ruta recorriendo carpetas; aquí solo se tiene el id
   * —es lo que viaja en los deep-links canónicos `/files/{id}` que componen el
   * Hub y la búsqueda federada—, y recorrer el árbol entero para encontrar su
   * nombre sería absurdo teniendo el endpoint directo.
   *
   * El servicio ACOMPAÑA la ruta absoluta en la respuesta (la resuelve él, que
   * sabe de qué carpeta cuelga: `GetFile` en services/drive). Ignorarla y
   * colgar el nombre de la raíz hacía que quien consumiera esta ruta buscara el
   * fichero donde NO está: «abrir en Docus/Calcus/Diapos» (open-from-drive →
   * openDownload) y «guardar en Drive» (upload) solo funcionaban con ficheros
   * de la raíz, y el resto volvía a la lista de documentos con un error.
   *
   * Si el servicio no logra resolver la ruta responde SIN ella —la hizo aditiva
   * a propósito—, y aquí queda colgado de la raíz: el metadato sigue siendo
   * válido, que es lo que el propio servicio decidió al no dejar caer la
   * respuesta entera por no poder situar el fichero.
   */
  async statById(id: string): Promise<DriveResource> {
    const f = await this.http.json<RawFile>({ path: `/files/${encodeURIComponent(id)}` });
    const padre = f.path ? f.path.slice(0, f.path.lastIndexOf('/')) || '/' : '/';
    return this.fileResource(padre, f);
  }

  /** Metadatos de un recurso concreto. */
  async stat(path: string): Promise<DriveResource> {
    const entry = await this.resolveEntry(path);
    const parent = '/' + segments(path).slice(0, -1).join('/');
    return entry.kind === 'folder'
      ? this.folderResource(parent, entry.folder)
      : this.fileResource(parent, entry.file);
  }

  /**
   * Abre el contenido de un fichero SIN leerlo: devuelve la respuesta cruda, cuyo
   * `body` es un stream. El servicio Go transmite el blob desde S3 con `io.Copy`, así
   * que quien solo tenga que reenviarlo (un BFF) puede encadenar los streams y gastar
   * memoria O(1) en vez de O(tamaño del fichero).
   *
   * Cabeceras útiles ya resueltas por el servicio: `content-type`, `content-length`
   * (exacto, del metadato) y `etag` (checksum).
   */
  async openDownload(path: string): Promise<Response> {
    const entry = await this.resolveEntry(path);
    if (entry.kind !== 'file') {
      throw new HttpError(400, 'Bad Request', path, 'No es un fichero.');
    }
    return this.http.request({
      path: `/files/${encodeURIComponent(entry.file.id)}/content`,
    });
  }

  /**
   * Descarga un archivo como Blob, es decir, ENTERO en memoria. Para reenviarlo tal
   * cual usa `openDownload()`: esto solo compensa cuando hay que mirar los bytes
   * (p. ej. manipular un PDF).
   */
  async download(path: string): Promise<Blob> {
    return (await this.openDownload(path)).blob();
  }

  /** Sube un archivo (la carpeta destino debe existir). */
  async upload(
    path: string,
    data: Blob | ArrayBuffer | string,
    contentType?: string,
  ): Promise<DriveResource> {
    const segs = segments(path);
    const name = segs[segs.length - 1];
    if (!name) throw new HttpError(400, 'Bad Request', path, 'Ruta de fichero inválida.');
    const parentPath = '/' + segs.slice(0, -1).join('/');
    const folderId = await this.resolveFolderId(parentPath);
    // EL NOMBRE VA EN `?name=`, NO EN UNA CABECERA. Una cabecera HTTP es una
    // ByteString: no puede llevar un nombre de fichero, que es Unicode. Con
    // `x-file-name` pasaban las dos cosas, y ninguna daba la cara:
    //
    //   - Con una tilde («PDF sin título.pdf»), `fetch` la mandaba en Latin-1
    //     —la í como un solo byte 0xED—, así que al servicio le llegaba una
    //     cadena que NO es UTF-8 válido y Postgres rechazaba el insert. Subir
    //     fallaba, y crear un PDF en blanco no hacía nada… en castellano: en
    //     inglés y en alemán el nombre por defecto no lleva acentos.
    //   - Con algo fuera de Latin-1 (漢, un emoji), `fetch` LANZA antes de
    //     enviar nada: «character has a value greater than 255».
    //
    // En el query no pasa: `URLSearchParams` lo escapa en UTF-8 y el servicio
    // lo lee ya decodificado. Es lo que `team-drive-content.ts` hacía desde el
    // principio, con esta misma razón escrita al lado.
    const file = await this.http.json<RawFile>({
      method: 'POST',
      path: `/folders/${encodeURIComponent(folderId)}/files?${new URLSearchParams({ name })}`,
      headers: { 'content-type': contentType ?? 'application/octet-stream' },
      body: data as BodyInit,
    });
    return this.fileResource(parentPath, file);
  }

  /**
   * Deja en la carpeta un CSV con la vista de Sytue Bases que abarca el token de una
   * conexión de solo lectura concedida a Sytue Drive. El servicio lee de Bases, no guarda
   * el token y anota la procedencia. 422 `bases_connection_invalid` si la conexión no vale.
   */
  async importBasesView(folderPath: string, token: string): Promise<DriveResource> {
    const folderId = await this.resolveFolderId(folderPath);
    const r = await this.http.postJson<{ file: RawFile }>(
      `/folders/${encodeURIComponent(folderId)}/imports/bases`,
      { token },
    );
    return this.fileResource(folderPath, r.file);
  }

  /** Crea una carpeta (su carpeta padre debe existir). */
  async createFolder(path: string): Promise<DriveResource> {
    const segs = segments(path);
    const name = segs[segs.length - 1];
    if (!name) throw new HttpError(400, 'Bad Request', path, 'Ruta de carpeta inválida.');
    const parentPath = '/' + segs.slice(0, -1).join('/');
    const parentId = await this.resolveFolderId(parentPath);
    const folder = await this.http.postJson<RawFolder>('/folders', {
      parent_id: parentId,
      name,
    });
    this.folderIds.set(joinPath(parentPath, name), folder.id);
    return this.folderResource(parentPath, folder);
  }

  /** Elimina un recurso (a la papelera). */
  async removeFileById(id: string, signal?: AbortSignal): Promise<void> {
    await this.http.request({
      method: 'DELETE',
      path: `/files/${encodeURIComponent(id)}`,
      ...(signal ? { signal } : {}),
    });
  }

  /** Elimina un recurso (a la papelera). */
  async remove(path: string): Promise<void> {
    const entry = await this.resolveEntry(path);
    const target =
      entry.kind === 'folder'
        ? `/folders/${encodeURIComponent(entry.folder.id)}`
        : `/files/${encodeURIComponent(entry.file.id)}`;
    await this.http.request({ method: 'DELETE', path: target });
    this.folderIds.clear(); // rutas cacheadas bajo el borrado quedan inválidas
  }

  // --- Papelera (trash) -------------------------------------------------------

  /**
   * Lista los elementos en la papelera (borrados de nivel superior: una carpeta
   * borrada aparece como una entrada, no su contenido suelto). El servicio devuelve
   * el `deleted_at` de cada elemento y su RETENCIÓN vigente (retention_seconds, 0 =
   * auto-purga desactivada): aquí se combinan en `purgesAt`, la fecha en la que el
   * worker lo borrará de verdad.
   */
  async listTrash(): Promise<TrashItem[]> {
    const r = await this.http.json<RawChildren & { retention_seconds?: number }>({
      path: '/trash',
    });
    const retention = typeof r.retention_seconds === 'number' ? r.retention_seconds : 0;
    const when = (deletedAt?: string): { deletedAt?: string; purgesAt?: string } => {
      if (!deletedAt) return {};
      if (retention <= 0) return { deletedAt };
      const purge = new Date(new Date(deletedAt).getTime() + retention * 1000);
      return Number.isNaN(purge.getTime())
        ? { deletedAt }
        : { deletedAt, purgesAt: purge.toISOString() };
    };
    return [
      ...(r.folders ?? []).map((f) => ({
        id: f.id,
        name: f.name,
        isDirectory: true,
        ...when(f.deleted_at),
      })),
      ...(r.files ?? []).map((f) => ({
        id: f.id,
        name: f.name,
        isDirectory: false,
        size: f.size_bytes,
        ...when(f.deleted_at),
      })),
    ];
  }

  /** Restaura un elemento de la papelera a su carpeta original (por ID). */
  async restore(id: string, kind: 'file' | 'folder'): Promise<void> {
    const base = kind === 'folder' ? 'folders' : 'files';
    await this.http.request({ method: 'POST', path: `/${base}/${encodeURIComponent(id)}/restore` });
    this.folderIds.clear(); // el árbol cambió: rutas cacheadas quedan invalidadas
  }

  /** Elimina DEFINITIVAMENTE un elemento de la papelera (borra el blob). */
  async purge(id: string, kind: 'file' | 'folder'): Promise<void> {
    const base = kind === 'folder' ? 'folders' : 'files';
    await this.http.request({ method: 'DELETE', path: `/${base}/${encodeURIComponent(id)}/purge` });
  }

  /** Mueve o renombra un recurso. */
  async move(from: string, to: string): Promise<void> {
    const entry = await this.resolveEntry(from);
    const toSegs = segments(to);
    const newName = toSegs[toSegs.length - 1];
    if (!newName) throw new HttpError(400, 'Bad Request', to, 'Ruta destino inválida.');
    const newParentId = await this.resolveFolderId('/' + toSegs.slice(0, -1).join('/'));
    const fromParent = '/' + segments(from).slice(0, -1).join('/');
    const sameParent = (await this.resolveFolderId(fromParent)) === newParentId;
    if (entry.kind === 'folder') {
      // El backend trata rename y move como operaciones separadas (PATCH excluyente).
      if (!sameParent) {
        await this.http.request({
          method: 'PATCH',
          path: `/folders/${encodeURIComponent(entry.folder.id)}`,
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ new_parent_id: newParentId }),
        });
      }
      if (newName !== entry.folder.name) {
        await this.http.request({
          method: 'PATCH',
          path: `/folders/${encodeURIComponent(entry.folder.id)}`,
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ name: newName }),
        });
      }
    } else {
      if (!sameParent) {
        await this.http.request({
          method: 'PATCH',
          path: `/files/${encodeURIComponent(entry.file.id)}`,
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ new_folder_id: newParentId }),
        });
      }
      if (newName !== entry.file.name) {
        await this.http.request({
          method: 'PATCH',
          path: `/files/${encodeURIComponent(entry.file.id)}`,
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ name: newName }),
        });
      }
    }
    this.folderIds.clear();
  }

  /**
   * Guarda en el espacio del usuario una copia de un adjunto de canal («Guardar en mi
   * Drive»). Autoriza la MEMBRESÍA del canal, no el permiso de «Mi unidad»: el adjunto
   * vive en la carpeta del canal y no es visible fuera de él.
   *
   * No duplica bytes: el servidor referencia el mismo blob y crea solo el metadato.
   */
  async saveChannelFileToMyDrive(
    channelId: string,
    fileId: string,
  ): Promise<{ id: string; name: string }> {
    const res = await this.http.request({
      method: 'POST',
      path: `/channels/${encodeURIComponent(channelId)}/files/${encodeURIComponent(fileId)}/copy`,
    });
    return (await res.json()) as { id: string; name: string };
  }

  /**
   * Copia un fichero EN EL SERVIDOR: `POST /files/{id}/copy` referencia el mismo blob y
   * solo crea el metadato nuevo.
   *
   * Antes descargaba y volvía a subir, con un comentario que afirmaba que «el servicio no
   * expone copia nativa» — y sí la expone desde hace tiempo. Para un fichero grande eso
   * eran sus bytes cruzando la red dos veces, y además rompía la cuota: la resubida
   * contaba almacenamiento nuevo que no se estaba ocupando.
   */
  async copy(from: string, to: string): Promise<void> {
    const entry = await this.resolveEntry(from);
    if (entry.kind !== 'file') {
      throw new HttpError(400, 'Bad Request', from, 'Copiar carpetas no está soportado.');
    }
    const segs = segments(to);
    const destName = segs[segs.length - 1] ?? entry.file.name;
    const destFolderId = await this.resolveFolderId('/' + segs.slice(0, -1).join('/'));

    const copia = await this.http.json<RawFile>({
      method: 'POST',
      path: `/files/${encodeURIComponent(entry.file.id)}/copy`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ dest_folder_id: destFolderId }),
    });
    // El servicio nombra la copia él («Copia de X» si hay colisión), así que el nombre
    // pedido se aplica después y solo si difiere: `copy(a, b)` promete dejarlo en `b`.
    if (copia.name !== destName) {
      await this.http.request({
        method: 'PATCH',
        path: `/files/${encodeURIComponent(copia.id)}`,
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: destName }),
      });
    }
    this.folderIds.clear();
  }

  /** Crea un enlace compartido publico. Devuelve el enlace. */
  async createShare(input: CreateShareInput): Promise<ShareLink> {
    const entry = await this.resolveEntry(input.path);
    // Con destinatario es una compartición con una persona; sin él, el enlace
    // público de siempre. Contraseña y caducidad solo tienen sentido en el
    // enlace: a una persona ya la identifica su sesión.
    const conPersona = Boolean(input.subject);
    const body: Record<string, unknown> = {
      kind: conPersona ? (input.subjectKind ?? 'user') : 'link',
      permission: input.permission === 'edit' ? 'write' : 'read',
    };
    if (conPersona) body.subject = input.subject;
    if (input.expiresAt) body.expires_at = input.expiresAt;
    // El servicio hashea la contraseña (Argon2id); nunca se guarda en claro.
    if (!conPersona && input.password) body.password = input.password;
    if (entry.kind === 'file') body.file_id = entry.file.id;
    else body.folder_id = entry.folder.id;
    // El servicio Go compone la URL con SU `PublicURL` configurado y solo la manda
    // aquí, en la creación (única vez que emite el token). Recomponerla en el
    // cliente con `this.base` sale mal cuando quien llama es un BFF: pasa la URL
    // INTERNA del clúster como `baseUrl` (evita el 502 de un nombre que no resuelve
    // desde un pod) y esa URL interna se colaba en el enlace público.
    const r = await this.http.postJson<{ share: RawShare; url?: string }>('/shares', body);
    return this.toShareLink(r.share, input.path, r.url);
  }

  /** Enlaces compartidos de un recurso concreto. */
  async listShares(path?: string): Promise<ShareLink[]> {
    if (!path) return []; // el servicio no expone un listado global de shares
    const entry = await this.resolveEntry(path);
    const target =
      entry.kind === 'file'
        ? `/files/${encodeURIComponent(entry.file.id)}/shares`
        : `/folders/${encodeURIComponent(entry.folder.id)}/shares`;
    const r = await this.http.json<{ shares: RawShare[] }>({ path: target });
    return (r.shares ?? []).map((s) => this.toShareLink(s, path));
  }

  /** Revoca un enlace compartido. */
  async revokeShare(shareId: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/shares/${encodeURIComponent(shareId)}` });
  }

  private toShareLink(s: RawShare, path: string, url?: string): ShareLink {
    return {
      id: s.id,
      // El token solo se emite al crear; los listados no lo devuelven (sin URL).
      // Con `url` del servicio (creación) se usa tal cual; de lo contrario se
      // recompone con `this.base`, que basta para el caso sin token (cadena vacía).
      url: url ?? (s.token ? `${this.base}/public/${encodeURIComponent(s.token)}` : ''),
      path,
      hasPassword: !!s.has_password,
      // 'write' del servicio = editar; cualquier otra cosa, solo lectura.
      permission: s.permission === 'write' ? 'edit' : 'view',
      kind: s.kind,
      subject: s.subject ?? '',
      ...(s.expires_at ? { expiresAt: s.expires_at } : {}),
    };
  }

  /**
   * URL WebDAV (RFC 4918) de un recurso, para montar el Drive en clientes de
   * sincronizacion. La autenticacion la negocia el cliente WebDAV (Bearer).
   */
  davUrl(path = '/'): string {
    const clean = path.startsWith('/') ? path : `/${path}`;
    return `${this.davRoot}${clean.split('/').map(encodeURIComponent).join('/')}`;
  }
}
