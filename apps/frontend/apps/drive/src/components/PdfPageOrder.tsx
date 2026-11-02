/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PdfPageOrder — las páginas ELEGIDAS de un PDF, en el orden en que van a salir,
 * y la forma de cambiarlo.
 *
 * El asa compartida reordena con dedo, ratón o flechas, y desplaza la lista en sus
 * bordes. Los botones subir/bajar conservan su foco y anuncian la posición final.
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Icon, IconButton, useListReorder } from '@sytue/ui';
import type { T } from '../i18n';

/**
 * Una página tal y como la describe `/api/pdf-inspect`: lo justo para
 * RECONOCERLA sin miniatura (número, tamaño y las primeras palabras).
 */
export interface PaginaIndice {
  numero: number;
  ancho: number;
  alto: number;
  rotacion: number;
  extracto: string;
}

/**
 * «1,3,5-8» → [1,3,5,6,7,8]. Misma gramática que la del servicio (`parsearPaginas`
 * en pdfpages.go), rangos al revés incluidos («8-5» son esas páginas del revés).
 *
 * Conserva el ORDEN y las repeticiones porque para `extraer` significan algo: la
 * lista es el documento que va a salir, no un conjunto. Lo que se sale del
 * documento se descarta —no se puede reordenar una página que no existe—; si es
 * una errata, el servicio la señala al enviar, que es donde se ve el número malo.
 */
export function expandirPaginas(spec: string, total: number): number[] {
  const MAX = 10000; // el mismo tope que el servicio, para no colgar el navegador
  const out: number[] = [];
  for (const trozo of spec.split(',')) {
    const t = trozo.trim();
    const m = /^(\d+)(?:-(\d+))?$/.exec(t);
    if (!m) continue;
    const a = Number(m[1]);
    const b = m[2] === undefined ? a : Number(m[2]);
    for (let p = a; ; p += b < a ? -1 : 1) {
      if (p >= 1 && p <= total) out.push(p);
      if (p === b || out.length >= MAX) break;
    }
    if (out.length >= MAX) break;
  }
  return out;
}

/** Mueve un elemento de `desde` a `hasta`. Fuera de rango, devuelve la lista igual. */
export function moverEnLista<X>(lista: readonly X[], desde: number, hasta: number): X[] {
  const copia = [...lista];
  if (desde === hasta || desde < 0 || hasta < 0 || desde >= copia.length || hasta >= copia.length) {
    return copia;
  }
  const [x] = copia.splice(desde, 1);
  copia.splice(hasta, 0, x as X);
  return copia;
}

export function PdfPageOrder({
  orden,
  indice,
  t,
  onChange,
}: {
  /** Las páginas elegidas, en el orden en que saldrán. */
  orden: number[];
  /** El índice completo del documento, para poner texto a cada número. */
  indice: PaginaIndice[];
  t: T;
  onChange: (siguiente: number[]) => void;
}) {
  const [aviso, setAviso] = useState('');
  const listaRef = useRef<HTMLUListElement>(null);
  // Fila y dirección del último movimiento hecho con el teclado, para devolverle
  // el foco después de que Preact repinte la lista.
  const focoRef = useRef<{ fila: number; dir: 'arriba' | 'abajo' } | null>(null);

  const extractos = useMemo(() => new Map(indice.map((p) => [p.numero, p.extracto])), [indice]);

  // Al mover con el teclado, la fila que tenía el foco se va a otro sitio y el
  // foco se quedaría en el `body`: la segunda pulsación no llegaría a ningún
  // botón. Se persigue la página hasta su nueva fila; si el botón que se usó
  // queda deshabilitado por haber llegado a un extremo, va al otro de esa fila
  // en vez de perderse.
  useEffect(() => {
    const pend = focoRef.current;
    if (pend === null || listaRef.current === null) return;
    focoRef.current = null;
    const fila = listaRef.current.querySelector(`[data-fila="${pend.fila}"]`);
    const botones = fila?.querySelectorAll<HTMLButtonElement>('button[data-mover-pagina]');
    if (!botones) return;
    const preferido = pend.dir === 'arriba' ? botones[0] : botones[1];
    const otro = pend.dir === 'arriba' ? botones[1] : botones[0];
    (preferido?.disabled === false ? preferido : otro)?.focus();
  });

  function mover(desde: number, hasta: number, dir?: 'arriba' | 'abajo'): void {
    if (desde === hasta || hasta < 0 || hasta >= orden.length) return;
    const pagina = orden[desde]!;
    onChange(moverEnLista(orden, desde, hasta));
    setAviso(t('pdf.order.moved', { n: pagina, pos: hasta + 1, total: orden.length }));
    if (dir !== undefined) focoRef.current = { fila: hasta, dir };
  }

  const reordenar = useListReorder({
    ids: orden.map((_, i) => String(i)),
    container: () => listaRef.current,
    onReorder: (desde, hasta) => {
      mover(desde, hasta);
      return String(hasta);
    },
  });

  return (
    <fieldset>
      <legend class="text-text mb-1 text-sm font-medium">{t('pdf.order')}</legend>
      <p class="text-text-muted mb-2 text-xs">{t('pdf.orderHint')}</p>
      <ul ref={listaRef} class="border-border max-h-56 overflow-y-auto rounded-md border">
        {orden.map((n, i) => (
          <li
            key={`${n}-${i}`}
            data-fila={i}
            {...reordenar.itemProps(String(i))}
            class="border-border/60 flex items-center gap-2 border-b px-2 py-1.5 last:border-b-0 pointer-coarse:min-h-[max(44px,2.75rem)]"
          >
            <button
              type="button"
              {...reordenar.handleProps(String(i))}
              aria-label={`${t('pdf.order')} · ${t('pdf.pageN', { n })}`}
              aria-keyshortcuts="ArrowUp ArrowDown Home End"
              title={t('pdf.orderHint')}
              class="sytue-focus-ring text-text-muted hover:text-text min-h-8 shrink-0 cursor-grab touch-none rounded px-1 py-1 select-none [-webkit-user-select:none] active:cursor-grabbing pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
            >
              <Icon name="grip" size={16} />
            </button>
            <span class="text-text-subtle w-5 shrink-0 text-right text-xs tabular-nums">
              {i + 1}
            </span>
            <span class="text-text shrink-0 text-sm font-medium tabular-nums">
              {t('pdf.pageN', { n })}
            </span>
            <span class="text-text-subtle min-w-0 flex-1 truncate text-xs">
              {extractos.get(n) || t('pdf.pageNoText')}
            </span>
            <IconButton
              data-mover-pagina
              size="sm"
              label={t('pdf.order.up', { n })}
              disabled={i === 0}
              onClick={() => mover(i, i - 1, 'arriba')}
            >
              <Icon name="chevronUp" size={14} />
            </IconButton>
            <IconButton
              data-mover-pagina
              size="sm"
              label={t('pdf.order.down', { n })}
              disabled={i === orden.length - 1}
              onClick={() => mover(i, i + 1, 'abajo')}
            >
              <Icon name="chevronDown" size={14} />
            </IconButton>
          </li>
        ))}
      </ul>
      <span class="sr-only" role="status">
        {aviso}
      </span>
    </fieldset>
  );
}
