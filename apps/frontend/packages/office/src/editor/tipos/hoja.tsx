/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor de HOJAS (Calcus): la rejilla con su motor de fórmulas (`@sytue/formula`),
 * las pestañas de la hoja, sus atajos y la franja de la hoja protegida. Lo baja
 * `cargarEditorDe('spreadsheet')` (ver `./index.ts`): un documento o una presentación ya
 * no bajan el motor de fórmulas.
 */
import { atajosPropiosDeHoja } from '../sheets/atajosHoja';
import { FranjaHojaProtegida } from '../sheets/FranjaHojaProtegida';
import { SheetCanvas } from '../sheets/SheetCanvas';
import { ColaboracionTab } from '../ribbon/tabs/Colaboracion';
import { DatosTab, FormulasTab } from '../ribbon/tabs/Datos';
import { InicioSheetsTab } from '../ribbon/tabs/InicioSheets';
import { VistaTab } from '../ribbon/tabs/Vista';
import type { EditorDeTipo } from './index';

export const editor: EditorDeTipo = {
  Lienzo: SheetCanvas,
  cuerpos: {
    inicio: InicioSheetsTab,
    datos: DatosTab,
    formulas: FormulasTab,
    revisar: ColaboracionTab,
    vista: VistaTab,
  },
  atajos: (t) => atajosPropiosDeHoja(t),
  Franja: FranjaHojaProtegida,
};
