/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El ESCENARIO de un visor de fotos: el hueco donde va la foto, con «anterior» y
 * «siguiente» a los lados y el deslizamiento con el dedo. Lo comparten el visor de la
 * galería (`Visor`, en PhotosApp.tsx) y el de un álbum compartido conmigo
 * (`VisorCompartido`), que antes tenían cada uno sus flechas y ninguna se veía bien.
 *
 * LAS FLECHAS. Medido el 2026-10-16 en el arnés de capturas con fotos reales: eran
 * mandos de 32 px con un galón de 20 y 1,5 px de trazo, negro al 60 % SIN BORDE. Sobre
 * una foto negra la placa medía 1:1 contra la foto (no se veía dónde acababa el botón) y
 * sobre las fotos de noche del arnés, entre 1,5:1 y 2,6:1; en un escritorio, un objetivo
 * de 32×32 a 12 px del borde. Ahora:
 *
 *   - 48 px de placa (`IconButton size="xl"`) con un galón de 24, el trazo de 1,5 px de
 *     todos los glifos de la suite: se encuentra de un vistazo sobre cualquier foto,
 *     también en un teléfono;
 *   - el tono `media` lleva un filete blanco al 40 %: sobre negro el borde queda a 3,7:1
 *     y sobre blanco la placa a 5,7:1, así que el botón se distingue en los dos extremos;
 *   - con ratón, el objetivo se alarga hasta el canto de la ventana y 120 px por encima y
 *     por debajo (un `::before` del propio botón: el aro de foco sigue en la placa). El
 *     canto es un objetivo infinito para el ratón: basta con llevarlo al lado;
 *   - solo tapan la foto cuando la foto llega al borde: van en el escenario, no sobre la
 *     foto, y una foto que deja margen a los lados las deja en el margen.
 */
import type { ComponentChildren, JSX, Ref } from 'preact';
import { Icon, IconButton, cx } from '@sytue/ui';
import { useGestosVisor } from '../lib/gestos.js';

/**
 * Con ratón el botón cubre una franja de 80 × 288 px pegada al canto (48 de placa + 16 de
 * margen a cada lado, y 120 por encima y por debajo). Con el dedo no: una franja así se
 * comería el arrastre que empieza cerca del borde, y ahí manda deslizar.
 */
const FRANJA =
  'pointer-fine:before:absolute pointer-fine:before:-inset-x-4 pointer-fine:before:-inset-y-30';

export function FlechaVisor({
  lado,
  label,
  onClick,
}: {
  lado: 'anterior' | 'siguiente';
  label: string;
  onClick: () => void;
}): JSX.Element {
  return (
    <IconButton
      label={label}
      tone="media"
      size="xl"
      onClick={onClick}
      class={cx(
        'absolute top-1/2 z-10 -translate-y-1/2',
        lado === 'anterior' ? 'left-2 sm:left-4' : 'right-2 sm:right-4',
        FRANJA,
      )}
    >
      <Icon name={lado === 'anterior' ? 'chevronLeft' : 'chevronRight'} size={24} />
    </IconButton>
  );
}

export function EscenarioVisor({
  prev,
  next,
  onPrev,
  onNext,
  real = false,
  foto,
  clave,
  contenedor,
  class: clase,
  children,
}: {
  /** Los rótulos de las dos flechas (`viewer.prev`, `viewer.next`). */
  prev: string;
  next: string;
  onPrev?: (() => void) | undefined;
  onNext?: (() => void) | undefined;
  /** A tamaño real la foto se recorre con el dedo: ni se desliza ni se pellizca. */
  real?: boolean;
  /** Lo que se mueve con el dedo. */
  foto: { current: HTMLElement | null };
  /** La foto que se enseña: al cambiar, lo ampliado con los dedos se olvida. */
  clave: string;
  /** Lo que se desplaza a tamaño real, para llevarlo al punto pulsado. */
  contenedor?: Ref<HTMLDivElement>;
  class?: string;
  children: ComponentChildren;
}): JSX.Element {
  const gestos = useGestosVisor({ objetivo: foto, clave, onPrev, onNext, activo: !real });
  return (
    // Con la foto encajada, TODO el dedo es nuestro (`touch-none`): deslizar, pellizcar
    // y mover lo ampliado. A tamaño real, el desplazamiento es del navegador. Las flechas
    // van FUERA de lo que desplaza: a tamaño real la foto se recorre y ellas se quedan.
    <div class={cx('relative flex min-h-0 flex-1', !real && 'touch-none')} {...gestos}>
      <div ref={contenedor ?? null} class={cx('relative flex min-h-0 min-w-0 flex-1', clase)}>
        {children}
      </div>
      {onPrev ? <FlechaVisor lado="anterior" label={prev} onClick={onPrev} /> : null}
      {onNext ? <FlechaVisor lado="siguiente" label={next} onClick={onNext} /> : null}
    </div>
  );
}
