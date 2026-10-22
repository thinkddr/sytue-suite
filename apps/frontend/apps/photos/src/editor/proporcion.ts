/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Recorte con proporción fija (1:1, 4:3, 3:2, 16:9). El marco de recorte se
 * guarda en FRACCIONES del lienzo, que no es cuadrado, así que la proporción
 * hay que calcularla en PÍXELES y devolverla otra vez a fracciones: en un
 * lienzo apaisado, un marco «cuadrado» en fracciones sale rectangular en la
 * foto, que es justo lo que la proporción viene a evitar.
 */
export interface Marco {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Lienzo {
  ancho: number;
  alto: number;
}

/** Las proporciones que ofrece el editor; `null` es libre. */
export const PROPORCIONES = [null, 1, 4 / 3, 3 / 2, 16 / 9] as const;

/**
 * Ajusta `marco` a `proporcion` (ancho/alto en píxeles) creciendo desde `ancla`
 * —el punto donde empezó el arrastre— y sin salirse del lienzo. Se conserva el
 * lado más pequeño de los dos, así el marco nunca crece por su cuenta.
 */
export function conProporcion(
  marco: Marco,
  proporcion: number,
  lienzo: Lienzo,
  ancla: { x: number; y: number },
): Marco {
  const { ancho: W, alto: H } = lienzo;
  if (W <= 0 || H <= 0 || proporcion <= 0) return marco;
  let wpx = marco.w * W;
  let hpx = marco.h * H;
  if (wpx / Math.max(hpx, 1e-9) > proporcion) wpx = hpx * proporcion;
  else hpx = wpx / proporcion;

  // Hacia dónde se arrastró: el marco crece desde el ancla en ese sentido.
  const haciaIzquierda = marco.x + marco.w <= ancla.x + 1e-9;
  const haciaArriba = marco.y + marco.h <= ancla.y + 1e-9;

  // Tope del lienzo en el sentido del arrastre, en píxeles.
  const maxW = (haciaIzquierda ? ancla.x : 1 - ancla.x) * W;
  const maxH = (haciaArriba ? ancla.y : 1 - ancla.y) * H;
  const escala = Math.min(1, maxW / Math.max(wpx, 1e-9), maxH / Math.max(hpx, 1e-9));
  wpx *= escala;
  hpx *= escala;

  const w = wpx / W;
  const h = hpx / H;
  return {
    x: haciaIzquierda ? ancla.x - w : ancla.x,
    y: haciaArriba ? ancla.y - h : ancla.y,
    w,
    h,
  };
}
