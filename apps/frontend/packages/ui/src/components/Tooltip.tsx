/**
 * Tooltip — ayuda contextual breve. Accesible por teclado (focus) y raton
 * (hover). No debe contener informacion esencial unica. docs/design.md §9.
 */
import type { ComponentChildren, JSX, VNode } from 'preact';
import { cloneElement, isValidElement } from 'preact';
import { useEffect, useId, useRef, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';

export interface TooltipProps {
  label: string;
  children: ComponentChildren;
  placement?: 'top' | 'bottom';
}

export function Tooltip({ label, children, placement = 'top' }: TooltipProps): JSX.Element {
  const [open, setOpen] = useState(false);
  const id = useId();
  // Retardo de 300 ms al hover (spec §Tooltip); el foco de teclado abre al instante.
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const show = () => {
    timerRef.current = setTimeout(() => setOpen(true), 300);
  };
  const hide = () => {
    clearTimeout(timerRef.current);
    setOpen(false);
  };
  useEffect(() => () => clearTimeout(timerRef.current), []);

  // Escape cierra la ayuda sin tocar el foco ni la acción (WAI-ARIA APG): un tooltip que
  // tapa lo que hay debajo y no se puede quitar con el teclado es una trampa. El listener
  // solo vive mientras está abierto.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') hide();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);

  // `aria-describedby` va en el CONTROL, no en un <span> que lo envuelva: el lector de
  // pantalla anuncia la descripción del elemento que recibe el foco, y si el atributo
  // cuelga del padre no lee nada. El tooltip se veía y no se anunciaba — que es
  // justamente lo único que un tooltip tiene que hacer por quien no lo ve.
  const descrito =
    isValidElement(children) && open
      ? cloneElement(children as VNode<Record<string, unknown>>, { 'aria-describedby': id })
      : children;

  return (
    <span
      class="relative inline-flex"
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocusCapture={() => setOpen(true)}
      onBlurCapture={hide}
    >
      {descrito}
      {open ? (
        <span
          id={id}
          role="tooltip"
          class={cx(
            'pointer-events-none absolute left-1/2 max-w-[240px] -translate-x-1/2 rounded-md text-center',
            // Superficie invertida via tokens de rol: contraste alto en claro y oscuro.
            'sytue-fade-in sytue-tooltip-surface px-2 py-1 text-xs font-medium shadow-[var(--shadow-md)]',
            placement === 'top' ? 'bottom-[calc(100%+6px)]' : 'top-[calc(100%+6px)]',
          )}
          style={{ zIndex: 'var(--z-tooltip)' }}
        >
          {label}
          <span
            aria-hidden="true"
            class={cx(
              'sytue-tooltip-arrow absolute left-1/2 h-1.5 w-1.5 -translate-x-1/2 rotate-45',
              placement === 'top' ? '-bottom-[3px]' : '-top-[3px]',
            )}
          />
        </span>
      ) : null}
    </span>
  );
}
