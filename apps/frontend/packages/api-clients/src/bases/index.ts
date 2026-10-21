/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Bases — SDK a la API Go de tablas relacionales colaborativas (puerto 8102,
 * ADR 0068). Bases, tablas, campos tipados, registros con enlaces, vistas guardadas,
 * permisos, auditoría e importar/exportar CSV. Autorización Bearer (client
 * `sytue-bases`, JWT de Sytue ID).
 *
 * `baseUrl` es la RAÍZ de la API, sin añadir nada: desde un servidor,
 * `http://bases:8102/api/v1`; desde el navegador, el proxy del BFF (`/bases/api`),
 * que antepone `/api/v1/` por dentro. El permiso lo decide SIEMPRE el servicio: el rol
 * que devuelve `Base.role` es para pintar la interfaz, no para autorizar.
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface BasesOptions {
  baseUrl: string;
  auth?: AuthProvider | undefined;
  fetchImpl?: typeof fetch | undefined;
}

export type BasesRole = 'owner' | 'editor' | 'commenter' | 'viewer';
export type BasesFieldKind =
  | 'text'
  | 'number'
  | 'date'
  | 'checkbox'
  | 'select'
  | 'multiselect'
  | 'link'
  | 'formula'
  | 'attachment';
export type BasesViewKind = 'grid' | 'kanban' | 'form';

export interface BasesBase {
  id: string;
  name: string;
  description: string;
  owner_sub: string;
  /** Vacío = solo accede a alguna de sus tablas. */
  role: BasesRole | '';
  /** La organización que aloja una base compartida conmigo desde fuera; ausente en las propias. */
  host_org?: string;
  created_at: string;
  updated_at: string;
}

export interface BasesField {
  id: string;
  table_id: string;
  name: string;
  kind: BasesFieldKind;
  choices?: string[];
  link_table_id?: string;
  /** Solo en un campo de fórmula: la expresión (referencias por id) y el tipo de su resultado. */
  formula?: string;
  result?: 'number' | 'text';
  position: number;
}

export interface BasesTable {
  id: string;
  base_id: string;
  name: string;
  position: number;
  role?: BasesRole;
  fields: BasesField[];
}

/** Un valor de celda tal como viaja: texto, número, booleano, lista o null. */
export type BasesValue = string | number | boolean | string[] | null;

/** La ficha de un fichero adjunto. Los bytes se piden aparte (`attachmentPath`). */
export interface BasesAttachment {
  id: string;
  record_id: string;
  field_id: string;
  name: string;
  /** El tipo que dice el contenido del fichero, no quien lo subió. */
  mime: string;
  size: number;
  has_thumb: boolean;
  created_by: string;
  created_at: string;
}

