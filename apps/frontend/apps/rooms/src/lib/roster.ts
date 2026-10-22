/**
 * Lógica pura del roster de canal («Miembros», Ola 2C): reconciliación de la lista
 * ante los frames WS channel_member_added/removed, filtro del buscador local y
 * candidatos de «Añadir personas». Sin DOM ni HTTP → testeable con `bun test`.
 */
import type { ChannelMember } from '../client/chatApi.js';
import { contiene } from '@sytue/i18n';

/** Orden por nombre, como el backend (ORDER BY lower(name)) pero con criterio de
 *  locale para que los acentos del español no rompan la secuencia. */
function byName(a: ChannelMember, b: ChannelMember): number {
  return a.name.localeCompare(b.name, 'es', { sensitivity: 'base' });
}

/** UPSERT por subject conservando el orden: el frame `channel_member_added` cubre
 *  tanto el alta como el CAMBIO DE ROL, así que una fila existente se reemplaza. */
export function upsertMember(list: ChannelMember[], m: ChannelMember): ChannelMember[] {
  return [...list.filter((x) => x.sub !== m.sub), m].sort(byName);
}

/** Filtro del buscador local: por nombre o por subject, sin distinguir mayúsculas. */
export function filterMembers(list: ChannelMember[], query: string): ChannelMember[] {
  const q = query.trim().toLowerCase();
  if (!q) return list;
  return list.filter((m) => contiene(m.name, q) || contiene(m.sub, q));
}

/** Candidatos de «Añadir personas»: los del EQUIPO que aún no están en el canal. El
 *  backend rechaza (403) a quien no pertenezca al team, así que la fuente es la
 *  misma lista que alimenta las menciones.
 *  ponytail: filtro EN CLIENTE sobre el equipo entero, que ya se descarga completo
 *  para el selector de menciones (techo preexistente). Con equipos de decenas de
 *  miles hace falta un endpoint de búsqueda paginado en services/chat. */
export function addableMembers(
  team: ChannelMember[],
  roster: ChannelMember[],
  query: string,
): ChannelMember[] {
  const inChannel = new Set(roster.map((m) => m.sub));
  return filterMembers(
    team.filter((m) => !inChannel.has(m.sub)),
    query,
  ).sort(byName);
}
