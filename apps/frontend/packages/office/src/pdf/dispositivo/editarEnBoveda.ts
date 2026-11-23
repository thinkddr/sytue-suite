/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GUARDAR EN LA BÓVEDA LO EDITADO EN EL DISPOSITIVO (ADR 0086, tercera entrega).
 *
 * Lo que el estudio de la bóveda anota, rellena u organiza con el motor local
 * (`motorPdf.ts`) vuelve CIFRADO al Drive de la bóveda por el mismo repositorio que el
 * firmado (`firmarEnBoveda.ts`), y de una de dos formas, nunca pisando un nombre:
 *
 * - como VERSIÓN NUEVA del mismo fichero: mismo nombre y misma carpeta, y el
 *   repositorio la sube como versión (la anterior queda en su historial), como hace el
 *   estudio del servidor al aplicar un cambio;
 * - como fichero NUEVO a su lado, «nombre (sufijo).pdf» con un nombre libre
 *   (`freePrivateDriveName`): combinar, o cuando la persona prefiere no tocar el
 *   original.
 */
import { freePrivateDriveName, privateDriveFolderEntries } from '@sytue/offline/vault/drive';
import { ErrorPdf } from './pdfObjetos';
import { bajarPdf, nombreFirmado, type RepositorioFirma } from './firmarEnBoveda';

export interface GuardadoEnBoveda {
  nombre: string;
  id: string;
  /** ¿Fichero nuevo (y no una versión nueva del mismo)? */
  nuevo: boolean;
}

const texto = (fila: Record<string, unknown>, clave: string): string => {
  const v = fila[clave];
  return typeof v === 'string' ? v : '';
};

/**
 * Sube `datos` cifrado a la bóveda: versión nueva del fichero `id` o, con `sufijo`,
 * fichero nuevo en su carpeta. Los bytes son de quien llama: no se borran aquí.
 */
export async function guardarPdfEnBoveda(options: {
  repositorio: RepositorioFirma;
  id: string;
  datos: Uint8Array<ArrayBuffer>;
  /** Con él, un fichero nuevo «nombre (sufijo).pdf»; sin él, una versión nueva. */
  sufijo?: string | undefined;
  signal?: AbortSignal | undefined;
}): Promise<GuardadoEnBoveda> {
  const { repositorio, id, signal } = options;
  const fila = await repositorio.file(id, signal);
  const carpeta = texto(fila, 'folder_id');
  const nombreActual = texto(fila, 'name');
  if (!carpeta || !nombreActual) throw new ErrorPdf('ilegible', 'sin carpeta');
  let nombre = nombreActual;
  if (options.sufijo) {
    const { folders, files } = await privateDriveFolderEntries(repositorio, carpeta, signal);
    nombre = freePrivateDriveName(
      nombreFirmado(nombreActual, options.sufijo),
      new Set([...folders, ...files].map((f) => texto(f, 'name'))),
    );
  }
  signal?.throwIfAborted();
  const nueva = await repositorio.upload(
    carpeta,
    nombre,
    'application/pdf',
    new Blob([options.datos], { type: 'application/pdf' }),
    signal,
  );
  return {
    nombre: texto(nueva, 'name') || nombre,
    id: texto(nueva, 'id') || id,
    nuevo: Boolean(options.sufijo),
  };
}

/** Baja y descifra en memoria el PDF `id` de la bóveda (para combinarlo con otro). */
export async function descifrarPdfDeBoveda(
  repositorio: Pick<RepositorioFirma, 'file' | 'download'>,
  id: string,
  signal?: AbortSignal,
): Promise<Uint8Array<ArrayBuffer>> {
  return bajarPdf(repositorio, await repositorio.file(id, signal), signal);
}
