/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cliente del BFF de scripts (/api/scripts) y orquestación de «ejecutar una versión»:
 * consentir el manifiesto → pedir la ejecución al servidor (autoriza y descuenta cuota) →
 * correr EXACTAMENTE el código y el manifiesto que el servidor devuelve → informar el
 * resultado. `fetch` y el sandbox se inyectan: sin red ni DOM en los tests.
 */
import { withBase } from '@sytue/ui';

import { ejecutarScript } from './anfitrion';
import type { Anfitrion, ResultadoScript, Sandbox } from './anfitrion';
import { hashManifiesto, validarManifiesto } from './manifiesto';
import type { ManifiestoScript } from './manifiesto';
import type { EntornoMacro } from '../macros';

export type EstadoVersion = 'draft' | 'pending' | 'approved' | 'rejected';

export interface ScriptResumen {
  id: string;
  name: string;
  kind: 'document' | 'spreadsheet' | 'presentation';
  scope: 'private' | 'org';
  document_id?: string;
  author_sub: string;
  latest_version: number;
  approved_version: number;
}

export interface VersionScript {
  script_id: string;
  version: number;
  code?: string;
  code_hash: string;
  manifest: unknown;
  status: EstadoVersion;
  created_by: string;
  reviewed_by?: string;
}

export interface EjecucionAuditada {
  id: number;
  script_id: string;
  version: number;
  code_hash: string;
  document_id: string;
  sub: string;
  outcome: string;
  error_code?: string;
  duration_ms?: number;
  ops: number;
  started_at: string;
}

export interface Cuota {
  used: { user_runs: number; org_runs: number; user_ms: number };
  limits: { user_runs: number; org_runs: number; user_ms: number };
}

export type Fetch = (url: string, init?: RequestInit) => Promise<Response>;

export class ErrorApi extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}

async function pedir<T>(f: Fetch, url: string, method = 'GET', cuerpo?: unknown): Promise<T> {
  const r = await f(url, {
    method,
    credentials: 'include',
    ...(cuerpo === undefined
      ? {}
      : { headers: { 'content-type': 'application/json' }, body: JSON.stringify(cuerpo) }),
  });
  if (!r.ok) {
    let code = String(r.status);
    try {
      code = ((await r.json()) as { error?: { code?: string } }).error?.code ?? code;
    } catch {
      /* cuerpo vacío */
    }
    throw new ErrorApi(r.status, code);
  }
  return r.status === 204 ? (undefined as T) : ((await r.json()) as T);
}

const q = (o: Record<string, string | number | undefined>): string => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v !== undefined) p.set(k, String(v));
  return withBase(import.meta.env.BASE_URL, `/api/scripts?${p.toString()}`);
};

export const api = (f: Fetch) => ({
  listar: () => pedir<{ scripts: ScriptResumen[]; can_review: boolean }>(f, '/api/scripts'),
  ver: (sid: string) =>
    pedir<{ script: ScriptResumen; versions: VersionScript[]; can_review: boolean }>(f, q({ sid })),
  crear: (b: {
    name: string;
    kind: string;
    scope: string;
    document_id?: string;
    code: string;
    manifest: ManifiestoScript;
  }) => pedir<{ script: ScriptResumen; version: VersionScript }>(f, '/api/scripts', 'POST', b),
  nuevaVersion: (sid: string, b: { code: string; manifest: ManifiestoScript }) =>
    pedir<{ version: VersionScript }>(f, q({ sid, op: 'version' }), 'POST', b),
  ambito: (sid: string, scope: 'private' | 'org') =>
    pedir<void>(f, q({ sid, op: 'scope' }), 'PUT', { scope }),
  proponer: (sid: string, v: number) => pedir<void>(f, q({ sid, v, op: 'propose' }), 'POST'),
  revisar: (sid: string, v: number, decision: 'approve' | 'reject', code_hash: string) =>
    pedir<void>(f, q({ sid, v, op: 'review' }), 'POST', { decision, code_hash }),
  borrar: (sid: string) => pedir<void>(f, q({ sid }), 'DELETE'),
  cuota: () => pedir<Cuota>(f, q({ op: 'quota' })),
  registroOrg: (before_id?: number) =>
    pedir<{ runs: EjecucionAuditada[]; next_before_id: number }>(
      f,
      q({ op: 'runs', before_id, limit: 50 }),
    ),
  registroDoc: (doc: string, before_id?: number) =>
    pedir<{ runs: EjecucionAuditada[]; next_before_id: number }>(
      f,
      q({ op: 'runs', doc, before_id, limit: 50 }),
    ),
  ejecutar: (sid: string, v: number, document_id: string) =>
    pedir<{ run_id: number; code: string; code_hash: string; manifest: unknown }>(
      f,
      q({ sid, v, op: 'run' }),
      'POST',
      { document_id },
    ),
  cerrar: (run: number, r: ResultadoScript) =>
    pedir<void>(f, q({ run }), 'PUT', {
      outcome: r.outcome,
      error_code: r.errorCode,
      duration_ms: r.durationMs,
      ops: r.ops,
    }),
});

