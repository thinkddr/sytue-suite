/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DÓNDE CAE LA FIRMA VISIBLE.
 *
 * El motor acepta cualquier recuadro en cualquier página; lo que faltaba era
 * dejar elegirlo. Se ofrecen las cuatro esquinas que se usan de verdad —abajo a
 * la izquierda, que es la de siempre y sigue siendo la de por defecto; abajo en
 * el centro y a la derecha, como en los contratos; y arriba a la derecha, donde
 * se sella un registro de entrada— y el recuadro se calcula con el tamaño REAL
 * de la página: con uno fijo, en una apaisada o en un tique estrecho la firma se
 * salía del papel.
 *
 * Todo en coordenadas del PDF (origen abajo-izquierda, sin /Rotate), que es lo
 * que espera el motor, y respecto a la MediaBox: si no empieza en 0,0, un
 * recuadro fijo caía fuera de la hoja.
 */
import type { Pagina } from './api';

export type Rect = [number, number, number, number];

export type PosicionFirma = 'abajoIzquierda' | 'abajoCentro' | 'abajoDerecha' | 'arribaDerecha';

/**
 * Lo que se elige: una de las cuatro posiciones o un recuadro DIBUJADO sobre la
 * página, para cuando la firma va en una línea concreta («Firma del cliente:»)
 * que ninguna esquina acierta.
 */
export type EleccionFirma = PosicionFirma | 'dibujada';

/** En el orden en que se ofrecen. La primera es la de por defecto. */
export const POSICIONES_FIRMA: readonly PosicionFirma[] = [
  'abajoIzquierda',
  'abajoCentro',
  'abajoDerecha',
  'arribaDerecha',
];

/** Margen desde el borde del papel, en puntos (unos 2 cm). */
export const MARGEN_FIRMA = 56;
/** Tamaño del recuadro: el de siempre, 240 × 74 pt. */
export const ANCHO_FIRMA = 240;
export const ALTO_FIRMA = 74;

/** A4 vertical: lo que se supone cuando aún no se sabe cómo es la página. */
const CAJA_A4: Rect = [0, 0, 595, 842];

/**
 * El recuadro de la firma en una página de MediaBox `caja`.
 *
 * Si no cabe con el margen entero —un tique, una etiqueta—, el margen se encoge
 * y el recuadro se ESCALA sin deformarse: la rúbrica se dibuja estirada al
 * recuadro, y cambiarle la proporción la aplastaría.
 */
export function rectFirma(posicion: PosicionFirma, caja: Rect = CAJA_A4): Rect {
  const [cx0, cy0, cx1, cy1] = caja;
  const anchoPagina = Math.max(0, cx1 - cx0);
  const altoPagina = Math.max(0, cy1 - cy0);
  // En cualquier hoja donde cabe (A4, carta, A5, apaisadas) el margen es el de
  // siempre; solo en una diminuta baja al 10 % del lado más corto, para dejar
  // papel a la firma.
  const cabe =
    anchoPagina >= ANCHO_FIRMA + 2 * MARGEN_FIRMA && altoPagina >= ALTO_FIRMA + 2 * MARGEN_FIRMA;
  const margen = cabe
    ? MARGEN_FIRMA
    : Math.min(MARGEN_FIRMA, Math.min(anchoPagina, altoPagina) * 0.1);
  const escala = Math.max(
    0,
    Math.min(1, (anchoPagina - 2 * margen) / ANCHO_FIRMA, (altoPagina - 2 * margen) / ALTO_FIRMA),
  );
  const w = ANCHO_FIRMA * escala;
  const h = ALTO_FIRMA * escala;

  const x0 =
    posicion === 'abajoIzquierda'
      ? cx0 + margen
      : posicion === 'abajoCentro'
        ? cx0 + (anchoPagina - w) / 2
        : cx1 - margen - w;
  const y0 = posicion === 'arribaDerecha' ? cy1 - margen - h : cy0 + margen;
  return [x0, y0, x0 + w, y0 + h].map(redondear) as Rect;
}

/**
 * La caja que tienen TODAS las páginas: «firmar en todas» pone el mismo recuadro
 * en cada una, así que se calcula sobre la zona común para que no se salga de la
 * más pequeña de un documento con hojas de varios tamaños.
 */
export function cajaComun(paginas: readonly Pick<Pagina, 'caja'>[]): Rect | undefined {
  if (paginas.length === 0) return undefined;
  return paginas.reduce<Rect>(
    (acc, p) => [
      Math.max(acc[0], p.caja[0]),
      Math.max(acc[1], p.caja[1]),
      Math.min(acc[2], p.caja[2]),
      Math.min(acc[3], p.caja[3]),
    ],
    [...paginas[0]!.caja],
  );
}

/**
 * La caja sobre la que se coloca la firma: la de la página elegida (1-based) o,
 * si se firma en todas, la común. Una página que no existe cae en la primera,
 * que es la que el motor rechazaría con un error más claro que un recuadro raro.
 */
export function cajaParaFirmar(
  paginas: readonly Pick<Pagina, 'caja'>[],
  pagina: number,
  todas: boolean,
): Rect | undefined {
  if (todas) return cajaComun(paginas);
  return (paginas[pagina - 1] ?? paginas[0])?.caja;
}

// ---- El recuadro dibujado ----

