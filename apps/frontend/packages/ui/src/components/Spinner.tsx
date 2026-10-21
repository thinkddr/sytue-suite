/** Spinner — indicador de carga accesible (aria-busy en el contenedor). */
import type { JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';

export interface SpinnerProps {
  size?: number;
  class?: string;
  label?: string;
}

export function Spinner({ size = 18, class: cls, label }: SpinnerProps): JSX.Element {
  const t = useT();
  return (
    <svg
      class={cx('animate-spin [animation-duration:700ms]', cls)}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      role="status"
      aria-label={label ?? t('ui.loading')}
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2" opacity="0.25" />
      <path d="M12 3a9 9 0 1 1-9 9" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
    </svg>
  );
}
