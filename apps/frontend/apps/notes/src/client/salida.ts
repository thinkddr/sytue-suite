/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LO QUE EL EDITOR ENTREGA, EN LA FORMA QUE EL SANEADOR CONSERVA.
 *
 * El saneador de las notas (`apps/nucleo/notes/model/html.go`) deja pasar `color` y
 * `background-color` SOLO con `#rrggbb`. El navegador, al aplicar un color con
 * `execCommand`, lo escribe en el `style` como `rgb(220, 38, 38)`: el color del texto y
 * el resaltado se veían al escribir y desaparecían al guardar. Aquí se reescriben a
 * hexadecimal antes de entregar el HTML, sobre una copia: el DOM del editor no se toca y
 * la pila de deshacer sigue intacta.
 *
 * Y dos cosas más que el DOM del editor tiene y la nota no:
 *
 *   - Una imagen del texto se pinta con los datos de su adjunto (`data-adjunto`, ver
 *     `adjuntos-texto.ts`): se entrega nombrándolo, `src="adjunto:ID"`.
 *   - Los atributos de un bloque van en el orden en que se pulsaron (`<pre data-ajuste
 *     data-lineas>`). Se entregan en el de siempre —el de `BLOQUES_DE_NOTAS.sinValor`,
 *     el mismo que escribe Fyne—, para que la nota vuelva igual byte a byte.
 */
import { ESQUEMA_ADJUNTO, idDeAdjunto } from '@sytue/ui/browser/bloques-de-notas';

const RGB =
  /rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*(1|1\.0+|0?\.\d+|0))?\s*\)/gi;

const hex2 = (n: number): string => Math.max(0, Math.min(255, n)).toString(16).padStart(2, '0');

/** `rgb(…)` y `rgba(…, 1)` en `#rrggbb`. Con transparencia no hay hexadecimal de seis: se deja. */
export function coloresEnHex(estilo: string): string {
  return estilo.replace(RGB, (todo, r: string, g: string, b: string, a?: string) =>
    a !== undefined && Number(a) !== 1 ? todo : `#${hex2(+r)}${hex2(+g)}${hex2(+b)}`,
  );
}

/**
 * El HTML de `origen` (el editor, o la copia de lo seleccionado) con los colores de sus
 * `style` en hexadecimal y sin los espacios de ancho cero que deja el Markdown en la línea
 * para seguir escribiendo fuera del formato.
 *
 * Recibe los NODOS y no su HTML: se trabaja sobre una copia de ellos, sin volver a
 * interpretar como HTML lo que ya estaba en el DOM (CodeQL `js/xss-through-dom`, alerta
 * 488 de sytue-suite). Lo serializado y vuelto a analizar no siempre es lo mismo; la
 * copia de los nodos, sí.
 */
export function normalizarSalida(origen: Element): string {
  const html = origen.innerHTML;
  // El paso rápido: casi ninguna pulsación trae un color, una imagen o un bloque con
  // atributos que ordenar.
  const colores = /style="[^"]*rgba?\(/i.test(html);
  if (!colores && !html.includes('data-adjunto') && !FUERA_DE_ORDEN.test(html))
    return sinAnchoCero(html);
  // La copia va al documento inerte de un `<template>`: no carga nada y el DOM del editor
  // no se toca.
  const caja = document.createElement('template');
  const inerte = caja.content.ownerDocument;
  for (const hijo of origen.childNodes) caja.content.append(inerte.importNode(hijo, true));
  if (colores)
    for (const el of caja.content.querySelectorAll<HTMLElement>('[style]')) {
      const antes = el.getAttribute('style') ?? '';
      const despues = coloresEnHex(antes);
      if (despues !== antes) el.setAttribute('style', despues);
    }
  for (const img of caja.content.querySelectorAll<HTMLImageElement>('img[data-adjunto]')) {
    const id = idDeAdjunto(ESQUEMA_ADJUNTO + (img.getAttribute('data-adjunto') ?? ''));
    img.removeAttribute('data-adjunto');
    if (id) img.setAttribute('src', ESQUEMA_ADJUNTO + id);
    else img.remove();
  }
  for (const el of caja.content.querySelectorAll<HTMLElement>('pre, details'))
    ordenarAtributos(el, ORDEN[el.tagName.toLowerCase()] ?? []);
  return sinAnchoCero(caja.innerHTML);
}

const sinAnchoCero = (html: string): string =>
  html.includes('\u200b') ? html.replaceAll('\u200b', '') : html;

/** Un bloque con sus atributos fuera del orden de siempre (lo único que hay que ordenar). */
const FUERA_DE_ORDEN = /<pre [^>]*data-ajuste[^>]*data-lineas|<details [^>]*open[^>]*data-expandir/;

/** El orden de los atributos de un bloque: el de `BLOQUES_DE_NOTAS.sinValor` y Fyne. */
const ORDEN: Readonly<Record<string, readonly string[]>> = {
  pre: ['data-lineas', 'data-ajuste'],
  details: ['data-expandir', 'open'],
};

/** Pone primero, y en su orden, los atributos de `orden`; los demás detrás, como estaban. */
function ordenarAtributos(el: Element, orden: readonly string[]): void {
  const actuales = [...el.attributes].map((a) => [a.name, a.value] as const);
  const rango = (n: string) => {
    const i = orden.indexOf(n);
    return i < 0 ? orden.length : i;
  };
  const queridos = [...actuales].sort((a, b) => rango(a[0]) - rango(b[0]));
  if (queridos.every(([n], i) => n === actuales[i]![0])) return;
  for (const [n] of actuales) el.removeAttribute(n);
  for (const [n, v] of queridos) el.setAttribute(n, v);
}
