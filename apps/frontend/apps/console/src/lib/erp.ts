/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Permisos de Sytue ERP por módulo, desde la consola de la organización.
 *
 * Habla con `services/syerp` y no con Sytue ID: los niveles son del ERP, que es
 * quien los aplica en cada petición. La llamada sale del SERVIDOR con el access
 * token de la sesión; el servicio exige owner/admin y el aislamiento por
 * organización lo pone RLS con el tenant del token.
 */
import { HttpClient, HttpError, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { erpBaseUrl } from './auth.js';

/** De menos a más: no ve · ve · trabaja · administra el módulo. */
export type NivelErp = 'ninguno' | 'lectura' | 'usuario' | 'administrador';

export interface ModuloErp {
  clave: string;
  rotulo: string;
  por_defecto: NivelErp;
}

export interface PersonaErp {
  sub: string;
  email: string;
  nombre: string;
  roles: string[];
  /** Solo lo distinto del defecto de cada módulo. */
  modulos: Record<string, NivelErp>;
  visto_en?: string;
}

export interface PermisosErp {
  niveles: NivelErp[];
  modulos: ModuloErp[];
  personas: PersonaErp[];
}

export interface ErpResult<T> {
  ok: boolean;
  status: number;
  data: T;
}

function http(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${erpBaseUrl().replace(/\/$/, '')}/api/v1/admin`,
    auth: bearer(session.tokens.accessToken),
  });
}

async function attempt<T>(fallback: T, fn: () => Promise<T>): Promise<ErpResult<T>> {
  try {
    return { ok: true, status: 200, data: await fn() };
  } catch (e) {
    return { ok: false, status: e instanceof HttpError ? e.status : 502, data: fallback };
  }
}

/** Módulos (con su rótulo en el idioma de quien mira) y personas con nivel. */
export async function loadErpPermisos(
  session: SytueSession,
  locale: string,
): Promise<ErpResult<PermisosErp>> {
  return attempt<PermisosErp>({ niveles: [], modulos: [], personas: [] }, () =>
    http(session).json<PermisosErp>({ path: '/permisos', headers: { 'accept-language': locale } }),
  );
}

/** Guarda los niveles de una persona (el servicio descarta los que son el defecto). */
export async function saveErpPermisos(
  session: SytueSession,
  persona: { sub: string; email: string; nombre: string; modulos: Record<string, string> },
): Promise<ErpResult<null>> {
  return attempt<null>(null, async () => {
    await http(session).json<void>({
      method: 'PUT',
      path: `/permisos/${encodeURIComponent(persona.sub)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        email: persona.email,
        nombre: persona.nombre,
        modulos: persona.modulos,
      }),
    });
    return null;
  });
}
