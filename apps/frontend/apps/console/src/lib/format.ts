/**
 * Utilidades de formato de Sytue Admin. Las FECHAS ya no se formatean con un
 * `Intl.DateTimeFormat('es-ES')` fijo: las pinta `formatDateTime` de
 * `@sytue/i18n` con el locale de la petición, que es lo único que hace que un
 * administrador alemán vea 28.07.2026 y un inglés 28 Jul 2026
 * (docs/CONVENTIONS.md §13.1).
 *
 * Lo que sí vive aquí son los mapas «valor de dominio → clave de catálogo» y los
 * tonos de badge, que no dependen del idioma.
 */
import type { BadgeTone } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { ConsoleMessageKey } from '../i18n/index.js';
import type { ProvisionState, SytueRole } from './types.js';

/** Fecha y hora de una tabla, o «—» si el campo viene vacío. */
export function formatWhen(locale: Locale, iso?: string): string {
  return iso ? formatDateTime(locale, iso) : '—';
}

export const roleLabelKey: Record<SytueRole, ConsoleMessageKey> = {
  owner: 'role.owner',
  admin: 'role.admin',
  member: 'role.member',
  billing: 'role.billing',
};

// «info» y no «primary» para owner: en claro rinden idéntico (mismo par de
// tokens) y «info» sí se reasigna en modo oscuro (contraste AA). «neutral» y no
// «accent» para billing: accent-50 no se reasigna en oscuro (chip deslumbrante)
// y en claro es invisible sobre la superficie; el texto ya distingue el rol.
export const roleTone: Record<SytueRole, BadgeTone> = {
  owner: 'info',
  admin: 'info',
  member: 'neutral',
  billing: 'neutral',
};

export const provisionTone: Record<ProvisionState, BadgeTone> = {
  synced: 'success',
  pending: 'warning',
  error: 'danger',
};

export const provisionLabelKey: Record<ProvisionState, ConsoleMessageKey> = {
  synced: 'provision.synced',
  pending: 'provision.pending',
  error: 'provision.error',
};
