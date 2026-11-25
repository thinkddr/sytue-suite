/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor de PRESENTACIONES (Diapos): el lienzo de diapositivas, sus pestañas y los
 * atajos de presentar. Lo baja `cargarEditorDe('presentation')` (ver `./index.ts`).
 */
import { presentShortcuts, SlidesCanvas } from '../slides/SlidesCanvas';
import { ColaboracionTab } from '../ribbon/tabs/Colaboracion';
import { DisenoSlidesTab, InicioSlidesTab, PresentarTab } from '../ribbon/tabs/InicioSlides';
import { InsertarTab } from '../ribbon/tabs/Insertar';
import { VistaTab } from '../ribbon/tabs/Vista';
import type { EditorDeTipo } from './index';

export const editor: EditorDeTipo = {
  Lienzo: SlidesCanvas,
  cuerpos: {
    inicio: InicioSlidesTab,
    insertar: InsertarTab,
    diseno: DisenoSlidesTab,
    presentar: PresentarTab,
    revisar: ColaboracionTab,
    vista: VistaTab,
  },
  // Con la pestaña delante, para que «Navegar» no se lea como algo del editor.
  atajos: (t) =>
    presentShortcuts(t).map((g) => ({
      ...g,
      titulo: `${t('ribbon.tab.present')} › ${g.titulo}`,
    })),
};
