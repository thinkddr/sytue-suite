/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El sandbox de los scripts EN DOCS: el iframe de origen opaco, su CSP, el Worker y el
 * reenvío son los de `@sytue/scripts` (comunes a los productos; allí está el porqué de
 * cada pieza). Aquí solo se le da la superficie `sytue.v1` de Docs (protocolo.ts).
 */
import {
  arranqueIframe,
  crearSandboxIframe as crearSandboxComun,
  preludioWorker,
  sandboxHtml,
} from '@sytue/scripts';
import type { Sandbox } from '@sytue/scripts';

import { SUPERFICIE } from './protocolo';

export { CSP_SANDBOX } from '@sytue/scripts';

/** Preludio del Worker con `sytue.v1` de Docs. */
export const PRELUDIO_WORKER = preludioWorker(SUPERFICIE);
/** Arranque del iframe. */
export const ARRANQUE_IFRAME = arranqueIframe(PRELUDIO_WORKER);
/** El documento del iframe, completo. */
export const SANDBOX_HTML = sandboxHtml(SUPERFICIE);

/** Crea el sandbox de Docs en el documento dado. */
export const crearSandboxIframe = (doc: Document = document): Sandbox =>
  crearSandboxComun(SANDBOX_HTML, doc);
