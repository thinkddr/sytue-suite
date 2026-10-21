/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La respuesta del asistente, convertida en DIAPOSITIVAS.
 *
 * En Diapos «insertar» no es escribir un párrafo: una presentación son láminas
 * con un título y unos puntos. Volcar la respuesta entera en una caja de texto
 * —que es lo que hacía el panel genérico— produce una diapositiva ilegible con
 * seiscientas palabras y ningún título.
 *
 * El corte lo marcan los TÍTULOS del modelo: cada uno abre una lámina y lo que
 * viene detrás es su cuerpo. Sin títulos, una sola lámina: es preferible una
 * diapositiva densa a inventarse cortes donde el modelo no los puso.
 */
import type { Bloque, Trozo } from '@sytue/ai';

export interface DiapoIA {
  /** Título de la lámina; vacío si el modelo no dio ninguno. */
  titulo: Trozo[];
  /** Puntos del cuerpo, uno por línea. */
  cuerpo: Bloque[];
}

/** Cuántos puntos caben en una lámina antes de que deje de leerse. */
export const PUNTOS_POR_DIAPO = 6;

/**
 * Reparte los bloques en diapositivas.
 *
 * SE PARTE TAMBIÉN POR TAMAÑO, no solo por título: un apartado con veinte
 * viñetas es una lámina que nadie lee desde la tercera fila. Al partir, la
 * continuación conserva el título —así se ve que sigue el mismo apartado— y esa
 * es la razón de que el corte no sea simplemente «una lámina por título».
 */
export function diapositivasDe(bloques: Bloque[]): DiapoIA[] {
  const salida: DiapoIA[] = [];
  let actual: DiapoIA | null = null;

  const abrir = (titulo: Trozo[]): void => {
    actual = { titulo, cuerpo: [] };
    salida.push(actual);
  };

  for (const b of bloques) {
    if (b.tipo === 'h1' || b.tipo === 'h2' || b.tipo === 'h3') {
      abrir(b.trozos);
      continue;
    }
    if (!actual) abrir([]);
    if (actual!.cuerpo.length >= PUNTOS_POR_DIAPO) {
      const titulo = actual!.titulo;
      abrir(titulo);
    }
    actual!.cuerpo.push(b);
  }

  // Una lámina con título y sin cuerpo es legítima (una portada); una sin nada,
  // no: sale de un texto vacío y no debe crear una diapositiva en blanco.
  return salida.filter((d) => d.titulo.length > 0 || d.cuerpo.length > 0);
}

/**
 * HTML de un tramo, con el texto ESCAPADO.
 *
 * El modelo aporta texto, nunca marcado: las únicas etiquetas que salen de aquí
 * son las que escribe esta función. Es lo que permite meter el resultado en el
 * `html` de una caja sin abrir la puerta a lo que devuelva un tercero.
 */
export function htmlDeTrozos(trozos: Trozo[], escapar: (s: string) => string): string {
  return trozos
    .map((t) => {
      const texto = escapar(t.texto);
      if (t.negrita) return `<strong>${texto}</strong>`;
      if (t.cursiva) return `<em>${texto}</em>`;
      return texto;
    })
    .join('');
}

/** HTML del cuerpo: viñetas si el modelo dio lista, párrafos si no. */
export function htmlDeCuerpo(cuerpo: Bloque[], escapar: (s: string) => string): string {
  if (cuerpo.length === 0) return '';
  const todoLista = cuerpo.every((b) => b.tipo === 'li');
  if (todoLista) {
    const puntos = cuerpo.map((b) => `<li>${htmlDeTrozos(b.trozos, escapar)}</li>`).join('');
    return `<ul>${puntos}</ul>`;
  }
  return cuerpo.map((b) => `<p>${htmlDeTrozos(b.trozos, escapar)}</p>`).join('');
}

/**
 * El HTML de cada caja de texto de la lámina, por orden: la primera lleva el
 * título y la segunda el cuerpo. Sin título, la disposición tiene UNA caja y esa
 * lleva el CUERPO —antes recibía el título vacío y el cuerpo se perdía—, igual
 * que `DiapositivasDe` en la app nativa.
 */
export function htmlDeCajas(d: DiapoIA, cajas: number, escapar: (s: string) => string): string[] {
  if (cajas === 1 && d.titulo.length === 0) return [htmlDeCuerpo(d.cuerpo, escapar)];
  return [htmlDeTrozos(d.titulo, escapar), htmlDeCuerpo(d.cuerpo, escapar)].slice(0, cajas);
}
