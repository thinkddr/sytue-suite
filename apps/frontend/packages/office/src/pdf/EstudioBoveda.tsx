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
 * su ruta, bajando carpeta a carpeta— y lo descifra EN MEMORIA, en este dispositivo:
 *
 * - para VERLO, con el visor de PDF del navegador sobre un `blob:` local (solo
 *   lectura; nada sale del navegador);
 * - para FIRMARLO con el `.p12` de la persona (el mismo diálogo que la portada:
 *   invisible o visible, con sello si el despliegue tiene autoridad), dejando el
 *   firmado cifrado a su lado;
 * - para DESCARGARLO (ya en claro, porque es de la persona y lo pide ella).
 *
 * Lo que necesita que el servicio LEA el PDF (anotar, rellenar, organizar, editar,
 * reconocer texto, pedir firma…) se enseña desactivado y se dice por qué, en vez de
 * esconderlo o fallar al pulsarlo.
 *
 * La bóveda NO se abre sola —abrirla puede pedir volver a entrar en Sytue ID—: se abre
 * al pulsar, con la puerta de consumidor de siempre (la bóveda solo si existe, el
 * repositorio solo con su recibo, `importar: false`), y se suelta al salir.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Button, EmptyState, FieldHint, InlineHelp, Spinner } from '@sytue/ui';
import { appUrl } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import type { ConsumerPrivateDrive } from '@sytue/offline/vault/drive';
import { docsT } from '../i18n';
import { r } from '../lib/rutas';
import { abrirRepositorioCifrado, driveEnBoveda } from '../editor/state/driveCifrado';
import { abrirPdfDeBoveda, motivoFirmaLocal, type PdfDeBoveda } from './dispositivo/firmarEnBoveda';
import { DocumentoPdf } from './dispositivo/pdfObjetos';
import {
  DialogoFirmar,
  MENSAJE_FIRMA_LOCAL,
  mensajeFirmado,
  rutaDelEstudio,
} from './FirmarEnDispositivo';
import { PdfApp, type PdfAppProps } from './PdfApp';

/** Lo que el estudio hace con el servicio y aquí no se puede: se enseña apagado. */
const SOLO_SERVIDOR = [
  'pdf.menu.organize',
  'pdf.menu.tools',
  'pdf.menu.protect',
  'pdf.menu.requestSignature',
] as const;

/** Abre el Drive de la bóveda; sustituible en pruebas. */
export type AbrirDrive = (signal: AbortSignal) => Promise<ConsumerPrivateDrive>;

/** Lo abierto: el PDF (sin sus bytes, ya en el blob), su vista y cuántas páginas tiene. */
interface Abierto {
  pdf: PdfDeBoveda;
  url: string;
  paginas: number | undefined;
}

