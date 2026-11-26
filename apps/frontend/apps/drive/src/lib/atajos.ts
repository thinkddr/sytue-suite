/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos de teclado de Drive, en UNA tabla, con el patrón de Calendar: de ella
 * salen lo que hace cada tecla (el oyente de FileBrowser) y la hoja de «?»
 * (`ShortcutsModal`). Así la hoja no puede prometer una tecla que no hace nada, ni
 * una tecla quedarse sin explicar. Antes solo existían «u» y «n», sin hoja: nadie
 * sabía que estaban.
 *
 * Son los de cualquier explorador de archivos (Supr, F2, Ctrl+A, Esc) más los de
 * las apps web de la suite (/, ?).
 */
import { atajoDelSistema, enCampoDeTexto, esMac, haySuperposicionAbierta } from '@sytue/ui';
import type { GrupoDeAtajos } from '@sytue/ui';
import type { DriveMessageKey, T } from '../i18n/index.js';

export type AccionAtajo =
  | 'search'
  | 'upload'
  | 'newFolder'
  | 'trash'
  | 'rename'
  | 'selectAll'
  | 'clearSelection'
  | 'toggleView'
  | 'help';

interface Atajo {
  /**
   * Las teclas, ya normalizadas por `teclaDeEvento`: un carácter en minúscula, el
   * `KeyboardEvent.key` de una tecla con nombre («Delete», «F2») o «Ctrl+a» para
   * ⌘/Ctrl más una letra.
   */
  teclas: string[];
  accion: AccionAtajo;
  descripcion: DriveMessageKey;
  /**
   * Necesita red. Sin ella la app se queda en solo lectura (la caché de la última
   * carpeta) y ni los botones ni las casillas de selección se pintan: la tecla
   * tampoco puede hacer lo que ellos no hacen.
   */
  conRed?: true;
}

const GRUPOS: { titulo: DriveMessageKey; atajos: Atajo[] }[] = [
  {
    titulo: 'shortcuts.group.actions',
    atajos: [
      { teclas: ['u'], accion: 'upload', descripcion: 'shortcuts.upload', conRed: true },
      { teclas: ['n'], accion: 'newFolder', descripcion: 'shortcuts.newFolder', conRed: true },
      // «Backspace» es la tecla «delete» de un teclado de Mac, que no tiene Supr.
      {
        teclas: ['Delete', 'Backspace'],
        accion: 'trash',
        descripcion: 'shortcuts.trash',
        conRed: true,
      },
      { teclas: ['F2'], accion: 'rename', descripcion: 'shortcuts.rename', conRed: true },
    ],
  },
  {
    titulo: 'shortcuts.group.selection',
    atajos: [
      { teclas: ['Ctrl+a'], accion: 'selectAll', descripcion: 'shortcuts.selectAll', conRed: true },
      { teclas: ['Escape'], accion: 'clearSelection', descripcion: 'shortcuts.clearSelection' },
    ],
  },
  {
    titulo: 'shortcuts.group.view',
    atajos: [
      { teclas: ['/'], accion: 'search', descripcion: 'shortcuts.search' },
      { teclas: ['v'], accion: 'toggleView', descripcion: 'shortcuts.toggleView' },
      { teclas: ['?'], accion: 'help', descripcion: 'shortcuts.help' },
    ],
  },
];

/**
 * La tecla de un `keydown` en la forma de la tabla, o null si lleva un modificador
 * que ningún atajo usa. ⌘ y Ctrl cuentan igual (los atajos de la suite escuchan los
 * dos); con ⌥ o con Mayús además de Ctrl la tecla es del navegador o del sistema.
 */
export function teclaDeEvento(e: KeyboardEvent): string | null {
  if (e.altKey) return null;
  const k = e.key.length === 1 ? e.key.toLowerCase() : e.key;
  if (e.ctrlKey || e.metaKey) return e.shiftKey ? null : `Ctrl+${k}`;
  return k;
}

/**
 * La acción de un `keydown`, o null si no es un atajo de Drive AHORA: ninguno salta
 * escribiendo en un campo —Ctrl+A en el buscador sigue marcando su texto—, ni con un
 * diálogo o un menú abiertos, que ya atienden el teclado, ni, sin red, los que
 * escriben.
 *
 * No usa `esAtajoDeApp` tal cual porque esa descarta cualquier modificador, y
 * Ctrl+A es justo un atajo con modificador; las otras dos comprobaciones son las
 * suyas, importadas.
 */
export function accionDeEvento(
  e: KeyboardEvent,
  { offline }: { offline: boolean },
): AccionAtajo | null {
  const tecla = teclaDeEvento(e);
  if (tecla === null) return null;
  if (enCampoDeTexto(e.target) || haySuperposicionAbierta()) return null;
  for (const g of GRUPOS) {
    for (const a of g.atajos) {
      if (!a.teclas.includes(tecla)) continue;
      return offline && a.conRed ? null : a.accion;
    }
  }
  return null;
}

/** Cómo se escribe una tecla de la tabla para quien la lee. */
function rotulo(tecla: string, t: T, mac: boolean): string {
  if (tecla === 'Delete') return t('shortcuts.key.delete');
  if (tecla === 'Backspace') return '⌫';
  if (tecla === 'Escape') return 'Esc';
  if (tecla.startsWith('Ctrl+'))
    return atajoDelSistema(`Ctrl+${tecla.slice(5).toUpperCase()}`, mac);
  return tecla;
}

/** Los grupos de la hoja de «?», en el idioma de `t` y con las teclas de este sistema.
 *  `solo` deja las acciones que la vista atiende de verdad (el Drive de la bóveda no
 *  selecciona, y una hoja con «Supr» o «⌘A» que no hacen nada engaña); un grupo que se
 *  queda sin ninguna no sale. */
export function gruposDeAtajos(
  t: T,
  mac: boolean = esMac(),
  solo?: readonly AccionAtajo[],
): GrupoDeAtajos[] {
  return GRUPOS.map((g) => ({
    titulo: t(g.titulo),
    atajos: g.atajos
      .filter((a) => !solo || solo.includes(a.accion))
      .map((a) => ({
        // En un Mac no hay Supr: se enseña solo ⌫. En el resto, solo Supr: «Retroceso»
        // también funciona, pero es la tecla que nadie espera que tire nada.
        teclas: a.teclas
          .filter((k) => (k === 'Delete' ? !mac : k === 'Backspace' ? mac : true))
          .map((k) => rotulo(k, t, mac)),
        descripcion: t(a.descripcion),
      })),
  })).filter((g) => g.atajos.length > 0);
}
