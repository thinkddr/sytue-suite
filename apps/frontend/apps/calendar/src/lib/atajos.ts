/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos de teclado de Calendar, en UNA tabla: de ella salen lo que hace cada
 * tecla (el oyente de CalendarApp) y la hoja de «?» (`ShortcutsModal`). Así la hoja
 * no puede prometer una tecla que no hace nada, ni una tecla quedarse sin explicar.
 * Son los de Apple Calendar y Google Calendar.
 */
import type { GrupoDeAtajos } from '@sytue/ui';
import type { CalendarMessageKey, T } from '../i18n/index.js';

export type AccionAtajo =
  'today' | 'next' | 'prev' | 'month' | 'week' | 'day' | 'agenda' | 'create' | 'search' | 'help';

interface Atajo {
  /** Las teclas, ya normalizadas: un carácter en minúscula. */
  teclas: string[];
  accion: AccionAtajo;
  descripcion: CalendarMessageKey;
}

const GRUPOS: { titulo: CalendarMessageKey; atajos: Atajo[] }[] = [
  {
    titulo: 'shortcuts.group.navigate',
    atajos: [
      { teclas: ['t'], accion: 'today', descripcion: 'shortcuts.today' },
      { teclas: ['n', 'j'], accion: 'next', descripcion: 'shortcuts.next' },
      { teclas: ['p', 'k'], accion: 'prev', descripcion: 'shortcuts.prev' },
    ],
  },
  {
    titulo: 'shortcuts.group.views',
    atajos: [
      { teclas: ['m'], accion: 'month', descripcion: 'view.month' },
      { teclas: ['w'], accion: 'week', descripcion: 'view.week' },
      { teclas: ['d'], accion: 'day', descripcion: 'view.day' },
      { teclas: ['a'], accion: 'agenda', descripcion: 'view.agenda' },
    ],
  },
  {
    titulo: 'shortcuts.group.actions',
    atajos: [
      { teclas: ['c'], accion: 'create', descripcion: 'nav.createEvent' },
      { teclas: ['/'], accion: 'search', descripcion: 'header.search' },
      { teclas: ['?'], accion: 'help', descripcion: 'shortcuts.help' },
    ],
  },
];

/** La acción de una tecla (`KeyboardEvent.key`), o null si no es un atajo. */
export function accionDeTecla(key: string): AccionAtajo | null {
  const k = key.length === 1 ? key.toLowerCase() : key;
  for (const g of GRUPOS) for (const a of g.atajos) if (a.teclas.includes(k)) return a.accion;
  return null;
}

/** Los grupos de la hoja de «?», en el idioma de `t`. */
export function gruposDeAtajos(t: T): GrupoDeAtajos[] {
  return GRUPOS.map((g) => ({
    titulo: t(g.titulo),
    atajos: g.atajos.map((a) => ({ teclas: a.teclas, descripcion: t(a.descripcion) })),
  }));
}
