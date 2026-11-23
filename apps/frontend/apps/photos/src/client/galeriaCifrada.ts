/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La galería cifrada (ADR 0086): el índice de las fotos que viven cifradas en
 * «Fotos» del Drive de la bóveda, escrito y leído SOLO en el navegador. Es el mismo
 * formato que el nativo (`apps/nucleo/photos/galeriacifrada`, que tiene el porqué
 * largo); los dos leen los ficheros de `testdata` de allí en sus pruebas.
 *
 *   Fotos/.sytue-galeria/indice/<id>.json      una entrada por foto
 *   Fotos/.sytue-galeria/miniaturas/<id>.jpg   su miniatura (hecha aquí)
 *   Fotos/.sytue-galeria/albumes/<id>.json     un registro por álbum
 *
 * Son ficheros del propio Drive de la bóveda: van cifrados como la foto, el servidor
 * solo ve bloques opacos y escribir otra vez una entrada crea una versión, no borra
 * la anterior. Una entrada por foto (no un índice entero) para que dos dispositivos
 * que tocan fotos distintas no se pisen; cada cambio relee la entrada y aplica solo
 * lo suyo, así que repetirlo no cambia nada. Si falta o no vale, la foto sale con lo
 * que dice Drive y se reconstruye desde el original.
 */
import { BYTES_DE_CABECERA, leerExif } from '../lib/exif.js';
import {
  CARPETA_FOTOS_CIFRADAS,
  carpetaFotos,
  leerFotoCifrada,
  MAXIMO_A_LA_VISTA,
  type RepositorioFotos,
} from './driveCifrado.js';

export const CARPETA_GALERIA = '.sytue-galeria';
export const CARPETA_INDICE = 'indice';
export const CARPETA_MINIATURAS = 'miniaturas';
export const CARPETA_ALBUMES = 'albumes';
export const VERSION_INDICE = 1;
export const TIPO_FOTO = 'sytue.photos.cifrada.foto';
export const TIPO_ALBUM = 'sytue.photos.cifrada.album';
export const LADO_MINIATURA = 512;
const MAX_ENTRADA = 64 * 1024;
const MAX_ALBUMES_POR_FOTO = 1000;
const MAX_TITULO = 200;
const LECTORES = 4;

export interface EntradaFoto {
  v: 1;
  tipo: typeof TIPO_FOTO;
  foto: string;
  suma: string;
  nombre: string;
  tomada: string;
  exif: boolean;
  ancho: number;
  alto: number;
  favorita: boolean;
  albumes: string[];
  miniatura: string;
  actualizada: string;
}

export interface AlbumCifrado {
  v: 1;
  tipo: typeof TIPO_ALBUM;
  id: string;
  titulo: string;
  creado: string;
  actualizado: string;
}

/** Una foto de «Fotos» con lo que el índice sabe de ella. */
export interface FotoDeGaleria {
  id: string;
  nombre: string;
  tipo: string;
  bytes: number;
  suma: string;
  entrada: EntradaFoto;
  /** Hay una entrada válida y suya. */
  conEntrada: boolean;
  /** Sin entrada, de otro contenido o sin miniatura: se puede reconstruir. */
  pendiente: boolean;
}

export interface IndiceCifrado {
  /** De la más reciente a la más antigua por fecha de captura. */
  fotos: FotoDeGaleria[];
  /** Por título. */
  albumes: AlbumCifrado[];
}

/** Lo que el navegador saca de una imagen: medidas ya orientadas y la miniatura. */
export type Procesar = (
  imagen: Blob,
) => Promise<{ ancho: number; alto: number; miniatura: Blob } | null>;

const ID = /^[A-Za-z0-9_-]{1,128}$/;
const esId = (v: unknown): v is string => typeof v === 'string' && ID.test(v);
const esFecha = (v: unknown): v is string =>
  typeof v === 'string' && v.length <= 64 && !Number.isNaN(Date.parse(v));
