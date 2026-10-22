/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Docs/Sheets/Slides — SDK al servicio Go propio (docs.sytue.eu,
 * dev :8083). Modelo documental + colaboracion en tiempo real (CRDT/OT) sobre
 * WebSocket. Estado en PostgreSQL, blobs en S3. Implementacion clean-room (NO
 * OnlyOffice). El editor propio Astro+Preact consume este SDK. Autorizacion
 * Bearer (JWT de Sytue ID).
 *
 * REST base:  /api/v1
 * Colab WS:   /api/v1/documents/{id}/collab  (ws://)
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface DocsOptions {
  /** URL base, p.ej. https://docs.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/** Tipo de documento ofimatico. */
export type DocKind = 'document' | 'spreadsheet' | 'presentation';

export interface DocumentMeta {
  id: string;
  title: string;
  kind: DocKind;
  ownerId: string;
  createdAt: string;
  updatedAt: string;
  /** Version del documento (para concurrencia optimista). */
  version: number;
  /** ¿Lo ha destacado QUIEN pregunta? El servicio lo calcula por solicitante
   *  (columna starred_by), asi que no es un atributo del documento sino de la
   *  relacion entre el documento y tu. */
  starred: boolean;
  /** Fichero del que nacio el documento («drive:<uuid>»), si vino de fuera. Lo que
   *  hace que abrirlo otra vez desde Drive vuelva aqui en vez de dejar otra copia. */
  sourceRef?: string;
}

export interface CreateDocumentInput {
  title: string;
  kind: DocKind;
  /** Ancla el documento al fichero del que sale («drive:<uuid>»). Con el, crear es
   *  idempotente: si esa persona ya abrio ese fichero, el servicio devuelve el
   *  documento que ya habia (`created: false`) en lugar de una copia nueva. */
  sourceRef?: string;
  /** Plantilla de la galería («carta», «presupuesto»…) de la que nace. El servicio
   *  siembra su contenido en la misma petición y su tipo manda sobre `kind`; con
   *  `title` vacío, el documento se llama como la plantilla. */
  template?: string;
  /** Idioma del texto de la plantilla (el de la sesión). */
  locale?: string;
  /** Con una plantilla de la organización («org:<id>»): la versión publicada que se
   *  copia (`DocTemplateVersion.id`); sin ella, la vigente. */
  templateVersion?: string;
  /** Tema (del catálogo común o «marca») que se aplica al contenido de la plantilla al
   *  crear; `brandVersion` elige una versión de la marca (sin ella, la vigente). */
  theme?: string;
  brandVersion?: number;
  // Aquí había un `drivePath?: string` documentado como «carpeta de Sytue Drive donde
  // anclar el documento». El servicio Go solo acepta `kind` y `title`: se enviaba y se
  // descartaba en silencio, así que el tipo prometía un anclaje que nunca ocurrió. Un
  // documento de Docs no tiene carpeta; desde Drive se ven en la sección «Documentos».
}

/** Una plantilla de la galería de «Crear nuevo» tal como la da
 *  `GET /api/v1/templates?locale=` (ya en ese idioma). */
export interface DocTemplate {
  key: string;
  kind: DocKind;
  name: string;
  description: string;
  /** Solo en las de la ORGANIZACIÓN (clave «org:<id>»): sus versiones publicadas, la
   *  vigente primero. Las de Sytue no las tienen. */
  versions?: DocTemplateVersion[];
}

/** Una versión publicada de una plantilla de la organización. */
export interface DocTemplateVersion {
  /** Número de versión: la primera es la 1. */
  no: number;
  id: string;
  author_sub: string;
  created_at: string;
}

/** Un tema del catálogo común de Docus, Calcus y Diapos, o la marca de la organización
 *  (`brand`, con su `name` y `version`). Colores «#RRGGBB». */
export interface DocTheme {
  id: string;
  brand?: boolean;
  name?: string;
  version?: number;
  bg: string;
  ink: string;
  accent: string;
  accent_ink: string;
  heading_font?: string;
  body_font?: string;
}

/** Una versión de la marca de la organización. */
export interface DocBrandVersion {
  version: number;
  name: string;
  bg: string;
  ink: string;
  accent: string;
  heading_font?: string;
  body_font?: string;
  created_by?: string;
  created_at: string;
}

/** Respuesta de `GET /api/v1/themes`. */
export interface DocThemesResponse {
  themes: DocTheme[];
  brand: DocTheme | null;
  versions: DocBrandVersion[];
  can_manage: boolean;
}

/** Respuesta de `GET /api/v1/templates`: las de Sytue, las de la organización y si
 *  quien mira puede marcar plantillas (administra la organización). */
export interface DocTemplatesResponse {
  templates: DocTemplate[];
  org_templates?: DocTemplate[];
  can_manage?: boolean;
}

/**
 * Qué puede hacer quien abre un documento. Los nombres son los del servicio
 * (`services/docs/internal/domain`), no unos propios: dos vocabularios para lo
 * mismo se desincronizan a la primera.
 */
export type DocumentRole = 'owner' | 'editor' | 'commenter' | 'viewer' | 'none';

/**
 * Normaliza el rol del cable. Lo que no se reconoce es 'none' y NO 'viewer': ante
 * la duda, la interfaz debe ofrecer menos y no más — un permiso inventado al alza
 * es exactamente el fallo que esto viene a cerrar.
 */
function toRole(raw: unknown): DocumentRole {
  return raw === 'owner' || raw === 'editor' || raw === 'commenter' || raw === 'viewer'
    ? raw
    : 'none';
}

/** ¿Este rol permite escribir? La escalera la fija el servicio: viewer < commenter < editor < owner. */
export function puedeEscribir(role: DocumentRole): boolean {
  return role === 'editor' || role === 'owner';
}

export class DocsClient {
  private readonly http: HttpClient;
  private readonly base: string;

  constructor(options: DocsOptions) {
    this.base = options.baseUrl.replace(/\/$/, '');
    this.http = new HttpClient({
      baseUrl: `${this.base}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /**
   * Lista los documentos accesibles por el usuario, paginado por keyset
   * (updated_at DESC, id DESC). `q` filtra por título EN EL SERVIDOR (índice
   * trigram). Si la página vino llena, `nextCursor` pide la siguiente.
   */
  async list(options?: {
    kind?: DocKind;
    /** Filtro por título (lo resuelve el servicio Go; nunca traer todo al cliente). */
    q?: string;
    /** Tamaño de página (el servicio acota a 1..200; por defecto 50). */
    limit?: number;
    cursor?: string;
    /** Solo los DESTACADOS por quien pregunta. Lo filtra el servicio: con
     *  paginacion keyset, filtrar la pagina traida enseñaria «ninguno» mientras
     *  los destacados esperan tres paginas mas alla. */
    starred?: boolean;
  }): Promise<{ documents: DocumentMeta[]; nextCursor?: string }> {
    const r = await this.http.json<{ documents: RawDocument[]; next_cursor?: string }>({
      path: '/documents',
      query: {
        kind: options?.kind,
        q: options?.q,
        limit: options?.limit ?? 50,
        cursor: options?.cursor,
        ...(options?.starred ? { starred: '1' } : {}),
      },
    });
    return {
      documents: (r.documents ?? []).map(toMeta),
      ...(r.next_cursor ? { nextCursor: r.next_cursor } : {}),
    };
  }

  /** Metadatos de un documento. */
  /**
   * Un documento y CUÁL ES MI PAPEL en él.
   *
   * El rol venía en la respuesta del servicio desde siempre y este método lo
   * descartaba, así que el editor no tenía forma de saber si quien abre puede
   * escribir: dejaba teclear a un lector y el fallo aparecía al guardar, con un
   * 4xx y el trabajo ya hecho. Devolverlo junto al documento —y no en una llamada
   * aparte— evita además que se pinte el lienzo antes de saber si es editable.
   */
  async get(
    id: string,
  ): Promise<{ document: DocumentMeta; role: DocumentRole; editRequested: boolean }> {
    const r = await this.http.json<{
      document: RawDocument;
      role?: string;
      edit_requested?: boolean;
    }>({
      path: `/documents/${encodeURIComponent(id)}`,
    });
    // editRequested: quien solo lee ya ha pedido editarlo (la franja de solo lectura).
    return {
      document: toMeta(r.document),
      role: toRole(r.role),
      editRequested: r.edit_requested === true,
    };
  }

  /**
   * Crea un documento vacio.
   *
   * Con `sourceRef` la operacion es idempotente y `created` dice cual de las dos
   * cosas paso: false significa que el documento ya existia y que sembrarlo con el
   * contenido del fichero pisaria lo que su dueño lleve escrito encima.
   */
  async create(input: CreateDocumentInput): Promise<DocumentMeta & { created: boolean }> {
    const res = await this.http.request({
      method: 'POST',
      path: '/documents',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        title: input.title,
        kind: input.kind,
        ...(input.sourceRef ? { source_ref: input.sourceRef } : {}),
        ...(input.template ? { template: input.template } : {}),
        ...(input.locale ? { locale: input.locale } : {}),
        ...(input.templateVersion ? { template_version: input.templateVersion } : {}),
        ...(input.theme ? { theme: input.theme } : {}),
        ...(input.brandVersion ? { brand_version: input.brandVersion } : {}),
      }),
    });
    const body = (await res.json()) as { document: RawDocument };
    return { ...toMeta(body.document), created: res.status === 201 };
  }

  /** El documento nacido de un fichero concreto («drive:<uuid>»), o null si aun no
   *  se ha abierto nunca. Evita descargar y convertir el fichero para descubrirlo. */
  async bySource(sourceRef: string): Promise<DocumentMeta | null> {
    const r = await this.http.json<{ documents: RawDocument[] }>({
      path: '/documents',
      query: { source_ref: sourceRef, limit: 1 },
    });
    const [first] = r.documents ?? [];
    return first ? toMeta(first) : null;
  }

  /** Renombra un documento. */
  async rename(id: string, title: string): Promise<DocumentMeta> {
    const r = await this.http.json<{ document: RawDocument }>({
      method: 'PATCH',
      path: `/documents/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ title }),
    });
    return toMeta(r.document);
  }

  /** Destaca (o deja de destacar) el documento PARA TI. El estado vive en el
   *  servidor, no en el navegador: se ve igual desde otro equipo. */
  async star(id: string, starred: boolean): Promise<void> {
    await this.http.request({
      method: 'PUT',
      path: `/documents/${encodeURIComponent(id)}/star`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ starred }),
    });
  }

  /** Elimina un documento. */
  async remove(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/documents/${encodeURIComponent(id)}` });
  }

  /**
   * Materializa el documento al formato pedido y registra el blob en S3 (paso 1
   * del export). Formatos que el backend serializa: 'pdf' | 'html' | 'markdown' |
   * 'odf' | 'ooxml'. El PDF sale accesible (etiquetado, PDF/UA-1) salvo con
   * `accessible = false`, que pide el PDF simple de siempre.
   */
  async createExport(
    id: string,
    format: string,
    template = false,
    accessible = true,
  ): Promise<ExportBlob> {
    const r = await this.http.postJson<{ export: RawBlob }>(
      `/documents/${encodeURIComponent(id)}/exports`,
      {
        format,
        ...(template ? { template: true } : {}),
        ...(accessible ? {} : { accessible: false }),
      },
    );
    return toBlob(r.export);
  }

  /**
   * Descarga los bytes de un export ya materializado (paso 2). Usa el streaming
   * inline del servicio (?inline=true): la URL prefirmada de S3 apunta al
   * almacenamiento interno (minio) y NO es alcanzable desde el navegador, así que
   * el BFF hace de proxy de estos bytes.
   */
  async downloadExport(
    id: string,
    blobId: string,
  ): Promise<{ bytes: ArrayBuffer; contentType: string }> {
    const res = await this.http.request({
      path: `/documents/${encodeURIComponent(id)}/exports/${encodeURIComponent(blobId)}`,
      query: { inline: 'true' },
    });
    return {
      bytes: await res.arrayBuffer(),
      contentType: res.headers.get('content-type') ?? 'application/octet-stream',
    };
  }

  /**
   * URL del canal WebSocket de colaboracion (CRDT/OT) de un documento. El editor
   * abre el socket y sincroniza ediciones. El token va por query porque
   * WebSocket no admite cabeceras personalizadas; el servicio Go lo valida via
   * JWKS de Sytue ID.
   */
  async collabUrl(id: string): Promise<string> {
    const token = await this.http.accessToken();
    return this.http.wsUrl({
      path: `/documents/${encodeURIComponent(id)}/ws`,
      query: { access_token: token },
    });
  }

  /** Abre el WebSocket de colaboracion (azucar sobre collabUrl). */
  async openCollab(id: string): Promise<WebSocket> {
    return new WebSocket(await this.collabUrl(id));
  }
}

/** Metadatos de un export materializado (blob en S3). */
export interface ExportBlob {
  id: string;
  format: string;
  /** Content-Type con el que servir la descarga (p.ej. text/html; charset=utf-8). */
  contentType: string;
  sizeBytes: number;
}

/** Blob tal y como lo emite el servicio Go (snake_case). */
interface RawBlob {
  id: string;
  format: string;
  content_type: string;
  size_bytes: number;
}

function toBlob(b: RawBlob): ExportBlob {
  return { id: b.id, format: b.format, contentType: b.content_type, sizeBytes: b.size_bytes };
}

/** Documento tal y como lo emite el servicio Go (snake_case, seq como versión). */
interface RawDocument {
  id: string;
  title: string;
  kind: DocKind;
  owner_sub: string;
  seq: number;
  created_at: string;
  updated_at: string;
  starred?: boolean;
  source_ref?: string;
}

/** Mapea el shape del servicio Go al DocumentMeta del SDK (camelCase). */
function toMeta(d: RawDocument): DocumentMeta {
  return {
    id: d.id,
    title: d.title,
    kind: d.kind,
    ownerId: d.owner_sub,
    createdAt: d.created_at,
    updatedAt: d.updated_at,
    version: d.seq,
    starred: d.starred === true,
    ...(d.source_ref ? { sourceRef: d.source_ref } : {}),
  };
}
