/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El Drive de la persona en la bóveda post-cuántica (ADR 0086), visto desde la
 * galería. Cuando Drive ya está en la bóveda, el Drive del servidor está congelado y
 * `/api/drive-subir` contesta 409 `drive_private_vault`: el original no puede ir ahí
 * en claro. Entonces la galería lo sube como lo sube Drive web: cifrado en el
 * navegador, a la carpeta «Fotos» del Drive de la bóveda, con el MISMO repositorio
 * (`PrivateDriveRepository`) y la misma puerta (`@sytue/offline/vault/drive`).
 *
 * Lo que decide esto, y por qué:
 *
 * - **Nada al índice del servidor.** syphotos indexa leyendo el original del Drive
 *   del servidor y genera ahí las miniaturas; con la foto en la bóveda no puede
 *   leerla, y darle una entrada con el id de la bóveda sería darle metadatos (fecha,
 *   nombre, tamaño) que hoy no tiene. Así que las fotos cifradas entran en la línea
 *   de tiempo, los álbumes y la búsqueda EN EL CLIENTE, desde su propio índice
 *   cifrado (`galeriaCifrada.ts`, `FotosCifradas.tsx`).
 * - **El índice es del cliente.** La galería cifrada (`galeriaCifrada.ts`) guarda
 *   fecha de captura, medidas, favorita, álbumes y una miniatura cifrada como
 *   ficheros del propio Drive de la bóveda: nada en claro sale del dispositivo.
 * - **Nunca se importa ni se congela nada desde aquí.** La bóveda se abre solo si ya
 *   existe (`existing`) y con el recibo de la migración (`importar: false`); si
 *   Drive aún está pasando a la bóveda, se dice («ábrelo en Drive»), no se hace.
 * - **No se pisa ninguna foto.** Subir con un nombre que ya está crearía una
 *   versión nueva de ese fichero; aquí, como con `on_conflict=keep_both` en el
 *   servidor, la nueva se llama «IMG_0001 (2).jpg».
 */
import { DriveClient } from '@sytue/api-clients';
import type { MatrixStorageScope } from '@sytue/config/matrix';
import {
  PrivateDriveError,
  safeAttachmentType,
  type PrivateDriveRepository,
} from '@sytue/offline/matrix';
import type { PqVault, PqVaultOpenOptions } from '@sytue/offline/vault';
import {
  collectPrivateDriveFile,
  consumerPrivateDriveFailure,
  freePrivateDriveName,
  openConsumerPrivateDrive,
  privateDriveFolderEntries,
  privateDriveRootFolder,
  type PrivateDriveSource,
} from '@sytue/offline/vault/drive';
import { withBase } from '@sytue/ui';

/** El prefijo de las rutas del BFF de Photos para la bóveda de Drive. */
export const RUTA_DRIVE_CIFRADO = '/api/drive-cifrado';
/** La carpeta de los originales: la misma que en el Drive del servidor. */
export const CARPETA_FOTOS_CIFRADAS = 'Fotos';
/** Lo más grande que se descifra para enseñarlo en la galería. */
export const MAXIMO_A_LA_VISTA = 64 * 1024 * 1024;

/**
 * Por qué no se pudo usar el Drive cifrado: `entrar` es volver a entrar para
 * desbloquearlo; `pendiente`, que Drive aún no ha terminado de pasar a la bóveda (o
 * no la tiene); `fallo`, cualquier otra cosa.
 */
export type MotivoDriveCifrado = 'entrar' | 'pendiente' | 'fallo';

export function motivoDriveCifrado(error: unknown): MotivoDriveCifrado {
  const motivo = consumerPrivateDriveFailure(error);
  return motivo === 'signin' ? 'entrar' : motivo === 'pending' ? 'pendiente' : 'fallo';
}

/** Lo que la galería usa del repositorio de Drive de la bóveda. */
export type RepositorioFotos = Pick<
  PrivateDriveRepository,
  'root' | 'children' | 'createFolder' | 'upload' | 'file' | 'download'
>;

/** El Drive de la bóveda abierto para la galería. `cerrar` lo suelta todo. */
export interface DriveCifrado {
  readonly repositorio: RepositorioFotos;
  readonly signal: AbortSignal;
  cerrar: () => Promise<void>;
}

const baseDelBff = (): string => withBase(import.meta.env.BASE_URL, RUTA_DRIVE_CIFRADO);

/** El cliente de Drive del navegador sobre las rutas de la bóveda de Drive de Photos. */
function fuenteDelNavegador(): PrivateDriveSource {
  return new DriveClient({ baseUrl: window.location.origin + baseDelBff() });
}

