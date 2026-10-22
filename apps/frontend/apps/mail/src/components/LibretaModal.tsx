/*
 * LibretaModal — la libreta de Sytue Contacts dentro del redactor: buscar por
 * nombre o correo, marcar varias personas y añadirlas al campo desde el que se
 * abrió (Para, CC o CCO). Es la puerta que faltaba junto al autocompletado: el
 * autocompletado sirve cuando se sabe a quién se escribe; esto, cuando hay que
 * MIRAR la libreta para acordarse.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Avatar, Button, EmptyState, Icon, Input, Modal, Spinner, Checkbox } from '@sytue/ui';
import { appDomains } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { cargarLibreta, type EntradaDeLibreta } from '../client/mailApi.js';
import { contiene } from '@sytue/i18n';

export interface Destinatario {
  name: string;
  email: string;
}

interface Fila extends Destinatario {
  /** El correo en minúsculas: una persona con dos correos son dos filas. */
  clave: string;
}

export function LibretaModal({
  open,
  onClose,
  onPick,
  locale,
  t,
}: {
  open: boolean;
  onClose: () => void;
  onPick: (elegidos: Destinatario[]) => void;
  locale: Locale;
  t: T;
}): JSX.Element {
  const [libreta, setLibreta] = useState<EntradaDeLibreta[] | null>(null);
  const [fallo, setFallo] = useState(false);
  const [q, setQ] = useState('');
  const [marcados, setMarcados] = useState<Map<string, Destinatario>>(new Map());

  useEffect(() => {
    if (!open) return;
    setQ('');
    setMarcados(new Map());
    // La libreta se pide UNA vez por redacción; buscar filtra en el cliente.
    if (libreta !== null || fallo) return;
    cargarLibreta()
      .then(setLibreta)
      .catch(() => setFallo(true));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const filas = useMemo<Fila[]>(() => {
    const collator = new Intl.Collator(locale, { sensitivity: 'base' });
    const texto = q.trim().toLowerCase();
    return (libreta ?? [])
      .flatMap((c) =>
        c.emails.map((email) => ({ name: c.name, email, clave: email.toLowerCase() })),
      )
      .filter((f) => !texto || contiene(f.name, texto) || contiene(f.clave, texto))
      .sort((a, b) => collator.compare(a.name || a.email, b.name || b.email));
  }, [libreta, q, locale]);

  const alternar = (f: Fila) =>
    setMarcados((prev) => {
      const m = new Map(prev);
      if (m.has(f.clave)) m.delete(f.clave);
      else m.set(f.clave, { name: f.name, email: f.email });
      return m;
    });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('compose.addressBookTitle')}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            disabled={marcados.size === 0}
            onClick={() => {
              onPick([...marcados.values()]);
              onClose();
            }}
          >
            {t('compose.addressBookAdd', { count: marcados.size })}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-3">
        <Input
          type="search"
          value={q}
          onInput={(e) => setQ(e.currentTarget.value)}
          placeholder={t('compose.addressBookSearch')}
          aria-label={t('compose.addressBookSearch')}
          leadingIcon={<Icon name="search" size={16} />}
        />
        {fallo ? (
          <p class="text-danger-text text-sm" role="alert">
            {t('compose.addressBookLoadFailed')}
          </p>
        ) : libreta === null ? (
          <div class="flex justify-center py-8" aria-busy="true">
            <Spinner size={22} />
          </div>
        ) : libreta.length === 0 ? (
          <EmptyState
            title={t('compose.addressBookTitle')}
            description={t('compose.addressBookEmpty')}
            action={
              <Button
                variant="secondary"
                size="sm"
                onClick={() => window.open(appDomains.contacts, '_blank', 'noopener,noreferrer')}
              >
                {t('compose.addressBookOpenContacts')}
              </Button>
            }
          />
        ) : filas.length === 0 ? (
          <EmptyState title={t('compose.addressBookNoMatch', { query: q })} />
        ) : (
          <ul class="divide-border/60 border-border max-h-[50dvh] divide-y overflow-y-auto rounded-md border">
            {filas.map((f) => (
              <li key={f.clave}>
                <label class="hover:bg-bg-subtle flex cursor-pointer items-center gap-3 px-3 py-2 pointer-coarse:min-h-[max(44px,2.75rem)]">
                  <Checkbox
                    class="accent-primary h-4 w-4 shrink-0"
                    checked={marcados.has(f.clave)}
                    onChange={() => alternar(f)}
                  />
                  <Avatar name={f.name || f.email} size="sm" />
                  <span class="flex min-w-0 flex-col">
                    <span class="text-text truncate text-sm font-medium">{f.name || f.email}</span>
                    {f.name ? (
                      <span class="text-text-muted truncate text-xs">{f.email}</span>
                    ) : null}
                  </span>
                </label>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
