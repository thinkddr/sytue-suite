/**
 * StatusMenu — selector del estado de presencia propio en la cabecera del sidebar
 * de Sytue Rooms: En línea / Ausente / No molestar + un estado personalizado
 * (emoji + texto). Reusa el Dropdown de @sytue/ui (foco gestionado, Esc y cierre
 * al hacer click fuera). El punto NUNCA es solo color: acompaña texto (WCAG AA).
 *
 * Al elegir un estado o guardar el texto, se aplica en vivo (heartbeat inmediato
 * en el padre); el menú se cierra con Esc / click fuera.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Dropdown, MenuItem, MenuSeparator, Button, Icon, cx } from '@sytue/ui';
import { SELECTABLE_STATES, STATE_KEY, type PresenceState } from '../client/presenceApi.js';
import { EXPIRY_OPTIONS, expiryAt, type StatusExpiry } from '../lib/status.js';
import type { T } from '../i18n/index.js';

/** Punto de color por estado (token de marca; el texto lo acompaña siempre). */
const DOT: Record<PresenceState, string> = {
  online: 'bg-presence-online',
  away: 'bg-presence-away',
  busy: 'bg-presence-dnd',
  offline: 'bg-presence-offline',
};

interface Props {
  state: PresenceState;
  status: string;
  /** Aplica presencia y/o estado personalizado. `expiresAt` (epoch ms) fija la
   *  caducidad del estado; null = no caduca; ausente = conserva la caducidad vigente
   *  (p. ej. al solo cambiar de presencia). Ver ChatApp.changeMyPresence. */
  onChange: (state: PresenceState, status: string, expiresAt?: number | null) => void;
  /** Ajuste: ¿notificar mensajes directos con la ventana sin foco? */
  dmNotify: boolean;
  onToggleDmNotify: () => void;
  /** Abre el modal de preferencias de notificación (palabras clave + No molestar). */
  onOpenNotifySettings: () => void;
  /** Contadores de todos los canales y DM a cero, de un golpe. */
  onMarkAllRead: () => void;
  /** Abre el modal de integraciones (webhooks entrantes/salientes + bots, Ola 15). */
  onOpenIntegrations: () => void;
  /** ¿Es admin de algún team? El backend exige ese rol para TODA la API de
   *  integraciones (isWorkspaceAdmin), así que a quien no lo sea el menú no se lo
   *  ofrece: abrirlo solo le daba «No se pudieron cargar las integraciones», que
   *  parece una avería y en realidad es un permiso que no tiene. */
  canManageIntegrations: boolean;
  /** Traductor ya atado al idioma. */
  t: T;
}

