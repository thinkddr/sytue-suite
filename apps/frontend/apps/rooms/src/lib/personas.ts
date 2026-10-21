/**
 * Encontrar a alguien de OTRA organización de Sytue One desde un buscador de
 * personas (ADR 0040): lo tecleado es un correo o un @usuario y se busca en Sytue ID,
 * no en la lista del equipo. Lógica pura, sin DOM ni HTTP → testeable con `bun test`.
 */
import type { MessageKey } from '../i18n/index.js';
import { ChatHttpError } from '../client/chatApi.js';

/**
 * ¿Lo tecleado es un correo o un @usuario? Basta con la arroba: un nombre de persona
 * no la lleva, y cualquier cosa más fina (validar el correo aquí) sería una segunda
 * regla que acabaría discrepando de la de Sytue ID, que es la que manda.
 */
export function pareceIdentificador(query: string): boolean {
  const q = query.trim();
  return q.length > 1 && q.includes('@') && !/\s/.test(q);
}

/** Qué decir cuando la búsqueda falla: no es lo mismo «no existe» que «espera». */
export function claveDeFalloDeBusqueda(err: unknown): MessageKey {
  if (err instanceof ChatHttpError) {
    if (err.status === 404) return 'people.notFound';
    if (err.status === 429) return 'people.tooMany';
    if (err.status === 400) return 'people.invalid';
  }
  return 'people.failed';
}
