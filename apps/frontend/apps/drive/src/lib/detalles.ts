/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Detalles» de un fichero o una carpeta: lo que el panel enseña y cómo se reúne.
 *
 * El servicio de Drive ya sabe todo esto, pero repartido: el metadato (`GET /files/{id}`
 * o `/folders/{id}`) dice quién es el propietario, quién lo cambió por última vez, la
 * versión y la clasificación; las comparticiones (`/{id}/shares`) dicen quién más entra.
 * Y las personas vienen como `sub` opacos, que hay que resolver contra el directorio.
 * Reunirlo aquí y no en el navegador ahorra tres viajes y, sobre todo, que el cliente
 * tenga que entender el formato del servicio.
 *
 * Vive en `lib` y no en la ruta porque la ruta arrastra Astro y la sesión: aquí las
 * dependencias entran por parámetro y la prueba las sustituye.
 */

/** Una persona tal y como la pinta el panel; vacía = hay alguien, pero sin cara. */
export interface PersonaDetalle {
  name?: string;
  email?: string;
}

export interface Detalles {
  kind: 'file' | 'folder';
  name: string;
  /** Ruta absoluta, si el servicio la dio (ficheros). */
  path?: string;
  contentType?: string;
  size?: number;
  /** `null` = el servicio no dijo de quién es (no es lo mismo que «no se sabe quién»). */
  owner: PersonaDetalle | null;
  createdAt?: string;
  modifiedAt?: string;
  modifiedBy: PersonaDetalle | null;
  version?: number;
  label?: { name: string; blockExternalShare: boolean };
  /**
   * Quién más tiene acceso. `null` cuando el listado de comparticiones no se pudo leer
   * (p. ej. un lector sin permiso para verlo): el resto del panel sigue valiendo, y
   * decir «no compartido» en ese caso sería mentir.
   */
  access: { links: number; people: number; groups: number } | null;
}

interface PersonaDirectorio {
  email?: string;
  name?: string;
}

export interface DependenciasDetalles {
  /** Petición autenticada al servicio (`driveFetch`); `null` = sin sesión. */
  pedir: (ruta: string) => Promise<Response | null>;
  /** Resolución de `sub` a personas (`resolverPersonas`). */
  resolver: (subs: string[]) => Promise<ReadonlyMap<string, PersonaDirectorio>>;
  /** Reloj, para descartar los enlaces caducados. */
  ahora?: () => number;
}

interface RawRecurso {
  name?: string;
  path?: string;
  size_bytes?: number;
  content_type?: string;
  owner_sub?: string;
  modified_by?: string;
  version?: number;
  created_at?: string;
  updated_at?: string;
  label?: { name?: string; block_external_share?: boolean } | null;
}

interface RawShare {
  kind?: string;
  subject?: string;
  expires_at?: string;
}

export interface RespuestaDetalles {
  status: number;
  body: { details: Detalles } | { error: string };
}

/** Cuenta quién más entra: enlaces vivos, personas y grupos distintos. */
export function contarAcceso(
  shares: readonly RawShare[],
  ahora: number,
): { links: number; people: number; groups: number } {
  let links = 0;
  const personas = new Set<string>();
  const grupos = new Set<string>();
  for (const s of shares) {
    if (s.kind === 'link') {
      // Un enlace caducado ya no abre nada; contarlo haría creer que el fichero sigue
      // expuesto cuando no lo está.
      const caduca = s.expires_at ? Date.parse(s.expires_at) : Number.NaN;
      if (Number.isNaN(caduca) || caduca > ahora) links += 1;
    } else if (s.kind === 'user' && s.subject) personas.add(s.subject);
    else if (s.kind === 'group' && s.subject) grupos.add(s.subject);
  }
  return { links, people: personas.size, groups: grupos.size };
}

function persona(
  sub: string | undefined,
  directorio: ReadonlyMap<string, PersonaDirectorio>,
): PersonaDetalle | null {
  if (!sub) return null;
  const p = directorio.get(sub);
  // Sin directorio (o sin esa persona en él) se devuelve vacía: la UI dice «Alguien»
  // en vez de enseñar el `sub`, que no le dice nada a nadie.
  if (!p) return {};
  return { ...(p.name ? { name: p.name } : {}), ...(p.email ? { email: p.email } : {}) };
}

/** Reúne los detalles de `id`. No lanza: cualquier fallo es una respuesta con su código. */
export async function reunirDetalles(
  id: string,
  kind: 'file' | 'folder',
  deps: DependenciasDetalles,
): Promise<RespuestaDetalles> {
  const base = `/api/v1/${kind === 'file' ? 'files' : 'folders'}/${encodeURIComponent(id)}`;
  // Las dos peticiones a la vez: son independientes y el panel espera a ambas.
  const [resRecurso, resShares] = await Promise.all([
    deps.pedir(base),
    deps.pedir(`${base}/shares`).catch(() => null),
  ]);
  if (!resRecurso) return { status: 401, body: { error: 'unauthenticated' } };
  if (!resRecurso.ok) {
    return {
      status: resRecurso.status >= 500 ? 502 : resRecurso.status,
      body: { error: 'details_failed' },
    };
  }
  let r: RawRecurso;
  try {
    r = (await resRecurso.json()) as RawRecurso;
  } catch {
    return { status: 502, body: { error: 'details_failed' } };
  }

  let access: Detalles['access'] = null;
  if (resShares?.ok) {
    try {
      const body = (await resShares.json()) as { shares?: RawShare[] };
      access = contarAcceso(body.shares ?? [], (deps.ahora ?? Date.now)());
    } catch {
      access = null;
    }
  }

  const subs = [r.owner_sub, r.modified_by].filter((s): s is string => !!s);
  let directorio: ReadonlyMap<string, PersonaDirectorio> = new Map();
  if (subs.length > 0) {
    try {
      directorio = await deps.resolver([...new Set(subs)]);
    } catch {
      // Sin directorio el panel sigue: las personas salen como «Alguien».
    }
  }

  const details: Detalles = {
    kind,
    name: r.name ?? '',
    ...(r.path ? { path: r.path } : {}),
    ...(kind === 'file' && r.content_type ? { contentType: r.content_type } : {}),
    ...(kind === 'file' && typeof r.size_bytes === 'number' ? { size: r.size_bytes } : {}),
    owner: persona(r.owner_sub, directorio),
    ...(r.created_at ? { createdAt: r.created_at } : {}),
    ...(r.updated_at ? { modifiedAt: r.updated_at } : {}),
    modifiedBy: persona(r.modified_by, directorio),
    ...(kind === 'file' && typeof r.version === 'number' && r.version > 0
      ? { version: r.version }
      : {}),
    ...(r.label?.name
      ? { label: { name: r.label.name, blockExternalShare: !!r.label.block_external_share } }
      : {}),
    access,
  };
  return { status: 200, body: { details } };
}
