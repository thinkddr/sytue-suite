/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ESTUDIO DE UN PDF DE LA BÓVEDA (ADR 0086, «Firmar en el dispositivo»).
 *
 * `/pdf?path=` abre un PDF por su ruta del Drive del servidor y lo dibuja el motor del
 * servicio de Docs. Con el Drive de la persona en la bóveda eso no puede ser: el
 * servidor no ve el PDF, y mandárselo para dibujarlo sería justo lo que la bóveda
 * impide. Así que, con la bóveda, `/pdf?path=` abre el MISMO fichero de la bóveda —por
 * su ruta, bajando carpeta a carpeta— y lo descifra EN MEMORIA, en este dispositivo, y
 * lo trabaja con el MISMO motor de PDF compilado a WebAssembly (`dispositivo/motorPdf.ts`,
 * que se descarga solo aquí y solo al abrir).
 *
 * Desde la cuarta entrega el estudio es EL DE SIEMPRE (`PdfApp`, con su cinta, sus
 * paneles y sus diálogos) sobre la fuente del dispositivo (`dispositivo/fuenteLocal.ts`):
 * anotar, rellenar, crear campos e importar sus datos, organizar, sellos y marcas de
 * agua, proteger con contraseña, censurar, editar el texto y las imágenes, el índice,
 * comparar con otro PDF de la bóveda, el historial de versiones, imprimir y leer en voz
 * alta, todo sin el servidor. Lo que se guarda vuelve CIFRADO a la bóveda, como versión
 * nueva del mismo fichero o como fichero nuevo a su lado. Se FIRMA con el `.p12` de la
 * persona (el diálogo de la portada, que además certifica, recuerda la rúbrica y firma
 * un PDF protegido) y se DESCARGA en claro, porque es de la persona y lo pide ella.
 *
 * Lo que solo puede hacer el servidor —pedir firma, reconocer texto, el asistente—
 * sale apagado con su motivo. Si el motor no llega a cargar (un navegador sin
 * WebAssembly, un despliegue sin el módulo), se cae al visor del navegador: verlo,
 * bajarlo y firmarlo siguen funcionando.
 *
 * La bóveda NO se abre sola —abrirla puede pedir volver a entrar en Sytue ID—: se abre
 * al pulsar, con la puerta de consumidor de siempre (la bóveda solo si existe, el
 * repositorio solo con su recibo, `importar: false`), y se suelta al salir.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Button, EmptyState, FieldHint, InlineHelp, Spinner, marcarAppLista } from '@sytue/ui';
import { appUrl } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import type { ConsumerPrivateDrive } from '@sytue/offline/vault/drive';
import { docsT, type DocsKey } from '../i18n';
import { r } from '../lib/rutas';
import { abrirRepositorioCifrado, driveEnBoveda } from '../editor/state/driveCifrado';
import { abrirPdfDeBoveda, motivoFirmaLocal, type PdfDeBoveda } from './dispositivo/firmarEnBoveda';
import { DocumentoPdf } from './dispositivo/pdfObjetos';
import { motorDelNavegador, type MotorPdf } from './dispositivo/motorPdf';
import { fuenteLocal, type FuenteLocal } from './dispositivo/fuenteLocal';
import {
  DialogoFirmar,
  MENSAJE_FIRMA_LOCAL,
  mensajeFirmado,
  rutaDelEstudio,
  type PdfProtegido,
} from './FirmarEnDispositivo';
import { ErrorPDF, pideContrasena, type Contenido } from './api';
import { mensajeDeError } from './errores';
import type { NoDisponible } from './Barra';
import { PdfApp, type PdfAppProps } from './PdfApp';

/** Lo que nunca se hace aquí: se enseña apagado siempre, con su motivo. */
const SOLO_SERVIDOR = ['pdf.menu.requestSignature', 'pdf.tools.ocr'] as const;
/** Lo que hace el motor del dispositivo: apagado si no llega a cargar. */
const CON_EL_MOTOR = [
  'pdf.menu.organize',
  'pdf.menu.tools',
  'pdf.menu.protect',
  'pdf.menu.stamp',
  'pdf.menu.compare',
  'pdf.menu.versions',
  'pdf.menu.print',
] as const;

/** Lo que la cinta del estudio enseña apagado con el motor en el dispositivo. */
const NO_DISPONIBLE: Omit<NoDisponible, 'motivo'> = {
  dialogos: ['pedirFirma'],
  herramientas: [],
  leer: false,
  imprimir: false,
};

