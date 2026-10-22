/**
 * Datos de ejemplo (seed) del control-plane de Sytue DevX para la DX del
 * dashboard. En produccion estos los sirve la Sytue DevX API (PostgREST +
 * control-plane); aqui se proveen como fuente realista para SSR e islas
 * mientras el backend se integra. NUNCA contienen secretos reales: las claves
 * y cadenas de conexion estan enmascaradas. docs/CONVENTIONS.md §10, §14.
 *
 * IDIOMA: lo que aqui hay es CONTENIDO, no interfaz — mensajes de commit, ramas
 * y salida de la cadena de construccion—, y va en ingles porque es lo que emite
 * lo real: git y Kaniko no hablan el idioma del navegador. El texto de interfaz
 * (avisos del evaluador de demostracion) sale del catalogo, como el resto.
 */
import type {
  DbApiInfo,
  DbRows,
  DbTable,
  Deployment,
  Domain,
  EnvVar,
  LogLine,
  ManagedService,
  Project,
  SqlResult,
  UsageMetric,
} from './types.js';
import type { T } from '../i18n/index.js';

export const projects: Project[] = [
  {
    slug: 'tienda-acme',
    name: 'tienda-acme',
    framework: 'astro',
    repo: 'acme/tienda-web',
    productionRef: 'main',
    productionUrl: 'https://tienda-acme.apps.sytue.eu',
    region: 'fr-par',
    tenant: 'acme',
    createdAt: '2026-02-11T09:24:00Z',
    services: ['postgres', 'storage', 'cache'],
    runtime: {
      instanceSize: 'standard',
      instanceCount: 2,
      appPort: 8080,
      healthPath: '/',
      releaseCommand: '',
    },
  },
  {
    slug: 'panel-interno',
    name: 'panel-interno',
    framework: 'preact',
    repo: 'acme/panel-interno',
    productionRef: 'main',
    productionUrl: 'https://panel-interno.apps.sytue.eu',
    region: 'fr-par',
    tenant: 'acme',
    createdAt: '2026-03-02T14:10:00Z',
    services: ['postgres', 'auth', 'functions'],
    runtime: {
      instanceSize: 'basic',
      instanceCount: 1,
      appPort: 8080,
      healthPath: '/',
      releaseCommand: '',
    },
  },
  {
    slug: 'api-pagos',
    name: 'api-pagos',
    framework: 'node',
    repo: 'acme/api-pagos',
    productionRef: 'main',
    productionUrl: 'https://api-pagos.apps.sytue.eu',
    region: 'fr-par',
    tenant: 'acme',
    createdAt: '2026-04-19T08:00:00Z',
    services: ['postgres', 'cache', 'cron'],
    runtime: {
      instanceSize: 'pro',
      instanceCount: 3,
      appPort: 3000,
      healthPath: '/healthz',
      releaseCommand: '',
    },
  },
];

export function getProject(slug: string): Project | undefined {
  return projects.find((p) => p.slug === slug);
}

export const deployments: Deployment[] = [
  {
    id: 'dpl_9Hx2',
    project: 'tienda-acme',
    status: 'ready',
    target: 'production',
    branch: 'main',
    commitSha: 'a3f91c2',
    commitMessage: 'feat: new checkout flow',
    author: 'marta.ruiz',
    url: 'https://tienda-acme.apps.sytue.eu',
    createdAt: '2026-06-30T07:42:00Z',
    durationSec: 38,
    current: true,
  },
  {
    id: 'dpl_8Qa1',
    project: 'tienda-acme',
    status: 'ready',
    target: 'preview',
    branch: 'feat/search',
    pr: 142,
    commitSha: 'b71d04e',
    commitMessage: 'wip: faceted search',
    author: 'jon.alava',
    url: 'https://tienda-acme-pr-142.preview.sytue.eu',
    createdAt: '2026-06-30T06:15:00Z',
    durationSec: 41,
  },
  {
    id: 'dpl_7Zb0',
    project: 'tienda-acme',
    status: 'building',
    target: 'preview',
    branch: 'fix/cart',
    pr: 143,
    commitSha: 'c0a9f12',
    commitMessage: 'fix: negative quantity in cart',
    author: 'marta.ruiz',
    url: 'https://tienda-acme-pr-143.preview.sytue.eu',
    createdAt: '2026-06-30T07:58:00Z',
  },
  {
    id: 'dpl_6Yc9',
    project: 'tienda-acme',
    status: 'error',
    target: 'preview',
    branch: 'chore/deps',
    pr: 141,
    commitSha: 'd4e7a88',
    commitMessage: 'chore: bump dependencies',
    author: 'ci-bot',
    url: 'https://tienda-acme-pr-141.preview.sytue.eu',
    createdAt: '2026-06-29T18:30:00Z',
    durationSec: 22,
  },
  {
    id: 'dpl_5Xd8',
    project: 'tienda-acme',
    status: 'ready',
    target: 'production',
    branch: 'main',
    commitSha: 'e9b1c30',
    commitMessage: 'perf: catalogue cache',
    author: 'jon.alava',
    url: 'https://tienda-acme.apps.sytue.eu',
    createdAt: '2026-06-28T11:05:00Z',
    durationSec: 35,
  },
];

