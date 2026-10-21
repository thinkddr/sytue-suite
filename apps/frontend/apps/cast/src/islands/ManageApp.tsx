/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Gestionar (/cast/manage/<id>): título y descripción, quién puede verlo (con las
 * personas por @usuario si es «personas concretas»), el póster (subir una imagen o
 * elegir un fotograma con el reproductor), las calidades en que se puede ver
 * (ManageQualities), comentarios, chat y descarga, y borrar.
 *
 * Los interruptores y la visibilidad se guardan al tocarlos (es lo que se espera
 * de un interruptor); título y descripción, con su botón, porque se escriben.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useRef, useState } from 'preact/hooks';
import {
  Button,
  Card,
  FieldGroup,
  Icon,
  Input,
  SectionHeader,
  Skeleton,
  Switch,
  Textarea,
  useConfirm,
  useToast,
} from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT, type T } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { Fallo, useEnLinea } from '../components/Estados.js';
import { Player } from '../components/Player.js';
import { PeoplePicker } from '../components/PeoplePicker.js';
import { VisibilityPicker } from '../components/VisibilityPicker.js';
import { ManageQualities } from '../components/ManageQualities.js';
import {
  actualizar,
  borrar,
  posterDeFotograma,
  reproducir,
  subirPoster,
  video as pedirVideo,
} from '../client/castApi.js';
import { tipoDeFallo, type TipoDeFallo } from '../lib/fallo.js';
import { ruta, rutaPublica, urlDeMedia } from '../lib/rutas.js';
import type { Reproduccion, Video, Visibilidad } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
  id: string;
}

const MAX_POSTER = 5 * 1024 * 1024;

export function ManageApp({ locale, user, hubBaseUrl, id }: Props): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const [v, setV] = useState<Video | null>(null);
  const [fallo, setFallo] = useState<TipoDeFallo | null>(null);

  const cargar = (): void => {
    setFallo(null);
    pedirVideo(id, true)
      .then((x) => (x.can.edit ? setV(x) : setFallo('sinPermiso')))
      .catch((e) => setFallo(tipoDeFallo(e, navigator.onLine)));
  };
  useEffect(cargar, [id]);

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa={null}>
      <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-6 px-4 py-6 sm:px-6">
        {fallo || (!enLinea && !v) ? (
          <Fallo t={t} tipo={!enLinea ? 'offline' : (fallo ?? 'error')} reintentar={cargar} />
        ) : !v ? (
          <div
            class="flex flex-col gap-4"
            role="status"
            aria-busy="true"
            aria-label={t('player.loading')}
          >
            <Skeleton height="2rem" width="40%" />
            <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_24rem]">
              <Skeleton class="h-96" />
              <Skeleton class="aspect-video" />
            </div>
          </div>
        ) : (
          <Formulario t={t} locale={locale} inicial={v} />
        )}
      </div>
    </Marco>
  );
}

