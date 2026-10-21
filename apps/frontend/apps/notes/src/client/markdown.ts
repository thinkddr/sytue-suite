/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Atajos de Markdown al escribir: «- », «1. », «# », «> » y «``` » al principio
 * de una línea se convierten en la lista, el título, la cita o el bloque de
 * código que nombran, y «---» seguido de Intro en una regla. Lo que hacen
 * Notion o Google Docs, con las MISMAS órdenes que pulsa la barra del editor:
 * nada nuevo que deshacer ni que sanear.
 *
 * Aquí solo vive la decisión (qué marcador dispara qué orden); el DOM lo toca
 * RichText, que además comprueba que el marcador abre la línea.
 */
export interface AtajoMarkdown {
  /** Orden de edición, la misma que aplica la barra. */
  cmd: string;
  valor?: string;
  /** Caracteres del marcador que hay que borrar antes de aplicar la orden. */
  borrar: number;
}

const POR_ESPACIO: [RegExp, (m: RegExpMatchArray) => AtajoMarkdown][] = [
  [/^[-*+]$/, () => ({ cmd: 'insertUnorderedList', borrar: 1 })],
  [/^1[.)]$/, () => ({ cmd: 'insertOrderedList', borrar: 2 })],
  [
    /^(#{1,5})$/,
    (m) => ({ cmd: 'formatBlock', valor: `<h${m[1]!.length}>`, borrar: m[1]!.length }),
  ],
  [/^>$/, () => ({ cmd: 'formatBlock', valor: '<blockquote>', borrar: 1 })],
  [/^```$/, () => ({ cmd: 'formatBlock', valor: '<pre>', borrar: 3 })],
  [
    /^(\[\]|\[\s?\]|[-*]\s*\[\s?\])$/,
    (m) => ({
      cmd: 'insertHTML',
      valor: '<div class="task-row"><input type="checkbox" /> <span>&nbsp;</span></div>',
      borrar: m[0].length,
    }),
  ],
];

/**
 * Qué dispara `tecla` cuando `linea` es lo que hay antes del cursor en su
 * línea. `null` = nada: la tecla sigue su curso.
 */
export function atajoMarkdown(linea: string, tecla: ' ' | 'Enter'): AtajoMarkdown | null {
  if (tecla === 'Enter') {
    return /^(-{3,}|\*{3,})$/.test(linea)
      ? { cmd: 'insertHorizontalRule', borrar: linea.length }
      : null;
  }
  for (const [re, hacer] of POR_ESPACIO) {
    const m = linea.match(re);
    if (m) return hacer(m);
  }
  return null;
}
