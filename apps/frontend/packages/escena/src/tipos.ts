/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Modelo de la escena vectorial común. Una `Escena` es un lienzo de ancho × alto
 * (unidades abstractas, px de usuario del SVG) con CAPAS con nombre ordenadas de
 * abajo arriba; cada capa lleva NODOS ordenados también de abajo arriba (el
 * orden del array ES el orden Z). Todo es JSON plano y serializable: nada de
 * funciones, referencias circulares ni DOM.
 *
 * Coordenadas: cada nodo vive en el sistema de su PADRE (el lienzo, o el local
 * de un grupo). Los nodos de caja (forma, texto, imagen, grupo) llevan x/y/w/h y
 * un giro `rot` en grados alrededor de su centro; los grupos colocan a sus hijos
 * en su sistema local (origen en la esquina superior izquierda del grupo).
 * Las líneas y los trazos llevan sus puntos; los conectores, extremos anclados
 * a nodos (por id) o puntos libres, siempre en coordenadas del lienzo.
 */
import type { ConnectorAnchor, ConnectorKind } from './conector.js';

/** Versión del formato serializado. Sube con cada cambio incompatible y deja una
 *  migración en `serializar.ts` (nunca se reescribe una ya publicada). */
export const ESCENA_VERSION = 1;

/** Formas de caja: las poligonales de `formas.ts` más rectángulo y elipse. */
export type TipoForma =
  | 'rect'
  | 'ellipse'
  | 'triangle'
  | 'diamond'
  | 'pentagon'
  | 'hexagon'
  | 'star'
  | 'cylinder'
  | 'callout';

/** Estilo común. Colores `#RRGGBB`; ausente = la tinta del tema (currentColor),
 *  de modo que el dibujo se adapta a claro y oscuro sin guardar un color fijo. */
export interface Estilo {
  relleno?: string;
  trazo?: string;
  /** Grosor del trazo en unidades de la escena (0 = sin borde). */
  grosor?: number;
  /** 0..1; ausente = 1. */
  opacidad?: number;
  discontinuo?: boolean;
}

interface NodoBase {
  id: string;
  /** Nombre accesible del nodo (lo leen los lectores de pantalla y el panel de capas). */
  nombre?: string;
  estilo?: Estilo;
}

/** Nodos con caja: x/y/w/h en el sistema del padre y giro en grados. */
interface NodoCaja extends NodoBase {
  x: number;
  y: number;
  w: number;
  h: number;
  rot?: number;
}

export type AlineacionTexto = 'start' | 'middle' | 'end';

export interface NodoForma extends NodoCaja {
  tipo: 'forma';
  forma: TipoForma;
  /** Rótulo centrado dentro de la forma. */
  texto?: string;
  /** Tamaño del texto en unidades de la escena. */
  tamTexto?: number;
}

export interface NodoTexto extends NodoCaja {
  tipo: 'texto';
  texto: string;
  tamTexto?: number;
  alinear?: AlineacionTexto;
  negrita?: boolean;
}

export interface NodoImagen extends NodoCaja {
  tipo: 'imagen';
  /** `data:image/(png|jpeg|webp|gif)` o ruta http(s)/absoluta (saneado al leer). */
  href: string;
  /** Texto alternativo. */
  alt: string;
}

export interface NodoGrupo extends NodoCaja {
  tipo: 'grupo';
  hijos: Nodo[];
}

export interface NodoLinea extends NodoBase {
  tipo: 'linea';
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  flechaIni?: boolean;
  flechaFin?: boolean;
}

/** Extremo de un conector: anclado a un nodo (por id, se recalcula al moverlo) o punto libre. */
export type ExtremoConector = { nodo: string; lado?: ConnectorAnchor } | { x: number; y: number };

export interface NodoConector extends NodoBase {
  tipo: 'conector';
  trazado: ConnectorKind;
  desde: ExtremoConector;
  hasta: ExtremoConector;
  flechaIni?: boolean;
  flechaFin?: boolean;
}

/** Trazo a mano alzada: puntos planos [x0, y0, x1, y1, …] en el sistema del padre. */
export interface NodoTrazo extends NodoBase {
  tipo: 'trazo';
  puntos: number[];
}

export type Nodo =
  NodoForma | NodoTexto | NodoImagen | NodoGrupo | NodoLinea | NodoConector | NodoTrazo;

export type TipoNodo = Nodo['tipo'];

export interface Capa {
  id: string;
  nombre: string;
  visible: boolean;
  bloqueada: boolean;
  nodos: Nodo[];
}

export interface Escena {
  v: number;
  ancho: number;
  alto: number;
  /** Título y descripción accesibles del dibujo entero (aria-label y <desc> del SVG). */
  titulo?: string;
  descripcion?: string;
  /** Fondo `#RRGGBB`; ausente = transparente. */
  fondo?: string;
  /** De abajo arriba. */
  capas: Capa[];
}

export interface Punto {
  x: number;
  y: number;
}

/** Grosor del trazo cuando el estilo no lo fija. */
export const GROSOR_DEFECTO = 1.5;