export interface BasesRecord {
  id: string;
  table_id: string;
  data: Record<string, BasesValue>;
  /** Ids de registro enlazados, por campo de enlace. */
  links: Record<string, string[]>;
  /** Los ficheros de cada campo de adjuntos, por campo. */
  attachments?: Record<string, BasesAttachment[]>;
  rev: number;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface BasesRecordPage {
  fields: BasesField[];
  records: BasesRecord[];
  role: BasesRole;
  /** Cursor opaco de la página siguiente; ausente = no hay más. */
  next_cursor?: string;
  /** Hay fórmulas de la tabla pendientes de recalcular: sus valores pueden ir por detrás. */
  recalculating?: boolean;
}

export interface BasesFilter {
  field: string;
  op: string;
  value?: BasesValue;
}

export interface BasesViewConfig {
  filters?: BasesFilter[];
  sort?: { field: string; dir: 'asc' | 'desc' }[];
  group_by?: string;
  fields?: string[];
}

export interface BasesView {
  id: string;
  table_id: string;
  name: string;
  kind: BasesViewKind;
  config: BasesViewConfig;
  created_by: string;
}

export interface BasesPermission {
  id: string;
  table_id?: string;
  /** `external` es una persona de OTRA organización: la base sigue en la anfitriona. */
  principal_kind: 'user' | 'group' | 'tenant' | 'external';
  /** El `sub` de la persona, el nombre del grupo o el slug de la organización. */
  principal_id: string;
  /** Cómo se llama la persona (del directorio, al concederlo). */
  principal_label?: string;
  role: Exclude<BasesRole, 'owner'>;
  created_by: string;
  created_at: string;
}

export interface BasesAuditEntry {
  id: number;
  table_id?: string;
  record_id?: string;
  field_id?: string;
  actor: string;
  action: string;
  before?: unknown;
  after?: unknown;
  at: string;
}

/** Un enlace público a un formulario. El token solo viene al crearlo. */
export interface BasesFormLink {
  id: string;
  view_id: string;
  max_responses: number;
  responses: number;
  expires_at?: string;
  revoked_at?: string;
  created_by: string;
  created_at: string;
}

/** Los productos que pueden leer por una conexión. Todos menos Calcus exigen una vista. */
export const BASES_CONSUMERS = ['calcus', 'drive', 'erp', 'web'] as const;
export type BasesConsumer = (typeof BASES_CONSUMERS)[number];

/** Una conexión de solo lectura hacia UN consumidor. El token solo viene al crearla o rotarla. */
export interface BasesConnection {
  id: string;
  base_id: string;
  table_id: string;
  view_id?: string;
  name: string;
  consumer: string;
  version: number;
  expires_at: string;
  revoked_at?: string;
  reads: number;
  last_read_at?: string;
  created_by: string;
  created_at: string;
}

/** Un comentario de un registro. Los hilos tienen un nivel: `parent_id` es un comentario raíz. */
export interface BasesComment {
  id: string;
  record_id: string;
  parent_id?: string;
  /** Vacío en una lápida (`deleted`). */
  author_sub: string;
  author_name?: string;
  body: string;
  created_at: string;
  /** Para pintar el botón; el borrado lo vuelve a decidir el servicio. */
  can_delete: boolean;
  /** Se borró, pero tiene respuestas: sale sin texto ni autor. */
  deleted?: boolean;
}

/** Versión del contrato de tiempo real que habla este cliente (flujo de cambios y celdas). */
export const BASES_REALTIME_VERSION = 1;

/**
 * Un cambio del flujo en vivo de una base. Dice QUÉ ha cambiado, no trae datos: el
 * registro se vuelve a pedir con `getRecord`, que autoriza la lectura.
 */
export interface BasesChange {
  /** `record.create|update|link|delete`, `field.*`, `table.*`, `view.*`, `comment.*`, `base.update` o `access`. */
  action: string;
  table_id?: string;
  record_id?: string;
  field_id?: string;
  actor?: string;
  /** La operación de celda que lo causó: el cliente reconoce el eco de las suyas. */
  op_id?: string;
}

/** Una pestaña con la base abierta y, si lo dice, la celda en la que está. */
export interface BasesPresence {
  conn: string;
  sub: string;
  name?: string;
  table_id?: string;
  record_id?: string;
  field_id?: string;
}

/** Lo que contesta una operación de celda. */
export interface BasesCellResult {
  record: BasesRecord;
  /** Se ha pisado un valor que quien escribe no había visto (gana el último). */
  conflict: boolean;
  /** La operación ya se había aplicado: no se ha vuelto a escribir. */
  replayed: boolean;
}

/** Un script de usuario de una base (ADR 0069). El código corre aislado en el navegador. */
export type BasesScriptScope = 'private' | 'shared';
export type BasesScriptStatus = 'draft' | 'pending' | 'approved' | 'rejected';
export type BasesScriptOutcome = 'ok' | 'error' | 'denied' | 'timeout' | 'limit';

export interface BasesScript {
  id: string;
  base_id: string;
  name: string;
  scope: BasesScriptScope;
  author_sub: string;
  created_at: string;
  latest_version: number;
  /** 0 = ninguna versión aprobada. */
  approved_version: number;
}

/** Una versión inmutable: editar crea otra, en borrador. */
export interface BasesScriptVersion {
  script_id: string;
  version: number;
  code?: string;
  /** SHA-256 del código: lo que una aprobación avala. */
  code_hash: string;
  /** El manifiesto canónico, tal como lo guarda el servidor. Se valida antes de usarlo. */
  manifest: unknown;
  status: BasesScriptStatus;
  created_by: string;
  created_at: string;
  reviewed_by?: string;
  reviewed_at?: string;
}

/** Lo que el servidor autoriza a ejecutar: exactamente este código con este manifiesto. */
export interface BasesScriptGrant {
  run_id: number;
  code: string;
  code_hash: string;
  manifest: unknown;
}

/** Una línea del registro de ejecuciones. Sin contenido: quién, qué versión y cómo acabó. */
export interface BasesScriptRun {
  id: number;
  script_id: string;
  version: number;
  code_hash: string;
  sub: string;
  outcome: BasesScriptOutcome | 'running';
  error_code?: string;
  duration_ms?: number;
  ops: number;
  started_at: string;
}

export interface BasesScriptQuota {
  used: { user_runs: number; org_runs: number; user_ms: number };
  limits: { user_runs: number; org_runs: number; user_ms: number };
}

export interface BasesNewField {
  name: string;
  kind: BasesFieldKind;
  choices?: string[];
  link_table_id?: string;
  formula?: string;
  result?: 'number' | 'text';
}

export class BasesClient {
  private readonly http: HttpClient;

