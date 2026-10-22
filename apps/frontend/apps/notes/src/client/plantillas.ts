// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { escapar } from './captura.js';
import type { T } from '../i18n/index.js';

export const PLANTILLAS = ['journal', 'meeting', 'plan'] as const;
export type Plantilla = (typeof PLANTILLAS)[number];

/** Una nota editable, con HTML del editor de Notes, también legible en Fyne. */
export function contenidoPlantilla(p: Plantilla, t: T): string {
  const prioridades = `<h2>${escapar(t('template.priorities'))}</h2><ul><li></li></ul>`;
  const notas = `<h2>${escapar(t('note.newText'))}</h2><p></p>`;
  const siguientes = `<h2>${escapar(t('template.next'))}</h2><div class="task-row"><input type="checkbox"><span></span></div>`;
  return p === 'meeting' ? notas + siguientes : prioridades + notas + siguientes;
}
