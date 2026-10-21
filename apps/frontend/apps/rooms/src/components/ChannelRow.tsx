/**
 * ChannelRow — fila de canal/DM del sidebar de Sytue Rooms (paridad Mattermost).
 * Réplica del estilo de @sytue/ui SidebarItem (fuera de este repo-scope, no editable)
 * más las acciones de la Ola 3: estrella de favorito y menú «…» (silenciar/archivar).
 * Un canal silenciado se atenúa; su no-leído se sigue mostrando, tenue.
 *
 * Estrella: visible siempre si es favorito (para poder desmarcar), y al pasar el
 * ratón / enfocar en caso contrario. Menú «…» y estrella se revelan con
 * group-hover / group-focus-within (accesible por teclado). El contador de no
 * leídos se oculta al revelar las acciones para no solaparse en la barra estrecha.
 */
import type { JSX } from 'preact';
import { cx, Icon } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { MessageMenu, type MenuEntry } from './MessageMenu.js';
import { StarGlyph } from './glyphs.js';

export interface ChannelRowProps {
  /** Icono principal (#, avatar de DM o píldora de mención). */
  leading: JSX.Element;
  label: string;
  active: boolean;
  unread: number;
  muted: boolean;
  favorite: boolean;
  /** Hay una llamada EN CURSO en este canal. */
  inCall?: boolean;
  /** Conversación de OTRA organización donde el usuario está como persona de fuera. */
  external?: boolean;
  onSelect: () => void;
  onToggleFavorite: () => void;
  /** Acciones del menú «…» (silenciar, archivar…). */
  menuItems: MenuEntry[];
  menuLabel: string;
  /** Traductor ya atado al idioma. */
  t: T;
  /** DnD (Ola 11): la fila es arrastrable para moverla a otra categoría del sidebar. */
  draggable?: boolean;
  onDragStart?: () => void;
  onDragEnd?: () => void;
}

/**
 * Favorito y «…» de la fila. No es `SIDEBAR_ACTION_CLASS` por el hover: la fila ya
 * lleva el velo de `sidebar-hover` al pasar por encima, y el mismo velo encima no
 * se vería. Sí lleva su suelo de 24 px: `h-6` son 21 con la raíz a 14, y los dos
 * botones van pegados (`gap-0.5`), así que no cabían en la excepción de separación
 * de WCAG 2.5.8.
 */
const SIDEBAR_TRIGGER =
  'flex h-6 min-h-[24px] w-6 min-w-[24px] items-center justify-center rounded text-sidebar-fg-muted transition-colors duration-150 hover:bg-sidebar-hover hover:text-sidebar-fg focus-visible:outline-none sytue-focus-ring-chrome';

