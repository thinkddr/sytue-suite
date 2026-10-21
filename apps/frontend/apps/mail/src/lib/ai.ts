/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Asistente de IA del redactor — helpers PUROS (sin DOM ni Preact).
 *
 * LAS ACCIONES NO SON LAS DE DOCS, y por eso esto no reutiliza `@sytue/office`: en un
 * documento se resume y se continúa; en un correo se ajusta el tono, se acorta y
 * se responde. Compartir el catálogo obligaría a enseñar «Continúa el texto» a
 * quien está escribiendo un correo de dos líneas.
 *
 * El BFF (`pages/api/ai.ts`) solo proxya; la inferencia la hace el servicio Go.
 */

/** Acciones rápidas del redactor. Sin petición libre: en un correo lo que se
 *  pide es ajustar lo escrito, no charlar con el modelo. */
export type AccionIA =
  'mejorar' | 'corregir' | 'acortar' | 'formal' | 'cercano' | 'responder' | 'traducir';

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
  mejorar:
    'Mejora la redacción de este correo (gramática, estilo y fluidez) sin cambiar lo que dice ni su tono. Devuelve solo el texto.',
  corregir:
    'Corrige la ortografía, la gramática y la puntuación de este correo. No reescribas ni cambies el estilo. Devuelve solo el texto corregido.',
  acortar:
    'Acorta este correo dejando lo esencial y todos los datos concretos (fechas, cifras, nombres). Devuelve solo el texto.',
  formal:
    'Reescribe este correo en un tono más formal y profesional, sin añadir información que no esté. Devuelve solo el texto.',
  cercano:
    'Reescribe este correo en un tono más cercano y natural, sin perder la información. Devuelve solo el texto.',
  responder:
    'Redacta una respuesta a este correo, breve y educada, en el mismo idioma. Devuelve solo el texto de la respuesta.',
};

export interface EntradaPeticion {
  accion: AccionIA;
  /** Idioma destino de «traducir». Se ignora en el resto. */
  idioma?: string;
  /** Texto sobre el que actuar: la selección, o el correo entero. */
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
      ? `Traduce este correo a ${(idioma ?? '').trim() || IDIOMA_POR_DEFECTO}. Devuelve solo la traducción.`
      : DIRECTIVAS[accion];
  return { action: accion, instruction: instruccion, context: contexto, kind: 'email' };
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
