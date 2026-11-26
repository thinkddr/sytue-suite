/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El teclado de Contacts en UNA tabla: de ella salen lo que hace cada tecla
 * (`accionDeAtajo`, `accionDeFlecha`) y la hoja de «?» (`gruposDeAtajos`). Así la
 * hoja no puede prometer una tecla que no hace nada, ni una tecla quedarse sin
 * explicar. Son los de Google Contacts (`/`, `j`/`k`, `x`, `s`, `e`, `?`) más las
 * flechas de una lista de verdad. Va aparte de ContactsApp para poder probarlo
 * ejecutándolo, como la hoja de Mail.
 */
import type { GrupoDeAtajos } from '@sytue/ui';
import type { ContactsMessageKey, T } from '../i18n/index.js';

/** Lo que hace una tecla. Las mismas acciones que `accionAtajo` en Fyne. */
export type AccionDeAtajo =
  | 'siguiente'
  | 'anterior'
  | 'primero'
  | 'ultimo'
  | 'abrir'
  | 'buscar'
  | 'nuevo'
  | 'editar'
  | 'destacar'
  | 'marcar'
  | 'papelera'
  | 'ayuda';

interface FilaDeAtajo {
  /** Valores de `KeyboardEvent.key`. */
  teclas: string[];
  accion: AccionDeAtajo;
  /** Dónde se escucha: en toda la app (letras) o solo con el foco en la lista (flechas). */
  ambito: 'app' | 'lista';
}

/**
 * La tabla. El orden es el de la hoja. Intro no se escucha aquí: la fila es un
 * `<button>` y abrir con Intro (o Espacio) lo da el navegador; está en la tabla
 * solo para que la hoja lo cuente.
 */
const ATAJOS: { grupo: 'move' | 'actions'; filas: FilaDeAtajo[] }[] = [
  {
    grupo: 'move',
    filas: [
      { teclas: ['j', 'ArrowDown'], accion: 'siguiente', ambito: 'app' },
      { teclas: ['k', 'ArrowUp'], accion: 'anterior', ambito: 'app' },
      { teclas: ['Home'], accion: 'primero', ambito: 'lista' },
      { teclas: ['End'], accion: 'ultimo', ambito: 'lista' },
      { teclas: ['Enter'], accion: 'abrir', ambito: 'lista' },
      { teclas: ['/'], accion: 'buscar', ambito: 'app' },
    ],
  },
  {
    grupo: 'actions',
    filas: [
      { teclas: ['n'], accion: 'nuevo', ambito: 'app' },
      { teclas: ['e'], accion: 'editar', ambito: 'app' },
      { teclas: ['s'], accion: 'destacar', ambito: 'app' },
      { teclas: ['x'], accion: 'marcar', ambito: 'app' },
      { teclas: ['#'], accion: 'papelera', ambito: 'app' },
      { teclas: ['?'], accion: 'ayuda', ambito: 'app' },
    ],
  },
];

const FLECHAS = new Set(['ArrowDown', 'ArrowUp', 'Home', 'End']);

/**
 * La acción de una letra que se escucha en toda la app, o null. Las flechas no
 * cuentan aunque compartan fila con `j`/`k`: fuera de la lista son del
 * desplazamiento de la página y de los controles que las usan.
 */
export function accionDeAtajo(tecla: string): AccionDeAtajo | null {
  if (FLECHAS.has(tecla)) return null;
  for (const g of ATAJOS) {
    for (const f of g.filas) {
      if (f.ambito === 'app' && f.teclas.includes(tecla)) return f.accion;
    }
  }
  return null;
}

/** La acción de una flecha (↑ ↓ Inicio Fin) con el foco en una fila, o null. */
export function accionDeFlecha(tecla: string): AccionDeAtajo | null {
  if (!FLECHAS.has(tecla)) return null;
  for (const g of ATAJOS) {
    for (const f of g.filas) if (f.teclas.includes(tecla)) return f.accion;
  }
  return null;
}

/**
 * La fila a la que lleva un movimiento. `actual` es la del cursor; con `desde`
 * false (nada abierto y el foco fuera de la lista) la primera `j`/`k` aterriza
 * en el cursor en vez de saltárselo, como en Gmail. null si no hay filas o la
 * acción no mueve.
 */
export function filaDestino(
  accion: AccionDeAtajo,
  actual: number,
  total: number,
  desde = true,
): number | null {
  if (total <= 0) return null;
  const aqui = Math.min(Math.max(actual, 0), total - 1);
  switch (accion) {
    case 'primero':
      return 0;
    case 'ultimo':
      return total - 1;
    case 'siguiente':
      return desde ? Math.min(aqui + 1, total - 1) : aqui;
    case 'anterior':
      return desde ? Math.max(aqui - 1, 0) : aqui;
    default:
      return null;
  }
}

/** Cómo se ESCRIBE cada tecla en la hoja: símbolos, o la palabra en el idioma de quien lee. */
function rotuloDeTecla(t: T, tecla: string): string {
  switch (tecla) {
    case 'ArrowDown':
      return '↓';
    case 'ArrowUp':
      return '↑';
    case 'Enter':
      return '↵';
    case 'Home':
      return t('shortcuts.keyHome');
    case 'End':
      return t('shortcuts.keyEnd');
    default:
      return tecla;
  }
}

const DESCRIPCION: Record<AccionDeAtajo, ContactsMessageKey> = {
  siguiente: 'shortcuts.next',
  anterior: 'shortcuts.prev',
  primero: 'shortcuts.first',
  ultimo: 'shortcuts.last',
  abrir: 'shortcuts.open',
  buscar: 'list.search',
  nuevo: 'nav.newContact',
  editar: 'shortcuts.edit',
  destacar: 'shortcuts.star',
  marcar: 'shortcuts.select',
  papelera: 'batch.toTrash',
  ayuda: 'shortcuts.help',
};

/** La hoja de «?», agrupada como la pinta `ShortcutsModal`. */
export function gruposDeAtajos(t: T): GrupoDeAtajos[] {
  return ATAJOS.map((g) => ({
    titulo: t(g.grupo === 'move' ? 'shortcuts.group.move' : 'shortcuts.group.actions'),
    atajos: g.filas.map((f) => ({
      teclas: f.teclas.map((k) => rotuloDeTecla(t, k)),
      descripcion: t(DESCRIPCION[f.accion]),
    })),
  }));
}
