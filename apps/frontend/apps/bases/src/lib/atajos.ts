/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja de atajos de Bases («?»), en UNA tabla. La rejilla ya atendía flechas, Inicio,
 * Fin, RePág, AvPág, Intro, F2, Escape, Espacio, Supr, Ctrl+V y Alt+flechas, y lo único
 * que lo contaba era una frase bajo la tabla con tres de ellas: el resto había que
 * adivinarlo. Fyne monta la misma hoja (`apps/fyne/apps/bases/atajos.go`) y un test de
 * allí compara sus filas con estas.
 */
import { atajoDelSistema, type GrupoDeAtajos } from '@sytue/ui';
import type { BasesMessageKey } from '../i18n/index.js';

type T = (key: BasesMessageKey) => string;

interface FilaDeAtajo {
  /** Las teclas como se dibujan; las de nombre («Intro», «Supr») van por el catálogo. */
  teclas: (t: T) => string[];
  descripcion: BasesMessageKey;
}

export const GRUPOS_BASES: { titulo: BasesMessageKey; filas: FilaDeAtajo[] }[] = [
  {
    titulo: 'shortcuts.group.move',
    filas: [
      { teclas: () => ['↑', '↓', '←', '→'], descripcion: 'shortcuts.move' },
      {
        teclas: (t) => [t('shortcuts.key.home'), t('shortcuts.key.end')],
        descripcion: 'shortcuts.edges',
      },
      {
        teclas: (t) => [t('shortcuts.key.pageUp'), t('shortcuts.key.pageDown')],
        descripcion: 'shortcuts.page',
      },
    ],
  },
  {
    titulo: 'shortcuts.group.edit',
    filas: [
      { teclas: (t) => [t('shortcuts.key.enter'), 'F2'], descripcion: 'shortcuts.edit' },
      { teclas: (t) => [t('shortcuts.key.esc')], descripcion: 'shortcuts.cancel' },
      { teclas: (t) => [t('shortcuts.key.space')], descripcion: 'shortcuts.check' },
      { teclas: (t) => [t('shortcuts.key.delete')], descripcion: 'shortcuts.clear' },
      { teclas: () => [atajoDelSistema('Ctrl+C')], descripcion: 'shortcuts.copy' },
      { teclas: () => [atajoDelSistema('Ctrl+V')], descripcion: 'shortcuts.paste' },
      { teclas: () => ['Alt+←', 'Alt+→'], descripcion: 'shortcuts.moveField' },
      { teclas: () => ['?'], descripcion: 'shortcuts.help' },
    ],
  },
];

/** Los grupos de la hoja, en el idioma de `t`. */
export function gruposDeAtajos(t: T): GrupoDeAtajos[] {
  return GRUPOS_BASES.map((g) => ({
    titulo: t(g.titulo),
    atajos: g.filas.map((f) => ({ teclas: f.teclas(t), descripcion: t(f.descripcion) })),
  }));
}
