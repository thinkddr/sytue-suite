/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mes · Semana · Día · Agenda. Lo pinta la cabecera de la vista y, en el mes del
 * teléfono —donde la cabecera no se pinta—, el cajón. Un solo marcado para los dos.
 *
 * En la cabecera va a su ancho natural, en una fila. En el cajón (`rejilla`) no cabe: el
 * cajón mide 260 y en una fila el grupo llega a 294 en finés y a 290 en búlgaro, y la
 * última vista salía cortada por el borde. Ahí es una rejilla de 2×2 que llena el ancho;
 * cada botón trunca antes que desbordar (el banco e2e afirma que en los 26 idiomas no
 * trunca ninguno). `GrupoVista.Rejilla` en Fyne.
 */
import type { JSX } from 'preact';
import { cx } from '@sytue/ui';
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
  return (
    <div
      class={cx(
        'bg-bg-subtle gap-0.5 rounded-md p-0.5',
        rejilla ? 'grid w-full grid-cols-2' : 'inline-flex items-center',
        cls,
      )}
      role="group"
      aria-label={t('view.label')}
    >
      {(['month', 'week', 'day', 'agenda'] as ViewMode[]).map((m) => (
        <button
          key={m}
          type="button"
          aria-pressed={mode === m}
          title={t('header.shortcut', {
            label: t(`view.${m}` as const),
            key: VIEW_KEY[m],
          })}
          onClick={() => onChange(m)}
          class={cx(
            'h-7 rounded-sm text-sm font-semibold transition-colors duration-150',
            rejilla ? 'min-w-0 truncate px-1.5' : 'px-2.5',
            'sytue-focus-ring',
            mode === m
              ? // `primary-text` y no `primary`: el relleno no se reasigna en
                // oscuro, así que sobre `surface` oscuro daba 2,96:1. El rol de
                // texto sí invierte (7,83:1 en claro, 8,03:1 en oscuro).
                'bg-surface text-primary-text shadow-[var(--shadow-xs)]'
              : // El grupo es una píldora CLARA sobre el cromo grafito: al pasar
                // por encima el texto se oscurece; `text-text` (blanco) aquí
                // dejaba la etiqueta invisible sobre `bg-surface`.
                'text-text-muted hover:bg-surface hover:text-text',
          )}
        >
          {m === 'month'
            ? t('view.month')
            : m === 'week'
              ? t('view.week')
              : m === 'day'
                ? t('view.day')
                : t('view.agenda')}
        </button>
      ))}
    </div>
  );
}
