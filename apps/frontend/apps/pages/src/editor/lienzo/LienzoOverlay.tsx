/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Capa de interacción superpuesta sobre el lienzo del editor (PAGES-PLAN §14.1).
 *
 * Responsabilidades:
 * - Detección de nodos mediante contentDocument.elementsFromPoint -> closest('[data-node]').
 * - Caja de selección con 8 tiradores de redimensión utilizando usePointerDrag y calcularRedimension.
 * - Caja de resaltado al pasar el ratón (hover).
 * - Línea indicadora de inserción en flujo.
 */

/* oxlint-disable jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */

import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { calcularRedimension, usePointerDrag } from '@sytue/ui';
import type { Rectangulo, TiradorRedimension } from '@sytue/ui';
import type { Tamano } from '@sytue/pages-render';

export interface RectanguloRelativo {
  top: number;
  left: number;
  width: number;
  height: number;
}

export interface IndicadorInsercion {
  padreId: string;
  indice: number;
  eje: 'row' | 'column';
  x: number;
  y: number;
  longitud: number;
}

export interface LienzoOverlayProps {
  documentoIframe: Document | null;
  nodoSeleccionadoId: string | null;
  zoom: number;
  onSeleccionar: (id: string | null, acumulativo: boolean) => void;
  onRedimensionar: (id: string, nuevoTamano: Tamano) => void;
  indicadorInsercion?: IndicadorInsercion | null | undefined;
}

interface EstadoArrastreTirador {
  tirador: TiradorRedimension;
  rectInicial: Rectangulo;
}

