/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El nombre con el que se descarga un documento exportado. Vive aparte porque lo
 * usan los DOS sitios que exportan: el editor abierto y la portada, que descarga
 * sin abrir nada.
 */
import type { DocKind } from '@sytue/api-clients';
import type { ExportFormat } from './types';

/** Extensión de cada formato para el tipo de documento; con `plantilla`, la de la
 *  plantilla de Office u OpenDocument (solo ooxml y odf la tienen). */
export function exportExt(kind: DocKind, format: ExportFormat, plantilla = false): string {
  if (plantilla && format === 'ooxml') {
    return { document: 'dotx', spreadsheet: 'xltx', presentation: 'potx' }[kind];
  }
  if (plantilla && format === 'odf') {
    return { document: 'ott', spreadsheet: 'ots', presentation: 'otp' }[kind];
  }
  switch (format) {
    case 'pdf':
      // Faltaba, y el `default` daba «.bin»: el PDF —lo que más se descarga— salía
      // con una extensión que ningún sistema sabe abrir.
      return 'pdf';
    case 'html':
      return 'html';
    case 'markdown':
      return 'md';
    case 'odf':
      return { document: 'odt', spreadsheet: 'ods', presentation: 'odp' }[kind];
    case 'ooxml':
      return { document: 'docx', spreadsheet: 'xlsx', presentation: 'pptx' }[kind];
    default:
      return 'bin';
  }
}

/** Nombre de descarga: título saneado + extensión del formato. */
export function exportFilename(
  title: string,
  kind: DocKind,
  format: ExportFormat,
  plantilla = false,
): string {
  const base = (title.trim() || 'documento').replace(/[\\/:*?"<>|]+/g, '_').slice(0, 120);
  return `${base}.${exportExt(kind, format, plantilla)}`;
}

const CLAVE_PDF_ACCESIBLE = 'sytue.docs.pdfAccesible';

/**
 * «PDF accesible» del menú Archivo: preferencia de quien descarga, por navegador. Por
 * defecto encendido (PDF/UA-1); apagado pide el PDF simple de catorce fuentes estándar.
 */
export function pdfAccesible(): boolean {
  try {
    return localStorage.getItem(CLAVE_PDF_ACCESIBLE) !== '0';
  } catch {
    return true; // ventana privada o almacenamiento bloqueado: el valor por defecto
  }
}

export function fijarPdfAccesible(activo: boolean): void {
  try {
    if (activo) localStorage.removeItem(CLAVE_PDF_ACCESIBLE);
    else localStorage.setItem(CLAVE_PDF_ACCESIBLE, '0');
  } catch {
    // sin almacenamiento la preferencia dura lo que la página
  }
}

/** Cuerpo de la petición de exportación (`/api/export`). */
export function cuerpoExport(
  format: ExportFormat,
  plantilla: boolean,
  accesible: boolean,
): { format: ExportFormat; template?: true; accessible?: false } {
  return {
    format,
    ...(plantilla ? { template: true as const } : {}),
    ...(format === 'pdf' && !accesible ? { accessible: false as const } : {}),
  };
}

/**
 * Descarga el documento `id` en `format`. El BFF hace de proxy del blob, así que
 * se baja por object URL + `<a download>`: same-origin y sin depender de la URL
 * de S3. Lanza si el servicio lo rechaza.
 */
export async function descargarExport(
  id: string,
  title: string,
  kind: DocKind,
  format: ExportFormat,
  plantilla = false,
): Promise<void> {
  const filename = exportFilename(title, kind, format, plantilla);
  const res = await fetch(
    `/api/export?id=${encodeURIComponent(id)}&filename=${encodeURIComponent(filename)}`,
    {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(cuerpoExport(format, plantilla, pdfAccesible())),
    },
  );
  if (!res.ok) throw new Error(`export ${res.status}`);
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
