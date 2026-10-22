/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Conector de datos externos (Power Query para Sheets) — helpers PUROS (sin DOM ni
 * Preact): construcción de los cuerpos que la UI envía al BFF (crear conexión /
 * ejecutar consulta), mapeo del resultado tabular del backend ({columns, rows}) a
 * una rejilla de celdas, saneado de la definición de tabla externa persistida en
 * el documento y mensajes de error legibles. Toda la lógica testeable vive aquí;
 * el BFF (api/dataquery.ts, api/connections.ts) SOLO proxya a services/docs
 * (memoria «backend siempre Go») y el backend REVALIDA cada consulta (guard de
 * SELECT único en PostgreSQL, barrera SSRF en REST).
 *
 * Modelo de seguridad: las credenciales viajan del navegador al backend SOLO en
 * el POST de crear/editar conexión (por HTTPS, vía el BFF), se cifran at-rest y
 * NUNCA vuelven al cliente ni se guardan en el documento. La definición que vive
 * en el SheetModel (ExternalTable) referencia la conexión por id y guarda la
 * consulta (sin secretos) para poder REFRESCAR.
 *
 * Self-test: packages/office/test/dataquery.test.ts (bun test).
 */
import type {
  ExternalTable,
  ExtProvenance,
  ExtQuerySpec,
  ExtRestQuery,
  ExtPostgresQuery,
  QueryAggFn,
  QueryColType,
  QueryFilterOp,
  QueryFilterStep,
  QueryGroupByStep,
  QueryJoinType,
  QueryMergeStep,
  QueryReorderStep,
  QuerySelectStep,
  QuerySortStep,
  QueryStep,
  QueryKeepRowsStep,
  QueryRemoveColumnsStep,
  QueryRemoveRowsStep,
} from './types';
import type { QueryTable } from './sheets/querytransform';
import { editorT } from '../i18n';

/* ---------- Tipos espejo del backend (services/docs/internal/connectors) ---------- */

/**
 * Tipo de origen externo. `gateway` es una aplicación YA CONECTADA de la
 * organización (Sytue Integration Gateway): no lleva credencial propia —la guarda
 * la pasarela— y se identifica por la instancia elegida de una lista.
 */
export type ConnKind = 'rest' | 'postgres' | 'gateway' | 'bases';

/** Esquema de autenticación de una API REST. */
export type RestAuthType = 'none' | 'api_key' | 'bearer' | 'basic' | 'oauth2_client_credentials';

/** Config de auth REST (sin el secreto; el secreto va en `credentials`). */
export interface RestAuthCfg {
  type: RestAuthType;
  /** Solo api_key: dónde va la clave. */
  in?: 'header' | 'query';
  /** Solo api_key: nombre de la cabecera/parámetro. */
  name?: string;
  /** Solo basic: usuario (público; la contraseña va cifrada). */
  username?: string;
  /* --- Solo oauth2_client_credentials (el client_secret va cifrado en `credentials`) --- */
  /** Token endpoint (URL externa; el backend la valida contra SSRF al pedir el token). */
  token_url?: string;
  /** Identificador público del cliente. */
  client_id?: string;
  /** Ámbitos solicitados (separados por espacios); opcional. */
  scope?: string;
  /** Audiencia del token (p. ej. Auth0); opcional. */
  audience?: string;
}

/** Config pública de una conexión REST. */
export interface RestConnConfig {
  base_url: string;
  auth: RestAuthCfg;
  headers?: Record<string, string>;
}

/** Config pública de una conexión PostgreSQL (la contraseña va cifrada aparte). */
export interface PgConnConfig {
  host: string;
  port?: number;
  database: string;
  user: string;
  sslmode?: string;
}

/** Config pública de una conexión por pasarela: SOLO la instancia de la app conectada
 *  (la raíz del gateway es config de SERVICIO del backend, nunca del usuario). */
export interface GatewayConnConfig {
  instance_id: string;
}

/** Una aplicación conectada de la organización, elegible como origen (BFF
 *  /api/gateway-instances). `driver` determina cómo se escribe la consulta. */
export interface GatewayInstance {
  id: string;
  name: string;
  /** Nombre de la aplicación en el catálogo (p. ej. «Jira Cloud»). */
  app: string;
  /** Driver tabular del gateway: odata | graph | odoo | jira | salesforce… */
  driver: string;
}

/** Conexión tal y como la lista el backend (la config es pública; el secreto
 *  NUNCA se expone: solo `has_credentials`). */
export interface DataConnection {
  id: string;
  name: string;
  kind: ConnKind;
  config: RestConnConfig | PgConnConfig | Record<string, unknown>;
  has_credentials: boolean;
}

/** Columna del resultado: nombre + tipo lógico inferido por el backend. */
export interface Column {
  name: string;
  type: string;
}

/** Tabla materializada devuelta por POST execute. */
export interface ExecuteResult {
  columns: Column[];
  rows: unknown[][];
  /** Procedencia declarada por el origen (Sytue Bases); ausente en los demás. */
  provenance?: ExtProvenance;
}

/** Cuerpo de crear/editar conexión (POST/PATCH /api/connections). */
export interface ConnectionBody {
  name: string;
  kind: ConnKind;
  config: RestConnConfig | PgConnConfig | GatewayConnConfig | Record<string, never>;
  /** Secreto en claro (solo de navegador→backend por HTTPS; nunca se guarda en el cliente). */
  credentials?: { secret: string };
}

/** Cuerpo de ejecutar/refrescar (POST /api/dataquery), modo inline. */
export interface ExecuteBody {
  connection_id: string;
  query: ExtQuerySpec;
}

/* ---------- Formularios de la UI (estado del gestor) ---------- */

