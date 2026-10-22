/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PdfPageOrder — las páginas ELEGIDAS de un PDF, en el orden en que van a salir,
 * y la forma de cambiarlo.
 *
 * Existe porque elegir y ordenar son dos cosas distintas y el diálogo solo hacía
 * la primera: el orden salía de pulsar las páginas en secuencia, así que corregir
 * la posición de UNA página en un documento de veinte obligaba a deseleccionarlo
 * todo y volver a empezar. Arrastrar es el gesto que la gente ya trae aprendido.
 *
 * Arrastrar NO puede ser la única forma. Va con dos botones por fila (subir y
 * bajar) que hacen exactamente lo mismo, porque:
 *   - el arrastre HTML5 no existe en pantallas táctiles (no hay `dragstart` con
 *     el dedo), así que sin botones el móvil se queda fuera;
 *   - un lector de pantalla no puede arrastrar, y el agarre es `aria-hidden`
 *     justamente por eso: lo que se anuncia son los botones, que sí se pulsan.
 * Cada movimiento se cuenta en una región `aria-live` («la página 5 pasa a la
 * posición 2 de 12»), porque quien no ve la lista no tiene otra forma de saber
 * que el arrastre —o el botón— hizo algo.
 *
 * Sin librería de drag-and-drop: `draggable` + `dragover` + `drop` es API del
 * navegador desde hace quince años y aquí solo hay que mover un elemento dentro
 * de un array.
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Icon, IconButton, cx } from '@sytue/ui';
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
  const [arrastrando, setArrastrando] = useState<number | null>(null);
  const [sobre, setSobre] = useState<number | null>(null);
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
    const botones = fila?.querySelectorAll<HTMLButtonElement>('button');
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

  return (
    <fieldset>
      <legend class="text-text mb-1 text-sm font-medium">{t('pdf.order')}</legend>
      <p class="text-text-muted mb-2 text-xs">{t('pdf.orderHint')}</p>
      {/* Los eventos de ARRASTRE van en la lista y en sus filas porque no hay
          ningún elemento interactivo al que puedan ir: una zona de suelta no es
          un botón, y darle un `role` interactivo para callar al linter le
          anunciaría al lector de pantalla algo que no existe.

          Desactivar la regla aquí es legítimo, y no por comodidad: lo que
          protege —que una función no quede solo al alcance del ratón— ya está
          cubierto. Reordenar se puede hacer entera con el teclado mediante los
          dos botones del final de cada fila, con el foco persiguiendo a la
          página y cada movimiento anunciado por `aria-live`. El arrastre es un
          atajo de ratón ENCIMA de eso, no la única vía. */}
      {/* eslint-disable jsx-a11y/no-noninteractive-element-interactions */}
      <ul
        ref={listaRef}
        class="border-border max-h-56 overflow-y-auto rounded-md border"
        onDragLeave={() => setSobre(null)}
      >
        {orden.map((n, i) => (
          <li
            key={`${n}-${i}`}
            data-fila={i}
            onDragOver={(e) => {
              if (arrastrando === null) return;
              // Sin `preventDefault` el navegador NO deja soltar aquí.
              e.preventDefault();
              if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
              setSobre(i);
            }}
            onDrop={(e) => {
              e.preventDefault();
              if (arrastrando !== null) mover(arrastrando, i);
              setArrastrando(null);
              setSobre(null);
            }}
            class={cx(
              'border-border/60 flex items-center gap-2 border-b px-2 py-1.5 last:border-b-0 pointer-coarse:min-h-[max(44px,2.75rem)]',
              sobre === i && arrastrando !== null && arrastrando !== i ? 'bg-primary/10' : '',
              arrastrando === i ? 'opacity-50' : '',
            )}
          >
            {/* El agarre es `aria-hidden` a propósito: arrastrar no está al
                alcance de un lector de pantalla ni de un dedo, y anunciarlo
                sería ofrecer algo que no se puede usar. Lo que se anuncia son
                los dos botones del final de la fila, que hacen lo mismo. */}
            <span
              draggable
              aria-hidden="true"
              onDragStart={(e) => {
                setArrastrando(i);
                if (e.dataTransfer) {
                  e.dataTransfer.effectAllowed = 'move';
                  // Firefox no inicia el arrastre sin datos en el portapapeles.
                  e.dataTransfer.setData('text/plain', String(n));
                }
              }}
              onDragEnd={() => {
                setArrastrando(null);
                setSobre(null);
              }}
              class="text-text-subtle hover:text-text shrink-0 cursor-grab px-1 py-1 active:cursor-grabbing"
            >
              <Icon name="menu" size={14} />
            </span>
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
              size="sm"
              label={t('pdf.order.up', { n })}
              disabled={i === 0}
              onClick={() => mover(i, i - 1, 'arriba')}
            >
              <Icon name="chevronUp" size={14} />
            </IconButton>
            <IconButton
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
      {/* eslint-enable jsx-a11y/no-noninteractive-element-interactions */}
      <span class="sr-only" role="status">
        {aviso}
      </span>
    </fieldset>
  );
}
