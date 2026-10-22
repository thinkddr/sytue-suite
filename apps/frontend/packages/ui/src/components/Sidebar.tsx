/**
 * Sidebar — navegacion de secciones de la app actual. Colapsable (260 -> 64px).
 * Es el CARRIL: del color elegido de arriba abajo, con texto claro; el ítem activo lleva un
 * velo blanco y la barra ámbar de «estás aquí». Lo que vive dentro toma los tokens
 * del carril por la regla de `bg-sidebar` en tokens/appearance.css.
 * nav + aria-current="page", navegable por teclado. docs/design.md §1.0.1 / §2 / §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Badge } from './Badge.js';
import { Icon } from './Icon.js';

export interface SidebarProps {
  /** Cabecera (p.ej. logo de la app o selector de workspace). */
  header?: ComponentChildren;
  /** Pie (p.ej. ayuda, version). */
  footer?: ComponentChildren;
  collapsed?: boolean;
  children: ComponentChildren;
  class?: string;
}

export function Sidebar({
  header,
  footer,
  collapsed = false,
  children,
  class: cls,
}: SidebarProps): JSX.Element {
  const t = useT();
  return (
    <nav
      aria-label={t('ui.mainNav')}
      data-collapsed={collapsed ? 'true' : 'false'}
      class={cx(
        // shrink-0 fija el ancho de token; overflow-hidden evita que el contenido
        // se derrame por los lados durante el colapso (nada de recortes laterales).
        // `sytue-no-print`: al imprimir, el carril de navegación no es contenido.
        // Sin filete a la derecha: el carril es de color y ya se separa del contenido.
        'sytue-no-print bg-sidebar text-sidebar-fg flex h-full shrink-0 flex-col overflow-hidden',
        'transition-[width] duration-150 ease-[var(--ease-standard)]',
        cls,
      )}
      // Aquí se fijaba el primario al escalón 600 para todo lo de dentro, de cuando el
      // menú y el botón principal eran la misma tinta. Con el relleno primario claro del
      // tema oscuro dejaba «Redactar» con rótulo oscuro sobre el 600 (4,1:1): el botón
      // de la cabecera sigue ahora al primario del tema, como fuera del menú.
      style={{
        width: collapsed ? 'var(--spacing-sidebar-collapsed)' : 'var(--spacing-sidebar)',
      }}
    >
      {/* El botón principal de la cabecera («Crear evento», «Redactar», «Nuevo contacto»)
          lleva el aro BLANCO del cromo, como `.sytue-focus-ring-chrome`. Solo el HIJO
          DIRECTO con aro de botón, y no el contenedor: la variable se hereda, y Rooms
          mete aquí su menú de presencia, que se abre dentro y es superficie clara. */}
      {header ? (
        <div class="px-4 py-3 [&>.sytue-focus-ring-inset]:[--sytue-focus-ring:var(--sytue-chrome-focus)]">
          {header}
        </div>
      ) : null}
      {/* min-h-0 permite que este panel desplace en vez de empujar la sidebar. */}
      {/* `px-4`, el mismo canalón que la topbar y que los contenedores de
          página. En el teléfono esta barra ES el cajón, pegado al borde de la
          pantalla, así que su relleno es todo el aire entre el primer elemento y
          el cristal; alineados, la primera letra de «Redactar» cae bajo la marca
          de arriba en vez de medio centímetro a su izquierda.

          Con `px-2` eran 7px reales —la raíz de la suite está en 14px— y el
          barrido lo sacó en SEIS apps a la vez, todas con la misma medida porque
          todas montan este componente: «Redactar», «Mi unidad», «Nuevo
          contacto», «Hilos», «Buscar notas» y «Plataforma». */}
      {/* `relative`: es un panel que desplaza, y lo que lleva dentro se ancla a él y no
          al documento (ver la nota del `main` de AdminShell). */}
      <div class="relative min-h-0 flex-1 overflow-y-auto px-4 py-2">{children}</div>
      {/* `empty:hidden`: el pie de Calendar y Contacts es `<ImportData>`, que no pinta nada
          si la organización no ha conectado ningún origen. `footer` es un elemento igual,
          y sin esto quedaba un filete con 15 px de nada debajo (medido el 2026-09-28). */}
      {footer ? (
        <div class="border-chrome-border border-t px-4 py-2 empty:hidden">{footer}</div>
      ) : null}
    </nav>
  );
}

export interface SidebarSectionProps {
  title?: string;
  collapsed?: boolean;
  /**
   * Acciones del encabezado (`SidebarAction`), a la derecha del rótulo:
   * «gestionar carpetas», «nuevo cuaderno», «suscribirse»… Antes cada app las
   * pintaba como un item más de la lista, y se leían como una carpeta más.
   */
  actions?: ComponentChildren;
  /** El rótulo pliega y despliega la sección (chevron, como las categorías de Rooms). */
  collapsible?: boolean;
  /** Solo con `collapsible`: cómo arranca. Ajustes y similares, plegados. */
  defaultOpen?: boolean;
  children: ComponentChildren;
}

