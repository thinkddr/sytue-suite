/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor de HOJAS (Calcus): la rejilla con su motor de fórmulas
 * (`@sytue/formula/motor`, sin las funciones, que llegan aparte), las pestañas de la
 * hoja, sus atajos y la franja de la hoja protegida. Lo baja
 * `cargarEditorDe('spreadsheet')` (ver `./index.ts`): un documento o una presentación ya
 * no bajan el motor de fórmulas.
 */
import { cargarFunciones } from '@sytue/formula/motor';
import { atajosPropiosDeHoja } from '../sheets/atajosHoja';
import { FranjaHojaProtegida } from '../sheets/FranjaHojaProtegida';
import { SheetCanvas } from '../sheets/SheetCanvas';
import { ColaboracionTab } from '../ribbon/tabs/Colaboracion';
import { DatosTab, FormulasTab } from '../ribbon/tabs/Datos';
import { InicioSheetsTab } from '../ribbon/tabs/InicioSheets';
import { VistaTab } from '../ribbon/tabs/Vista';
import type { EditorDeTipo } from './index';

// Las funciones de fórmula van en su propio trozo: se piden en cuanto baja el editor de
// hojas, a la vez que monta, y el lienzo construye el motor de verdad al llegar (ver
// SheetCanvas). Un fallo de red lo reintenta el lienzo al montarse.
cargarFunciones().catch(() => undefined);

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
