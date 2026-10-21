/**
 * OrgMark — la marca que ve una organización que NO ha subido la suya.
 *
 * Es NUESTRO logotipo con la cuña teñida de un color derivado del slug. Ni el badge
 * ni el monograma cambian: lo que se personaliza es el acento, para que dos empresas
 * con la suite abierta en pestañas contiguas no vean exactamente lo mismo, y sin que
 * ninguna acabe con una marca que no es de esta casa.
 *
 * DETERMINISTA, no aleatorio, aunque el encargo dijera «aleatorio»: un color que
 * cambiara entre recargas o entre apps se lee como un fallo, no como personalización.
 * Mismo slug, mismo color, siempre y en las once superficies.
 *
 * Doce colores medidos (`--sytue-orgmark-1..12`, en branding/tokens/colors.css) y no
 * un tono libre: un hue aleatorio da fucsias y verdes fluorescentes que no son de la
 * marca, y no habría forma de garantizar el contraste. Con cientos de miles de
 * organizaciones se repiten por construcción — esto ayuda a reconocer, no identifica.
 */
import type { JSX } from 'preact';
// El SVG como TEXTO y no como URL: dentro de un <img> no se puede teñir nada, y el
// color tiene que entrar por dentro. `?raw` lo resuelve Vite, que es lo que compila
// las once apps.
import marcaRaw from '../assets/logos/sytue-one-white.svg?raw';
import { cx } from '../lib/cx.js';

/**
 * Ancla del tinte: el ÚNICO `fill` de la cuña en el SVG de marca.
 *
 * Si alguien reexporta el logotipo y este literal deja de aparecer, el reemplazo no
 * hace nada y TODAS las organizaciones vuelven al latón: se ve bien, así que nadie lo
 * nota. Por eso hay un test que comprueba que el ancla sigue en el fichero.
 */
export const ANCLA_CUNA = 'fill="#9c743a"';

/** Cuántos colores hay. El test comprueba que los doce existen y contrastan. */
export const COLORES_ORGMARK = 12;

/** Índice de color a partir del slug. Mismo bucle que `bgFor` en Avatar. */
export function colorDeOrg(slug: string): number {
  let hash = 0;
  for (let i = 0; i < slug.length; i += 1) hash = (hash * 31 + slug.charCodeAt(i)) >>> 0;
  return (hash % COLORES_ORGMARK) + 1;
}

export interface OrgMarkProps {
  /** Slug de la organización. Vacío ⇒ el latón de la marca, sin teñir. */
  slug?: string | undefined;
  /** Alto en píxeles (la barra superior usa 28). */
  size?: number;
  class?: string;
}

export function OrgMark({ slug, size = 28, class: cls }: OrgMarkProps): JSX.Element {
  const svg = slug
    ? marcaRaw.replace(ANCLA_CUNA, `fill="var(--sytue-orgmark-${colorDeOrg(slug)})"`)
    : marcaRaw;
  return (
    <span
      // Decorativo: el nombre de la organización ya lo dice el texto de al lado, y
      // el disparador del selector de aplicaciones tiene su propia etiqueta.
      aria-hidden="true"
      // El contorno de 1px sigue el radio del badge (9 de 40 = 22,5 %). El símbolo es
      // la versión REVERSA —badge blanco— y sin borde se leía como un cuadro suelto
      // encima de cualquier superficie clara (la vista previa del logo, el fondo del
      // buscador). Con el borde de tinta al 16 % se lee como la baldosa que es; sobre
      // la barra de tinta o en tema oscuro, el mismo borde no se ve y no estorba.
      class={cx(
        'inline-flex shrink-0 items-center rounded-[22.5%] shadow-[0_0_0_1px_var(--sytue-border-strong)] [&>svg]:h-full [&>svg]:w-auto',
        cls,
      )}
      style={{ height: `${size}px` }}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
