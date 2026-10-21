/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/** Panel de lista ajustable. El ancho se recuerda; en móvil ocupa la pantalla. */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useId, useRef, useState } from 'preact/hooks';
import { usePointerDrag } from '../lib/usePointerDrag.js';
import { cx } from '../lib/cx.js';

export function ResizablePanel({
  children,
  label,
  storageKey,
  detailOpen,
}: {
  children: ComponentChildren;
  label: string;
  storageKey: string;
  detailOpen: boolean;
}): JSX.Element {
  const panel = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(280);
  const [maximum, setMaximum] = useState(640);
  const id = useId();
  const minimum = 180;
  const clamp = (value: number) => Math.min(maximum, Math.max(minimum, value));
  const save = (value: number) => {
    try {
      localStorage.setItem(storageKey, String(value));
    } catch {
      // La preferencia sigue funcionando si el navegador no permite guardarla.
    }
  };

  useEffect(() => {
    try {
      const saved = Number(localStorage.getItem(storageKey));
      if (Number.isFinite(saved) && saved >= minimum) setWidth(saved);
    } catch {
      // Sin almacenamiento, se conserva el ancho inicial.
    }
    const parent = panel.current?.parentElement;
    if (!parent) return;
    const measure = () => {
      const max = Math.max(minimum, parent.clientWidth - 280);
      setMaximum(max);
      setWidth((value) => Math.min(max, Math.max(minimum, value)));
    };
    const observer = new ResizeObserver(measure);
    observer.observe(parent);
    measure();
    return () => observer.disconnect();
  }, [storageKey]);

  const drag = usePointerDrag<{ width: number; x: number }>({
    onMove: ({ data, dxRaw }) => setWidth(clamp(data.width + dxRaw)),
    onCancel: (data) => setWidth(data.width),
    onEnd: (data, e) => {
      const value = clamp(data.width + e.clientX - data.x);
      setWidth(value);
      save(value);
    },
  });

  return (
    <>
      <div
        ref={panel}
        id={id}
        class={cx(
          // Las esquinas de la IZQUIERDA de la hoja: ver la nota de AppShell.
          'sytue-sheet-corners marco:overflow-clip md:[--sytue-sheet-br:0px] md:[--sytue-sheet-tr:0px]',
          'bg-surface flex h-full min-h-0 shrink-0 flex-col max-md:w-full md:w-[var(--sytue-panel-width)]',
          detailOpen && 'max-md:hidden',
        )}
        style={{ '--sytue-panel-width': `${width}px` }}
      >
        {children}
      </div>
      <div
        role="separator"
        tabIndex={0}
        aria-label={label}
        aria-controls={id}
        aria-orientation="vertical"
        aria-valuemin={minimum}
        aria-valuemax={maximum}
        aria-valuenow={Math.round(width)}
        class="bg-border hover:bg-primary focus-visible:bg-primary z-10 w-1.5 shrink-0 cursor-col-resize touch-none select-none focus-visible:outline-none max-md:hidden"
        onPointerDown={(e) => {
          if (e.button !== 0) return;
          e.preventDefault();
          const handle = e.currentTarget as HTMLElement;
          handle.focus();
          drag.iniciarArrastre(e, { width, x: e.clientX });
        }}
        onPointerMove={drag.onPointerMove}
        onPointerUp={drag.onPointerUp}
        onPointerCancel={drag.onPointerCancel}
        onLostPointerCapture={drag.onPointerCancel}
        onDblClick={() => {
          setWidth(280);
          save(280);
        }}
        onKeyDown={(e) => {
          const value =
            e.key === 'ArrowLeft'
              ? width - 24
              : e.key === 'ArrowRight'
                ? width + 24
                : e.key === 'Home'
                  ? minimum
                  : e.key === 'End'
                    ? maximum
                    : null;
          if (value === null) return;
          e.preventDefault();
          const next = clamp(value);
          setWidth(next);
          save(next);
        }}
      />
    </>
  );
}
