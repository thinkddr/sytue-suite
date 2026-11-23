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
 *
 * CON EL DRIVE EN LA BÓVEDA (ADR 0086) el Drive del servidor no admite escrituras
 * y el BFF contesta 409 `drive_private_vault`. Entonces esa foto y las que quedan
 * van cifradas en el navegador a «Fotos» del Drive de la bóveda
 * (`driveCifrado.ts`), sin pasar por el índice del servidor: no hay nada en claro
 * que indexar.
 */
import { withBase } from '@sytue/ui';
import { ERROR_DRIVE_CIFRADO } from '../lib/unidadCifrada.js';
import { motivoDriveCifrado, type MotivoDriveCifrado } from './driveCifrado.js';
import { ErrorDeFotos, indexar, type Foto } from './photosApi.js';

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
  if (res.status === 409) {
    const cuerpo = (await res.json().catch(() => null)) as { error?: unknown } | null;
    if (cuerpo?.error === ERROR_DRIVE_CIFRADO) throw new DriveEnBoveda();
  }
  if (!res.ok) throw new ErrorDeFotos(res.status);
  const { id } = (await res.json()) as { id?: string };
  // Sin id no hay qué indexar: el BFF contestó algo que no es lo pactado (502).
  if (!id) throw new ErrorDeFotos(502);
  return id;
}

/** El 409 del BFF que dice que el Drive de esta persona está en la bóveda. */
export class DriveEnBoveda extends ErrorDeFotos {
  constructor() {
    super(409);
    this.name = 'DriveEnBoveda';
  }
}

/** Dónde se suben las fotos cifradas: lo abre `driveCifrado.ts` la primera vez. */
export interface DestinoCifrado {
  subir: (fichero: File) => Promise<unknown>;
}

export function esImagen(f: File): boolean {
  return f.type.startsWith('image/');
}

export interface ResultadoSubida {
  fotos: Foto[];
  /** Nombres de los ficheros que no se pudieron subir o indexar. */
  fallos: string[];
  /** Nombres de las que quedaron cifradas en el Drive de la bóveda (no van al índice). */
  cifradas: string[];
  /** Por qué no se pudo abrir el Drive de la bóveda, si hizo falta y no se pudo. */
  motivoCifrado?: MotivoDriveCifrado;
}

export async function subirFotos(
  ficheros: readonly File[],
  onProgreso: (hechas: number, total: number) => void,
  opciones: {
    /** Abre (una vez) el destino cifrado. Sin él, la bóveda cuenta como fallo. */
    cifrado?: () => Promise<DestinoCifrado>;
  } = {},
): Promise<ResultadoSubida> {
  const resultado: ResultadoSubida = { fotos: [], fallos: [], cifradas: [] };
  // `servidor` hasta el primer 409 de la bóveda; luego todo va cifrado (o falla con
  // su motivo, sin volver a intentar abrir la bóveda foto a foto).
  let modo: 'servidor' | 'cifrado' = 'servidor';
  let destino: Promise<DestinoCifrado> | undefined;
  let hechas = 0;
  for (const f of ficheros) {
    try {
      if (modo === 'servidor') {
        try {
          resultado.fotos.push(await indexar(await subirADrive(f)));
        } catch (e) {
          if (!(e instanceof DriveEnBoveda)) throw e;
          modo = 'cifrado';
        }
      }
      if (modo === 'cifrado') {
        if (!opciones.cifrado) throw new DriveEnBoveda();
        destino ??= opciones.cifrado();
        let abierto: DestinoCifrado;
        try {
          abierto = await destino;
        } catch (e) {
          resultado.motivoCifrado = motivoDriveCifrado(e);
          throw e;
        }
        await abierto.subir(f);
        resultado.cifradas.push(f.name);
      }
    } catch {
      resultado.fallos.push(f.name);
    }
    onProgreso(++hechas, ficheros.length);
  }
  return resultado;
}
