/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El árbol de buzones del lateral, como el de Apple Mail: arriba los buzones
 * unificados de cada rol —«Entrada» junta la de Sytue y la de cada cuenta externa—
 * y debajo una sección por cuenta con sus carpetas. Es puro: lo pintan el lateral
 * de la web y el de Fyne (`arbolDeBuzones` en apps/fyne/apps/mail/buzones.go), que
 * pasan los mismos casos (apps/fyne/apps/mail/testdata/arbol-de-buzones.json).
 */
import type { Locale } from '@sytue/i18n';
import {
  isICloudMail,
  servesMail,
  type ExternalMailAccountSummary,
  type ExternalMailFolder,
  type ExternalMailSummaryState,
} from '@sytue/api-clients';
import { ROLE_ORDER, sortMailboxes } from './utils.js';

/** La cuenta de las fuentes que son carpetas de Sytue. */
export const CUENTA_SYTUE = '';

/** Una carpeta de Sytue tal y como la necesita el árbol (un `Mailbox` de JMAP). */
export interface CarpetaSytue {
  id: string;
  name: string;
  role?: string | undefined;
  unreadEmails: number;
}

/** Una carpeta que forma parte de un buzón unificado. */
export interface FuenteDeBuzon {
  /** La instancia de la cuenta externa, o `CUENTA_SYTUE`. */
  cuenta: string;
  carpeta: string;
}

/** «Todos los buzones de entrada», «Todos los enviados»…: un rol de todas las cuentas. */
export interface BuzonUnificado {
  rol: (typeof ROLE_ORDER)[number];
  /** Sytue primero y luego las cuentas en el orden de la lista. */
  fuentes: FuenteDeBuzon[];
  noLeidos: number;
}

/** La sección de una cuenta externa. */
export interface CuentaDeBuzones {
  id: string;
  /** La dirección; sin ella, el nombre de la conexión. */
  titulo: string;
  icloud: boolean;
  /** Una cuenta desconectada pide volver a iniciar sesión, diga lo que diga el resumen. */
  estado: ExternalMailSummaryState;
  /** Los no leídos de su Entrada, como el contador de Apple Mail junto a la cuenta. */
  noLeidos: number;
  /** En el orden del lateral; vacías salvo con `estado` «ok». */
  carpetas: ExternalMailFolder[];
}

export interface ArbolDeBuzones {
  unificados: BuzonUnificado[];
  cuentas: CuentaDeBuzones[];
}

/**
 * Arma el árbol con las carpetas de Sytue y el resumen de las cuentas externas
 * (`GET /external-mail/summary`). Un rol sale en los unificados si lo tiene alguna
 * cuenta; las carpetas propias (sin rol) no se unifican. Las cuentas conectadas
 * solo para Calendario, Contactos o Notas no salen.
 */
export function arbolDeBuzones(
  locale: Locale,
  sytue: readonly CarpetaSytue[],
  externas: readonly ExternalMailAccountSummary[],
): ArbolDeBuzones {
  const cuentas = externas.filter(servesMail).map((cuenta): CuentaDeBuzones => {
    const estado = cuenta.connected ? cuenta.state : 'reconnect';
    const carpetas = estado === 'ok' ? sortMailboxes(locale, cuenta.folders) : [];
    return {
      id: cuenta.instance_id,
      titulo: cuenta.address || cuenta.name,
      icloud: isICloudMail(cuenta),
      estado,
      noLeidos: carpetas.find((c) => c.role === 'inbox')?.unread ?? 0,
      carpetas,
    };
  });
  const unificados: BuzonUnificado[] = [];
  for (const rol of ROLE_ORDER) {
    const fuentes: FuenteDeBuzon[] = [];
    let noLeidos = 0;
    const propia = sytue.find((c) => c.role === rol);
    if (propia) {
      fuentes.push({ cuenta: CUENTA_SYTUE, carpeta: propia.id });
      noLeidos += propia.unreadEmails;
    }
    for (const cuenta of cuentas) {
      const carpeta = cuenta.carpetas.find((c) => c.role === rol);
      if (!carpeta) continue;
      fuentes.push({ cuenta: cuenta.id, carpeta: carpeta.id });
      noLeidos += carpeta.unread ?? 0;
    }
    if (fuentes.length > 0) unificados.push({ rol, fuentes, noLeidos });
  }
  return { unificados, cuentas };
}