const entero = (v: unknown) =>
  typeof v === 'number' && Number.isInteger(v) && v >= 0 && v <= 1 << 20;

const sinRepetir = (ids: readonly string[]) => [...new Set(ids)].sort();

/** Lee y valida una entrada; `null` si no sigue el formato. */
export function decodificarEntrada(texto: string): EntradaFoto | null {
  if (texto.length > MAX_ENTRADA) return null;
  let v: Record<string, unknown>;
  try {
    v = JSON.parse(texto) as Record<string, unknown>;
  } catch {
    return null;
  }
  if (!v || typeof v !== 'object') return null;
  const albumes = v.albumes ?? [];
  if (
    v.v !== VERSION_INDICE ||
    v.tipo !== TIPO_FOTO ||
    !esId(v.foto) ||
    (v.miniatura !== undefined && v.miniatura !== '' && !esId(v.miniatura)) ||
    typeof (v.nombre ?? '') !== 'string' ||
    typeof (v.suma ?? '') !== 'string' ||
    !esFecha(v.tomada) ||
    !entero(v.ancho ?? 0) ||
    !entero(v.alto ?? 0) ||
    !Array.isArray(albumes) ||
    albumes.length > MAX_ALBUMES_POR_FOTO ||
    !albumes.every(esId)
  )
    return null;
  return {
    v: 1,
    tipo: TIPO_FOTO,
    foto: v.foto,
    suma: (v.suma as string | undefined) ?? '',
    nombre: (v.nombre as string | undefined) ?? '',
    tomada: new Date(v.tomada).toISOString(),
    exif: v.exif === true,
    ancho: (v.ancho as number | undefined) ?? 0,
    alto: (v.alto as number | undefined) ?? 0,
    favorita: v.favorita === true,
    albumes: sinRepetir(albumes),
    miniatura: typeof v.miniatura === 'string' ? v.miniatura : '',
    actualizada: esFecha(v.actualizada) ? new Date(v.actualizada).toISOString() : '',
  };
}

/** Lee y valida un álbum; `null` si no sigue el formato. */
export function decodificarAlbum(texto: string): AlbumCifrado | null {
  if (texto.length > MAX_ENTRADA) return null;
  let v: Record<string, unknown>;
  try {
    v = JSON.parse(texto) as Record<string, unknown>;
  } catch {
    return null;
  }
  const titulo = typeof v?.titulo === 'string' ? v.titulo.trim() : '';
  if (
    v?.v !== VERSION_INDICE ||
    v.tipo !== TIPO_ALBUM ||
    !esId(v.id) ||
    !titulo ||
    Array.from(titulo).length > MAX_TITULO
  )
    return null;
  return {
    v: 1,
    tipo: TIPO_ALBUM,
    id: v.id,
    titulo,
    creado: esFecha(v.creado) ? new Date(v.creado).toISOString() : '',
    actualizado: esFecha(v.actualizado) ? new Date(v.actualizado).toISOString() : '',
  };
}

const texto = (fila: Record<string, unknown>, clave: string): string => {
  const valor = fila[clave];
  return typeof valor === 'string' ? valor : '';
};

/** Lo que se enseña de una foto sin entrada: lo que dice Drive. */
function entradaDeDrive(fila: Record<string, unknown>): EntradaFoto {
  const fecha = texto(fila, 'created_at') || texto(fila, 'updated_at');
  return {
    v: 1,
    tipo: TIPO_FOTO,
    foto: texto(fila, 'id'),
    suma: '',
    nombre: texto(fila, 'name'),
    tomada: esFecha(fecha) ? new Date(fecha).toISOString() : new Date(0).toISOString(),
    exif: false,
    ancho: 0,
    alto: 0,
    favorita: false,
    albumes: [],
    miniatura: '',
    actualizada: '',
  };
}