export function deploymentsFor(project: string): Deployment[] {
  return deployments.filter((d) => d.project === project);
}

export const domains: Domain[] = [
  {
    host: 'tienda-acme.apps.sytue.eu',
    kind: 'platform',
    verified: true,
    tls: 'active',
    primary: true,
  },
  {
    host: 'www.tienda-acme.com',
    kind: 'custom',
    verified: true,
    tls: 'active',
    cname: 'ingress.sytue.eu',
  },
  {
    host: 'tienda-acme.com',
    kind: 'custom',
    verified: false,
    tls: 'pending',
    cname: 'ingress.sytue.eu',
  },
];

export const envVars: EnvVar[] = [
  {
    key: 'DATABASE_URL',
    preview: 'postgres://app:••••••@db.tienda-acme.sytue.eu:5432/app',
    secret: true,
    scopes: ['production', 'preview'],
    source: 'postgres',
    updatedAt: '2026-06-11T10:00:00Z',
  },
  {
    key: 'SYTUE_STORAGE_BUCKET',
    preview: 'tienda-acme-assets',
    secret: false,
    scopes: ['production', 'preview', 'development'],
    source: 'storage',
    updatedAt: '2026-06-11T10:00:00Z',
  },
  {
    key: 'CACHE_URL',
    preview: 'postgres://••••••@db.tienda-acme.sytue.eu:5432/cache',
    secret: true,
    scopes: ['production', 'preview'],
    source: 'cache',
    updatedAt: '2026-06-11T10:00:00Z',
  },
  {
    key: 'PAYMENTS_ALT_KEY',
    preview: '••••••••••••',
    secret: true,
    scopes: ['production'],
    source: 'manual',
    updatedAt: '2026-06-20T16:42:00Z',
  },
  {
    key: 'PUBLIC_SITE_URL',
    preview: 'https://www.tienda-acme.com',
    secret: false,
    scopes: ['production'],
    source: 'manual',
    updatedAt: '2026-05-30T09:12:00Z',
  },
];

export const services: ManagedService[] = [
  {
    kind: 'postgres',
    name: 'Sytue Postgres',
    status: 'running',
  },
  {
    kind: 'storage',
    name: 'Sytue Storage',
    status: 'running',
  },
  {
    kind: 'cache',
    name: 'Sytue Cache/Queue',
    status: 'running',
  },
];

/**
 * Consumo de ejemplo del modo demostración: SOLO datos. La etiqueta y el valor
 * que se ven en pantalla los compone `cloud.ts` en el idioma de la petición
 * (catálogo `metric.*` + formato por locale), así que aquí no hay texto de
 * interfaz que se quedaría clavado en un idioma.
 */
export interface UsageSample {
  metric: string;
  quantity: number;
  unit: string;
  delta?: string;
  trend?: UsageMetric['trend'];
}

// Métricas facturables reales del control-plane (build_minutes, compute_gb_hours,
// storage_gb). El modo demostración las muestra con cantidades de ejemplo para que
// el estimador de coste sea coherente (el DemoBanner deja claro que son de ejemplo).
export const usage: UsageSample[] = [
  { metric: 'build_minutes', quantity: 128, unit: 'min', delta: '-6 %', trend: 'down' },
  { metric: 'compute_gb_hours', quantity: 342, unit: 'GB·h', delta: '+4 %', trend: 'up' },
  { metric: 'storage_gb', quantity: 18, unit: 'GB', delta: '+2 %', trend: 'up' },
];

