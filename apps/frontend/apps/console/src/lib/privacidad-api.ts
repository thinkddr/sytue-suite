/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Derechos RGPD del lado del responsable: los proxys de SOLO servidor a Sytue ID
 * (/api/v1/admin/privacy/requests) con el token httpOnly de la sesión. Los tipos y
 * las cuentas sin red están en `privacidad.ts`.
 */
import { HttpClient, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { adminBaseUrl } from './auth.js';
import type {
  AltaSolicitud,
  DetalleRGPD,
  EstadoSolicitud,
  ListadoRGPD,
  SolicitudRGPD,
  TipoSolicitud,
} from './privacidad.js';

function idHttp(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${adminBaseUrl().replace(/\/$/, '')}/api/v1/admin/privacy`,
    auth: bearer(session.tokens.accessToken),
  });
}

/** Filtros del listado que entiende Sytue ID. */
export interface FiltroRGPD {
  status?: 'open' | EstadoSolicitud;
  kind?: TipoSolicitud;
  overdue?: boolean;
}

export async function listarSolicitudes(
  session: SytueSession,
  f: FiltroRGPD = {},
): Promise<ListadoRGPD> {
  return idHttp(session).json<ListadoRGPD>({
    path: '/requests',
    query: { status: f.status, kind: f.kind, overdue: f.overdue ? '1' : undefined },
  });
}

export async function crearSolicitud(
  session: SytueSession,
  alta: AltaSolicitud,
): Promise<{ request: SolicitudRGPD }> {
  return idHttp(session).postJson('/requests', alta);
}

export async function verSolicitud(session: SytueSession, id: string): Promise<DetalleRGPD> {
  return idHttp(session).json<DetalleRGPD>({ path: `/requests/${encodeURIComponent(id)}` });
}

/** Cambios que admite una solicitud abierta. */
export interface CambiosSolicitud {
  status?: EstadoSolicitud;
  resolution?: string;
  notes?: string;
  identity_verified?: boolean;
  identity_method?: string;
}

export async function cambiarSolicitud(
  session: SytueSession,
  id: string,
  cambios: CambiosSolicitud,
): Promise<{ request: SolicitudRGPD }> {
  return idHttp(session).json<{ request: SolicitudRGPD }>({
    method: 'PATCH',
    path: `/requests/${encodeURIComponent(id)}`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(cambios),
  });
}

export async function prorrogarSolicitud(
  session: SytueSession,
  id: string,
  reason: string,
): Promise<{ request: SolicitudRGPD }> {
  return idHttp(session).postJson(`/requests/${encodeURIComponent(id)}/extend`, { reason });
}

/** La supresión física: pide reautenticación reciente en Sytue ID. */
export async function suprimirCuenta(
  session: SytueSession,
  id: string,
  confirm: string,
): Promise<{ request: SolicitudRGPD; note?: string }> {
  return idHttp(session).postJson(`/requests/${encodeURIComponent(id)}/erase`, { confirm });
}
