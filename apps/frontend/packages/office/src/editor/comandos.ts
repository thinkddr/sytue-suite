/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Registro común de comandos de la cinta (ROADMAP-OFFICE, «Cinta de herramientas»).
 *
 * UNA sola fuente de verdad para lo que antes iba repartido por botón: la clave del
 * rótulo, el atajo, en qué tipos de documento existe, en qué pestaña y grupo vive, qué
 * pide para estar disponible (permiso de edición, una tabla o una imagen seleccionada) y
 * si se puede contar su uso. Los datos viven en `comandos.json` (copia de build de
 * `apps/nucleo/office/comandos.json`, que publica `scripts/sync-office-catalog.sh`) y no
 * aquí porque los lee también la app nativa: el mismo documento se edita en las dos, y dos vocabularios son
 * dos comportamientos (`apps/fyne/editor/costura.go`).
 *
 * La telemetría, si un día la hay, cuenta el `id` y el tipo de documento y NUNCA el
 * valor: el valor de un comando es contenido (una URL, un color, una fórmula, una
 * contraseña). `telemetry: false` marca los que ni eso.
 *
 * Los dos guardianes del registro:
 *  - De compilación: un comando de `EditorCommand`, `SheetCommand` o `SlidesCommand` sin
 *    su fila en el JSON no compila (`ComandosSinRegistrar`, abajo).
 *  - En Go: `TestCadaComandoNativoEstaEnElRegistro` (apps/fyne/editor) hace lo mismo
 *    con cada constante `Comando` de la app nativa.
 *
 * Módulo puro (sin componentes): lo importan la cinta, los lienzos y las macros, y se
 * prueba sin DOM en test/comandos.test.ts.
 */
import type { DocKind } from '@sytue/api-clients';
import type { DocsKey } from '../i18n';
import type { EditorCommand, FormatState } from './types';
import type { SheetCommand } from './sheets/SheetCanvas';
import type { SlidesCommand } from './slides/SlidesCanvas';
import type { RibbonTabId } from './pestanas';
import datos from './comandos.json';

/** Un atajo de teclado de un comando, escrito siempre con «Ctrl» («Ctrl+Shift+X»). */
export interface Atajo {
  readonly keys: string;
  /** Valor con el que se despacha el comando (Ctrl+X es `copy` con `cut`). */
  readonly value?: string;
  /** Lo atiende el propio lienzo editable (deshacer del navegador): se enseña, no se captura. */
  readonly canvas?: boolean;
}

/** Qué selección pide un comando: el cursor en una tabla o una imagen seleccionada. */
export type SeleccionRequerida = 'table' | 'image';

export interface DefComando {
  /** Clave del catálogo con el rótulo (aria-label y title del control). */
  readonly labelKey: DocsKey;
  /** Tipos de documento en los que el comando existe. */
  readonly kinds: readonly DocKind[];
  /** Pestaña de la cinta en la que vive (la web y Fyne usan los mismos ids). */
  readonly tab: RibbonTabId;
  /** Clave del rótulo del grupo de la cinta. */
  readonly group: DocsKey;
  /** Cambia el documento: apagado en solo lectura y fuera de las macros que se reproducen ahí. */
  readonly edits: boolean;
  /**
   * Un script de usuario puede despacharlo (ADR 0069). Ausente = no: la lista de los
   * scripts es más corta que la de las macros y lo que no se marca no se abre por olvido.
   */
  readonly script?: boolean;
  readonly selection?: SeleccionRequerida;
  /** El primero es el que se enseña en el tooltip. */
  readonly shortcuts?: readonly Atajo[];
  /** Se puede contar su uso (id y tipo, nunca el valor). */
  readonly telemetry: boolean;
}

/** Id de un comando: las claves del JSON, así que un id inventado no compila. */
export type IdComando = keyof typeof datos;

/*
 * Gate de compilación: todo comando de las tres uniones tiene su fila. Si falta uno, el
 * error lo nombra: «Type '"nuevo"' does not satisfy the constraint 'never'».
 */
type Vacio<T extends never> = T;
export type ComandosSinRegistrar = Vacio<
  Exclude<EditorCommand | SheetCommand | SlidesCommand, IdComando>
