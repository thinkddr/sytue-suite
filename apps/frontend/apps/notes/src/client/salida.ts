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
 */

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
 * El HTML del editor con los colores de sus `style` en hexadecimal y sin los espacios de
 * ancho cero que deja el Markdown en la línea para seguir escribiendo fuera del formato.
 */
export function normalizarSalida(entrada: string): string {
  const html = entrada.includes('\u200b') ? entrada.replaceAll('\u200b', '') : entrada;
  // El paso rápido: casi ninguna pulsación trae un color.
  if (!/style="[^"]*rgba?\(/i.test(html)) return html;
  // Un `<template>`: su contenido es inerte y no carga nada.
  const caja = document.createElement('template');
  caja.innerHTML = html;
  for (const el of caja.content.querySelectorAll<HTMLElement>('[style]')) {
    const antes = el.getAttribute('style') ?? '';
    const despues = coloresEnHex(antes);
    if (despues !== antes) el.setAttribute('style', despues);
  }
  return caja.innerHTML;
}
