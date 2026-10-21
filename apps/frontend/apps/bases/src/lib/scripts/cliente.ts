/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Ejecutar una versión» de un script de Bases: consentir el manifiesto → pedir la
 * ejecución al servidor (autoriza y reserva cuota) → correr EXACTAMENTE el código y el
 * manifiesto que el servidor devuelve → informar el resultado. La API y el sandbox se
 * inyectan: sin red ni DOM en las pruebas.
 */
import { HttpError } from '@sytue/api-clients';
import type { BasesClient, BasesScriptVersion } from '@sytue/api-clients/bases';
import type { ResultadoScript, Sandbox } from '@sytue/scripts';

import { ejecutarScriptDeBase } from './anfitrion.js';
import type { ApiDeScripts } from './anfitrion.js';
import { hashManifiesto, validarManifiesto } from './manifiesto.js';
import type { ManifiestoScript } from './manifiesto.js';

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
  api: ApiDeScripts & Pick<BasesClient, 'authorizeScriptRun' | 'closeScriptRun'>;
  baseId: string;
  scriptId: string;
  version: BasesScriptVersion;
  /** Pide el consentimiento si no está dado para ESTE manifiesto; true = adelante. */
  consentir: (manifiesto: ManifiestoScript, hash: string) => Promise<boolean>;
  crearSandbox: () => Sandbox;
}

/**
 * Lo que corre es lo que el servidor autorizó, no lo que la interfaz tenía a mano; si el
 * manifiesto autorizado no es el que se consintió, no se ejecuta nada y queda anotado.
 */
export async function ejecutarVersion(o: OpcionesEjecutarVersion): Promise<Intento> {
  const visto = validarManifiesto(o.version.manifest);
  if (!visto.ok) return { tipo: 'rechazado', motivo: 'servidor', code: 'manifiesto_invalido' };
  const hash = await hashManifiesto(visto.manifiesto);
  if (!(await o.consentir(visto.manifiesto, hash)))
    return { tipo: 'rechazado', motivo: 'sin_consentimiento' };

  let aut: Awaited<ReturnType<BasesClient['authorizeScriptRun']>>;
  try {
    aut = await o.api.authorizeScriptRun(o.baseId, o.scriptId, o.version.version);
  } catch (e) {
    return e instanceof HttpError
      ? { tipo: 'rechazado', motivo: 'servidor', status: e.status, code: e.code ?? '' }
      : { tipo: 'rechazado', motivo: 'servidor', status: 0, code: 'red' };
  }
  const cerrar = (r: ResultadoScript): Promise<void> =>
    o.api
      .closeScriptRun(o.baseId, aut.run_id, {
        outcome: r.outcome,
        error_code: r.errorCode,
        duration_ms: r.durationMs,
        ops: r.ops,
      })
      .catch(() => undefined);
  const autorizado = validarManifiesto(aut.manifest);
  if (!autorizado.ok || (await hashManifiesto(autorizado.manifiesto)) !== hash) {
    await cerrar({
      outcome: 'denied',
      errorCode: 'manifiesto_cambio',
      message: '',
      durationMs: 0,
      ops: 0,
      output: [],
    });
    return { tipo: 'rechazado', motivo: 'manifiesto_cambio' };
  }
  const resultado = await ejecutarScriptDeBase({
    codigo: aut.code,
    manifiesto: autorizado.manifiesto,
    baseId: o.baseId,
    api: o.api,
    crearSandbox: o.crearSandbox,
  });
  await cerrar(resultado);
  return { tipo: 'ejecutado', resultado };
}
