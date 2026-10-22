/**
 * ForwardMessageModal — «Reenviar» un mensaje a otro canal (Ola 8). Modal con un
 * filtro por nombre sobre los canales/DMs del sidebar (los que ya cargó ChatApp);
 * al elegir uno, publica en el destino un mensaje que cita el original (cuerpo
 * citado + atribución al autor + enlace canónico) reusando el envío existente.
 * Mismo patrón que BrowseChannelsModal / NewDmModal.
 *
 * ponytail: reenvía SOLO texto citado + enlace al original (no reempaqueta los
 * adjuntos ni las menciones — el enlace lleva al mensaje real). Suficiente para el
 * caso de uso; portar adjuntos si se pide.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Modal, Input, Avatar, Spinner, useToast, cx, Icon } from '@sytue/ui';
import type { Channel, ChatMessage } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { ChannelIcon } from './glyphs.js';
import { contiene } from '@sytue/i18n';

interface Props {
  open: boolean;
  onClose: () => void;
  /** Canales/DMs candidatos (la lista del sidebar, ya cargada por ChatApp). */
  channels: Channel[];
  /** Mensaje que se está reenviando (para el preview y la deshabilitación). */
  message: ChatMessage | null;
  /** Publica el reenvío en el canal destino; resuelve al confirmarlo el servidor. */
  onForward: (target: Channel) => Promise<void>;
  /** Traductor ya atado al idioma. */
  t: T;
}

export function ForwardMessageModal({
  open,
  onClose,
  channels,
  message,
  onForward,
  t,
}: Props): JSX.Element | null {
  const [query, setQuery] = useState('');
  const [sendingId, setSendingId] = useState<string | null>(null);
  const toast = useToast();

  useEffect(() => {
    if (open) setQuery('');
  }, [open]);

  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase();
    return channels.filter((c) => !q || contiene(c.name, q));
  }, [channels, query]);

  function pick(target: Channel): void {
    if (sendingId) return;
    setSendingId(target.id);
    onForward(target)
      .then(() => {
        toast.notify({
          tone: 'success',
          title: t('forward.done'),
          description: target.kind === 'direct' ? target.name : `#${target.name}`,
        });
        onClose();
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('forward.failed') }))
      .finally(() => setSendingId(null));
  }

  if (!open) return null;

  const preview = (message?.body ?? '').trim();

  return (
    <Modal open={open} onClose={onClose} title={t('forward.title')} size="sm">
      <div class="flex flex-col gap-3">
        {/* Vista previa del original que se reenvía. */}
        {message ? (
          <div class="border-border bg-bg-subtle rounded-md border px-3 py-2">
            <p class="text-text-muted text-xs font-semibold">{message.author.name}</p>
            <p class="text-text mt-0.5 line-clamp-2 text-sm break-words whitespace-pre-wrap">
              {preview || t('forward.noText')}
            </p>
          </div>
        ) : null}

        <Input
          type="search"
          value={query}
          placeholder={t('search.channelOrPerson')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setQuery(e.currentTarget.value)}
          aria-label={t('forward.searchLabel')}
        />

        {candidates.length === 0 ? (
          <p class="text-text-muted py-8 text-center text-sm">
            {query.trim() ? t('forward.emptyQuery') : t('forward.empty')}
          </p>
        ) : (
          <ul class="flex max-h-[46dvh] flex-col gap-0.5 overflow-y-auto">
            {candidates.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  disabled={sendingId !== null}
                  onClick={() => pick(c)}
                  class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-center gap-3 rounded-md px-2 py-2 text-left transition-colors duration-150 disabled:opacity-60"
                >
                  {c.kind === 'direct' ? <Avatar name={c.name} size="sm" /> : <ChannelIcon />}
                  <span class={cx('text-text min-w-0 flex-1 truncate text-base font-medium')}>
                    {c.name}
                  </span>
                  {sendingId === c.id ? <Spinner size={16} /> : null}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
