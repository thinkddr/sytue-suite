/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor de DOCUMENTOS (Docus): el folio y las pestañas que se ven en un documento.
 * Lo baja `cargarEditorDe('document')` (ver `./index.ts`).
 */
import { PageCanvas } from '../canvas/PageCanvas';
import { ColaboracionTab } from '../ribbon/tabs/Colaboracion';
import { DisposicionTab } from '../ribbon/tabs/Disposicion';
import { InicioTab } from '../ribbon/tabs/Inicio';
import { InsertarTab } from '../ribbon/tabs/Insertar';
import { ReferenciasTab } from '../ribbon/tabs/Referencias';
import { VistaTab } from '../ribbon/tabs/Vista';
import type { EditorDeTipo } from './index';

export const editor: EditorDeTipo = {
  Lienzo: PageCanvas,
  cuerpos: {
    inicio: InicioTab,
    insertar: InsertarTab,
    diseno: DisposicionTab,
    referencias: ReferenciasTab,
    revisar: ColaboracionTab,
    vista: VistaTab,
  },
  atajos: () => [],
};
