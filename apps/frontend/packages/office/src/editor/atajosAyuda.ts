/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja «Atajos de teclado» del editor (Ctrl+/ y Vista › Mostrar). Los atajos solo se
 * descubrían en el `title` de cada botón, y uno que no se ve no existe. La hoja NO es una
 * lista escrita a mano: sale del registro común (`comandos.json`), agrupada como la cinta
 * (pestaña › grupo), más los del marco que el registro no lleva porque no son comandos
 * (guardar, buscar, zoom, F6 y la propia hoja, en `atajos.ts` y `ribbon/banda.ts`). Un
 * atajo nuevo en el registro aparece aquí sin tocar nada; `test/atajos-ayuda.test.ts`
 * compara la hoja con el registro fila a fila.
 *
 * El gemelo de Fyne es `apps/fyne/editor/atajosayuda.go`: mismas filas del registro, en el
 * mismo orden; el marco nativo es el suyo (allí Ctrl+P imprime, no hay F6 y Ctrl+S y Ctrl+F
 * solo los atiende el folio de Docus).
 *
 * Módulo puro: sin componentes, para probarse sin DOM.
 */
import type { DocKind } from '@sytue/api-clients';
import { atajoDelSistema, esMac, type GrupoDeAtajos } from '@sytue/ui';
import type { DocsKey, DocsT } from '../i18n';
import { COMANDOS, type IdComando } from './comandos';
import { pestanasDe, type RibbonTabId } from './pestanas';

/**
 * Un atajo como lo lee quien tiene delante el teclado: «⌘⇧X» en un Mac y, en el resto,
 * con los nombres de tecla del idioma («Strg+Umschalt+X», «Ctrl+Mayús+X»).
 */
export function textoAtajo(keys: string, t: DocsT, mac: boolean = esMac()): string {
  if (mac)
    return atajoDelSistema(keys, true)
      .replace(/Shift\+/g, '⇧')
      .replace(/Alt\+/g, '⌥');
  return keys
    .replace(/^Ctrl\+/, `${t('shortcut.ctrl')}+`)
    .replace(/Shift\+/, `${t('shortcut.shift')}+`);
}

/** Una fila de la hoja: la combinación (escrita como en el registro) y qué hace. */
export interface FilaDeAyuda {
  readonly keys: string;
  readonly labelKey: DocsKey;
  /** El comando del registro del que sale; los del marco no tienen. */
  readonly id?: IdComando;
}

/** Un apartado: un grupo de la cinta dentro de su pestaña. */
export interface GrupoDeAyuda {
  readonly tab: RibbonTabId;
  readonly group: DocsKey;
  readonly filas: readonly FilaDeAyuda[];
}

/**
 * Los atajos que despachan un comando con valor dicen ese valor, no el rótulo del comando:
 * Ctrl+X no es «Copiar» y Ctrl+Alt+2 no es «Estilo de párrafo». La prueba exige que cada
 * atajo con valor del registro tenga aquí su rótulo, o la hoja tendría filas repetidas.
 */
export const ROTULO_DEL_VALOR: Readonly<Record<string, DocsKey>> = {
  'copy:cut': 'shortcut.cut',
  'paste:values': 'shortcut.pasteValues',
  'heading:p': 'style.normal',
  'heading:h1': 'style.h1',
  'heading:h2': 'style.h2',
  'heading:h3': 'style.h3',
};

/** La combinación que abre la hoja (y la cierra); va en el botón de Vista. */
export const ATAJO_AYUDA = 'Ctrl+/';

/**
 * Los atajos del marco del editor web: no son comandos del registro sino del cromo, y los
 * atienden `atajos.ts` (window) y `ribbon/banda.ts` (F6). Si uno cambia allí, cambia aquí.
 */
export const ATAJOS_DEL_MARCO: readonly FilaDeAyuda[] = [
  { keys: 'Ctrl+S', labelKey: 'shortcut.save' },
  { keys: 'Ctrl+F', labelKey: 'shortcut.find' },
  { keys: 'Ctrl++', labelKey: 'view.zoomIn' },
  { keys: 'Ctrl+-', labelKey: 'view.zoomOut' },
  { keys: 'Ctrl+0', labelKey: 'shortcut.zoomReset' },
  { keys: 'F6', labelKey: 'shortcut.ribbonJump' },
  { keys: ATAJO_AYUDA, labelKey: 'view.shortcuts' },
];

