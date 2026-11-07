/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué dice la lista cuando no hay filas. La papelera tiene el SUYO: vacía decía
 * «Aquí todavía no hay nada · Crea el primero con Nuevo», y en la papelera no se
 * crea nada. Espejo en `apps/fyne/apps/erp/lista.go` (`vacioDeLista`).
 */
import type { ErpMessageKey } from '../i18n/index.js';

export interface VacioDeLista {
  titulo: ErpMessageKey;
  /** La segunda línea; sin ella, ninguna. */
  pista?: ErpMessageKey;
  icono: 'trash' | 'building';
  /** `limpiar` es «Restablecer vista» (también saca de la papelera); `nuevo`, crear. */
  accion: 'limpiar' | 'nuevo';
}

export function vacioDeLista(f: {
  papelera: boolean;
  estado: string;
  busqueda: string;
}): VacioDeLista {
  const filtrada = f.estado !== '' || f.busqueda !== '';
  if (f.papelera) {
    return filtrada
      ? { titulo: 'list.trashEmpty', icono: 'trash', accion: 'limpiar' }
      : {
          titulo: 'list.trashEmpty',
          pista: 'list.trashEmptyHint',
          icono: 'trash',
          accion: 'limpiar',
        };
  }
  if (filtrada) return { titulo: 'list.empty', icono: 'building', accion: 'limpiar' };
  return { titulo: 'list.empty', pista: 'list.emptyHint', icono: 'building', accion: 'nuevo' };
}
