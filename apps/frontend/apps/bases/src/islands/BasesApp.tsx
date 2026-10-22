/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Mis bases»: la lista de bases a las que se tiene acceso y el alta de una nueva.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, EmptyState, FieldGroup, Input, Skeleton } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import type { BasesBase } from '@sytue/api-clients/bases';
import { Marco } from '../components/Marco.js';
import { useT } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { ruta } from '../lib/rutas.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

export function BasesApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = useT(locale);
  const [bases, setBases] = useState<BasesBase[]>([]);
  const [cargando, setCargando] = useState(true);
  const [error, setError] = useState('');
  const [nombre, setNombre] = useState('');
  const [creando, setCreando] = useState(false);

  const cargar = useCallback(async () => {
    setCargando(true);
    setError('');
    try {
      setBases(await cliente().listBases());
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      setCargando(false);
    }
  }, [t]);

  useEffect(() => {
    void cargar();
  }, [cargar]);

  async function crear(e: Event): Promise<void> {
    e.preventDefault();
    const limpio = nombre.trim();
    if (!limpio || creando) return;
    setCreando(true);
    setError('');
    try {
      const b = await cliente().createBase(limpio);
      window.location.assign(ruta(`/base/${b.id}`));
    } catch (err) {
      setError(mensajeDeError(err, t));
      setCreando(false);
    }
  }

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="bases">
      <div class="mx-auto flex max-w-4xl flex-col gap-6 p-4 md:p-6">
        <h1 class="text-text text-2xl font-semibold">{t('bases.title')}</h1>

        <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void crear(e)}>
          <FieldGroup label={t('bases.newName')} htmlFor="nueva-base" class="min-w-0 flex-1">
            <Input
              id="nueva-base"
              value={nombre}
              maxLength={120}
              autoComplete="off"
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
          </FieldGroup>
          <Button type="submit" loading={creando} disabled={!nombre.trim()}>
            {t('bases.create')}
          </Button>
        </form>

        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}

        <section aria-labelledby="lista-bases" aria-busy={cargando}>
          <h2 id="lista-bases" class="sr-only">
            {t('bases.listLabel')}
          </h2>
          {cargando ? (
            <div class="flex flex-col gap-2">
              <Skeleton height="3.5rem" />
              <Skeleton height="3.5rem" />
            </div>
          ) : bases.length === 0 && !error ? (
            <EmptyState title={t('bases.empty.title')} description={t('bases.empty.body')} />
          ) : (
            <ul class="flex flex-col gap-2">
              {bases.map((b) => (
                <li key={b.id}>
                  <a
                    href={ruta(`/base/${b.id}`)}
                    class="border-border bg-surface hover:bg-bg-subtle sytue-focus-ring flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3"
                  >
                    <span class="flex min-w-0 flex-col">
                      <span class="text-text truncate font-semibold">{b.name}</span>
                      <span class="text-text-muted text-sm">
                        {t('bases.updated', { date: formatDate(locale, b.updated_at) })}
                        {b.host_org ? ` · ${t('bases.external', { org: b.host_org })}` : ''}
                      </span>
                    </span>
                    <Badge tone={b.role === 'owner' ? 'primary' : 'neutral'}>
                      {b.role ? t(`role.${b.role}`) : t('bases.partial')}
                    </Badge>
                  </a>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </Marco>
  );
}
