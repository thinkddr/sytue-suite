/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Mis bases»: la lista de bases a las que se tiene acceso y el alta de una nueva.
 * La búsqueda filtra en el navegador (la lista ya está entera) por nombre y
 * descripción con la regla `plegar` de la suite: «cafe» encuentra «Café».
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';
import { MatrixStorageGate } from '@sytue/shell/vault';
import {
  Badge,
  Button,
  CardIcon,
  EmptyState,
  FieldGroup,
  Icon,
  Input,
  Skeleton,
  marcarAppLista,
} from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import type { BasesBase } from '@sytue/api-clients/bases';
import { Marco } from '../components/Marco.js';
import { useT } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { filtrarBases } from '../lib/actividad.js';
import { tonoDeBase } from '../lib/aspecto.js';
import { ruta } from '../lib/rutas.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

export function BasesApp(props: Props): JSX.Element {
  return (
    <MatrixStorageGate
      actor={props.user.id}
      tenant={props.user.tenant}
      locale={props.locale}
      product="bases"
      deviceName="Sytue Bases · Web"
    >
      <BasesAppBody {...props} />
    </MatrixStorageGate>
  );
}

function BasesAppBody({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = useT(locale);
  const [bases, setBases] = useState<BasesBase[]>([]);
  const [cargando, setCargando] = useState(true);
  const [error, setError] = useState('');
  const [nombre, setNombre] = useState('');
  const [creando, setCreando] = useState(false);
  const [q, setQ] = useState('');
  const vistas = useMemo(() => filtrarBases(bases, q), [bases, q]);

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

  // `sytue:app-ready`: la lista de bases ya está pintada (o su fallo). Lo que se
  // presupuesta en vez del LCP y lo que espera la bóveda para abrirse detrás.
  useEffect(() => {
    if (!cargando) marcarAppLista();
  }, [cargando]);

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

        <form
          class="border-border-subtle flex flex-wrap items-end gap-3 border-b pb-6"
          onSubmit={(e) => void crear(e)}
        >
          <FieldGroup label={t('bases.newName')} htmlFor="nueva-base" class="min-w-0 flex-1">
            <Input
              id="nueva-base"
              value={nombre}
              maxLength={120}
              autoComplete="off"
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
          </FieldGroup>
          <Button
            type="submit"
            leadingIcon={<Icon name="plus" size={16} />}
            loading={creando}
            disabled={!nombre.trim()}
          >
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
              <Skeleton height="5rem" />
              <Skeleton height="5rem" />
            </div>
          ) : bases.length === 0 && !error ? (
            <EmptyState
              icon="database"
              title={t('bases.empty.title')}
              description={t('bases.empty.body')}
            />
          ) : (
            <div class="flex flex-col gap-3">
              {bases.length > 1 || q ? (
                <div role="search">
                  <Input
                    type="search"
                    value={q}
                    aria-label={t('bases.search')}
                    placeholder={t('bases.search')}
                    leadingIcon={<Icon name="search" size={16} />}
                    onInput={(e) => setQ(e.currentTarget.value)}
                  />
                </div>
              ) : null}
              {vistas.length === 0 && q.trim() ? (
                <p role="status" class="text-text-muted text-sm">
                  {t('bases.noMatches', { q: q.trim() })}
                </p>
              ) : null}
              <ul class="flex flex-col gap-2">
                {vistas.map((b) => (
                  <li key={b.id}>
                    <a
                      href={ruta(`/base/${b.id}`)}
                      class="border-border bg-surface hover:bg-bg-subtle sytue-focus-ring flex w-full min-w-0 items-center gap-3 rounded-lg border p-4"
                    >
                      <CardIcon tone={tonoDeBase(b.id)} class="h-10 w-10 rounded-lg">
                        <Icon name="database" size={20} />
                      </CardIcon>
                      <span class="flex min-w-0 flex-1 flex-col gap-1">
                        <span class="text-text truncate font-semibold" title={b.name}>
                          {b.name}
                        </span>
                        {b.description ? (
                          <span class="text-text-muted line-clamp-2 text-sm" title={b.description}>
                            {b.description}
                          </span>
                        ) : null}
                        <span class="text-text-muted text-sm">
                          {t('bases.updated', { date: formatDate(locale, b.updated_at) })}
                          {b.host_org ? ` · ${t('bases.external', { org: b.host_org })}` : ''}
                        </span>
                      </span>
                      <Badge tone={b.role === 'owner' ? 'primary' : 'neutral'} class="shrink-0">
                        {b.role ? t(`role.${b.role}`) : t('bases.partial')}
                      </Badge>
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </section>
      </div>
    </Marco>
  );
}
