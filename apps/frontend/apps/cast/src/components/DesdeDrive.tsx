/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DesdeDrive — «Subir a Cast» de un vídeo de Sytue Drive (/cast/upload?drive=<id>).
 *
 * Es una COPIA explícita: el vídeo pasa a vivir en Cast con su transcodificación y
 * el fichero sigue en Drive, y la pantalla lo dice antes del botón, con lo que
 * ocupa. Lo que el servicio rechazaría (no es un vídeo, pasa de 8 GiB, no cabe,
 * no se ve) se sabe al llegar, por `GET /videos:from-drive`, y no después de pulsar.
 * Antes del botón, las calidades con lo que ocupará cada una (QualityPicker): de un
 * fichero de Drive solo se sabe el tamaño, así que la estimación es aproximada.
 *
 * Seis estados: cargando (la tarjeta en esqueleto), vacío (la URL no nombra un
 * fichero), error (con el motivo y, si tiene sentido, reintentar), sin conexión,
 * sin permiso y listo (el formulario). Al subir lleva a la gestión del vídeo, que
 * lo enseña «subiendo» hasta que la copia llega.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import {
  Button,
  Card,
  EmptyState,
  FieldGroup,
  Icon,
  Input,
  ProductIcon,
  SectionHeader,
  Skeleton,
  formatBytes,
} from '@sytue/ui';
import { appUrl } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { cuota, estimar, ficheroDeDrive, subirDeDrive } from '../client/castApi.js';
import { recomendada } from '../lib/calidades.js';
import { libresDe, motivoDeDrive, reintentable, type MotivoDeDrive } from '../lib/desde-drive.js';
import { orgDeLaUrl, ruta } from '../lib/rutas.js';
import type { Estimacion, FicheroDeDrive, Visibilidad } from '../lib/tipos.js';
import { Fallo, useEnLinea } from './Estados.js';
import { VisibilityPicker } from './VisibilityPicker.js';
import { QualityPicker } from './QualityPicker.js';

export interface DesdeDriveProps {
  t: T;
  locale: Locale;
  /** El identificador de la URL; '' si la URL no nombra un fichero válido. */
  fileId: string;
}

export function DesdeDrive({ t, locale, fileId }: DesdeDriveProps): JSX.Element {
  const enLinea = useEnLinea();
  const [fichero, setFichero] = useState<FicheroDeDrive | null>(null);
  const [fallo, setFallo] = useState<MotivoDeDrive | null>(null);
  const [libres, setLibres] = useState<number | null>(null);

  const cargar = (): void => {
    if (!fileId) return;
    setFallo(null);
    setFichero(null);
    ficheroDeDrive(fileId)
      .then(setFichero)
      .catch((e: unknown) => {
        setLibres(libresDe(e));
        setFallo(motivoDeDrive(e, navigator.onLine));
      });
  };
  useEffect(cargar, [fileId]);

  const abrirDrive = (
    <Button variant="secondary" href={appUrl('drive', '', orgDeLaUrl())}>
      {t('upload.drive.openDrive')}
    </Button>
  );

  let cuerpo: JSX.Element;
  if (!fileId) {
    cuerpo = (
      <EmptyState
        illustration={<Icon name="video" size={48} />}
        title={t('upload.drive.empty.title')}
        description={t('upload.drive.empty.description')}
        action={abrirDrive}
      />
    );
  } else if (!enLinea && !fichero) {
    cuerpo = <Fallo t={t} tipo="offline" reintentar={cargar} />;
  } else if (fallo === 'offline') {
    cuerpo = <Fallo t={t} tipo="offline" reintentar={cargar} />;
  } else if (fallo === 'sinPermiso') {
    cuerpo = (
      <EmptyState
        illustration={<Icon name="lock" size={48} />}
        title={t('upload.drive.forbidden.title')}
        description={t('upload.drive.forbidden.description')}
        action={abrirDrive}
      />
    );
  } else if (fallo) {
    cuerpo = (
      <EmptyState
        illustration={<Icon name="warning" size={48} />}
        title={t('upload.drive.error.title')}
        description={textoDelMotivo(t, locale, fallo, libres)}
        action={
          reintentable(fallo) ? (
            <Button variant="secondary" onClick={cargar}>
              {t('errors.retry')}
            </Button>
          ) : (
            abrirDrive
          )
        }
      />
    );
  } else if (!fichero) {
    cuerpo = (
      <div
        class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]"
        role="status"
        aria-busy="true"
        aria-label={t('upload.drive.loading')}
      >
        <Card class="flex flex-col gap-4 p-4">
          <Skeleton height="1.25rem" width="60%" />
          <Skeleton height="2.25rem" />
          <Skeleton height="0.75rem" width="80%" />
        </Card>
        <Skeleton class="h-64" />
      </div>
    );
  } else {
    cuerpo = <Formulario t={t} locale={locale} fichero={fichero} enLinea={enLinea} />;
  }

  return (
    <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-6 px-4 py-6 sm:px-6">
      <SectionHeader
        level={1}
        title={t('upload.drive.title')}
        actions={
          <Button variant="ghost" href={ruta('/upload')}>
            {t('upload.drive.other')}
          </Button>
        }
      />
      {cuerpo}
    </div>
  );
}

