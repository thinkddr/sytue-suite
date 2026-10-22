/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Emitir (/cast/studio): la clave de OBS con sus instrucciones, programar un
 * directo, emitir ahora desde el navegador, y la lista de mis directos
 * (programados, en curso y terminados).
 *
 * Un directo programado para «este navegador» se emite desde aquí mismo: al
 * programarlo queda elegido en el panel de emitir, que usa su token de emisor.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  FieldGroup,
  Input,
  Radio,
  SectionHeader,
  Select,
  Skeleton,
  Textarea,
  useConfirm,
  useSondeo,
  useToast,
} from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT, type T } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { Fallo, useEnLinea } from '../components/Estados.js';
import { StudioPanel } from '../components/StudioPanel.js';
import { EmitirAqui } from '../components/EmitirAqui.js';
import { VisibilityPicker } from '../components/VisibilityPicker.js';
import { biblioteca, programar, publicacion, terminar } from '../client/castApi.js';
import { tipoDeFallo, type TipoDeFallo } from '../lib/fallo.js';
import { rutaPublica } from '../lib/rutas.js';
import { VISIBILIDADES, type Video, type Visibilidad } from '../lib/tipos.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

export function StudioApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [directos, setDirectos] = useState<Video[] | null>(null);
  const [fallo, setFallo] = useState<TipoDeFallo | null>(null);
  // El directo que se emitirá desde el navegador si ya está programado.
  const [elegido, setElegido] = useState<Video | null>(null);
  const [tituloAhora, setTituloAhora] = useState('');
  const [visAhora, setVisAhora] = useState<Visibilidad>('org');
  const idAhora = useId();
  // «Presentar con Cast» (Sytue Diapos) llega con `?live=<id>`: el directo ya programado
  // queda elegido para emitir, una sola vez (no vuelve a elegirse tras terminarlo).
  const deDiapos = useRef(
    typeof location === 'undefined' ? null : new URLSearchParams(location.search).get('live'),
  );

  const cargar = (): void => {
    biblioteca('mine')
      .then((p) => {
        const lista = p.videos.filter((v) => v.kind === 'live');
        setDirectos(lista);
        setFallo(null);
        const pedido = deDiapos.current;
        const programado = pedido
          ? lista.find((v) => v.id === pedido && (v.status === 'scheduled' || v.status === 'live'))
          : undefined;
        if (programado && programado.live?.source === 'webrtc') {
          deDiapos.current = null;
          setElegido(programado);
        }
      })
      .catch((e) => setFallo(tipoDeFallo(e, navigator.onLine)));
  };
  useEffect(cargar, []);
  useSondeo(cargar, 15_000, enLinea);

  const empezar = async () => {
    if (elegido) {
      const c = await publicacion(elegido.id);
      return { videoId: elegido.id, conexion: c };
    }
    const r = await programar({
      title: tituloAhora.trim(),
      visibility: visAhora,
      source: 'webrtc',
    });
    cargar();
    return r.publish ? { videoId: r.video.id, conexion: r.publish } : null;
  };

  const acabar = async (id: string): Promise<void> => {
    await terminar(id);
    setElegido(null);
    cargar();
  };

  const terminarDeLista = async (v: Video): Promise<void> => {
    const ok = await confirmar({
      titulo: t('studio.browser.stopTitle'),
      mensaje: t('studio.browser.stopDescription'),
      confirmLabel: t('studio.lives.end'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await terminar(v.id);
      cargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    }
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="studio">
      <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-6 px-4 py-6 sm:px-6">
        <SectionHeader level={1} title={t('studio.title')} />
        {fallo === 'sinPermiso' ? (
          <Fallo t={t} tipo="sinPermiso" />
        ) : (
          <div class="grid gap-6 lg:grid-cols-2">
            <div class="flex min-w-0 flex-col gap-6">
              <Card class="flex flex-col gap-4 p-5">
                <h2 class="text-text text-base font-semibold">{t('studio.browser.title')}</h2>
                {elegido ? (
                  <p class="text-text-muted text-sm">
                    <span class="text-text font-semibold">{elegido.title}</span>
                  </p>
                ) : (
                  <div class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_11rem]">
                    <FieldGroup
                      label={t('studio.schedule.titleField')}
                      htmlFor={`${idAhora}-t`}
                      required
                    >
                      <Input
                        id={`${idAhora}-t`}
                        value={tituloAhora}
                        maxLength={200}
                        onInput={(e) => setTituloAhora(e.currentTarget.value)}
                      />
                    </FieldGroup>
                    <FieldGroup label={t('visibility.label')} htmlFor={`${idAhora}-v`}>
                      <Select
                        id={`${idAhora}-v`}
                        value={visAhora}
                        onChange={(e) => setVisAhora(e.currentTarget.value as Visibilidad)}
                      >
                        {VISIBILIDADES.map((v) => (
                          <option key={v} value={v}>
                            {t(`visibility.${v}`)}
                          </option>
                        ))}
                      </Select>
                    </FieldGroup>
                  </div>
                )}
                <EmitirAqui
                  t={t}
                  listo={Boolean(elegido) || tituloAhora.trim().length > 0}
                  empezar={empezar}
                  terminar={acabar}
                />
              </Card>
              <StudioPanel t={t} />
            </div>
            <div class="flex min-w-0 flex-col gap-6">
              <Programar
                t={t}
                alProgramar={(v, desdeAqui) => {
                  cargar();
                  if (desdeAqui) setElegido(v);
                  toast.notify({ tone: 'success', title: t('studio.schedule.done') });
                }}
              />
              <MisDirectos
                t={t}
                locale={locale}
                directos={directos}
                offline={!enLinea}
                elegir={setElegido}
                terminar={(v) => void terminarDeLista(v)}
              />
            </div>
          </div>
        )}
      </div>
      {dialogo}
    </Marco>
  );
}