/** Junta un fichero de «Fotos» con su entrada, si vale y es suya. */
export function juntar(fila: Record<string, unknown>, entrada: EntradaFoto | null): FotoDeGaleria {
  const id = texto(fila, 'id');
  const suma = texto(fila, 'checksum');
  const base = {
    id,
    nombre: texto(fila, 'name'),
    tipo: texto(fila, 'content_type'),
    bytes: typeof fila.size_bytes === 'number' ? fila.size_bytes : 0,
    suma,
  };
  if (entrada && entrada.foto === id)
    return {
      ...base,
      entrada,
      conEntrada: true,
      pendiente: !entrada.miniatura || (suma !== '' && entrada.suma !== suma),
    };
  return { ...base, entrada: entradaDeDrive(fila), conEntrada: false, pendiente: true };
}

/** De la más reciente a la más antigua; a igual fecha, por id (estable). */
export function ordenar(fotos: FotoDeGaleria[]): FotoDeGaleria[] {
  return fotos.sort((a, b) =>
    a.entrada.tomada < b.entrada.tomada
      ? 1
      : a.entrada.tomada > b.entrada.tomada
        ? -1
        : a.id < b.id
          ? -1
          : a.id > b.id
            ? 1
            : 0,
  );
}

/**
 * La búsqueda de la galería sobre el índice descifrado, en el navegador: por nombre
 * de fichero y por fecha («2026», «2026-07», «14/07/2026» o el mes escrito en el
 * idioma de la sesión).
 */
export function buscarCifradas(
  fotos: readonly FotoDeGaleria[],
  consulta: string,
  mes: (iso: string) => string = () => '',
): FotoDeGaleria[] {
  const q = consulta.trim().toLocaleLowerCase();
  if (!q) return [...fotos];
  const partes = q.split(/\s+/);
  return fotos.filter((f) => {
    const iso = f.entrada.tomada;
    const [a, m, d] = [iso.slice(0, 4), iso.slice(5, 7), iso.slice(8, 10)];
    const campos = [
      f.entrada.nombre || f.nombre,
      iso.slice(0, 10),
      `${d}/${m}/${a}`,
      `${d}.${m}.${a}`,
      mes(iso),
    ]
      .join(' ')
      .toLocaleLowerCase();
    return partes.every((p) => campos.includes(p));
  });
}

/** El procesado de verdad, en el navegador: medidas orientadas y miniatura JPEG. */
export const procesarEnNavegador: Procesar = async (imagen) => {
  if (typeof createImageBitmap !== 'function') return null;
  let bm: ImageBitmap;
  try {
    bm = await createImageBitmap(imagen, { imageOrientation: 'from-image' });
  } catch {
    return null; // un formato que el navegador no sabe abrir (HEIC en muchos)
  }
  try {
    const escala = Math.min(1, LADO_MINIATURA / Math.max(bm.width, bm.height));
    const w = Math.max(1, Math.round(bm.width * escala));
    const h = Math.max(1, Math.round(bm.height * escala));
    const lienzo = new OffscreenCanvas(w, h);
    const ctx = lienzo.getContext('2d');
    if (!ctx) return null;
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, w, h);
    ctx.drawImage(bm, 0, 0, w, h);
    const miniatura = await lienzo.convertToBlob({ type: 'image/jpeg', quality: 0.82 });
    return { ancho: bm.width, alto: bm.height, miniatura };
  } finally {
    bm.close();
  }
};

async function hijos(repo: RepositorioFotos, id: string, signal?: AbortSignal) {
  const carpetas = new Map<string, Record<string, unknown>>();
  const ficheros: Record<string, unknown>[] = [];
  let cursor = '';
  do {
    const pagina = await repo.children(id, cursor, signal);
    for (const c of pagina.folders) carpetas.set(texto(c, 'id'), c);
    ficheros.push(...pagina.files);
    cursor = pagina.next_cursor;
  } while (cursor);
  return { carpetas: [...carpetas.values()], ficheros };
}

