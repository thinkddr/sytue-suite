/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * REMITENTES DE CONFIANZA: de quién se enseñan las imágenes remotas sin preguntar.
 *
 * Cada correo con imágenes remotas se abre bloqueado (un píxel de seguimiento
 * delata cuándo y dónde se lee). Quien recibe cada semana el boletín de su
 * tienda tenía que pulsar «Mostrar imágenes» cada semana. «Mostrar siempre las
 * imágenes de este remitente» guarda la dirección en la CUENTA (servidor, por
 * cuenta), y desde entonces sus correos se abren con las imágenes, en cualquier
 * equipo.
 *
 * Las imágenes siguen pasando por nuestro proxy: confiar quita la pregunta, no la
 * protección de la IP. Y la confianza NO se aplica cuando hay motivos para dudar
 * de que el correo sea de quien dice (DMARC falla) ni en la carpeta de spam: el
 * `From` se puede falsificar, y confiar en «boletin@tienda.eu» no debe abrir las
 * imágenes a quien lo suplanta.
 *
 * MailApp guarda la lista y se la da al lector, que la reparte por contexto hasta
 * el cuerpo de cada mensaje, varios componentes más abajo (ThreadMessage →
 * MessageBodyView → HtmlBody).
 */
import { createContext } from 'preact';
import { normalizarRemitente } from '../client/mailApi.js';
import type { ResumenRemitente } from '../lib/autenticacionRemitente.js';

export interface ConfianzaDeImagenes {
  /** Las direcciones de confianza, ya normalizadas (minúsculas, sin espacios). */
  confiados: ReadonlySet<string>;
  /** Confía en un remitente; null si aquí no se puede (solo lectura, sin cuenta propia). */
  confiar: ((direccion: string) => void) | null;
}

/** Sin proveedor (tests, demo, lector sin cuenta): nadie es de confianza y no se ofrece confiar. */
export const SIN_CONFIANZA: ConfianzaDeImagenes = { confiados: new Set(), confiar: null };

export const ConfianzaImagenes = createContext<ConfianzaDeImagenes>(SIN_CONFIANZA);

/** Lo que el lector sabe de un mensaje para decidir si su remitente cuenta. */
export interface ContextoDelRemitente {
  /** El mensaje está en la carpeta de spam. */
  enSpam: boolean;
  /** Lo que dijo el MX del remitente (SPF/DKIM/DMARC). */
  autenticacion: ResumenRemitente;
}

/**
 * ¿Se le puede ofrecer (o aplicar) la confianza a este remitente? No en spam ni
 * con la autenticación en contra: ahí el `From` es justo lo que no se cree.
 */
export function remitenteConfiable(
  remitente: string | undefined,
  contexto: ContextoDelRemitente,
): boolean {
  return (
    Boolean(remitente?.includes('@')) && !contexto.enSpam && contexto.autenticacion !== 'aviso'
  );
}

/** ¿Se abren sus imágenes sin preguntar? Remitente confiable y en la lista. */
export function imagenesSinPreguntar(
  remitente: string | undefined,
  contexto: ContextoDelRemitente,
  confiados: ReadonlySet<string>,
): boolean {
  return (
    remitenteConfiable(remitente, contexto) && confiados.has(normalizarRemitente(remitente ?? ''))
  );
}
