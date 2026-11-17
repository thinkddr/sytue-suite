/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Programar reunión» desde un canal o un chat (D-ROOM-06): el enlace profundo que
 * abre Sytue Calendar con el evento ya relleno. Funciones puras, sin DOM, para
 * probarlas con `bun test`.
 *
 * El enlace es el intent de creación que Calendar ya lee y sanea
 * (`?action=create&title=&description=&attendees=`, `readCreateIntent` en
 * CalendarApp): el mismo que componen el Hub y la ficha de Contactos. No se inventa
 * otro formato: tres apps hablando el mismo idioma es lo que evita que uno se rompa.
 *
 * Los invitados salen de quien está en la conversación y tiene correo en el
 * directorio de la organización (el mismo que pinta la tarjeta de persona). Quien
 * no tiene correo aquí —gente de otra organización, invitaciones sin aceptar— no
 * entra: Calendar invita por correo y uno inventado sería peor que ninguno; quien
 * programa lo añade a mano si quiere.
 */
import type { Channel, ChannelMember } from './chat.js';
import { directorio, type FichaDirectorio } from './tarjetaPersona.js';

/** Calendar recorta a 100 invitados; más allá el enlace solo crecería para nada. */
export const MAX_INVITADOS = 100;

/**
 * Los subs de las OTRAS personas de la conversación: en un canal, su roster sin
 * invitaciones pendientes; en un chat, sus `peerSubs`. Yo nunca: el organizador ya
 * está en el evento por ser quien lo crea.
 */
export function participantesDeReunion(
  canal: Pick<Channel, 'kind' | 'peerSubs'>,
  roster: readonly Pick<ChannelMember, 'sub' | 'pending'>[],
  yo: string,
): string[] {
  const subs =
    canal.kind === 'direct'
      ? (canal.peerSubs ?? [])
      : roster.filter((m) => !m.pending).map((m) => m.sub);
  return Array.from(new Set(subs.filter((s) => s && s !== yo)));
}

/** Los correos de esos subs según el directorio, sin repetir, en minúsculas y acotados. */
export function correosDeInvitados(
  subs: readonly string[],
  correoDe: ReadonlyMap<string, { email: string }>,
): string[] {
  const correos = new Set<string>();
  for (const sub of subs) {
    const email = correoDe.get(sub)?.email.trim().toLowerCase();
    if (email) correos.add(email);
    if (correos.size >= MAX_INVITADOS) break;
  }
  return [...correos];
}

/**
 * La URL de Calendar con el evento relleno. `base` es la raíz de Calendar (con la
 * organización, si la hay); sin invitados no se manda `attendees` vacío.
 */
export function enlaceProgramarReunion(
  base: string,
  evento: { titulo: string; descripcion?: string; invitados?: readonly string[] },
): string {
  const q = new URLSearchParams({ action: 'create', title: evento.titulo });
  if (evento.descripcion) q.set('description', evento.descripcion);
  if (evento.invitados?.length) q.set('attendees', evento.invitados.join(','));
  return `${base.replace(/\/+$/, '')}/?${q}`;
}

/** Lo que `abrirEnCalendar` toca del navegador; inyectable para los tests. */
export interface EntornoReunion {
  /** Abre la pestaña en blanco donde irá Calendar (null si el navegador la bloquea). */
  abrir: () => Pick<Window, 'location' | 'opener'> | null;
  /** Sin pestaña nueva, se va en esta. */
  irAqui: (url: string) => void;
  fichas: () => Promise<ReadonlyMap<string, FichaDirectorio>>;
  /** Cuánto se espera al directorio antes de abrir Calendar sin invitados. */
  esperaMs?: number;
}

/**
 * El directorio es un `fetch` sin plazo: con la red colgada la pestaña nueva se
 * quedaría en blanco para siempre. Pasado este rato, Calendar se abre sin invitados.
 */
export const ESPERA_DIRECTORIO_MS = 4000;

const navegador: EntornoReunion = {
  abrir: () => window.open('', '_blank'),
  irAqui: (url) => window.location.assign(url),
  fichas: directorio,
};

/**
 * Abre Calendar con el evento relleno e invita a `subs`. La pestaña se abre ANTES de
 * esperar al directorio: un `window.open` tras un `await` ya no cuenta como gesto del
 * usuario y el navegador lo bloquea. Se le quita `opener` porque Calendar no tiene
 * nada que hacer con esta pestaña. Si el directorio falla o tarda demasiado, Calendar
 * se abre igual, sin invitados: quien programa los añade allí.
 */
export async function abrirEnCalendar(
  base: string,
  evento: { titulo: string; descripcion?: string },
  subs: readonly string[],
  entorno: EntornoReunion = navegador,
): Promise<void> {
  const ventana = entorno.abrir();
  if (ventana) ventana.opener = null;
  let invitados: string[] = [];
  if (subs.length > 0) {
    try {
      let plazo: ReturnType<typeof setTimeout> | undefined;
      const fichas = await Promise.race([
        entorno.fichas(),
        new Promise<never>((_, rechazar) => {
          plazo = setTimeout(
            () => rechazar(new Error('directorio: sin respuesta')),
            entorno.esperaMs ?? ESPERA_DIRECTORIO_MS,
          );
        }),
      ]).finally(() => clearTimeout(plazo));
      invitados = correosDeInvitados(subs, fichas);
    } catch {
      invitados = [];
    }
  }
  const url = enlaceProgramarReunion(base, { ...evento, invitados });
  if (ventana) ventana.location.href = url;
  else entorno.irAqui(url);
}
