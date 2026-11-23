/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Firmar en este dispositivo» en la portada de CertiPDF, para quien tiene su Drive en
 * la bóveda (ADR 0086). El servidor no puede leer esos PDF, así que la lista, el
 * descifrado, la firma y el cifrado de vuelta ocurren aquí (`dispositivo/`).
 *
 * La bóveda NO se abre sola: abrirla puede pedir volver a entrar en Sytue ID, y eso no
 * se hace al cargar una portada. Se abre al pulsar, se queda abierta mientras se ve
 * esta sección y se suelta al salir.
 *
 * Y se dice por qué «Pedir firma» no está: pedirla a otra persona u organización
 * necesita que el servicio lea el PDF, que es justo lo que la bóveda impide.
 *
 * El diálogo de firmar (`DialogoFirmar`) es el mismo aquí y en el estudio de un PDF de
 * la bóveda (`EstudioBoveda.tsx`): firma invisible por defecto o un recuadro visible
 * (página, posición y rúbrica opcional, la misma apariencia que el servidor), y sello
 * de tiempo si el despliegue tiene autoridad; si no la tiene, lo dice ANTES de firmar.
 */
import { useEffect, useId, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  SegmentedControl,
  Spinner,
} from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { ConsumerPrivateDrive } from '@sytue/offline/vault/drive';
import type { DocsKey, docsT } from '../i18n';
import { abrirRepositorioCifrado } from '../editor/state/driveCifrado';
import { r } from '../lib/rutas';
import { leerPkcs12 } from './dispositivo/pkcs12';
import {
  firmarEnBoveda,
  motivoFirmaLocal,
  pdfsDeBoveda,
  type FirmadoEnBoveda,
  type MotivoFirmaLocal,
  type PdfDeBoveda,
} from './dispositivo/firmarEnBoveda';
import { pedirSelloAlServidor, selloDisponible, type SelloDisponible } from './dispositivo/sello';
import { POSICIONES_FIRMA, type PosicionFirma } from './firmaPosicion';
import { LienzoDeFirma } from './LienzoDeFirma';

/** La frase de cada motivo por el que no se pudo firmar. */
export const MENSAJE_FIRMA_LOCAL: Record<MotivoFirmaLocal, DocsKey> = {
  contrasena: 'pdf.device.error.password',
  certificado: 'pdf.error.certificate',
  cifrado: 'pdf.device.error.encrypted',
  bloqueado: 'pdf.device.error.locked',
  ilegible: 'pdf.device.error.unreadable',
  pagina: 'pdf.device.error.page',
  entrar: 'pdf.device.error.signin',
  pendiente: 'pdf.device.error.pending',
  fallo: 'pdf.device.error.failed',
};

type T = ReturnType<typeof docsT>;

/** Lo que se dice al terminar: con sello, sin él porque falló, o sin autoridad. */
export function mensajeFirmado(
  t: T,
  f: Pick<FirmadoEnBoveda, 'nombre' | 'sello' | 'selloFallido'>,
): string {
  if (f.sello) return t('pdf.device.signedStamped', { name: f.nombre });
  if (f.selloFallido) return t('pdf.device.signedNoStamp', { name: f.nombre });
  return t('pdf.device.signed', { name: f.nombre });
}

/** La ruta del estudio para un PDF de la bóveda (`/pdf?path=`). */
export const rutaDelEstudio = (ruta: string) => r(`/pdf?path=${encodeURIComponent(ruta)}`);

type Apariencia = 'invisible' | 'visible';

