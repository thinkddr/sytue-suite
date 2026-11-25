/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El teclado de Fotos en UNA tabla: de ella salen lo que hace cada tecla
 * (`accionDeGaleria`, `accionDelVisor`) y la hoja de «?» (`gruposDeAtajos`). Así
 * la hoja no puede prometer una tecla que no hace nada, ni una tecla quedarse sin
 * explicar: los del visor solo los contaba un párrafo `sr-only`, y quien no usa
 * lector de pantalla no tenía dónde leerlos. La misma tabla vive en Fyne
 * (apps/fyne/apps/photos/atajos.go).
 */
import type { GrupoDeAtajos } from '@sytue/ui';
import type { PhotosMessageKey, T } from '../i18n/index.js';

/** Lo que hace una tecla en la galería (la línea de tiempo y Favoritas). */
export type AccionDeGaleria = 'subir' | 'buscar' | 'salirDeSeleccion' | 'ayuda';

/** Lo que hace una tecla con el visor abierto. */
export type AccionDelVisor =
  | 'anterior'
  | 'siguiente'
  | 'primera'
  | 'ultima'
  | 'pase'
  | 'ficha'
  | 'tamanyoReal'
  | 'favorita'
  | 'editar'
  | 'papelera'
  | 'cerrar'
  | 'ayuda';

interface FilaDeAtajo<A> {
  /**
   * Valores de `KeyboardEvent.key`. Las letras, en minúscula: se comparan sin
   * mayúsculas, porque con el bloqueo de mayúsculas puesto «i» llega como «I».
   */
  teclas: string[];
  accion: A;
  descripcion: PhotosMessageKey;
}

/** La galería. El orden es el de la hoja. */
const GALERIA: FilaDeAtajo<AccionDeGaleria>[] = [
  // El atajo que pulsa el menú nativo de la app de escritorio.
  { teclas: ['u'], accion: 'subir', descripcion: 'gallery.add' },
  // «/» lleva al buscador, como en Drive, Mail y la búsqueda de la suite: buscar una
  // foto por su nombre o por lo que sale en ella era ir a por el ratón.
  { teclas: ['/'], accion: 'buscar', descripcion: 'gallery.search' },
  { teclas: ['Escape'], accion: 'salirDeSeleccion', descripcion: 'gallery.cancelSelection' },
  { teclas: ['?'], accion: 'ayuda', descripcion: 'shortcuts.help' },
];

/**
 * El visor. Escape no se escucha: lo atiende el `<dialog>` (su `cancel`), y está en
 * la tabla solo para que la hoja lo cuente.
 */
const VISOR: FilaDeAtajo<AccionDelVisor>[] = [
  { teclas: ['ArrowLeft'], accion: 'anterior', descripcion: 'viewer.prev' },
  { teclas: ['ArrowRight'], accion: 'siguiente', descripcion: 'viewer.next' },
  { teclas: ['Home'], accion: 'primera', descripcion: 'shortcuts.first' },
  { teclas: ['End'], accion: 'ultima', descripcion: 'shortcuts.last' },
  { teclas: [' '], accion: 'pase', descripcion: 'shortcuts.slideshow' },
  { teclas: ['i'], accion: 'ficha', descripcion: 'shortcuts.info' },
  { teclas: ['z'], accion: 'tamanyoReal', descripcion: 'shortcuts.zoom' },
  // Las dos cosas que más se hacen con una foto abierta, sin buscar su botón: el
  // corazón y el editor.
  { teclas: ['f'], accion: 'favorita', descripcion: 'shortcuts.favorite' },
  { teclas: ['e'], accion: 'editar', descripcion: 'viewer.edit' },
  { teclas: ['Delete', 'Backspace'], accion: 'papelera', descripcion: 'viewer.delete' },
  { teclas: ['Escape'], accion: 'cerrar', descripcion: 'viewer.close' },
  { teclas: ['?'], accion: 'ayuda', descripcion: 'shortcuts.help' },
];

/** Una letra suelta se compara en minúscula; «ArrowLeft» o «?» van tal cual. */
function normalizar(tecla: string): string {
  return tecla.length === 1 ? tecla.toLowerCase() : tecla;
}

function accionEn<A>(tabla: FilaDeAtajo<A>[], tecla: string): A | null {
  const k = normalizar(tecla);
  return tabla.find((f) => f.teclas.includes(k))?.accion ?? null;
}

/** La acción de una tecla en la galería, o null. */
export function accionDeGaleria(tecla: string): AccionDeGaleria | null {
  return accionEn(GALERIA, tecla);
}

/** La acción de una tecla con el visor abierto, o null. */
export function accionDelVisor(tecla: string): AccionDelVisor | null {
  return accionEn(VISOR, tecla);
}

/** Cómo se ESCRIBE cada tecla en la hoja: símbolos, o la palabra en el idioma de quien lee. */
function rotuloDeTecla(t: T, tecla: string): string {
  switch (tecla) {
    case 'ArrowLeft':
      return '←';
    case 'ArrowRight':
      return '→';
    case 'Backspace':
      return '⌫';
    case 'Home':
      return t('shortcuts.keyHome');
    case 'End':
      return t('shortcuts.keyEnd');
    case ' ':
      return t('shortcuts.keySpace');
    case 'Delete':
      return t('shortcuts.keyDelete');
    case 'Escape':
      return t('shortcuts.keyEsc');
    default:
      return tecla.toUpperCase();
  }
}

/** Qué ofrece el visor que se está mirando: sin lista no hay saltos, y no todos tiran a la papelera. */
export interface OpcionesDelVisor {
  /** Inicio y Fin saltan (hay una lista que recorrer). */
  saltos?: boolean;
  /** Supr tira la foto a la papelera. */
  papelera?: boolean;
  /** F marca o desmarca la favorita (un álbum compartido no la ofrece). */
  favorita?: boolean;
}

function grupo<A>(t: T, titulo: PhotosMessageKey, filas: FilaDeAtajo<A>[]): GrupoDeAtajos {
  return {
    titulo: t(titulo),
    atajos: filas.map((f) => ({
      teclas: f.teclas.map((k) => rotuloDeTecla(t, k)),
      descripcion: t(f.descripcion),
    })),
  };
}

/**
 * La hoja de «?», agrupada como la pinta `ShortcutsModal`. En la galería lleva los
 * dos grupos (el del visor es lo que se puede hacer al abrir una foto); en el visor,
 * solo el suyo, y sin las filas que ESE visor no atiende: la hoja no promete una
 * tecla que no hace nada.
 */
export function gruposDeAtajos(
  t: T,
  donde: 'galeria' | 'visor',
  { saltos = true, papelera = true, favorita = true }: OpcionesDelVisor = {},
): GrupoDeAtajos[] {
  const visor = VISOR.filter(
    (f) =>
      (saltos || (f.accion !== 'primera' && f.accion !== 'ultima')) &&
      (papelera || f.accion !== 'papelera') &&
      (favorita || f.accion !== 'favorita'),
  );
  const delVisor = grupo(t, 'shortcuts.group.viewer', visor);
  if (donde === 'visor') return [delVisor];
  return [grupo(t, 'shortcuts.group.gallery', GALERIA), delVisor];
}