export function ChannelRow({
  leading,
  label,
  active,
  unread,
  muted,
  favorite,
  inCall = false,
  external = false,
  onSelect,
  onToggleFavorite,
  menuItems,
  menuLabel,
  t,
  draggable = false,
  onDragStart,
  onDragEnd,
}: ChannelRowProps): JSX.Element {
  return (
    // Los handlers del <li> son SOLO de arrastre (reordenar canales), que no es una
    // interacción alcanzable por teclado ni la única vía: el canal se abre con el
    // botón de dentro. oxlint cuenta drag/hover como interacción; jsx-a11y no.
    // oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <li
      draggable={draggable}
      onDragStart={(e) => {
        if (!draggable) return;
        if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
        onDragStart?.();
      }}
      onDragEnd={onDragEnd}
    >
      <div
        class={cx(
          // Radio, relleno y barra de señal: tokens de FORMA, como `SidebarItem`.
          'group rounded-nav relative flex items-center pr-1',
          // La barra de 2px del canal activo: la misma que pinta SidebarItem en el
          // resto de la suite (docs/design.md §3.2). Sin ella, Rooms era la única app
          // cuyo «estás aquí» dependía solo del fondo del ítem.
          active
            ? 'bg-sidebar-active before:bg-indicator before:absolute before:top-1.5 before:bottom-1.5 before:left-0 before:w-[min(2px,var(--sytue-nav-indicator))] before:rounded-full before:content-[""]'
            : 'hover:bg-sidebar-hover',
        )}
      >
        <button
          type="button"
          aria-current={active ? 'page' : undefined}
          title={label}
          onClick={onSelect}
          class={cx(
            'rounded-nav flex min-w-0 flex-1 items-center gap-2 px-[var(--sytue-nav-pad)] py-2 text-left text-base',
            // No leído = negrita y texto a pleno contraste (§3.7 «unread»: peso
            // semibold), como en Slack o Teams; el contador solo no se ve al recorrer
            // una lista larga. Silenciado: se queda tenue, que es lo que pidió. Tenue es
            // el tono de «ítem en reposo» del carril y un peso menos, NO opacidad: con
            // `opacity-60` sobre toda la fila el nombre se quedaba por debajo de 3:1. El
            // par lo mide contraste-paleta.test.ts («Carril: ítem en reposo», ≥4,5:1).
            // El canal activo, con el peso de «estás aquí» de la forma (negrita en «Marco»).
            unread > 0 && !muted
              ? 'font-semibold'
              : muted
                ? 'font-normal'
                : active
                  ? '[font-weight:var(--sytue-nav-active-weight)]'
                  : 'font-medium',
            'sytue-focus-ring-chrome transition-colors duration-150 focus-visible:outline-none',
            active || (unread > 0 && !muted)
              ? 'text-sidebar-active-fg'
              : muted
                ? 'text-sidebar-fg-muted'
                : 'text-sidebar-fg',
          )}
        >
          <span class="grid h-6 w-6 shrink-0 place-items-center">{leading}</span>
          <span class="min-w-0 flex-1 truncate">{label}</span>
          {/* De otra organización: el edificio, con su texto para quien no lo ve. Va
              dentro del botón por lo mismo que la llamada en curso. */}
          {external ? (
            <span class="text-sidebar-fg-muted flex shrink-0" title={t('people.externalChannel')}>
              <Icon name="building" size={14} />
              <span class="sr-only">{t('people.externalChannel')}</span>
            </span>
          ) : null}
          {/* Llamada en curso: punto vivo, no un icono más. Va DENTRO del botón
              para que se lea con el nombre del canal en un lector de pantalla
              —«general, hay una llamada»— y no como un control aparte que se
              tabula sin poder pulsarlo. */}
          {inCall ? (
            <span class="text-success flex shrink-0 items-center gap-1 text-xs font-medium">
              <span class="bg-success h-1.5 w-1.5 animate-pulse rounded-full" aria-hidden="true" />
              <span class="sr-only">{t('call.ongoing')}</span>
            </span>
          ) : null}
        </button>

        <span class="flex shrink-0 items-center gap-0.5">
          {unread > 0 ? (
            <span
              class={cx(
                'grid h-[18px] min-w-[18px] place-items-center rounded-full px-1 text-xs leading-none tabular-nums',
                'group-focus-within:hidden group-hover:hidden',
                // El rol primario con su rótulo, y no `primary-text`: aquel es un token
                // de TINTA y en oscuro se aclara, con lo que el blanco de encima daba
                // 2,11:1. Mismo arreglo que en `ui.Badge count`, del que esta pastilla
                // es copia. El del canal silenciado es tenue por su PASTILLA, no por su
                // número: con el tono atenuado encima se quedaba en 4,46:1 sobre el
                // carril cobalto (contraste-paleta.test.ts lo mide).
                // Y un peso menos, que es lo que lo deja en segundo plano sin quitarle contraste.
                // La pastilla es el VELO (el token crudo) a la mitad, no `bg-sidebar-active`:
                // esa utilidad es el fondo del ítem ACTIVO, que cada forma pinta a su manera
                // (llegó a ser una píldora blanca, y el número claro encima no se leía).
                muted
                  ? 'text-sidebar-fg bg-[var(--sytue-sidebar-active-bg)]/50 font-medium'
                  : 'bg-primary text-on-primary font-bold',
              )}
            >
              {unread > 99 ? '99+' : unread}
            </span>
          ) : null}

          <button
            type="button"
            aria-pressed={favorite}
            aria-label={favorite ? t('channel.unfavourite') : t('channel.favourite')}
            title={favorite ? t('channel.unfavourite') : t('channel.favouriteShort')}
            onClick={onToggleFavorite}
            class={cx(
              SIDEBAR_TRIGGER,
              favorite ? 'text-indicator flex' : 'hidden group-focus-within:flex group-hover:flex',
            )}
          >
            <StarGlyph size={16} filled={favorite} />
          </button>

          {/* `pointer-coarse:flex`: Tailwind 4 encierra `hover:` en `@media (hover: hover)`,
              que con el dedo es falsa SIEMPRE. Sin esto, en un móvil el «…» no aparecía
              nunca y con él se iban favorito, nivel de notificación, mover de categoría y
              archivar: la fila del canal solo se podía abrir. Con puntero grueso se queda
              fijo, que es donde Slack y Teams lo ponen en el móvil. */}
          <span class="hidden group-focus-within:flex group-hover:flex pointer-coarse:flex">
            <MessageMenu label={menuLabel} items={menuItems} triggerClass={SIDEBAR_TRIGGER} />
          </span>
        </span>
      </div>
    </li>
  );
}
