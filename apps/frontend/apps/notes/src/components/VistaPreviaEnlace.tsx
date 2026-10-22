/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La vista previa de un enlace a OTRA NOTA: al pasar el ratón por él (o llegar
 * con el teclado), una ficha con su título y su principio, como la de Twos.
 *
 * Solo con lo que la suite ya resuelve: las notas del cuaderno abierto, y si no,
 * la lista de notas del cuaderno del enlace, con el token de quien mira. Nada se
 * pide a terceros ni se «despliega» una URL ajena: un enlace a la web no tiene
 * vista previa. Una nota cifrada no enseña su contenido.
 */
import type { JSX, RefObject } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { createPortal } from 'preact/compat';
import { useFitInViewport } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { destinoInterno, type DestinoInterno } from '../client/enlaces.js';

export interface VistaPrevia {
  titulo: string;
  texto: string;
  cifrada?: boolean;
}

/** Lo que tarda en salir: pasar por encima camino de otra cosa no la abre. */
const RETARDO = 350;

/**
 * Engancha la vista previa a los enlaces de `ref` y devuelve la ficha, si la hay.
 * Los manejadores van con `addEventListener`, como el clic del cuerpo de lectura:
 * el `div` que contiene la nota no es interactivo, lo son sus `<a>`.
 */
export function useVistaPreviaEnlaces(
  ref: RefObject<HTMLElement>,
  t: T,
  publicBase: string | undefined,
  previa: ((d: DestinoInterno) => Promise<VistaPrevia | null>) | undefined,
): JSX.Element | null {
  const [ficha, setFicha] = useState<{
    x: number;
    y: number;
    vista: VistaPrevia | null;
  } | null>(null);
  const reloj = useRef<number | undefined>(undefined);
  const vuelta = useRef(0);
  const fichaRef = useRef<HTMLDivElement>(null);
  useFitInViewport(fichaRef, ficha, { vertical: true });

  useEffect(() => {
    const el = ref.current;
    if (!el || !publicBase || !previa) return;
    const cerrar = () => {
      window.clearTimeout(reloj.current);
      vuelta.current++;
      setFicha(null);
    };
    const abrir = (e: Event) => {
      const a = (e.target as HTMLElement | null)?.closest('a');
      const d = a ? destinoInterno(a.href, publicBase) : null;
      if (!a || !d) return;
      window.clearTimeout(reloj.current);
      const mia = ++vuelta.current;
      reloj.current = window.setTimeout(() => {
        void previa(d).then((vista) => {
          if (mia !== vuelta.current) return;
          const caja = a.getBoundingClientRect();
          setFicha({
            x: Math.max(8, Math.min(caja.left, window.innerWidth - 288)),
            y: caja.bottom + 6,
            vista,
          });
        });
      }, RETARDO);
    };
    const salir = (e: Event) => {
      if ((e.target as HTMLElement | null)?.closest('a')) cerrar();
    };
    el.addEventListener('mouseover', abrir);
    el.addEventListener('focusin', abrir);
    el.addEventListener('mouseout', salir);
    el.addEventListener('focusout', salir);
    return () => {
      cerrar();
      el.removeEventListener('mouseover', abrir);
      el.removeEventListener('focusin', abrir);
      el.removeEventListener('mouseout', salir);
      el.removeEventListener('focusout', salir);
    };
  }, [ref, publicBase, previa]);

  if (!ficha || typeof document === 'undefined') return null;
  const v = ficha.vista;
  return createPortal(
    <div
      ref={fichaRef}
      role="tooltip"
      class="sytue-pop-in sytue-menu-surface border-border pointer-events-none fixed grid w-72 gap-1 rounded-lg border p-3 shadow-[var(--shadow-md)]"
      style={{ zIndex: 'var(--z-dropdown)', left: `${ficha.x}px`, top: `${ficha.y}px` }}
    >
      {v === null ? (
        <span class="text-text-muted text-sm">{t('linkPreview.unavailable')}</span>
      ) : (
        <>
          <span class="text-text truncate text-sm font-medium">{v.titulo || t('note.new')}</span>
          <span class="sytue-meta line-clamp-3">
            {v.cifrada ? t('linkPreview.encrypted') : v.texto || t('linkPreview.empty')}
          </span>
        </>
      )}
    </div>,
    document.body,
  );
}

/** El texto plano del principio de una nota, para la ficha. */
export function principioDe(html: string, n = 180): string {
  // DOMParser y no un `div`: lo que se analiza así no carga imágenes ni corre nada.
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const texto = (doc.body.textContent ?? '').replace(/\s+/g, ' ').trim();
  return texto.length > n ? `${texto.slice(0, n).trimEnd()}…` : texto;
}