function Programar({
  t,
  alProgramar,
}: {
  t: T;
  alProgramar: (v: Video, desdeAqui: boolean) => void;
}): JSX.Element {
  const id = useId();
  const toast = useToast();
  const [titulo, setTitulo] = useState('');
  const [descripcion, setDescripcion] = useState('');
  const [vis, setVis] = useState<Visibilidad>('org');
  const [fuente, setFuente] = useState<'rtmp' | 'webrtc'>('rtmp');
  const [enviando, setEnviando] = useState(false);

  const enviar = async (e: Event): Promise<void> => {
    e.preventDefault();
    if (!titulo.trim()) return;
    setEnviando(true);
    try {
      const r = await programar({
        title: titulo.trim(),
        ...(descripcion.trim() ? { description: descripcion.trim() } : {}),
        visibility: vis,
        source: fuente,
      });
      setTitulo('');
      setDescripcion('');
      alProgramar(r.video, fuente === 'webrtc');
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    } finally {
      setEnviando(false);
    }
  };

  return (
    <Card class="p-5">
      <form onSubmit={(e) => void enviar(e)} class="flex flex-col gap-4">
        <h2 class="text-text text-base font-semibold">{t('studio.schedule.title')}</h2>
        <FieldGroup label={t('studio.schedule.titleField')} htmlFor={`${id}-t`} required>
          <Input
            id={`${id}-t`}
            value={titulo}
            maxLength={200}
            onInput={(e) => setTitulo(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup label={t('studio.schedule.descriptionField')} htmlFor={`${id}-d`}>
          <Textarea
            id={`${id}-d`}
            rows={2}
            value={descripcion}
            maxLength={5000}
            onInput={(e) => setDescripcion(e.currentTarget.value)}
          />
        </FieldGroup>
        <fieldset class="flex flex-col gap-2">
          <legend class="text-text mb-1.5 text-sm font-semibold">
            {t('studio.schedule.source')}
          </legend>
          <div class="flex flex-wrap gap-4">
            <Radio
              name={`${id}-f`}
              label={t('studio.source.rtmp')}
              checked={fuente === 'rtmp'}
              onChange={() => setFuente('rtmp')}
            />
            <Radio
              name={`${id}-f`}
              label={t('studio.source.webrtc')}
              checked={fuente === 'webrtc'}
              onChange={() => setFuente('webrtc')}
            />
          </div>
        </fieldset>
        <VisibilityPicker t={t} valor={vis} alCambiar={setVis} denso />
        <Button
          type="submit"
          variant="primary"
          class="self-end"
          loading={enviando}
          disabled={!titulo.trim()}
        >
          {t('studio.schedule.submit')}
        </Button>
      </form>
    </Card>
  );
}

function MisDirectos({
  t,
  locale,
  directos,
  offline,
  elegir,
  terminar,
}: {
  t: T;
  locale: Locale;
  directos: Video[] | null;
  offline: boolean;
  elegir: (v: Video) => void;
  terminar: (v: Video) => void;
}): JSX.Element {
  return (
    <Card class="flex flex-col gap-3 p-5">
      <h2 class="text-text text-base font-semibold">{t('studio.lives.title')}</h2>
      {directos === null ? (
        offline ? (
          <p class="text-text-muted text-sm">{t('errors.offline.description')}</p>
        ) : (
          <div class="flex flex-col gap-2">
            <Skeleton height="2.5rem" />
            <Skeleton height="2.5rem" />
          </div>
        )
      ) : directos.length === 0 ? (
        <p class="text-text-muted text-sm">{t('studio.lives.empty')}</p>
      ) : (
        <ul class="divide-border -mx-5 divide-y">
          {directos.map((v) => (
            <li key={v.id} class="flex flex-wrap items-center gap-3 px-5 py-2.5">
              <div class="min-w-0 flex-1">
                <p class="text-text truncate text-sm font-semibold">{v.title}</p>
                <p class="text-text-subtle text-xs">
                  {formatDateTime(locale, v.live?.started_at ?? v.created_at)}
                </p>
              </div>
              <Badge
                tone={
                  v.status === 'live' ? 'danger' : v.status === 'scheduled' ? 'info' : 'neutral'
                }
              >
                {t(`status.${v.status}`)}
              </Badge>
              <div class="flex gap-1">
                {v.status === 'scheduled' && v.live?.source === 'webrtc' ? (
                  <Button size="sm" variant="ghost" onClick={() => elegir(v)}>
                    {t('studio.lives.emitHere')}
                  </Button>
                ) : null}
                {v.status === 'live' || v.status === 'scheduled' ? (
                  <Button size="sm" variant="ghost" onClick={() => terminar(v)}>
                    {t('studio.lives.end')}
                  </Button>
                ) : null}
                <Button size="sm" variant="secondary" href={rutaPublica(`/watch/${v.id}`)}>
                  {t('studio.lives.open')}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
