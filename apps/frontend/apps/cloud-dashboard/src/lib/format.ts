/**
 * Utilidades de formato de Sytue DevX, POR LOCALE.
 *
 * Antes este módulo estaba clavado en `es-ES`: un desarrollador alemán veía su
 * factura con separadores españoles. Ahora todo delega en los helpers de
 * `@sytue/i18n`, que son los que conocen la etiqueta BCP-47 de cada idioma
 * (docs/CONVENTIONS.md §13.1). El locale entra siempre como primer argumento:
 * lo resuelve el middleware una vez por petición y baja por props a las islas.
 */
import {
  formatCurrency,
  formatDate as formatDateI18n,
  formatNumber as formatNumberI18n,
  formatTimeAgo,
  type Locale,
} from '@sytue/i18n';
import type { BadgeTone } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { DeployStatus } from './types.js';

/** Fecha corta con hora: «28 Jul, 14:03» / «28 jul, 14:03» / «28.07., 14:03». */
export function formatDate(locale: Locale, iso: string): string {
  return formatDateI18n(locale, iso, {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** Fecha SIN hora — períodos de facturación: «5 Jan 2026» / «5 ene 2026». */
export function formatDay(locale: Locale, iso: string): string {
  return formatDateI18n(locale, iso, { day: 'numeric', month: 'short', year: 'numeric' });
}

/** Número localizado con hasta 1 decimal — cantidades de uso. */
export function formatNumber(locale: Locale, n: number): string {
  return formatNumberI18n(locale, n, { maximumFractionDigits: 1 });
}

/** Importe en euros: «€1.36» en inglés europeo, «1,36 €» en es/fr/it, «1,36 €» en de. */
export function formatEur(locale: Locale, n: number): string {
  return formatCurrency(locale, n);
}

/** Antigüedad legible («5 min ago», «hace 5 min», «vor 5 Min.»). */
export function relativeTime(locale: Locale, iso: string, now = Date.now()): string {
  return formatTimeAgo(locale, iso, now);
}

/** Etiqueta traducida del estado de un despliegue. */
export function deployStatusLabel(t: T, status: DeployStatus): string {
  switch (status) {
    case 'building':
      return t('deploy.status.building');
    case 'ready':
      return t('deploy.status.ready');
    case 'error':
      return t('deploy.status.error');
    case 'queued':
      return t('deploy.status.queued');
    case 'canceled':
      return t('deploy.status.canceled');
  }
}

export const deployStatusTone: Record<DeployStatus, BadgeTone> = {
  building: 'info',
  ready: 'success',
  error: 'danger',
  queued: 'neutral',
  canceled: 'warning',
};

/**
 * Convierte un valor de celda de la base de datos (tipo desconocido) en texto
 * seguro para mostrar. Los objetos/arrays se serializan como JSON en lugar de
 * `[object Object]`; el resto usa su representacion primitiva.
 */
export function cellToString(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      // Referencia circular: marcador técnico, no texto de interfaz.
      return '[object]';
    }
  }
  // Primitivos (string, number, boolean, bigint, symbol): stringify seguro.
  // eslint-disable-next-line @typescript-eslint/no-base-to-string
  return String(value);
}
