/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/** Catálogo CC0 paginado: miniaturas quietas; solo se anima el GIF pedido expresamente. */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Input, Spinner, withBase } from '@sytue/ui';
import {
  importFreeGif,
  searchFreeGifs,
  type CustomEmoji,
  type FreeGif,
  type FreeGifPage,
} from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

interface Props {
  onPick: (emoji: CustomEmoji) => void | Promise<void>;
  t: T;
}

export function FreeGifLibrary({ onPick, t }: Props): JSX.Element {
  const [query, setQuery] = useState('');
  const [cursors, setCursors] = useState(['']);
  const [page, setPage] = useState<FreeGifPage | null>(null);
  const [failed, setFailed] = useState(false);
  const [retry, setRetry] = useState(0);
  const [playing, setPlaying] = useState<number | null>(null);
  const [busy, setBusy] = useState<number | null>(null);
  const [importFailed, setImportFailed] = useState(false);
  const importingRef = useRef<AbortController | null>(null);
  const cursor = cursors[cursors.length - 1] ?? '';

  useEffect(() => () => importingRef.current?.abort(), []);

  useEffect(() => {
    const controller = new AbortController();
    setPage(null);
    setFailed(false);
    setPlaying(null);
    const timer = window.setTimeout(() => {
      void searchFreeGifs(query, cursor, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) setPage(result);
        })
        .catch(() => {
          if (!controller.signal.aborted) setFailed(true);
        });
    }, 300);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [query, cursor, retry]);

  async function choose(gif: FreeGif): Promise<void> {
    importingRef.current?.abort();
    const controller = new AbortController();
    importingRef.current = controller;
    setBusy(gif.id);
    setImportFailed(false);
    setPlaying(null);
    try {
      const emoji = await importFreeGif(gif, controller.signal);
      if (!controller.signal.aborted) await onPick(emoji);
    } catch {
      if (!controller.signal.aborted) setImportFailed(true);
    } finally {
      if (!controller.signal.aborted) setBusy(null);
      if (importingRef.current === controller) importingRef.current = null;
    }
  }

  return (
    <div class="flex flex-col gap-3">
      <Input
        type="search"
        value={query}
        placeholder={t('gif.search')}
        aria-label={t('gif.search')}
        onInput={(event) => {
          setCursors(['']);
          setQuery(event.currentTarget.value);
        }}
      />
      <p class="text-text-muted text-sm">{t('gif.freeHelp')}</p>
      <div
        class="border-border h-64 overflow-y-auto rounded-md border p-2"
        aria-busy={!page && !failed}
      >
        {failed ? (
          <div class="grid h-full place-content-center gap-3 text-center">
            <p role="alert">{t('gif.loadFailed')}</p>
            <Button variant="secondary" onClick={() => setRetry((value) => value + 1)}>
              {t('action.retry')}
            </Button>
          </div>
        ) : !page ? (
          <div class="grid h-full place-content-center">
            <Spinner size={20} />
            <span class="sr-only" role="status">
              {t('state.loading')}
            </span>
          </div>
        ) : page.items.length === 0 ? (
          <p class="text-text-muted p-4 text-center">{t('search.noResults')}</p>
        ) : (
          <ul class="grid grid-cols-3 gap-2">
            {page.items.map((gif) => (
              <li key={gif.id} class="border-border overflow-hidden rounded-md border">
                <button
                  type="button"
                  class="sytue-focus-ring-inner block w-full"
                  aria-label={`${t('gif.preview')}: ${gif.title}`}
                  aria-pressed={playing === gif.id}
                  onClick={() => setPlaying(playing === gif.id ? null : gif.id)}
                >
                  <img
                    src={withBase(
                      import.meta.env.BASE_URL,
                      `/api/gifs?id=${gif.id}${playing === gif.id ? '' : '&preview=1'}`,
                    )}
                    alt={gif.title}
                    width={gif.width}
                    height={gif.height}
                    loading="lazy"
                    decoding="async"
                    class="h-24 w-full bg-[var(--sytue-surface-sunken)] object-contain"
                  />
                </button>
                <div class="flex flex-col gap-1 p-2">
                  <span class="text-text block truncate text-xs" title={gif.title}>
                    {gif.title}
                  </span>
                  <a
                    href={gif.source}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="text-primary text-xs underline"
                  >
                    {gif.license} · Commons
                  </a>
                  <Button
                    size="sm"
                    loading={busy === gif.id}
                    disabled={busy !== null}
                    onClick={() => void choose(gif)}
                  >
                    {t('action.add')}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div class="flex justify-between gap-2">
        <Button
          variant="ghost"
          size="sm"
          disabled={cursors.length === 1 || !page}
          onClick={() => setCursors((values) => values.slice(0, -1))}
        >
          {t('action.back')}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          disabled={!page?.next || busy !== null}
          onClick={() => {
            const next = page?.next;
            if (next) setCursors((values) => [...values, next]);
          }}
        >
          {t('action.loadMore')}
        </Button>
      </div>
      {importFailed ? (
        <p class="text-danger-text text-sm" role="alert">
          {t('gif.uploadFailed')}
        </p>
      ) : null}
    </div>
  );
}