/** Proporción del recuadro: la de siempre, para que la rúbrica no se deforme. */
const PROPORCION_FIRMA = ANCHO_FIRMA / ALTO_FIRMA;
/** Lo más pequeño que se deja dibujar, en puntos: menos sería ilegible. */
export const ANCHO_MINIMO_FIRMA = 48;

/**
 * El recuadro dibujado se guarda en FRACCIONES de la caja (0-1, origen abajo a
 * la izquierda) y no en puntos: si luego se cambia de página o se marca «firmar
 * en todas» y la caja cambia, el recuadro sigue en el mismo sitio de la hoja en
 * vez de quedarse fuera.
 */
export type Fracciones = Rect;

/** Puntos de la caja → fracciones. */
export function aFracciones(caja: Rect, r: Rect): Fracciones {
  const w = Math.max(1e-9, caja[2] - caja[0]);
  const h = Math.max(1e-9, caja[3] - caja[1]);
  return [(r[0] - caja[0]) / w, (r[1] - caja[1]) / h, (r[2] - caja[0]) / w, (r[3] - caja[1]) / h];
}

/**
 * Fracciones → el recuadro en puntos sobre `caja`. En otra caja de proporción
 * distinta se conserva la esquina de abajo a la izquierda y el ancho relativo, y
 * el alto sale de la proporción de la firma: estirar las fracciones tal cual la
 * deformaría.
 */
export function deFracciones(caja: Rect, f: Fracciones): Rect {
  const w = caja[2] - caja[0];
  const ancho = (f[2] - f[0]) * w;
  return encajar(caja, [
    caja[0] + f[0] * w,
    caja[1] + f[1] * (caja[3] - caja[1]),
    caja[0] + f[0] * w + ancho,
    caja[1] + f[1] * (caja[3] - caja[1]) + ancho / PROPORCION_FIRMA,
  ]);
}

/**
 * El recuadro que sale de arrastrar de `a` a `b` (puntos del PDF). La proporción
 * va fija —manda el lado que más se ha estirado—, con un mínimo para que un clic
 * suelto deje un recuadro usable, y nunca se sale del papel.
 */
export function recuadroArrastrado(caja: Rect, a: [number, number], b: [number, number]): Rect {
  const dx = b[0] - a[0];
  const dy = b[1] - a[1];
  const w = Math.max(Math.abs(dx), Math.abs(dy) * PROPORCION_FIRMA, ANCHO_MINIMO_FIRMA);
  const h = w / PROPORCION_FIRMA;
  const x0 = dx >= 0 ? a[0] : a[0] - w;
  const y0 = dy >= 0 ? a[1] : a[1] - h;
  return encajar(caja, [x0, y0, x0 + w, y0 + h]);
}

/** Desplaza el recuadro (puntos del PDF) sin dejarlo salir del papel. */
export function moverRecuadro(caja: Rect, r: Rect, dx: number, dy: number): Rect {
  return encajar(caja, [r[0] + dx, r[1] + dy, r[2] + dx, r[3] + dy]);
}

/** Agranda o encoge el recuadro por su centro, sin bajar del mínimo ni salirse. */
export function escalarRecuadro(caja: Rect, r: Rect, factor: number): Rect {
  const w = Math.max(ANCHO_MINIMO_FIRMA, (r[2] - r[0]) * factor);
  const h = w / PROPORCION_FIRMA;
  const cx = (r[0] + r[2]) / 2;
  const cy = (r[1] + r[3]) / 2;
  return encajar(caja, [cx - w / 2, cy - h / 2, cx + w / 2, cy + h / 2]);
}

/**
 * Mete el recuadro en la caja: si es más grande que la hoja lo encoge sin
 * deformarlo, y si asoma por un lado lo empuja hacia dentro.
 */
export function encajar(caja: Rect, r: Rect): Rect {
  const anchoCaja = Math.max(0, caja[2] - caja[0]);
  const altoCaja = Math.max(0, caja[3] - caja[1]);
  let w = Math.abs(r[2] - r[0]);
  let h = Math.abs(r[3] - r[1]);
  const escala = Math.min(1, w > 0 ? anchoCaja / w : 1, h > 0 ? altoCaja / h : 1);
  w *= escala;
  h *= escala;
  const x0 = Math.min(Math.max(Math.min(r[0], r[2]), caja[0]), caja[2] - w);
  const y0 = Math.min(Math.max(Math.min(r[1], r[3]), caja[1]), caja[3] - h);
  return [x0, y0, x0 + w, y0 + h].map(redondear) as Rect;
}

/**
 * El recuadro de lo elegido: el de la posición o, si se dibujó, el dibujado. Uno
 * dibujado que aún no existe sale en la posición de siempre, que es de donde se
 * empieza a moverlo.
 */
export function rectElegido(
  eleccion: EleccionFirma,
  caja: Rect | undefined,
  dibujo: Fracciones | null,
): Rect {
  if (eleccion !== 'dibujada') return rectFirma(eleccion, caja);
  const c = caja ?? CAJA_A4;
  return dibujo ? deFracciones(c, dibujo) : rectFirma(POSICIONES_FIRMA[0]!, c);
}

/** Medio punto basta: más decimales solo ensucian el PDF. */
function redondear(n: number): number {
  return Math.round(n * 2) / 2;
}
