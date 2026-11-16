/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Glyph — iconos semánticos propios de Sytue Drive (carpeta, fichero, estrella,
 * los tres tipos de Docs…) que no existen en el set de cromo de @sytue/ui. Mismo
 * lenguaje visual, porque el envoltorio es el mismo (`Glifo`): aquí solo vive el
 * DIBUJO. Decorativos siempre (aria-hidden); la etiqueta accesible la aporta el
 * control que los usa.
 *
 * UN DIBUJO POR CONCEPTO. La papelera, la descarga, el enlace, el candado o las
 * personas se dibujaban aquí con OTRO trazado que el común (varios con la geometría
 * de Feather), así que la misma acción se veía distinta en Drive que en el resto de
 * la suite. Esos nombres se piden igual (`<Glyph name="trash" />`) pero los pinta
 * `Icon`, y el guardián `glifos-al-comun.test.ts` de @sytue/ui impide que vuelvan.
 */
import type { JSX } from 'preact';
import { Glifo, Icon, type IconName } from '@sytue/ui';

const GLYPHS = {
  folder: (
    <path d="M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17" />
  ),
  file: (
    <path d="M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5ZM13.5 3.5V8.5h5" />
  ),
  // Los TRES tipos de Sytue Docs. Mismos trazados que usa el propio Docs
  // (`packages/office/src/components/DocList.tsx` → KIND_PATH): el mismo concepto debe
  // llevar el mismo signo en las dos apps. Antes los tres se pintaban con `file`,
  // así que el menú «Nuevo» mostraba tres entradas con el icono idéntico y la
  // columna de tipo de «Documentos» no distinguía nada.
  document: (
    <path d="M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5ZM13.5 3.5V8.5h5M8.5 13h7M8.5 16.5h4.5" />
  ),
  // La tabla se reconoce por sus divisiones, no por su marco.
  spreadsheet: <path d="M4 5h16v14H4zM4 10h16M4 15h16M10 5v14" />,
  // Lienzo apaisado sobre pie, como una diapositiva proyectada.
  presentation: <path d="M4 4h16v11H4zM12 15v4M9 21h6" />,
  star: (
    <path d="M12 3.6L14.4 9.3L20.6 9.8L15.9 13.9L17.3 19.9L12 16.7L6.7 19.9L8.1 13.9L3.4 9.8L9.6 9.3Z" />
  ),
  more: (
    <>
      <circle cx="12" cy="5" r="1" />
      <circle cx="12" cy="12" r="1" />
      <circle cx="12" cy="19" r="1" />
    </>
  ),
  pencil: <path d="M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />,
  copy: (
    <>
      <rect x="9" y="9" width="12" height="12" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </>
  ),
  folderMove: (
    <>
      <path d="M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17" />
      <path d="M9 15h6M12.5 12.5 15 15l-2.5 2.5" />
    </>
  ),
} as const;

/** Los nombres de Drive que dibuja el catálogo común. El sobre de «Adjuntar a un
 *  correo» sigue sin confundirse con «Copiar enlace»: son `mail` y `link`, dos
 *  dibujos distintos también en el común. */
const DEL_COMUN = [
  'clock',
  'users',
  'user',
  'mail',
  'trash',
  'download',
  'upload',
  'link',
  'lock',
  'building',
] as const satisfies readonly IconName[];

type Propio = keyof typeof GLYPHS;
export type GlyphName = Propio | (typeof DEL_COMUN)[number];

function esPropio(name: GlyphName): name is Propio {
  return Object.hasOwn(GLYPHS, name);
}

export interface GlyphProps {
  name: GlyphName;
  /** Tamaño en px (cuadrado). Por defecto 20. */
  size?: number;
  class?: string;
}

export function Glyph({ name, size = 20, class: cls }: GlyphProps): JSX.Element {
  return esPropio(name) ? (
    <Glifo size={size} class={cls}>
      {GLYPHS[name]}
    </Glifo>
  ) : (
    <Icon name={name} size={size} class={cls} />
  );
}
