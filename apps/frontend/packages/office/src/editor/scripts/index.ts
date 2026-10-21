/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Punto de entrada de los scripts de usuario (ADR 0069): lo que el editor y las
 * pruebas en navegador importan.
 */
export { ejecutarScript } from './anfitrion';
export type {
  Anfitrion,
  OpcionesEjecucion,
  Resultado,
  ResultadoScript,
  Sandbox,
} from './anfitrion';
export { traducirLlamada } from './api';
export { canonico, hashManifiesto, validarManifiesto } from './manifiesto';
export type { ManifiestoScript } from './manifiesto';
export { crearSandboxIframe, CSP_SANDBOX, SANDBOX_HTML } from './sandbox';