function Formulario({ t, locale, inicial }: { t: T; locale: Locale; inicial: Video }): JSX.Element {
  const id = useId();
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [v, setV] = useState(inicial);
  const [titulo, setTitulo] = useState(inicial.title);
  const [descripcion, setDescripcion] = useState(inicial.description);
  const [guardando, setGuardando] = useState(false);
  const [play, setPlay] = useState<Reproduccion | null>(null);
  const [posterOcupado, setPosterOcupado] = useState(false);
  const videoEl = useRef<HTMLVideoElement | null>(null);
  const selector = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (v.status === 'ready')
      reproducir(v.id, true)
        .then(setPlay)
        .catch(() => undefined);
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [v.id, v.status]);

  const cambiar = async (cambios: Parameters<typeof actualizar>[1]): Promise<boolean> => {
    const antes = v;
    setV({ ...v, ...cambios });
    try {
      setV(await actualizar(v.id, cambios));
      return true;
    } catch {
      setV(antes);
      toast.notify({ tone: 'danger', title: t('manage.saveError') });
      return false;
    }
  };

  const guardar = async (e: Event): Promise<void> => {
    e.preventDefault();
    setGuardando(true);
    const ok = await cambiar({ title: titulo.trim(), description: descripcion.trim() });
    setGuardando(false);
    if (ok) toast.notify({ tone: 'success', title: t('manage.saved') });
  };

  const fotograma = async (): Promise<void> => {
    const el = videoEl.current;
    if (!el) return;
    setPosterOcupado(true);
    try {
      await posterDeFotograma(v.id, el.currentTime * 1000);
      toast.notify({ tone: 'success', title: t('manage.posterQueued') });
    } catch {
      toast.notify({ tone: 'danger', title: t('manage.saveError') });
    } finally {
      setPosterOcupado(false);
    }
  };

  const subir = async (f: File): Promise<void> => {
    if (f.size > MAX_POSTER) {
      toast.notify({ tone: 'danger', title: t('manage.posterTooBig') });
      return;
    }
    setPosterOcupado(true);
    try {
      setV(await subirPoster(v.id, f));
      toast.notify({ tone: 'success', title: t('manage.posterDone') });
    } catch {
      toast.notify({ tone: 'danger', title: t('manage.saveError') });
    } finally {
      setPosterOcupado(false);
    }
  };

  const eliminar = async (): Promise<void> => {
    const ok = await confirmar({
      titulo: t('manage.deleteTitle'),
      mensaje: t('manage.deleteDescription'),
      confirmLabel: t('manage.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await borrar(v.id);
      toast.notify({ tone: 'success', title: t('manage.deleted') });
      window.location.assign(ruta('/'));
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    }
  };

  const sucio = titulo.trim() !== v.title || descripcion.trim() !== v.description;
  const poster = urlDeMedia(v.poster_url);

  return (
    <>
      <SectionHeader
        level={1}
        title={t('manage.title')}
        subtitle={v.title}
        actions={
          <Button
            variant="secondary"
            href={rutaPublica(`/watch/${v.id}`)}
            leadingIcon={<Icon name="arrowUpRight" size={16} />}
          >
            {t('manage.view')}
          </Button>
        }
      />
      {v.status === 'failed' && v.failure ? (
        <p
          class="bg-danger-bg text-danger-text flex items-start gap-2 rounded-md px-4 py-3 text-sm"
          role="alert"
        >
          <Icon name="danger" size={16} />
          {t('manage.failure', { reason: v.failure })}
        </p>
      ) : null}
      <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_24rem]">
        <div class="flex min-w-0 flex-col gap-6">
          <Card class="p-5">
            <form onSubmit={(e) => void guardar(e)} class="flex flex-col gap-4">
              <h2 class="text-text text-base font-semibold">{t('manage.details')}</h2>
              <FieldGroup label={t('manage.titleField')} htmlFor={`${id}-t`} required>
                <Input
                  id={`${id}-t`}
                  value={titulo}
                  maxLength={200}
                  onInput={(e) => setTitulo(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup label={t('manage.descriptionField')} htmlFor={`${id}-d`}>
                <Textarea
                  id={`${id}-d`}
                  rows={5}
                  value={descripcion}
                  maxLength={5000}
                  onInput={(e) => setDescripcion(e.currentTarget.value)}
                />
              </FieldGroup>
              <Button
                type="submit"
                variant="primary"
                class="self-end"
                loading={guardando}
                disabled={!sucio || !titulo.trim()}
              >
                {translate(locale, 'ui.save')}
              </Button>
            </form>
          </Card>
          <Card class="flex flex-col gap-4 p-5">
            <VisibilityPicker
              t={t}
              valor={v.visibility}
              alCambiar={(vis: Visibilidad) => void cambiar({ visibility: vis })}
            />
            {v.visibility === 'people' ? <PeoplePicker t={t} videoId={v.id} /> : null}
          </Card>
        </div>
        <div class="flex min-w-0 flex-col gap-6">
          <Card class="flex flex-col gap-3 p-5">
            <h2 class="text-text text-base font-semibold">{t('manage.poster')}</h2>
            {play ? (
              <Player
                t={t}
                hls={urlDeMedia(play.hls_url)}
                webm={urlDeMedia(play.webm_url)}
                poster={poster}
                duracionMs={play.duration_ms ?? v.duration_ms}
                titulo={v.title}
                alVideo={(el) => {
                  videoEl.current = el;
                }}
              />
            ) : (
              <div class="bg-surface-sunken aspect-video overflow-hidden rounded-md">
                {poster ? <img src={poster} alt="" class="h-full w-full object-cover" /> : null}
              </div>
            )}
            <p class="text-text-muted text-xs">{t('manage.posterHint')}</p>
            <div class="flex flex-wrap gap-2">
              {play ? (
                <Button
                  variant="secondary"
                  loading={posterOcupado}
                  onClick={() => void fotograma()}
                >
                  {t('manage.posterFrame')}
                </Button>
              ) : null}
              <Button
                variant="ghost"
                disabled={posterOcupado}
                leadingIcon={<Icon name="image" size={16} />}
                onClick={() => selector.current?.click()}
              >
                {t('manage.posterUpload')}
              </Button>
              <input
                ref={selector}
                type="file"
                accept="image/jpeg,image/png,image/webp"
                class="hidden"
                aria-hidden="true"
                tabIndex={-1}
                onChange={(e) => {
                  const f = e.currentTarget.files?.[0];
                  if (f) void subir(f);
                  e.currentTarget.value = '';
                }}
              />
            </div>
          </Card>
          {v.status !== 'scheduled' && v.status !== 'live' ? (
            <ManageQualities t={t} locale={locale} videoId={v.id} />
          ) : null}
          <Card class="flex flex-col gap-3 p-5">
            <h2 class="text-text text-base font-semibold">{t('manage.options')}</h2>
            <Switch
              label={t('manage.comments')}
              checked={v.comments_enabled}
              onChange={(e) => void cambiar({ comments_enabled: e.currentTarget.checked })}
            />
            {v.kind === 'live' ? (
              <Switch
                label={t('manage.chat')}
                checked={v.chat_enabled}
                onChange={(e) => void cambiar({ chat_enabled: e.currentTarget.checked })}
              />
            ) : null}
            <Switch
              label={t('manage.download')}
              checked={v.download_enabled}
              onChange={(e) => void cambiar({ download_enabled: e.currentTarget.checked })}
            />
          </Card>
          <Button
            variant="danger"
            class="self-start"
            leadingIcon={<Icon name="trash" size={16} />}
            onClick={() => void eliminar()}
          >
            {t('manage.delete')}
          </Button>
        </div>
      </div>
      {dialogo}
    </>
  );
}
