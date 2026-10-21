/*
 * BirthdaysModal — los próximos cumpleaños de la libreta, del más cercano al más
 * lejano, con la edad que cumple si el vCard trae el año. Pulsar un nombre abre
 * el contacto. Lo que Google Contacts enseña en su portada y aquí faltaba.
 */
import type { JSX } from 'preact';
import { Avatar, Modal } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { Contact } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import type { Cumpleanos } from '../client/cumpleanos.js';

export function BirthdaysModal({
  t,
  locale,
  open,
  cumpleanos,
  onClose,
  onOpen,
}: {
  t: T;
  locale: Locale;
  open: boolean;
  cumpleanos: Cumpleanos[];
  onClose: () => void;
  onOpen: (contact: Contact) => void;
}): JSX.Element {
  const cuando = (c: Cumpleanos): string =>
    c.dias === 0
      ? t('birthdays.today')
      : c.dias === 1
        ? t('birthdays.tomorrow')
        : t('birthdays.inDays', { count: c.dias });
  return (
    <Modal open={open} onClose={onClose} title={t('birthdays.title')} size="md">
      {cumpleanos.length === 0 ? (
        <p class="text-text-muted text-sm">{t('birthdays.empty')}</p>
      ) : (
        <ul class="divide-border/60 border-border max-h-[60dvh] divide-y overflow-y-auto rounded-md border">
          {cumpleanos.map((c) => (
            <li key={c.contact.uid}>
              <button
                type="button"
                onClick={() => onOpen(c.contact)}
                class={`hover:bg-bg-subtle sytue-focus-ring-inner flex w-full items-center gap-3 px-3 py-2 text-left ${
                  c.dias === 0 ? 'bg-selected' : ''
                }`}
              >
                <Avatar name={c.contact.fullName} size="sm" />
                <span class="flex min-w-0 flex-1 flex-col">
                  <span class="text-text truncate text-sm font-medium">{c.contact.fullName}</span>
                  <span class="text-text-muted truncate text-xs">
                    {formatDate(locale, c.fecha, { day: 'numeric', month: 'long' })}
                    {c.edad !== undefined ? ` · ${t('birthdays.turns', { age: c.edad })}` : ''}
                  </span>
                </span>
                <span
                  class={`shrink-0 text-xs tabular-nums ${c.dias === 0 ? 'text-primary-text font-semibold' : 'text-text-muted'}`}
                >
                  {cuando(c)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
