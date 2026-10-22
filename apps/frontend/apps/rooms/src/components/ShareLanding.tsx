/**
 * ShareLanding — destino del intent «compartir a Rooms» (share-to-chat,
 * docs/CONVENTIONS.md §11). El Hub trae aquí (deep-link /share) el recurso a
 * compartir (origen + enlace ya compuesto en el texto). El usuario ajusta el
 * mensaje y elige un canal/DM de LOS SUYOS; se publica reutilizando el envío
 * existente (BFF /api/send → servicio Go Sytue Chat).
 *
 * Seguridad: el selector solo lista los canales del usuario (GET /api/channels) y
 * el envío lo autoriza el backend (no se puede publicar en un canal del que no eres
 * miembro). El token vive solo en el servidor (cookie httpOnly). El texto se pinta
 * como valor de formulario (escapado por Preact): sin inyección.
 */
import type { JSX } from 'preact';
import { MatrixRoomsGate } from './MatrixRoomsGate.js';
import { sendMessage } from '../client/chatApi.js';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { SytueShell } from '@sytue/shell';
import { Avatar, Button, EmptyState, Icon, Input, Spinner, Textarea, useToast } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { orgActual, rutaConOrg } from '@sytue/config';
import type { Channel } from '../client/chatApi.js';
import { getAccessToken } from '@sytue/auth/client';
import { roomsT, type MessageKey } from '../i18n/index.js';
import { ChannelIcon } from './glyphs.js';
import { contiene } from '@sytue/i18n';

interface Props {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma negociado en el servidor: la isla NO lo detecta, lo recibe. */
  locale: Locale;
}

/**
 * Clave de catálogo del origen del recurso compartido (contexto para el usuario). El
 * NOMBRE de la app es marca y no se traduce; la frase que lo envuelve sí.
 */
const SOURCE_KEY: Record<string, MessageKey> = {
  drive: 'share.source.drive',
  mail: 'share.source.mail',
  calendar: 'share.source.calendar',
  contacts: 'share.source.contacts',
};

/** Límite de cuerpo que acepta el servicio (ver /api/send). */
const MAX_BODY = 8000;

export function ShareLanding({ user, hubBaseUrl, locale }: Props): JSX.Element {
  const t = roomsT(locale);
  const toast = useToast();
  const [params] = useState(() => new URLSearchParams(window.location.search));
  const source = params.get('share_source') ?? '';
  const [message, setMessage] = useState(() => (params.get('text') ?? '').slice(0, MAX_BODY));
  const [channels, setChannels] = useState<Channel[] | null>(null);
  const [error, setError] = useState(false);
  const [query, setQuery] = useState('');
  const [sendingId, setSendingId] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    fetch('/api/channels', { credentials: 'include', headers: { accept: 'application/json' } })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((d: { channels?: Channel[] }) => {
        if (alive) setChannels(d.channels ?? []);
      })
      .catch(() => {
        if (alive) setError(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (channels ?? []).filter((c) => !q || contiene(c.name, q));
  }, [channels, query]);

  const canShare = message.trim().length > 0;

  async function share(target: Channel): Promise<void> {
    const body = message.trim();
    if (!body || sendingId) return;
    setSendingId(target.id);
    try {
      await sendMessage({ channelId: target.id, body, clientId: crypto.randomUUID() });
      // Publicado: abre el canal destino para ver el mensaje en su contexto.
      // CON la organización en el path (ADR 0056): la URL canónica de un canal
      // la lleva tras el prefijo, y `orgActual` la lee de la URL vista —la de
      // esta pantalla, que el guard ya dejó en forma canónica o por marca—.
      // Sin org en la sesión (alta a medias), forma sin org, que el middleware
      // deja pasar.
      window.location.assign(
        rutaConOrg(
          import.meta.env.BASE_URL,
          user.tenant ? orgActual(import.meta.env.BASE_URL) : null,
          `/channels/${encodeURIComponent(target.id)}`,
        ),
      );
    } catch {
      toast.notify({ tone: 'danger', title: t('share.failed') });
      setSendingId(null);
    }
  }

  return (
    <SytueShell
      currentApp="rooms"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      <div class="mx-auto flex h-[calc(100dvh-var(--spacing-topbar))] w-full max-w-xl flex-col gap-4 overflow-y-auto p-6">
        <div>
          <h1 class="sytue-h3 text-text">{t('share.title')}</h1>
          <p class="text-text-muted mt-1 text-base">
            {t('share.intro', {
              what: t(SOURCE_KEY[source] ?? 'share.source.generic'),
            })}
          </p>
        </div>

        <label class="flex flex-col gap-1.5">
          <span class="sytue-overline text-text-subtle">{t('share.message')}</span>
          <Textarea
            rows={3}
            value={message}
            aria-label={t('share.messageAria')}
            onInput={(e) => setMessage(e.currentTarget.value.slice(0, MAX_BODY))}
          />
        </label>

        <div class="flex min-h-0 flex-col gap-2">
          <Input
            type="search"
            value={query}
            placeholder={t('search.channelOrPerson')}
            leadingIcon={<Icon name="search" size={16} />}
            aria-label={t('forward.searchLabel')}
            onInput={(e) => setQuery(e.currentTarget.value)}
          />

          {channels === null && !error ? (
            <div class="grid place-items-center py-10" aria-busy="true">
              <Spinner size={20} />
              <span class="sr-only" role="status">
                {t('share.loadingChannels')}
              </span>
            </div>
          ) : error ? (
            <EmptyState
              title={t('browse.loadFailed')}
              description={t('error.checkConnection')}
              illustration={<Icon name="warning" size={36} class="text-danger-text" />}
              action={
                <Button size="sm" variant="secondary" onClick={() => location.reload()}>
                  {t('action.retry')}
                </Button>
              }
            />
          ) : candidates.length === 0 ? (
            <p class="text-text-muted py-8 text-center text-sm">
              {query.trim() ? t('forward.emptyQuery') : t('share.noChannels')}
            </p>
          ) : (
            <ul class="flex flex-col gap-0.5">
              {candidates.map((c) => (
                <li key={c.id}>
                  <button
                    type="button"
                    disabled={sendingId !== null || !canShare}
                    onClick={() => void share(c)}
                    class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-center gap-3 rounded-md px-2 py-2 text-left transition-colors duration-150 disabled:opacity-50"
                  >
                    {c.kind === 'direct' ? <Avatar name={c.name} size="sm" /> : <ChannelIcon />}
                    <span class="text-text min-w-0 flex-1 truncate text-base font-medium">
                      {c.name}
                    </span>
                    {sendingId === c.id ? <Spinner size={16} /> : null}
                  </button>
                </li>
              ))}
            </ul>
          )}
          {!canShare ? <p class="sytue-meta">{t('share.writeSomething')}</p> : null}
        </div>
      </div>
    </SytueShell>
  );
}

export function EncryptedShareLanding(props: Props): JSX.Element {
  return (
    <MatrixRoomsGate actor={props.user.id} locale={props.locale}>
      <ShareLanding {...props} />
    </MatrixRoomsGate>
  );
}