  constructor(options: BasesOptions) {
    this.http = new HttpClient({
      baseUrl: options.baseUrl.replace(/\/$/, ''),
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  private send<T>(method: string, path: string, body?: unknown): Promise<T> {
    return this.http.json<T>({
      method,
      path,
      ...(body === undefined
        ? {}
        : { body: JSON.stringify(body), headers: { 'content-type': 'application/json' } }),
    });
  }

  private async sendEmpty(method: string, path: string, body?: unknown): Promise<void> {
    await this.http.request({
      method,
      path,
      ...(body === undefined
        ? {}
        : { body: JSON.stringify(body), headers: { 'content-type': 'application/json' } }),
    });
  }

  listBases(): Promise<BasesBase[]> {
    return this.send('GET', '/bases');
  }
  createBase(name: string, description = ''): Promise<BasesBase> {
    return this.send('POST', '/bases', { name, description });
  }
  getBase(id: string): Promise<BasesBase> {
    return this.send('GET', `/bases/${id}`);
  }
  updateBase(id: string, name: string, description = ''): Promise<void> {
    return this.sendEmpty('PATCH', `/bases/${id}`, { name, description });
  }
  deleteBase(id: string): Promise<void> {
    return this.sendEmpty('DELETE', `/bases/${id}`);
  }

  listTables(baseId: string): Promise<BasesTable[]> {
    return this.send('GET', `/bases/${baseId}/tables`);
  }
  createTable(baseId: string, name: string): Promise<BasesTable> {
    return this.send('POST', `/bases/${baseId}/tables`, { name });
  }
  renameTable(tableId: string, name: string): Promise<void> {
    return this.sendEmpty('PATCH', `/tables/${tableId}`, { name });
  }
  deleteTable(tableId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/tables/${tableId}`);
  }

  createField(tableId: string, field: BasesNewField): Promise<BasesField> {
    return this.send('POST', `/tables/${tableId}/fields`, field);
  }
  renameField(fieldId: string, name: string): Promise<void> {
    return this.sendEmpty('PATCH', `/fields/${fieldId}`, { name });
  }
  /** Sube un fichero a un campo de adjuntos de un registro. El tipo lo decide el servicio. */
  uploadAttachment(recordId: string, fieldId: string, file: File): Promise<BasesAttachment> {
    return this.http.json<BasesAttachment>({
      method: 'POST',
      path: `/records/${recordId}/attachments`,
      query: { field: fieldId, name: file.name },
      body: file,
      headers: { 'content-type': 'application/octet-stream' },
    });
  }
  deleteAttachment(recordId: string, attachmentId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/records/${recordId}/attachments/${attachmentId}`);
  }

  /** Cambia la expresión de un campo de fórmula (solo el propietario). */
  setFieldFormula(fieldId: string, formula: string): Promise<void> {
    return this.sendEmpty('PATCH', `/fields/${fieldId}`, { formula });
  }
  deleteField(fieldId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/fields/${fieldId}`);
  }

  listRecords(
    tableId: string,
    opts: { view?: string; limit?: number; cursor?: string } = {},
  ): Promise<BasesRecordPage> {
    return this.http.json<BasesRecordPage>({ path: `/tables/${tableId}/records`, query: opts });
  }
  createRecord(
    tableId: string,
    data: Record<string, BasesValue>,
    links?: Record<string, string[]>,
  ): Promise<BasesRecord> {
    return this.send('POST', `/tables/${tableId}/records`, { data, links });
  }
  /** `rev` es la versión que se vio: si otra persona escribió entretanto, 409 `rev_conflict`. */
  updateRecord(
    recordId: string,
    rev: number,
    data: Record<string, BasesValue>,
    links?: Record<string, string[]>,
  ): Promise<BasesRecord> {
    return this.send('PATCH', `/records/${recordId}`, { rev, data, links });
  }
  getRecord(recordId: string): Promise<BasesRecord> {
    return this.send('GET', `/records/${recordId}`);
  }
  /**
   * Operación de celda: escribe UN campo sin pedir la versión del registro (dos personas
   * en celdas distintas no se estorban; en la misma gana quien escribe después).
   * `expected` es lo que se veía en la celda: si ya no era eso, `conflict` lo dice.
   * `opId` (UUID) hace la operación idempotente: repetirla no la aplica dos veces.
   */
  writeCell(
    recordId: string,
    fieldId: string,
    celda: { value?: BasesValue; links?: string[]; expected?: BasesValue },
    opId: string,
  ): Promise<BasesCellResult> {
    return this.http.json<BasesCellResult>({
      method: 'PUT',
      path: `/records/${recordId}/cells/${fieldId}`,
      query: { v: BASES_REALTIME_VERSION },
      body: JSON.stringify(celda),
      headers: { 'content-type': 'application/json', 'idempotency-key': opId },
    });
  }
  /** Latido de presencia: quién es lo pone el servicio; aquí solo la pestaña y la celda. */
  heartbeat(
    baseId: string,
    p: { conn: string; table_id?: string; record_id?: string; field_id?: string },
  ): Promise<void> {
    return this.sendEmpty('PUT', `/bases/${baseId}/presence`, p);
  }

  /** `force` quita los enlaces que apuntan al registro; solo el propietario. */
  deleteRecord(recordId: string, force = false): Promise<void> {
    return this.sendEmpty('DELETE', `/records/${recordId}${force ? '?force=true' : ''}`);
  }

  listComments(recordId: string): Promise<BasesComment[]> {
    return this.send('GET', `/records/${recordId}/comments`);
  }
  /** Comentar exige rol comentarista o superior. `parentId` responde a un comentario raíz. */
  createComment(recordId: string, body: string, parentId?: string): Promise<BasesComment> {
    return this.send('POST', `/records/${recordId}/comments`, {
      body,
      ...(parentId ? { parent_id: parentId } : {}),
    });
  }
  /** Lo borra quien lo escribió o quien modera (editor o propietario). */
  deleteComment(commentId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/comments/${commentId}`);
  }

  listViews(tableId: string): Promise<BasesView[]> {
    return this.send('GET', `/tables/${tableId}/views`);
  }
  createView(
    tableId: string,
    view: { name: string; kind: BasesViewKind; config: BasesViewConfig },
  ): Promise<BasesView> {
    return this.send('POST', `/tables/${tableId}/views`, view);
  }
  deleteView(viewId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/views/${viewId}`);
  }

  listFormLinks(viewId: string): Promise<BasesFormLink[]> {
    return this.send('GET', `/views/${viewId}/form-links`);
  }
  /** Crea un enlace público (solo el propietario). `token` no se puede volver a leer. */
  createFormLink(
    viewId: string,
    opts: { max_responses?: number; expires_at?: string } = {},
  ): Promise<BasesFormLink & { token: string }> {
    return this.send('POST', `/views/${viewId}/form-links`, opts);
  }
  revokeFormLink(linkId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/form-links/${linkId}`);
  }

