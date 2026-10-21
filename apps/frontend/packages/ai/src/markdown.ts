/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El Markdown que devuelve el modelo, convertido en bloques neutros.
 *
 * POR QUÉ EXISTE. Los modelos escriben en Markdown aunque se les pida que no, y
 * el asistente insertaba su respuesta como TEXTO PLANO. Resultado, medido en
 * Sytue Docus: un documento lleno de `**Introducción**` y `1. **Ámbito**` en
 * crudo, con los asteriscos a la vista y sin un solo título de verdad.
 *
 * NO DEVUELVE HTML, y esa es la decisión que sostiene todo lo demás. El panel
 * tenía escrito que «la respuesta de la IA nunca inyecta HTML», y con razón: es
 * texto de un tercero. Aquí sale una estructura de datos con tipos cerrados, y
 * cada app la pinta con SUS PROPIOS comandos —el editor con `heading`, una nota
 * con sus etiquetas, un correo con las suyas—. Si esto devolviera HTML, la
 * primera app que lo inyectara reabriría el agujero para todas.
 *
 * SUBCONJUNTO DELIBERADO: títulos, listas, negrita y cursiva. Ni tablas, ni
 * enlaces, ni bloques de código. Lo que no se reconoce se queda como texto, que
 * es la degradación correcta: peor formato, nunca contenido perdido.
 */

/** Un tramo de texto con su énfasis. */
export interface Trozo {
  texto: string;
  negrita?: boolean;
  cursiva?: boolean;
}

/** Tipo de bloque. `li` lleva el nivel de lista en `ordenada`. */
export type TipoBloque = 'p' | 'h1' | 'h2' | 'h3' | 'li';

export interface Bloque {
  tipo: TipoBloque;
  trozos: Trozo[];
  /** Solo en `li`: si el punto venía numerado. */
  ordenada?: boolean;
}

/** Marcadores de lista sin numerar que se aceptan. */
const VINETA = /^\s*[-*+]\s+(.*)$/;
/** Lista numerada: `1.` o `1)`. */
const NUMERADA = /^\s*\d{1,3}[.)]\s+(.*)$/;
/** Títulos ATX: de una a tres almohadillas. */
const TITULO = /^\s*(#{1,3})\s+(.*)$/;

/**
 * Parte una línea en tramos según `**negrita**`, `*cursiva*` y `_cursiva_`.
 *
 * UNA PASADA Y SIN ANIDAR. `**muy _raro_**` sale como negrita entera y el
 * subrayado se queda en el texto; anidar de verdad pide un analizador con pila y
 * no compensa para lo que escribe un modelo en un correo o una nota.
 *
 * Los marcadores IMPARES no se comen: `2 * 3 = 6` no debe perder su asterisco.
 */
export function trozosDe(linea: string): Trozo[] {
  const salida: Trozo[] = [];
  const re = /\*\*(.+?)\*\*|__(.+?)__|\*(.+?)\*|_(.+?)_/g;
  let ultimo = 0;
  for (const m of linea.matchAll(re)) {
    const i = m.index ?? 0;
    if (i > ultimo) salida.push({ texto: linea.slice(ultimo, i) });
    const negrita = m[1] ?? m[2];
    const cursiva = m[3] ?? m[4];
    if (negrita !== undefined) salida.push({ texto: negrita, negrita: true });
    else if (cursiva !== undefined) salida.push({ texto: cursiva, cursiva: true });
    ultimo = i + m[0].length;
  }
  if (ultimo < linea.length) salida.push({ texto: linea.slice(ultimo) });
  return salida.filter((t) => t.texto !== '');
}

/**
 * Convierte el texto del modelo en bloques.
 *
 * Las líneas en blanco separan bloques y no producen ninguno: un documento con
 * párrafos vacíos entre títulos es lo que hacía que la respuesta insertada
 * ocupara el triple.
 */
export function bloquesDe(texto: string): Bloque[] {
  const bloques: Bloque[] = [];
  for (const cruda of texto.replace(/\r\n?/g, '\n').split('\n')) {
    const linea = cruda.trimEnd();
    if (linea.trim() === '') continue;

    const titulo = TITULO.exec(linea);
    if (titulo) {
      const nivel = (titulo[1] ?? '#').length;
      bloques.push({
        tipo: nivel === 1 ? 'h1' : nivel === 2 ? 'h2' : 'h3',
        trozos: trozosDe(titulo[2] ?? ''),
      });
      continue;
    }

    const num = NUMERADA.exec(linea);
    if (num) {
      bloques.push({ tipo: 'li', ordenada: true, trozos: trozosDe(num[1] ?? '') });
      continue;
    }

    const vin = VINETA.exec(linea);
    if (vin) {
      bloques.push({ tipo: 'li', ordenada: false, trozos: trozosDe(vin[1] ?? '') });
      continue;
    }

    // UN PÁRRAFO QUE ES SOLO NEGRITA ES UN TÍTULO, y no un capricho: los modelos
    // escriben `**Introducción**` en vez de `## Introducción` la mitad de las
    // veces. Sin esta regla, el caso reportado —un documento entero con
    // `**Introducción**` en crudo— seguiría saliendo sin un solo título.
    const trozos = trozosDe(linea);
    const soloNegrita =
      trozos.length === 1 && trozos[0]?.negrita === true && trozos[0].texto.length <= 80;
    bloques.push({
      tipo: soloNegrita ? 'h2' : 'p',
      trozos: soloNegrita ? [{ texto: trozos[0]!.texto }] : trozos,
    });
  }
  return bloques;
}

/** El texto pelado de un bloque, sin marcadores. Para superficies sin formato. */
export function textoPlano(bloques: Bloque[]): string {
  return bloques
    .map((b) => {
      const texto = b.trozos.map((t) => t.texto).join('');
      return b.tipo === 'li' ? `• ${texto}` : texto;
    })
    .join('\n');
}
