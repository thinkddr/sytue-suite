/**
 * ReviewSummary — «esto es lo que vas a aplicar»: lista clave→valor del último
 * paso de un asistente (o de cualquier confirmación). Cada fila puede llevar su
 * propio «Editar», que devuelve al paso donde se rellenó ese dato.
 *
 *   <ReviewSummary items={[
 *     { label: 'Aplicación', value: 'Slack' },
 *     { label: 'Workspace', value: 'acme', mono: true, onEdit: () => setStep(2) },
 *     { label: 'Token SCIM', value: 'Guardado (no se muestra)' },
 *   ]} />
 */
import type { ComponentChildren, JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';

export interface ReviewItem {
  label: string;
  value: ComponentChildren;
  /** Valor técnico (URL, identificador): tipografía monoespaciada. */
  mono?: boolean;
  /** Volver a donde se rellenó este dato. */
  onEdit?: () => void;
}

export interface ReviewSummaryProps {
  items: ReviewItem[];
  /** Etiqueta del botón de edición. Por defecto la del catálogo común (`ui.review.edit`). */
  editLabel?: string;
  class?: string;
}

export function ReviewSummary({ items, editLabel, class: cls }: ReviewSummaryProps): JSX.Element {
  // Del catálogo y no «Editar» a mano: escrito en español salía así en los 26 idiomas.
  const t = useT();
  const editar = editLabel ?? t('ui.review.edit');
  return (
    <dl class={cx('border-border bg-surface divide-border-subtle divide-y rounded-md border', cls)}>
      {items.map((item) => (
        <div
          key={item.label}
          class="flex items-baseline gap-3 px-3 py-2.5 sm:grid sm:grid-cols-[minmax(0,11rem)_1fr_auto]"
        >
          <dt class="text-text-muted shrink-0 text-sm">{item.label}</dt>
          <dd
            class={cx(
              'text-text min-w-0 flex-1 text-sm break-words',
              item.mono ? 'font-mono text-xs' : '',
            )}
          >
            {item.value}
          </dd>
          {item.onEdit ? (
            <dd class="shrink-0">
              <button
                type="button"
                onClick={item.onEdit}
                class={cx(
                  'text-primary-text rounded text-sm font-semibold hover:underline',
                  // Clase canónica: escrito a mano se perdía el contorno neutro, y
                  // el latón solo mide 2,81:1 sobre marfil (WCAG 1.4.11 pide 3:1).
                  'sytue-focus-ring-inset',
                )}
              >
                {editar}
                <span class="sr-only"> {item.label}</span>
              </button>
            </dd>
          ) : null}
        </div>
      ))}
    </dl>
  );
}