export const buildLogs: LogLine[] = [
  { ts: '07:42:01', level: 'info', source: 'build', message: 'Cloning acme/tienda-web @ a3f91c2' },
  {
    ts: '07:42:03',
    level: 'info',
    source: 'build',
    message: 'Detected framework: Astro 7 (output server)',
  },
  {
    ts: '07:42:04',
    level: 'info',
    source: 'build',
    message: 'Restoring pnpm cache (build-cache@fr-par)',
  },
  { ts: '07:42:09', level: 'info', source: 'build', message: 'pnpm install --frozen-lockfile' },
  { ts: '07:42:22', level: 'info', source: 'build', message: 'astro build (Kaniko, rootless)' },
  {
    ts: '07:42:36',
    level: 'warn',
    source: 'build',
    message: 'Warning: hero image without explicit width/height',
  },
  {
    ts: '07:42:38',
    level: 'info',
    source: 'build',
    message: 'Image pushed to Scaleway Registry (fr-par)',
  },
  {
    ts: '07:42:39',
    level: 'info',
    source: 'runtime',
    message: 'Deploying to production · automatic TLS issued',
  },
  {
    ts: '07:42:39',
    level: 'info',
    source: 'runtime',
    message: 'Ready at https://tienda-acme.apps.sytue.eu',
  },
];

// ---- Sytue DB Studio ----

export const dbTables: DbTable[] = [
  {
    schema: 'public',
    name: 'clientes',
    rowCount: 1842,
    apiExposed: true,
    columns: [
      {
        name: 'id',
        type: 'uuid',
        nullable: false,
        isPrimaryKey: true,
        default: 'gen_random_uuid()',
      },
      { name: 'email', type: 'text', nullable: false, isPrimaryKey: false },
      { name: 'nombre', type: 'text', nullable: true, isPrimaryKey: false },
      {
        name: 'creado_en',
        type: 'timestamptz',
        nullable: false,
        isPrimaryKey: false,
        default: 'now()',
      },
    ],
  },
  {
    schema: 'public',
    name: 'pedidos',
    rowCount: 9530,
    apiExposed: true,
    columns: [
      { name: 'id', type: 'bigint', nullable: false, isPrimaryKey: true, default: 'identity' },
      {
        name: 'cliente_id',
        type: 'uuid',
        nullable: false,
        isPrimaryKey: false,
        references: 'clientes.id',
      },
      { name: 'total_cent', type: 'integer', nullable: false, isPrimaryKey: false },
      {
        name: 'estado',
        type: 'text',
        nullable: false,
        isPrimaryKey: false,
        default: "'pendiente'",
      },
      {
        name: 'creado_en',
        type: 'timestamptz',
        nullable: false,
        isPrimaryKey: false,
        default: 'now()',
      },
    ],
  },
  {
    schema: 'public',
    name: 'productos',
    rowCount: 312,
    apiExposed: true,
    columns: [
      { name: 'sku', type: 'text', nullable: false, isPrimaryKey: true },
      { name: 'nombre', type: 'text', nullable: false, isPrimaryKey: false },
      { name: 'precio_cent', type: 'integer', nullable: false, isPrimaryKey: false },
      { name: 'stock', type: 'integer', nullable: false, isPrimaryKey: false, default: '0' },
    ],
  },
  {
    schema: 'auth',
    name: 'sesiones',
    rowCount: 74,
    apiExposed: false,
    columns: [
      { name: 'id', type: 'uuid', nullable: false, isPrimaryKey: true },
      {
        name: 'cliente_id',
        type: 'uuid',
        nullable: false,
        isPrimaryKey: false,
        references: 'clientes.id',
      },
      { name: 'expira_en', type: 'timestamptz', nullable: false, isPrimaryKey: false },
    ],
  },
];