export function SidebarSection({
  title,
  collapsed,
  actions,
  collapsible = false,
  defaultOpen = true,
  children,
}: SidebarSectionProps): JSX.Element {
  const [open, setOpen] = useState(defaultOpen);
  const visible = !collapsible || open;
  return (
    <div class="mb-4">
      {title && !collapsed ? (
        // Encabezado de grupo (rol `overline`): mayusculas, tracking y peso del
        // sistema; alineado a la izquierda con el icono de los items. Las acciones
        // van a la derecha en la misma línea, con las medidas que ya usa Calendar.
        <div class="flex items-center justify-between gap-2 pt-1 pr-1 pb-1.5 pl-[var(--sytue-nav-pad)]">
          {collapsible ? (
            <button
              type="button"
              aria-expanded={open}
              // ESTE BOTÓN SOLO PLIEGA Y DESPLIEGA, así que en móvil NO debe
              // cerrar el cajón de navegación. Sin la marca lo cerraba: el
              // delegador de AppShell cierra al pulsar cualquier `button` de
              // dentro, y este es uno. El resultado era que desplegar un grupo
              // —para ver qué hay— hacía desaparecer la barra entera, y había
              // que volver a abrirla para llegar a lo que se acababa de
              // desplegar.
              //
              // Va en el componente compartido y no en cada app: el grupo
              // plegable es de aquí, así que lo heredan las once superficies de
              // una vez. Cerrar al ELEGIR algo se conserva intacto: eso lo
              // deciden los `SidebarItem`, que no llevan esta marca.
              data-mantiene-nav
              onClick={() => setOpen((v) => !v)}
              class={cx(
                // `min-h-[24px] -my-[5px]`: el rótulo solo mide su línea, 14 px, y es un
                // botón (el «Ajustes» de Mail medía 69×14 en producción, 2026-09-21).
                // Crece a los 24 de WCAG 2.5.8 y devuelve los 10 px con el margen, así
                // que la cabecera de la sección mide lo mismo que antes.
                'sytue-overline text-sidebar-fg-muted hover:text-sidebar-fg -my-[5px] -ml-1 inline-flex min-h-[24px] min-w-0 items-center gap-1 rounded-sm pl-1 transition-colors',
                'sytue-focus-ring-chrome focus-visible:outline-none',
              )}
            >
              <Icon
                name="chevronDown"
                size={14}
                class={cx('shrink-0 transition-transform duration-150', !open && '-rotate-90')}
              />
              <span class="truncate">{title}</span>
            </button>
          ) : (
            <p class="sytue-overline text-sidebar-fg-muted min-w-0 truncate">{title}</p>
          )}
          {actions ? <div class="flex shrink-0 items-center gap-0.5">{actions}</div> : null}
        </div>
      ) : null}
      {visible ? <ul class="flex flex-col gap-0.5">{children}</ul> : null}
    </div>
  );
}

export interface SidebarItemProps {
  href?: string;
  icon?: JSX.Element;
  label: string;
  active?: boolean;
  /** Conteo (no leidos). */
  count?: number;
  collapsed?: boolean;
  /** Sección aún no implementada: visible pero no seleccionable. */
  disabled?: boolean;
  onClick?: (e: MouseEvent) => void;
  /**
   * Acción propia del item, a la derecha y alineada con la del encabezado de
   * sección (refrescar la carpeta que se está viendo, por ejemplo).
   *
   * Va HERMANA del enlace y no dentro: un botón dentro de otro botón es marcado
   * inválido, y el navegador lo desanida por su cuenta dejando el segundo fuera
   * del sitio donde se escribió.
   */
  action?: JSX.Element;
}

