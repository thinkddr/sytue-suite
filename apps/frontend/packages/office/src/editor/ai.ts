/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Asistente de IA — helpers PUROS (sin DOM ni Preact): construcción del mensaje
 * que el panel envía a POST /api/ai y parseo de la respuesta. Toda la lógica de
 * «prompting» (mapa acción→directiva, recorte del contexto) vive aquí y está
 * testeada; el BFF (api/ai.ts) SOLO proxya al servicio Go configurable
 * (SYTUE_AI_URL), que hace la inferencia real (memoria «backend siempre Go»).
 */
import type { DocKind } from '@sytue/api-clients';
import type { DocsKey } from '../i18n';

/** Acciones rápidas del panel; 'chat' = petición libre («pide lo que quieras»). */
export type AiAction =
  | 'resumir'
  | 'reescribir-claro'
  | 'reescribir-formal'
  | 'reescribir-corto'
  | 'traducir'
  | 'mejorar'
  | 'corregir'
  | 'explicar'
  | 'continuar';

/** Tope del contexto enviado (caracteres).
 *  ponytail: recorte duro; subir aquí y en el servicio Go si admite ventanas mayores. */
export const MAX_CONTEXT_CHARS = 12000;

/** Idioma destino por defecto de «traducir» si el usuario no elige otro. */
const DEFAULT_LANG = 'inglés';

/** Directiva en español por acción (petición en lenguaje natural). «traducir» se
 *  compone aparte porque lleva el idioma destino. */
const INSTRUCTIONS: Record<Exclude<AiAction, 'traducir'>, string> = {
  resumir: 'Resume el texto de forma concisa, conservando las ideas principales.',
  'reescribir-claro':
    'Reescribe el texto para que sea más claro y fácil de leer, sin cambiar su significado.',
  'reescribir-formal': 'Reescribe el texto en un tono más formal y profesional.',
  'reescribir-corto': 'Reescribe el texto de forma más breve, eliminando lo superfluo.',
  mejorar:
    'Mejora la redacción del texto (gramática, estilo y fluidez), conservando su significado.',
  corregir:
    'Corrige la ortografía, la gramática y la puntuación del texto. Devuelve solo el texto corregido.',
  explicar: 'Explica de forma sencilla el significado del texto.',
  continuar: 'Continúa el texto de forma coherente con su estilo y contenido.',
};

export interface AiRequestInput {
  /** Acción rápida o 'chat' (petición libre). */
  action: AiAction | 'chat';
  /** Texto libre: la petición del chat, o el idioma destino de «traducir». */
  prompt?: string;
  /** Contexto: texto de la selección o del documento. */
  context?: string;
  kind: DocKind;
}

/** Cuerpo que se envía a POST /api/ai (el BFF lo reenvía verbatim al servicio Go). */
export interface AiRequestBody {
  action: AiAction | 'chat';
  /** Directiva ya resuelta en lenguaje natural (español). */
  instruction: string;
  /** Contexto recortado a MAX_CONTEXT_CHARS. */
  context: string;
  kind: DocKind;
}

/**
 * Compone el cuerpo de la petición: resuelve la directiva de la acción (o usa el
 * prompt libre en 'chat', o el idioma en 'traducir') y RECORTA el contexto a
 * MAX_CONTEXT_CHARS —no enviar el documento entero sin límite—. Función total: no
 * valida vacíos (el panel deshabilita el envío), solo transforma.
 */
export const buildAiRequest = (input: AiRequestInput): AiRequestBody => {
  const prompt = (input.prompt ?? '').trim();
  const context = (input.context ?? '').trim().slice(0, MAX_CONTEXT_CHARS);
  const instruction =
    input.action === 'chat'
      ? prompt
      : input.action === 'traducir'
        ? `Traduce el texto a ${prompt || DEFAULT_LANG}. Devuelve solo la traducción.`
        : INSTRUCTIONS[input.action];
  return { action: input.action, instruction, context, kind: input.kind };
};

/**
 * Extrae el texto de la respuesta del servicio ({ text }); `null` si el cuerpo no
 * tiene la forma esperada (frontera de confianza: llega de un servicio externo a
 * través del BFF). Devuelve `null` en vez de lanzar porque el mensaje de error lo
 * pone el panel desde el catálogo (`ai.error.badResponse`): una función pura no
 * puede saber el idioma del usuario.
 */
export const parseAiResponse = (raw: unknown): string | null => {
  if (raw !== null && typeof raw === 'object' && 'text' in raw) {
    const { text } = raw;
    if (typeof text === 'string' && text.trim() !== '') return text.trim();
  }
  return null;
};

/** Extrae el código de error del cuerpo del BFF ({ error }); undefined si no lo hay. */
export const aiErrorCode = (data: unknown): string | undefined => {
  if (data !== null && typeof data === 'object' && 'error' in data) {
    const { error } = data;
    if (typeof error === 'string') return error;
  }
  return undefined;
};

/** Clave del catálogo a partir del código de error que emite el BFF (o genérica). */
export const aiErrorKey = (code?: string): DocsKey => {
  switch (code) {
    case 'ai_not_configured':
      return 'ai.error.notConfigured';
    case 'unauthenticated':
      return 'ai.error.unauthenticated';
    case 'ai_timeout':
      return 'ai.error.timeout';
    case 'bad_request':
      return 'ai.error.badRequest';
    case 'upstream_unreachable':
    case 'ai_failed':
      return 'ai.error.unreachable';
    default:
      return 'ai.error.generic';
  }
};
