/**
 * Fusión de contactos de Sytue Contacts (v1, puro frontend): construye el
 * ContactInput superviviente como unión de emails y teléfonos (trim + dedupe
 * case-insensitive), conservando el uid del superviviente y el
 * nombre/organización elegidos (los del superviviente por defecto).
 *
 * ponytail: la vCard se reconstruye desde ContactInput — la misma proyección
 * que el editar de hoy — así que los campos no proyectados de las tarjetas
 * (PHOTO/BDAY/ADR/X-*…) no se conservan; paridad con el edit actual, mejora
 * futura si hace falta fusión campo a campo sobre la vCard cruda.
 */
import type { Contact } from '@sytue/api-clients';
import type { ContactInput } from './contactsApi.js';

/** Unión de listas con trim + dedupe case-insensitive; conserva el orden y la primera grafía. */
export function unionValues(...lists: string[][]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of lists.flat()) {
    const value = raw.trim();
    const key = value.toLowerCase();
    if (!value || seen.has(key)) continue;
    seen.add(key);
    out.push(value);
  }
  return out;
}

/** Valores elegidos cuando superviviente y perdedor difieren (por defecto, los del superviviente). */
export interface MergeChoices {
  fullName?: string;
  organization?: string;
}

export function buildMergeInput(
  survivor: Contact,
  loser: Contact,
  chosen: MergeChoices = {},
): ContactInput {
  // La organización «hereda» la del perdedor si el superviviente no tiene:
  // fusionar nunca debe perder datos que sí proyectamos.
  const organization = chosen.organization ?? survivor.organization ?? loser.organization;
  return {
    uid: survivor.uid,
    fullName: chosen.fullName ?? survivor.fullName,
    emails: unionValues(survivor.emails, loser.emails),
    phones: unionValues(survivor.phones, loser.phones),
    ...(organization ? { organization } : {}),
  };
}
