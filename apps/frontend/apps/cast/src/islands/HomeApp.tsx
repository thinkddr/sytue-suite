/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Inicio (/cast/): mis vídeos, los directos en curso de la organización, lo último
 * de la organización y lo que me han compartido otras personas. Los botones Subir
 * y Emitir arriba; en el móvil, Subir es además el botón flotante.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Input, SectionHeader, useSondeo } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT, type T } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { CargandoRejilla, Fallo, REJILLA, useEnLinea } from '../components/Estados.js';
import { VideoCard } from '../components/VideoCard.js';
import { biblioteca, compartidos } from '../client/castApi.js';
import { tipoDeFallo, type TipoDeFallo } from '../lib/fallo.js';
import { ruta } from '../lib/rutas.js';
import type { Pagina, Video } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

interface Datos {
  mios: Pagina;
  directos: Video[];
  org: Video[];
  compartidos: Video[];
}

export function HomeApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const [datos, setDatos] = useState<Datos | null>(null);
  const [fallo, setFallo] = useState<TipoDeFallo | null>(null);
  const [q, setQ] = useState('');
  const [mas, setMas] = useState(false);

  const cargar = async (): Promise<void> => {
    setFallo(null);
    try {
      const [mios, directos, org, comp] = await Promise.all([
        biblioteca('mine', q.trim() ? { q: q.trim() } : {}),
        biblioteca('org', { status: 'live' }),
        biblioteca('org', { status: 'ready' }),
        compartidos().catch(() => ({ videos: [] })),
      ]);
      setDatos({
        mios,
        directos: directos.videos,
        // Lo de la organización que no es mío: lo mío ya está arriba.
        org: org.videos.filter((v) => v.owner.sub !== user.id).slice(0, 8),
        compartidos: comp.videos,
      });
    } catch (e) {
      setFallo(tipoDeFallo(e, navigator.onLine));
    }
  };

  // La búsqueda va al servicio (`q`), con un respiro para no preguntar por letra.
  useEffect(() => {
    const id = setTimeout(() => void cargar(), q ? 250 : 0);
    return () => clearTimeout(id);
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);

  // Lo que se procesa o está en directo cambia solo: se refresca cada 20 s.
  const hayVivo =
    datos?.mios.videos.some((v) =>
      ['uploading', 'processing', 'live', 'scheduled'].includes(v.status),
    ) || Boolean(datos?.directos.length);
  useSondeo(() => void cargar(), 20_000, Boolean(hayVivo) && enLinea);

  const masMios = async (): Promise<void> => {
    if (!datos?.mios.next_cursor) return;
    setMas(true);
    try {
      const p = await biblioteca('mine', {
        cursor: datos.mios.next_cursor,
        ...(q.trim() ? { q: q.trim() } : {}),
      });
      setDatos({
        ...datos,
        mios: {
          videos: [...datos.mios.videos, ...p.videos],
          ...(p.next_cursor ? { next_cursor: p.next_cursor } : {}),
        },
      });
    } finally {
      setMas(false);
    }
  };

  const vacio =
    datos &&
    !q &&
    datos.mios.videos.length === 0 &&
    datos.directos.length === 0 &&
    datos.org.length === 0 &&
    datos.compartidos.length === 0;

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="home" fab>
      <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-8 px-4 py-6 sm:px-6">
        <SectionHeader
          level={1}
          title={t('home.title')}
          actions={
            <div class="hidden gap-2 sm:flex">
              <Button
                variant="secondary"
                href={ruta('/studio')}
                leadingIcon={<Icon name="emitir" size={16} />}
              >
                {t('nav.studio')}
              </Button>
              <Button
                variant="primary"
                href={ruta('/upload')}
                leadingIcon={<Icon name="upload" size={16} />}
              >
                {t('nav.upload')}
              </Button>
            </div>
          }
        />

        {!enLinea && !datos ? (
          <Fallo t={t} tipo="offline" reintentar={() => void cargar()} />
        ) : fallo && !datos ? (
          <Fallo t={t} tipo={fallo} reintentar={() => void cargar()} />
        ) : !datos ? (
          <CargandoRejilla etiqueta={t('player.loading')} />
        ) : vacio ? (
          <EmptyState
            illustration={<Illustration name="spotUpload" size={160} />}
            title={t('home.empty.title')}
            description={t('home.empty.description')}
            action={
              <div class="flex flex-wrap justify-center gap-2">
                <Button
                  variant="primary"
                  href={ruta('/upload')}
                  leadingIcon={<Icon name="upload" size={16} />}
                >
                  {t('nav.upload')}
                </Button>
                <Button
                  variant="secondary"
                  href={ruta('/studio')}
                  leadingIcon={<Icon name="emitir" size={16} />}
                >
                  {t('nav.studio')}
                </Button>
              </div>
            }
          />
        ) : (
          <>
            {datos.directos.length ? (
              <Seccion t={t} titulo={t('home.liveNow')}>
                <div class={REJILLA}>
                  {datos.directos.map((v) => (
                    <VideoCard key={v.id} t={t} locale={locale} video={v} />
                  ))}
                </div>
              </Seccion>
            ) : null}

            <Seccion
              t={t}
              titulo={t('home.mine')}
              acciones={
                <div class="w-full sm:w-64">
                  <Input
                    type="search"
                    value={q}
                    aria-label={t('home.search')}
                    placeholder={t('home.search')}
                    leadingIcon={<Icon name="search" size={16} />}
                    onInput={(e) => setQ(e.currentTarget.value)}
                    class="w-full"
                  />
                </div>
              }
            >
              {datos.mios.videos.length === 0 ? (
                <p class="text-text-muted text-sm">
                  {q ? t('home.noMatch', { query: q }) : t('home.empty.description')}
                </p>
              ) : (
                <div class={REJILLA}>
                  {datos.mios.videos.map((v) => (
                    <VideoCard key={v.id} t={t} locale={locale} video={v} propia />
                  ))}
                </div>
              )}
              {datos.mios.next_cursor ? (
                <Button
                  variant="secondary"
                  class="self-center"
                  loading={mas}
                  onClick={() => void masMios()}
                >
                  {t('home.more')}
                </Button>
              ) : null}
            </Seccion>

            {datos.compartidos.length ? (
              <Seccion t={t} titulo={t('home.shared')}>
                <div class={REJILLA}>
                  {datos.compartidos.map((v) => (
                    <VideoCard key={v.id} t={t} locale={locale} video={v} />
                  ))}
                </div>
              </Seccion>
            ) : null}

            {datos.org.length ? (
              <Seccion t={t} titulo={t('home.org')}>
                <div class={REJILLA}>
                  {datos.org.map((v) => (
                    <VideoCard key={v.id} t={t} locale={locale} video={v} />
                  ))}
                </div>
              </Seccion>
            ) : null}
          </>
        )}
      </div>
    </Marco>
  );
}

function Seccion({
  titulo,
  acciones,
  children,
}: {
  t: T;
  titulo: string;
  acciones?: JSX.Element;
  children: JSX.Element | (JSX.Element | null)[] | null;
}): JSX.Element {
  return (
    <section class="flex flex-col gap-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h2 class="sytue-h4 text-text">{titulo}</h2>
        {acciones ?? null}
      </div>
      {children}
    </section>
  );
}
