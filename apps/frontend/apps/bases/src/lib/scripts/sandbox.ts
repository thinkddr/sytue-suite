/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El sandbox de los scripts EN BASES: el iframe de origen opaco, su CSP, el Worker y el
 * reenvío son los de `@sytue/scripts` (los mismos que usa Docs; no hay copia). Aquí solo
 * se le da la superficie `sytue.v1` de Bases.
 */
import { crearSandboxIframe as crearSandboxComun, sandboxHtml } from '@sytue/scripts';
import type { Sandbox } from '@sytue/scripts';

import { SUPERFICIE } from './superficie.js';

/** El documento del iframe, completo. */
export const SANDBOX_HTML = sandboxHtml(SUPERFICIE);

export const crearSandboxIframe = (doc: Document = document): Sandbox =>
  crearSandboxComun(SANDBOX_HTML, doc);
