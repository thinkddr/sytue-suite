/**
 * Tabs — navegacion segmentada dentro de una vista. tablist/tab/tabpanel,
 * flechas para moverse, aria-selected. docs/design.md §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';

export interface TabItem {
  id: string;
  label: string;
  icon?: JSX.Element;
  content: ComponentChildren;
}

export interface TabsProps {
  items: TabItem[];
  /** Pestaña activa inicial (id). */
  defaultTab?: string;
  /**
   * Pestaña activa CONTROLADA (id). Con ella manda quien llama: el componente solo avisa por
   * `onChange`. Hace falta cuando la pestaña vive también fuera del componente —el `#hash` de
   * la URL— y `defaultTab` (solo el valor inicial) no basta para leerlo tras hidratar.
   */
  value?: string;
  /** Se llama con el id de la pestaña elegida, por clic o por teclado. */
  onChange?: (id: string) => void;
  /** Nombre accesible de la tira de pestañas, cuando la vista tiene más de un grupo. */
  label?: string;
  class?: string;
}

export function Tabs({
  items,
  defaultTab,
  value,
  onChange,
  label,
  class: cls,
}: TabsProps): JSX.Element {
  const baseId = useId();
  const [interna, setInterna] = useState(defaultTab ?? items[0]?.id ?? '');
  const active = value ?? interna;
  const setActive = (id: string) => {
    setInterna(id);
    onChange?.(id);
  };
  // La pestaña activa, a la vista dentro de la tira que se desliza: en el teléfono, seis
  // pestañas miden 582 px en 342 y la elegida podía quedar fuera. Solo al CAMBIAR: al
  // montar, desplazaría la página hasta unas pestañas que estuvieran más abajo.
  const [montada, setMontada] = useState(false);
  useEffect(() => {
    if (!montada) {
      setMontada(true);
      return;
    }
    document
      .getElementById(`${baseId}-tab-${active}`)
      ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  }, [active, baseId, montada]);

  const onKeyDown = (e: KeyboardEvent, index: number) => {
    let nextIndex: number;
    if (e.key === 'ArrowRight') nextIndex = (index + 1) % items.length;
    else if (e.key === 'ArrowLeft') nextIndex = (index - 1 + items.length) % items.length;
    else if (e.key === 'Home') nextIndex = 0;
    else if (e.key === 'End') nextIndex = items.length - 1;
    else return;
    e.preventDefault();
    const next = items[nextIndex];
    if (next) {
      setActive(next.id);
      document.getElementById(`${baseId}-tab-${next.id}`)?.focus();
    }
  };

  return (
    <div class={cls}>
      {/* La tira se DESLIZA en horizontal en vez de estirar la vista. Cinco
          pestañas de texto largo («Admins globales») no caben en 360px, y un
          `flex` a secas empuja el ancho del documento entero: la página se puede
          desplazar de lado y todo se lee cortado, cabecera incluida.

          `data-scroll-x` marca el desplazamiento como deliberado para la
          auditoría de móvil (test/e2e/mobile-audit.mjs), y `shrink-0` en cada
          pestaña evita lo otro que pasa al estrechar: que el rótulo se parta en
          dos líneas en vez de dejarse deslizar. Aquí es seguro dar overflow —a
          diferencia de la fila de grupos del ribbon de Docs— porque una pestaña
          no abre ningún popover que el `overflow-y:auto` de rebote pudiera
          recortar. */}
      <div
        role="tablist"
        aria-label={label}
        data-scroll-x
        class="border-border flex gap-1 overflow-x-auto border-b"
        aria-orientation="horizontal"
      >
        {items.map((item, i) => {
          const selected = item.id === active;
          return (
            <button
              key={item.id}
              id={`${baseId}-tab-${item.id}`}
              role="tab"
              type="button"
              aria-selected={selected}
              aria-controls={`${baseId}-panel-${item.id}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => setActive(item.id)}
              onKeyDown={(e) => onKeyDown(e, i)}
              class={cx(
                // Versalitas con aire, la pestaña de oficina que se lee como índice de
                // un archivador.
                'inline-flex shrink-0 items-center gap-2 border-b-2 px-3 py-2.5 text-sm font-semibold tracking-[0.06em] whitespace-nowrap uppercase pointer-coarse:min-h-[max(44px,2.75rem)]',
                // `sytue-focus-ring-inner` es exactamente este box-shadow (styles.css
                // §focus): se escribía a mano aquí, en el componente canónico de
                // pestañas, que es justo desde donde se copia al resto.
                'sytue-focus-ring-inner transition-colors duration-150',
                // La activa con el subrayado de «estás aquí» (cobre). El color lo lleva
                // el subrayado; si también lo llevara el texto saldrían dos colores en
                // una palabra.
                selected
                  ? 'text-text border-indicator'
                  : 'text-text-muted hover:border-border-strong hover:text-text border-transparent',
              )}
            >
              {item.icon}
              {item.label}
            </button>
          );
        })}
      </div>
      {items.map((item) => (
        <div
          key={item.id}
          id={`${baseId}-panel-${item.id}`}
          role="tabpanel"
          aria-labelledby={`${baseId}-tab-${item.id}`}
          hidden={item.id !== active}
          tabIndex={0}
          class="sytue-focus-ring-inner pt-4"
        >
          {item.id === active ? item.content : null}
        </div>
      ))}
    </div>
  );
}