/** `<id><ext>` → fichero; con dos del mismo nombre, el más reciente. */
function porNombre(ficheros: Record<string, unknown>[], ext: string) {
  const out = new Map<string, Record<string, unknown>>();
  for (const f of ficheros) {
    const nombre = texto(f, 'name');
    if (!nombre.endsWith(ext)) continue;
    const id = nombre.slice(0, -ext.length);
    if (!ID.test(id)) continue;
    const ya = out.get(id);
    if (!ya || texto(f, 'updated_at') > texto(ya, 'updated_at')) out.set(id, f);
  }
  return out;
}

export interface OpcionesGaleria {
  signal?: AbortSignal | undefined;
  procesar?: Procesar;
  ahora?: () => Date;
  nuevoId?: () => string;
}

/** La galería cifrada sobre el Drive de la bóveda abierto. */
export class GaleriaCifrada {
  private readonly carpetas = new Map<string, Promise<string>>();
  private readonly procesar: Procesar;
  private readonly ahora: () => Date;
  private readonly nuevoId: () => string;
  private readonly signal: AbortSignal | undefined;

  constructor(
    private readonly repo: RepositorioFotos,
    opciones: OpcionesGaleria = {},
  ) {
    this.procesar = opciones.procesar ?? procesarEnNavegador;
    this.ahora = opciones.ahora ?? (() => new Date());
    this.nuevoId = opciones.nuevoId ?? (() => crypto.randomUUID());
    this.signal = opciones.signal;
  }

  /** Una carpeta de la galería: «fotos», «galeria» o una subcarpeta. '' si no está. */
  private carpeta(cual: string, crear: boolean): Promise<string> {
    const ya = this.carpetas.get(cual);
    if (ya) return ya;
    const buscar = async (): Promise<string> => {
      if (cual === 'fotos') return (await carpetaFotos(this.repo, crear, this.signal)) ?? '';
      const padre = await this.carpeta(cual === 'galeria' ? 'fotos' : 'galeria', crear);
      if (!padre) return '';
      const nombre = cual === 'galeria' ? CARPETA_GALERIA : cual;
      const { carpetas } = await hijos(this.repo, padre, this.signal);
      // Dos creadas a la vez por dos dispositivos: siempre la de id menor.
      const ids = carpetas
        .filter((c) => texto(c, 'name') === nombre)
        .map((c) => texto(c, 'id'))
        .sort();
      if (ids[0]) return ids[0];
      if (!crear) return '';
      return texto(await this.repo.createFolder(padre, nombre, this.signal), 'id');
    };
    const p = buscar().then((id) => {
      if (!id) this.carpetas.delete(cual);
      return id;
    });
    p.catch(() => this.carpetas.delete(cual));
    this.carpetas.set(cual, p);
    return p;
  }

  private async leerTexto(id: string): Promise<string> {
    const blob = await leerFotoCifrada(this.repo, { id }, this.signal);
    if (blob.size > MAX_ENTRADA) throw new Error('galeria_entrada_grande');
    return blob.text();
  }

  /** Descifra unos pocos a la vez; lo que no se lee se queda fuera sin parar el resto. */
  private async leerTodas(sub: string): Promise<Map<string, string>> {
    const carpeta = await this.carpeta(sub, false);
    const out = new Map<string, string>();
    if (!carpeta) return out;
    const { ficheros } = await hijos(this.repo, carpeta, this.signal);
    const cola = [...porNombre(ficheros, '.json')];
    const trabajador = async () => {
      for (let par = cola.shift(); par; par = cola.shift()) {
        this.signal?.throwIfAborted();
        try {
          out.set(par[0], await this.leerTexto(texto(par[1], 'id')));
        } catch {
          // Se reconstruye después.
        }
      }
    };
    await Promise.all(Array.from({ length: LECTORES }, trabajador));
    this.signal?.throwIfAborted();
    return out;
  }

