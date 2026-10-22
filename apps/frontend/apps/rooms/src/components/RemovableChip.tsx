/**
 * RemovableChip — píldora extraíble: etiqueta + «×», donde todo el chip es un botón
 * que la quita. `leading` (p. ej. un avatar) va antes de la etiqueta; sin él, el texto
 * lleva algo más de sangría izquierda. Compartida por NewDmModal (personas elegidas)
 * y NotifySettingsModal (palabras clave).
 */
import type { JSX } from 'preact';
import { cx } from '@sytue/ui';

export function RemovableChip({
  label,
  ariaLabel,
  onRemove,
  leading,
}: {
  label: string;
  /** Nombre accesible del botón (p. ej. «Quitar a Ana»). */
  ariaLabel: string;
  onRemove: () => void;
  /** Elemento antes de la etiqueta (un avatar…). */
  leading?: JSX.Element;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onRemove}
      aria-label={ariaLabel}
      class={cx(
        'rounded-control border-border bg-bg-subtle text-text hover:bg-selected sytue-focus-ring inline-flex items-center gap-1.5 border py-1 pr-2 text-sm font-medium transition-colors duration-150',
        leading ? 'pl-1.5' : 'pl-2.5',
      )}
    >
      {leading}
      <span class="max-w-[10rem] truncate">{label}</span>
      <span aria-hidden="true" class="text-text-muted">
        ×
      </span>
    </button>
  );
}