  listConnections(baseId: string): Promise<BasesConnection[]> {
    return this.send('GET', `/bases/${baseId}/connections`);
  }
  /** Concede la lectura de una tabla (o de una vista) a un consumidor; solo el propietario. */
  createConnection(
    tableId: string,
    opts: { name: string; view_id?: string; days?: number; consumer?: BasesConsumer },
  ): Promise<BasesConnection & { token: string }> {
    return this.send('POST', `/tables/${tableId}/connections`, opts);
  }
  revokeConnection(connectionId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/connections/${connectionId}`);
  }
  /** Cambia el token: el anterior deja de valer en el acto y la versión sube. */
  rotateConnection(connectionId: string): Promise<{ version: number; token: string }> {
    return this.send('POST', `/connections/${connectionId}/rotate`);
  }

  listPermissions(baseId: string): Promise<BasesPermission[]> {
    return this.send('GET', `/bases/${baseId}/permissions`);
  }
  /**
   * Concede a toda la organización, a un grupo (por su nombre) o a una persona. A una
   * persona se le concede por su `email`: el servicio la resuelve en el directorio y
   * contesta 422 `not_a_member` si no es de la organización. A alguien de OTRA
   * organización (`external`), por `query`: su @usuario o su dirección (422 `is_member`
   * si resulta ser de la propia).
   */
  grantPermission(
    baseId: string,
    p: Pick<BasesPermission, 'principal_kind' | 'role'> & {
      principal_id?: string;
      email?: string;
      query?: string;
      table_id?: string;
    },
  ): Promise<BasesPermission> {
    return this.send('POST', `/bases/${baseId}/permissions`, p);
  }
  revokePermission(baseId: string, permissionId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/bases/${baseId}/permissions/${permissionId}`);
  }

