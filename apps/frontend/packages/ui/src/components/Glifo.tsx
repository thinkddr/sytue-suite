/**
 * Glifo — EL envoltorio de todo icono de interfaz de la suite: el `<svg>`, el
 * grosor del trazo y la capa de relleno. `Icon` y los catálogos propios de cada
 * app (Rooms, Pages, Drive, Mail, la cinta de Docs…) pintan a través de él: lo
 * que cambia de un catálogo a otro es el DIBUJO, no cómo se traza.
 *
 * Estaba repetido: cada catálogo traía su `<svg … stroke-width={2}>` y los
 * sueltos lo escribían en línea. Un estilo nuevo habría sido cuarenta ediciones,
 * y el grosor ya había empezado a divergir solo (2, 1,8, 1,7 y un 3). Ahora el
 * estilo se decide aquí, y `glifos-al-comun.test.ts` se pone en rojo si alguien
 * vuelve a escribir un envoltorio propio.
 *
 * EL ESTILO, «duotono con acento» (docs/design.md §3.20):
 *
 *  1. TRAZO DE 1,5 px EN PANTALLA a cualquier tamaño. El `stroke-width={2}` fijo
 *     sobre una rejilla de 24 escalaba con el icono —1,3 px a 16 y 2,7 px a 32—,
 *     así que el mismo glifo pesaba distinto en una barra y en un estado vacío.
 *  2. RELLENO DE ACENTO bajo el trazo, solo en las formas CERRADAS del dibujo y
 *     con `fill-rule: evenodd`, de modo que una forma dentro de otra queda como
 *     hueco (la pupila del ojo, el eje del engranaje). Los glifos de línea pura
 *     —chevrones, aspa, visto— no llevan relleno: solo cambian de grosor.
 *     Los PUNTOS del dibujo (los de «…», las viñetas) van macizos y de tamaño
 *     propio: con el trazo fino dejaban de verse.
 *  3. EL COLOR DEL RELLENO es la variable `--sytue-glyph-fill`, que por defecto
 *     cae en el primario. La cambia quien pone el icono sobre otro fondo: el
 *     carril de color la fija en su regla, y vale `currentColor` (styles.css)
 *     sobre un relleno de color, porque acento sobre acento no se ve, y cuando la
 *     tinta ya es de color —rojo de peligro, verde de éxito—, porque el relleno
 *     sigue a la tinta.
 *
 * No se redibuja nada: son los mismos trazos de cada catálogo.
 */
import { Fragment, toChildArray, type ComponentChildren, type JSX } from 'preact';
import { cx } from '../lib/cx.js';

/** Lado de la rejilla en la que se dibujan todos los glifos. */
const REJILLA = 24;
/** Grosor del trazo en píxeles de pantalla, a cualquier tamaño. */
const TRAZO_PX = 1.5;

/**
 * `stroke-width` en unidades de la rejilla para que el trazo mida 1,5 px con el
 * icono a `size` px. Se exporta para el glifo que lleva un grupo ESCALADO dentro
 * y tiene que compensar el grosor a mano (`PdfGlyph` de Sytue Docs).
 */
export function grosorDeGlifo(size: number): number {
  return Math.round(((TRAZO_PX * REJILLA) / size) * 100) / 100;
}

const r3 = (v: number): number => Math.round(v * 1000) / 1000;

const NUMERO = '[-+]?(?:\\d*\\.\\d+|\\d+)';
/** El `M x y` (o `m dx dy`) con el que empieza cada subtrazo. */
const INICIO = new RegExp(`^([Mm])\\s*(${NUMERO})[\\s,]*(${NUMERO})[\\s,]*`);

/**
 * Los subtrazos CERRADOS de un `d` —los que acaban en `Z`—, unidos en un solo
 * trazado: es lo que se rellena. Los abiertos no, porque SVG los cerraría por su
 * cuenta y un chevrón saldría con un triángulo dentro.
 *
 * Al sacar un subtrazo de su sitio, una `m` RELATIVA deja de valer: su origen es
 * donde quedó la pluma. Tras un `Z` eso se sabe sin interpretar nada —vuelve al
 * inicio del subtrazo que cierra—, así que se reescribe como `M` absoluta (el
 * tercer nodo de «compartir» en Sytue Docs se dibuja así).
 *
 * ponytail: tras un subtrazo ABIERTO la pluma está donde acabó su último tramo,
 * y saberlo pide interpretar el trazado entero (arcos incluidos). Un cerrado que
 * empiece con `m` relativa justo ahí se queda sin relleno. Ningún catálogo tiene
 * uno; si aparece, se escribe con `M` o se sigue el punto aquí.
 */