const TIRADORES: readonly TiradorRedimension[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'];

export function LienzoOverlay({
  documentoIframe,
  nodoSeleccionadoId,
  zoom,
  onSeleccionar,
  onRedimensionar,
  indicadorInsercion,
}: LienzoOverlayProps): JSX.Element {
  const contenedorRef = useRef<HTMLDivElement>(null);
  const [hoverId, setHoverId] = useState<string | null>(null);
  const [rectSeleccion, setRectSeleccion] = useState<RectanguloRelativo | null>(null);
  const [rectHover, setRectHover] = useState<RectanguloRelativo | null>(null);
  const [tamanoArrastre, setTamanoArrastre] = useState<{ width: number; height: number } | null>(
    null,
  );

  // Calcula el rectángulo relativo al body del iframe
  const medirElemento = useCallback(
    (id: string): RectanguloRelativo | null => {
      if (!documentoIframe) return null;
      try {
        const el = documentoIframe.querySelector(`[data-node="${id}"]`);
        if (!el || !documentoIframe.body) return null;

        const elRect = el.getBoundingClientRect();
        const bodyRect = documentoIframe.body.getBoundingClientRect();

        return {
          top: elRect.top - bodyRect.top,
          left: elRect.left - bodyRect.left,
          width: elRect.width,
          height: elRect.height,
        };
      } catch {
        return null;
      }
    },
    [documentoIframe],
  );

  // Actualiza la caja de selección cuando cambia el nodo seleccionado o el documento
  useEffect(() => {
    if (!nodoSeleccionadoId) {
      setRectSeleccion(null);
      return;
    }
    const r = medirElemento(nodoSeleccionadoId);
    setRectSeleccion(r);
  }, [nodoSeleccionadoId, documentoIframe, medirElemento]);

  // Actualiza la caja de hover
  useEffect(() => {
    if (!hoverId || hoverId === nodoSeleccionadoId) {
      setRectHover(null);
      return;
    }
    const r = medirElemento(hoverId);
    setRectHover(r);
  }, [hoverId, nodoSeleccionadoId, medirElemento]);

  // Hook de arrastre para los 8 tiradores de redimensión
  const {
    iniciarArrastre,
    onPointerMove: dragMove,
    onPointerUp: dragUp,
  } = usePointerDrag<EstadoArrastreTirador>({
    escala: zoom,
    onMove: ({ data, dxRaw, dyRaw }) => {
      const redim = calcularRedimension(
        data.rectInicial,
        dxRaw,
        dyRaw,
        16, // lado mínimo 16 px
        2400,
        data.tirador,
      );
      setTamanoArrastre({ width: Math.round(redim.w), height: Math.round(redim.h) });
    },
    onEnd: () => {
      if (tamanoArrastre && nodoSeleccionadoId) {
        onRedimensionar(nodoSeleccionadoId, {
          width: tamanoArrastre.width,
          height: tamanoArrastre.height,
        });
      }
      setTamanoArrastre(null);
    },
  });

  // Manejo de movimiento de puntero para hit-testing en el iframe
  const handlePointerMove = (e: PointerEvent) => {
    dragMove(e);
    if (!documentoIframe || !contenedorRef.current) return;

    const overlayRect = contenedorRef.current.getBoundingClientRect();
    const x = (e.clientX - overlayRect.left) / zoom;
    const y = (e.clientY - overlayRect.top) / zoom;

    try {
      const elementos = documentoIframe.elementsFromPoint(x, y);
      for (const el of elementos) {
        const nodoEl = el.closest('[data-node]');
        if (nodoEl) {
          const id = nodoEl.getAttribute('data-node');
          if (id) {
            setHoverId(id);
            return;
          }
        }
      }
      setHoverId(null);
    } catch {
      setHoverId(null);
    }
  };

  const handlePointerLeave = () => {
    setHoverId(null);
  };

  const handleClick = (e: MouseEvent) => {
    if (e.defaultPrevented) return;
    if (hoverId) {
      onSeleccionar(hoverId, e.shiftKey);
    } else {
      onSeleccionar(null, false);
    }
  };

  // Posición de los 8 tiradores
  const obtenerEstiloTirador = (t: TiradorRedimension) => {
    const estilo: Record<string, string> = {
      position: 'absolute',
      width: '10px',
      height: '10px',
      backgroundColor: '#ffffff',
      border: '1.5px solid var(--sytue-focus-ring)',
      borderRadius: '2px',
      zIndex: '20',
      pointerEvents: 'auto',
    };

    switch (t) {
      case 'nw':
        return { ...estilo, top: '-5px', left: '-5px', cursor: 'nwse-resize' };
      case 'n':
        return { ...estilo, top: '-5px', left: 'calc(50% - 5px)', cursor: 'ns-resize' };
      case 'ne':
        return { ...estilo, top: '-5px', right: '-5px', cursor: 'nesw-resize' };
      case 'e':
        return { ...estilo, top: 'calc(50% - 5px)', right: '-5px', cursor: 'ew-resize' };
      case 'se':
        return { ...estilo, bottom: '-5px', right: '-5px', cursor: 'nwse-resize' };
      case 's':
        return { ...estilo, bottom: '-5px', left: 'calc(50% - 5px)', cursor: 'ns-resize' };
      case 'sw':
        return { ...estilo, bottom: '-5px', left: '-5px', cursor: 'nesw-resize' };
      case 'w':
        return { ...estilo, top: 'calc(50% - 5px)', left: '-5px', cursor: 'ew-resize' };
    }
  };

  const anchoCaja = tamanoArrastre ? tamanoArrastre.width : (rectSeleccion?.width ?? 0);
  const altoCaja = tamanoArrastre ? tamanoArrastre.height : (rectSeleccion?.height ?? 0);

  // oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- superficie técnica para hit-testing y arrastre sobre el iframe aislado
  return (
    <div
      ref={contenedorRef}
      onPointerMove={handlePointerMove}
      onPointerUp={dragUp}
      onPointerLeave={handlePointerLeave}
      onClick={handleClick}
      class="pointer-events-auto absolute inset-0 cursor-default select-none"
      style={{ overflow: 'visible' }}
    >
      {/* Caja de hover */}
      {rectHover && (
        <div
          class="border-primary/80 bg-primary/5 pointer-events-none absolute border border-dashed transition-all duration-75"
          style={{
            top: `${rectHover.top}px`,
            left: `${rectHover.left}px`,
            width: `${rectHover.width}px`,
            height: `${rectHover.height}px`,
          }}
        />
      )}

      {/* Caja de selección activa */}
      {rectSeleccion && (
        <div
          class="bg-selected pointer-events-none absolute shadow-xs outline-2 outline-[var(--sytue-focus-ring)]"
          style={{
            top: `${rectSeleccion.top}px`,
            left: `${rectSeleccion.left}px`,
            width: `${anchoCaja}px`,
            height: `${altoCaja}px`,
          }}
        >
          {/* Etiqueta del nodo */}
          <div class="bg-primary pointer-events-none absolute -top-5 left-0 rounded px-1 py-0.5 font-mono text-[9px] font-medium text-[var(--sytue-on-primary)] shadow-xs">
            {nodoSeleccionadoId} ({Math.round(anchoCaja)}×{Math.round(altoCaja)})
          </div>

          {/* 8 Tiradores de redimensión */}
          {TIRADORES.map((t) => (
            <div
              key={t}
              style={obtenerEstiloTirador(t)}
              onPointerDown={(e) => {
                iniciarArrastre(e, {
                  tirador: t,
                  rectInicial: {
                    x: rectSeleccion.left,
                    y: rectSeleccion.top,
                    w: rectSeleccion.width,
                    h: rectSeleccion.height,
                  },
                });
              }}
            />
          ))}
        </div>
      )}

      {/* Línea indicadora de inserción en flujo */}
      {indicadorInsercion && (
        <div
          class="bg-primary pointer-events-none absolute z-30 shadow-md"
          style={
            indicadorInsercion.eje === 'row'
              ? {
                  top: `${indicadorInsercion.y}px`,
                  left: `${indicadorInsercion.x}px`,
                  width: '3px',
                  height: `${indicadorInsercion.longitud}px`,
                  borderRadius: '2px',
                }
              : {
                  top: `${indicadorInsercion.y}px`,
                  left: `${indicadorInsercion.x}px`,
                  width: `${indicadorInsercion.longitud}px`,
                  height: '3px',
                  borderRadius: '2px',
                }
          }
        >
          {/* Puntos en los extremos */}
          <div class="bg-primary absolute -top-1 -left-1 h-2.5 w-2.5 rounded-full" />
          <div
            class="bg-primary absolute h-2.5 w-2.5 rounded-full"
            style={
              indicadorInsercion.eje === 'row'
                ? { bottom: '-4px', left: '-4px' }
                : { right: '-4px', top: '-4px' }
            }
          />
        </div>
      )}
    </div>
  );
}
