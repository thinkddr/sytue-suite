/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Subir (/cast/upload): arrastrar o elegir vídeos, la cola con el progreso de cada
 * uno, el título editable mientras sube, la visibilidad con la que nacen y el
 * medidor de la cuota del plan (compartida con Drive).
 *
 * ANTES de subir, los ficheros elegidos esperan en «Antes de subir»: en qué
 * calidades se generarán, con lo que ocupará cada una y lo que queda en el plan
 * (QualityPicker). La recomendada viene marcada; nada sube hasta «Empezar a subir».
 *
 * Por fichero: POST /videos crea el registro (queda «subiendo») y PUT
 * /videos/{id}/original manda el fichero en streaming por el proxy del BFF. Lo que
 * no es vídeo o pasa de 8 GiB se rechaza aquí sin gastar un byte; la cuota la
 * comprueba el servicio ANTES de aceptar el cuerpo (507 con lo que queda libre).
 *
 * Con `?drive=<id>` (el «Subir a Cast» del menú de Drive) la página es otra: el
 * fichero de Drive, su título y su visibilidad, y un botón (DesdeDrive).
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Card, Icon, Meter, ProgressBar, SectionHeader, cx, formatBytes } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT } from '../i18n/index.js';
import { Marco } from '../components/Marco.js';
import { Fallo, useEnLinea } from '../components/Estados.js';
import { UploadQueue } from '../components/UploadQueue.js';
import { DesdeDrive } from '../components/DesdeDrive.js';
import { VisibilityPicker } from '../components/VisibilityPicker.js';
import { QualityPicker } from '../components/QualityPicker.js';
import {
  actualizar,
  borrar,
  crear,
  estimar,
  cuota as pedirCuota,
  subirOriginal,
} from '../client/castApi.js';
import { medir } from '../client/medir.js';
import { recomendada, sumar } from '../lib/calidades.js';
import {
  cambiar,
  nuevo,
  progresoTotal,
  siguiente,
  type Elemento,
  type MotivoFallo,
} from '../lib/cola.js';
import { FalloDelServicio, tipoDeFallo, type TipoDeFallo } from '../lib/fallo.js';
import type { Cuota, Estimacion, Visibilidad } from '../lib/tipos.js';

/** Un fichero elegido que aún no ha empezado a subir: espera a que se confirmen las calidades. */
interface Preparado {
  clave: string;
  fichero: File;
  estimacion: Estimacion | null;
  error: boolean;
}

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
  /** `?drive=` de la URL: null sin él; '' si no nombra un fichero válido. */
  drive?: string | null;
}

export function UploadApp({ drive = null, ...props }: Props): JSX.Element {
  const t = useT(props.locale);
  if (drive === null) return <SubirFicheros {...props} />;
  return (
    <Marco user={props.user} hubBaseUrl={props.hubBaseUrl} locale={props.locale} activa="upload">
      <DesdeDrive t={t} locale={props.locale} fileId={drive} />
    </Marco>
  );
}