/** Los del marco que solo atiende Docus: Ctrl/⌘+Mayús+C abre el recuento (`recuento.ts`,
 *  en la barra de estado); en Calcus y Diapos no es de nadie. */
export const ATAJOS_DE_DOCUS: readonly FilaDeAyuda[] = [
  { keys: 'Ctrl+Shift+C', labelKey: 'wordcount.title' },
];

/** Los del texto del folio que no son del registro porque no llevan Ctrl: Mayús+F3 alterna
 *  las mayúsculas de lo elegido (`canvas/atajos.ts`). En la hoja de Docus, tras los suyos. */
export const ATAJOS_DEL_TEXTO: readonly FilaDeAyuda[] = [
  { keys: 'Shift+F3', labelKey: 'shortcut.cycleCase' },
];

/**
 * Los atajos del registro que existen en ese tipo de documento, agrupados como la cinta:
 * las pestañas en su orden y, dentro, los grupos y los comandos en el del registro. Van
 * TODOS, también los que atiende el propio lienzo (deshacer): funcionan igual.
 */
export function atajosDelRegistro(kind: DocKind): GrupoDeAyuda[] {
  const grupos: GrupoDeAyuda[] = [];
  for (const pestana of pestanasDe(kind)) {
    const porGrupo = new Map<DocsKey, FilaDeAyuda[]>();
    for (const id of Object.keys(COMANDOS) as IdComando[]) {
      const def = COMANDOS[id];
      if (def.tab !== pestana.id || !def.shortcuts || !def.kinds.includes(kind)) continue;
      let filas = porGrupo.get(def.group);
      if (!filas) porGrupo.set(def.group, (filas = []));
      for (const a of def.shortcuts) {
        const rotulo = a.value === undefined ? def.labelKey : ROTULO_DEL_VALOR[`${id}:${a.value}`];
        filas.push({ keys: a.keys, labelKey: rotulo ?? def.labelKey, id });
      }
    }
    for (const [group, filas] of porGrupo) grupos.push({ tab: pestana.id, group, filas });
  }
  return grupos;
}

/**
 * La hoja lista para `ShortcutsModal`: primero el marco, luego el registro y al final los
 * `propios` del tipo (en Diapos, las teclas de la presentación). Las teclas, ya escritas
 * para el sistema de quien lee (`textoAtajo`).
 */
export function gruposDeAtajos(
  kind: DocKind,
  t: DocsT,
  propios: readonly GrupoDeAtajos[] = [],
  mac: boolean = esMac(),
): GrupoDeAtajos[] {
  const fila = (f: FilaDeAyuda) => ({
    teclas: [textoAtajo(f.keys, t, mac)],
    descripcion: t(f.labelKey),
  });
  const rotuloPestana = new Map(pestanasDe(kind).map((p) => [p.id, t(p.labelKey)]));
  return [
    {
      titulo: t('shortcut.group.editor'),
      atajos: [
        ...ATAJOS_DEL_MARCO,
        ...(kind === 'document' ? [...ATAJOS_DE_DOCUS, ...ATAJOS_DEL_TEXTO] : []),
      ].map(fila),
    },
    ...atajosDelRegistro(kind).map((g) => ({
      titulo: `${rotuloPestana.get(g.tab) ?? g.tab} › ${t(g.group)}`,
      atajos: g.filas.map(fila),
    })),
    ...propios,
  ];
}

/**
 * ¿Es esta pulsación Ctrl/⌘+/? Por `key` o por la posición de la tecla (`Slash`), y
 * también la «/» del teclado numérico, que es la única que se escribe sin Mayús en un
 * teclado español. Con Mayús NO: en ese teclado Ctrl+Mayús+7 escribe «/» y es la lista
 * numerada del registro.
 */
export function esAtajoAyuda(
  e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey'> &
    Partial<Pick<KeyboardEvent, 'code'>>,
): boolean {
  if (!(e.ctrlKey || e.metaKey) || e.shiftKey || e.altKey) return false;
  return e.key === '/' || e.code === 'Slash' || e.code === 'NumpadDivide';
}
