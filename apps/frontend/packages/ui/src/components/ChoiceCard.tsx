/**
 * ChoiceCard — tarjeta-opción de un grupo de radios: título, la CONSECUENCIA de
 * elegirla en lenguaje llano y, si hace falta, una insignia. Es la forma canónica
 * de «elige una de estas» en la configuración guiada: el admin lee lo que pasa, no
 * adivina lo que significa un identificador.
 *
 *   <ChoiceCard name="ambito" title="Solo el correo" selected={s === 'mail'}
 *     description="Se preserva el buzón. Los ficheros de Drive se siguen borrando."
 *     onSelect={() => setS('mail')} />
 *
 * Es un `<label>` con un `<input type="radio">` nativo dentro: el grupo se navega
 * con las flechas del teclado y el foco se ve gracias a `.sytue-focus-ring-inset-within`.
 * Seleccionada, el icono de la izquierda pasa a ser un check (si hay `media`, el
 * check acompaña al título para no tapar el logo).
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';
import type { IconName } from './Icon.js';

export interface ChoiceCardProps {
  /** Grupo del radio nativo: mismo `name` = una sola opción activa. */
  name: string;
  /** `value` del radio nativo. Por defecto, el título. */
  value?: string;
  title: string;
  /** Qué implica elegir esta opción, en una o dos frases. */
  description?: ComponentChildren;
  /** Línea menor bajo la descripción (ejemplos, categoría). */
  footnote?: string;
  /** Insignia junto al título (`<Badge>` o texto corto). */
  badge?: ComponentChildren;
  selected: boolean;
  /** No elegible (p. ej. ya está en uso): se atenúa y el radio queda inerte. */
  disabled?: boolean;
  onSelect: () => void;
  /** Icono de la izquierda cuando NO está seleccionada. Por defecto, un chevron. */
  icon?: IconName;
  /** Sustituye al icono (logo, avatar): para catálogos con marca propia. */
  media?: ComponentChildren;
  /** Menos aire: rejillas con muchas opciones. */
  dense?: boolean;
  class?: string;
}

export function ChoiceCard({
  name,
  value,
  title,
  description,
  footnote,
  badge,
  selected,
  disabled = false,
  onSelect,
  icon = 'chevronRight',
  media,
  dense = false,
  class: cls,
}: ChoiceCardProps): JSX.Element {
  return (
    <label
      class={cx(
        'flex gap-3 rounded-lg border transition-colors duration-150',
        // `-within`: el foco lo recibe el radio oculto de dentro y el aro lo pinta
        // la TARJETA, con el hueco de `-inset` (1px de superficie + 2px de aro).
        'sytue-focus-ring-inset-within',
        dense ? 'p-3' : 'p-4',
        disabled
          ? 'border-border bg-bg-subtle cursor-not-allowed opacity-60'
          : selected
            ? 'border-primary bg-surface-raised cursor-pointer'
            : 'border-border bg-surface-raised hover:border-border-strong cursor-pointer',
        cls,
      )}
    >
      <input
        type="radio"
        name={name}
        value={value ?? title}
        class="sr-only"
        checked={selected}
        disabled={disabled}
        onChange={onSelect}
      />
      {media ?? (
        <Icon
          name={selected ? 'check' : icon}
          size={18}
          class={cx('mt-0.5 shrink-0', selected ? 'text-primary-text' : 'text-text-subtle')}
        />
      )}
      <span class="min-w-0">
        <span class="flex flex-wrap items-center gap-2">
          {/* `text-base` explícito: sin clase de tamaño, el título heredaba y medía
              DISTINTO según dónde se montara la tarjeta —16px en el asistente del
              portal y 14 dentro de un Modal—, porque ningún `html`/`body` declara
              tamaño base y el navegador pone 16 mientras la escala del sistema
              arranca en 14. Con la descripción a `text-sm`, este es el escalón que
              mantiene la jerarquía en los dos sitios. */}
          <span class="text-text text-base font-semibold">{title}</span>
          {badge}
          {media && selected ? <Icon name="check" size={16} class="text-primary-text" /> : null}
        </span>
        {description ? (
          <span class="text-text-muted mt-0.5 block text-sm leading-snug">{description}</span>
        ) : null}
        {footnote ? <span class="sytue-meta mt-1 block">{footnote}</span> : null}
      </span>
    </label>
  );
}
