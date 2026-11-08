/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixSecurityError } from './types.js';

/** Qué paso de «Configurar este dispositivo» estaba en marcha cuando falló. */
export type MatrixStep = 'open' | 'connect' | 'setup' | 'recover' | 'reset' | 'verify' | 'other';

/**
 * Código corto y sin secretos de un fallo del cifrado, para que «No se pudo
 * configurar el cifrado» diga EN QUÉ paso y POR QUÉ. Sin él, una CSP que bloquea
 * Matrix, una sincronización que no llega y una clave de recuperación rechazada se
 * veían igual (piloto, 2026-11-08), y no había forma de saber cuál era.
 *
 * Nunca incluye el mensaje del error: el SDK puede meter ahí URLs o datos del
 * servidor. Solo el motivo propio, el `errcode`/estado HTTP de Matrix o el nombre
 * de la clase (`TypeError`, `CompileError` de WebAssembly…).
 */
export function matrixFailureCode(step: MatrixStep, failure: unknown): string {
  return `${step}:${cause(failure)}`;
}

function cause(failure: unknown): string {
  if (failure instanceof MatrixSecurityError) return failure.reason;
  if (failure && typeof failure === 'object') {
    const { errcode, httpStatus, name } = failure as {
      errcode?: unknown;
      httpStatus?: unknown;
      name?: unknown;
    };
    const code = typeof errcode === 'string' && /^[A-Z0-9_.]{1,64}$/i.test(errcode) ? errcode : '';
    const status = typeof httpStatus === 'number' ? String(httpStatus) : '';
    if (code || status) return [code, status].filter(Boolean).join('/');
    if (typeof name === 'string' && /^[A-Za-z]{1,40}$/.test(name)) return name;
  }
  return 'unknown';
}
