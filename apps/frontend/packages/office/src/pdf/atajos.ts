/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos de teclado del visor de PDF, en UNA tabla: de ella salen lo que hace
 * cada tecla (el `alPulsar` de PdfApp) y la hoja de «?» (`ShortcutsModal`). El visor
 * tenía diez atajos de escritorio y ninguna pantalla que los enseñara; con la tabla
 * la hoja no puede prometer una tecla que no hace nada, ni quedarse una sin explicar.
 * Fyne monta la misma hoja en `atajos.go` (`gruposDeAtajos`).
 */
import { atajoDelSistema, type GrupoDeAtajos } from '@sytue/ui';
import type { DocsKey, DocsT } from '../i18n';

/** Lo que hace el visor con una tecla. */
export type AccionVisor =
  | 'imprimir'
  | 'buscar'
  | 'acercar'
  | 'alejar'
  | 'ancho'
  | 'soltar'
  | 'siguiente'
  | 'anterior'
  | 'primera'
  | 'ultima'
  | 'ayuda';

/** Una pulsación: `KeyboardEvent.key`, con Ctrl (⌘ en el Mac) o sin él. */
export interface Combinacion {
  key: string;
  ctrl?: boolean;
}

export interface AtajoVisor {
  /** Las teclas como se dibujan; las de nombre largo («AvPág») van por el catálogo. */
  teclas: (t: DocsT) => string[];
  descripcion: DocsKey;
  /**
   * Lo que engancha el visor: cada pulsación y su acción. Vacío cuando lo hace el
   * navegador (las flechas y Espacio desplazan el `overflow-auto`, Ctrl+C copia la
   * capa de texto), que también es un atajo activo y se enseña igual.
   */
  combinaciones: { combinacion: Combinacion; accion: AccionVisor }[];
}

const ctrl = (key: string): Combinacion => ({ key, ctrl: true });
const tecla = (key: string): Combinacion => ({ key });
const conCtrl = (letra: string) => (): string[] => [atajoDelSistema(`Ctrl+${letra}`)];

export const GRUPOS_VISOR: { titulo: DocsKey; atajos: AtajoVisor[] }[] = [
  {
    titulo: 'pdf.shortcuts.group.move',
    atajos: [
      {
        teclas: (t) => [t('slides.present.key.pageDown')],
        descripcion: 'pdf.shortcuts.nextPage',
        combinaciones: [{ combinacion: tecla('PageDown'), accion: 'siguiente' }],
      },
      {
        teclas: (t) => [t('slides.present.key.pageUp')],
        descripcion: 'pdf.shortcuts.prevPage',
        combinaciones: [{ combinacion: tecla('PageUp'), accion: 'anterior' }],
      },
      {
        teclas: (t) => [t('slides.present.key.home'), t('slides.present.key.end')],
        descripcion: 'pdf.shortcuts.ends',
        combinaciones: [
          { combinacion: tecla('Home'), accion: 'primera' },
          { combinacion: tecla('End'), accion: 'ultima' },
        ],
      },
      {
        teclas: () => ['↑', '↓', '←', '→'],
        descripcion: 'pdf.shortcuts.scroll',
        combinaciones: [],
      },
      {
        teclas: (t) => [t('slides.present.key.space')],
        descripcion: 'pdf.shortcuts.screen',
        combinaciones: [],
      },
    ],
  },
  {
    titulo: 'pdf.shortcuts.group.view',
    atajos: [
      {
        teclas: conCtrl('F'),
        descripcion: 'pdf.shortcuts.search',
        combinaciones: [{ combinacion: ctrl('f'), accion: 'buscar' }],
      },
      {
        teclas: conCtrl('+'),
        descripcion: 'pdf.zoom.in',
        // `=` es el `+` sin Mayúsculas del teclado estadounidense.
        combinaciones: [
          { combinacion: ctrl('+'), accion: 'acercar' },
          { combinacion: ctrl('='), accion: 'acercar' },
        ],
      },
      {
        teclas: conCtrl('−'),
        descripcion: 'pdf.zoom.out',
        combinaciones: [{ combinacion: ctrl('-'), accion: 'alejar' }],
      },
      {
        teclas: conCtrl('0'),
        descripcion: 'pdf.zoom.width',
        combinaciones: [{ combinacion: ctrl('0'), accion: 'ancho' }],
      },
    ],
  },
  {
    titulo: 'pdf.shortcuts.group.actions',
    atajos: [
      { teclas: conCtrl('C'), descripcion: 'pdf.shortcuts.copy', combinaciones: [] },
      {
        teclas: conCtrl('P'),
        descripcion: 'pdf.menu.print',
        combinaciones: [{ combinacion: ctrl('p'), accion: 'imprimir' }],
      },
      {
        teclas: (t) => [t('slides.present.key.esc')],
        descripcion: 'pdf.shortcuts.escape',
        combinaciones: [{ combinacion: tecla('Escape'), accion: 'soltar' }],
      },
      {
        teclas: () => ['?'],
        descripcion: 'pdf.shortcuts.help',
        combinaciones: [{ combinacion: tecla('?'), accion: 'ayuda' }],
      },
    ],
  },
];

/** Lo que importa de un `keydown` (así se prueba sin DOM). */
export interface Pulsacion {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
}

/**
 * La acción de una pulsación, o null si no es un atajo del visor. Las de Ctrl van
 * aunque el foco esté en un campo (Ctrl+F desde el propio buscador sigue buscando
 * en el documento); las sueltas no, que Inicio en un campo va al principio de la
 * línea. Con Alt no hay atajo: AltGr es Ctrl+Alt en Windows y escribe caracteres.
 */
export function accionDeTecla(e: Pulsacion, enCampo: boolean): AccionVisor | null {
  if (e.altKey) return null;
  const conCtrl = e.ctrlKey || e.metaKey;
  if (!conCtrl && enCampo) return null;
  const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
  for (const g of GRUPOS_VISOR)
    for (const a of g.atajos)
      for (const { combinacion, accion } of a.combinaciones)
        if (combinacion.key === key && Boolean(combinacion.ctrl) === conCtrl) return accion;
  return null;
}

/** Los grupos de la hoja de «?», en el idioma de `t`. */
export function gruposDeAtajos(t: DocsT): GrupoDeAtajos[] {
  return GRUPOS_VISOR.map((g) => ({
    titulo: t(g.titulo),
    atajos: g.atajos.map((a) => ({ teclas: a.teclas(t), descripcion: t(a.descripcion) })),
  }));
}
