/**
 * CategorySection — sección colapsable del sidebar de Sytue Rooms (Ola 11, paridad
 * Mattermost). Cabecera con overline (como SidebarSection de @sytue/ui) más:
 *   - control de colapsar/expandir (chevron, aria-expanded/aria-controls), que persiste
 *     el estado (backend en categorías personalizadas, localStorage en las por defecto);
 *   - contador de no-leídos agregado, visible al colapsar (para no ocultar la señal);
 *   - menú «…» de cabecera (renombrar/borrar/reordenar) en las categorías personalizadas;
 *   - zona de soltar (arrastrar-soltar) para mover un canal a la sección o reordenarla.
 *
 * El arrastrar-soltar NO es el único camino (WCAG AA): mover un canal tiene su entrada de
 * menú «Mover a…» en la fila, y reordenar/borrar/renombrar están en el menú de cabecera.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useState } from 'preact/hooks';
import { cx, Badge, SIDEBAR_ACTION_CLASS, Glifo } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { MessageMenu, type MenuEntry } from './MessageMenu.js';

/**
 * Disparador «…» de la cabecera: la acción del carril de `@sytue/ui`. Era una copia
 * de sus clases con `h-5 w-5`, 17,5 px con la raíz a 14.
 */
const HEADER_TRIGGER = SIDEBAR_ACTION_CLASS;

/** Chevron de colapso (rota 0/-90º). Inline: es el único punto que lo usa. */
function Chevron({ collapsed }: { collapsed: boolean }): JSX.Element {
  return (
    <Glifo
      d="m6 9 6 6 6-6"
      size={12}
      class={cx('transition-transform duration-150', collapsed ? '-rotate-90' : 'rotate-0')}
    />
  );
}

export interface CategorySectionProps {
  title: string;
  /** No-leídos agregados de la sección (se muestra al colapsar). */
  unread: number;
  collapsed: boolean;
  onToggleCollapse: () => void;
  /** Menú «…» de cabecera (renombrar/borrar/reordenar); ausente en secciones por defecto. */
  menuItems?: MenuEntry[];
  menuLabel?: string;
  /** Acción SIEMPRE visible en la cabecera (el «+» de «Crear canal»): a diferencia del
   *  menú «…», no se oculta tras el hover — es una puerta principal, no una acción rara. */
  headerAction?: ComponentChildren;
  /** DnD: ¿puede soltarse aquí lo que se arrastra ahora (un canal o una categoría)? */
  dropActive?: boolean;
  /** DnD: realiza el soltar (el padre decide según lo que se arrastra). */
  onDrop?: () => void;
  /** DnD: la cabecera es arrastrable (reordenar categorías personalizadas). */
  draggable?: boolean;
  onDragStart?: () => void;
  onDragEnd?: () => void;
  /** id estable para aria-controls (relaciona la cabecera con la lista). */
  bodyId: string;
  /** Traductor ya atado al idioma. */
  t: T;
  children?: ComponentChildren;
}

export function CategorySection({
  title,
  unread,
  collapsed,
  onToggleCollapse,
  menuItems,
  menuLabel,
  headerAction,
  dropActive = false,
  onDrop,
  draggable = false,
  onDragStart,
  onDragEnd,
  bodyId,
  t,
  children,
}: CategorySectionProps): JSX.Element {
  const [over, setOver] = useState(false);

  function handleDrop(e: DragEvent): void {
    e.preventDefault();
    setOver(false);
    if (dropActive) onDrop?.();
  }
  function handleDragOver(e: DragEvent): void {
    if (!dropActive) return;
    e.preventDefault(); // habilita el soltar
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    if (!over) setOver(true);
  }

  return (
    <div
      class={cx(
        'mb-4 rounded-md',
        dropActive && over && 'ring-2 ring-[var(--sytue-indicator)] ring-inset',
      )}
      onDragOver={handleDragOver}
      onDragLeave={() => setOver(false)}
      onDrop={handleDrop}
    >
      <div
        class="group/cat flex items-center gap-1 pr-1"
        draggable={draggable}
        onDragStart={(e) => {
          if (!draggable) return;
          if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
          onDragStart?.();
        }}
        onDragEnd={onDragEnd}
      >
        <button
          type="button"
          // Colapsar una categoría ocurre DENTRO del menú y no lo termina: sin
          // esto, en móvil plegar «General» cerraba el drawer entero.
          data-mantiene-nav
          aria-expanded={!collapsed}
          aria-controls={bodyId}
          title={
            collapsed
              ? t('category.expand', { name: title })
              : t('category.collapse', { name: title })
          }
          onClick={onToggleCollapse}
          class="sytue-overline text-sidebar-fg-muted hover:text-sidebar-fg sytue-focus-ring-chrome flex min-w-0 flex-1 items-center gap-1 rounded px-2.5 pt-1 pb-1.5 text-left transition-colors duration-150 focus-visible:outline-none"
        >
          <Chevron collapsed={collapsed} />
          <span class="min-w-0 flex-1 truncate">{title}</span>
          {/* El no-leído agregado se muestra al colapsar (las filas lo llevan al expandir). */}
          {collapsed && unread > 0 ? <Badge count>{unread}</Badge> : null}
        </button>
        {headerAction}
        {/* `pointer-coarse`: sin hover el «…» quedaba invisible pero pulsable —una diana
            fantasma sobre la que nadie acierta— y con ella se iban renombrar, reordenar y
            borrar la categoría. Con el dedo se muestra siempre. */}
        {menuItems && menuItems.length > 0 ? (
          <span class="opacity-0 transition-opacity duration-150 group-focus-within/cat:opacity-100 group-hover/cat:opacity-100 pointer-coarse:opacity-100">
            <MessageMenu
              label={menuLabel ?? t('category.actions', { name: title })}
              items={menuItems}
              triggerClass={HEADER_TRIGGER}
            />
          </span>
        ) : null}
      </div>
      <ul id={bodyId} hidden={collapsed} class="flex flex-col gap-0.5">
        {children}
      </ul>
    </div>
  );
}
