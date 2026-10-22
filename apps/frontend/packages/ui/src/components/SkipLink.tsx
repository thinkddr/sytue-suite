/**
 * SkipLink — enlace «Saltar al contenido» para teclado/lectores de pantalla.
 * Patrón WCAG 2.4.1; visible solo al recibir foco. docs/design.md §3.
 */
import type { JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';

export interface SkipLinkProps {
  /** id del landmark destino (por defecto `contenido`). */
  targetId?: string;
  /** Texto del enlace; por defecto, el del catálogo en el idioma activo. */
  label?: string;
  class?: string;
}

export function SkipLink({
  targetId = 'contenido',
  label,
  class: cls,
}: SkipLinkProps): JSX.Element {
  const t = useT();
  return (
    <a
      href={`#${targetId}`}
      class={cx(
        'sr-only focus:not-sr-only focus:absolute focus:top-4 focus:left-4 focus:z-50',
        'focus:bg-primary focus:rounded-md focus:px-4 focus:py-2',
        // `-inset` y no el aro normal: este enlace aparece CON relleno primario, y
        // un aro azul encima de un relleno azul no se ve. La variante abre antes
        // un hueco del color de la superficie.
        'sytue-focus-ring-inset focus:text-[var(--sytue-on-primary)]',
        cls,
      )}
    >
      {label ?? t('shell.skipToContent')}
    </a>
  );
}