  /** La galería entera. */
  async leer(): Promise<IndiceCifrado> {
    const fotos = await this.carpeta('fotos', false);
    if (!fotos) return { fotos: [], albumes: [] };
    const [{ ficheros }, entradas, crudos] = await Promise.all([
      hijos(this.repo, fotos, this.signal),
      this.leerTodas(CARPETA_INDICE),
      this.leerTodas(CARPETA_ALBUMES),
    ]);
    const lista = ficheros
      .filter((f) => ID.test(texto(f, 'id')) && texto(f, 'content_type').startsWith('image/'))
      .map((f) => {
        const crudo = entradas.get(texto(f, 'id'));
        return juntar(f, crudo === undefined ? null : decodificarEntrada(crudo));
      });
    const albumes: AlbumCifrado[] = [];
    for (const [id, crudo] of crudos) {
      const a = decodificarAlbum(crudo);
      if (a && a.id === id) albumes.push(a);
    }
    albumes.sort(
      (a, b) =>
        a.titulo.localeCompare(b.titulo, undefined, { sensitivity: 'base' }) ||
        (a.id < b.id ? -1 : 1),
    );
    return { fotos: ordenar(lista), albumes };
  }

  /** La entrada guardada de una foto, releída ahora; `null` sin entrada válida. */
  private async entradaActual(foto: string): Promise<EntradaFoto | null> {
    const carpeta = await this.carpeta(CARPETA_INDICE, false);
    if (!carpeta) return null;
    const { ficheros } = await hijos(this.repo, carpeta, this.signal);
    const fila = porNombre(ficheros, '.json').get(foto);
    if (!fila) return null;
    const e = decodificarEntrada(await this.leerTexto(texto(fila, 'id')));
    return e && e.foto === foto ? e : null;
  }

  private async subir(sub: string, nombre: string, tipo: string, cuerpo: Blob) {
    const carpeta = await this.carpeta(sub, true);
    return this.repo.upload(carpeta, nombre, tipo, cuerpo, this.signal);
  }

  private async escribir(e: EntradaFoto): Promise<EntradaFoto> {
    const cuerpo = new Blob([JSON.stringify(e)], { type: 'application/json' });
    await this.subir(CARPETA_INDICE, `${e.foto}.json`, 'application/json', cuerpo);
    return e;
  }

  /**
   * Apunta la foto en el índice con su contenido: fecha de captura del EXIF, medidas
   * orientadas y miniatura cifrada. Conserva favorita y álbumes; si ya es de este
   * contenido y tiene miniatura, no escribe nada.
   */
  async indexar(fila: Record<string, unknown>, contenido: Blob): Promise<EntradaFoto> {
    const id = texto(fila, 'id');
    if (!ID.test(id)) throw new Error('galeria_foto');
    const previa = await this.entradaActual(id);
    const suma = texto(fila, 'checksum');
    if (previa && previa.miniatura && suma && previa.suma === suma) return previa;
    const e = entradaDeDrive(fila);
    if (previa) {
      e.favorita = previa.favorita;
      e.albumes = previa.albumes;
    }
    e.suma = suma;
    const exif = leerExif(
      new Uint8Array(await contenido.slice(0, BYTES_DE_CABECERA).arrayBuffer()),
    );
    if (exif.tomada) {
      e.tomada = exif.tomada;
      e.exif = true;
    } else if (previa?.exif) {
      // Una edición sale del lienzo sin EXIF: la fecha de captura es la de antes.
      e.tomada = previa.tomada;
      e.exif = true;
    }
    const hecho = await this.procesar(contenido);
    this.signal?.throwIfAborted();
    if (hecho) {
      e.ancho = hecho.ancho;
      e.alto = hecho.alto;
      const mini = await this.subir(CARPETA_MINIATURAS, `${id}.jpg`, 'image/jpeg', hecho.miniatura);
      e.miniatura = texto(mini, 'id');
    } else if (previa) {
      e.miniatura = previa.miniatura;
      e.ancho = previa.ancho;
      e.alto = previa.alto;
    }
    e.actualizada = this.ahora().toISOString();
    return this.escribir(e);
  }

