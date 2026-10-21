/**
 * MessageMenu — menú de acciones con disparador propio. Nació para el «…» de cada
 * mensaje (paridad Mattermost) y sirve también para el NOMBRE del canal en su
 * cabecera: `trigger` sustituye el glifo por lo que haga falta, y toda la gestión de
 * foco, teclado y portal se reutiliza en vez de escribirse dos veces. Reusa el
 * lenguaje del Dropdown de @sytue/ui (role=menu, flechas/Home/End/Esc, foco
 * gestionado, aria-haspopup/expanded) PERO se renderiza en un portal con posición
 * fija anclada al disparador: el histórico va en un scroller virtualizado
 * (overflow + translateY por fila), que recortaría un menú `absolute` y del que un
 * `position:fixed` normal no escapa (ancestro con transform). El portal a
 * document.body lo evita y permite voltear hacia arriba cerca del borde inferior.
 *
 * ponytail: alto del menú estimado (nº de ítems) para decidir el volteo; si algún
 * día los ítems varían mucho de alto, medir tras montar.
 */
import type { JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useRef, useState } from 'preact/hooks';
import { MenuItem, MenuSeparator, limitesViewport, useFitInViewport } from '@sytue/ui';
import { MoreGlyph } from './glyphs.js';
import { useCierreDePopover } from '../lib/popover.js';

export interface MenuAction {
  key: string;
  label: string;
  icon?: JSX.Element;
  onSelect: () => void;
  danger?: boolean;
}

/** Ítem del menú o separador. */
export type MenuEntry = MenuAction | 'separator';

const MENU_W = 232;

/** Clases por defecto del disparador «…» (histórico, tema claro). El sidebar pasa
 *  las suyas vía triggerClass para contrastar sobre el fondo oscuro. */
const DEFAULT_TRIGGER =
  'flex h-7 w-7 items-center justify-center rounded text-text-muted transition-colors duration-150 hover:bg-bg-subtle hover:text-text sytue-focus-ring';

export function MessageMenu({
  label,
  items,
  triggerClass,
  trigger,
}: {
  label: string;
  items: MenuEntry[];
  /** Sobrescribe las clases del botón disparador (p.ej. para el sidebar oscuro). */
  triggerClass?: string;
  /** Contenido del disparador. Por defecto el glifo «…»; la cabecera del canal pasa
   *  su nombre con un chevron, que es donde el usuario espera pulsar. */
  trigger?: JSX.Element;
}): JSX.Element {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useFitInViewport(menuRef, open, { vertical: true });

  const actionCount = items.filter((i) => i !== 'separator').length;

  function close(refocus = true): void {
    setOpen(false);
    if (refocus) triggerRef.current?.focus();
  }

  function openMenu(): void {
    const el = triggerRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const v = limitesViewport(8);
    const estH = Math.min(actionCount * 40 + 12, v.height); // ~40px por ítem + padding
    const top =
      r.bottom + 4 + estH <= v.bottom
        ? Math.max(v.top, r.bottom + 4)
        : Math.max(v.top, Math.min(r.top - estH - 4, v.bottom - estH));
    const ancho = Math.min(MENU_W, v.width);
    const left = Math.max(v.left, Math.min(r.right - ancho, v.right - ancho));
    setPos({ top, left });
    setOpen(true);
  }

  useEffect(() => {
    if (!open) return;
    // Foco al primer ítem al abrir.
    menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
  }, [open]);

  // Mismo cierre que el selector de emoji: el teclado virtual no cuenta como
  // cambio de tamaño y el scroll de dentro no cuenta como scroll (ver
  // lib/popover.ts). Aquí el síntoma era más callado —este menú no enfoca un
  // input— pero la regla equivocada era la misma.
  useCierreDePopover(open, () => setOpen(false), [menuRef, triggerRef]);

  function onMenuKeyDown(e: KeyboardEvent): void {
    const els = Array.from(
      menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [],
    );
    if (els.length === 0) return;
    const idx = els.indexOf(document.activeElement as HTMLElement);
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        close();
        break;
      case 'ArrowDown':
        e.preventDefault();
        (els[(idx + 1) % els.length] ?? els[0]!).focus();
        break;
      case 'ArrowUp':
        e.preventDefault();
        (els[(idx - 1 + els.length) % els.length] ?? els[0]!).focus();
        break;
      case 'Home':
        e.preventDefault();
        els[0]!.focus();
        break;
      case 'End':
        e.preventDefault();
        els[els.length - 1]!.focus();
        break;
      case 'Tab':
        // Dejar salir el foco con Tab cierra el menú (sin robar el foco).
        setOpen(false);
        break;
    }
  }

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={label}
        title={label}
        onClick={() => (open ? close() : openMenu())}
        class={triggerClass ?? DEFAULT_TRIGGER}
      >
        {trigger ?? <MoreGlyph size={16} />}
      </button>
      {open && pos
        ? createPortal(
            <div
              ref={menuRef}
              role="menu"
              tabIndex={-1}
              aria-label={label}
              onKeyDown={onMenuKeyDown}
              style={{
                position: 'fixed',
                top: `${pos.top}px`,
                left: `${pos.left}px`,
                width: `${MENU_W}px`,
                zIndex: 'var(--z-dropdown)',
              }}
              class="sytue-pop-in sytue-menu-surface border-border rounded-lg border p-1 shadow-[var(--shadow-md)]"
            >
              {items.map((it, i) =>
                it === 'separator' ? (
                  <MenuSeparator key={`sep-${i}`} />
                ) : (
                  <MenuItem
                    key={it.key}
                    danger={it.danger ?? false}
                    {...(it.icon ? { icon: it.icon } : {})}
                    onClick={() => {
                      close();
                      it.onSelect();
                    }}
                  >
                    {it.label}
                  </MenuItem>
                ),
              )}
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
