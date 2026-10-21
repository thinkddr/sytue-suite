/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ¿Esta tecla es para la app, o es de quien está escribiendo?
 *
 * La comprobación de la que depende que teclear sea seguro. Sin ella, escribir
 * «este» en el asunto de un correo lo archiva y «#3» lo tira a la papelera. Está
 * escrita TRES veces en la suite —Mail en su `utils.ts`, Calendar dentro del
 * `useEffect` de la rejilla, Docs dentro del visor de PDF— y ninguna de las tres
 * comprueba lo mismo: Mail no mira si hay un diálogo delante (lo lleva en su
 * estado, así que solo funciona para SUS diálogos), Calendar sí pero solo
 * `[role="dialog"]`, y la de Docs no mira modificadores.
 *
 * Las tres partes tienen que ir juntas, porque cada una tapa un agujero distinto:
 *
 *   · Modificadores fuera: ⌘R sigue recargando, ⌘F sigue siendo buscar y ⌥ es de
 *     los acentos del teclado.
 *   · Campos y `contenteditable` fuera: el cuerpo del compositor de Mail y el
 *     lienzo de Docs son `contenteditable`, no `<textarea>`.
 *   · Capas flotantes fuera: un diálogo, un popover o un menú abiertos ya están
 *     escuchando el teclado, y ejecutar además el atajo de la app te mueve el
 *     sitio dos veces de una tecla.
 */
import { useEffect, useState } from 'preact/hooks';

const SUPERPOSICIONES = '[role="dialog"], [role="menu"]';

/** El foco está en algo donde se escribe (campo o `contenteditable`). */
export function enCampoDeTexto(destino: EventTarget | null): boolean {
  const el = destino as HTMLElement | null;
  if (!el) return false;
  return el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/i.test(el.tagName ?? '');
}

/**
 * Hay una capa flotante abierta que ya atiende el teclado.
 *
 * Se mira el DOM y no un estado: la capa puede ser de otro componente, de otra
 * isla o de la propia shell, y ninguna app puede llevar la cuenta de todas. Los
 * dos roles son los que pintan `Modal`/`Popover` (dialog) y `Dropdown`/`AppGrid`
 * (menu) de este paquete, y solo los pintan cuando están abiertos.
 *
 * `listbox` NO cuenta, aunque también sea una capa en la búsqueda global: es el
 * rol de una `List` seleccionable —la lista de correos, la de contactos—, que
 * está siempre puesta. Incluirlo apagaría todos los atajos de esas apps. La
 * búsqueda global ya queda fuera por el campo de texto, que es donde vive el foco.
 */
export function haySuperposicionAbierta(): boolean {
  if (typeof document === 'undefined') return false;
  return document.querySelector(SUPERPOSICIONES) !== null;
}

/** El teclado pertenece a la capa enfocada, aunque su portal quede fuera del diálogo padre. */
export function tecladoEnOtraSuperposicion(panel: HTMLElement | null): boolean {
  if (typeof document === 'undefined') return false;
  const capa = document.activeElement?.closest(SUPERPOSICIONES);
  return capa !== null && capa !== undefined && capa !== panel;
}

/** ¿Este `keydown` es un atajo de la aplicación (c, j/k, Escape, /…)? */
export function esAtajoDeApp(e: KeyboardEvent): boolean {
  if (e.ctrlKey || e.metaKey || e.altKey) return false;
  if (enCampoDeTexto(e.target)) return false;
  return !haySuperposicionAbierta();
}

/**
 * ¿Hay un Mac (o un iPhone/iPad) delante? Solo decide cómo se ESCRIBE un atajo:
 * equivocarse no rompe nada, porque los atajos de la suite escuchan ⌘ y Ctrl a la
 * vez. `navigator.platform` está desaconsejado, pero es el único que dice el
 * sistema sin pedir permiso; estaba copiado en cinco sitios.
 */
export function esMac(): boolean {
  return typeof navigator !== 'undefined' && /Mac|iP(hone|ad|od)/i.test(navigator.platform);
}

/**
 * Un atajo escrito como lo lee quien tiene delante el teclado: tal cual en Linux y
 * Windows («Ctrl+G», «Ctrl K») y con los símbolos del Mac en macOS e iOS («⌘G»,
 * «⌘K»). Se escribe SIEMPRE con «Ctrl» y pasa por aquí. Rehacer es la excepción:
 * ⌘⇧Z en un Mac, no ⌘Y. Es el `ui.AtajoDelSistema` de las apps nativas.
 */
export function atajoDelSistema(atajo: string, mac: boolean = esMac()): string {
  if (!mac) return atajo;
  return atajo
    .replace(/^Ctrl\+Y$/, '⌘⇧Z')
    .replace(/Ctrl[+ ]/g, '⌘')
    .replace(/Mayús\+/g, '⇧');
}

/**
 * `atajoDelSistema` para pintarlo en una isla que también se sirve por SSR: el
 * servidor no sabe si hay un Mac delante, así que la pista sale vacía y se escribe
 * al montar (pintar «Ctrl K» para cambiarlo luego parpadea).
 */
export function useAtajo(atajo: string): string {
  const [texto, setTexto] = useState('');
  useEffect(() => setTexto(atajoDelSistema(atajo)), [atajo]);
  return texto;
}