>;

/**
 * El registro. El JSON llega con sus valores como `string` —TypeScript no estrecha los
 * literales de un JSON—, así que el tipo se afirma aquí y los valores se validan campo a
 * campo en test/comandos.test.ts (que cada `labelKey` exista en el catálogo, etc.).
 */
export const COMANDOS = datos as unknown as Readonly<Record<IdComando, DefComando>>;

/** ¿Es este texto un id del registro? (frontera de confianza: macros guardadas). */
/** ¿Puede un script de usuario declarar y despachar este comando? */
export const esComandoDeScript = (id: string): boolean =>
  Object.hasOwn(COMANDOS, id) && COMANDOS[id as IdComando].script === true;

export const esComando = (id: string): id is IdComando => Object.hasOwn(COMANDOS, id);

/* ---------- Disponibilidad ---------- */

/** Por qué un comando no se puede usar ahora mismo. */
export type Bloqueo = 'soloLectura' | 'seleccion';

export interface EstadoEditor {
  soloLectura: boolean;
  fmt?: Pick<FormatState, 'inTable' | 'imageSelected'>;
}

/** `null` si el comando está disponible; si no, el motivo. */
export function bloqueo(id: IdComando, estado: EstadoEditor): Bloqueo | null {
  const def = COMANDOS[id];
  if (def.edits && estado.soloLectura) return 'soloLectura';
  if (def.selection === 'table' && estado.fmt && !estado.fmt.inTable) return 'seleccion';
  if (def.selection === 'image' && estado.fmt && !estado.fmt.imageSelected) return 'seleccion';
  return null;
}

/* ---------- Atajos ---------- */

interface Teclas {
  ctrl: boolean;
  shift: boolean;
  alt: boolean;
  key: string;
}

const leerTeclas = (keys: string): Teclas => {
  const partes = keys.split('+');
  const key = (partes.pop() ?? '').toLowerCase();
  return {
    ctrl: partes.includes('Ctrl'),
    shift: partes.includes('Shift'),
    alt: partes.includes('Alt'),
    key,
  };
};

/** Lo que un `keydown` necesita para casar con un atajo. */
export type Pulsacion = Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey'>;

/**
 * El comando que dispara esta pulsación en un lienzo de ese tipo, o `null`. Es el ÚNICO
 * manejador de atajos de comandos: el folio, las cajas de texto de Diapos, el editor de
 * celda y la rejilla de la hoja lo llaman en vez de llevar cada uno su lista. Ctrl y ⌘
 * valen igual, como en el resto de la suite.
 *
 * El texto editable (folio, caja de Diapos, editor de celda) pregunta con `'document'`:
 * sus comandos son los de `exec`, sea cual sea el documento.
 */
export function comandoDelAtajo(
  e: Pulsacion,
  kind: DocKind,
): { id: IdComando; value?: string } | null {
  if (!e.ctrlKey && !e.metaKey) return null;
  const key = e.key.toLowerCase();
  for (const id of Object.keys(COMANDOS) as IdComando[]) {
    const def = COMANDOS[id];
    if (!def.shortcuts || !def.kinds.includes(kind)) continue;
    for (const a of def.shortcuts) {
      if (a.canvas) continue;
      const t = leerTeclas(a.keys);
      if (t.ctrl && t.key === key && t.shift === e.shiftKey && t.alt === e.altKey) {
        return a.value === undefined ? { id } : { id, value: a.value };
      }
    }
  }
  return null;
}

/** El atajo que se enseña en el control de ese comando (el primero), si tiene. */
export const atajoDe = (id: IdComando): string | undefined => COMANDOS[id].shortcuts?.[0]?.keys;

/**
 * `aria-keyshortcuts` de un atajo: «Ctrl+Shift+X» → «Control+Shift+X Meta+Shift+X»,
 * las dos formas porque la cinta escucha Ctrl y ⌘.
 */
export function ariaAtajo(keys: string): string {
  const control = keys.replace(/^Ctrl\+/, 'Control+');
  return control.startsWith('Control+')
    ? `${control} ${control.replace(/^Control\+/, 'Meta+')}`
    : control;
}
