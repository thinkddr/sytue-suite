/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «PDF en blanco»: crear el fichero y abrirlo. Sytue CertiPDF se abre por RUTA
 * de Drive, no por identificador de documento, así que empezar de cero es subir
 * un PDF de una página (pdf/blanco.ts) por `/api/pdf-abrir` —el mismo endpoint
 * que usa «abrir un PDF del ordenador»: ya crea la carpeta /PDF, evita
 * colisiones de nombre y valida que lo que llega es un PDF— y navegar a él.
 * Lo comparten la portada conjunta de Docs y la de CertiPDF.
 */
import { pdfEnBlanco } from './blanco';

/** Crea el PDF con ese nombre y devuelve su ruta en Drive. Lanza si falla. */
export async function crearPdfEnBlanco(nombre: string): Promise<string> {
  const res = await fetch(`/api/pdf-abrir?name=${encodeURIComponent(nombre)}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/octet-stream' },
    body: new Blob([pdfEnBlanco()]),
  });
  if (!res.ok) throw new Error(`pdf-abrir: ${res.status}`);
  const { path } = (await res.json()) as { path: string };
  if (!path) throw new Error('pdf-abrir: respuesta sin path');
  return path;
}