export function StatusMenu({
  state,
  status,
  onChange,
  dmNotify,
  onToggleDmNotify,
  onOpenNotifySettings,
  onOpenIntegrations,
  onMarkAllRead,
  canManageIntegrations,
  t,
}: Props): JSX.Element {
  const [draft, setDraft] = useState(status);
  // Opción de caducidad elegida al fijar el estado (no se persiste la opción, sí el
  // instante que produce); se reinicia a «No borrar» cada vez que se reabre el menú.
  const [expiry, setExpiry] = useState<StatusExpiry>('never');

  return (
    <Dropdown
      align="end"
      label={t('status.yours')}
      trigger={({ onClick, ...aria }) => (
        <button
          type="button"
          onClick={() => {
            setDraft(status); // reabre con el estado guardado vigente
            setExpiry('never');
            onClick();
          }}
          {...aria}
          title={
            status
              ? t('status.yoursWithCustom', { state: t(STATE_KEY[state]), status })
              : t('status.yoursIs', { state: t(STATE_KEY[state]) })
          }
          class="text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg sytue-focus-ring-chrome rounded-control inline-flex min-h-[24px] max-w-[9rem] items-center gap-1.5 px-1.5 py-1 text-xs font-medium transition-colors duration-150 focus-visible:outline-none"
        >
          {/* Con aro, solo aquí: este punto va sobre el carril (los del menú, sobre
              superficie), y «en línea» sobre un carril verde no se veía. */}
          <span
            aria-hidden="true"
            class={cx('ring-sidebar-fg h-2 w-2 shrink-0 rounded-full ring-[1.5px]', DOT[state])}
          />
          <span class="min-w-0 truncate">{status || t(STATE_KEY[state])}</span>
        </button>
      )}
    >
      {SELECTABLE_STATES.map((s) => (
        <MenuItem
          key={s}
          onClick={() => onChange(s, status)}
          aria-current={s === state ? 'true' : undefined}
          class={s === state ? 'bg-bg-subtle' : undefined}
          icon={<span aria-hidden="true" class={cx('h-2.5 w-2.5 rounded-full', DOT[s])} />}
        >
          {t(STATE_KEY[s])}
        </MenuItem>
      ))}

      <MenuSeparator />

      <div class="px-2 py-1.5">
        <label class="sytue-label mb-1 block font-medium" for="sytue-status-input">
          {t('status.custom')}
        </label>
        <input
          id="sytue-status-input"
          type="text"
          value={draft}
          maxLength={100}
          placeholder={t('status.customPlaceholder')}
          onInput={(e) => setDraft(e.currentTarget.value)}
          onKeyDown={(e) => {
            // Evita que el Dropdown capture las flechas mientras editas el texto.
            if (e.key === 'ArrowUp' || e.key === 'ArrowDown') e.stopPropagation();
            if (e.key === 'Enter') {
              e.preventDefault();
              onChange(state, draft.trim(), expiryAt(expiry));
            }
          }}
          class="border-border bg-bg text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-sm outline-none"
        />
        {/* Caducidad del estado (Mattermost/Slack): «Borrar después de». <select> nativo
            = accesible por teclado y con etiqueta asociada, sin dependencias. */}
        <label class="sytue-label mt-2 mb-1 block font-medium" for="sytue-status-expiry">
          {t('status.clearAfter')}
        </label>
        <select
          id="sytue-status-expiry"
          value={expiry}
          onChange={(e) => setExpiry(e.currentTarget.value as StatusExpiry)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowUp' || e.key === 'ArrowDown') e.stopPropagation();
          }}
          class="border-border bg-bg text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-sm outline-none"
        >
          {EXPIRY_OPTIONS.map((o) => (
            <option key={o} value={o}>
              {t(`status.expiry.${o}`)}
            </option>
          ))}
        </select>
        <div class="mt-2 flex items-center justify-between gap-2">
          {status ? (
            <button
              type="button"
              onClick={() => onChange(state, '', null)}
              class="sytue-meta hover:text-text sytue-focus-ring rounded-sm underline underline-offset-2"
            >
              {t('action.clear')}
            </button>
          ) : (
            <span />
          )}
          <Button
            size="sm"
            variant="primary"
            onClick={() => onChange(state, draft.trim(), expiryAt(expiry))}
          >
            {t('action.save')}
          </Button>
        </div>
      </div>

      <MenuSeparator />

      {/* Ajuste de avisos de DM (persistido en localStorage por el padre). La acción
          va en el texto (activar/desactivar): el nombre accesible describe el efecto,
          sin depender de aria-checked (el Dropdown navega por [role=menuitem]). */}
      {/* Todo leído de un golpe (Shift+Esc en Slack): va aquí y no en la cabecera de
          «Canales» porque alcanza también a los mensajes directos. */}
      <MenuItem onClick={onMarkAllRead} icon={<Icon name="check" size={16} />}>
        {t('status.markAllRead')}
      </MenuItem>

      <MenuItem onClick={onToggleDmNotify} icon={<Icon name="bell" size={16} />}>
        {dmNotify ? t('status.dmAlertsOff') : t('status.dmAlertsOn')}
      </MenuItem>

      {/* Preferencias de notificación: palabras clave + No molestar (Ola 13). */}
      <MenuItem onClick={onOpenNotifySettings} icon={<Icon name="settings" size={16} />}>
        {t('notify.openSettings')}
      </MenuItem>

      {/* Integraciones: webhooks entrantes/salientes + bots (Ola 15). Solo para quien
          las puede administrar; ver canManageIntegrations. */}
      {canManageIntegrations && (
        <MenuItem onClick={onOpenIntegrations} icon={<Icon name="grid" size={16} />}>
          {t('integrations.open')}
        </MenuItem>
      )}
    </Dropdown>
  );
}