export interface ConnectionForm {
  name: string;
  kind: ConnKind;
  /* Pasarela: id de la instancia elegida de la lista de apps conectadas. */
  instanceId: string;
  /* REST */
  baseUrl: string;
  authType: RestAuthType;
  apiKeyIn: 'header' | 'query';
  apiKeyName: string;
  username: string;
  /* REST OAuth2 client_credentials (el client_secret va en `secret`) */
  tokenUrl: string;
  clientId: string;
  scope: string;
  audience: string;
  /* PostgreSQL */
  host: string;
  port: string;
  database: string;
  user: string;
  sslmode: string;
  /* Secreto (api-key / token / contraseña / client_secret), según auth/kind. */
  secret: string;
}

export interface QueryParam {
  key: string;
  value: string;
}

export interface QueryForm {
  kind: ConnKind;
  /* REST */
  method: 'GET' | 'POST';
  path: string;
  params: QueryParam[];
  rootPath: string;
  /* PostgreSQL */
  sql: string;
  /* Pasarela: los dos campos que puede pedir un driver (ver gatewayQueryFields;
     cada driver usa uno, el otro o los dos). */
  dataset: string;
  gatewayQuery: string;
}

/** Un campo de la consulta a una app conectada: cómo se llama para el usuario y con
 *  qué clave viaja en la spec que el gateway reenvía a su driver. */
export interface GatewayField {
  /** Clave en la spec del driver (dataset, jql, filter, query…). */
  key: string;
  label: string;
  placeholder: string;
  hint: string;
  required: boolean;
  /** Es un lenguaje de consulta (se pinta en monoespaciado, área de texto). */
  code?: boolean;
}

/** Los campos que pide la consulta de una app conectada, según su driver. */
export interface GatewayQueryFields {
  dataset?: GatewayField;
  query?: GatewayField;
}

const DATASET = (label: string, placeholder: string, hint: string): GatewayField => ({
  key: 'dataset',
  label,
  placeholder,
  hint,
  required: true,
});

/**
 * Qué hay que rellenar para traer datos de cada aplicación conectada. La forma de la
 * consulta la define el DRIVER del gateway (services/integrations/internal/driver),
 * no Sytue Docs: por eso cada uno pide lo suyo con su nombre real —una colección
 * OData no es una consulta JQL— y la clave con la que viaja es la que ese driver
 * entiende. Un driver desconocido cae al par genérico (conjunto + consulta), que es
 * el alias que aceptan todos.
 */
export function gatewayQueryFields(driver: string): GatewayQueryFields {
  switch (driver) {
    case 'odata':
      return {
        dataset: DATASET(
          'Colección',
          'Products',
          'El nombre del entity set tal y como lo publica el servicio OData (aparece en su $metadata).',
        ),
        query: {
          key: 'filter',
          label: 'Filtro (opcional)',
          placeholder: "Country eq 'ES'",
          hint: editorT()('extdata.q.odata'),
          required: false,
        },
      };
    case 'graph':
      return {
        dataset: DATASET(
          'Qué quieres listar',
          'users',
          'El recurso de Microsoft Graph: users, groups, devices…',
        ),
        query: {
          key: 'filter',
          label: 'Filtro (opcional)',
          placeholder: 'accountEnabled eq true',
          hint: editorT()('extdata.q.graph'),
          required: false,
        },
      };
    case 'gws':
      return {
        dataset: DATASET(
          'Qué quieres listar',
          'users',
          'users o groups del directorio de Google Workspace.',
        ),
        query: {
          key: 'query',
          label: 'Filtro (opcional)',
          placeholder: 'orgUnitPath=/Ventas',
          hint: editorT()('extdata.q.google'),
          required: false,
        },
      };
    case 'odoo':
      return {
        dataset: DATASET(
          'Modelo',
          'res.partner',
          'El nombre técnico del modelo de Odoo (lo ves en la URL o en el modo desarrollador).',
        ),
      };
    case 'holded':
      return {
        dataset: DATASET(
          'Colección',
          'documents/invoice',
          'contacts, products o documents/<tipo>: invoice, purchase, creditnote, salesorder, purchaseorder o estimate.',
        ),
      };
    case 'servicenow':
      return {
        dataset: DATASET('Tabla', 'incident', 'La tabla de ServiceNow que quieres leer.'),
        query: {
          key: 'query',
          label: 'Filtro (opcional)',
          placeholder: 'active=true',
          hint: editorT()('extdata.q.servicenow'),
          required: false,
        },
      };
    case 'powerbi':
      return {
        dataset: DATASET(
          'Identificador del dataset',
          '00000000-0000-0000-0000-000000000000',
          'El id del dataset en Power BI (está en la URL del informe).',
        ),
        query: {
          key: 'query',
          label: 'Consulta DAX',
          placeholder: 'EVALUATE TOPN(100, Ventas)',
          hint: editorT()('extdata.q.powerbi'),
          required: true,
          code: true,
        },
      };
    case 'jira':
      return {
        query: {
          key: 'jql',
          label: 'Consulta JQL',
          placeholder: 'project = ACME AND status = "In Progress"',
          hint: editorT()('extdata.q.jira'),
          required: true,
          code: true,
        },
      };
    case 'salesforce':
      return {
        query: {
          key: 'query',
          label: 'Consulta SOQL',
          placeholder: 'SELECT Id, Name FROM Account',
          hint: editorT()('extdata.q.salesforce'),
          required: true,
          code: true,
        },
      };
    case 'databricks':
      return {
        query: {
          key: 'query',
          label: 'Consulta SQL',
          placeholder: 'SELECT * FROM ventas LIMIT 100',
          hint: editorT()('extdata.q.databricks'),
          required: true,
          code: true,
        },
      };
    default:
      return {
        dataset: {
          key: 'dataset',
          label: editorT()('extdata.field.dataset'),
          placeholder: '',
          hint: editorT()('extdata.field.dataset.hint'),
          required: false,
        },
        query: {
          key: 'query',
          label: 'Consulta (opcional)',
          placeholder: '',
          hint: editorT()('extdata.q.generic'),
          required: false,
          code: true,
        },
      };
  }
}