function textoDelMotivo(t: T, locale: Locale, m: MotivoDeDrive, libres: number | null): string {
  switch (m) {
    case 'noVideo':
      return t('upload.error.tipo');
    case 'grande':
      return t('upload.error.grande');
    case 'cuota':
      return libres !== null
        ? t('upload.error.cuotaQuedan', { available: formatBytes(locale, libres) })
        : t('upload.error.cuota');
    case 'drive':
      return t('upload.drive.error.drive');
    default:
      return t('upload.drive.error.servicio');
  }
}

function Formulario({
  t,
  locale,
  fichero,
  enLinea,
}: {
  t: T;
  locale: Locale;
  fichero: FicheroDeDrive;
  enLinea: boolean;
}): JSX.Element {
  const id = useId();
  const [titulo, setTitulo] = useState(fichero.title);
  const [visibilidad, setVisibilidad] = useState<Visibilidad>('org');
  const [enviando, setEnviando] = useState(false);
  const [fallo, setFallo] = useState<string | null>(null);
  const [estimacion, setEstimacion] = useState<Estimacion | null>(null);
  const [errorEstimacion, setErrorEstimacion] = useState(false);
  const [calidades, setCalidades] = useState<number[]>([]);
  const [tocadas, setTocadas] = useState(false);
  const [libres, setLibres] = useState<number | null>(null);

  useEffect(() => {
    estimar({ bytes: fichero.size_bytes })
      .then((e) => {
        setEstimacion(e);
        setCalidades(recomendada(e));
      })
      .catch(() => setErrorEstimacion(true));
    cuota()
      .then((c) => setLibres(c.available_bytes))
      .catch(() => undefined);
  }, [fichero.file_id, fichero.size_bytes]);

  const enviar = async (e: Event): Promise<void> => {
    e.preventDefault();
    if (enviando || !titulo.trim()) return;
    setEnviando(true);
    setFallo(null);
    try {
      const v = await subirDeDrive({
        file_id: fichero.file_id,
        title: titulo.trim(),
        visibility: visibilidad,
        ...(tocadas ? { qualities: calidades } : {}),
      });
      window.location.assign(ruta(`/manage/${v.id}`));
    } catch (err) {
      const m = motivoDeDrive(err, navigator.onLine);
      setFallo(
        m === 'offline'
          ? t('errors.offline.title')
          : m === 'sinPermiso'
            ? t('upload.drive.forbidden.description')
            : textoDelMotivo(t, locale, m, libresDe(err)),
      );
      setEnviando(false);
    }
  };

  return (
    <form
      class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]"
      onSubmit={(e) => void enviar(e)}
      aria-busy={enviando}
    >
      <Card class="flex min-w-0 flex-col gap-5 p-4 sm:p-6">
        <div class="flex min-w-0 items-center gap-3">
          <ProductIcon appKey="drive" size={32} />
          <div class="flex min-w-0 flex-col">
            <p class="text-text truncate text-base font-semibold">{fichero.name}</p>
            <p class="text-text-muted text-xs">
              {formatBytes(locale, fichero.size_bytes)} · {fichero.content_type}
            </p>
          </div>
        </div>
        <FieldGroup label={t('upload.titleField')} htmlFor={`${id}-titulo`} required>
          <Input
            id={`${id}-titulo`}
            value={titulo}
            maxLength={200}
            required
            disabled={enviando}
            onInput={(e) => setTitulo(e.currentTarget.value)}
          />
        </FieldGroup>
        <p class="text-text-muted text-sm">{t('upload.drive.note')}</p>
        <QualityPicker
          t={t}
          locale={locale}
          estimacion={estimacion}
          error={errorEstimacion}
          elegidas={calidades}
          alCambiar={(qs) => {
            setCalidades(qs);
            setTocadas(true);
          }}
          disponibles={libres}
          avisar1080
          deshabilitado={enviando}
        />
        {fallo ? (
          <p class="bg-danger-bg text-danger-text rounded-md px-4 py-3 text-sm" role="alert">
            {fallo}
          </p>
        ) : null}
        <div class="flex flex-wrap items-center gap-3">
          <Button
            type="submit"
            variant="primary"
            loading={enviando}
            disabled={!enLinea || !titulo.trim()}
          >
            {t('upload.drive.submit')}
          </Button>
        </div>
      </Card>
      <VisibilityPicker
        t={t}
        valor={visibilidad}
        alCambiar={setVisibilidad}
        deshabilitado={enviando}
        denso
      />
    </form>
  );
}
