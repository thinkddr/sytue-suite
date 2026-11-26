/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Organizar en álbumes con IA», lo que no es Sytue AI: de qué fotos se parte, qué
 * se crea con la propuesta que se aceptó y cómo se deshace.
 *
 * Lo que se le pide al modelo y cómo se lee está en `agruparIA.ts`. Esto es el
 * espejo de `apps/fyne/apps/photos/organizar.go`, y las dos pruebas dicen lo mismo.
 * Las llamadas al servicio llegan como funciones para que la prueba no necesite red.
 */
import { plegar } from '@sytue/i18n';
import { enTandas } from '@sytue/ui';
import { TOPE_ORGANIZAR } from './agruparIA.js';

interface ConId {
  id: string;
}

/** Un álbum de la persona, lo justo para organizar. */
export interface AlbumMio {
  id: string;
  title: string;
  count: number;
}

/**
 * Cuántas páginas de la línea de tiempo se miran como mucho buscando fotos sin
 * álbum, y de cuántas fotos (el máximo del servicio). Dos mil fotos: si entre
 * ellas no hay doscientas sueltas, la biblioteca ya está ordenada, y recorrerla
 * entera para encontrar las últimas no cabe en una pulsación.
 */
export const PAGINAS_ORGANIZAR = 10;
export const POR_PAGINA_ORGANIZAR = 200;

/** Lo que hace falta del servicio para saber de qué fotos se parte. */
export interface FuenteOrganizar<F extends ConId> {
  albumes: () => Promise<AlbumMio[]>;
  fotosDelAlbum: (id: string) => Promise<ConId[]>;
  pagina: (cursor: string | undefined) => Promise<{ items: F[]; nextCursor?: string }>;
}

/**
 * Las fotos que no están en ninguno de los álbumes de la persona, de la más
 * reciente a la más antigua y [`TOPE_ORGANIZAR`] como mucho. `topado` dice que
 * quedan fotos sin mirar: la pasada siguiente sigue por ahí.
 *
 * SOLO SUS ÁLBUMES: lo que alguien le comparte para editar es de otra persona, y
 * una foto suya metida ahí sigue sin estar en ninguno de los suyos.
 *
 * Si un álbum no se puede leer, no se sigue: sin saber qué tiene, se propondría
 * ordenar fotos que ya están ordenadas.
 */
export async function fotosSinAlbum<F extends ConId>(
  fuente: FuenteOrganizar<F>,
): Promise<{ albumes: AlbumMio[]; fotos: F[]; topado: boolean }> {
  const albumes = await fuente.albumes();
  const enAlbum = new Set<string>();
  let fallo: unknown = null;
  // Los vacíos no se piden: no hay nada que leer en ellos.
  const { fallidos } = await enTandas(
    albumes.filter((a) => a.count > 0),
    async (a) => {
      try {
        for (const f of await fuente.fotosDelAlbum(a.id)) enAlbum.add(f.id);
      } catch (e) {
        fallo ??= e;
        throw e;
      }
    },
  );
  if (fallidos.length > 0) throw fallo;

  const fotos: F[] = [];
  let cursor: string | undefined;
  for (let n = 0; n < PAGINAS_ORGANIZAR; n++) {
    const pagina = await fuente.pagina(cursor);
    const sueltas = pagina.items.filter((f) => !enAlbum.has(f.id));
    const caben = TOPE_ORGANIZAR - fotos.length;
    fotos.push(...sueltas.slice(0, caben));
    cursor = pagina.nextCursor;
    if (sueltas.length > caben) return { albumes, fotos, topado: true };
    if (!cursor) return { albumes, fotos, topado: false };
    if (fotos.length >= TOPE_ORGANIZAR) break;
  }
  return { albumes, fotos, topado: true };
}

/** Un álbum de la propuesta tal como queda en la vista previa. */
export interface AlbumElegido {
  titulo: string;
  ids: readonly string[];
  incluido: boolean;
}

/** Un paso de lo que se va a crear: en un álbum que ya existe, o en uno nuevo. */
export interface PasoOrganizar {
  titulo: string;
  /** El álbum de la persona que se llama igual; `null`, hay que crearlo. */
  existente: AlbumMio | null;
  ids: string[];
}