export function EstudioBoveda({
  path,
  locale,
  actor,
  tenant,
  abrirDrive,
}: {
  path: string;
  locale: Locale;
  actor: string;
  tenant: string | undefined;
  abrirDrive?: AbrirDrive | undefined;
}) {
  const t = useMemo(() => docsT(locale), [locale]);
  const nombre = path.split('/').filter(Boolean).pop() ?? path;
  const [drive, setDrive] = useState<ConsumerPrivateDrive | null>(null);
  const [abierto, setAbierto] = useState<Abierto | null>(null);
  const [noEsta, setNoEsta] = useState(false);
  const [abriendo, setAbriendo] = useState(false);
  const [error, setError] = useState('');
  const [firmando, setFirmando] = useState(false);
  const [hecho, setHecho] = useState<{ texto: string; ruta: string } | null>(null);
  const [vida] = useState(() => new AbortController());
  useEffect(() => () => vida.abort(), [vida]);
  // Al salir se suelta la bóveda y la vista en claro.
  useEffect(
    () => () => {
      void drive?.close().catch(() => {});
    },
    [drive],
  );
  useEffect(
    () => () => {
      if (abierto) URL.revokeObjectURL(abierto.url);
    },
    [abierto],
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
      const pdf = await abrirPdfDeBoveda(d.repository, path, d.signal);
      if (!pdf) {
        setNoEsta(true);
        return;
      }
      const { datos, ...resto } = pdf;
      try {
        // Cuántas páginas: para ofrecer solo las que hay al firmar visible.
        const paginas = await DocumentoPdf.abrir(datos)
          .then((doc) => doc.paginas())
          .then((p) => p.length)
          .catch(() => undefined);
        const url = URL.createObjectURL(new Blob([datos], { type: 'application/pdf' }));
        setAbierto({ pdf: resto, url, paginas });
      } finally {
        // El blob tiene su copia; la de aquí se borra.
        datos.fill(0);
      }
    } catch (e) {
      if (!vida.signal.aborted) setError(t(MENSAJE_FIRMA_LOCAL[motivoFirmaLocal(e)]));
    } finally {
      setAbriendo(false);
    }
  };

  const carpeta = path.replace(/\/[^/]*$/, '');

  return (
    <div
      class="sytue-sheet-corners mx-auto flex h-full min-h-0 w-full max-w-[var(--container-content,1280px)] flex-col gap-4 overflow-y-auto px-4 pt-6 pb-8 sm:px-6"
      data-estudio-boveda
    >
      <header class="flex shrink-0 flex-wrap items-center justify-between gap-3">
        <div class="flex min-w-0 flex-col gap-1">
          <a href={r('/')} class="text-text-muted sytue-focus-ring w-fit rounded text-xs underline">
            {t('pdf.list.heading')}
          </a>
          <h1 class="sytue-h3 text-text min-w-0 truncate" title={path}>
            {nombre}
          </h1>
        </div>
        {abierto && (
          <div class="flex flex-wrap items-center gap-2">
            <a
              href={abierto.url}
              download={abierto.pdf.nombre}
              class="border-border bg-surface text-text hover:bg-bg-subtle sytue-focus-ring rounded-control inline-flex min-h-8 items-center border px-3 text-sm"
            >
              {t('pdf.menu.download')}
            </a>
            <Button
              onClick={() => {
                setHecho(null);
                setFirmando(true);
              }}
            >
              {t('pdf.menu.sign')}
            </Button>
          </div>
        )}
      </header>

      <p class="text-text-muted max-w-2xl text-sm text-pretty">{t('pdf.vault.help')}</p>

      {hecho && (
        <FieldHint tone="success">
          <span role="status">
            {hecho.texto}{' '}
            <a href={rutaDelEstudio(hecho.ruta)} class="sytue-focus-ring rounded underline">
              {t('pdf.vault.openSigned')}
            </a>
          </span>
        </FieldHint>
      )}
      {error && (
        <FieldHint tone="danger">
          <span role="alert">{error}</span>
        </FieldHint>
      )}

      {noEsta ? (
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
      ) : !abierto ? (
        <div>
          <Button variant="secondary" onClick={() => void abrir()} loading={abriendo}>
            {t('pdf.device.open')}
          </Button>
        </div>
      ) : (
        <iframe
          src={abierto.url}
          title={abierto.pdf.nombre}
          class="border-border bg-bg-subtle min-h-[60dvh] w-full flex-1 rounded-lg border"
          data-visor-boveda
        />
      )}

      <InlineHelp tone="info" class="max-w-2xl" live="off">
        <span id="pdf-vault-server-only">{t('pdf.vault.serverOnly')}</span>
      </InlineHelp>
      <div class="flex flex-wrap gap-2" data-solo-servidor>
        {SOLO_SERVIDOR.map((k) => (
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

      {firmando && drive && abierto && (
        <DialogoFirmar
          t={t}
          pdf={abierto.pdf}
          drive={drive}
          paginas={abierto.paginas}
          onCerrar={() => setFirmando(false)}
          onFirmado={(f) => {
            setFirmando(false);
            setHecho({ texto: mensajeFirmado(t, f), ruta: `${carpeta}/${f.nombre}` });
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
    <EstudioBoveda path={app.path} locale={app.locale} actor={actor} tenant={tenant} />
  ) : (
    <PdfApp {...app} />
  );
}