/**
 * ¿El Drive del servidor de esta persona ya no admite escrituras (está, o está
 * pasando, a la bóveda)? Solo lee el estado de la fuente, sin claves. Ante la duda
 * (sin organización, error de red) contesta que no: la galería sigue como siempre.
 */
export async function driveEnBoveda(
  actor: string,
  tenant: string | undefined,
  fuente: () => Pick<PrivateDriveSource, 'privateStorageState'> = fuenteDelNavegador,
  signal?: AbortSignal,
): Promise<boolean> {
  if (!tenant) return false;
  const scope: MatrixStorageScope = { tenant, subject: actor, product: 'drive' };
  try {
    return (await fuente().privateStorageState(scope, signal)).mode !== 'legacy';
  } catch {
    return false;
  }
}

/**
 * Abre el Drive de la bóveda para la galería: solo si ya existe y tiene el recibo
 * de la migración. Lanza lo que haya que contar (`motivoDriveCifrado`).
 */
export async function abrirDriveCifrado(options: {
  actor: string;
  tenant?: string | undefined;
  signal?: AbortSignal | undefined;
  /** Sustituibles en pruebas. */
  abrir?: (options: PqVaultOpenOptions) => Promise<PqVault>;
  fuente?: () => PrivateDriveSource;
  base?: string;
}): Promise<DriveCifrado> {
  const drive = await openConsumerPrivateDrive({
    actor: options.actor,
    tenant: options.tenant,
    basePath: options.base ?? baseDelBff(),
    source: options.fuente ?? fuenteDelNavegador,
    signal: options.signal,
    open: options.abrir,
  });
  return { repositorio: drive.repository, signal: drive.signal, cerrar: drive.close };
}

/** `nombre`, o «nombre (2).ext» y siguientes si ya está cogido (como `nombreLibre` en Go). */
export const nombreLibre = freePrivateDriveName;

const texto = (fila: Record<string, unknown>, clave: string): string => {
  const valor = fila[clave];
  return typeof valor === 'string' ? valor : '';
};

/** Todos los hijos de una carpeta. */
async function hijos(repo: RepositorioFotos, id: string, signal?: AbortSignal) {
  const { folders, files } = await privateDriveFolderEntries(repo, id, signal);
  return { carpetas: folders, ficheros: files };
}

/** La carpeta «Fotos» de la raíz: su id, o `null` si no está y no se pide crearla. */
export function carpetaFotos(
  repo: RepositorioFotos,
  crear: boolean,
  signal?: AbortSignal,
): Promise<string | null> {
  return privateDriveRootFolder(repo, CARPETA_FOTOS_CIFRADAS, crear, signal);
}

/**
 * Sube fotos a «Fotos» del Drive de la bóveda, sin pisar ninguna. `alSubir` apunta
 * cada una en la galería cifrada (`galeriaCifrada.ts`); si falla, la foto ya está a
 * salvo y su entrada se reconstruye al leer la galería.
 */
export function destinoCifrado(
  drive: Pick<DriveCifrado, 'repositorio' | 'signal'>,
  alSubir?: (fila: Record<string, unknown>, fichero: File) => Promise<unknown>,
) {
  let preparado: Promise<{ carpeta: string; nombres: Set<string> }> | undefined;
  const preparar = async () => {
    const carpeta = await carpetaFotos(drive.repositorio, true, drive.signal);
    if (!carpeta) throw new PrivateDriveError('not_found');
    const { carpetas, ficheros } = await hijos(drive.repositorio, carpeta, drive.signal);
    return {
      carpeta,
      nombres: new Set([...carpetas, ...ficheros].map((f) => texto(f, 'name'))),
    };
  };
  return {
    /** Devuelve el nombre con el que quedó. */
    subir: async (fichero: File): Promise<string> => {
      preparado ??= preparar();
      let destino;
      try {
        destino = await preparado;
      } catch (error) {
        // Que el siguiente intento vuelva a buscar la carpeta.
        preparado = undefined;
        throw error;
      }
      const nombre = nombreLibre(fichero.name.trim(), destino.nombres);
      const fila = await drive.repositorio.upload(
        destino.carpeta,
        nombre,
        fichero.type || 'application/octet-stream',
        fichero,
        drive.signal,
      );
      const final = texto(fila, 'name') || nombre;
      destino.nombres.add(final);
      if (alSubir) await alSubir(fila, fichero).catch(() => {});
      return final;
    },
  };
}

/** Una foto de «Fotos» del Drive de la bóveda, como la enseña la galería. */
export interface FotoCifrada {
  id: string;
  nombre: string;
  tipo: string;
  bytes: number;
  fecha: string;
}

