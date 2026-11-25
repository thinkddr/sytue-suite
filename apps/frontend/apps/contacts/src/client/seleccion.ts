/**
 * «Enviar correo a todos» y «Exportar (.vcf)» sobre la selección o sobre una
 * etiqueta. El mismo algoritmo, con los mismos nombres, en
 * `apps/fyne/contactos/seleccion.go`.
 */
import type { Contact } from './contactsApi.js';
import { parseVcardDetails, roleLine, type ContactDetails } from '../lib/vcard.js';

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
  const correos = correosParaCopiar(contactos);
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
  return enlaceRedactar(mailBase, { to: correos.join(', ') });
}

/**
 * El redactor de Mail con lo que se le pase ya escrito: destinatarios, asunto y cuerpo
 * de texto (`to`, `subject` y `body` que lee la bandeja de Mail, como `api.Redactar`).
 */
export function enlaceRedactar(
  mailBase: string,
  campos: { to?: string; subject?: string; body?: string },
): string {
  const q = new URLSearchParams({ action: 'compose' });
  for (const [k, v] of Object.entries(campos)) if (v) q.set(k, v);
  return `${mailBase}/?${q.toString()}`;
}

/**
 * El correo principal de cada uno, sin repetir (sin mirar mayúsculas) y SIN tope: lo
 * que copia «Copiar los correos» de la selección. El tope es del enlace a Mail, no de
 * lo que cabe en el portapapeles.
 */
export function correosParaCopiar(contactos: Contact[]): string[] {
  const vistos = new Set<string>();
  const correos: string[] = [];
  for (const c of contactos) {
    const correo = correoPrincipal(c)?.trim();
    if (!correo || vistos.has(correo.toLowerCase())) continue;
    vistos.add(correo.toLowerCase());
    correos.push(correo);
  }
  return correos;
}

/**
 * La ficha como texto para mandarla por correo («Compartir por correo»): el nombre, el
 * cargo y la organización, y cada correo, teléfono y dirección en su línea. Sin
 * etiquetas («Trabajo», «Casa») ni rótulos: se lee igual en cualquier idioma y quien la
 * recibe la copia tal cual. La de Fyne es `TextoDeFicha`.
 */
export function textoDeFicha(
  nombre: string,
  d: Pick<ContactDetails, 'title' | 'emails' | 'phones' | 'addresses'>,
  organizacion: string | undefined,
): string {
  const direccion = (a: ContactDetails['addresses'][number]): string =>
    [a.street, [a.postalCode, a.city].filter(Boolean).join(' '), a.region, a.country]
      .map((v) => (v ?? '').trim())
      .filter(Boolean)
      .join(', ');
  return [
    nombre,
    roleLine(d.title, organizacion),
    ...d.emails.map((e) => e.value),
    ...d.phones.map((p) => p.value),
    ...d.addresses.map(direccion),
  ]
    .map((l) => l.trim())
    .filter(Boolean)
    .join('\n');
}