export const SSL_MODES: readonly string[] = [
  'prefer',
  'require',
  'verify-full',
  'verify-ca',
  'allow',
  'disable',
];

/* ---------- Mapeo resultado → celdas ---------- */

/**
 * Texto crudo de una celda a partir de un valor JSON-nativo del backend. Los
 * anidados (json.RawMessage: objeto/array) se serializan a texto JSON compacto.
 * ponytail: los números se emiten con punto decimal (canónico JSON); si el motor
 * es-ES no los cuenta como número, convertir aquí a coma es el punto de mejora.
 */
export function cellText(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : '';
  if (typeof v === 'boolean') return v ? 'VERDADERO' : 'FALSO';
  try {
    return JSON.stringify(v);
  } catch {
    return '';
  }
}

/**
 * Un dato EXTERNO como literal de celda. En la hoja, todo texto que empieza por «=» es
 * una fórmula; un origen de datos no es de fiar (en Sytue Bases un registro puede venir
 * de un formulario público, sin sesión), así que lo que traiga un «=» delante —también
 * tras espacios, que la celda recorta al re-renderizarse— se guarda con un apóstrofo:
 * se ve el texto y no se evalúa nada. Vale para cabeceras y valores de TODO conector.
 */
export function literalDeDato(txt: string): string {
  return /^\s*=/.test(txt) ? `'${txt}` : txt;
}

/**
 * Convierte {columns, rows} en una rejilla de texto: 1.ª fila = nombres de
 * columna, luego una fila por registro con un valor por columna (rellena huecos
 * de filas irregulares con ''). Sin recorte: el llamador acota a la rejilla de la
 * hoja (26×100) al materializar y al previsualizar.
 */
export function resultToGrid(result: ExecuteResult): string[][] {
  const header = result.columns.map((c) => c.name);
  const rows = result.rows.map((row) => header.map((_, i) => cellText(row[i])));
  return [header, ...rows];
}

/**
 * Convierte una rejilla ya MATERIALIZADA (1.ª fila = nombres de columna, resto =
 * filas de texto) en una QueryTable para que el motor la use como la «otra» tabla
 * de un paso append/merge. Todas las columnas quedan de tipo 'text' (el motor
 * coacciona el texto al comparar claves o agregar). Rejilla vacía/sin cabecera →
 * null. Es la inversa práctica de `resultToGrid`, compartida por la vista previa
 * del gestor y el materializado (SheetCanvas): ambos leen el SNAPSHOT de la hoja.
 */
export function gridToQueryTable(grid: readonly (readonly string[])[] | null): QueryTable | null {
  if (!grid || grid.length === 0 || (grid[0]?.length ?? 0) === 0) return null;
  const header = grid[0]!;
  const columns = header.map((name, i) => ({
    name: name.trim() === '' ? `Columna ${i + 1}` : name,
    type: 'text',
  }));
  const width = columns.length;
  const rows = grid.slice(1).map((r) => Array.from({ length: width }, (_, i) => r[i] ?? ''));
  return { columns, rows };
}

/* ---------- Construcción de cuerpos para el BFF ---------- */

/** Cuerpo de crear/editar conexión desde el formulario. El backend revalida y
 *  canoniza; aquí solo se ensambla la forma esperada (sin campos vacíos superfluos). */
export function buildConnectionBody(form: ConnectionForm): ConnectionBody {
  const name = form.name.trim();
  // Sytue Bases: sin config (la raíz es del servicio); la credencial es el token de la
  // conexión, que concede y revoca quien es propietario de la base.
  if (form.kind === 'bases') {
    return { name, kind: 'bases', config: {}, credentials: { secret: form.secret.trim() } };
  }
  // Pasarela: no lleva credencial (la guarda el gateway); solo la instancia elegida.
  if (form.kind === 'gateway') {
    return { name, kind: 'gateway', config: { instance_id: form.instanceId.trim() } };
  }
  if (form.kind === 'rest') {
    const auth: RestAuthCfg = { type: form.authType };
    if (form.authType === 'api_key') {
      auth.in = form.apiKeyIn;
      auth.name = form.apiKeyName.trim();
    }
    if (form.authType === 'basic') auth.username = form.username.trim();
    if (form.authType === 'oauth2_client_credentials') {
      auth.token_url = form.tokenUrl.trim();
      auth.client_id = form.clientId.trim();
      const scope = form.scope.trim();
      if (scope !== '') auth.scope = scope;
      const audience = form.audience.trim();
      if (audience !== '') auth.audience = audience;
    }
    const body: ConnectionBody = {
      name,
      kind: 'rest',
      config: { base_url: form.baseUrl.trim(), auth },
    };
    // La api-key/token/contraseña solo se envía si hay auth y secreto tecleado.
    if (form.authType !== 'none' && form.secret !== '') body.credentials = { secret: form.secret };
    return body;
  }
  const config: PgConnConfig = {
    host: form.host.trim(),
    database: form.database.trim(),
    user: form.user.trim(),
  };
  const port = parseInt(form.port, 10);
  if (Number.isFinite(port) && port > 0) config.port = port;
  if (form.sslmode) config.sslmode = form.sslmode;
  const body: ConnectionBody = { name, kind: 'postgres', config };
  if (form.secret !== '') body.credentials = { secret: form.secret };
  return body;
}

/**
 * Especificación de consulta desde el formulario (forma del backend, snake_case).
 * REST siempre produce spec válida (una GET a la URL base es una consulta legítima
 * y sirve de «probar conexión»); PostgreSQL exige SQL no vacío (null si falta). Para
 * una app conectada, `driver` decide qué campos se piden y con qué clave viajan.
 */
