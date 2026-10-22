/**
 * ReadOnlyBar — la franja de solo lectura, con la acción que la desbloquea.
 *
 * La franja existe porque sin ella quien abre algo que no puede editar pulsa negrita,
 * no pasa nada y concluye que la aplicación está rota. Y lleva un botón porque una
 * franja que solo informa deja a la persona sin salida (petición del dueño, 2026-09-25):
 *
 * - (a) PUEDE editar —el modo lectura lo eligió ella o es un bloqueo suyo—: «Editar»,
 *   que desbloquea al momento.
 * - (b) NO tiene permiso: «Pedir acceso de edición». Enviada la petición, el botón se
 *   cambia por `note` («Petición enviada»), y la franja se quita sola cuando el dueño
 *   concede (cada app escucha su canal en vivo y deja de pintarla).
 *
 * Es la MISMA pieza en todas las apps, y su espejo nativo es `ui.FranjaSoloLectura`
 * de `apps/fyne/ui`. Qué botón lleva lo decide la app, que es quien sabe de dónde sale
 * su permiso: aquí solo se pinta.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';

export interface ReadOnlyBarAction {
  label: string;
  onClick: () => void;
  /** Mientras se envía: el botón gira y no se puede volver a pulsar. */
  loading?: boolean;
}

export interface ReadOnlyBarProps {
  /** Por qué es de solo lectura, ya traducido. */
  message: string;
  /** El botón que la desbloquea. Sin él, la franja solo informa. */
  action?: ReadOnlyBarAction;
  /** Una segunda salida, discreta: «Rechazar» en la franja del dueño que recibe una petición. */
  secondaryAction?: ReadOnlyBarAction;
  /** Lo que sustituye al botón cuando ya no hay nada que pulsar («Petición enviada»). */
  note?: string;
  class?: string;
}

export function ReadOnlyBar({
  message,
  action,
  secondaryAction,
  note,
  class: cls,
}: ReadOnlyBarProps): JSX.Element {
  return (
    <div
      role="status"
      class={cx(
        'border-border bg-bg-subtle text-text-muted flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-1 text-sm',
        cls,
      )}
    >
      <Icon name="lock" size={14} />
      <span class="min-w-0 flex-1">{message}</span>
      {note ? (
        <span class="text-text font-medium">{note}</span>
      ) : (
        <>
          {secondaryAction ? (
            <Button
              size="xs"
              variant="ghost"
              loading={secondaryAction.loading ?? false}
              onClick={secondaryAction.onClick}
            >
              {secondaryAction.label}
            </Button>
          ) : null}
          {action ? (
            <Button
              size="xs"
              variant="secondary"
              loading={action.loading ?? false}
              onClick={action.onClick}
            >
              {action.label}
            </Button>
          ) : null}
        </>
      )}
    </div>
  );
}
