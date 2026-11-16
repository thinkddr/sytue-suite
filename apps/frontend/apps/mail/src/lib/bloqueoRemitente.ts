/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «BLOQUEAR A …»: SUS CORREOS VAN A SPAM (MAIL-13).
 *
 * Es «Crear una regla para este remitente» en un clic: una regla `from` → Spam y el
 * hilo abierto movido a Spam ya. Puro (sin red ni estado de la vista): lo orquesta
 * MailApp con las llamadas de mailApi y lo prueba bloqueo-remitente.test.ts. Fyne
 * hace lo mismo en apps/fyne/apps/mail/triaje.go (`bloquear`).
 */
import type { MailFilterInput } from '@sytue/api-clients/mail';

/** Dónde está el hilo y quién es uno mismo: lo que decide si se ofrece la entrada. */
export interface ContextoBloqueo {
  /** El hilo ya está en Spam: bloquear no movería nada que no esté ya ahí. */
  enSpam: boolean;
  /** Las direcciones propias (la principal, alias y buzones con envío). */
  propias: readonly string[];
}

/**
 * ¿Se ofrece «Bloquear a …»? No en Spam, ni sin dirección, ni a uno mismo: una regla
 * con tu propia dirección mandaría a Spam tus respuestas en cada hilo. A un `no-reply`
 * sí: los boletines que no se dejan dar de baja son justo a quien se bloquea.
 */
export function ofrecerBloqueo(correo: string, ctx: ContextoBloqueo): boolean {
  const c = correo.trim().toLowerCase();
  if (!c.includes('@') || ctx.enSpam) return false;
  return !ctx.propias.some((p) => p.trim().toLowerCase() === c);
}

/**
 * La regla que bloquea: la dirección EXACTA del remitente (no su dominio: bloquear a
 * una persona no puede llevarse a Spam a toda su empresa) y la carpeta Spam. El nombre
 * es el mismo rótulo de la entrada del menú, para que en «Reglas» se lea qué es y de
 * dónde salió.
 */
export function reglaDeBloqueo(correo: string, spamId: string, nombre: string): MailFilterInput {
  return { name: nombre, enabled: true, matchFrom: correo.trim(), moveTo: spamId };
}

/** Las llamadas que hacen falta (las de mailApi; en las pruebas, simuladas). */
export interface LlamadasBloqueo {
  guardarRegla: (input: MailFilterInput) => Promise<{ filter?: { id: string } }>;
  borrarRegla: (id: string) => Promise<unknown>;
  mover: (ids: string[], carpeta: string) => Promise<unknown>;
}

/**
 * Bloquea: crea la regla y mueve los mensajes del hilo a Spam. Todo o nada: si el
 * movimiento falla se borra la regla recién creada, porque un aviso de error con la
 * regla ya puesta dejaría a la persona creyendo que no ha bloqueado a nadie. Devuelve
 * el id de la regla (lo que «Deshacer» necesita para quitarla).
 */
export async function bloquearRemitente(
  regla: MailFilterInput,
  ids: string[],
  llamadas: LlamadasBloqueo,
): Promise<string> {
  const { filter } = await llamadas.guardarRegla(regla);
  const id = filter?.id;
  if (!id) throw new Error('regla sin id');
  if (ids.length === 0 || !regla.moveTo) return id;
  try {
    await llamadas.mover(ids, regla.moveTo);
  } catch (err) {
    await llamadas.borrarRegla(id).catch(() => undefined);
    throw err;
  }
  return id;
}

/**
 * «Deshacer» del bloqueo, entero: quita la regla y devuelve cada mensaje a SU carpeta
 * (`origenesDeMovimiento`, utils.ts). Se intentan las dos cosas aunque una falle —que
 * se quede la regla sin el hilo, o al revés, es peor que medio deshecho avisado— y
 * devuelve si salieron las dos.
 */
export async function deshacerBloqueo(
  reglaId: string,
  origenes: Map<string, string[]>,
  llamadas: Pick<LlamadasBloqueo, 'borrarRegla' | 'mover'>,
): Promise<boolean> {
  const resultados = await Promise.allSettled([
    llamadas.borrarRegla(reglaId),
    ...Array.from(origenes, ([carpeta, ids]) => llamadas.mover(ids, carpeta)),
  ]);
  return resultados.every((r) => r.status === 'fulfilled');
}
