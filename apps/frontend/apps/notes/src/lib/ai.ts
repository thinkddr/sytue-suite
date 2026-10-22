/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Asistente de IA del editor de notas — helpers PUROS (sin DOM ni Preact).
 *
 * LAS ACCIONES NO SON LAS DE DOCS NI LAS DE MAIL. Una nota es un cuaderno: lo
 * que se pide ahí es ordenar lo apuntado deprisa, sacar las tareas que hay
 * escondidas en un párrafo y ampliar una idea suelta. No se ajusta el tono —a
 * nadie— ni se redacta una respuesta: no hay destinatario.
 *
 * El BFF (`pages/api/ai.ts`) solo proxya; la inferencia la hace el servicio Go.
 */

/** Acciones rápidas del redactor. Sin petición libre: en un correo lo que se
 *  pide es ajustar lo escrito, no charlar con el modelo. */
export type AccionIA =
  'ordenar' | 'corregir' | 'resumir' | 'tareas' | 'ampliar' | 'titular' | 'traducir';

/** Tope del contexto enviado, en caracteres. El BFF acota también por bytes. */
export const MAX_CONTEXTO = 12000;

/** Idioma destino por defecto de «traducir» si no se elige otro. */
const IDIOMA_POR_DEFECTO = 'inglés';

/**
 * Directiva en lenguaje natural por acción.
 *
 * TODAS TERMINAN DICIENDO QUE SE DEVUELVA SOLO EL TEXTO. Sin eso el modelo
 * contesta «Claro, aquí tienes tu correo mejorado:» y ese preámbulo acaba
 * pegado en el cuerpo del mensaje que alguien envía.
 */
const DIRECTIVAS: Record<Exclude<AccionIA, 'traducir'>, string> = {
  ordenar:
    'Ordena estas notas en apartados con títulos y viñetas, sin añadir ni quitar información. Usa Markdown. Devuelve solo el texto.',
  corregir:
    'Corrige la ortografía, la gramática y la puntuación. No reescribas ni cambies el estilo. Devuelve solo el texto corregido.',
  resumir:
    'Resume estas notas conservando los datos concretos (fechas, cifras, nombres). Devuelve solo el texto.',
  tareas:
    'Saca las tareas pendientes que haya en estas notas, una por línea y empezando por un verbo. Si se dice de quién se espera algo, ponlo. Devuelve solo la lista.',
  ampliar:
    'Desarrolla estas notas en un texto completo, sin inventar datos que no estén. Devuelve solo el texto.',
  titular:
    'Escribe un título de menos de sesenta caracteres para estas notas. Devuelve solo el título, sin comillas ni punto final.',
};

export interface EntradaPeticion {
  accion: AccionIA;
  /** Idioma destino de «traducir». Se ignora en el resto. */
  idioma?: string;
  /** Texto sobre el que actuar: la selección, o la nota entera. */
  texto?: string;
}

/** Cuerpo que se envía a POST /api/ai. Lo reenvía el BFF tal cual. */
export interface CuerpoPeticion {
  action: string;
  instruction: string;
  context: string;
  kind: string;
}

/**
 * Compone el cuerpo de la petición. Función total: no valida vacíos —de eso se
 * encarga quien llama, deshabilitando el botón—, solo transforma.
 */
export function construirPeticion({ accion, idioma, texto }: EntradaPeticion): CuerpoPeticion {
  const contexto = (texto ?? '').trim().slice(0, MAX_CONTEXTO);
  const instruccion =
    accion === 'traducir'
      ? `Traduce estas notas a ${(idioma ?? '').trim() || IDIOMA_POR_DEFECTO}. Devuelve solo la traducción.`
      : DIRECTIVAS[accion];
  return { action: accion, instruction: instruccion, context: contexto, kind: 'note' };
}

/**
 * Saca el texto de la respuesta del servicio (`{ text }`); `null` si el cuerpo no
 * tiene la forma esperada.
 *
 * DEVUELVE `null` Y NO LANZA porque el mensaje de error lo pone la interfaz desde
 * su catálogo: una función pura no sabe en qué idioma habla quien mira.
 */
export function leerRespuesta(bruto: unknown): string | null {
  if (bruto === null || typeof bruto !== 'object' || !('text' in bruto)) return null;
  // El `in` de arriba ya estrecha el tipo: TypeScript sabe que `text` existe.
  const { text } = bruto;
  return typeof text === 'string' && text.trim() !== '' ? text.trim() : null;
}
