/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lectores de datos de los scripts, PUROS: convierten lo que guarda el editor (HTML de
 * celdas y de folio, modelo de diapositivas) en los datos planos que `sytue.v1.read.*`
 * puede devolver. Quitan el marcado y nunca devuelven HTML: lo que el script recibe es
 * texto, no algo que se pueda inyectar en ningún sitio.
 */

/** Texto plano de un trozo de HTML sin ejecutar nada: sin DOM, con un recorte de etiquetas. */
export function textoPlano(html: string): string {
  let texto = removeScripts(html);
  texto = texto
    .replace(/<\s*(br|\/p|\/div|\/h[1-6]|\/li|\/tr)\b[^>]*>/gi, '\n')
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&amp;/g, '&');
  return removeScripts(texto)
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

function removeScripts(input: string): string {
  let texto = input;
  let anterior: string;
  do {
    anterior = texto;
    texto = texto.replace(/<(script|style)\b[\s\S]*?<\/\1\s*>/gi, '');
  } while (texto !== anterior);
  return texto;
}

interface CajaMinima {
  html?: unknown;
  text?: unknown;
}

/** Diapositivas como `{ index, text }` desde el contenido guardado (JSON de `SlidesModel`). */
export function diapositivasTexto(contenido: string): { index: number; text: string }[] | null {
  let m: unknown;
  try {
    m = JSON.parse(contenido);
  } catch {
    return null;
  }
  const slides = (m as { slides?: unknown })?.slides;
  if (!Array.isArray(slides)) return null;
  return slides.slice(0, 500).map((s, i) => {
    const cajas = Array.isArray((s as { boxes?: unknown })?.boxes)
      ? (s as { boxes: CajaMinima[] }).boxes
      : [];
    const texto = cajas
      .map((c) =>
        typeof c?.html === 'string'
          ? textoPlano(c.html)
          : typeof c?.text === 'string'
            ? c.text
            : '',
      )
      .filter(Boolean)
      .join('\n');
    return { index: i + 1, text: texto.slice(0, 20_000) };
  });
}

/** Valores de un rango como escalares (la celda vacía es `null`). */
export function escalares(
  valores: readonly (readonly unknown[])[],
): (string | number | boolean | null)[][] {
  return valores.map((fila) =>
    fila.map((v) =>
      typeof v === 'number' && Number.isFinite(v)
        ? v
        : typeof v === 'boolean'
          ? v
          : typeof v === 'string'
            ? v.slice(0, 4096)
            : null,
    ),
  );
}