export function buildQuerySpec(form: QueryForm, driver = ''): ExtQuerySpec | null {
  // Sytue Bases: nada que consultar; el token ya dice qué tabla o vista.
  if (form.kind === 'bases') return { bases: {} };
  if (form.kind === 'postgres') {
    const sql = form.sql.trim();
    return sql === '' ? null : { postgres: { sql } };
  }
  // Pasarela: la spec es OPACA para docs (la interpreta el driver del gateway); se
  // arma con las claves que ese driver entiende. Falta un campo obligatorio => null.
  if (form.kind === 'gateway') {
    const fields = gatewayQueryFields(driver);
    const values: [GatewayField | undefined, string][] = [
      [fields.dataset, form.dataset.trim()],
      [fields.query, form.gatewayQuery.trim()],
    ];
    const spec: Record<string, string> = {};
    for (const [field, value] of values) {
      if (!field) continue;
      if (value === '') {
        if (field.required) return null;
        continue;
      }
      spec[field.key] = value;
    }
    return Object.keys(spec).length === 0 ? null : { gateway: { spec } };
  }
  const rest: ExtRestQuery = {};
  if (form.method === 'POST') rest.method = 'POST';
  const path = form.path.trim();
  if (path !== '') rest.path = path;
  const query: Record<string, string> = {};
  for (const p of form.params) {
    const k = p.key.trim();
    if (k !== '') query[k] = p.value;
  }
  if (Object.keys(query).length > 0) rest.query = query;
  const rootPath = form.rootPath.trim();
  if (rootPath !== '') rest.root_path = rootPath;
  return { rest };
}

/**
 * Inversa de `buildQuerySpec`: rellena el formulario con una consulta GUARDADA
 * para poder reutilizarla (y editarla) sin reescribirla. Lo que la spec no trae
 * —cabeceras, cuerpo, parámetros posicionales de SQL— no lo pinta el asistente y
 * se pierde al volver a guardar desde aquí; por eso solo se mapea lo que el
 * formulario sabe representar, sin inventar campos.
 *
 * `driver` solo hace falta para las apps conectadas: la spec de la pasarela es
 * OPACA (la define el driver), así que las claves salen de `gatewayQueryFields`,
 * las mismas con las que se compuso.
 */
export function querySpecToForm(spec: ExtQuerySpec, kind: ConnKind, driver = ''): QueryForm {
  const form: QueryForm = {
    kind,
    method: 'GET',
    path: '',
    params: [],
    rootPath: '',
    sql: '',
    dataset: '',
    gatewayQuery: '',
  };
  if (kind === 'bases') return form;
  if (kind === 'postgres') {
    form.sql = typeof spec.postgres?.sql === 'string' ? spec.postgres.sql : '';
    return form;
  }
  if (kind === 'gateway') {
    const fields = gatewayQueryFields(driver);
    const raw = spec.gateway?.spec;
    const opaque = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
    const texto = (key?: string): string => {
      const v = key ? opaque[key] : undefined;
      return typeof v === 'string' ? v : '';
    };
    form.dataset = texto(fields.dataset?.key);
    form.gatewayQuery = texto(fields.query?.key);
    return form;
  }
  const rest = spec.rest ?? {};
  form.method = rest.method === 'POST' ? 'POST' : 'GET';
  form.path = typeof rest.path === 'string' ? rest.path : '';
  form.rootPath = typeof rest.root_path === 'string' ? rest.root_path : '';
  if (rest.query && typeof rest.query === 'object') {
    for (const [key, value] of Object.entries(rest.query)) {
      if (typeof value === 'string') form.params.push({ key, value });
    }
  }
  return form;
}

/** Cuerpo de ejecutar/refrescar (modo inline: conexión + consulta). */
export function buildExecuteBody(connectionId: string, spec: ExtQuerySpec): ExecuteBody {
  return { connection_id: connectionId, query: spec };
}

/** Consulta GUARDADA en el backend (tabla `data_queries`), reutilizable en cualquier
 *  hoja de la organización. La `spec` es la misma forma que viaja en execute. */
export interface SavedQuery {
  id: string;
  name: string;
  spec: ExtQuerySpec;
}

/** Extrae la lista de `{ queries: [...] }`, descartando lo que no encaja. */
export function parseSavedQueries(raw: unknown): SavedQuery[] {
  const list = raw && typeof raw === 'object' ? (raw as { queries?: unknown }).queries : null;
  if (!Array.isArray(list)) return [];
  const out: SavedQuery[] = [];
  for (const q of list) {
    if (!q || typeof q !== 'object') continue;
    const dto = q as Record<string, unknown>;
    if (typeof dto.id !== 'string' || typeof dto.name !== 'string') continue;
    if (!dto.spec || typeof dto.spec !== 'object') continue;
    out.push({ id: dto.id, name: dto.name, spec: dto.spec });
  }
  return out;
}

/* ---------- Parseo de respuestas del backend (frontera de confianza) ---------- */

/** Extrae la lista de conexiones de `{ connections: [...] }`, saneando cada una. */
export function parseConnections(raw: unknown): DataConnection[] {
  const list =
    raw && typeof raw === 'object' ? (raw as { connections?: unknown }).connections : null;
  if (!Array.isArray(list)) return [];
  const out: DataConnection[] = [];
  for (const c of list) {
    const conn = sanitizeConnectionDTO(c);
    if (conn) out.push(conn);
  }
  return out;
}

/** Extrae la conexión de `{ connection: {...} }` (respuesta de crear/editar). */
export function parseConnection(raw: unknown): DataConnection | null {
  const c = raw && typeof raw === 'object' ? (raw as { connection?: unknown }).connection : null;
  return sanitizeConnectionDTO(c);
}