  /** Vuelve a indexar una foto desde su original en la bóveda. */
  async reconstruir(foto: FotoDeGaleria): Promise<EntradaFoto> {
    if (foto.bytes > MAXIMO_A_LA_VISTA) throw new Error('galeria_grande');
    const original = await this.original(foto);
    return this.indexar(
      {
        id: foto.id,
        name: foto.nombre,
        checksum: foto.suma,
        created_at: foto.entrada.exif ? '' : foto.entrada.tomada,
      },
      original,
    );
  }

  /**
   * Cambia la entrada releyéndola justo antes: lo que otro dispositivo cambió en
   * otro campo se conserva. Si el cambio no cambia nada, no escribe.
   */
  async actualizar(
    foto: FotoDeGaleria,
    cambio: (e: EntradaFoto) => EntradaFoto,
  ): Promise<EntradaFoto> {
    const actual = await this.entradaActual(foto.id);
    const base = actual ?? { ...foto.entrada, foto: foto.id };
    const nueva = cambio({ ...base, albumes: [...base.albumes] });
    nueva.albumes = sinRepetir(nueva.albumes);
    if (actual && JSON.stringify(actual) === JSON.stringify({ ...nueva })) return actual;
    nueva.actualizada = this.ahora().toISOString();
    return this.escribir(nueva);
  }

  marcarFavorita(foto: FotoDeGaleria, valor: boolean): Promise<EntradaFoto> {
    return this.actualizar(foto, (e) => ({ ...e, favorita: valor }));
  }

  ponerEnAlbum(foto: FotoDeGaleria, album: string, dentro: boolean): Promise<EntradaFoto> {
    if (!ID.test(album)) return Promise.reject(new Error('galeria_album'));
    return this.actualizar(foto, (e) => ({
      ...e,
      albumes: dentro ? [...e.albumes, album] : e.albumes.filter((a) => a !== album),
    }));
  }

  async crearAlbum(titulo: string): Promise<AlbumCifrado> {
    const ahora = this.ahora().toISOString();
    const album: AlbumCifrado = {
      v: 1,
      tipo: TIPO_ALBUM,
      id: this.nuevoId(),
      titulo: titulo.trim(),
      creado: ahora,
      actualizado: ahora,
    };
    if (!decodificarAlbum(JSON.stringify(album))) throw new Error('galeria_album');
    const cuerpo = new Blob([JSON.stringify(album)], { type: 'application/json' });
    await this.subir(CARPETA_ALBUMES, `${album.id}.json`, 'application/json', cuerpo);
    return album;
  }

  /** La miniatura descifrada, en memoria. */
  async miniatura(foto: FotoDeGaleria): Promise<Blob> {
    if (!foto.entrada.miniatura) throw new Error('galeria_sin_miniatura');
    const blob = await leerFotoCifrada(this.repo, { id: foto.entrada.miniatura }, this.signal);
    return blob.type === 'image/jpeg' ? blob : new Blob([blob], { type: 'image/jpeg' });
  }

  /** El original descifrado, en memoria (solo tras su compromiso completo). */
  original(foto: Pick<FotoDeGaleria, 'id'>): Promise<Blob> {
    return leerFotoCifrada(this.repo, foto, this.signal);
  }

  /**
   * Guarda una edición: el resultado sube como VERSIÓN NUEVA del mismo fichero de
   * «Fotos» (el original sigue en el historial de versiones de Drive) y la entrada se
   * rehace con su contenido, sin perder favorita ni álbumes.
   */
  async guardarEdicion(foto: FotoDeGaleria, editada: Blob): Promise<EntradaFoto> {
    const carpeta = await this.carpeta('fotos', false);
    if (!carpeta) throw new Error('galeria_sin_fotos');
    const fila = await this.repo.upload(
      carpeta,
      foto.nombre,
      editada.type || foto.tipo || 'image/jpeg',
      editada,
      this.signal,
    );
    if (texto(fila, 'id') !== foto.id) throw new Error('galeria_version');
    return this.indexar({ ...fila, created_at: foto.entrada.tomada }, editada);
  }
}

export { CARPETA_FOTOS_CIFRADAS };
