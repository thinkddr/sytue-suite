/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La taxonomía de pestañas de la cinta (ROADMAP-OFFICE, «Cinta de herramientas»): qué
 * pestañas hay, en qué orden y en qué tipos de documento se ven.
 *
 *   Inicio y Revisar, siempre · Insertar y Diseño, en documento y presentación ·
 *   Datos y Fórmulas, en la hoja · Presentar, en las diapositivas.
 *   (Referencias, del documento, y Vista, de todos, completan la tira.)
 *
 * Los datos viven en `pestanas.json`, copia de build de `apps/nucleo/office/pestanas.json`
 * (la publica `scripts/sync-office-catalog.sh`, igual que el registro de comandos):
 * la lee también la cinta nativa, y dos taxonomías son dos cintas distintas.
 *
 * El `tab` de cada fila de `comandos.json` apunta aquí, y `test/taxonomia.test.ts` exige
 * que esa pestaña exista en TODOS los tipos en que el comando existe: así un comando no
 * puede quedarse sin sitio en la cinta al mover una pestaña.
 *
 * Módulo puro (sin componentes): lo importan la cinta y el registro de comandos.
 */
import type { DocKind } from '@sytue/api-clients';
import type { DocsKey } from '../i18n';
import datos from './pestanas.json';

/** Id de una pestaña. `test/taxonomia.test.ts` comprueba que son los del JSON, ni más ni menos. */
export type RibbonTabId =
  | 'inicio'
  | 'insertar'
  | 'diseno'
  | 'referencias'
  | 'datos'
  | 'formulas'
  | 'presentar'
  | 'revisar'
  | 'vista';

export interface DefPestana {
  readonly id: RibbonTabId;
  /** Clave del catálogo; el rótulo se resuelve al pintar, con el idioma activo. */
  readonly labelKey: DocsKey;
  /** Tipos de documento en los que se ve. */
  readonly kinds: readonly DocKind[];
}

/** Las pestañas, en el orden en que se pintan. */
export const PESTANAS = datos.pestanas as unknown as readonly DefPestana[];

/** Las pestañas que se ven en un documento de ese tipo, en orden. */
export const pestanasDe = (kind: DocKind): readonly DefPestana[] =>
  PESTANAS.filter((p) => p.kinds.includes(kind));
