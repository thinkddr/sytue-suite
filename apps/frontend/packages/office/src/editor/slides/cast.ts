/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Presentar con Cast»: lógica del cliente. Diapos programa un directo en Sytue Cast
 * (la emisión se hace en la página de Cast) y, mientras se presenta, avisa de la
 * diapositiva en la que va. Sin Preact ni DOM salvo `fetch`: se prueba con bun
 * (test/slides-cast.test.ts).
 *
 * SOLO SALE LA POSICIÓN: el número de la diapositiva, cuántas hay y su título. Ni el
 * contenido ni las notas del ponente viajan a Cast (el BFF `/api/cast-presentar`
 * tampoco las acepta). Las notas siguen siendo del ponente, en su vista del moderador.
 */
import { appUrl } from '@sytue/config';
import type { Slide, SlideBox } from '../types';

/** Lo que se envía del título de una diapositiva (Cast guarda 120 caracteres). */
export const MAX_TITULO = 120;

const ENTIDADES: Record<string, string> = {
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
  '&nbsp;': ' ',
};

/** Texto plano de un trozo de HTML del editor: sin etiquetas y con las entidades de siempre. */
export function textoPlano(html: string): string {
  return html
    .replace(/<br\s*\/?>/gi, ' ')
    .replace(/<[^>]*>/g, ' ')
    .replace(/&(?:amp|lt|gt|quot|#39|nbsp);/g, (e) => ENTIDADES[e] ?? e)
    .replace(/\s+/g, ' ')
    .trim();
}

const esTexto = (b: SlideBox): boolean => b.kind === undefined || b.kind === 'text';

/**
 * El título de una diapositiva: el de su primera caja de texto que empieza por un
 * encabezado (`<h1>`…`<h6>`) o, si no hay, la primera línea de la primera caja de texto
 * con algo escrito. "" si no tiene texto.
 */
export function tituloDeDiapositiva(slide: Slide | undefined): string {
  if (!slide) return '';
  const cajas = slide.boxes.filter(esTexto);
  const conEncabezado = cajas.find((b) => /^\s*<h[1-6][\s>]/i.test(b.html) && textoPlano(b.html));
  const caja = conEncabezado ?? cajas.find((b) => textoPlano(b.html) !== '');
  if (!caja) return '';
  // Solo el primer bloque: el encabezado, o la primera línea.
  const texto = textoPlano(caja.html.split(/<\/(?:p|div|li|h[1-6])>/i)[0] ?? '');
  return Array.from(texto).slice(0, MAX_TITULO).join('');
}

export type VisibilidadCast = 'org' | 'people' | 'unlisted' | 'public';

export class FalloCast extends Error {
  constructor(
    readonly estado: number,
    readonly codigo: string,
  ) {
    super(`cast: ${estado} ${codigo}`);
  }
}

async function pedir(url: string, metodo: 'POST' | 'PUT', cuerpo: unknown): Promise<unknown> {
  const res = await fetch(url, {
    method: metodo,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(cuerpo),
  });
  let datos: unknown = null;
  try {
    datos = await res.json();
  } catch {
    // Sin cuerpo JSON: basta con el estado.
  }
  if (!res.ok) {
    throw new FalloCast(res.status, (datos as { error?: string } | null)?.error ?? 'error');
  }
  return datos;
}

/** Programa el directo en Cast con el título dado. Devuelve el id del vídeo de Cast. */
export async function programarEmision(
  titulo: string,
  visibilidad: VisibilidadCast,
  ref: string,
): Promise<string> {
  const out = (await pedir('/api/cast-presentar', 'POST', {
    title: titulo,
    visibility: visibilidad,
    ref,
  })) as { videoId?: unknown };
  if (typeof out.videoId !== 'string') throw new FalloCast(502, 'cast_error');
  return out.videoId;
}

/** La página de Cast desde la que se emite el directo programado. */
export function urlDeEmision(videoId: string, org?: string | null): string {
  return appUrl('cast', `studio?live=${encodeURIComponent(videoId)}`, org);
}

export interface PosicionCast {
  index: number;
  total: number;
  title: string;
  ref: string;
}

/** La posición de la diapositiva `index` (desde 0) de una presentación. */
export function posicionDe(
  slides: readonly Slide[],
  index: number,
  docId: string,
): PosicionCast | null {
  if (slides.length === 0 || index < 0 || index >= slides.length) return null;
  return {
    index,
    total: slides.length,
    title: tituloDeDiapositiva(slides[index]),
    ref: `docs:${docId}`,
  };
}

/**
 * Un avisador de posiciones para UN directo: manda la última, de una en una y sin
 * amontonar. Si se pasa de diapositiva mientras una petición va, se manda solo la más
 * reciente al acabar esa. Los fallos se entregan a `alFallar` (una vez por racha) y no
 * paran la presentación: presentar no depende de que Cast conteste.
 */
export function avisador(
  directo: string,
  alFallar: (e: unknown) => void,
  enviar: (url: string, cuerpo: PosicionCast) => Promise<unknown> = (url, cuerpo) =>
    pedir(url, 'PUT', cuerpo),
): (p: PosicionCast) => void {
  let enCurso = false;
  let pendiente: PosicionCast | null = null;
  let falloAvisado = false;
  const url = `/api/cast-presentar?live=${encodeURIComponent(directo)}`;
  const bombear = (): void => {
    if (enCurso || !pendiente) return;
    const p = pendiente;
    pendiente = null;
    enCurso = true;
    enviar(url, p)
      .then(() => {
        falloAvisado = false;
      })
      .catch((e: unknown) => {
        if (!falloAvisado) alFallar(e);
        falloAvisado = true;
      })
      .finally(() => {
        enCurso = false;
        bombear();
      });
  };
  return (p) => {
    pendiente = p;
    bombear();
  };
}