export function subtrazosCerrados(d: string): string {
  const cerrados: string[] = [];
  // Dónde está la pluma al empezar el subtrazo; `null` si no se sabe.
  let pluma: readonly [number, number] | null = [0, 0];
  for (const s of d.trim().split(/(?=[Mm])/)) {
    const sub = s.trim();
    const m = INICIO.exec(sub);
    const relativa = m?.[1] === 'm';
    const inicio: readonly [number, number] | null = !m
      ? null
      : relativa
        ? pluma && [pluma[0] + Number(m[2]), pluma[1] + Number(m[3])]
        : [Number(m[2]), Number(m[3])];
    const cerrado = /z$/i.test(sub);
    if (m && inicio && cerrado) {
      const resto = sub.slice(m[0].length);
      // Los pares que siguen a una `m` son líneas relativas implícitas: con la
      // `M` absoluta delante hay que decirlo, o pasarían a ser absolutas.
      const sigue = !/^[-+.\d]/.test(resto) ? '' : relativa ? 'l' : ' ';
      cerrados.push(`M${r3(inicio[0])} ${r3(inicio[1])}${sigue}${resto}`);
    }
    pluma = cerrado ? inicio : null;
  }
  return cerrados.join(' ');
}

/** Radio, en la rejilla, de un punto del dibujo: 2,1 px a 20 y 2,5 px a 24. */
const RADIO_PUNTO = 1.25;
/** Un subtrazo que es solo un punto: `M x y h.01`, que es como se dibujan en la casa. */
const PUNTO = new RegExp(`^M\\s*(${NUMERO})[\\s,]*(${NUMERO})\\s*h\\.01$`);

/**
 * Los PUNTOS de un `d` —los tres de «…», las viñetas de una lista, el punto de la
 * «i»— como círculos, para pintarlos macizos. Dibujados como un tramo de longitud
 * cero dependían del grosor para verse: con el trazo de 2 eran un punto de 2
 * unidades, y con el de 1,5 px se quedaban en una mota a 20–24 px. No se redibuja
 * nada: es el mismo punto, en el mismo sitio, con un tamaño que ya no sale del trazo.
 */
export function puntos(d: string): string {
  return d
    .trim()
    .split(/(?=M)/)
    .map((sub) => {
      const m = PUNTO.exec(sub.trim());
      if (!m) return '';
      const r = RADIO_PUNTO;
      return `M${r3(Number(m[1]) - r)} ${m[2]}a${r} ${r} 0 1 0 ${2 * r} 0a${r} ${r} 0 1 0 ${-2 * r} 0Z`;
    })
    .filter(Boolean)
    .join(' ');
}

/**
 * El contorno de una figura hija como `d`, para la capa de relleno. Los catálogos
 * que dibujan con primitivas (`<rect>`, `<circle>`…) se rellenan igual que los que
 * dibujan con un `d`, y en UN solo trazado, que es lo que hace que el par-impar
 * abra huecos entre figuras distintas (el ojo y su pupila).
 */
