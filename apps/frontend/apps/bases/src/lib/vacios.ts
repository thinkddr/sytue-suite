/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los estados vacíos de una base abierta (design.md §2.8: el vacío es la puerta de
 * entrada). Cada uno dice qué falta y ofrece el paso siguiente, pero solo el que la
 * persona puede dar: quien no es propietario no ve «Añadir tabla» para que el servicio
 * se lo niegue después. Espejo de `apps/fyne/apps/bases/vacios.go`.
 */
import type { BasesView } from '@sytue/api-clients/bases';
import type { IconName } from '@sytue/ui';
import type { BasesMessageKey } from '../i18n/index.js';
import type { Lectura } from './lectura.js';

/** Qué falta: tablas, campos, registros, o registros que pasen la búsqueda o la vista. */
export type EstadoVacio = 'sinTablas' | 'sinCampos' | 'sinRegistros' | 'sinResultados';

/** El paso que ofrece cada estado. */
export type AccionVacio =
  'crearTabla' | 'importarCsv' | 'anadirCampo' | 'anadirRegistro' | 'verTodos';

export interface Permisos {
  /** Cambiar el esquema (tablas y campos): solo el propietario. */
  esquema: boolean;
  /** Añadir y cambiar registros: propietario y editor. */
  escribir: boolean;
}

export interface Vacio {
  icono: IconName;
  titulo: BasesMessageKey;
  descripcion: BasesMessageKey;
  acciones: AccionVacio[];
}

/**
 * El estado vacío completo: icono, claves de título y descripción, y las acciones en
 * orden (la primera es la principal). La descripción cambia con el rol para que nadie
 * lea «crea una tabla» sin poder crearla.
 */
export function vacioDe(estado: EstadoVacio, p: Permisos, busqueda = false): Vacio {
  switch (estado) {
    case 'sinTablas':
      return p.esquema
        ? {
            icono: 'table',
            titulo: 'base.noTables',
            descripcion: 'base.noTables.owner',
            acciones: ['crearTabla', 'importarCsv'],
          }
        : {
            icono: 'table',
            titulo: 'base.noTables',
            descripcion: 'base.noTables.reader',
            acciones: [],
          };
    case 'sinCampos':
      return {
        icono: 'formField',
        titulo: 'grid.noFields',
        descripcion: p.esquema ? 'grid.noFields.owner' : 'grid.noFields.reader',
        acciones: p.esquema ? ['anadirCampo'] : [],
      };
    case 'sinRegistros':
      return {
        icono: 'list',
        titulo: 'grid.empty',
        descripcion: p.escribir ? 'grid.empty.editor' : 'grid.empty.reader',
        acciones: p.escribir ? ['anadirRegistro'] : [],
      };
    case 'sinResultados':
      // Volver a verlo todo no cambia nada guardado: lo puede hacer cualquiera.
      return busqueda
        ? {
            icono: 'search',
            titulo: 'grid.noMatches',
            descripcion: 'grid.noMatches.body',
            acciones: ['verTodos'],
          }
        : {
            icono: 'search',
            titulo: 'grid.viewEmpty',
            descripcion: 'grid.viewEmpty.body',
            acciones: ['verTodos'],
          };
  }
}

/**
 * Si lo que se mira es un recorte de la tabla: una búsqueda escrita o una vista con
 * filtros. Sin registros, entonces no es que la tabla esté vacía, sino que nada pasa el
 * recorte. Ordenar no recorta.
 */
export function esRecorte(lectura: Lectura, vista: BasesView | undefined): boolean {
  return lectura.q.trim() !== '' || (vista?.config.filters ?? []).length > 0;
}
