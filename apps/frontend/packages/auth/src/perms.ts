/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Permisos granulares (admin delegada) leídos del ACCESS token de Sytue ID.
 *
 * POR QUÉ DEL ACCESS TOKEN Y NO DEL id_token: Sytue ID emite `perms` SOLO en el
 * access token a propósito — es la superficie de enforcement que leen los servicios
 * del Bearer (`services/id/internal/oidc/token.go`). Además es la fuente FRESCA: los
 * permisos efectivos son la unión de los roles personalizados asignados y las
 * activaciones PIM VIVAS, así que se recalculan en cada emisión/refresh; el id_token
 * de hace una hora podría conceder una elevación JIT ya caducada.
 *
 * SEGURIDAD — por qué se decodifica SIN verificar la firma: el token no viene del
 * cliente, sale de NUESTRA cookie de sesión cifrada del lado servidor, que ya se
 * validó al iniciar sesión. Y, sobre todo, esto NO es el control de acceso real: el
 * enforcement vive en los servicios Go, que sí verifican la firma del Bearer. Lo de
 * aquí decide qué se NAVEGA y qué se PINTA (evitar ofrecer una página que el backend
 * va a rechazar con 403). Deny-by-default: si el token no se puede leer, no hay
 * permisos.
 */
import type { SytuePermission } from './types.js';

/** Catálogo fijo, en el orden del backend (útil para UI de selección de permisos). */
export const ALL_PERMISSIONS: readonly SytuePermission[] = [
  'users.manage',
  'users.read',
  'sso.manage',
  'apps.manage',
  'access_policies.manage',
  'groups.manage',
  'audit.view',
  'billing.manage',
  'roles.manage',
] as const;

const KNOWN: ReadonlySet<string> = new Set(ALL_PERMISSIONS);

/** Decodifica el payload de un JWT (sin verificar; ver nota de seguridad arriba). */
function jwtPayload(token: string): Record<string, unknown> | null {
  const part = token.split('.')[1];
  if (part === undefined || part === '') return null;
  try {
    // base64url → base64 y relleno a múltiplo de 4 (atob no tolera base64url).
    const b64 = part.replace(/-/g, '+').replace(/_/g, '/');
    const padded = b64.padEnd(Math.ceil(b64.length / 4) * 4, '=');
    return JSON.parse(atob(padded)) as Record<string, unknown>;
  } catch {
    return null;
  }
}

/**
 * Permisos granulares del access token. Vacío si no los lleva — es lo normal:
 * Sytue ID omite el claim cuando el sujeto no tiene permisos delegados (owner y
 * admin no lo necesitan, son superusuarios).
 */
export function permsFromAccessToken(accessToken: string): SytuePermission[] {
  const payload = jwtPayload(accessToken);
  const raw = payload?.perms;
  if (!Array.isArray(raw)) return [];
  return raw.filter((p): p is SytuePermission => typeof p === 'string' && KNOWN.has(p));
}