export function SidebarItem({
  href,
  icon,
  label,
  active,
  count,
  collapsed,
  disabled = false,
  onClick,
  action,
}: SidebarItemProps): JSX.Element {
  const content = (
    <>
      {icon ? <span class="grid h-5 w-5 shrink-0 place-items-center">{icon}</span> : null}
      {/* Hasta DOS líneas y después puntos suspensivos, no una: con `truncate` se
          cortaban rótulos fijos de la suite en cuanto el idioma alarga un poco —
          «Buzones compartidos y delegación» de Mail, en alemán, estonio, finés,
          húngaro, irlandés, letón, polaco, búlgaro y griego; «Certificado de firma
          del correo» en cinco más (medido en el frontal a los 260 px del menú,
          2026-09-22)—. `min-w-0` sigue siendo imprescindible: sin él el rótulo
          empuja el icono y el contador fuera del menú. */}
      {!collapsed ? <span class="line-clamp-2 min-w-0 flex-1 break-words">{label}</span> : null}
      {!collapsed && count ? <Badge count>{count}</Badge> : null}
    </>
  );
  const className = cx(
    // Radio, relleno y barra de señal son tokens de FORMA: píldora sin barra en «Marco»,
    // radio 9 con la barra ámbar en «Carril» (theme.css, «Tokens de forma»).
    // El PESO va en cada estado y no aquí: el del ítem activo es un token de forma, y dos
    // utilidades de `font-weight` en el mismo elemento se resolverían por el orden de la hoja.
    'rounded-nav relative flex items-center gap-2 px-[var(--sytue-nav-pad)] py-2 text-base',
    'transition-colors duration-150 focus-visible:outline-none',
    // El anillo del CROMO, no el global: sobre el carril el primario de foco no se
    // ve, y ahí el token es blanco.
    'sytue-focus-ring-chrome',
    collapsed && 'justify-center',
    // Con acción, el relleno derecho deja su hueco: si no, un rótulo largo pasa
    // por debajo del botón y el contador queda tapado.
    action && !collapsed && 'pr-9',
    disabled
      ? 'cursor-not-allowed font-medium opacity-50'
      : active
        ? // La barra ámbar de 3px del item activo (docs/design.md §3.2): el velo blanco
          // solo se distingue del carril por un paso de tono, y la barra es lo que dice
          // «estás aquí» de un vistazo.
          // En «Marco» no hay barra: «estás aquí» lo dicen el velo y la NEGRITA
          // (`--sytue-nav-active-weight`: 650 en Marco, 500 en Carril).
          'bg-sidebar-active text-sidebar-active-fg [font-weight:var(--sytue-nav-active-weight)] before:bg-indicator before:absolute before:top-2 before:bottom-2 before:left-0 before:w-[var(--sytue-nav-indicator)] before:rounded-full before:content-[""]'
        : // En reposo, atenuado: sobre el carril todo es blanco, y si los ítems fueran
          // tan blancos como el activo, a este solo lo delataría el velo.
          'text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg active:bg-sidebar-active/60 font-medium',
  );
  // Solo el rótulo. Aquí se añadía « (próximamente)» a los desactivados, en castellano
  // para los 26 idiomas; que no se puede pulsar ya lo dicen `aria-disabled` y la opacidad.
  const titleAttr = label;
  return (
    // `relative` solo cuando hay acción: posicionar por defecto cambiaría el
    // contexto de apilamiento de todos los items de la suite sin necesidad.
    <li class={cx(action && !collapsed && 'relative')}>
      {href ? (
        <a
          href={href}
          aria-current={active ? 'page' : undefined}
          aria-disabled={disabled ? 'true' : undefined}
          title={titleAttr}
          class={className}
          onClick={disabled ? (e) => e.preventDefault() : onClick}
        >
          {content}
        </a>
      ) : (
        <button
          type="button"
          aria-current={active ? 'page' : undefined}
          aria-disabled={disabled ? 'true' : undefined}
          disabled={disabled}
          title={titleAttr}
          class={cx(className, 'w-full text-left')}
          onClick={onClick}
        >
          {content}
        </button>
      )}
      {action && !collapsed ? (
        // `right-1` es el mismo que el `pr-1` del encabezado de sección, y los
        // dos botones miden 24px: así caen en la MISMA columna que la rueda de
        // gestionar carpetas, que es lo que hace que la barra se lea como una
        // rejilla y no como iconos sueltos.
        <span class="absolute top-1/2 right-1 -translate-y-1/2">{action}</span>
      ) : null}
    </li>
  );
}

/**
 * Clases del botón de acción del encabezado de sección. Exportadas para el caso
 * en que el disparador no puede ser un <button>: un <label> con un
 * <input type="file"> dentro (importar en Notepads).
 *
 * `min-h-[24px] min-w-[24px]` porque `h-6` NO son 24 px: son 1.5rem, y la raíz de
 * la suite está en 14 px. Medían 21×21 en producción (2026-09-21, medidas-web.mjs)
 * en Mail, Calendar, Notepads y Rooms, y dos pegados con `gap-0.5` —suscribirse y
 * crear calendario— no caben en la excepción de separación de WCAG 2.5.8 (AA). Es
 * el mismo suelo en px que ya lleva `IconButton size="sm"`.
 */
export const SIDEBAR_ACTION_CLASS =
  'text-sidebar-fg-muted grid h-6 min-h-[24px] w-6 min-w-[24px] place-items-center rounded-sm transition-colors hover:bg-sidebar-hover hover:text-sidebar-fg disabled:pointer-events-none disabled:opacity-50 sytue-focus-ring-chrome focus-visible:outline-none';

export interface SidebarActionProps {
  /** Nombre accesible y tooltip: el botón es solo icono. */
  label: string;
  icon: JSX.Element;
  onClick: () => void;
  disabled?: boolean;
  class?: string;
}

/** Botón de icono para `SidebarSection.actions`: 24px, sobre el rail, sin texto. */
export function SidebarAction({
  label,
  icon,
  onClick,
  disabled = false,
  class: cls,
}: SidebarActionProps): JSX.Element {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      class={cx(SIDEBAR_ACTION_CLASS, cls)}
    >
      {icon}
    </button>
  );
}
