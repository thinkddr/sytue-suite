/**
 * AppGrid / Launcher — grid de 9+ puntos con iconos de producto (app switcher).
 * role=menu, navegacion con flechas, Esc cierra; el nombre visible ES el accesible.
 * Consume el catalogo de apps de la suite. docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { ProductIcon } from './ProductIcon.js';
import { suiteApps, type SuiteApp } from '../lib/apps.js';

export interface AppGridProps {
  /** Apps a mostrar (por defecto el catalogo completo de la suite). */
  apps?: SuiteApp[];
  /** Clave de la app actual (resaltada). */
  current?: string;
  /** Cierre del popover (al pulsar Esc o navegar). */
  onClose?: () => void;
  class?: string;
}

export function AppGrid({
  apps = suiteApps,
  current,
  onClose,
  class: cls,
}: AppGridProps): JSX.Element {
  const gridRef = useRef<HTMLDivElement>(null);
  const t = useT();

  useEffect(() => {
    const grid = gridRef.current;
    grid?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose?.();
        return;
      }
      const items = Array.from(grid?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
      if (items.length === 0) return;
      const idx = items.indexOf(document.activeElement as HTMLElement);
      const cols = 3;
      const move: Record<string, number> = {
        ArrowRight: 1,
        ArrowLeft: -1,
        ArrowDown: cols,
        ArrowUp: -cols,
      };
      const delta = move[e.key];
      if (delta !== undefined) {
        e.preventDefault();
        const next = (idx + delta + items.length) % items.length;
        items[next]?.focus();
      }
    };
    grid?.addEventListener('keydown', onKeyDown);
    return () => grid?.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  return (
    <div
      ref={gridRef}
      role="menu"
      aria-label={t('shell.apps')}
      class={cx(
        // Popover = elevacion nivel 2 (dropdown): borde + shadow-md, radio lg.
        'sytue-menu-surface border-border grid grid-cols-3 gap-1 rounded-lg border p-2 shadow-[var(--shadow-md)]',
        cls,
      )}
      style={{ width: '288px' }}
    >
      {apps.map((app) => {
        const active = app.key === current;
        const disabled = app.disabled;
        // El nombre es marca (no se traduce); la descripción sí, desde el catálogo.
        // Va en `title` y NO en `aria-label`: el nombre accesible tiene que contener
        // el texto visible («Mail»), y «Sytue Mail — Correo electrónico» no lo
        // contenía tal cual (WCAG 2.5.3; Lighthouse lo marcaba en las once apps).
        const baseDesc = app.description ?? t(`app.${app.key}.description`);
        const caption = disabled
          ? `${app.name} — ${baseDesc} (${t('app.disabled')})`
          : `${app.name} — ${baseDesc}`;
        return (
          <a
            key={app.key}
            role="menuitem"
            href={disabled ? undefined : app.href}
            aria-disabled={disabled ? 'true' : undefined}
            aria-current={active ? 'page' : undefined}
            title={caption}
            onClick={disabled ? (e) => e.preventDefault() : undefined}
            class={cx(
              'flex flex-col items-center gap-1.5 rounded-md p-3 text-center',
              'transition-colors duration-150 focus-visible:outline-none',
              disabled
                ? 'cursor-not-allowed opacity-40 grayscale'
                : active
                  ? 'bg-selected'
                  : 'hover:bg-bg-subtle focus-visible:bg-bg-subtle active:bg-border',
            )}
          >
            <ProductIcon appKey={app.key} size={40} />
            <span
              class={cx(
                'w-full truncate text-xs',
                disabled
                  ? 'text-text-muted font-normal'
                  : active
                    ? 'text-text font-semibold'
                    : 'text-text font-medium',
              )}
            >
              {app.name.replace('Sytue ', '')}
            </span>
          </a>
        );
      })}
    </div>
  );
}