function contorno(tipo: string, p: Record<string, unknown>): string {
  const n = (k: string): number => Number(p[k] ?? 0);
  const texto = (k: string): string => {
    const v = p[k];
    return typeof v === 'string' ? v : '';
  };
  switch (tipo) {
    case 'path':
      return subtrazosCerrados(texto('d'));
    case 'circle':
    case 'ellipse': {
      const rx = n(tipo === 'circle' ? 'r' : 'rx');
      const ry = n(tipo === 'circle' ? 'r' : 'ry');
      return `M${r3(n('cx') - rx)} ${n('cy')}a${rx} ${ry} 0 1 0 ${r3(2 * rx)} 0a${rx} ${ry} 0 1 0 ${r3(-2 * rx)} 0Z`;
    }
    case 'rect': {
      const w = n('width');
      const h = n('height');
      // `rx` sin `ry` (y al revés) vale para los dos, como dice SVG.
      const rx = Math.min(n(p.rx === undefined ? 'ry' : 'rx'), w / 2);
      const ry = Math.min(n(p.ry === undefined ? 'rx' : 'ry'), h / 2);
      const esquina = (dx: number, dy: number): string =>
        rx && ry ? `a${rx} ${ry} 0 0 1 ${dx} ${dy}` : '';
      return (
        `M${r3(n('x') + rx)} ${n('y')}h${r3(w - 2 * rx)}${esquina(rx, ry)}v${r3(h - 2 * ry)}` +
        `${esquina(-rx, ry)}h${r3(2 * rx - w)}${esquina(-rx, -ry)}v${r3(2 * ry - h)}${esquina(rx, -ry)}Z`
      );
    }
    case 'polygon':
      return `M${texto('points').trim()}Z`;
    default:
      // `line` y `polyline` son abiertas. Un `<g>` tampoco se rellena: lo que
      // lleva dentro va en otro sistema de coordenadas (el sello de `PdfGlyph`).
      return '';
  }
}

/** Las figuras hijas que se pintan con el estilo del glifo, aplanando fragmentos. */
function figuras(hijos: ComponentChildren): { tipo: string; p: Record<string, unknown> }[] {
  return toChildArray(hijos).flatMap((h) => {
    if (typeof h !== 'object') return [];
    const p = h.props as Record<string, unknown>;
    if (h.type === Fragment) return figuras(p.children as ComponentChildren);
    // Una figura con `fill` o `stroke` propios ya dice cómo se pinta (el punto
    // macizo de «no leído»): no se le pone nada debajo ni encima.
    if (typeof h.type !== 'string' || p.fill !== undefined || p.stroke !== undefined) return [];
    return [{ tipo: h.type, p }];
  });
}

const unir = (trozos: string[]): string => trozos.filter(Boolean).join(' ');

export interface GlifoProps extends JSX.SVGAttributes<SVGSVGElement> {
  /** Tamaño en px (cuadrado). Por defecto 20. De él sale el grosor del trazo. */
  size?: number | undefined;
  /** El dibujo como un único `d`. Si el catálogo dibuja con primitivas, van como hijos. */
  d?: string | undefined;
  /** Etiqueta accesible; si se omite, el icono es decorativo (aria-hidden). */
  label?: string | undefined;
}

export function Glifo({
  size = 20,
  d,
  label,
  class: cls,
  // Con valor por defecto y no en `...rest`: quien pasa `fill={marcada ? … : undefined}`
  // manda un `undefined` explícito, que al esparcirse BORRARÍA el `fill="none"` y
  // dejaría el icono relleno de negro, que es lo que hace SVG si no se le dice nada.
  fill = 'none',
  children,
  ...rest
}: GlifoProps): JSX.Element {
  const decorativo = label === undefined;
  // Con `fill` propio la forma va LLENA (la estrella o el corazón marcados): el
  // acento quedaría debajo y no se vería.
  const hijas = d === undefined ? figuras(children) : [{ tipo: 'path', p: { d } }];
  const relleno = fill !== 'none' ? '' : unir(hijas.map((f) => contorno(f.tipo, f.p)));
  const motas = unir(hijas.map((f) => (typeof f.p.d === 'string' ? puntos(f.p.d) : '')));
  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${REJILLA} ${REJILLA}`}
      fill={fill}
      stroke="currentColor"
      stroke-width={grosorDeGlifo(size)}
      stroke-linecap="round"
      stroke-linejoin="round"
      class={cx('shrink-0', cls)}
      role={decorativo ? undefined : 'img'}
      aria-hidden={decorativo ? 'true' : undefined}
      aria-label={label}
      {...rest}
    >
      {relleno && (
        <path
          d={relleno}
          fill="var(--sytue-glyph-fill, var(--sytue-primary))"
          fill-opacity="0.34"
          fill-rule="evenodd"
          stroke="none"
        />
      )}
      {d !== undefined && <path d={d} />}
      {children}
      {motas && <path d={motas} fill="currentColor" stroke="none" />}
    </svg>
  );
}
