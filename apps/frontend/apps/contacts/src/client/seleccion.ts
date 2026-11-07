/**
 * «Enviar correo a todos» y «Exportar (.vcf)» sobre la selección o sobre una
 * etiqueta. El mismo algoritmo, con los mismos nombres, en
 * `apps/fyne/contactos/seleccion.go`.
 */
import type { Contact } from './contactsApi.js';
import { parseVcardDetails } from '../lib/vcard.js';

/**
 * Cuántos destinatarios caben en el enlace a Mail. Van en la dirección
 * (`?to=`), y una dirección de miles de correos la corta antes el proxy que
 * el navegador; un correo a más de cien personas, además, es una lista de
 * distribución y no un correo.
 */
export const TOPE_DESTINATARIOS = 100;

/** El correo que abre «Enviar correo» en la ficha: el principal, o el primero. */
export function correoPrincipal(c: Contact): string | undefined {
  const emails = parseVcardDetails(c.vcard).emails;
  return (emails.find((e) => e.primary) ?? emails[0])?.value ?? c.emails[0];
}

/**
 * Un correo por contacto (el principal), sin repetir —el mismo correo en dos
 * fichas va una vez, sin importar mayúsculas— y hasta el tope. Quien no tiene
 * correo no cuenta.
 */
export function correosDe(contactos: Contact[]): { correos: string[]; recortado: boolean } {
  const vistos = new Set<string>();
  const correos: string[] = [];
  for (const c of contactos) {
    const correo = correoPrincipal(c)?.trim();
    if (!correo || vistos.has(correo.toLowerCase())) continue;
    vistos.add(correo.toLowerCase());
    correos.push(correo);
  }
  return {
    correos: correos.slice(0, TOPE_DESTINATARIOS),
    recortado: correos.length > TOPE_DESTINATARIOS,
  };
}

/**
 * El redactor de Mail con esos destinatarios: la raíz de la app con
 * `action=compose`, que es lo que abre el redactor (`api.RutaRedactar` en nativo).
 */
export function enlaceCorreoATodos(mailBase: string, correos: string[]): string {
  const q = new URLSearchParams({ action: 'compose', to: correos.join(', ') });
  return `${mailBase}/?${q.toString()}`;
}
