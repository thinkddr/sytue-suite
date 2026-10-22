/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Denuncias y bloqueos (/cast/moderation).
 *
 * Para cualquiera: las personas que tiene bloqueadas, con «Desbloquear». Para quien
 * administra la organización, además: la cola de denuncias sobre el contenido de la
 * organización (abiertas y resueltas) con sus acciones, y las personas bloqueadas en
 * toda ella. La cola no dice quién denunció: el servicio no lo manda.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Badge, Button, Spinner, Tabs, useToast } from '@sytue/ui';
import { formatTimeAgo, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT, type T } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { Fallo } from '../components/Estados.js';
import {
  bloqueados,
  bloqueadosEnOrg,
  denuncias,
  desbloquear,
  desbloquearEnOrg,
  resolverDenuncia,
} from '../client/castApi.js';
import { rutaPublica } from '../lib/rutas.js';
import type { AccionDeDenuncia, Bloqueado, Denuncia } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

export function ModerationApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = useT(locale);
  const administra = user.roles.some((r) => r === 'owner' || r === 'admin');
  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="moderation">
      <div class="mx-auto flex w-full max-w-[960px] flex-col gap-8 px-4 py-6 sm:px-6">
        <h1 class="sytue-h2 text-text">{t('nav.moderation')}</h1>
        {administra ? (
          <Tabs
            label={t('nav.moderation')}
            items={[
              {
                id: 'open',
                label: t('moderation.open'),
                content: <Cola t={t} locale={locale} resueltas={false} />,
              },
              {
                id: 'resolved',
                label: t('moderation.resolved'),
                content: <Cola t={t} locale={locale} resueltas />,
              },
              {
                id: 'org',
                label: t('moderation.orgBlocks'),
                content: (
                  <Bloqueados
                    t={t}
                    locale={locale}
                    vacio={t('moderation.orgBlocks.empty')}
                    leer={bloqueadosEnOrg}
                    quitar={desbloquearEnOrg}
                  />
                ),
              },
            ]}
          />
        ) : null}
        <section aria-labelledby="mis-bloqueos" class="flex flex-col gap-3">
          <h2 id="mis-bloqueos" class="text-text text-base font-semibold">
            {t('moderation.myBlocks')}
          </h2>
          <Bloqueados
            t={t}
            locale={locale}
            vacio={t('moderation.myBlocks.empty')}
            leer={bloqueados}
            quitar={desbloquear}
          />
        </section>
      </div>
    </Marco>
  );
}

/** Lo que tarda en llegar, lo que falla y lo que llega: los tres estados de una lista. */
function useLista<F>(leer: () => Promise<F[]>): {
  filas: F[] | null;
  fallo: boolean;
  cargar: () => void;
  setFilas: (f: (prev: F[]) => F[]) => void;
} {
  const [filas, setFilas] = useState<F[] | null>(null);
  const [fallo, setFallo] = useState(false);
  const cargar = (): void => {
    setFallo(false);
    leer()
      .then(setFilas)
      .catch(() => setFallo(true));
  };
  // Una vez al montar: `leer` es estable para quien llama.
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useEffect(cargar, []);
  return { filas, fallo, cargar, setFilas: (f) => setFilas((prev) => f(prev ?? [])) };
}