/** Abre el Drive de la bóveda; sustituible en pruebas. */
export type AbrirDrive = (signal: AbortSignal) => Promise<ConsumerPrivateDrive>;
/** Carga el motor de PDF; sustituible en pruebas. */
export type CrearMotor = (signal: AbortSignal) => Promise<MotorPdf>;

/** Lo abierto SIN motor: el PDF, sus bytes en claro (solo en memoria) y su `blob:`. */
interface Visor {
  pdf: PdfDeBoveda;
  datos: Uint8Array<ArrayBuffer>;
  url: string;
  paginas: number | undefined;
}

/** Lo que se dice tras firmar, con su enlace al fichero nuevo. */
interface Hecho {
  texto: string;
  ruta?: string | undefined;
  enlace?: DocsKey | undefined;
}

/** El diálogo de firmar abierto: cuántas páginas tiene y si va protegido. */
interface Firmando {
  paginas?: number | undefined;
  protegido?: PdfProtegido | undefined;
}

export function EstudioBoveda({
  path,
  locale,
  actor,
  tenant,
  autor,
  volverA,
  abrirDrive,
  crearMotor,
}: {
  path: string;
  locale: Locale;
  actor: string;
  tenant: string | undefined;
  /** Quien anota, para firmar los comentarios (el `autor` del estudio de siempre). */
  autor?: string | undefined;
  /** Carpeta de Drive desde la que se abrió, si vino de allí. */
  volverA?: string | undefined;
  abrirDrive?: AbrirDrive | undefined;
  crearMotor?: CrearMotor | undefined;
}) {
  const t = useMemo(() => docsT(locale), [locale]);
  const nombre = path.split('/').filter(Boolean).pop() ?? path;
  const carpeta = path.replace(/\/[^/]*$/, '');
  const [drive, setDrive] = useState<ConsumerPrivateDrive | null>(null);
  const [motor, setMotor] = useState<MotorPdf | null>(null);
  const [fuente, setFuente] = useState<FuenteLocal | null>(null);
  const [pdf, setPdf] = useState<PdfDeBoveda | null>(null);
  const [visor, setVisor] = useState<Visor | null>(null);
  const [sinMotor, setSinMotor] = useState(false);
  const [noEsta, setNoEsta] = useState(false);
  const [abriendo, setAbriendo] = useState(false);
  const [error, setError] = useState('');
  const [firmando, setFirmando] = useState<Firmando | null>(null);
  const [hecho, setHecho] = useState<Hecho | null>(null);
  const [vida] = useState(() => new AbortController());
  useEffect(() => () => vida.abort(), [vida]);
  // `sytue:app-ready`: el estudio (PdfApp) pone la marca él mismo al cargar; aquí, el
  // visor del navegador o lo que impide abrir el PDF, ya en pantalla. Sin ella, la puerta
  // de la bóveda esperaba 5 s.
  useEffect(() => {
    if (visor !== null || noEsta || error !== '') marcarAppLista();
  }, [visor, noEsta, error]);
  // Al salir se suelta la bóveda, el motor (su worker y su memoria) y lo que haya en claro.
  useEffect(
    () => () => {
      void drive?.close().catch(() => {});
    },
    [drive],
  );
  useEffect(() => () => motor?.apagar(), [motor]);
  useEffect(
    () => () => {
      void fuente?.cerrar().catch(() => {});
    },
    [fuente],
  );
  useEffect(
    () => () => {
      if (!visor) return;
      URL.revokeObjectURL(visor.url);
      visor.datos.fill(0);
    },
    [visor],
  );

  const abrir = async () => {
    if (abriendo) return;
    setAbriendo(true);
    setError('');
    setNoEsta(false);
    try {
      const d = await (
        abrirDrive ?? ((signal) => abrirRepositorioCifrado({ actor, tenant, signal }))
      )(vida.signal);
      if (vida.signal.aborted) {
        await d.close();
        return;
      }
      setDrive(d);
      const encontrado = await abrirPdfDeBoveda(d.repository, path, d.signal);
      if (!encontrado) {
        setNoEsta(true);
        return;
      }
      const { datos, ...elegido } = encontrado;
      setPdf(elegido);
      // El motor, SOLO ahora: el `import()` del runtime, su worker y el wasm se piden
      // al abrir un PDF de la bóveda, nunca antes ni en el estudio de siempre.
      let m: MotorPdf | undefined;
      try {
        m = await (crearMotor ?? motorDelNavegador)(vida.signal);
        const local = fuenteLocal({
          motor: m,
          repositorio: d.repository,
          signal: d.signal,
          pdf: elegido,
          datos: datos.slice(),
          autor: autor || actor,
        });
        try {
          // Se abre ya para saber si el motor sirve para ESTE PDF; la contraseña, si la
          // pide, la pregunta el estudio con su pantalla de siempre.
          await local.contenido('');
        } catch (e) {
          if (!pideContrasena(e)) {
            await local.cerrar();
            throw e;
          }
        }
        datos.fill(0);
        setMotor(m);
        setFuente(local);
      } catch (e) {
        // Un motor que cargó y no sirve para este PDF se apaga: su worker no se queda.
        m?.apagar();
        if (vida.signal.aborted) {
          datos.fill(0);
          return;
        }
        // Sin motor, el visor del navegador: verlo, bajarlo y firmarlo. Si el motor
        // cargó pero no abre ESTE PDF (un formato raro), se dice eso; si no cargó, que
        // no hay motor.
        if (e instanceof ErrorPDF) setError(mensajeDeError(e, t, 'pdf.error.open'));
        else setSinMotor(true);
        const url = URL.createObjectURL(new Blob([datos], { type: 'application/pdf' }));
        const paginas = await DocumentoPdf.abrir(datos)
          .then((doc) => doc.paginas())
          .then((p) => p.length)
          .catch(() => undefined);
        setVisor({ pdf: elegido, datos, url, paginas });
      }
    } catch (e) {
      if (!vida.signal.aborted) setError(t(MENSAJE_FIRMA_LOCAL[motivoFirmaLocal(e)]));
    } finally {
      setAbriendo(false);
    }
  };

  /**
   * Firmar desde el estudio. Un PDF protegido con contraseña lo reescribe antes sin
   * ella el motor del dispositivo (como el servidor, que firma reescribiendo).
   */
  const firmar = async () => {
    setHecho(null);
    if (!fuente) {
      setFirmando({ paginas: visor?.paginas });
      return;
    }
    let contenido: Contenido | null = null;
    try {
      contenido = await fuente.contenido(fuente.clave());
    } catch {
      contenido = null;
    }
    setFirmando({
      paginas: contenido?.total,
      protegido: contenido?.cifrado
        ? { firmado: (contenido.firmas?.length ?? 0) > 0, desproteger: fuente.sinProteccion }
        : undefined,
    });
  };

  const apagados = fuente
    ? SOLO_SERVIDOR
    : visor
      ? [...CON_EL_MOTOR, ...SOLO_SERVIDOR]
      : SOLO_SERVIDOR;

  return (
    <div class="flex h-full min-h-0 w-full flex-col" data-estudio-boveda>
      {!fuente && (
        <div class="sytue-sheet-corners mx-auto flex w-full max-w-[var(--container-content,1280px)] shrink-0 flex-col gap-3 px-4 pt-6 pb-3 sm:px-6">
          <header class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex min-w-0 flex-col gap-1">
              <a
                href={r('/')}
                class="text-text-muted sytue-focus-ring w-fit rounded text-xs underline"
              >
                {t('pdf.list.heading')}
              </a>
              <h1 class="sytue-h3 text-text min-w-0 truncate" title={path}>
                {nombre}
              </h1>
            </div>
            {visor && (
              <div class="flex flex-wrap items-center gap-2">
                <a
                  href={visor.url}
                  download={visor.pdf.nombre}
                  class="border-border bg-surface text-text hover:bg-bg-subtle sytue-focus-ring rounded-control inline-flex min-h-8 items-center border px-3 text-sm"
                >
                  {t('pdf.menu.download')}
                </a>
                <Button onClick={() => void firmar()}>{t('pdf.menu.sign')}</Button>
              </div>
            )}
          </header>
          <p class="text-text-muted max-w-2xl text-sm text-pretty">{t('pdf.vault.help')}</p>
        </div>
      )}

      {(hecho || error || sinMotor) && (
        <div class="mx-auto flex w-full max-w-[var(--container-content,1280px)] shrink-0 flex-col gap-2 px-4 py-2 sm:px-6">
          {hecho && (
            <FieldHint tone="success">
              <span role="status">
                {hecho.texto}{' '}
                {hecho.ruta && hecho.enlace && (
                  <a href={rutaDelEstudio(hecho.ruta)} class="sytue-focus-ring rounded underline">
                    {t(hecho.enlace)}
                  </a>
                )}
              </span>
            </FieldHint>
          )}
          {error && (
            <FieldHint tone="danger">
              <span role="alert">{error}</span>
            </FieldHint>
          )}
          {sinMotor && (
            <InlineHelp tone="warning" class="max-w-2xl" live="off">
              {t('pdf.vault.engineUnavailable')}
            </InlineHelp>
          )}
        </div>
      )}

      {noEsta ? (
        <div class="px-4 sm:px-6">
          <EmptyState
            title={t('pdf.vault.notFound.title')}
            description={t('pdf.vault.notFound.description')}
            action={
              <Button
                variant="secondary"
                onClick={() => window.location.assign(appUrl('drive', '/'))}
              >
                {t('pdf.backToDrive')}
              </Button>
            }
          />
        </div>
      ) : fuente && pdf ? (
        <div class="flex min-h-0 flex-1 flex-col" data-lienzo-boveda>
          <PdfApp
            path={pdf.ruta}
            locale={locale}
            autor={autor || actor}
            volverA={volverA}
            fuente={fuente}
            noDisponible={{ ...NO_DISPONIBLE, motivo: t('pdf.vault.unavailable') }}
            onFirmar={() => void firmar()}
          />
        </div>
      ) : visor ? (
        <div class="mx-auto flex w-full max-w-[var(--container-content,1280px)] flex-1 px-4 pb-6 sm:px-6">
          <iframe
            src={visor.url}
            title={visor.pdf.nombre}
            class="border-border bg-bg-subtle min-h-[60dvh] w-full flex-1 rounded-lg border"
            data-visor-boveda
          />
        </div>
      ) : (
        <div class="mx-auto w-full max-w-[var(--container-content,1280px)] px-4 sm:px-6">
          <Button variant="secondary" onClick={() => void abrir()} loading={abriendo}>
            {t('pdf.device.open')}
          </Button>
        </div>
      )}

      <div class="mx-auto flex w-full max-w-[var(--container-content,1280px)] shrink-0 flex-col gap-2 px-4 py-3 sm:px-6">
        <InlineHelp tone="info" class="max-w-2xl" live="off">
          <span id="pdf-vault-server-only">{t('pdf.vault.serverOnly')}</span>
        </InlineHelp>
        <div class="flex flex-wrap gap-2" data-solo-servidor>
          {apagados.map((k) => (
            <Button
              key={k}
              size="sm"
              variant="secondary"
              disabled
              aria-describedby="pdf-vault-server-only"
            >
              {t(k)}
            </Button>
          ))}
        </div>
      </div>

      {firmando && drive && pdf && (
        <DialogoFirmar
          t={t}
          pdf={pdf}
          drive={drive}
          paginas={firmando.paginas}
          protegido={firmando.protegido}
          onCerrar={() => setFirmando(null)}
          onFirmado={(f) => {
            setFirmando(null);
            setHecho({
              texto: mensajeFirmado(t, f),
              ruta: `${carpeta}/${f.nombre}`,
              enlace: 'pdf.vault.openSigned',
            });
          }}
        />
      )}
    </div>
  );
}

/**
 * `/pdf?path=`: el estudio de siempre o, con el Drive en la bóveda, el de la bóveda.
 * Se mira sin claves (el estado de la fuente); ante la duda, el de siempre.
 */
export function EstudioPdf(props: PdfAppProps & { actor: string; tenant: string | undefined }) {
  const { actor, tenant, ...app } = props;
  const [enBoveda, setEnBoveda] = useState<boolean | null>(null);
  useEffect(() => {
    const vida = new AbortController();
    void driveEnBoveda(actor, tenant, undefined, vida.signal).then((si) => {
      if (!vida.signal.aborted) setEnBoveda(si);
    });
    return () => vida.abort();
  }, [actor, tenant]);
  if (enBoveda === null)
    return (
      <div class="flex h-full items-center justify-center" aria-busy="true">
        <Spinner size={24} />
      </div>
    );
  return enBoveda ? (
    <EstudioBoveda
      path={app.path}
      locale={app.locale}
      actor={actor}
      tenant={tenant}
      autor={app.autor}
      volverA={app.volverA}
    />
  ) : (
    <PdfApp {...app} />
  );
}
