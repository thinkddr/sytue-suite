/**
 * StorageMeter — indicador de cuota de almacenamiento en el PIE del sidebar.
 * Barra sobria (hairline) «X de Y usados» con relleno proporcional. Lee
 * GET /api/quota (proxy BFF → servicio Go): el uso es el contador agregado que el
 * store mantiene (sin N+1) y es owner-scoped. El color del relleno escala
 * neutro → ámbar → rojo al acercarse al límite, pero NUNCA es el único
 * indicador: el texto («casi lleno» / «lleno») y el aria del progressbar lo
 * acompañan (WCAG AA). Estados de carga y error discretos: es un indicador
 * best-effort, no bloquea el correo.
 *
 * Tokens --sytue-* del cromo (sin gradientes ni emojis), sobre el fondo del menú
 * lateral (text-sidebar-fg / -muted). docs/design.md §1.0.1
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { formatPercent, type Locale } from '@sytue/i18n';
import { getStorageQuota, type StorageQuota } from '../client/mailApi.js';
import { useT } from '../i18n/index.js';
import { formatBytes } from './utils.js';

/** Umbrales de escalado (fracción usada): ámbar de aviso, rojo crítico. */
const WARN = 0.8;
const FULL = 0.95;

/** refreshKey: al cambiar, recarga la cuota (p. ej. tras enviar/guardar). */
export function StorageMeter({
  locale,
  refreshKey,
}: {
  /** Idioma ya negociado; baja por props desde la isla raíz. */
  locale: Locale;
  refreshKey?: number;
}): JSX.Element | null {
  const t = useT(locale);
  const [quota, setQuota] = useState<StorageQuota | null>(null);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(false);
    void getStorageQuota()
      .then((q) => active && setQuota(q))
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [refreshKey]);

  // Primera carga: aviso discreto (no un spinner ruidoso en el pie).
  if (loading && !quota) {
    return <p class="text-sidebar-fg-muted px-1 text-xs">{t('storage.calculating')}</p>;
  }
  // Error o cuota no informable (0 = sin límite configurado): no molestar.
  if (error) {
    return <p class="text-sidebar-fg-muted px-1 text-xs">{t('storage.unavailable')}</p>;
  }
  if (!quota || quota.quotaBytes <= 0) return null;

  const frac = Math.min(quota.usedBytes / quota.quotaBytes, 1);
  const pct = Math.min(Math.round(frac * 100), 100);
  const level: 'ok' | 'warn' | 'full' = frac >= FULL ? 'full' : frac >= WARN ? 'warn' : 'ok';
  // Relleno: neutro (holgado) → ámbar (aviso) → rojo (crítico). El neutro y el ámbar
  // son los del CROMO y no los de página: el menú lateral es blanco, de tinta u
  // oscuro según registro y tema, y cada uno declara los suyos.
  const fill =
    level === 'full'
      ? 'var(--sytue-danger)'
      : level === 'warn'
        ? 'var(--sytue-on-chrome-warning)'
        : 'var(--sytue-on-chrome-muted)';
  // El TEXTO también señala el estado (no solo el color): así el indicador es
  // legible sin percibir el color (WCAG 1.4.1).
  const note =
    level === 'full' ? t('storage.full') : level === 'warn' ? t('storage.almostFull') : '';
  const used = formatBytes(locale, quota.usedBytes);
  const total = formatBytes(locale, quota.quotaBytes);
  // El signo «%» y su separación los pone el locale («95%» / «95 %»).
  const pctText = formatPercent(locale, frac, { maximumFractionDigits: 0 });

  return (
    <div class="flex flex-col gap-1.5 px-1">
      <div class="flex items-baseline justify-between gap-2">
        {/* La etiqueta alemana («Speicher») cabe, pero se trunca por si acaso. */}
        <span class="text-sidebar-fg-muted min-w-0 truncate text-xs">{t('storage.title')}</span>
        <span class="text-sidebar-fg-muted shrink-0 text-xs tabular-nums">{pctText}</span>
      </div>
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-label={t('storage.meter', { used, total, percent: pct, note })}
        // Canal del medidor con el token del velo blanco sobre cromo
        // (`--sytue-chrome-hover`), no con un rgba escrito a mano: es el mismo
        // recurso con el que el rail pinta sus estados, y así el día que el cromo
        // cambie de opacidad no se queda esta barra descolgada.
        class="bg-chrome-hover h-1.5 w-full overflow-hidden rounded-full"
      >
        <div
          class="h-full rounded-full transition-[width] duration-300 ease-[var(--ease-standard)]"
          style={{ width: `${pct}%`, background: fill }}
        />
      </div>
      <p class="text-sidebar-fg text-xs">
        {used} <span class="text-sidebar-fg-muted">{t('storage.of', { total })}</span>
        {note ? <span class="text-sidebar-fg-muted">{note}</span> : null}
      </p>
    </div>
  );
}
