/**
 * List — listas densas (mail, chat, drive). Item con icono + texto + meta y
 * estado de seleccion. role=list/listbox, roving tabindex + flechas, aria-selected.
 * docs/design.md §9.
 */
import type { ComponentChildren, JSX, TargetedEvent } from 'preact';
import { createContext } from 'preact';
import { useContext, useEffect, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';

interface ListFocusContextValue {
  focusedIndex: number;
  setFocusedIndex: (index: number) => void;
}

const ListFocusContext = createContext<ListFocusContextValue | null>(null);

export interface ListProps extends JSX.HTMLAttributes<HTMLUListElement> {
  /** Si es seleccionable se expone como listbox. */
  selectable?: boolean;
  label?: string;
}

export function List({
  selectable = false,
  label,
  class: cls,
  children,
  onKeyDown,
  ...rest
}: ListProps): JSX.Element {
  const [focusedIndex, setFocusedIndex] = useState(0);

  const handleKeyDown = (e: TargetedEvent<HTMLUListElement, KeyboardEvent>) => {
    if (selectable && ['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) {
      const options = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('[role="option"]'));
      const count = options.length;
      if (count > 0) {
        // El índice actual se deriva del DOM (no del estado): sobrevive a
        // filtrados/recargas que desplacen los índices, y a foco por ratón.
        const domIndex = options.indexOf(e.target as HTMLElement);
        const current = domIndex >= 0 ? domIndex : Math.min(focusedIndex, count - 1);
        let next = current;
        if (e.key === 'ArrowDown') next = Math.min(current + 1, count - 1);
        else if (e.key === 'ArrowUp') next = Math.max(current - 1, 0);
        else if (e.key === 'Home') next = 0;
        else next = count - 1;
        e.preventDefault();
        if (next !== current || domIndex < 0) {
          setFocusedIndex(next);
          options[next]?.focus();
        }
      }
    }
    onKeyDown?.(e);
  };

  // onKeyDown solo se adjunta al listbox interactivo (selectable) o si el
  // consumidor lo pasa; una lista plana (role="list") no es interactiva.
  const list = (
    <ul
      role={selectable ? 'listbox' : 'list'}
      aria-label={label}
      class={cx('flex flex-col', cls)}
      {...(selectable || onKeyDown ? { onKeyDown: handleKeyDown } : {})}
      {...rest}
    >
      {children}
    </ul>
  );

  if (!selectable) return list;

  return (
    <ListFocusContext.Provider value={{ focusedIndex, setFocusedIndex }}>
      {list}
    </ListFocusContext.Provider>
  );
}

export interface ListItemProps extends Omit<JSX.HTMLAttributes<HTMLLIElement>, 'icon' | 'title'> {
  leading?: ComponentChildren;
  title: ComponentChildren;
  subtitle?: ComponentChildren;
  meta?: ComponentChildren;
  selected?: boolean;
  /** Marca el item como activo/no leido (acento lateral). */
  unread?: boolean;
  /** Si el item es interactivo (selectable list). */
  interactive?: boolean;
  /** Índice en la lista (obligatorio para roving tabindex en listbox). */
  index?: number;
}

export function ListItem({
  leading,
  title,
  subtitle,
  meta,
  selected,
  unread,
  interactive = true,
  index,
  class: cls,
  onFocus,
  onKeyDown,
  ...rest
}: ListItemProps): JSX.Element {
  const focusCtx = useContext(ListFocusContext);
  // El List solo provee ListFocusContext cuando es `selectable` (role=listbox).
  // `role=option` fuera de un listbox es ARIA inválido, así que solo lo emitimos
  // dentro de un listbox; en una lista plana el item queda como <li> (listitem).
  const inListbox = focusCtx !== null;

  // Item interactivo FUERA de un listbox = role=button: Enter/Espacio deben
  // activarlo como el ratón (WCAG 2.1.1). En listbox el List gestiona las
  // flechas y la selección, así que ahí no interceptamos teclado.
  const handleKeyDown = (e: TargetedEvent<HTMLLIElement, KeyboardEvent>) => {
    if (interactive && !inListbox && (e.key === 'Enter' || e.key === ' ')) {
      if (e.key === ' ') e.preventDefault(); // evita el scroll de página
      e.currentTarget.click();
    }
    onKeyDown?.(e);
  };

  useEffect(() => {
    if (selected && focusCtx && index !== undefined) {
      focusCtx.setFocusedIndex(index);
    }
  }, [selected, focusCtx, index]);

  const tabIndex =
    interactive && focusCtx && index !== undefined
      ? focusCtx.focusedIndex === index
        ? 0
        : -1
      : interactive
        ? 0
        : undefined;

  // Tooltip nativo para textos truncados (solo si son cadenas simples).
  const tooltip =
    [title, subtitle].filter((v): v is string => typeof v === 'string').join(' — ') || undefined;
  return (
    // El li lleva role=button/option cuando es interactivo y el onKeyDown ES el
    // arreglo de teclado (WCAG 2.1.1); el role dinámico despista a la regla.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <li
      role={interactive ? (inListbox ? 'option' : 'button') : undefined}
      aria-selected={interactive && inListbox ? Boolean(selected) : undefined}
      tabIndex={tabIndex}
      title={tooltip}
      onKeyDown={handleKeyDown}
      onFocus={(e) => {
        if (focusCtx && index !== undefined) focusCtx.setFocusedIndex(index);
        onFocus?.(e);
      }}
      class={cx(
        // Forma: en «Marco» la fila es más alta y, elegida o bajo el cursor, un bloque
        // redondeado que se separa de los bordes (el texto no se mueve: lo que entra de
        // margen sale de relleno). En «Carril» los tokens valen lo de siempre; el radio lo
        // pone `sytue-row` solo en «Marco» (appearance.css): declarado aquí, en «Carril»
        // le quitaría al aro de foco su esquina de 4 px.
        'sytue-row mx-[var(--sytue-row-inset)] flex items-center gap-3 border-l-[length:var(--sytue-row-bar)] text-base',
        'px-[calc(0.75rem-var(--sytue-row-inset))] py-[calc(0.5rem+var(--sytue-row-grow))]',
        'transition-colors duration-150',
        interactive && 'cursor-pointer focus-visible:outline-none',
        selected
          ? 'border-l-primary bg-selected'
          : cx(
              'border-l-transparent',
              interactive && 'hover:bg-bg-subtle focus-visible:bg-bg-subtle',
            ),
        cls,
      )}
      {...rest}
    >
      {leading ? <span class="shrink-0">{leading}</span> : null}
      <span class="flex min-w-0 flex-1 flex-col">
        <span
          class={cx(
            unread ? 'text-primary-text font-bold' : 'text-text',
            typeof title === 'string' ? 'truncate' : 'min-w-0',
            unread ? 'font-bold' : 'font-medium',
          )}
        >
          {title}
        </span>
        {subtitle ? (
          <span
            class={cx(
              unread ? 'text-primary-text text-sm font-bold' : 'text-text-muted text-sm',
              typeof subtitle === 'string' ? 'truncate' : 'min-w-0',
            )}
          >
            {subtitle}
          </span>
        ) : null}
      </span>
      {meta ? <span class="sytue-meta shrink-0 whitespace-nowrap tabular-nums">{meta}</span> : null}
    </li>
  );
}