/** Resultado de intentar ejecutar una versión guardada. */
export type Intento =
  | {
      tipo: 'rechazado';
      motivo: 'sin_consentimiento' | 'manifiesto_cambio' | 'servidor';
      status?: number;
      code?: string;
    }
  | { tipo: 'ejecutado'; resultado: ResultadoScript };

export interface OpcionesEjecutarVersion {
  fetch: Fetch;
  sid: string;
  version: VersionScript;
  documentId: string;
  /** Pide el consentimiento si no está dado para ESTE manifiesto; true = adelante. */
  consentir: (manifiesto: ManifiestoScript, hash: string) => Promise<boolean>;
  anfitrion: Anfitrion;
  entorno: EntornoMacro;
  crearSandbox: () => Sandbox;
  interactivo?: boolean;
}

/**
 * Ejecuta una versión: consentimiento → autorización del servidor → ejecución aislada →
 * informe. Lo que corre es lo que el servidor autorizó, no lo que la interfaz tenía a mano;
 * si el manifiesto autorizado no es el que se consintió, no se ejecuta nada.
 */
export async function ejecutarVersion(o: OpcionesEjecutarVersion): Promise<Intento> {
  const visto = validarManifiesto(o.version.manifest);
  if (!visto.ok) return { tipo: 'rechazado', motivo: 'servidor', code: 'manifiesto_invalido' };
  const hash = await hashManifiesto(visto.manifiesto);
  if (!(await o.consentir(visto.manifiesto, hash)))
    return { tipo: 'rechazado', motivo: 'sin_consentimiento' };

  const cliente = api(o.fetch);
  let auth: Awaited<ReturnType<typeof cliente.ejecutar>>;
  try {
    auth = await cliente.ejecutar(o.sid, o.version.version, o.documentId);
  } catch (e) {
    const err = e instanceof ErrorApi ? e : new ErrorApi(0, 'red');
    return { tipo: 'rechazado', motivo: 'servidor', status: err.status, code: err.code };
  }
  const autorizado = validarManifiesto(auth.manifest);
  if (!autorizado.ok || (await hashManifiesto(autorizado.manifiesto)) !== hash) {
    const r: ResultadoScript = {
      outcome: 'denied',
      errorCode: 'manifiesto_cambio',
      message: '',
      durationMs: 0,
      ops: 0,
      output: [],
    };
    await cliente.cerrar(auth.run_id, r).catch(() => undefined);
    return { tipo: 'rechazado', motivo: 'manifiesto_cambio' };
  }
  const resultado = await ejecutarScript({
    codigo: auth.code,
    manifiesto: autorizado.manifiesto,
    contexto: { entorno: o.entorno },
    anfitrion: o.anfitrion,
    crearSandbox: o.crearSandbox,
    ...(o.interactivo === undefined ? {} : { interactivo: o.interactivo }),
  });
  await cliente.cerrar(auth.run_id, resultado).catch(() => undefined);
  return { tipo: 'ejecutado', resultado };
}