function sanitizeConnectionDTO(raw: unknown): DataConnection | null {
  if (!raw || typeof raw !== 'object') return null;
  const c = raw as Record<string, unknown>;
  if (typeof c.id !== 'string' || typeof c.name !== 'string') return null;
  if (c.kind !== 'rest' && c.kind !== 'postgres' && c.kind !== 'gateway' && c.kind !== 'bases')
    return null;
  return {
    id: c.id,
    name: c.name,
    kind: c.kind,
    config: c.config && typeof c.config === 'object' ? (c.config as Record<string, unknown>) : {},
    has_credentials: c.has_credentials === true,
  };
}

/** Valida y extrae {columns, rows} de la respuesta de execute; null si no encaja. */
export function parseExecuteResult(raw: unknown): ExecuteResult | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as { columns?: unknown; rows?: unknown };
  if (!Array.isArray(r.columns) || !Array.isArray(r.rows)) return null;
  const columns: Column[] = [];
  for (const c of r.columns) {
    if (!c || typeof c !== 'object') return null;
    const col = c as { name?: unknown; type?: unknown };
    if (typeof col.name !== 'string') return null;
    columns.push({ name: col.name, type: typeof col.type === 'string' ? col.type : '' });
  }
  const rows: unknown[][] = [];
  for (const row of r.rows) {
    if (!Array.isArray(row)) return null;
    rows.push(row);
  }
  const provenance = sanitizeProvenance((raw as { provenance?: unknown }).provenance);
  return provenance ? { columns, rows, provenance } : { columns, rows };
}

/**
 * Valida la procedencia: la que declara Sytue Bases (snake_case) o la ya guardada en el
 * documento (camelCase). Solo texto acotado y un número; cualquier otra cosa se descarta.
 */
export function sanitizeProvenance(raw: unknown): ExtProvenance | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const p = raw as Record<string, unknown>;
  if (p.source !== 'sytue-bases') return undefined;
  const texto = (...claves: string[]): string => {
    for (const k of claves) {
      const v = p[k];
      if (typeof v === 'string' && v !== '') return v.slice(0, 200);
    }
    return '';
  };
  const baseName = texto('base_name', 'baseName');
  const tableName = texto('table_name', 'tableName');
  if (baseName === '' || tableName === '') return undefined;
  const version = typeof p.version === 'number' && Number.isFinite(p.version) ? p.version : 1;
  const out: ExtProvenance = {
    source: 'sytue-bases',
    baseName,
    tableName,
    grantedBy: texto('granted_by', 'grantedBy'),
    grantedAt: texto('granted_at', 'grantedAt'),
    version,
  };
  const viewName = texto('view_name', 'viewName');
  if (viewName !== '') out.viewName = viewName;
  return out;
}

/** El tipo de origen de una consulta guardada en el documento. */
export function kindOfSpec(spec: ExtQuerySpec): ConnKind {
  if (spec.bases) return 'bases';
  if (spec.postgres) return 'postgres';
  return spec.gateway ? 'gateway' : 'rest';
}

/* ---------- Saneado de la definición persistida en el documento ---------- */

const A1_RE = /^[A-Z]+[1-9][0-9]*$/;

function isJsonScalar(v: unknown): v is string | number | boolean | null {
  return v === null || typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean';
}

function sanitizeStrMap(raw: unknown): Record<string, string> | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const out: Record<string, string> = {};
  for (const k in raw as Record<string, unknown>) {
    const v = (raw as Record<string, unknown>)[k];
    if (k !== '' && typeof v === 'string') out[k] = v;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** QuerySpec saneada (REST xor PostgreSQL); null si no vale. El backend la
 *  revalida al ejecutar, así que aquí basta con conservar una forma bien formada. */
export function sanitizeQuerySpec(raw: unknown): ExtQuerySpec | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as { rest?: unknown; postgres?: unknown; gateway?: unknown; bases?: unknown };
  if (r.bases && typeof r.bases === 'object') return { bases: {} };
  if (r.gateway && typeof r.gateway === 'object') {
    // La spec del gateway es OPACA (la define el driver): se conserva tal cual si es
    // un objeto JSON; el backend y el driver la revalidan al ejecutar.
    const gw = r.gateway as { spec?: unknown };
    return gw.spec && typeof gw.spec === 'object' ? { gateway: { spec: gw.spec } } : null;
  }
  if (r.postgres && typeof r.postgres === 'object') {
    const pg = r.postgres as { sql?: unknown; params?: unknown };
    if (typeof pg.sql !== 'string' || pg.sql.trim() === '') return null;
    const out: ExtPostgresQuery = { sql: pg.sql };
    if (Array.isArray(pg.params)) {
      const params = pg.params.filter(isJsonScalar);
      if (params.length > 0) out.params = params;
    }
    return { postgres: out };
  }
  if (r.rest && typeof r.rest === 'object') {
    const rest = r.rest as ExtRestQuery;
    const out: ExtRestQuery = {};
    if (rest.method === 'POST') out.method = 'POST';
    if (typeof rest.path === 'string' && rest.path !== '') out.path = rest.path;
    const q = sanitizeStrMap(rest.query);
    if (q) out.query = q;
    const h = sanitizeStrMap(rest.headers);
    if (h) out.headers = h;
    if (typeof rest.root_path === 'string' && rest.root_path !== '') out.root_path = rest.root_path;
    if (rest.body !== undefined) out.body = rest.body; // JSON opaco; el backend lo revalida
    return { rest: out };
  }
  return null;
}

/* ---------- Saneado del pipeline de transformación (pasos aplicados) ---------- */

