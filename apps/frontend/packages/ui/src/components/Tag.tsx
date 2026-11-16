/**
 * Tag — la etiqueta (chip) de un recurso: el grupo de un contacto, la etiqueta de un
 * correo, un invitado de un evento. Texto, punto de color opcional y, si se puede
 * quitar, el aspa que la quita. docs/design.md §3.16.
 *
 * No es `Badge`: la insignia es estado o conteo (`text-xs`, sin borde, sin acciones) y la
 * etiqueta es un DATO del recurso que a veces se quita. Hasta ahora cada app se escribía
 * la suya (EventModal, ContactDetail, MessageReader) con la misma píldora, y dos de las
 * tres se quedaron con un aspa de 20 px —por debajo de los 24 de WCAG 2.5.8— y sin suelo
 * táctil: copiar las clases del botón no copia su diana.
 *
 * El aspa es `IconButton sm`, que trae la diana de 24 px en px (`min-h-[24px]`), los 44
 * con puntero grueso y el anillo de foco canónico. El `-my-1` le devuelve al chip su
 * alto de siempre (`py-0.5 text-sm`): la diana sobresale por arriba y por abajo en vez
 * de engordar la fila de etiquetas.
 *
 * La forma es la píldora de las tres copias y no la esquina `md` que dibuja la spec: así
 * adoptar el componente no cambia el aspecto de ninguna pantalla.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';
import { IconButton } from './IconButton.js';

/**
 * - `default`: el texto a tinta plena, para etiquetas que son el contenido (grupos,
 *   invitados).
 * - `muted`: el texto apagado, para etiquetas que acompañan a otro contenido (las de un
 *   hilo de correo, debajo del asunto).
 */
export type TagTone = 'default' | 'muted';

/** `sm` es el chip corriente; `xs`, el de las filas densas (las etiquetas de un hilo). */
export type TagSize = 'sm' | 'xs';

interface TagBase extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, 'size' | 'children'> {
  /** El texto de la etiqueta. Se trunca en `maxWidth` y su entero va al `title`. */
  children: ComponentChildren;
  /**
   * Color del punto de la izquierda, como valor CSS (`var(--sytue-…)` o el que guarde el
   * recurso). Sin él no hay punto. Es decorativo: el significado lo lleva el texto.
   */
  dot?: string;
  tone?: TagTone;
  size?: TagSize;
  /**
   * Tope del texto, como clase de ancho máximo. Por defecto `max-w-48` (192 px con la
   * raíz de 16): un nombre largo no debe empujar a los demás chips a otra línea.
   */
  maxWidth?: string;
  /** El texto completo para el `title` cuando `children` no es una cadena. */
  title?: string;
  /**
   * Lo que acompaña al texto sin ser parte de él, entre el texto y el aspa: la `Badge`
   * con la respuesta de un invitado. Va fuera del tramo que se trunca, de modo que un
   * correo largo se recorta él y no se come la respuesta.
   */
  trailing?: ComponentChildren;
}

/**
 * Con `onRemove` el `removeLabel` es obligatorio: el aspa es un botón de solo icono, y un
 * botón sin nombre accesible es invisible para un lector de pantalla. Debe nombrar la
 * etiqueta («Quitar la etiqueta Trabajo»), no solo la acción: con cinco aspas en fila,
 * cinco «Quitar» iguales no dicen cuál es cuál.
 */
export type TagProps = TagBase &
  (
    | { onRemove?: undefined; removeLabel?: undefined; removeDisabled?: undefined }
    | {
        onRemove: () => void;
        removeLabel: string;
        /** Apaga el aspa (p. ej. mientras se guarda el cambio anterior). */
        removeDisabled?: boolean;
      }
  );

const TONE: Record<TagTone, string> = {
  default: 'text-text',
  muted: 'text-text-muted',
};

const SIZE: Record<TagSize, string> = {
  sm: 'gap-1 text-sm',
  xs: 'gap-1.5 text-xs',
};

export function Tag({
  children,
  dot,
  tone = 'default',
  size = 'sm',
  maxWidth = 'max-w-48',
  title,
  trailing,
  onRemove,
  removeLabel,
  removeDisabled,
  class: className,
  ...rest
}: TagProps): JSX.Element {
  const removable = onRemove !== undefined;
  const titulo = title ?? (typeof children === 'string' ? children : undefined);
  return (
    <span
      class={cx(
        'border-border bg-bg-subtle rounded-pill inline-flex max-w-full items-center border py-0.5',
        // Con aspa el lado derecho se estrecha: el propio botón ya trae su aire alrededor
        // del glifo, y con `pl-2.5` a los dos lados el aspa quedaría despegada del borde.
        removable ? 'ps-2.5 pe-1' : 'px-2.5',
        SIZE[size],
        TONE[tone],
        className,
      )}
      {...rest}
    >
      {dot !== undefined ? (
        <span
          class="h-2 w-2 shrink-0 rounded-full"
          style={{ background: dot }}
          aria-hidden="true"
        />
      ) : null}
      <span class={cx('min-w-0 truncate', maxWidth)} title={titulo}>
        {children}
      </span>
      {trailing !== undefined && trailing !== null ? (
        <span class="inline-flex shrink-0">{trailing}</span>
      ) : null}
      {removable ? (
        <IconButton
          size="sm"
          tone="subtle"
          label={removeLabel}
          disabled={removeDisabled}
          class="-my-1"
          onClick={onRemove}
        >
          <Icon name="close" size={12} />
        </IconButton>
      ) : null}
    </span>
  );
}
