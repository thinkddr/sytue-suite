/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los COMENTARIOS de un fichero importado (.docx/.odt, y las notas de las celdas de un
 * .xlsx/.ods). El servicio los devuelve junto al contenido (`/import?comments=1`); en un texto
 * deja en el HTML un `<mark data-comment-id>` por cada uno con texto al que anclarse, y en una
 * hoja cada uno lleva su celda (`cell`), que el servicio guarda como su ancla. Quien importa
 * los guarda al crear el documento (`POST /documents/{id}/comments/import`, el autor es quien
 * importa, con la autoría original delante del texto) y SIEMPRE siembra el contenido de un
 * texto con `conservarMarks`: un mark sin comentario queda resaltado para siempre y no hay
 * forma de quitarlo desde la interfaz.
 */

import type { DocKind } from '@sytue/api-clients';

/** Un comentario del fichero, tal como lo devuelve el servicio (`domain.ImportedComment`). */
export interface ComentarioImportado {
  id: string;
  parent_id?: string;
  text: string;
  quote?: string;
  anchored: boolean;
  resolved?: boolean;
  /** La celda de un comentario de una hoja: la hoja (su id) y la identidad de su fila y de su
   *  columna (sheets/anclas.ts), más la referencia que se ve («B4»). */
  cell?: { hoja: string; fila: string; col: string; text: string };
}

/** Contenido convertido con sus comentarios (`ImportResult`). */
export interface Convertido {
  kind: DocKind;
  title?: string;
  content?: string;
  comments?: ComentarioImportado[];
  /** Contraseñas de protección del fichero, ya derivadas (solo su hash), por ámbito. */
  hashes?: Record<string, unknown>;
}

/** Ruta del servicio que guarda los comentarios de un documento recién creado. */
export const rutaImportarComentarios = (docId: string): string =>
  `/documents/${encodeURIComponent(docId)}/comments/import`;

/**
 * Quita del HTML los `<mark data-comment-id>` cuyo id no está en `guardados`, dejando su
 * contenido. Con un conjunto vacío (no se pudieron guardar los comentarios) no queda ningún
 * mark. Los ids son hex (los genera el servicio), así que el patrón no necesita escapes.
 */
export function conservarMarks(html: string, guardados: ReadonlySet<string>): string {
  return html.replace(
    /<mark data-comment-id="([^"]*)">([\s\S]*?)<\/mark>/g,
    (todo: string, id: string, dentro: string) => (guardados.has(id) ? todo : dentro),
  );
}

/**
 * Guarda los comentarios con `enviar` (que hace la petición, según el cliente) y devuelve el
 * contenido para sembrar: con los marks de los comentarios que quedaron guardados y sin los
 * demás. Si no hay comentarios, o `enviar` falla, no queda ningún mark. `fallo` dice si hubo
 * comentarios que no se guardaron, para avisar.
 */
export async function guardarComentarios(
  conv: Convertido,
  enviar: (comments: ComentarioImportado[]) => Promise<string[]>,
): Promise<{ content: string; fallo: boolean }> {
  const content = conv.content ?? '';
  const comments = conv.comments ?? [];
  // Solo un documento de texto lleva marks (el contenido de una hoja es JSON): los comentarios
  // de una hoja se anclan a su celda y el contenido va tal cual.
  if (conv.kind === 'spreadsheet') {
    if (comments.length === 0) return { content, fallo: false };
    try {
      await enviar(comments);
      return { content, fallo: false };
    } catch {
      return { content, fallo: true };
    }
  }
  if (conv.kind !== 'document') return { content, fallo: false };
  if (comments.length === 0) return { content: conservarMarks(content, new Set()), fallo: false };
  try {
    const anclados = new Set(await enviar(comments));
    return { content: conservarMarks(content, anclados), fallo: false };
  } catch {
    return { content: conservarMarks(content, new Set()), fallo: true };
  }
}
