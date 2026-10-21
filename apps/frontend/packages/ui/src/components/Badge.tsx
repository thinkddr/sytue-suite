/**
 * Badge / Tag — etiquetas de estado o conteo (no leidos, roles, presencia).
 * Comunica con texto + color (no solo color). docs/design.md §9.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';

export type BadgeTone =
  'neutral' | 'primary' | 'accent' | 'success' | 'warning' | 'danger' | 'info';

export interface BadgeProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
  /** Forma pildora (por defecto) o cuadrada. */
  pill?: boolean;
  /** Variante de conteo: circulo compacto para numeros (no leidos). */
  count?: boolean;
}

/*
 * Tokens SEMÁNTICOS, no escalones crudos. `primary` y `accent` eran los dos
 * únicos tonos que usaban `-50`/`-700` directamente, y son justo los escalones
 * que el bloque oscuro no reasigna: en tema oscuro salían como manchas claras
 * casi blancas con texto marrón encima. Los otros cinco tonos ya iban por
 * `-bg`/`-text`, que conmutan por tema.
 *
 * `accent` usa violeta suave para distinguir las etiquetas de recursos de los
 * tonos neutros y las acciones azules; sus roles también cambian en oscuro.
 */
const tones: Record<BadgeTone, string> = {
  neutral: 'bg-bg-subtle text-text-muted',
  primary: 'bg-selected text-primary-text',
  accent: 'bg-violet-bg text-violet-text',
  success: 'bg-success-bg text-success-text',
  warning: 'bg-warning-bg text-warning-text',
  danger: 'bg-danger-bg text-danger-text',
  info: 'bg-info-bg text-info-text',
};

export function Badge({
  tone = 'neutral',
  pill = true,
  count = false,
  class: cls,
  children,
  ...rest
}: BadgeProps): JSX.Element {
  return (
    <span
      class={cx(
        'inline-flex items-center justify-center align-middle leading-none font-semibold whitespace-nowrap',
        count
          ? cx(
              'rounded-pill h-[18px] min-w-[18px] px-1 text-xs tabular-nums',
              // El relleno es el ROL primario con SU rótulo (`on-primary`), que conmutan
              // juntos: el primario con blanco en claro y el primario claro con
              // rótulo oscuro en oscuro (8,9:1). Fue `primary-text` —un token de TINTA,
              // que en oscuro se aclara y dejaba el blanco de encima en 2,11:1— y después
              // el escalón 600 fijo, que no sigue al tema. El de peligro lleva blanco: su
              // rojo es el mismo en los tres temas (5,1:1).
              tone === 'danger' ? 'bg-danger text-white' : 'bg-primary text-on-primary',
            )
          : cx('px-2 py-1 text-xs', pill ? 'rounded-pill' : 'rounded-sm', tones[tone]),
        cls,
      )}
      {...rest}
    >
      {children}
    </span>
  );
}
