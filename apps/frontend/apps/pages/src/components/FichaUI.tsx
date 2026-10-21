/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las dos piezas que comparten las pestañas de la ficha del sitio (publicar, versiones, dominios,
 * pedidos…): la tarjeta de sección y el aviso en línea. Ninguna sabe nada del sitio.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useId } from 'preact/hooks';

export interface SeccionProps {
  titulo: string;
  descripcion?: string;
  /** Limita el ancho a un formulario; sin él ocupa todo el panel (listas y tablas). */
  estrecha?: boolean;
  children: ComponentChildren;
}

/** Tarjeta de una pestaña: título de sección, descripción opcional y contenido. */
export function Seccion({ titulo, descripcion, estrecha, children }: SeccionProps): JSX.Element {
  const id = useId();
  return (
    <section
      aria-labelledby={id}
      class={`border-border bg-surface min-w-0 rounded-xl border p-4 sm:p-6 ${estrecha ? 'max-w-2xl' : ''}`}
    >
      <h2 id={id} class="sytue-h4 text-text">
        {titulo}
      </h2>
      {descripcion ? <p class="text-text-muted mt-1 text-sm">{descripcion}</p> : null}
      <div class="mt-5">{children}</div>
    </section>
  );
}

const TONOS = {
  exito: 'bg-success-bg text-success-text',
  error: 'bg-danger-bg text-danger-text',
  info: 'bg-info-bg text-info-text',
} as const;

export interface AvisoProps {
  tono: keyof typeof TONOS;
  class?: string;
  children: ComponentChildren;
}

/**
 * Mensaje en línea. El error es `role="alert"` (interrumpe) y el resto `role="status"` (espera su
 * turno): lo que cambia solo, y no solo con color, llega también al lector de pantalla.
 */
export function Aviso({ tono, class: cls, children }: AvisoProps): JSX.Element {
  return (
    <p
      role={tono === 'error' ? 'alert' : 'status'}
      class={`${TONOS[tono]} rounded-md p-3 text-sm ${cls ?? ''}`}
    >
      {children}
    </p>
  );
}