  /** Los grupos de quien llama (los de su token), para ofrecerlos al compartir. */
  async myGroups(): Promise<string[]> {
    return (await this.send<{ groups: string[] }>('GET', '/groups')).groups;
  }
  /**
   * Da la propiedad a otra persona de la organización, por su dirección. Quien llama
   * queda como editor. Solo el propietario.
   */
  transferBase(baseId: string, email: string): Promise<void> {
    return this.sendEmpty('POST', `/bases/${baseId}/transfer`, { email });
  }

  listAudit(
    baseId: string,
    opts: { before?: number; limit?: number } = {},
  ): Promise<BasesAuditEntry[]> {
    return this.http.json<BasesAuditEntry[]>({ path: `/bases/${baseId}/audit`, query: opts });
  }

  // --- Scripts de usuario (ADR 0069): cuelgan de la base ---
  listScripts(baseId: string): Promise<{ scripts: BasesScript[]; can_review: boolean }> {
    return this.send('GET', `/bases/${baseId}/scripts`);
  }
  getScript(
    baseId: string,
    scriptId: string,
  ): Promise<{ script: BasesScript; versions: BasesScriptVersion[]; can_review: boolean }> {
    return this.send('GET', `/bases/${baseId}/scripts/${scriptId}`);
  }
  createScript(
    baseId: string,
    s: { name: string; scope: BasesScriptScope; code: string; manifest: unknown },
  ): Promise<{ script: BasesScript; version: BasesScriptVersion }> {
    return this.send('POST', `/bases/${baseId}/scripts`, s);
  }
  addScriptVersion(
    baseId: string,
    scriptId: string,
    v: { code: string; manifest: unknown },
  ): Promise<{ version: BasesScriptVersion }> {
    return this.send('POST', `/bases/${baseId}/scripts/${scriptId}/versions`, v);
  }
  setScriptScope(baseId: string, scriptId: string, scope: BasesScriptScope): Promise<void> {
    return this.sendEmpty('PUT', `/bases/${baseId}/scripts/${scriptId}/scope`, { scope });
  }
  deleteScript(baseId: string, scriptId: string): Promise<void> {
    return this.sendEmpty('DELETE', `/bases/${baseId}/scripts/${scriptId}`);
  }
  proposeScriptVersion(baseId: string, scriptId: string, version: number): Promise<void> {
    return this.sendEmpty(
      'POST',
      `/bases/${baseId}/scripts/${scriptId}/versions/${version}/propose`,
    );
  }
  /** `codeHash` es el del código que se ha revisado: si no es el de la versión, 409. */
  reviewScriptVersion(
    baseId: string,
    scriptId: string,
    version: number,
    decision: 'approve' | 'reject',
    codeHash: string,
  ): Promise<void> {
    return this.sendEmpty(
      'POST',
      `/bases/${baseId}/scripts/${scriptId}/versions/${version}/review`,
      { decision, code_hash: codeHash },
    );
  }
  /** Autoriza una ejecución y reserva la cuota. 403 sin aprobar o sin permiso; 429 sin cuota. */
  authorizeScriptRun(baseId: string, scriptId: string, version: number): Promise<BasesScriptGrant> {
    return this.send('POST', `/bases/${baseId}/scripts/${scriptId}/versions/${version}/runs`);
  }
  closeScriptRun(
    baseId: string,
    runId: number,
    r: { outcome: BasesScriptOutcome; error_code: string; duration_ms: number; ops: number },
  ): Promise<void> {
    return this.sendEmpty('PUT', `/bases/${baseId}/script-runs/${runId}`, r);
  }
  listScriptRuns(
    baseId: string,
    opts: { before?: number; limit?: number } = {},
  ): Promise<{ runs: BasesScriptRun[]; next_before: number }> {
    return this.http.json({ path: `/bases/${baseId}/script-runs`, query: opts });
  }
  scriptQuota(): Promise<BasesScriptQuota> {
    return this.send('GET', '/script-quota');
  }

  /** Crea una tabla nueva a partir de un CSV (solo el propietario). */
  async importCsv(baseId: string, name: string, csv: string): Promise<BasesTable> {
    return this.http.json<BasesTable>({
      method: 'POST',
      path: `/bases/${baseId}/import`,
      query: { name },
      body: csv,
      headers: { 'content-type': 'text/csv' },
    });
  }
}
