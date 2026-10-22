/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/editar-ia  {"id": "<foto>", "instruccion": "…", "accion": "…"}
 *
 * Solicita a Sytue AI una edición o retoque fotográfico inteligente sobre una foto
 * utilizando visión artificial, devolviendo los parámetros calculados y los pasos
 * de la receta no destructiva para aplicarlos en el lienzo.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/bff.js';
import { convertirEdicionEnPasos, interpretarEdicionIA } from '../../lib/edicionIA.js';
import { json, miniaturaEnBase64, urlDeIA, verLaFoto } from '../../lib/visionIA.js';

export const prerender = false;

const TOPE_INSTRUCCION = 500;

/** Descripción de los preajustes reconocidos para alimentar el contexto del modelo. */
const PREAJUSTES: Record<string, string> = {
  'mejora-auto':
    'Mejora automática inteligente: analiza la iluminación, histograma y rango dinámico para equilibrar exposición, contraste y color con aspecto profesional y natural.',
  atardecer:
    'Atardecer cálido / Golden hour: potencia tonos cálidos dorados, contraste suave y luces crepusculares envolventes.',
  cinematografico:
    'Estilo cinematográfico: contraste tonal marcado, sombras profundas con carácter y temperatura sutilmente cálida estilo celuloide de 35 mm.',
  'bn-dramatico':
    'Blanco y negro dramático: desaturación total (-1.0), alto contraste y realce de texturas con negros densos y luces limpias.',
  vivido:
    'Paisaje vívido: mayor riqueza cromática en vegetación, cielos y agua con contraste limpio sin quemar altas luces.',
  vintage:
    'Retro / Vintage: tonos nostálgicos atenuados, sombras suaves y ligera calidez evocando fotografía clásica.',
  reencuadre:
    'Reencuadre inteligente: analiza la composición y sugiere un recorte armónico centrado en el sujeto principal siguiendo la regla de los tercios.',
  noche:
    'Poca luz / Noche: rescata sombras subexpuestas, controla el ruido aumentando la claridad sin perder el ambiente nocturno.',
};

/**
 * Instrucción de sistema para el modelo de visión.
 *
 * Se le pide que actúe como director de fotografía y revelador experto,
 * analice la foto visualmente y devuelva ÚNICAMENTE un objeto JSON
 * con los ajustes numéricos de la receta de revelado de Sytue Photos.
 */
const INSTRUCCION = [
  'You are an expert photographic director and master digital darkroom retoucher.',
  'Analyze this photo visually (lighting, exposure, shadows, highlights, color balance, composition).',
  'Determine the optimal non-destructive adjustments to fulfill the user’s request while preserving artistic and photographic quality.',
  'Answer ONLY with a JSON object: {"explicacion": string, "ajustes": {"exposicion"?: number, "contraste"?: number, "saturacion"?: number, "temperatura"?: number}, "recorte"?: {"x": number, "y": number, "ancho": number, "alto": number}}.',
  'The "explicacion" must be 1-2 concise sentences in the language of this request explaining what adjustments were made and why.',
  'The "ajustes" must use numbers in these ranges: exposicion (-5.0 to 5.0), contraste (-1.0 to 1.0), saturacion (-1.0 to 1.0, where -1.0 is full monochrome), temperatura (-1.0 to 1.0, where negative is cold/blue and positive is warm/golden).',
  'Only provide "recorte" if the user explicitly requested reframing, centering or cropping.',
  'Do not name or identify people.',
].join(' ');

export const POST: APIRoute = async (ctx) => {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return json({ error: 'unauthenticated' }, 401);

  const base = urlDeIA();
  if (!base) return json({ error: 'ai_not_configured' }, 503);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    id?: string;
    instruccion?: string;
    accion?: string;
  };

  const id = body.id;
  if (!id) return json({ error: 'missing_id' }, 400);

  const textoUsuario = (body.instruccion ?? '').trim();
  const accion = (body.accion ?? '').trim();

  if (!textoUsuario && !accion) {
    return json({ error: 'missing_instruction' }, 400);
  }
  if (textoUsuario.length > TOPE_INSTRUCCION) {
    return json({ error: 'instruction_too_long' }, 400);
  }

  // Componer el contexto del usuario (datos, nunca instrucciones del sistema)
  const partesContexto: string[] = [];
  if (accion && accion in PREAJUSTES) {
    partesContexto.push(`Objetivo de edición: ${PREAJUSTES[accion]}`);
  }
  if (textoUsuario) {
    partesContexto.push(`Petición específica del usuario: ${textoUsuario}`);
  }
  const contextoFinal = partesContexto.join('\n\n');

  // Solo se pide la miniatura tras validar que la petición es válida
  const foto = await miniaturaEnBase64(id, sesion.tokens.accessToken);
  if ('error' in foto) return foto.error;

  const visto = await verLaFoto({
    base,
    token: sesion.tokens.accessToken,
    locale: ctx.locals.locale,
    action: 'editar-foto',
    instruction: INSTRUCCION,
    context: contextoFinal,
    // `medium` para que el modelo razone sobre la exposición y colores de la imagen
    esfuerzo: 'medium',
    base64: foto.base64,
  });
  if ('error' in visto) return visto.error;

  const edicion = interpretarEdicionIA(visto.text);
  if (!edicion) return json({ error: 'ai_unreadable' }, 502);

  const pasos = convertirEdicionEnPasos(edicion);

  return json({
    ok: true,
    explicacion: edicion.explicacion,
    ajustes: edicion.ajustes,
    recorte: edicion.recorte,
    pasos,
  });
};

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);