function SubirFicheros({ locale, user, hubBaseUrl }: Omit<Props, 'drive'>): JSX.Element {
  const t = useT(locale);
  const enLinea = useEnLinea();
  const [cola, setCola] = useState<Elemento[]>([]);
  const [visibilidad, setVisibilidad] = useState<Visibilidad>('org');
  const [cuota, setCuota] = useState<Cuota | null>(null);
  const [falloCuota, setFalloCuota] = useState<TipoDeFallo | null>(null);
  const [disponibles, setDisponibles] = useState<number | null>(null);
  const [encima, setEncima] = useState(false);
  const [preparados, setPreparados] = useState<Preparado[]>([]);
  const [calidades, setCalidades] = useState<number[]>([]);
  const [calidadesTocadas, setCalidadesTocadas] = useState(false);
  const ficheros = useRef(new Map<string, File>());
  const abortos = useRef(new Map<string, AbortController>());
  const selector = useRef<HTMLInputElement>(null);
  // La cola vista desde las promesas de la subida, que viven más que un render.
  const colaRef = useRef(cola);
  useEffect(() => {
    colaRef.current = cola;
  }, [cola]);

  const leerCuota = (): void => {
    pedirCuota()
      .then((c) => {
        setCuota(c);
        setFalloCuota(null);
      })
      .catch((e) => setFalloCuota(tipoDeFallo(e, navigator.onLine)));
  };
  useEffect(leerCuota, []);

  // Lo que no puede subir va directo a la cola con su motivo; lo demás espera en
  // «Antes de subir» mientras el servicio estima cuánto ocupará.
  const anadir = (lista: FileList | File[]): void => {
    const rechazados: Elemento[] = [];
    const listos: Preparado[] = [];
    for (const f of Array.from(lista)) {
      const clave = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const e = nuevo(clave, f, visibilidad);
      if (e.estado === 'fallo') rechazados.push(e);
      else listos.push({ clave, fichero: f, estimacion: null, error: false });
    }
    if (rechazados.length) setCola((c) => [...c, ...rechazados]);
    if (!listos.length) return;
    setPreparados((p) => [...p, ...listos]);
    for (const p of listos)
      void medir(p.fichero)
        .then((m) => estimar({ bytes: p.fichero.size, ...m }))
        .then((est) =>
          setPreparados((ps) =>
            ps.map((x) => (x.clave === p.clave ? { ...x, estimacion: est } : x)),
          ),
        )
        .catch(() =>
          setPreparados((ps) => ps.map((x) => (x.clave === p.clave ? { ...x, error: true } : x))),
        );
  };

  // La estimación de todo lo preparado, y la recomendada marcada mientras nadie toque nada.
  const estimacion = preparados.every((p) => p.estimacion)
    ? sumar(preparados.map((p) => p.estimacion!))
    : null;
  const errorEstimacion = preparados.some((p) => p.error);
  useEffect(() => {
    if (estimacion && !calidadesTocadas) setCalidades(recomendada(estimacion));
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [estimacion?.recommended.join(), calidadesTocadas]);

  const empezar = (): void => {
    const elegidas = calidadesTocadas ? calidades : [];
    const nuevos = preparados.map((p) => {
      ficheros.current.set(p.clave, p.fichero);
      return nuevo(p.clave, p.fichero, visibilidad, elegidas);
    });
    setCola((c) => [...c, ...nuevos]);
    descartar();
  };
  const descartar = (): void => {
    setPreparados([]);
    setCalidadesTocadas(false);
    setCalidades([]);
  };

  // El motor: en cuanto no hay ninguno subiendo, arranca el siguiente.
  useEffect(() => {
    const e = siguiente(cola);
    if (!e) return;
    const f = ficheros.current.get(e.clave);
    if (!f) return;
    setCola((c) => cambiar(c, e.clave, { estado: 'subiendo', progreso: 0 }));
    const aborto = new AbortController();
    abortos.current.set(e.clave, aborto);
    void (async () => {
      let id: string | undefined;
      try {
        const v = await crear({
          title: e.titulo,
          visibility: e.visibilidad,
          ...(e.calidades?.length ? { qualities: e.calidades } : {}),
        });
        id = v.id;
        setCola((c) => cambiar(c, e.clave, { videoId: v.id }));
        // Si el título cambió mientras se creaba el registro, se guarda ahora.
        const actual = colaRef.current.find((x) => x.clave === e.clave);
        if (actual && actual.titulo.trim() && actual.titulo !== e.titulo)
          void actualizar(v.id, { title: actual.titulo.trim() }).catch(() => undefined);
        let ultimo = 0;
        await subirOriginal(
          v.id,
          f,
          (p) => {
            // Un repintado por punto porcentual basta; el evento llega cada pocos KB.
            if (p - ultimo < 0.01 && p < 1) return;
            ultimo = p;
            setCola((c) => cambiar(c, e.clave, { progreso: p }));
          },
          aborto.signal,
        );
        setCola((c) => cambiar(c, e.clave, { estado: 'procesando', progreso: 1 }));
        leerCuota();
      } catch (err) {
        let motivo: MotivoFallo = 'servicio';
        if (err instanceof DOMException && err.name === 'AbortError') motivo = 'cancelada';
        else if (err instanceof TypeError) motivo = 'red';
        else if (err instanceof FalloDelServicio && err.status === 507) {
          motivo = 'cuota';
          const d = err.detalles['available_bytes'];
          if (typeof d === 'number') setDisponibles(d);
        } else if (err instanceof FalloDelServicio && err.status === 413) motivo = 'grande';
        setCola((c) => cambiar(c, e.clave, { estado: 'fallo', motivo }));
        // Un registro sin original no sirve para nada: se retira.
        if (id) void borrar(id).catch(() => undefined);
      } finally {
        abortos.current.delete(e.clave);
        ficheros.current.delete(e.clave);
      }
    })();
  }, [cola]);

  // Salir a mitad de una subida la corta: el navegador lo pregunta antes.
  const subiendo = cola.some((e) => e.estado === 'subiendo' || e.estado === 'esperando');
  useEffect(() => {
    if (!subiendo) return undefined;
    const aviso = (ev: BeforeUnloadEvent): void => {
      ev.preventDefault();
    };
    window.addEventListener('beforeunload', aviso);
    return () => window.removeEventListener('beforeunload', aviso);
  }, [subiendo]);

  const renombrar = (clave: string, titulo: string): void =>
    setCola((c) => cambiar(c, clave, { titulo }));
  const guardarTitulo = (clave: string): void => {
    const e = colaRef.current.find((x) => x.clave === clave);
    if (e?.videoId && e.titulo.trim())
      void actualizar(e.videoId, { title: e.titulo.trim() }).catch(() => undefined);
  };
  const cancelar = (clave: string): void => {
    const a = abortos.current.get(clave);
    if (a) a.abort();
    else setCola((c) => cambiar(c, clave, { estado: 'fallo', motivo: 'cancelada' }));
  };
  const quitar = (clave: string): void => setCola((c) => c.filter((e) => e.clave !== clave));

  const total = progresoTotal(cola);

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="upload">
      <div class="mx-auto flex w-full max-w-[1280px] flex-col gap-6 px-4 py-6 sm:px-6">
        <SectionHeader level={1} title={t('upload.title')} />
        <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <div class="flex min-w-0 flex-col gap-6">
            {!enLinea ? (
              <p class="bg-warning-bg text-warning-text rounded-md px-4 py-3 text-sm" role="status">
                {t('errors.offline.title')}. {t('errors.offline.description')}
              </p>
            ) : null}
            {/* La zona de soltar: también un botón, para quien no arrastra. */}
            <div
              onDragOver={(e) => {
                e.preventDefault();
                setEncima(true);
              }}
              onDragLeave={() => setEncima(false)}
              onDrop={(e) => {
                e.preventDefault();
                setEncima(false);
                if (e.dataTransfer?.files.length) anadir(e.dataTransfer.files);
              }}
              class={cx(
                'flex flex-col items-center justify-center gap-3 rounded-md border-2 border-dashed px-6 py-12 text-center transition-colors duration-150 motion-reduce:transition-none',
                encima ? 'border-primary bg-selected' : 'border-border-strong bg-surface',
              )}
            >
              <span class="text-text-subtle">
                <Icon name="upload" size={32} />
              </span>
              <p class="text-text text-base font-semibold pointer-coarse:hidden">
                {t('upload.drop')}
              </p>
              <Button
                variant="primary"
                disabled={!enLinea}
                onClick={() => selector.current?.click()}
              >
                {t('upload.choose')}
              </Button>
              <p class="text-text-muted text-xs">{t('upload.hint')}</p>
              <input
                ref={selector}
                type="file"
                accept="video/*,.mkv,.mov,.m4v,.webm"
                multiple
                class="hidden"
                aria-hidden="true"
                tabIndex={-1}
                onChange={(e) => {
                  if (e.currentTarget.files?.length) anadir(e.currentTarget.files);
                  e.currentTarget.value = '';
                }}
              />
            </div>

            {preparados.length ? (
              <Card class="flex flex-col gap-4 p-4 sm:p-5">
                <h2 class="text-text text-base font-semibold">{t('upload.prepare.title')}</h2>
                <ul class="flex flex-col gap-1">
                  {preparados.map((p) => (
                    <li key={p.clave} class="text-text-muted truncate text-sm">
                      <span class="text-text font-medium">{p.fichero.name}</span> ·{' '}
                      {formatBytes(locale, p.fichero.size)}
                    </li>
                  ))}
                </ul>
                <QualityPicker
                  t={t}
                  locale={locale}
                  estimacion={estimacion}
                  error={errorEstimacion}
                  elegidas={calidades}
                  alCambiar={(qs) => {
                    setCalidades(qs);
                    setCalidadesTocadas(true);
                  }}
                  disponibles={cuota?.available_bytes ?? null}
                  avisar1080
                />
                <div class="flex flex-wrap items-center gap-3">
                  <Button
                    variant="primary"
                    disabled={!enLinea || (!estimacion && !errorEstimacion)}
                    onClick={empezar}
                  >
                    {t('upload.prepare.start')}
                  </Button>
                  <Button variant="ghost" onClick={descartar}>
                    {t('upload.prepare.discard')}
                  </Button>
                </div>
              </Card>
            ) : null}

            {cola.length ? (
              <>
                {total !== undefined && subiendo ? (
                  <ProgressBar value={total} label={t('upload.queue')} />
                ) : null}
                <UploadQueue
                  t={t}
                  locale={locale}
                  cola={cola}
                  disponibles={disponibles}
                  alRenombrar={renombrar}
                  alGuardarTitulo={guardarTitulo}
                  alCancelar={cancelar}
                  alQuitar={quitar}
                />
              </>
            ) : null}
          </div>

          <aside class="flex flex-col gap-6">
            <Card class="p-4">
              {cuota ? (
                <Meter
                  value={cuota.used_bytes}
                  max={cuota.limit_bytes}
                  label={t('upload.quota')}
                  valueText={t('upload.quotaText', {
                    used: formatBytes(locale, cuota.used_bytes),
                    limit: formatBytes(locale, cuota.limit_bytes),
                    cast: formatBytes(locale, cuota.cast_bytes),
                  })}
                />
              ) : falloCuota ? (
                <p class="text-text-muted text-sm">{t('upload.quotaError')}</p>
              ) : (
                <ProgressBar label={t('upload.quota')} />
              )}
            </Card>
            <VisibilityPicker
              t={t}
              valor={visibilidad}
              alCambiar={setVisibilidad}
              leyenda={t('upload.defaultVisibility')}
              denso
            />
          </aside>
        </div>
        {falloCuota === 'sinPermiso' ? <Fallo t={t} tipo="sinPermiso" /> : null}
      </div>
    </Marco>
  );
}
