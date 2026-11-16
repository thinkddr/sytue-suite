/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos de teclado del ERP, en UNA tabla: de ella salen lo que hace cada tecla
 * (el `alPulsar` de ErpApp) y la hoja de «?» (`ShortcutsModal`). El ERP tenía ⌘K,
 * «/», «n» y Escape y ninguna pantalla que los enseñara; con la tabla, la hoja no
 * puede prometer una tecla que no hace nada ni quedarse una sin explicar. Fyne monta
 * la misma hoja en `apps/erp/atajos.go` (`gruposDelERP`), y un test de allí compara
 * las filas con estas.
 */
import { atajoDelSistema, type GrupoDeAtajos } from '@sytue/ui';
import type { ErpMessageKey } from '../i18n/index.js';

type T = (key: ErpMessageKey) => string;

/** Lo que hace el ERP con una tecla. */
export type AccionErp = 'paleta' | 'buscar' | 'nueva' | 'cerrar' | 'ayuda';

/** Una pulsación: `KeyboardEvent.key`, con Ctrl (⌘ en el Mac) o sin él. */
interface Combinacion {
  key: string;
  ctrl?: boolean;
}

interface AtajoErp {
  /** Las teclas como se dibujan; las de nombre («Esc») van por el catálogo. */
  teclas: (t: T) => string[];
  descripcion: ErpMessageKey;
  accion: AccionErp;
  combinaciones: Combinacion[];
}

export const GRUPOS_ERP: { titulo: ErpMessageKey; atajos: AtajoErp[] }[] = [
  {
    titulo: 'shortcuts.group.move',
    atajos: [
      {
        teclas: () => [atajoDelSistema('Ctrl+K')],
        descripcion: 'shortcuts.palette',
        accion: 'paleta',
        combinaciones: [{ key: 'k', ctrl: true }],
      },
      {
        teclas: () => ['/'],
        descripcion: 'shortcuts.search',
        accion: 'buscar',
        combinaciones: [{ key: '/' }],
      },
    ],
  },
  {
    titulo: 'shortcuts.group.actions',
    atajos: [
      {
        teclas: () => ['n'],
        descripcion: 'shortcuts.new',
        accion: 'nueva',
        combinaciones: [{ key: 'n' }],
      },
      {
        teclas: (t) => [t('shortcuts.key.esc')],
        descripcion: 'shortcuts.close',
        accion: 'cerrar',
        combinaciones: [{ key: 'Escape' }],
      },
      {
        teclas: () => ['?'],
        descripcion: 'shortcuts.help',
        accion: 'ayuda',
        combinaciones: [{ key: '?' }],
      },
    ],
  },
];

/**
 * La acción de un `keydown`, o null si no es un atajo del ERP. Ctrl y ⌘ cuentan
 * igual; sin ellos, la tecla tiene que ser exactamente la de la tabla (⌘N es del
 * navegador, no «n»). Mayúsculas no cuenta: «?» la necesita en casi todos los
 * teclados.
 */
export function accionDeTecla(ev: {
  key: string;
  ctrlKey?: boolean | undefined;
  metaKey?: boolean | undefined;
}): AccionErp | null {
  const ctrl = Boolean(ev.ctrlKey || ev.metaKey);
  for (const g of GRUPOS_ERP) {
    for (const a of g.atajos) {
      for (const c of a.combinaciones) {
        if (Boolean(c.ctrl) !== ctrl) continue;
        // Con Ctrl, la letra en mayúscula (Bloq Mayús, ⇧) sigue siendo la misma.
        if (ctrl ? ev.key.toLowerCase() === c.key : ev.key === c.key) return a.accion;
      }
    }
  }
  return null;
}

/** Los grupos de la hoja de «?», en el idioma de `t`. */
export function gruposDeAtajos(t: T): GrupoDeAtajos[] {
  return GRUPOS_ERP.map((g) => ({
    titulo: t(g.titulo),
    atajos: g.atajos.map((a) => ({ teclas: a.teclas(t), descripcion: t(a.descripcion) })),
  }));
}