/**
 * Lo que se crea con lo que se dejó marcado.
 *
 * SI YA HAY UN ÁLBUM QUE SE LLAMA IGUAL —sin contar mayúsculas ni acentos—, las
 * fotos van a ese y no se crea otro: dos «Verano» en la lista es justo el desorden
 * que se quería arreglar. Por lo mismo, dos propuestas que tras editar el título se
 * llaman igual se juntan en una. Lo que queda sin título o sin fotos no se crea.
 */
export function planDeAlbumes(
  elegidos: readonly AlbumElegido[],
  existentes: readonly AlbumMio[],
): PasoOrganizar[] {
  const plan: PasoOrganizar[] = [];
  const porTitulo = new Map<string, PasoOrganizar>();
  for (const e of elegidos) {
    const titulo = e.titulo.trim();
    if (!e.incluido || titulo === '' || e.ids.length === 0) continue;
    const clave = plegar(titulo);
    const ya = porTitulo.get(clave);
    if (ya) {
      for (const id of e.ids) if (!ya.ids.includes(id)) ya.ids.push(id);
      continue;
    }
    const paso: PasoOrganizar = {
      titulo,
      existente: existentes.find((a) => plegar(a.title.trim()) === clave) ?? null,
      ids: [...new Set(e.ids)],
    };
    porTitulo.set(clave, paso);
    plan.push(paso);
  }
  return plan;
}

/** Lo que se hizo de verdad: con esto se deshace. */
export interface HechoOrganizar {
  album: AlbumMio;
  /** Se creó ahora: deshacer lo borra entero (las fotos se quedan en la biblioteca). */
  nuevo: boolean;
  /** Las fotos que se metieron: deshacer las saca de un álbum que ya existía. */
  ids: string[];
}

/**
 * Crea los álbumes del plan y les mete sus fotos, uno detrás de otro. Un álbum que
 * falla no para el resto: lo hecho, hecho está, y se dice cuántos fallaron.
 */
export async function crearAlbumes(
  plan: readonly PasoOrganizar[],
  servicio: {
    crearAlbum: (titulo: string) => Promise<AlbumMio>;
    anyadirAlAlbum: (id: string, ids: string[]) => Promise<unknown>;
  },
): Promise<{ hechos: HechoOrganizar[]; fallidos: number }> {
  const hechos: HechoOrganizar[] = [];
  let fallidos = 0;
  for (const paso of plan) {
    let album = paso.existente;
    const nuevo = album === null;
    try {
      album ??= await servicio.crearAlbum(paso.titulo);
    } catch {
      fallidos++;
      continue;
    }
    try {
      await servicio.anyadirAlAlbum(album.id, paso.ids);
      hechos.push({
        album: { ...album, count: album.count + paso.ids.length },
        nuevo,
        ids: paso.ids,
      });
    } catch {
      fallidos++;
      // Uno recién creado y vacío se queda para que «Deshacer» también lo quite.
      if (nuevo) hechos.push({ album, nuevo, ids: [] });
    }
  }
  return { hechos, fallidos };
}

/**
 * «Deshacer»: borra los álbumes que se crearon y saca de los que ya existían las
 * fotos que se metieron. Las fotos no se tocan: un álbum solo las agrupa. Devuelve
 * cuántas cosas no se pudieron deshacer.
 */
export async function deshacerOrganizar(
  hechos: readonly HechoOrganizar[],
  servicio: {
    borrarAlbum: (id: string) => Promise<unknown>;
    quitarDelAlbum: (id: string, foto: string) => Promise<unknown>;
  },
): Promise<{ fallidas: number }> {
  const tareas: (() => Promise<unknown>)[] = [];
  for (const h of hechos) {
    if (h.nuevo) tareas.push(() => servicio.borrarAlbum(h.album.id));
    else for (const id of h.ids) tareas.push(() => servicio.quitarDelAlbum(h.album.id, id));
  }
  const { fallidos } = await enTandas(tareas, (t) => t());
  return { fallidas: fallidos.length };
}
