/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/** Cuántas etiquetas entran en el título. Cuatro ya no es un nombre, es una lista. */
const TOPE_ETIQUETAS = 3;

/**
 * Un nombre propuesto para un álbum, a partir de lo que Sytue AI vio en sus
 * fotos.
 *
 * SIN NINGUNA LLAMADA. Las etiquetas ya están escritas de cuando se describió
 * cada foto; pedirle a un modelo que resuma lo que ya está resumido sería pagar
 * una consulta por un `title` que quien lo crea va a reescribir la mitad de las
 * veces.
 *
 * SOLO LO QUE COMPARTE LA MITAD, y como poco dos fotos. Lo que sale en una sola
 * no es el tema del álbum: es lo que había en una de sus fotos, y ponerlo en el
 * nombre convierte «Playa» en «Playa · sombrilla · gaviota». Con una sola foto
 * no se propone nada, porque «lo que tienen en común» no significa nada ahí.
 *
 * DEVUELVE VACÍO cuando no hay nada claro, y entonces el diálogo pide el nombre
 * en blanco como siempre. Un nombre inventado a la fuerza es peor que ninguno:
 * el que llega escrito se acepta sin leerlo, y el álbum se queda llamándose
 * «foto» para siempre.
 */
export function tituloSugerido(fotos: readonly { tags?: string[] | undefined }[]): string {
  if (fotos.length < 2) return '';
  const cuenta = new Map<string, number>();
  for (const f of fotos) {
    // Únicas POR FOTO: una foto con «playa» tres veces en sus etiquetas no
    // vota tres veces.
    for (const etiqueta of new Set((f.tags ?? []).map((x) => x.trim().toLowerCase()))) {
      if (etiqueta) cuenta.set(etiqueta, (cuenta.get(etiqueta) ?? 0) + 1);
    }
  }
  const minimo = Math.max(2, Math.ceil(fotos.length / 2));
  const elegidas = [...cuenta.entries()]
    .filter(([, n]) => n >= minimo)
    // Por frecuencia, y a igualdad alfabético: sin el segundo criterio el
    // nombre cambiaría de una vez a otra con las mismas fotos.
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, TOPE_ETIQUETAS)
    .map(([etiqueta]) => etiqueta);
  if (elegidas.length === 0) return '';
  const nombre = elegidas.join(' · ');
  return nombre.charAt(0).toLocaleUpperCase() + nombre.slice(1);
}