/** El diálogo de firmar un PDF de la bóveda con un `.p12`. */
export function DialogoFirmar({
  t,
  pdf,
  drive,
  paginas,
  onCerrar,
  onFirmado,
}: {
  t: T;
  pdf: PdfDeBoveda;
  drive: Pick<ConsumerPrivateDrive, 'repository' | 'signal'>;
  /** Cuántas páginas tiene, si se sabe (el estudio lo sabe; la portada no). */
  paginas?: number | undefined;
  onCerrar: () => void;
  onFirmado: (firmado: FirmadoEnBoveda) => void;
}) {
  const id = useId();
  const [p12, setP12] = useState<Uint8Array<ArrayBuffer> | null>(null);
  const [clave, setClave] = useState('');
  const [motivo, setMotivo] = useState('');
  const [lugar, setLugar] = useState('');
  const [apariencia, setApariencia] = useState<Apariencia>('invisible');
  const [pagina, setPagina] = useState('1');
  const [posicion, setPosicion] = useState<PosicionFirma>(POSICIONES_FIRMA[0]!);
  const [trazos, setTrazos] = useState<[number, number][][]>([]);
  const [sello, setSello] = useState<SelloDisponible | null>(null);
  const [firmando, setFirmando] = useState(false);
  const [error, setError] = useState('');

  // Si habrá sello se dice ANTES de firmar, no al terminar.
  useEffect(() => {
    const vida = new AbortController();
    void selloDisponible(vida.signal).then((s) => {
      if (!vida.signal.aborted) setSello(s);
    });
    return () => vida.abort();
  }, []);

  // El `.p12` no se queda en memoria al cerrar.
  useEffect(
    () => () => {
      p12?.fill(0);
    },
    [p12],
  );

  const firmar = async () => {
    if (!p12 || firmando) return;
    setFirmando(true);
    setError('');
    try {
      const identidad = await leerPkcs12(p12.slice(), clave);
      const hecho = await firmarEnBoveda({
        repositorio: drive.repository,
        id: pdf.id,
        identidad,
        opciones: {
          motivo,
          lugar,
          sello: sello?.disponible ? pedirSelloAlServidor() : undefined,
        },
        visible:
          apariencia === 'visible'
            ? { pagina: Math.max(1, Math.trunc(Number(pagina)) || 1), posicion, trazos }
            : undefined,
        sufijo: t('pdf.suffix.signed'),
        signal: drive.signal,
      });
      setClave('');
      onFirmado(hecho);
    } catch (e) {
      setError(t(MENSAJE_FIRMA_LOCAL[motivoFirmaLocal(e)]));
      setFirmando(false);
    }
  };

  return (
    <Modal
      open
      onClose={() => !firmando && onCerrar()}
      title={t('pdf.dialog.firmar.title')}
      footer={
        <>
          <Button variant="secondary" onClick={onCerrar} disabled={firmando}>
            {t('pdf.cancel')}
          </Button>
          <Button onClick={() => void firmar()} disabled={!p12 || !sello} loading={firmando}>
            {t('pdf.device.sign')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-3">
        <p class="text-text min-w-0 truncate text-sm font-medium" title={pdf.ruta}>
          {pdf.nombre}
        </p>
        <FieldGroup
          label={t('pdf.dialog.firmar.p12')}
          htmlFor={`${id}-p12`}
          help={t('pdf.dialog.firmar.p12Help')}
        >
          <input
            id={`${id}-p12`}
            type="file"
            accept=".p12,.pfx"
            class="sytue-focus-ring text-xs"
            onChange={(e) => {
              const f = (e.target as HTMLInputElement).files?.[0];
              if (!f) return setP12(null);
              void f.arrayBuffer().then((b) => setP12(new Uint8Array(b)));
            }}
          />
        </FieldGroup>
        <FieldGroup label={t('pdf.dialog.firmar.p12Clave')} htmlFor={`${id}-clave`}>
          <Input
            id={`${id}-clave`}
            type="password"
            autocomplete="off"
            value={clave}
            onInput={(e) => setClave(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup label={t('pdf.dialog.firmar.reason')} htmlFor={`${id}-motivo`}>
          <Input
            id={`${id}-motivo`}
            value={motivo}
            onInput={(e) => setMotivo(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup label={t('pdf.dialog.firmar.place')} htmlFor={`${id}-lugar`}>
          <Input
            id={`${id}-lugar`}
            value={lugar}
            onInput={(e) => setLugar(e.currentTarget.value)}
          />
        </FieldGroup>
        <div class="flex flex-col gap-1.5">
          <span id={`${id}-apariencia`} class="text-text text-sm font-medium">
            {t('pdf.device.appearance')}
          </span>
          <SegmentedControl
            size="sm"
            etiquetadoPor={`${id}-apariencia`}
            valor={apariencia}
            onChange={setApariencia}
            opciones={[
              { valor: 'invisible', rotulo: t('pdf.device.appearance.invisible') },
              { valor: 'visible', rotulo: t('pdf.device.appearance.visible') },
            ]}
          />
        </div>
        {apariencia === 'visible' && (
          <div class="flex flex-col gap-3" data-firma-visible>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-[8rem_minmax(0,1fr)]">
              <FieldGroup label={t('pdf.dialog.firmar.page')} htmlFor={`${id}-pagina`}>
                <Input
                  id={`${id}-pagina`}
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={paginas}
                  value={pagina}
                  onInput={(e) => setPagina(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup label={t('pdf.dialog.firmar.posicion')} htmlFor={`${id}-posicion`}>
                <select
                  id={`${id}-posicion`}
                  value={posicion}
                  onChange={(e) => setPosicion(e.currentTarget.value as PosicionFirma)}
                  class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-sm"
                >
                  {POSICIONES_FIRMA.map((p) => (
                    <option key={p} value={p}>
                      {t(`pdf.dialog.firmar.posicion.${p}`)}
                    </option>
                  ))}
                </select>
              </FieldGroup>
            </div>
            <LienzoDeFirma
              trazos={trazos}
              onCambio={setTrazos}
              etiqueta={t('pdf.dialog.firmar.dibuja')}
            />
            <div class="flex flex-wrap items-center justify-between gap-2">
              <FieldHint>{t('pdf.dialog.firmar.dibuja')}</FieldHint>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setTrazos([])}
                disabled={trazos.length === 0}
              >
                {t('pdf.dialog.firmar.limpiar')}
              </Button>
            </div>
          </div>
        )}
        {sello && (
          <FieldHint tone={sello.disponible ? 'info' : 'warning'}>
            <span data-sello={sello.disponible ? 'si' : 'no'}>
              {sello.disponible
                ? t('pdf.device.stamp.on', { authority: sello.autoridad })
                : t('pdf.device.stamp.off')}
            </span>
          </FieldHint>
        )}
        {error && (
          <FieldHint tone="danger" class="min-w-0">
            <span role="alert">{error}</span>
          </FieldHint>
        )}
      </div>
    </Modal>
  );
}

export function FirmarEnDispositivo({
  t,
  locale,
  actor,
  tenant,
}: {
  t: T;
  locale: Locale;
  actor: string;
  tenant: string | undefined;
}) {
  const [drive, setDrive] = useState<ConsumerPrivateDrive | null>(null);
  const [lista, setLista] = useState<PdfDeBoveda[] | null>(null);
  const [abriendo, setAbriendo] = useState(false);
  const [error, setError] = useState('');
  const [hecho, setHecho] = useState('');
  const [elegido, setElegido] = useState<PdfDeBoveda | null>(null);
  // La vida de la sección: al salir se corta lo que aún esté abriendo.
  const [vida] = useState(() => new AbortController());
  useEffect(() => () => vida.abort(), [vida]);
  useEffect(
    () => () => {
      void drive?.close().catch(() => {});
    },
    [drive],
  );

  const cargar = async (d: ConsumerPrivateDrive) => {
    setLista(null);
    setLista(await pdfsDeBoveda(d.repository, d.signal));
  };

  const abrir = async () => {
    if (abriendo) return;
    setAbriendo(true);
    setError('');
    try {
      const d = await abrirRepositorioCifrado({ actor, tenant, signal: vida.signal });
      if (vida.signal.aborted) {
        await d.close();
        return;
      }
      setDrive(d);
      await cargar(d);
    } catch (e) {
      if (!vida.signal.aborted) setError(t(MENSAJE_FIRMA_LOCAL[motivoFirmaLocal(e)]));
    } finally {
      setAbriendo(false);
    }
  };

  return (
    <section aria-labelledby="pdf-device-heading" class="mb-10 flex shrink-0 flex-col gap-3">
      <h2 id="pdf-device-heading" class="sytue-overline text-text-subtle">
        {t('pdf.device.title')}
      </h2>
      <p class="text-text-muted max-w-2xl text-sm text-pretty">{t('pdf.device.help')}</p>
      <InlineHelp tone="info" class="max-w-2xl">
        {t('pdf.device.requests')}
      </InlineHelp>
      {hecho && (
        <FieldHint tone="success">
          <span role="status">{hecho}</span>
        </FieldHint>
      )}
      {error && (
        <FieldHint tone="danger">
          <span role="alert">{error}</span>
        </FieldHint>
      )}
      {!drive ? (
        <div>
          <Button variant="secondary" onClick={() => void abrir()} loading={abriendo}>
            {t('pdf.device.open')}
          </Button>
        </div>
      ) : lista === null ? (
        <p class="text-text-muted flex items-center gap-2 text-sm" aria-busy="true">
          <Spinner size={16} />
          {t('list.loading')}
        </p>
      ) : lista.length === 0 ? (
        <EmptyState
          compact
          title={t('pdf.device.empty.title')}
          description={t('pdf.device.empty.description')}
        />
      ) : (
        <ul class="flex flex-col gap-2" data-pdf-boveda>
          {lista.map((p) => (
            <li
              key={p.id}
              class="border-border bg-surface flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm"
            >
              {/* El nombre abre el PDF en el estudio, descifrado en memoria: verlo y firmarlo. */}
              <a
                href={rutaDelEstudio(p.ruta)}
                class="sytue-focus-ring flex min-w-0 flex-col rounded"
                aria-label={`${t('pdf.device.view')}: ${p.nombre}`}
              >
                <span class="text-text truncate font-medium hover:underline" title={p.ruta}>
                  {p.nombre.replace(/\.pdf$/i, '')}
                </span>
                <span class="text-text-muted truncate text-xs">
                  {p.ruta}
                  {p.modificado ? ` · ${formatDate(locale, p.modificado)}` : ''}
                </span>
              </a>
              <Button
                size="sm"
                onClick={() => {
                  setHecho('');
                  setElegido(p);
                }}
              >
                {t('pdf.device.sign')}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {elegido && drive && (
        <DialogoFirmar
          t={t}
          pdf={elegido}
          drive={drive}
          onCerrar={() => setElegido(null)}
          onFirmado={(f) => {
            setElegido(null);
            setHecho(mensajeFirmado(t, f));
            void cargar(drive).catch(() => {});
          }}
        />
      )}
    </section>
  );
}