const TABLE_ROWS: Record<string, DbRows> = {
  'public.clientes': {
    columns: ['id', 'email', 'nombre', 'creado_en'],
    totalRows: 1842,
    rows: [
      {
        id: '7c1f…a02',
        email: 'marta@example.eu',
        nombre: 'Marta Ruiz',
        creado_en: '2026-01-04 09:12',
      },
      {
        id: '9b3a…c41',
        email: 'jon@example.eu',
        nombre: 'Jon Alava',
        creado_en: '2026-01-09 17:33',
      },
      {
        id: 'd5e8…f90',
        email: 'lucia@example.eu',
        nombre: 'Lucia Mota',
        creado_en: '2026-02-02 11:05',
      },
      { id: 'a1c2…b73', email: 'pau@example.eu', nombre: null, creado_en: '2026-02-21 08:47' },
    ],
  },
  'public.pedidos': {
    columns: ['id', 'cliente_id', 'total_cent', 'estado', 'creado_en'],
    totalRows: 9530,
    rows: [
      {
        id: 10231,
        cliente_id: '7c1f…a02',
        total_cent: 4990,
        estado: 'enviado',
        creado_en: '2026-06-29 10:11',
      },
      {
        id: 10232,
        cliente_id: '9b3a…c41',
        total_cent: 12990,
        estado: 'pendiente',
        creado_en: '2026-06-29 12:40',
      },
      {
        id: 10233,
        cliente_id: 'd5e8…f90',
        total_cent: 2500,
        estado: 'entregado',
        creado_en: '2026-06-30 06:02',
      },
    ],
  },
  'public.productos': {
    columns: ['sku', 'nombre', 'precio_cent', 'stock'],
    totalRows: 312,
    rows: [
      { sku: 'CAM-001', nombre: 'Camiseta Sytue', precio_cent: 1990, stock: 120 },
      { sku: 'TAZ-014', nombre: 'Taza soberana', precio_cent: 990, stock: 0 },
      { sku: 'STK-007', nombre: 'Pack pegatinas', precio_cent: 350, stock: 540 },
    ],
  },
  'auth.sesiones': {
    columns: ['id', 'cliente_id', 'expira_en'],
    totalRows: 74,
    rows: [
      { id: 'f0a1…22', cliente_id: '7c1f…a02', expira_en: '2026-07-01 09:00' },
      { id: 'b9c4…81', cliente_id: '9b3a…c41', expira_en: '2026-07-02 12:00' },
    ],
  },
};

export function tableRows(schema: string, name: string): DbRows {
  return TABLE_ROWS[`${schema}.${name}`] ?? { columns: [], rows: [], totalRows: 0 };
}

export const dbApiInfo: DbApiInfo = {
  restUrl: 'https://db.tienda-acme.sytue.eu/rest/v1',
  anonKeyPreview: 'eyJhbGciOiJIUzI1NiI…(anon)',
  serviceKeyPreview: 'eyJhbGciOiJIUzI1NiI…(service)',
  connectionString: 'postgres://app:••••••@db.tienda-acme.sytue.eu:5432/app?sslmode=require',
};

/**
 * Evaluador de SQL de demostracion (cliente). Reconoce SELECT sobre las tablas
 * sembradas y devuelve filas reales del seed; para otras sentencias devuelve un
 * aviso. En produccion la ejecucion corre en la Sytue DevX API contra el
 * Postgres del proyecto (rol acotado, solo lectura por defecto).
 *
 * `t` llega por parametro porque esto NO es un componente: los avisos los lee
 * una persona y estaban escritos en castellano dentro del seed, asi que salian
 * en castellano tambien en las otras cuatro lenguas.
 */
export function runDemoSql(sql: string, t: T): SqlResult {
  const started = Date.now();
  const trimmed = sql.trim().replace(/;$/, '');
  const lower = trimmed.toLowerCase();

  if (!lower.startsWith('select')) {
    return {
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: Date.now() - started + 3,
      notice: t('db.demo.selectOnly'),
    };
  }

  const match = lower.match(/from\s+([a-z_][\w]*\.)?([a-z_][\w]*)/);
  if (!match) {
    return {
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: Date.now() - started + 2,
      error: t('db.demo.noFrom'),
    };
  }
  const schema = (match[1] ?? 'public.').replace('.', '');
  const name = match[2];
  const data = TABLE_ROWS[`${schema}.${name}`];
  if (!data) {
    return {
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: Date.now() - started + 2,
      error: t('db.demo.unknownTable', { table: `${schema}.${name}` }),
    };
  }

  const limitMatch = lower.match(/limit\s+(\d+)/);
  const limit = limitMatch ? Number(limitMatch[1]) : data.rows.length;
  const rows = data.rows.slice(0, limit);
  return {
    columns: data.columns,
    rows,
    rowCount: rows.length,
    elapsedMs: Date.now() - started + 7,
  };
}
