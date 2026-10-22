/**
 * GifPickerModal — GIF de la organización y biblioteca animada CC0.
 *
 * La biblioteca libre la consulta Rooms, sin terceros en el navegador. La biblioteca
 * de la propia organización usa el mismo catálogo de imágenes del tenant que sostiene los emoji
 * personalizados (migración 0013), reusado tal cual: un GIF es una imagen con nombre.
 * Por eso subir uno aquí lo deja disponible para todo el equipo.
 *
 * Al elegir se inserta el atajo `:nombre:` en el compositor, NO un adjunto: un adjunto
 * vive en el ámbito del canal (habría que copiar los bytes en cada canal donde se use),
 * mientras que el catálogo es del tenant y se sirve igual en todos. Un mensaje cuyo
 * cuerpo es SOLO ese atajo se pinta en grande (ver MessageBody).
 *
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Input, Modal, Spinner, Tabs } from '@sytue/ui';
import { FreeGifLibrary } from './FreeGifLibrary.js';
import {
  listCustomEmoji,
  createCustomEmoji,
  uploadAttachment,
  type CustomEmoji,
} from '../client/chatApi.js';
import { customEmojiList, customEmojiSrc, setCustomEmoji } from '../lib/customEmoji.js';
import type { T } from '../i18n/index.js';
import { contiene } from '@sytue/i18n';

interface Props {
  onClose: () => void;
  /** Inserta el atajo `:nombre:` del GIF elegido en el compositor. */
  onPick: (shortcode: string) => void;
  t: T;
}

/** Nombre válido de catálogo (mismo invariante que el CHECK de la migración 0013). */
function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/\.[a-z0-9]+$/, '')
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64);
}

export function GifPickerModal({ onClose, onPick, t }: Props): JSX.Element {
  const [all, setAll] = useState<CustomEmoji[] | null>(null);
  const [query, setQuery] = useState('');
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    setError(false);
    try {
      const { emoji } = await listCustomEmoji();
      setAll(emoji);
      setCustomEmoji(emoji);
    } catch {
      setError(true);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // Solo GIF: el catálogo mezcla logotipos PNG y caritas; aquí interesa lo que se mueve.
  const gifs = (all ?? []).filter(
    (e) => e.contentType === 'image/gif' && contiene(e.name, query.trim()),
  );

  async function upload(file: File): Promise<void> {
    const name = slugify(file.name);
    if (!name) {
      setUploadError(t('gif.invalidName'));
      return;
    }
    setBusy(true);
    setUploadError(null);
    try {
      const att = await uploadAttachment(file);
      await createCustomEmoji({ name, fileId: att.fileId });
      await load();
    } catch {
      setUploadError(t('gif.uploadFailed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('gif.title')}
      size="md"
      footer={
        <Button variant="ghost" onClick={onClose}>
          {t('action.cancel')}
        </Button>
      }
    >
      <Tabs
        label={t('gif.title')}
        defaultTab="free"
        items={[
          {
            id: 'free',
            label: t('gif.free'),
            content: (
              <FreeGifLibrary
                t={t}
                onPick={(emoji) => {
                  const known = customEmojiList();
                  if (!known.some((item) => item.name === emoji.name))
                    setCustomEmoji([...known, emoji]);
                  onPick(`:${emoji.name}:`);
                }}
              />
            ),
          },
          {
            id: 'organization',
            label: t('gif.organization'),
            content: (
              <div class="flex flex-col gap-3">
                <Input
                  type="search"
                  value={query}
                  aria-label={t('gif.search')}
                  placeholder={t('gif.search')}
                  onInput={(e) => setQuery(e.currentTarget.value)}
                />

                <div class="border-border h-64 overflow-y-auto rounded-md border p-2">
                  {all === null && !error ? (
                    <div class="grid h-full place-items-center" aria-busy="true">
                      <Spinner size={20} />
                      <span class="sr-only" role="status">
                        {t('state.loading')}
                      </span>
                    </div>
                  ) : error ? (
                    <div class="grid h-full place-items-center p-4 text-center">
                      <EmptyState
                        title={t('gif.loadFailed')}
                        description={t('error.checkConnection')}
                        illustration={<Icon name="warning" size={36} class="text-danger-text" />}
                        action={
                          <Button variant="secondary" size="sm" onClick={() => void load()}>
                            {t('action.retry')}
                          </Button>
                        }
                      />
                    </div>
                  ) : gifs.length === 0 ? (
                    <div class="grid h-full place-items-center p-4 text-center">
                      <p class="text-text-muted text-base">{t('gif.empty')}</p>
                    </div>
                  ) : (
                    <ul class="grid grid-cols-3 gap-2">
                      {gifs.map((g) => (
                        <li key={g.name}>
                          <button
                            type="button"
                            onClick={() => onPick(`:${g.name}:`)}
                            title={`:${g.name}:`}
                            class="border-border hover:border-border-strong sytue-focus-ring rounded-control block w-full overflow-hidden border transition-colors duration-150"
                          >
                            <img
                              src={customEmojiSrc(g.fileId)}
                              alt={g.name}
                              loading="lazy"
                              class="h-24 w-full bg-[var(--sytue-surface-sunken)] object-contain"
                            />
                            <span class="text-text-muted block truncate px-1.5 py-1 text-xs">
                              {g.name}
                            </span>
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>

                {/* Subir: el fichero entra en el catálogo del tenant con el nombre del fichero
            como atajo, así que queda para todo el equipo (no solo para este mensaje). */}
                <div class="flex flex-wrap items-center gap-2">
                  <input
                    ref={fileRef}
                    type="file"
                    accept="image/gif"
                    class="sr-only"
                    tabIndex={-1}
                    aria-hidden="true"
                    onChange={(e) => {
                      const file = e.currentTarget.files?.[0];
                      e.currentTarget.value = '';
                      if (file) void upload(file);
                    }}
                  />
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={busy}
                    disabled={busy}
                    onClick={() => fileRef.current?.click()}
                  >
                    {t('gif.upload')}
                  </Button>
                  <p class="text-text-subtle text-sm">{t('gif.help')}</p>
                </div>
                {uploadError ? (
                  <p class="text-danger-text text-sm" role="alert">
                    {uploadError}
                  </p>
                ) : null}
              </div>
            ),
          },
        ]}
      />
    </Modal>
  );
}
