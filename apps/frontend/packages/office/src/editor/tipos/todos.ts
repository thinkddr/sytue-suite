/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SOLO PARA LOS BANCOS DE PRUEBAS (`editor/dev`) Y LAS PRUEBAS: los tres editores de
 * tipo cargados de forma estática. La isla del editor NO lo importa: baja el suyo con
 * `cargarEditorDe` (ver `./index.ts`).
 */
import type { DocKind } from '@sytue/api-clients';
import type { EditorDeTipo } from './index';
import { editor as documento } from './documento';
import { editor as hoja } from './hoja';
import { editor as presentacion } from './presentacion';

export const EDITORES: Readonly<Record<DocKind, EditorDeTipo>> = {
  document: documento,
  spreadsheet: hoja,
  presentation: presentacion,
};
