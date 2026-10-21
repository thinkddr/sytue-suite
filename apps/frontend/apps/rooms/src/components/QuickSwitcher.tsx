/*
 * QuickSwitcher — «Ir a un canal» (Ctrl+G): escribe, flechas, Intro. Lo que Slack,
 * Teams y Mattermost ponen bajo Ctrl+K; aquí Ctrl+K ya es la búsqueda de toda
 * la suite, así que va en Ctrl+G, como en Teams. Solo canales a los que ya
 * perteneces; para unirte a otros está «Explorar canales».
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Input, Modal, cx } from '@sytue/ui';
import type { Channel } from '../client/chatApi.js';
import { ordenarCanales } from '../client/conmutador.js';
import type { T } from '../i18n/index.js';

const MAX = 12;
const ID_CAJA = 'rooms-switcher-q';

export function QuickSwitcher({
  open,
  channels,
  activeId,
  leading,
  onClose,
  onPick,
  t,
}: {
  open: boolean;
  channels: Channel[];
  activeId: string | null;
  /** El mismo icono que pinta la barra lateral para ese canal. */
  leading: (c: Channel) => JSX.Element;
  onClose: () => void;
  onPick: (c: Channel) => void;
  t: T;
}): JSX.Element {
  const [q, setQ] = useState('');
  const [idx, setIdx] = useState(0);
  useEffect(() => {
    if (!open) return;
    setQ('');
    setIdx(0);
    // El foco a la caja en cuanto el modal esté pintado; sin `autofocus`, que el
    // gate de accesibilidad rechaza y que además no funciona tras el primer abrir.
    const id = window.setTimeout(() => document.getElementById(ID_CAJA)?.focus(), 30);
    return () => window.clearTimeout(id);
  }, [open]);
  const filas = useMemo(() => ordenarCanales(channels, q).slice(0, MAX), [channels, q]);
  useEffect(() => setIdx(0), [q]);
  const elegir = (c: Channel) => {
    onPick(c);
    onClose();
  };
  const actual = filas[idx];
  return (
    <Modal open={open} onClose={onClose} title={t('switcher.title')} size="sm">
      <div class="flex flex-col gap-2">
        <Input
          id={ID_CAJA}
          type="search"
          value={q}
          placeholder={t('switcher.placeholder')}
          aria-label={t('switcher.placeholder')}
          role="combobox"
          aria-expanded={filas.length > 0}
          aria-controls="rooms-switcher-list"
          aria-activedescendant={actual ? `rooms-switcher-${actual.id}` : undefined}
          autocomplete="off"
          onInput={(e) => setQ(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              setIdx((i) => Math.min(i + 1, filas.length - 1));
            } else if (e.key === 'ArrowUp') {
              e.preventDefault();
              setIdx((i) => Math.max(i - 1, 0));
            } else if (e.key === 'Enter' && actual) {
              e.preventDefault();
              elegir(actual);
            }
          }}
        />
        {filas.length === 0 ? (
          <p class="text-text-muted px-1 py-2 text-sm">{t('switcher.empty')}</p>
        ) : (
          <ul
            id="rooms-switcher-list"
            role="listbox"
            aria-label={t('switcher.title')}
            class="border-border divide-border/60 max-h-[50dvh] divide-y overflow-y-auto rounded-md border"
          >
            {filas.map((c, i) => (
              <li key={c.id} id={`rooms-switcher-${c.id}`} role="option" aria-selected={i === idx}>
                <button
                  type="button"
                  tabIndex={-1}
                  onMouseEnter={() => setIdx(i)}
                  onClick={() => elegir(c)}
                  class={cx(
                    'flex w-full items-center gap-3 px-3 py-2 text-left text-sm pointer-coarse:min-h-[max(44px,2.75rem)]',
                    i === idx ? 'bg-selected text-text' : 'text-text hover:bg-bg-subtle',
                  )}
                >
                  <span class="grid min-h-[24px] min-w-[24px] shrink-0 place-items-center">
                    {leading(c)}
                  </span>
                  <span
                    class={cx('min-w-0 flex-1 truncate', (c.unread ?? 0) > 0 && 'font-semibold')}
                  >
                    {c.name}
                  </span>
                  {c.id === activeId ? (
                    <span class="sytue-meta shrink-0">{t('switcher.current')}</span>
                  ) : (c.unread ?? 0) > 0 ? (
                    <span class="bg-primary shrink-0 rounded-full px-1.5 text-xs font-bold text-[var(--sytue-on-primary)] tabular-nums">
                      {c.unread}
                    </span>
                  ) : null}
                </button>
              </li>
            ))}
          </ul>
        )}
        <p class="sytue-meta px-1">{t('switcher.hint')}</p>
      </div>
    </Modal>
  );
}
