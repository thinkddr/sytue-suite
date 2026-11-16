/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «AÑADIR A CONTACTOS» / «VER EN CONTACTOS» DESDE EL REMITENTE (MAIL-12).
 *
 * Puro: lo usan el lector (si ofrece la entrada) y el BFF (si el remitente ya está en
 * la libreta). Fyne decide lo mismo con su copia en apps/fyne/apps/mail/libreta.go.
 */

/**
 * Buzones que no leen respuestas: guardarlos en la libreta solo ensucia el
 * autocompletado de Para, y nadie los quiere como contacto. Se mira la parte local
 * sin signos (`no-reply`, `no_reply`, `noreply`, `do-not-reply`…).
 */
const SIN_RESPUESTA = /^(noreply|donotreply|mailerdaemon|postmaster|bounces?)$/;

/** ¿Es una dirección que no admite respuesta (`no-reply@…`, `mailer-daemon@…`)? */
export function esSinRespuesta(correo: string): boolean {
  const local = correo.trim().toLowerCase().split('@')[0] ?? '';
  // `bounces+1234@…` y `noreply-avisos@…`: lo que va tras el `+` o tras un separador
  // final con más texto sigue siendo el mismo buzón que no lee.
  const base = local.split('+')[0] ?? '';
  const plana = base.replace(/[-_.]/g, '');
  return SIN_RESPUESTA.test(plana) || /^no[-_.]?reply[-_.]/.test(base);
}

/** Dónde está el hilo y quién es uno mismo: lo que decide si se ofrece la entrada. */
export interface ContextoContacto {
  /** El hilo está en Spam: guardar al remitente sería premiar al que se ha marcado. */
  enSpam: boolean;
  /** Las direcciones propias (la principal, alias y buzones con envío). */
  propias: readonly string[];
}

/**
 * ¿Se ofrece «Añadir a Contactos» / «Ver en Contactos» para este remitente? No en
 * Spam, ni para un `no-reply`, ni para uno mismo (la respuesta propia desde Enviados).
 */
export function ofrecerContacto(correo: string, ctx: ContextoContacto): boolean {
  const c = correo.trim().toLowerCase();
  if (!c.includes('@') || ctx.enSpam || esSinRespuesta(c)) return false;
  return !ctx.propias.some((p) => p.trim().toLowerCase() === c);
}

/** Un contacto de la búsqueda de Contacts, en la forma del servicio. */
export interface ContactoDelServicio {
  id?: string;
  emails?: { email?: string }[];
}

/**
 * El id del contacto que tiene EXACTAMENTE esa dirección, o null. La búsqueda de
 * Contacts es por subcadena: `ana@sytue.eu` también trae a `mariana@sytue.eu`, y
 * «Ver en Contactos» no puede abrir la ficha de otra persona.
 */
export function contactoConCorreo(
  contactos: readonly ContactoDelServicio[],
  correo: string,
): string | null {
  const c = correo.trim().toLowerCase();
  for (const k of contactos) {
    if (!k.id) continue;
    if ((k.emails ?? []).some((e) => (e.email ?? '').trim().toLowerCase() === c)) return k.id;
  }
  return null;
}

/** El nombre con el que se guarda: el del remitente, o su dirección si no trae. */
export function nombreDelContacto(nombre: string | undefined, correo: string): string {
  return (nombre ?? '').trim() || correo.trim();
}
