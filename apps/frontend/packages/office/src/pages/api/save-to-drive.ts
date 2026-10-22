/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/save-to-drive?id=<documento>  → { name, path }
 *
 * DEVOLVER AL FICHERO DE DRIVE los cambios del documento que salió de él. Cierra
 * el viaje que abre /open-from-drive: allí el .docx se convierte en documento
 * editable; aquí el documento vuelve a ser el .docx, en su sitio y con su nombre.
 * Sin esto, editar un fichero de Drive en Sytue Docs dejaba el original intacto y
 * los cambios en un documento aparte — que es justo lo que nadie espera.
 *
 * SOBRESCRIBIR AQUÍ ES LO CORRECTO (y no lo es en otros sitios): es el MISMO
 * fichero, y Drive no pisa nada: guarda una versión nueva y conserva la anterior
 * en el historial. Quien se arrepienta la recupera desde «Versiones».
 *
 * SOLO LOS FORMATOS CON IDA Y VUELTA. Un .pdf o un .txt se pueden abrir pero no
 * se pueden volver a serializar con su forma original, así que devolverlos
 * significaría cambiar el fichero de formato a espaldas de su dueño: se rechaza
 * con `unsupported_format` y la acción ni siquiera se ofrece en el editor.
 */
import type { APIRoute } from 'astro';
import { DocsClient, DriveClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/**
 * Extensión del fichero de origen → familia de exportación de Docs. El servicio
 * elige dentro de la familia según el tipo de documento (texto→docx, hoja→xlsx,
 * presentación→pptx), así que basta con acertar la familia.
 */
const FORMAT_BY_EXT: Record<string, string> = {
  docx: 'ooxml',
  xlsx: 'ooxml',
  pptx: 'ooxml',
  odt: 'odf',
  ods: 'odf',
  odp: 'odf',
  md: 'markdown',
  markdown: 'markdown',
  html: 'html',
  htm: 'html',
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = new URL(ctx.request.url).searchParams.get('id');
  if (!id) return json({ error: 'missing_id' }, 400);

  const auth = bearer(session.tokens.accessToken);
  const docs = new DocsClient({ baseUrl: docsBaseUrl(), auth });
  const drive = new DriveClient({ baseUrl: driveBaseUrl(), auth });

  try {
    const { document: doc } = await docs.get(id);
    const origen = doc.sourceRef ?? '';
    if (!origen.startsWith('drive:')) return json({ error: 'not_from_drive' }, 400);
    const fileId = origen.slice('drive:'.length);

    // El nombre y la ruta se leen de Drive, no del título del documento: si lo
    // han renombrado o movido allí, los cambios tienen que ir a donde está ahora.
    const file = await drive.statById(fileId);
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    const format = FORMAT_BY_EXT[ext];
    if (!format) return json({ error: 'unsupported_format' }, 400);

    const blob = await docs.createExport(id, format);
    const { bytes, contentType } = await docs.downloadExport(id, blob.id);
    await drive.upload(file.path, bytes, blob.contentType || contentType);
    return json({ name: file.name, path: file.path });
  } catch {
    return json({ error: 'save_failed' }, 502);
  }
};
