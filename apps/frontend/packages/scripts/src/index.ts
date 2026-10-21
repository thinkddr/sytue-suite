/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Runtime aislado de los scripts de usuario (ADR 0069), común a los productos. Docs y
 * Bases lo importan y aportan solo su superficie `sytue.v1` y su traductor de llamadas.
 */
export {
  cargarConsentimientos,
  consentir,
  guardarConsentimientos,
  haConsentido,
  leerConsentimientos,
  sha256Hex,
} from './consentimiento';
export { ejecutarAislado } from './ejecutor';
export type { Atencion, OpcionesAisladas, Resultado, ResultadoScript, Sandbox } from './ejecutor';
export { LIMITES, leerMensaje, sanearDato } from './protocolo';
export type { Escalar, MensajeDeSandbox, Respuesta } from './protocolo';
export {
  arranqueIframe,
  crearSandboxIframe,
  CSP_SANDBOX,
  preludioWorker,
  sandboxHtml,
} from './sandbox';
export type { OperacionDeSuperficie } from './sandbox';