const FILTER_OPS: readonly QueryFilterOp[] = [
  'eq',
  'neq',
  'contains',
  'gt',
  'lt',
  'between',
  'empty',
  'nonempty',
];
const COL_TYPES: readonly QueryColType[] = ['text', 'number', 'date', 'boolean'];
const AGG_FNS: readonly QueryAggFn[] = ['sum', 'count', 'avg', 'min', 'max', 'first'];
const JOIN_TYPES: readonly QueryJoinType[] = ['inner', 'left', 'right', 'full', 'antiLeft'];

const asString = (v: unknown): string | null => (typeof v === 'string' ? v : null);

/** Lista de nombres de columna (cadenas no vacías); [] si ninguna es válida. */
const asColumnList = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x !== '') : [];

/** Paso «elegir/quitar/reordenar columnas» (comparten forma: kind + columns). */
const colListStep = (
  kind: QuerySelectStep['kind'] | QueryRemoveColumnsStep['kind'] | QueryReorderStep['kind'],
  raw: unknown,
): QuerySelectStep | QueryRemoveColumnsStep | QueryReorderStep | null => {
  const columns = asColumnList(raw);
  return columns.length > 0 ? { kind, columns } : null;
};

/** Paso «mantener/quitar N filas» (comparten forma: kind + end + count). */
const rowCountStep = (
  kind: QueryKeepRowsStep['kind'] | QueryRemoveRowsStep['kind'],
  s: Record<string, unknown>,
): QueryKeepRowsStep | QueryRemoveRowsStep | null => {
  if (typeof s.count !== 'number' || !Number.isFinite(s.count)) return null;
  return {
    kind,
    end: s.end === 'last' ? 'last' : 'first',
    count: Math.max(0, Math.trunc(s.count)),
  };
};

/** Un paso del pipeline saneado desde datos no confiables; null si no encaja. */
function sanitizeStep(raw: unknown): QueryStep | null {
  if (!raw || typeof raw !== 'object') return null;
  const s = raw as Record<string, unknown>;
  switch (s.kind) {
    case 'filter': {
      const column = asString(s.column);
      const op = asString(s.op);
      if (!column || !op || !FILTER_OPS.includes(op as QueryFilterOp)) return null;
      const step: QueryFilterStep = { kind: 'filter', column, op: op as QueryFilterOp };
      if (typeof s.value === 'string') step.value = s.value;
      if (typeof s.value2 === 'string') step.value2 = s.value2;
      return step;
    }
    case 'select':
      return colListStep('select', s.columns);
    case 'removeColumns':
      return colListStep('removeColumns', s.columns);
    case 'reorder':
      return colListStep('reorder', s.columns);
    case 'rename': {
      const column = asString(s.column);
      const to = asString(s.to);
      return column && to && to.trim() !== '' ? { kind: 'rename', column, to } : null;
    }
    case 'changeType': {
      const column = asString(s.column);
      const type = asString(s.type);
      return column && type && COL_TYPES.includes(type as QueryColType)
        ? { kind: 'changeType', column, type: type as QueryColType }
        : null;
    }
    case 'sort': {
      if (!Array.isArray(s.by)) return null;
      const by: QuerySortStep['by'] = [];
      for (const k of s.by) {
        if (!k || typeof k !== 'object') continue;
        const column = asString((k as Record<string, unknown>).column);
        if (column)
          by.push({ column, dir: (k as Record<string, unknown>).dir === 'desc' ? 'desc' : 'asc' });
      }
      return by.length > 0 ? { kind: 'sort', by } : null;
    }
    case 'groupBy': {
      const columns = asColumnList(s.columns);
      if (columns.length === 0 || !Array.isArray(s.aggs)) return null;
      const aggs: QueryGroupByStep['aggs'] = [];
      for (const a of s.aggs) {
        if (!a || typeof a !== 'object') continue;
        const aa = a as Record<string, unknown>;
        const fn = asString(aa.fn);
        const as = asString(aa.as);
        if (!fn || !AGG_FNS.includes(fn as QueryAggFn) || !as || as.trim() === '') continue;
        aggs.push({ column: asString(aa.column) ?? '', fn: fn as QueryAggFn, as });
      }
      return aggs.length > 0 ? { kind: 'groupBy', columns, aggs } : null;
    }
    case 'addColumn': {
      const name = asString(s.name);
      const expr = asString(s.expr);
      return name && name.trim() !== '' && expr !== null ? { kind: 'addColumn', name, expr } : null;
    }
    case 'replace': {
      const column = asString(s.column);
      const find = asString(s.find);
      const replace = asString(s.replace);
      return column && find !== null && replace !== null
        ? { kind: 'replace', column, find, replace }
        : null;
    }
    case 'dedupe': {
      const columns = asColumnList(s.columns);
      return columns.length > 0 ? { kind: 'dedupe', columns } : { kind: 'dedupe' };
    }
    case 'keepRows':
      return rowCountStep('keepRows', s);
    case 'removeRows':
      return rowCountStep('removeRows', s);
    case 'unpivot': {
      const columns = asColumnList(s.columns);
      const attribute = asString(s.attribute);
      const value = asString(s.value);
      return columns.length > 0 &&
        attribute &&
        attribute.trim() !== '' &&
        value &&
        value.trim() !== ''
        ? { kind: 'unpivot', columns, attribute, value }
        : null;
    }
    case 'pivot': {
      const key = asString(s.key);
      const value = asString(s.value);
      const fn = asString(s.fn);
      return key && value && fn && AGG_FNS.includes(fn as QueryAggFn)
        ? { kind: 'pivot', key, value, fn: fn as QueryAggFn }
        : null;
    }
    case 'append': {
      const from = asString(s.from);
      return from && from.trim() !== '' ? { kind: 'append', from: from.trim() } : null;
    }
    case 'merge': {
      const from = asString(s.from);
      const join = asString(s.join);
      if (!from || from.trim() === '' || !join || !JOIN_TYPES.includes(join as QueryJoinType))
        return null;
      if (!Array.isArray(s.on)) return null;
      const on: QueryMergeStep['on'] = [];
      for (const p of s.on) {
        if (!p || typeof p !== 'object') continue;
        const left = asString((p as Record<string, unknown>).left);
        const right = asString((p as Record<string, unknown>).right);
        if (left && right) on.push({ left, right });
      }
      if (on.length === 0) return null;
      const step: QueryMergeStep = {
        kind: 'merge',
        from: from.trim(),
        join: join as QueryJoinType,
        on,
        bring: asColumnList(s.bring),
      };
      const prefix = asString(s.prefix);
      if (prefix !== null && prefix !== '') step.prefix = prefix;
      return step;
    }
    default:
      return null;
  }
}

