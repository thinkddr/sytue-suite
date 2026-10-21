/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Esquema» (BLUEPRINT.md §6.4): lista de los h1–h3 del lienzo con
 * sangría por nivel, clicable para desplazarse al título. Se refresca al
 * abrirse, en cada transición del guardado a «saved» y cuando el contenido se
 * reemplaza por debajo (adopción remota o restauración de versión).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { getEditorRoot } from '../canvas/exec';
import type { EditorContext } from '../types';

interface OutlineItem {
  /** Nivel del título: 1..3 (h1..h3). */
  level: number;
  text: string;
  el: HTMLElement;
}

/** Sangría por nivel (§6.4: pl-2/4/6). */
const INDENT: Record<number, string> = { 1: 'pl-2', 2: 'pl-4', 3: 'pl-6' };

const ROW_BTN =
  'block w-full truncate rounded-sm py-1 pr-2 text-left text-sm text-text hover:bg-bg-subtle ' +
  ' sytue-focus-ring-inner';

export function OutlinePanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const [items, setItems] = useState<OutlineItem[]>([]);

  const refresh = useCallback((): void => {
    const root = getEditorRoot();
    if (root === null) {
      setItems([]);
      return;
    }
    const found: OutlineItem[] = [];
    root.querySelectorAll<HTMLElement>('h1, h2, h3').forEach((el) => {
      found.push({
        level: Number(el.tagName.charAt(1)),
        text: (el.textContent ?? '').trim() || t('outline.untitled'),
        el,
      });
    });
    setItems(found);
  }, [t]);

  // Al abrir y en cada reemplazo del contenido. El setTimeout(0) da margen a
  // que el lienzo registre su root cuando panel y página montan a la vez.
  useEffect(() => {
    const timer = window.setTimeout(refresh, 0);
    return () => window.clearTimeout(timer);
  }, [ctx.contentEpoch, refresh]);

  // En cada transición del guardado a 'saved' (§6.4).
  const prevSaveRef = useRef(ctx.save);
  useEffect(() => {
    const before = prevSaveRef.current;
    prevSaveRef.current = ctx.save;
    if (ctx.save === 'saved' && before !== 'saved') refresh();
  }, [ctx.save, refresh]);

  const goTo = (item: OutlineItem): void => {
    if (!item.el.isConnected) {
      refresh(); // el DOM cambió por debajo: se rehace la lista en vez de saltar a un nodo suelto
      return;
    }
    item.el.scrollIntoView({
      block: 'start',
      behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    });
  };

  if (items.length === 0) {
    return <p class="text-text-muted p-3 text-sm">{t('outline.empty')}</p>;
  }

  return (
    <nav aria-label={t('outline.label')} class="px-1 py-3">
      <ul class="flex flex-col gap-px">
        {items.map((item, index) => (
          <li key={`${index}-${item.text}`}>
            <button
              type="button"
              class={`${ROW_BTN} ${INDENT[item.level] ?? 'pl-2'} ${item.level === 1 ? 'font-medium' : ''}`}
              title={item.text}
              onClick={() => goTo(item)}
            >
              {item.text}
            </button>
          </li>
        ))}
      </ul>
    </nav>
  );
}
