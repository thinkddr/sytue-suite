/**
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tipos y lógica pura de las herramientas de gestión de personas de la consola:
 * acceso efectivo, bajas recuperables, historial de lotes, export del directorio,
 * dispositivos y rangos de la auditoría. Sin red ni DOM: lo usan a la vez el BFF
 * (`admin.ts`, solo servidor) y las islas, y se prueba en Bun.
 */
import { contiene } from '@sytue/i18n';
import type { Device } from './types.js';

// ---- Acceso efectivo (GET /admin/users/{id}/effective-access) ----

export interface Concesion {
  permission: string;
  /**
   * De dónde sale el permiso, que es lo que hay que tocar para quitarlo:
   * `rol_integrado`, `rol_personalizado` o `elevacion_temporal`. Texto abierto: una
   * fuente nueva en Sytue ID se enseña tal cual en vez de romper la ficha.
   */
  source: string;
  /** Nombre del rol personalizado, o el rol al que se elevó. */
  detail?: string;
  /** Hora de apagado de una elevación temporal. */
  expires_at?: string;
}

export interface AccesoEfectivo {
  role: string;
  permissions: Concesion[];
  custom_roles: { id: string; name: string; permissions: string[] }[];
  groups: string[];
  pending_elevations: { id: string; role: string; requested_at: string; justification?: string }[];
  as_of: string;
}

// ---- Sesiones de una persona (GET /admin/users/{id}/sessions) ----

export interface SesionAdmin {
  id: string;
  ip: string;
  user_agent: string;
  device: string;
  amr: string[];
  auth_time: string;
  expires_at: string;
}

/** ¿Entró con segundo factor? `amr` es el RFC 8176: otp, hwk, swk, mfa… */
export function conSegundoFactor(amr: readonly string[]): boolean {
  return amr.some((m) => m === 'mfa' || m === 'otp' || m === 'hwk' || m === 'swk' || m === 'sms');
}

// ---- Bajas recuperables (GET /admin/users/closed) ----

export interface CuentaCerrada {
  id: string;
  email: string;
  name: string;
  closed_at: string;
  closed_by?: string;
  reason?: string;
  restorable_until: string;
}

/** Días enteros que quedan para poder restaurar (0 si ya no se puede). */
export function diasParaRestaurar(hasta: string, ahora: number): number {
  const ms = Date.parse(hasta) - ahora;
  return Number.isFinite(ms) && ms > 0 ? Math.ceil(ms / 86_400_000) : 0;
}

// ---- Lotes (POST /admin/users/bulk y GET /admin/jobs) ----

export interface ResultadoLote {
  id: string;
  ok: boolean;
  code?: string;
  sessions_revoked?: number;
}

export interface TrabajoLote {
  id: string;
  /** suspend | reactivate | revoke_sessions | set_role */
  action: string;
  total: number;
  done: number;
  /** ejecutando | terminado | cancelado | interrumpido */
  status: string;
  cancel_requested?: boolean;
  created_at: string;
  finished_at?: string | null;
  results?: ResultadoLote[];
}

/** Los que NO se aplicaron, con su código estable (el texto lo pone la pantalla). */
export function fallosDelLote(results: readonly ResultadoLote[] | undefined): ResultadoLote[] {
  return (results ?? []).filter((r) => !r.ok);
}

/**
 * Lo que cabe seleccionar de una vez: el servicio acota un lote a 200 personas
 * (`bulkMax` en Sytue ID), así que «todas las visibles» se queda en esas y lo dice.
 */
export const MAX_LOTE = 200;

export function seleccionarVisibles(ids: readonly string[]): {
  ids: Set<string>;
  recortado: boolean;
} {
  return { ids: new Set(ids.slice(0, MAX_LOTE)), recortado: ids.length > MAX_LOTE };
}

// ---- Export del directorio ----

/** Fila del directorio tal y como la devuelve Sytue ID (GET /admin/users). */
export interface FilaDirectorio {
  email: string;
  name: string;
  role: string;
  status: string;
  mfa_enabled: boolean;
  email_verified: boolean;
  department?: string;
  title?: string;
  location?: string;
  created_at: string;
}

export const COLUMNAS_DIRECTORIO = [
  'email',
  'name',
  'role',
  'status',
  'mfa',
  'email_verified',
  'department',
  'title',
  'location',
  'created_at',
] as const;

/**
 * El directorio en CSV. `celda` es la que neutraliza fórmulas y entrecomilla (la
 * misma del export de auditoría): los nombres y los departamentos los escribe
 * gente, y quien abre el fichero lo hace en una hoja de cálculo.
 */
export function csvDirectorio(
  filas: readonly FilaDirectorio[],
  celda: (s: string) => string,
): string {
  const lineas = [COLUMNAS_DIRECTORIO.join(',')];
  for (const f of filas) {
    lineas.push(
      [
        f.email,
        f.name,
        f.role,
        f.status,
        f.mfa_enabled ? 'yes' : 'no',
        f.email_verified ? 'yes' : 'no',
        f.department ?? '',
        f.title ?? '',
        f.location ?? '',
        f.created_at,
      ]
        .map(celda)
        .join(','),
    );
  }
  return lineas.join('\r\n') + '\r\n';
}

// ---- Dispositivos de la organización ----

export type FiltroConfianza = '' | 'trusted' | 'untrusted';

/** Busca por persona, dispositivo o IP (sin acentos) y filtra por confianza. */
export function filtrarDispositivos(
  lista: Device[],
  q: string,
  confianza: FiltroConfianza,
): Device[] {
  const texto = q.trim();
  if (!texto && !confianza) return lista;
  return lista.filter((d) => {
    if (confianza === 'trusted' && !d.trusted) return false;
    if (confianza === 'untrusted' && d.trusted) return false;
    if (!texto) return true;
    return [d.userEmail, d.userName, d.name, d.lastIp].some(
      (v) => Boolean(v) && contiene(v ?? '', texto),
    );
  });
}

// ---- Auditoría ----

/**
 * Rango «desde hace N días hasta hoy» en el formato de los campos de fecha
 * (YYYY-MM-DD, hora LOCAL de quien mira: «hoy» es el suyo, no el de UTC). Con
 * N = 0 es solo hoy.
 */
export function rangoRapido(dias: number, ahora: Date): { since: string; until: string } {
  const dia = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const desde = new Date(ahora.getFullYear(), ahora.getMonth(), ahora.getDate() - dias);
  return { since: dia(desde), until: dia(ahora) };
}
