/**
 * BrowseChannelsModal — «Explorar canales» (paridad Mattermost). Modal con caja de
 * búsqueda con debounce que consulta GET /api/browse (canales públicos del team a
 * los que el usuario NO pertenece) y permite unirse por fila. Al unirse, avisa al
 * padre para refrescar el sidebar y seleccionar el canal.
 *
 * ponytail: lista simple con overflow; el backend acota a 50 resultados, así que no
 * necesita VirtualList. Si algún día se pagina, virtualizar aquí.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Modal, Button, Input, Spinner, Icon, EmptyState, Illustration } from '@sytue/ui';
import { browseChannels, joinChannel, type Channel } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { ChannelIcon } from './glyphs.js';

interface Props {
  open: boolean;
  onClose: () => void;
  /** Llamado con el canal recién unido (para refrescar y seleccionar). */
  onJoined: (channel: Channel) => void;
  /** Traductor ya atado al idioma (lo baja ChatApp). */
  t: T;
}

export function BrowseChannelsModal({ open, onClose, onJoined, t }: Props): JSX.Element | null {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [tick, setTick] = useState(0);
  const [joiningId, setJoiningId] = useState<string | null>(null);

  // Limpia la caja al abrir.
  useEffect(() => {
    if (open) setQuery('');
  }, [open]);

  // Búsqueda con debounce (250 ms); se dispara al abrir, al teclear y al reintentar.
  useEffect(() => {
    if (!open) return;
    let live = true;
    setLoading(true);
    setError(false);
    const t = setTimeout(() => {
      browseChannels(query)
        .then(({ channels }) => live && setResults(channels))
        .catch(() => live && setError(true))
        .finally(() => live && setLoading(false));
    }, 250);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [open, query, tick]);

  function join(ch: Channel): void {
    setJoiningId(ch.id);
    joinChannel(ch.id)
      .then(({ channel }) => {
        setResults((prev) => prev.filter((c) => c.id !== ch.id));
        onJoined(channel);
      })
      .catch(() => undefined)
      .finally(() => setJoiningId(null));
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('browse.title')}
      size="md"
      footer={
        <Button variant="secondary" onClick={onClose}>
          {t('action.close')}
        </Button>
      }
    >
      <div class="flex flex-col gap-3">
        <Input
          type="search"
          value={query}
          placeholder={t('browse.searchPlaceholder')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setQuery(e.currentTarget.value)}
          aria-label={t('browse.searchLabel')}
        />
        {loading ? (
          <div class="grid place-items-center py-10">
            <Spinner size={22} />
          </div>
        ) : error ? (
          <div class="text-text-muted flex flex-col items-center gap-2 py-8 text-sm">
            <span>{t('browse.loadFailed')}</span>
            <Button size="sm" variant="secondary" onClick={() => setTick((n) => n + 1)}>
              {t('action.retry')}
            </Button>
          </div>
        ) : results.length === 0 ? (
          <EmptyState
            class="py-4"
            title={query.trim() ? t('browse.emptyQuery') : t('browse.empty')}
            illustration={
              <Illustration name={query.trim() ? 'emptySearch' : 'emptyChat'} size={96} />
            }
          />
        ) : (
          <ul class="flex max-h-[46dvh] flex-col gap-1 overflow-y-auto">
            {results.map((c) => (
              <li
                key={c.id}
                class="hover:bg-bg-subtle flex items-center gap-3 rounded-md px-2 py-2"
              >
                <ChannelIcon />
                <div class="min-w-0 flex-1">
                  <p class="text-text truncate text-base font-medium">{c.name}</p>
                  {/* El PROPÓSITO responde a «¿de qué va y me interesa entrar?», que es
                      la pregunta de esta lista; el tema (estado del día) solo se usa si
                      el canal aún no tiene propósito. */}
                  {c.purpose || c.topic ? (
                    <p class="text-text-muted truncate text-xs">{c.purpose || c.topic}</p>
                  ) : null}
                </div>
                <Button
                  size="sm"
                  variant="secondary"
                  loading={joiningId === c.id}
                  onClick={() => join(c)}
                >
                  {t('action.join')}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
