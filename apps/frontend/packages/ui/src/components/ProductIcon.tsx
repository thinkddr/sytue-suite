/**
 * ProductIcon — icono de un producto de la suite, en el TAMAÑO ÓPTICO correcto.
 *
 * Existe para que la elección de variante no dependa de acordarse. Los iconos
 * tienen dos masters: el normal (trazo 1.7) y el `-sm` (trazo 2.1, con el detalle
 * podado). Por debajo de 24 px el trazo fino no llega a blanco —0,68 px a 16 px—
 * y el glifo cae por debajo del 3:1 que WCAG 1.4.11 exige a un objeto gráfico.
 *
 * Cuando esto se cableaba a mano, se cableó en UNA superficie de cuatro y el
 * incumplimiento siguió vivo en las otras tres. Con este componente, pedir un
 * icono pequeño ya devuelve el master pequeño.
 */
import type { JSX } from 'preact';
import type { ProductKey } from '@sytue/config';
import { productIcons, productIconsSm } from '../assets/icons.js';
import { productMarks } from '../assets/marks.js';

/**
 * Por debajo de 24 px se usa el master -sm. Medido: a 22 px el trazo fino de
 * `one` da 2,60:1 contra su badge, por debajo del 3:1 de WCAG 1.4.11 — el hueco
 * 21-23 px no es seguro, así que el corte va en 24, no en 20.
 */
export const PRODUCT_ICON_SM_BELOW = 24;

export interface ProductIconProps extends Omit<JSX.HTMLAttributes<HTMLImageElement>, 'src'> {
  /** Producto cuyo icono se pinta. */
  appKey: ProductKey;
  /** Lado en píxeles. Determina qué master se usa. */
  size: number;
  /** Texto alternativo. Vacío (por defecto) = decorativo, junto a su etiqueta. */
  alt?: string;
}

export function ProductIcon({ appKey, size, alt = '', ...rest }: ProductIconProps): JSX.Element {
  // LA MARCA MANDA, y el icono de línea es solo la reserva.
  //
  // Son DOS familias y esto usaba la vieja: `branding/icons/` no se toca desde que
  // nació Sytue Notepads, y las marcas se rehicieron enteras esa misma noche. El
  // favicon, el dock y la pantalla de inicio ya salían de la familia nueva, así
  // que el mismo producto se veía de dos maneras según dónde se mirara —el
  // conmutador decía una cosa y la pestaña otra—.
  //
  // Con marca NO hay master pequeño, y no hace falta: el problema que motivó el
  // `-sm` es el trazo fino, que a 16 px no llega a blanco y cae por debajo del 3:1
  // de WCAG 1.4.11. El glifo de las marcas es SÓLIDO —`gen-marks.sh` lo razona— y
  // están diseñadas para leerse justo a ese tamaño, que es donde vive un favicon.
  //
  // La reserva es para `chat` y `meet`, que no tienen marca por ser hosts legados
  // que solo redirigen. Ahí sí sigue valiendo el corte por tamaño óptico.
  const marca = productMarks[appKey];
  const src =
    marca ?? (size < PRODUCT_ICON_SM_BELOW ? productIconsSm[appKey] : productIcons[appKey]);
  // El badge no llena el lienzo: va de 4 a 36 sobre 40 (80 % centrado) con radio
  // 9 (22,5 %). El anillo replica esa geometría para ceñirse al borde de la
  // tesela, no al del <img>.
  const inset = size * 0.1;
  const radio = size * 0.225;
  return (
    <span
      key={appKey}
      class="relative inline-flex shrink-0"
      style={{ width: `${size}px`, height: `${size}px` }}
    >
      <img
        key={appKey}
        src={src}
        alt={alt}
        width={size}
        height={size}
        aria-hidden={alt === '' ? 'true' : undefined}
        {...rest}
      />
      {/* Contorno de la tesela. El token es transparente en claro y se enciende en
          oscuro, donde el badge se disuelve contra la superficie (sobre #22262b,
          10 de 11 productos quedan bajo 3:1). Va aquí y no dentro del SVG porque
          los iconos se sirven con <img> y ahí las custom properties no resuelven. */}
      <span
        aria-hidden="true"
        class="pointer-events-none absolute"
        style={{
          inset: `${inset}px`,
          borderRadius: `${radio}px`,
          boxShadow: '0 0 0 1px var(--sytue-product-tile-ring, transparent)',
        }}
      />
    </span>
  );
}
