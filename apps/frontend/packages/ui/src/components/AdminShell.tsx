/**
 * AdminShell — patron compartido sidebar + contenido para las apps de
 * administración (Console, Cloud) que viven bajo la Sytue Shell (topbar sticky).
 * En ≥768px la sidebar va en flujo, como siempre; en <768px se convierte en un
 * drawer sobre overlay con botón de menú (aria-expanded/aria-controls), foco
 * gestionado (entra al abrir, vuelve al toggle al cerrar) y Esc para cerrar.
 * docs/design.md §2 / §9 / §10.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { useNavDrawer } from '../lib/useNavDrawer.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';

export interface AdminShellProps {
  /** Sidebar (típicamente un <Sidebar> de @sytue/ui). */
  sidebar: ComponentChildren;
  /** Título de la barra móvil (<768px), junto al botón de menú. */
  title?: string;
  children: ComponentChildren;
  class?: string;
}

export function AdminShell({ sidebar, title, children, class: cls }: AdminShellProps): JSX.Element {
  const t = useT();
  const [open, setOpen] = useState(false);
  const base = useId();
  const drawerId = `sytue-admin-nav-${base}`;
  const toggleId = `sytue-admin-menu-${base}`;
  useNavDrawer(open, drawerId, toggleId, () => setOpen(false));

  return (
    // Alto fijo (viewport menos la topbar sticky de la Sytue Shell): la sidebar
    // llena toda la altura y el contenido desplaza por dentro — sin doble scroll.
    // `sytue-frame` y `sytue-sheet`: el marco y la hoja de la forma «Marco», como en
    // AppShell (tokens/appearance.css). En «Carril» no hacen nada.
    <div
      class={cx(
        'sytue-frame sytue-safe-workspace flex h-[calc(100dvh_-_var(--spacing-topbar))] min-h-0',
        cls,
      )}
    >
      {open ? (
        <div
          // EMPIEZA BAJO EL TOPBAR. Con `inset-0` el velo cubría también la
          // barra superior de la Sytue Shell —el buscador global, el selector de
          // apps y la cuenta— porque `--z-overlay` (1050) está por encima de
          // `--z-topbar` (1040). Acotarlo por arriba es mejor que rebajar el z:
          // la escala se queda como está y el menú sigue tapando el contenido,
          // que es lo único que tiene que tapar.
          class="sytue-fade-in fixed inset-x-0 bottom-0 bg-[var(--sytue-overlay)] md:hidden"
          style={{ top: 'var(--spacing-topbar)', zIndex: 'var(--z-overlay)' }}
          onClick={() => setOpen(false)}
          aria-hidden="true"
        />
      ) : null}
      {/* Delegador, no un <aside> clicable: el `onClick` escucha lo que burbujea
          desde controles que ya son accesibles por teclado. Un `click` de Enter o
          Espacio burbujea igual, así que un `onKeyDown` aquí sobra. */}
      {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions */}
      <aside
        id={drawerId}
        tabIndex={-1}
        class={cx(
          'sytue-nav-drawer shrink-0 focus-visible:outline-none',
          // <768px: drawer fijo a la izquierda, por encima del overlay y POR
          // DEBAJO del topbar: arranca a la altura de la barra en vez de en el
          // borde de la pantalla. Ver la nota del velo.
          'max-md:fixed max-md:top-[var(--spacing-topbar)] max-md:bottom-0 max-md:left-0',
          // El z de modal es SOLO para el drawer móvil, que sí debe tapar el
          // contenido. Aplicarlo también en escritorio dejaba la barra lateral
          // (1060) por encima de la topbar (1040): como la topbar crea contexto
          // de apilamiento, su popover —el selector de apps— no podía escapar y
          // se veía POR DEBAJO del menú lateral en Cloud y en Admin.
          'z-[var(--z-sidebar)] max-md:z-[var(--z-modal)]',
          open ? 'sytue-drawer-in' : 'max-md:hidden',
        )}
        // Activar algo del menú lo cierra, en móvil: mismo motivo y misma forma
        // que en AppShell. Aquí el drawer va con z de MODAL, así que lo que se
        // abra detrás queda tapado del todo y parece que no ha pasado nada.
        // Los controles que actúan DENTRO del propio menú se marcan con
        // `data-mantiene-nav` y quedan fuera.
        onClick={(e) => {
          const el = (e.target as HTMLElement | null)?.closest?.(
            'a[href],button,[role="treeitem"],[role="menuitem"]',
          );
          if (el && !el.closest('[data-mantiene-nav]')) setOpen(false);
        }}
      >
        {sidebar}
      </aside>
      <div class="sytue-sheet-column flex min-h-0 min-w-0 flex-1 flex-col">
        {/* Barra móvil: única vía de abrir la nav en <768px.

            `px-4`, EL MISMO CANALÓN QUE LA TOPBAR, para que el botón de menú
            caiga en la línea vertical de la marca de arriba. Esta fila es byte a
            byte la de `AppShell` salvo por esto: allí se corrigió y aquí se
            quedó en `px-2` —7px reales, con la raíz de la suite en 14px—, así
            que el cromo de Sytue Admin iba escalonado mientras el del resto ya
            no. Dos copias de la misma fila divergiendo en un número es
            exactamente lo que pasa cuando se copian. */}
        <div class="sytue-mobilebar border-border bg-surface flex h-11 shrink-0 items-center gap-1.5 border-b px-4 md:hidden">
          <Button
            id={toggleId}
            variant="ghost"
            size="sm"
            iconOnly
            aria-label={open ? t('ui.closeNav') : t('ui.openNav')}
            aria-expanded={open}
            aria-controls={drawerId}
            onClick={() => setOpen((v) => !v)}
          >
            <Icon name="menu" size={18} />
          </Button>
          {title ? <span class="truncate text-sm font-semibold">{title}</span> : null}
        </div>
        {/* La hoja empieza debajo de la barra móvil, que forma parte del marco. */}
        <div class="sytue-sheet flex min-h-0 min-w-0 flex-1">
          <main
            id="contenido"
            tabIndex={-1}
            // `relative`: el `main` DESPLAZA, así que tiene que ser el bloque contenedor de lo
            // que lleva dentro. Sin él, un `sr-only` (o cualquier `absolute` sin ancestro
            // posicionado) se ancla al DOCUMENTO en la posición que ocuparía sin desplazar:
            // el documento crecía hasta ahí —+315 px en Sytue Admin y +674 en Mi cuenta a
            // 1280×800— y se desplazaba entero por debajo del armazón.
            // La hoja no recorta: sus esquinas las redondea este panel, también en móvil.
            class="sytue-sheet-corners bg-bg relative min-h-0 min-w-0 flex-1 overflow-y-auto focus-visible:outline-none"
          >
            {children}
          </main>
        </div>
      </div>
    </div>
  );
}