/** Las imágenes de «Fotos» del Drive de la bóveda, de la más reciente a la más antigua. */
export async function fotosCifradas(
  repo: RepositorioFotos,
  signal?: AbortSignal,
): Promise<FotoCifrada[]> {
  const carpeta = await carpetaFotos(repo, false, signal);
  if (!carpeta) return [];
  const { ficheros } = await hijos(repo, carpeta, signal);
  return ficheros
    .map((f) => ({
      id: texto(f, 'id'),
      nombre: texto(f, 'name'),
      tipo: texto(f, 'content_type'),
      bytes: typeof f.size_bytes === 'number' ? f.size_bytes : 0,
      fecha: texto(f, 'updated_at'),
    }))
    .filter((f) => f.id && f.tipo.startsWith('image/'))
    .sort((a, b) => (a.fecha < b.fecha ? 1 : a.fecha > b.fecha ? -1 : a.id < b.id ? -1 : 1));
}

/** Descifra una foto en memoria (solo tras el compromiso autenticado completo). */
export async function leerFotoCifrada(
  repo: RepositorioFotos,
  foto: Pick<FotoCifrada, 'id'>,
  signal?: AbortSignal,
): Promise<Blob> {
  const fila = await repo.file(foto.id, signal);
  return collectPrivateDriveFile({
    size: fila.size_bytes,
    maximum: MAXIMO_A_LA_VISTA,
    type: safeAttachmentType(typeof fila.content_type === 'string' ? fila.content_type : ''),
    check: () => signal?.throwIfAborted(),
    run: (sink) => repo.download(foto.id, sink, signal),
  });
}

/** En qué está el Drive de la bóveda para la galería. */
export type EstadoDriveCifrado =
  | { fase: 'cerrado' }
  | { fase: 'abriendo' }
  | { fase: 'abierto'; drive: DriveCifrado }
  | { fase: 'fallo'; motivo: MotivoDriveCifrado };

/**
 * Una apertura a la vez del Drive de la bóveda, y su cierre: quien pida abrirlo
 * mientras se abre espera a la misma, y lo que termine de abrirse después de cerrar
 * se suelta en vez de quedarse vivo. Vive fuera de los hooks para no mutar refs.
 */
export class AperturaDriveCifrado {
  private abriendo: Promise<DriveCifrado> | null = null;
  private vigente: DriveCifrado | null = null;
  private cerrada = false;
  constructor(
    private readonly abrirDrive: () => Promise<DriveCifrado>,
    private readonly alCambiar: (estado: EstadoDriveCifrado) => void,
  ) {}

  abrir(): Promise<DriveCifrado> {
    if (this.cerrada) return Promise.reject(new PrivateDriveError('closed'));
    if (this.vigente) return Promise.resolve(this.vigente);
    if (this.abriendo) return this.abriendo;
    this.alCambiar({ fase: 'abriendo' });
    const pendiente: Promise<DriveCifrado> = this.abrirDrive().then(
      async (drive) => {
        if (this.cerrada || this.abriendo !== pendiente) {
          await drive.cerrar().catch(() => {});
          throw new PrivateDriveError('closed');
        }
        this.abriendo = null;
        this.vigente = drive;
        this.alCambiar({ fase: 'abierto', drive });
        return drive;
      },
      (error: unknown) => {
        if (!this.cerrada && this.abriendo === pendiente) {
          this.abriendo = null;
          this.alCambiar({ fase: 'fallo', motivo: motivoDriveCifrado(error) });
        }
        throw error;
      },
    );
    this.abriendo = pendiente;
    return pendiente;
  }

  async cerrar(): Promise<void> {
    this.cerrada = true;
    this.abriendo = null;
    const drive = this.vigente;
    this.vigente = null;
    await drive?.cerrar();
  }
}

/**
 * Las miniaturas descifradas de una apertura: URL en memoria que se sueltan juntas,
 * a mano o cuando se cierra el Drive del que salieron (`signal`).
 */
export class MiniaturasCifradas {
  private readonly urls = new Map<string, string>();
  private soltadas = false;
  constructor(signal?: AbortSignal) {
    if (signal?.aborted) this.soltadas = true;
    else signal?.addEventListener('abort', () => this.soltar(), { once: true });
  }
  tiene(id: string): boolean {
    return this.urls.has(id);
  }
  poner(id: string, blob: Blob): void {
    if (this.soltadas || this.urls.has(id)) return;
    this.urls.set(id, URL.createObjectURL(blob));
  }
  todas(): Record<string, string> {
    return Object.fromEntries(this.urls);
  }
  soltar(): void {
    this.soltadas = true;
    for (const url of this.urls.values()) URL.revokeObjectURL(url);
    this.urls.clear();
  }
}