/** Pipeline de transformación saneado (descarta pasos malformados); [] si ninguno. */
export function sanitizeTransform(raw: unknown): QueryStep[] {
  if (!Array.isArray(raw)) return [];
  const out: QueryStep[] = [];
  for (const s of raw) {
    const step = sanitizeStep(s);
    if (step) out.push(step);
  }
  return out;
}

/**
 * ExternalTable saneada desde datos no confiables (persistencia); null si no vale.
 * Exige el enlace mínimo para refrescar: nombre de tabla, id de conexión, ancla A1
 * y una consulta bien formada. Retrocompatible (Go trata doc:sheet como blob).
 */
export function sanitizeExternalTable(raw: unknown): ExternalTable | null {
  if (!raw || typeof raw !== 'object') return null;
  const t = raw as Partial<ExternalTable>;
  if (typeof t.table !== 'string' || t.table.trim() === '') return null;
  if (typeof t.connectionId !== 'string' || t.connectionId.trim() === '') return null;
  if (typeof t.anchor !== 'string' || !A1_RE.test(t.anchor)) return null;
  const spec = sanitizeQuerySpec(t.spec);
  if (!spec) return null;
  const out: ExternalTable = {
    table: t.table.trim(),
    connectionId: t.connectionId.trim(),
    anchor: t.anchor,
    spec,
  };
  const transform = sanitizeTransform(t.transform);
  if (transform.length > 0) out.transform = transform;
  if (typeof t.refreshedAt === 'string') out.refreshedAt = t.refreshedAt;
  const provenance = sanitizeProvenance(t.provenance);
  if (provenance) out.provenance = provenance;
  return out;
}

/* ---------- Errores ---------- */

/** Mensaje legible a partir del código de error del BFF o del backend. */
export function dataQueryErrorMessage(code?: string): string {
  switch (code) {
    case 'unauthenticated':
      return editorT()('extdata.msg.unauth');
    case 'bad_request':
    case 'invalid_request':
      return editorT()('extdata.msg.badRequest');
    case 'not_found':
      return editorT()('extdata.msg.notFound');
    case 'too_many_requests':
      return editorT()('extdata.msg.rate');
    case 'encryption_unavailable':
      return editorT()('extdata.msg.noEncryption');
    case 'upstream_error':
      return editorT()('extdata.msg.upstream');
    case 'timeout':
      return editorT()('extdata.msg.timeout');
    case 'upstream_unreachable':
      return editorT()('extdata.msg.unreachable');
    case 'connection_not_allowed':
      return editorT()('extdata.msg.notAllowed');
    default:
      return editorT()('extdata.msg.generic');
  }
}

/**
 * Mensaje de error a partir del cuerpo JSON de una respuesta fallida. El backend
 * usa el sobre `{ error: { code, message } }` (message en español, apto para el
 * usuario); el BFF usa `{ error: 'code' }`. Se prefiere el `message` del backend.
 */
export function errorMessageFrom(raw: unknown, fallbackCode?: string): string {
  if (raw && typeof raw === 'object') {
    const e = (raw as { error?: unknown }).error;
    if (typeof e === 'string') return dataQueryErrorMessage(e);
    // No estar en la lista de la conexión (NEG-06) lo dice la interfaz, en el idioma de
    // quien consulta: el `message` del servidor viene en español.
    if (errorCodeFrom(raw) === 'connection_not_allowed') {
      return dataQueryErrorMessage('connection_not_allowed');
    }
    if (e && typeof e === 'object') {
      const msg = (e as { message?: unknown }).message;
      if (typeof msg === 'string' && msg.trim() !== '') return msg;
      const code = (e as { code?: unknown }).code;
      if (typeof code === 'string') return dataQueryErrorMessage(code);
    }
  }
  return dataQueryErrorMessage(fallbackCode);
}

/** Código de error del sobre: `{error:'code'}` (BFF) o `{error:{code}}` (backend). */
function errorCodeFrom(raw: unknown): string {
  if (!raw || typeof raw !== 'object') return '';
  const e = (raw as { error?: unknown }).error;
  if (typeof e === 'string') return e;
  if (e && typeof e === 'object') {
    const code = (e as { code?: unknown }).code;
    if (typeof code === 'string') return code;
  }
  return '';
}

/** `message` del sobre del backend (español, apto para el usuario); '' si no hay. */
function backendMessage(raw: unknown): string {
  if (!raw || typeof raw !== 'object') return '';
  const e = (raw as { error?: unknown }).error;
  if (e && typeof e === 'object') {
    const msg = (e as { message?: unknown }).message;
    if (typeof msg === 'string' && msg.trim() !== '') return msg.trim();
  }
  return '';
}

/** A dónde tiene que ir el usuario para arreglarlo (la UI decide qué botón ofrecer). */
export type QueryFix = 'credential' | 'query' | 'connection' | 'retry' | 'admin' | 'session';

