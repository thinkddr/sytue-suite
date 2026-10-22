/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Subir fotos desde la galería. Los originales viven en Sytue Drive (decisión
 * de almacén de Sytue Photos), así que subir es DOS pasos: el fichero entra en la
 * carpeta «Fotos» del Drive de la persona y, con el id que devuelve Drive, el
 * servicio de Fotos lo indexa. Hasta ahora la galería no tenía ninguna puerta
 * de entrada: el servicio sabía indexar, pero nadie se lo pedía.
 *
 * Se sube de una en una y en orden: un carrete de doscientas fotos a la vez
 * abriría doscientas conexiones y el progreso saltaría a ciegas; así la barra
 * avanza de una en una y un fallo no se lleva a las demás.
 */
import { withBase } from '@sytue/ui';
import { indexar, type Foto } from './photosApi.js';

/** Carpeta del Drive donde van los originales que se suben desde aquí. */
export const CARPETA_FOTOS = '/Fotos';

/**
 * Sube UN fichero por el BFF y devuelve el id que le dio Drive.
 *
 * NO se habla con `drive.<dominio>` desde el navegador, aunque el SDK lo
 * permita: la página vive en el host de la organización, así que sería
 * cross-origin, y el preflight —que va sin credenciales— se lleva un 401 de
 * Drive antes de que salga un solo byte. Medido en producción el 2026-09-10.
 * El BFF tiene el token y habla con Drive por la red interna.
 */
async function subirADrive(f: File): Promise<string> {
  const res = await fetch(
    withBase(import.meta.env.BASE_URL, `/api/drive-subir?nombre=${encodeURIComponent(f.name)}`),
    {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': f.type || 'application/octet-stream' },
      body: f,
    },
  );
  if (!res.ok) throw new Error(`Drive respondió ${res.status}`);
  const { id } = (await res.json()) as { id?: string };
  if (!id) throw new Error('Drive no devolvió el id del fichero');
  return id;
}

export function esImagen(f: File): boolean {
  return f.type.startsWith('image/');
}

export interface ResultadoSubida {
  fotos: Foto[];
  /** Nombres de los ficheros que no se pudieron subir o indexar. */
  fallos: string[];
}

export async function subirFotos(
  ficheros: readonly File[],
  onProgreso: (hechas: number, total: number) => void,
): Promise<ResultadoSubida> {
  const fotos: Foto[] = [];
  const fallos: string[] = [];
  let hechas = 0;
  for (const f of ficheros) {
    try {
      fotos.push(await indexar(await subirADrive(f)));
    } catch {
      fallos.push(f.name);
    }
    onProgreso(++hechas, ficheros.length);
  }
  return { fotos, fallos };
}
