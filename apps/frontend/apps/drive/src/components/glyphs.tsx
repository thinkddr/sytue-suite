/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Glyph — iconos semánticos propios de Sytue Drive (carpeta, reloj, estrella,
 * papelera…) que aún no existen en el set de cromo de @sytue/ui. Mismo lenguaje
 * visual, porque el envoltorio es el mismo (`Glifo`): aquí solo vive el DIBUJO.
 * Decorativos siempre (aria-hidden); la etiqueta accesible la aporta el control
 * que los usa.
 */
import type { JSX } from 'preact';
import { Glifo } from '@sytue/ui';

const GLYPHS = {
  folder: <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />,
  file: <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6" />,
  // Los TRES tipos de Sytue Docs. Mismos trazados que usa el propio Docs
  // (`packages/office/src/components/DocList.tsx` → KIND_PATH): el mismo concepto debe
  // llevar el mismo signo en las dos apps. Antes los tres se pintaban con `file`,
  // así que el menú «Nuevo» mostraba tres entradas con el icono idéntico y la
  // columna de tipo de «Documentos» no distinguía nada.
  document: (
    <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5M9 13h6M9 17h4" />
  ),
  // La tabla se reconoce por sus divisiones, no por su marco.
  spreadsheet: <path d="M4 5h16v14H4zM4 10h16M4 15h16M10 5v14" />,
  // Lienzo apaisado sobre pie, como una diapositiva proyectada.
  presentation: <path d="M4 4h16v11H4zM12 15v4M9 21h6" />,
  clock: <path d="M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18ZM12 7v5l3 2" />,
  users: (
    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm7-7.87a4 4 0 0 1 0 7.74M22 21v-2a4 4 0 0 0-3-3.87" />
  ),
  star: <path d="m12 3 2.9 5.9 6.5.9-4.7 4.6 1.1 6.4L12 17.8l-5.8 3 1.1-6.4-4.7-4.6 6.5-.9Z" />,
  // Sobre, para «Adjuntar a un correo». Se dibuja aquí y no se reaprovecha `link`
  // porque en el mismo menú ya hay «Copiar enlace»: dos entradas distintas con el
  // mismo icono se leen como la misma acción.
  mail: (
    <path d="M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 1.5 9 6 9-6" />
  ),
  trash: (
    <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M10 11v6M14 11v6" />
  ),
  download: <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3" />,
  // Subir: el mismo cajón que `download` con la flecha invertida (misma caja
  // óptica y mismo eje) — «Subir» no es «Nuevo», así que no lleva el signo +.
  upload: <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12" />,
  // Persona: el destinatario de una compartición con alguien de la casa, frente
  // al eslabón del enlace público.
  user: <path d="M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM4 21v-1a6 6 0 0 1 6-6h4a6 6 0 0 1 6 6v1" />,
  link: (
    <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
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
      <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
      <path d="M9 14h6m-2.5-2.5L15 14l-2.5 2.5" />
    </>
  ),
  lock: (
    <>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
  ),
  // Unidad de equipo (edificio/organización): la propiedad es de la ORG, no de un
  // usuario. Se distingue de «Compartido conmigo» (users), que es entre personas.
  building: (
    <>
      <path d="M3 21h18" />
      <path d="M5 21V5a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v16" />
      <path d="M15 21V9h2a2 2 0 0 1 2 2v10" />
      <path d="M8.5 7h1.5M8.5 11h1.5M8.5 15h1.5" />
    </>
  ),
} as const;

export type GlyphName = keyof typeof GLYPHS;

export interface GlyphProps {
  name: GlyphName;
  /** Tamaño en px (cuadrado). Por defecto 20. */
  size?: number;
  class?: string;
}

export function Glyph({ name, size = 20, class: cls }: GlyphProps): JSX.Element {
  return (
    <Glifo size={size} class={cls}>
      {GLYPHS[name]}
    </Glifo>
  );
}