/** Error de una consulta traducido a ACCIÓN, no al error crudo del proveedor. */
export interface QueryAdvice {
  /** Qué ha pasado, en una frase llana. */
  title: string;
  /** Qué hacer ahora, concreto. */
  action: string;
  fix: QueryFix;
}

/**
 * Consejo para un origen tipo PASARELA. Aquí el usuario NO ha configurado ni URL ni
 * credencial —las guarda el administrador en la conexión de la organización—, así
 * que las salidas reales son otras: corregir la consulta o avisar a quien administra
 * esa aplicación conectada. Decirle «revisa la URL base» sería mentirle.
 */
function gatewayErrorAdvice(code: string, raw: unknown): QueryAdvice {
  switch (code) {
    case 'unauthenticated':
    case '401':
      return {
        title: editorT()('extdata.adv.session.title'),
        action: editorT()('extdata.adv.session.action'),
        fix: 'session',
      };
    case 'too_many_requests':
    case '429':
      return {
        title: editorT()('extdata.adv.rate.title'),
        action: editorT()('extdata.adv.rate.action'),
        fix: 'retry',
      };
    case 'timeout':
    case '504':
      return {
        title: editorT()('extdata.adv.timeout.title'),
        action: editorT()('extdata.adv.timeout.action'),
        fix: 'query',
      };
    case 'not_found':
    case '404':
      return {
        title: editorT()('extdata.adv.gone.title'),
        action: editorT()('extdata.adv.gone.action'),
        fix: 'connection',
      };
    case 'bad_request':
    case 'invalid_request':
    case '400':
      return {
        title: backendMessage(raw) || 'La consulta no es válida para esta aplicación.',
        action: editorT()('extdata.adv.badRequest.action'),
        fix: 'query',
      };
    default:
      return {
        title: editorT()('extdata.adv.appError.title'),
        action: editorT()('extdata.adv.appError.action'),
        fix: 'admin',
      };
  }
}

/**
 * Traduce el fallo de una consulta a algo accionable. El backend colapsa los
 * errores del proveedor (401, 404, SQL) en `upstream_error`/500 sin detalle —a
 * propósito: no filtra la respuesta ajena—, así que para esos casos se nombran las
 * DOS causas reales y frecuentes en vez de inventar una que no se puede saber.
 *
 * Self-test: packages/office/test/dataquery.test.ts.
 */
export function queryErrorAdvice(raw: unknown, status: number, kind: ConnKind): QueryAdvice {
  if (kind === 'gateway') return gatewayErrorAdvice(errorCodeFrom(raw) || String(status), raw);
  // Sytue Bases contesta 400 cuando la conexión se ha revocado o ha caducado: no es una
  // consulta mal escrita (no hay consulta), es que hay que pedir un token nuevo.
  if (kind === 'bases' && (status === 400 || errorCodeFrom(raw) === 'bad_request')) {
    return {
      title: editorT()('extdata.bases.revoked.title'),
      action: editorT()('extdata.bases.revoked.action'),
      fix: 'credential',
    };
  }
  const rest = kind === 'rest';
  switch (errorCodeFrom(raw) || String(status)) {
    case 'unauthenticated':
    case '401':
      return {
        title: editorT()('extdata.adv.session.title'),
        action: editorT()('extdata.adv.session.action'),
        fix: 'session',
      };
    case 'too_many_requests':
    case '429':
      return {
        title: editorT()('extdata.adv.rate.title'),
        action: editorT()('extdata.adv.rate.action'),
        fix: 'retry',
      };
    case 'timeout':
    case '504':
      return {
        title: rest
          ? 'La API ha tardado demasiado en contestar.'
          : 'La base de datos ha tardado demasiado en contestar.',
        action: rest
          ? 'Pide menos datos: filtra por parámetros o apunta a una ruta más concreta.'
          : 'Acota la consulta con un WHERE o un LIMIT y vuelve a probar.',
        fix: 'query',
      };
    case 'upstream_unreachable':
      return {
        title: editorT()('extdata.adv.unreachable.title'),
        action: rest
          ? 'Comprueba que la URL base es correcta y que ese servicio es accesible desde internet.'
          : 'Comprueba el host y el puerto, y que la base de datos acepta conexiones desde fuera.',
        fix: 'connection',
      };
    case 'upstream_error':
    case '502':
      return rest
        ? {
            title: editorT()('extdata.adv.apiError.title'),
            action: editorT()('extdata.adv.apiError.action'),
            fix: 'credential',
          }
        : {
            title: editorT()('extdata.adv.dbError.title'),
            action: editorT()('extdata.adv.dbError.action'),
            fix: 'credential',
          };
    case 'not_found':
    case '404':
      return {
        title: editorT()('extdata.adv.sourceGone.title'),
        action: editorT()('extdata.adv.sourceGone.action'),
        fix: 'connection',
      };
    case 'encryption_unavailable':
      return {
        title: editorT()('extdata.adv.noVault.title'),
        action: editorT()('extdata.adv.noVault.action'),
        fix: 'admin',
      };
    case 'bad_request':
    case 'invalid_request':
    case '400':
      return {
        title: backendMessage(raw) || 'Hay algo mal en lo que has escrito.',
        action: rest
          ? 'Revisa la URL base, la ruta y los parámetros de la consulta.'
          : 'La consulta tiene que ser UNA sola sentencia SELECT, sin punto y coma de más.',
        fix: 'query',
      };
    default:
      return {
        title: rest
          ? 'No se han podido traer los datos.'
          : 'La consulta no ha llegado a devolver datos.',
        action: rest
          ? 'Vuelve a probar. Si sigue igual, revisa la credencial y la ruta del origen.'
          : 'Lo más habitual: la tabla no existe con ese nombre, el usuario no tiene permiso de lectura sobre ella, o la contraseña ya no vale.',
        fix: 'credential',
      };
  }
}