function Bloqueados({
  t,
  locale,
  vacio,
  leer,
  quitar,
}: {
  t: T;
  locale: Locale;
  vacio: string;
  leer: () => Promise<Bloqueado[]>;
  quitar: (sub: string) => Promise<void>;
}): JSX.Element {
  const { filas, fallo, cargar, setFilas } = useLista(leer);
  const toast = useToast();
  const desbloquearA = async (b: Bloqueado): Promise<void> => {
    try {
      await quitar(b.sub);
      setFilas((prev) => prev.filter((x) => x.sub !== b.sub));
      toast.notify({ tone: 'success', title: t('moderation.unblocked', { name: b.name }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    }
  };
  if (fallo) return <Fallo t={t} tipo="error" reintentar={cargar} />;
  if (!filas) return <Spinner label={t('player.loading')} />;
  if (filas.length === 0) return <p class="text-text-muted text-sm">{vacio}</p>;
  return (
    <ul class="divide-border border-border divide-y rounded-md border">
      {filas.map((b) => (
        <li key={b.sub} class="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <div class="min-w-0">
            <p class="text-text truncate text-sm font-semibold">{b.name || b.sub}</p>
            <time class="text-text-subtle text-xs" dateTime={b.created_at}>
              {formatTimeAgo(locale, b.created_at)}
            </time>
          </div>
          <Button size="sm" variant="secondary" onClick={() => void desbloquearA(b)}>
            {t('moderation.unblock', { name: b.name || b.sub })}
          </Button>
        </li>
      ))}
    </ul>
  );
}

function Cola({ t, locale, resueltas }: { t: T; locale: Locale; resueltas: boolean }): JSX.Element {
  const { filas, fallo, cargar, setFilas } = useLista(() =>
    denuncias(resueltas).then((r) => r.reports),
  );
  const [enCurso, setEnCurso] = useState<string | null>(null);
  const toast = useToast();
  const aplicar = async (d: Denuncia, accion: AccionDeDenuncia): Promise<void> => {
    setEnCurso(d.id);
    try {
      await resolverDenuncia(d.id, accion);
      // Actuar cierra todas las denuncias sobre lo mismo; descartar, solo esta.
      setFilas((prev) =>
        prev.filter((x) =>
          accion === 'dismiss'
            ? x.id !== d.id
            : !(x.kind === d.kind && x.target_id === d.target_id),
        ),
      );
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    } finally {
      setEnCurso(null);
    }
  };
  if (fallo) return <Fallo t={t} tipo="error" reintentar={cargar} />;
  if (!filas) return <Spinner label={t('player.loading')} />;
  if (filas.length === 0) return <p class="text-text-muted text-sm">{t('moderation.empty')}</p>;
  return (
    <ul class="flex flex-col gap-3">
      {filas.map((d) => (
        <li key={d.id}>
          <article class="border-border flex flex-col gap-2 rounded-md border px-4 py-3">
            <p class="flex flex-wrap items-center gap-2 text-xs">
              <Badge>{t(`moderation.kind.${d.kind}`)}</Badge>
              <span class="text-text font-semibold">{t(`report.reason.${d.reason}`)}</span>
              <time class="text-text-subtle" dateTime={d.created_at}>
                {formatTimeAgo(locale, d.created_at)}
              </time>
              {d.anonymous ? (
                <span class="text-text-subtle">{t('moderation.anonymous')}</span>
              ) : null}
              {d.resolution ? <Badge>{t(`moderation.action.${d.resolution}`)}</Badge> : null}
            </p>
            {d.target_name ? <p class="text-text text-sm font-semibold">{d.target_name}</p> : null}
            {d.excerpt ? (
              <blockquote class="border-border text-text border-l-2 pl-3 text-sm break-words whitespace-pre-wrap">
                {d.excerpt}
              </blockquote>
            ) : null}
            {d.details ? (
              <p class="text-text-muted text-sm break-words whitespace-pre-wrap">{d.details}</p>
            ) : null}
            <div class="flex flex-wrap gap-2 pt-1">
              {d.video_id ? (
                <Button size="sm" variant="ghost" href={rutaPublica(`/watch/${d.video_id}`)}>
                  {t('manage.view')}
                </Button>
              ) : null}
              {d.actions.map((a) => (
                <Button
                  key={a}
                  size="sm"
                  variant={a === 'dismiss' ? 'ghost' : 'secondary'}
                  disabled={enCurso === d.id}
                  onClick={() => void aplicar(d, a)}
                >
                  {t(`moderation.action.${a}`)}
                </Button>
              ))}
            </div>
          </article>
        </li>
      ))}
    </ul>
  );
}
