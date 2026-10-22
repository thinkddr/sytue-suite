/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ASISTENTE DE IMPORTACIÓN — traerse uno sus propios datos de Google o Microsoft 365.
 *
 * El motor de migración vive en el Sytue Integration Gateway y lo tenía SOLO la consola
 * de administración: quien llegaba a Sytue y abría Mail con el buzón vacío no encontraba
 * por dónde traerse su correo. Esto es el lado servidor de la puerta que ahora vive
 * dentro de cada producto — Mail, Drive, Calendar y Contacts—, que es donde se busca.
 *
 * Vive en un paquete compartido porque las cuatro apps necesitan exactamente lo mismo y
 * cada una tiene su propio BFF: sin esto habría cuatro copias del mismo proxy
 * divergiendo. Lo que cambia por app es UN dato —qué contenido trae— y viaja como
 * parámetro.
 *
 * El token de sesión NUNCA sale del servidor: estas funciones se llaman desde el
 * endpoint BFF de cada app, no desde el navegador.
 */

/** Contenido que se puede traer. Coincide con `MigrationDomain` del gateway. */
export type ImportDomain = 'mail' | 'drive' | 'calendar' | 'contacts';

/** SaaS del que se importa. */
export type ImportSource = 'google' | 'm365';

/** Un origen disponible: una conexión que la organización ya autorizó. */
export interface ImportOrigin {
  instanceId: string;
  /** Nombre que le puso la organización a la conexión. */
  name: string;
  source: ImportSource;
}

/** Estado de una importación en curso o terminada. */
export interface ImportJob {
  id: string;
  domain: ImportDomain;
  source: ImportSource;
  status: 'queued' | 'running' | 'done' | 'error' | 'canceled';
  migrated: number;
  skipped: number;
  errors: number;
  lastError: string;
  createdAt: string;
  finishedAt?: string;
}

interface WireInstance {
  id: string;
  name: string;
  template_key: string;
  status: string;
}

interface WireJob {
  id: string;
  domain: string;
  source: string;
  status: string;
  counts?: { migrated?: number; skipped?: number; errors?: number };
  last_error?: string;
  created_at: string;
  finished_at?: string;
}

/**
 * Qué plantilla del catálogo es cada origen migrable.
 *
 * Se mira la CLAVE de la plantilla y no el nombre: el nombre lo pone quien conecta («el
 * Google de ventas») y no dice de qué producto se trata. Cualquier otra conexión de la
 * organización —Jira, Salesforce…— no tiene contenido personal que traer y no aparece.
 */
const ORIGENES: Record<string, ImportSource> = {
  google: 'google',
  'google-workspace': 'google',
  gws: 'google',
  m365: 'm365',
  microsoft365: 'm365',
  'microsoft-365': 'm365',
};

/** Reconoce la clave de plantilla, tolerando sufijos («google-workspace-eu»). */
function sourceOf(templateKey: string): ImportSource | null {
  const k = templateKey.trim().toLowerCase();
  if (ORIGENES[k]) return ORIGENES[k];
  if (k.startsWith('google')) return 'google';
  if (k.startsWith('m365') || k.startsWith('microsoft')) return 'm365';
  return null;
}

// `Record<string, string>` y no `HeadersInit`: este objeto se esparce con `...` en la
// línea de abajo, y `HeadersInit` admite además `Headers` y `string[][]` —formas cuyo
// spread produce índices numéricos y dejaría la petición SIN cabecera `authorization`.
function auth(accessToken: string): Record<string, string> {
  return { authorization: `Bearer ${accessToken}`, accept: 'application/json' };
}

/**
 * Orígenes de los que esta persona puede traerse datos: las conexiones ACTIVAS de su
 * organización con Google o Microsoft.
 *
 * Degrada a lista vacía ante cualquier fallo: sin orígenes la interfaz no ofrece nada, y
 * eso es correcto en una organización que no ha conectado ninguno. Enseñar un error donde
 * la respuesta honesta es «no hay» solo asustaría.
 */
export async function listImportOrigins(
  baseUrl: string,
  accessToken: string,
): Promise<ImportOrigin[]> {
  try {
    const res = await fetch(`${baseUrl.replace(/\/$/, '')}/api/v1/instances`, {
      headers: auth(accessToken),
    });
    if (!res.ok) return [];
    const { instances = [] } = (await res.json()) as { instances?: WireInstance[] };
    return instances.flatMap((i) => {
      if (i.status !== 'active') return [];
      const source = sourceOf(i.template_key);
      return source ? [{ instanceId: i.id, name: i.name, source }] : [];
    });
  } catch {
    return [];
  }
}

function toJob(w: WireJob): ImportJob {
  const job: ImportJob = {
    id: w.id,
    domain: w.domain as ImportDomain,
    source: w.source as ImportSource,
    status: w.status as ImportJob['status'],
    migrated: w.counts?.migrated ?? 0,
    skipped: w.counts?.skipped ?? 0,
    errors: w.counts?.errors ?? 0,
    lastError: w.last_error ?? '',
    createdAt: w.created_at,
  };
  if (w.finished_at) job.finishedAt = w.finished_at;
  return job;
}

/**
 * Lanza la importación. El gateway ignora a quién se le diga que migrar: destino y
 * origen son siempre quien llama, así que aquí no viaja ninguna identidad.
 */
export async function startImport(
  baseUrl: string,
  accessToken: string,
  input: { instanceId: string; source: ImportSource; domains: ImportDomain[] },
): Promise<{ ok: boolean; status: number; jobs: ImportJob[] }> {
  const res = await fetch(`${baseUrl.replace(/\/$/, '')}/api/v1/migrations/self`, {
    method: 'POST',
    headers: { ...auth(accessToken), 'content-type': 'application/json' },
    body: JSON.stringify({
      instance_id: input.instanceId,
      source: input.source,
      domains: input.domains,
    }),
  });
  if (!res.ok) return { ok: false, status: res.status, jobs: [] };
  const { migrations = [] } = (await res.json()) as { migrations?: WireJob[] };
  return { ok: true, status: res.status, jobs: migrations.map(toJob) };
}

/**
 * Importaciones de esta persona, filtradas al contenido que le interesa a ESTA app.
 *
 * Se filtra aquí y no en el gateway porque el mismo trabajo le importa a dos sitios: una
 * migración de contactos la enseñan Contacts y Mail, cada uno con sus palabras.
 */
export async function listMyImports(
  baseUrl: string,
  accessToken: string,
  domains: ImportDomain[],
): Promise<ImportJob[]> {
  try {
    const res = await fetch(`${baseUrl.replace(/\/$/, '')}/api/v1/migrations/self`, {
      headers: auth(accessToken),
    });
    if (!res.ok) return [];
    const { migrations = [] } = (await res.json()) as { migrations?: WireJob[] };
    const quiero = new Set<string>(domains);
    return migrations.filter((m) => quiero.has(m.domain)).map(toJob);
  } catch {
    return [];
  }
}
