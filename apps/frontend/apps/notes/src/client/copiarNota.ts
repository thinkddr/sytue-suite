/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Copiar el texto» de una nota: el título y el cuerpo en texto plano, para pegarlo en
 * un correo, un chat o un formulario que no entiende el formato de Notes.
 *
 * La conversión es la del portapapeles de la suite (`htmlATexto`), la misma que
 * deja el texto plano al copiar desde el editor: listas con su viñeta, tareas con su
 * casilla, párrafos separados. Se hace en el navegador con el contenido ya descifrado:
 * de una nota cifrada no sale nada que no estuviera ya en pantalla.
 */
import { cargarHtml, htmlATexto } from './html.js';

/** El texto que se copia: el título (si lo hay), una línea en blanco y el cuerpo. */
export function unirTitulo(titulo: string, cuerpo: string): string {
  const t = titulo.trim();
  const c = cuerpo.trim();
  if (!t) return c;
  if (!c) return t;
  return `${t}\n\n${c}`;
}

/** Título y contenido HTML de la nota → el texto que va al portapapeles. */
export async function textoParaCopiar(titulo: string, html: string): Promise<string> {
  await cargarHtml();
  return unirTitulo(titulo, htmlATexto(html));
}
