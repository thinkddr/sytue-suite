/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Leer un escaneo: qué páginas se miran y cómo se junta lo leído.
 *
 * POR QUÉ NO VA EN LA GPU. Vectorizar el carrete de Fotos es un lote semanal y
 * cabe en los veinticinco minutos de máquina al mes; leer un escaneo es a
 * petición de una persona que está esperando, tantas veces al día como
 * documentos abra. Una GPU encendida a demanda no es un lote, es un servidor, y
 * cuesta lo que cuesta un servidor. Esto va por Gemma 4 en las Generative APIs
 * de Scaleway, que se paga por consulta.
 *
 * CADA PÁGINA ES UNA LLAMADA DE VISIÓN Y SE PAGA. De ahí que aquí lo único que
 * hay sea un tope y un reparto: un contrato de trescientas páginas no se lee
 * entero «por si acaso».
 */

/** Tope de páginas que se leen de una vez. */
export const MAX_PAGINAS = 12;

/**
 * Qué páginas se leen, dadas las que tiene el documento y por dónde va quien
 * mira.
 *
 * EMPIEZA POR LA PÁGINA ACTUAL Y SIGUE HACIA DELANTE, que es lo que hace
 * cualquiera al leer. Empezar siempre por la primera obligaría a leer —y pagar—
 * doce páginas de portada y anexos para preguntar por algo de la página
 * cuarenta.
 */
export function paginasALeer(total: number, actual: number, tope = MAX_PAGINAS): number[] {
  if (total <= 0) return [];
  const desde = Math.min(Math.max(1, actual), total);
  const paginas: number[] = [];
  for (let p = desde; p <= total && paginas.length < tope; p++) paginas.push(p);
  // Si desde la página actual no se llega al tope, se completa hacia atrás: el
  // hueco que queda es contexto que ya está pagado por el mismo viaje.
  for (let p = desde - 1; p >= 1 && paginas.length < tope; p--) paginas.unshift(p);
  return paginas;
}

/** Lo leído de una página. `texto` vacío = la página no tenía nada legible. */
export interface PaginaLeida {
  pagina: number;
  texto: string;
}

/**
 * Junta lo leído en un solo texto, con la página delante.
 *
 * LA MARCA DE PÁGINA NO ES ADORNO: es lo que permite que el asistente cite
 * dónde está lo que afirma, igual que con un PDF que sí trae capa de texto. Sin
 * ella, una respuesta sobre un escaneo no se puede comprobar.
 *
 * Las páginas vacías se caen: un `[página 7]` sin nada debajo le dice al modelo
 * que ahí no hay nada, que es falso —lo que pasa es que no se ha sabido leer— y
 * le invita a rellenar el hueco.
 */
export function juntar(paginas: PaginaLeida[]): string {
  return paginas
    .filter((p) => p.texto.trim() !== '')
    .sort((a, b) => a.pagina - b.pagina)
    .map((p) => `[página ${p.pagina}]\n${p.texto.trim()}`)
    .join('\n\n');
}

/**
 * ¿Merece la pena lo que se ha leído?
 *
 * Un escaneo del que se sacan cuatro caracteres sueltos no es un documento
 * legible: es ruido, y dárselo al asistente produce una respuesta inventada con
 * aire de segura. Mejor decir que no se ha podido leer.
 */
export function esLegible(texto: string): boolean {
  return texto.replace(/\s+/g, '').length >= 40;
}
