/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que el editor y la lectura de una nota hacen sobre su DOM y que no es una
 * orden de edición: buscar dentro de la nota y plegar una lista.
 */
import { rangos } from '@sytue/i18n';

/**
 * Las coincidencias de `q` en el texto de `raiz`, como rangos del DOM, con la
 * regla de `rangos` de @sytue/i18n (sin acentos ni mayúsculas).
 * ponytail: se busca nodo de texto a nodo de texto, así que algo partido por el
 * formato («reu<b>nión</b>») no sale; unir los nodos es lo que toca si se echa en falta.
 */
export function coincidenciasEn(raiz: Node, q: string): Range[] {
  const out: Range[] = [];
  if (!q.trim()) return out;
  const w = document.createTreeWalker(raiz, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) {
    for (const [a, b] of rangos((n as Text).data, q)) {
      const r = document.createRange();
      r.setStart(n, a);
      r.setEnd(n, b);
      out.push(r);
    }
  }
  return out;
}

/** La clase que marca un elemento de lista plegado. La deja pasar el saneador del servicio. */
export const CLASE_PLEGADA = 'plegada';

/**
 * El elemento de lista que se pliega desde `nodo`: el `<li>` que lo contiene, si
 * tiene una sublista —dentro, o justo detrás, que es como la anida `indent` en
 * Chromium—. Sin sublista no hay nada que plegar.
 */
export function liPlegable(nodo: Node | null): HTMLLIElement | null {
  const el = nodo?.nodeType === Node.TEXT_NODE ? nodo.parentElement : (nodo as Element | null);
  const li = el?.closest('li') ?? null;
  if (!li) return null;
  const sub = li.querySelector(':scope > ul, :scope > ol');
  const siguiente = li.nextElementSibling;
  return sub || (siguiente && /^(UL|OL)$/.test(siguiente.tagName)) ? li : null;
}

/**
 * El HTML de una nota con el `n`-ésimo `<li>` plegado o desplegado. Es lo que
 * hace pulsar la viñeta en la lectura: el plegado se guarda con la nota, como en
 * Twos, y se ve igual en la web y en el escritorio.
 */
export function alternarPlegado(html: string, n: number): string {
  // Un `<template>` y no un `div`: su contenido es inerte, no carga imágenes.
  const caja = document.createElement('template');
  caja.innerHTML = html;
  const li = caja.content.querySelectorAll('li')[n];
  if (!li) return html;
  li.classList.toggle(CLASE_PLEGADA);
  if (li.classList.length === 0) li.removeAttribute('class');
  return caja.innerHTML;
}
