/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El lienzo de la rúbrica, compartido por la firma del estudio (`Paneles.tsx`) y la
 * firma en el dispositivo (`FirmarEnDispositivo.tsx`): los mismos trazos, con el
 * mismo tamaño de lienzo, acaban en la misma apariencia visible.
 */
import { useEffect, useRef } from 'preact/hooks';
import { ALTO_LIENZO, ANCHO_LIENZO } from './firmaPosicion';

/**
 * Lienzo para firmar de su puño y letra.
 *
 * Va con eventos de PUNTERO y no de ratón ni de toque, y esa es toda la razón de
 * que funcione en el móvil: `pointer*` cubre dedo, lápiz y ratón con el mismo
 * código, y `touch-action: none` impide que el navegador se lleve el gesto como
 * un desplazamiento de la página, que es lo que convierte firmar en pelearse con
 * el scroll.
 *
 * Los trazos se guardan en coordenadas del lienzo y se traducen al salir: en un
 * PDF el eje vertical crece hacia ARRIBA y en un lienzo hacia abajo, así que sin
 * invertirlo la firma sale del revés.
 */
export function LienzoDeFirma({
  trazos,
  onCambio,
  etiqueta,
}: {
  trazos: [number, number][][];
  onCambio: (t: [number, number][][]) => void;
  etiqueta: string;
}) {
  const ref = useRef<HTMLCanvasElement | null>(null);
  const dibujandoRef = useRef(false);

  useEffect(() => {
    const c = ref.current;
    const ctx = c?.getContext('2d');
    if (!c || !ctx) return;
    ctx.clearRect(0, 0, c.width, c.height);
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.strokeStyle = '#1f2937';
    for (const trazo of trazos) {
      if (trazo.length === 0) continue;
      ctx.beginPath();
      ctx.moveTo(trazo[0]![0], trazo[0]![1]);
      for (const [x, y] of trazo.slice(1)) ctx.lineTo(x, y);
      ctx.stroke();
    }
  }, [trazos]);

  const punto = (e: PointerEvent): [number, number] => {
    const c = ref.current;
    if (!c) return [0, 0];
    const r = c.getBoundingClientRect();
    // El lienzo se dibuja a su resolución interna aunque CSS lo estire.
    return [
      ((e.clientX - r.left) / r.width) * c.width,
      ((e.clientY - r.top) / r.height) * c.height,
    ];
  };

  return (
    <div class="flex flex-col gap-1">
      <canvas
        ref={ref}
        width={ANCHO_LIENZO}
        height={ALTO_LIENZO}
        aria-label={etiqueta}
        class="border-border bg-surface w-full cursor-crosshair touch-none rounded-md border"
        onPointerDown={(e) => {
          dibujandoRef.current = true;
          e.currentTarget.setPointerCapture(e.pointerId);
          onCambio([...trazos, [punto(e)]]);
        }}
        onPointerMove={(e) => {
          if (!dibujandoRef.current || trazos.length === 0) return;
          const resto = trazos.slice(0, -1);
          const ultimo = [...trazos[trazos.length - 1]!, punto(e)];
          onCambio([...resto, ultimo]);
        }}
        onPointerUp={() => {
          dibujandoRef.current = false;
        }}
        onPointerCancel={() => {
          dibujandoRef.current = false;
        }}
      />
    </div>
  );
}
