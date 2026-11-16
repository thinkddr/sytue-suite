/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mes · Semana · Día · Agenda. Lo pinta la cabecera de la vista y, en el mes del
 * teléfono —donde la cabecera no se pinta—, el cajón. Un solo marcado para los dos: el
 * `SegmentedControl` de `@sytue/ui`, un `radiogroup` con flechas.
 *
 * En la cabecera va a su ancho natural, en una fila. En el cajón (`rejilla`) no cabe: el
 * cajón mide 260 y en una fila el grupo llega a 294 en finés y a 290 en búlgaro, y la
 * última vista salía cortada por el borde. Ahí es una rejilla de 2×2 que llena el ancho;
 * cada botón trunca antes que desbordar (el banco e2e afirma que en los 26 idiomas no
 * trunca ninguno). `ui.Segmentado` con `Rejilla = 2` en Fyne.
 */
import type { JSX } from 'preact';
import { SegmentedControl } from '@sytue/ui';
import type { ViewMode } from './dates.js';
import type { T } from '../i18n/index.js';

/** Tecla que conmuta cada vista (la misma letra que en Google Calendar). */
export const VIEW_KEY: Record<ViewMode, string> = { month: 'M', week: 'W', day: 'D', agenda: 'A' };

export function SelectorVista({
  t,
  mode,
  onChange,
  rejilla = false,
  class: cls,
}: {
  t: T;
  mode: ViewMode;
  onChange: (m: ViewMode) => void;
  /** 2×2 a todo el ancho: el cajón del teléfono. */
  rejilla?: boolean;
  class?: string;
}): JSX.Element {
  const rotulos: Record<ViewMode, string> = {
    month: t('view.month'),
    week: t('view.week'),
    day: t('view.day'),
    agenda: t('view.agenda'),
  };
  return (
    <SegmentedControl
      aria-label={t('view.label')}
      opciones={(['month', 'week', 'day', 'agenda'] as ViewMode[]).map((m) => ({
        valor: m,
        rotulo: rotulos[m],
        title: t('header.shortcut', { label: rotulos[m], key: VIEW_KEY[m] }),
      }))}
      valor={mode}
      onChange={onChange}
      rejilla={rejilla ? 2 : undefined}
      class={cls}
    />
  );
}
