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
 * que se descarga solo aquí y solo al abrir):
 *
 * - se VE con el lienzo SVG del estudio (`Hoja`, la cinta, el zoom), no con el visor
 *   del navegador;
 * - se ANOTA, se RELLENA, se ORGANIZA (extraer, borrar, girar, reordenar, combinar con
 *   otros PDF de la bóveda) y se le pasan las HERRAMIENTAS (comprimir, recortar,
 *   archivo PDF/A), con los mismos diálogos y la misma tabla de operaciones que el
 *   estudio del servidor (`dispositivo/aplicarLocal.ts`). Cada cambio vuelve CIFRADO a
 *   la bóveda como versión nueva del mismo fichero o, donde el servidor deja copia,
 *   como fichero nuevo a su lado, sin pisar ningún nombre (`dispositivo/editarEnBoveda.ts`);
 * - se FIRMA con el `.p12` de la persona (el mismo diálogo que la portada);
 * - se DESCARGA (ya en claro, porque es de la persona y lo pide ella).
 *
 * Lo que solo puede hacer el servidor (pedir firma, reconocer texto) o que aún no se
 * hace en el dispositivo (proteger, censurar, editar texto e imágenes, sellos, comparar,
 * versiones, imprimir, leer en voz alta) sale apagado con su motivo. Si el motor no
 * llega a cargar (un navegador sin WebAssembly, un despliegue sin el módulo), se cae al
 * visor del navegador de antes: verlo, bajarlo y firmarlo siguen funcionando.
 *
 * La bóveda NO se abre sola —abrirla puede pedir volver a entrar en Sytue ID—: se abre
 * al pulsar, con la puerta de consumidor de siempre (la bóveda solo si existe, el
 * repositorio solo con su recibo, `importar: false`), y se suelta al salir.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Button,
  DialogosDelEditor,
  EmptyState,
  FieldHint,
  InlineHelp,
  Spinner,
  confirmarEnEditor,
  cx,
} from '@sytue/ui';
import { appUrl } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import type { ConsumerPrivateDrive } from '@sytue/offline/vault/drive';
import { docsT, type DocsKey } from '../i18n';
import { r } from '../lib/rutas';
import { abrirRepositorioCifrado, driveEnBoveda } from '../editor/state/driveCifrado';
import { abrirPdfDeBoveda, motivoFirmaLocal, type PdfDeBoveda } from './dispositivo/firmarEnBoveda';
import { DocumentoPdf } from './dispositivo/pdfObjetos';
import { motorDelNavegador, type MotorPdf } from './dispositivo/motorPdf';
import { aplicarEnDispositivo } from './dispositivo/aplicarLocal';
import { guardarPdfEnBoveda } from './dispositivo/editarEnBoveda';
import {
  DialogoFirmar,
  MENSAJE_FIRMA_LOCAL,
  mensajeFirmado,
  rutaDelEstudio,
} from './FirmarEnDispositivo';
import {
  ErrorPDF,
  invalidaFirmas,
  type AnotacionNueva,
  type Contenido,
  type InformeArchivo,
} from './api';
import { mensajeDeError } from './errores';
import { BarraHerramientas, type Herramienta, type NoDisponible } from './Barra';
import { CampoEditable, Dialogos, type Aplicar, type Dialogo } from './Paneles';
import { Hoja, PdfApp, type PdfAppProps } from './PdfApp';

/** Lo que nunca se hace aquí: se enseña apagado antes de abrir y si el motor no carga. */
const SOLO_SERVIDOR = [
  'pdf.menu.requestSignature',
  'pdf.menu.protect',
  'pdf.menu.compare',
  'pdf.menu.versions',
] as const;
/** Lo que hace el motor del dispositivo: apagado también si no llega a cargar. */
const CON_EL_MOTOR = ['pdf.menu.organize', 'pdf.menu.tools'] as const;

/** Lo que la cinta enseña apagado con el motor en el dispositivo (ver arriba). */
const NO_DISPONIBLE: Omit<NoDisponible, 'motivo'> = {
  dialogos: ['pedirFirma', 'proteger', 'capas', 'comparar', 'versiones'],
  herramientas: ['campo', 'recorte', 'censura'],
  leer: true,
  imprimir: true,
};

/** Cuántas páginas se dibujan de golpe, como el estudio del servidor. */
const LOTE = 4;

/** Abre el Drive de la bóveda; sustituible en pruebas. */
export type AbrirDrive = (signal: AbortSignal) => Promise<ConsumerPrivateDrive>;
/** Carga el motor de PDF; sustituible en pruebas. */
export type CrearMotor = (signal: AbortSignal) => Promise<MotorPdf>;

/** Lo abierto: el PDF, sus bytes en claro (solo en memoria), su `blob:` y cuántas páginas. */
interface Abierto {
  pdf: PdfDeBoveda;
  datos: Uint8Array<ArrayBuffer>;
  url: string;
  paginas: number | undefined;
  /** Con el motor: el documento abierto en él y lo que dice de él. */
  doc?: string | undefined;
  contenido?: Contenido | undefined;
}

/** Lo que se dice tras un cambio o una firma, con su enlace si dejó un fichero nuevo. */
interface Hecho {
  texto: string;
  ruta?: string | undefined;
  enlace?: DocsKey | undefined;
}

export function EstudioBoveda({
  path,
  locale,
  actor,
  tenant,
  autor,
  abrirDrive,
  crearMotor,
}: {
  path: string;
  locale: Locale;
  actor: string;
  tenant: string | undefined;
  /** Quien anota, para firmar los comentarios (el `autor` del estudio de siempre). */
  autor?: string | undefined;
  abrirDrive?: AbrirDrive | undefined;
  crearMotor?: CrearMotor | undefined;
}) {
  const t = useMemo(() => docsT(locale), [locale]);
  const nombre = path.split('/').filter(Boolean).pop() ?? path;
  const carpeta = path.replace(/\/[^/]*$/, '');
  const [drive, setDrive] = useState<ConsumerPrivateDrive | null>(null);
  const [motor, setMotor] = useState<MotorPdf | null>(null);
  const [sinMotor, setSinMotor] = useState(false);
  const [abierto, setAbierto] = useState<Abierto | null>(null);
  const [noEsta, setNoEsta] = useState(false);
  const [abriendo, setAbriendo] = useState(false);
  const [trabajando, setTrabajando] = useState(false);
  const [error, setError] = useState('');
  const [firmando, setFirmando] = useState(false);
  const [hecho, setHecho] = useState<Hecho | null>(null);
  const [svgs, setSvgs] = useState<Record<number, string>>({});
  const [herramienta, setHerramienta] = useState<Herramienta>('mano');
  const [color, setColor] = useState('#f9b923');
  const [grosor, setGrosor] = useState(2);
  const [zoom, setZoom] = useState(1);
  const [ajuste, setAjuste] = useState<'ancho' | 'pagina' | 'libre'>('libre');
  const [dosPaginas, setDosPaginas] = useState(false);
  const [pagina, setPagina] = useState(1);
  const [dialogo, setDialogo] = useState<Dialogo>(null);
  const [panelAbierto, setPanelAbierto] = useState(false);
  const hojas = useRef<HTMLDivElement>(null);
  const [vida] = useState(() => new AbortController());
  useEffect(() => () => vida.abort(), [vida]);
  // Al salir se suelta la bóveda, el motor (su worker y su memoria) y la vista en claro.
  useEffect(
    () => () => {
      void drive?.close().catch(() => {});
    },
    [drive],
  );
  useEffect(() => () => motor?.apagar(), [motor]);
  const urlAbierta = abierto?.url;
  const datosAbiertos = abierto?.datos;
  useEffect(
    () => () => {
      if (urlAbierta) URL.revokeObjectURL(urlAbierta);
    },
    [urlAbierta],
  );
  useEffect(() => () => datosAbiertos?.fill(0), [datosAbiertos]);

  /** Lo que se enseña de un fallo: la frase del motor, de la bóveda o la genérica. */
  const fraseDeError = useCallback(
    (e: unknown, generico: DocsKey) => {
      if (e instanceof ErrorPDF) return mensajeDeError(e, t, generico);
      const motivo = motivoFirmaLocal(e);
      return motivo === 'fallo' ? t(generico) : t(MENSAJE_FIRMA_LOCAL[motivo]);
    },
    [t],
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
      const url = URL.createObjectURL(new Blob([datos], { type: 'application/pdf' }));
      // El motor, SOLO ahora: el `import()` del runtime, su worker y el wasm se piden
      // al abrir un PDF de la bóveda, nunca antes ni en el estudio de siempre.
      let m: MotorPdf | undefined;
      try {
        m = await (crearMotor ?? motorDelNavegador)(vida.signal);
        const { doc, contenido } = await m.abrir(datos);
        setMotor(m);
        setAbierto({ pdf: resto, datos, url, paginas: contenido.total, doc, contenido });
      } catch (e) {
        // Un motor que cargó y no sirve para este PDF se apaga: su worker no se queda.
        m?.apagar();
        if (vida.signal.aborted) {
          URL.revokeObjectURL(url);
          datos.fill(0);
          return;
        }
        // Sin motor, el visor del navegador de antes: verlo, bajarlo y firmarlo. Si el
        // motor cargó pero no abre ESTE PDF (una contraseña, un formato raro), se dice
        // eso; si no cargó, que no hay motor.
        if (e instanceof ErrorPDF) setError(mensajeDeError(e, t, 'pdf.error.open'));
        else setSinMotor(true);
        const paginas = await DocumentoPdf.abrir(datos)
          .then((doc) => doc.paginas())
          .then((p) => p.length)
          .catch(() => undefined);
        setAbierto({ pdf: resto, datos, url, paginas });
      }
    } catch (e) {
      if (!vida.signal.aborted) setError(t(MENSAJE_FIRMA_LOCAL[motivoFirmaLocal(e)]));
    } finally {
      setAbriendo(false);
    }
  };

  /* --- Dibujo: todas las páginas, por lotes y en orden, con el motor local ------ */

  const doc = abierto?.doc;
  const total = abierto?.contenido?.total ?? 0;
  useEffect(() => {
    if (!motor || !doc) return;
    let vivo = true;
    void (async () => {
      for (let desde = 1; desde <= total && vivo; desde += LOTE) {
        const hasta = Math.min(desde + LOTE - 1, total);
        try {
          const lote = await motor.render(doc, `${desde}-${hasta}`, 1);
          if (!vivo) return;
          setSvgs((prev) => {
            const out = { ...prev };
            // Una página que el motor no supo dibujar se marca vacía y enseña el hueco.
            for (let n = desde; n <= hasta; n++) out[n] = lote[n] ?? '';
            return out;
          });
        } catch (e) {
          if (vivo) setError(fraseDeError(e, 'pdf.error.render'));
          return;
        }
      }
    })();
    return () => {
      vivo = false;
    };
  }, [motor, doc, total, fraseDeError]);

  /** Tras guardar una versión nueva: el motor abre los bytes nuevos y se dibuja todo. */
  const recargar = async (m: MotorPdf, actual: Abierto, datos: Uint8Array<ArrayBuffer>) => {
    const { doc: nuevo, contenido } = await m.abrir(datos);
    if (actual.doc) await m.cerrar(actual.doc);
    setSvgs({});
    setAbierto({
      ...actual,
      datos,
      url: URL.createObjectURL(new Blob([datos], { type: 'application/pdf' })),
      paginas: contenido.total,
      doc: nuevo,
      contenido,
    });
  };

  /* --- Aplicar: el mismo cuerpo que el estudio del servidor, en el dispositivo --- */

  const aplicar: Aplicar = async (pedido, mensaje) => {
    if (!motor || !drive || !abierto?.doc) return false;
    let cuerpo = pedido;
    for (;;) {
      setTrabajando(true);
      setError('');
      setHecho(null);
      try {
        const cambio = await aplicarEnDispositivo({
          motor,
          doc: abierto.doc,
          nombre: abierto.pdf.nombre,
          cuerpo,
          autor: autor || actor,
          // Los otros de una combinación: por su ruta de la bóveda, descifrados aquí.
          abrirOtro: async (ruta) => {
            const otro = await abrirPdfDeBoveda(drive.repository, ruta, drive.signal);
            if (!otro) throw new ErrorPDF('download_failed', 'download_failed', 404);
            try {
              return (await motor.abrir(otro.datos)).doc;
            } finally {
              otro.datos.fill(0);
            }
          },
        });
        let guardado;
        try {
          guardado = await guardarPdfEnBoveda({
            repositorio: drive.repository,
            id: abierto.pdf.id,
            datos: cambio.datos,
            sufijo: cambio.copia ? cambio.sufijo : undefined,
            signal: drive.signal,
          });
        } catch (e) {
          cambio.datos.fill(0);
          throw e;
        }
        const partes: string[] = [];
        if (cambio.pdfa === 'perdido') partes.push(t('pdf.saved.pdfa.lost'));
        if (cambio.pdfa === 'rebajado') partes.push(t('pdf.saved.pdfa.downgraded'));
        if (guardado.nuevo) {
          cambio.datos.fill(0);
          partes.push(t('pdf.saved.copy', { name: guardado.nombre }));
          setHecho({
            texto: partes.join(' · '),
            ruta: `${carpeta}/${guardado.nombre}`,
            enlace: 'pdf.saved.open',
          });
        } else {
          await recargar(motor, abierto, cambio.datos);
          if (mensaje) partes.push(mensaje);
          if (partes.length) setHecho({ texto: partes.join(' · ') });
        }
        return true;
      } catch (e) {
        // Firmado y sin poder anexarse: se pregunta, y solo con un sí se repite
        // permitiendo que las firmas se pierdan, como en el estudio del servidor.
        const campos = (cuerpo.campos as Record<string, unknown> | undefined) ?? {};
        if (invalidaFirmas(e) && !campos.invalidar_firmas) {
          setTrabajando(false);
          const seguir = await confirmarEnEditor({
            mensaje: t('pdf.signatures.invalidate.confirm'),
            confirmLabel: t('pdf.signatures.invalidate.apply'),
            tone: 'danger',
          });
          if (!seguir) return false;
          cuerpo = { ...cuerpo, campos: { ...campos, invalidar_firmas: '1' } };
          continue;
        }
        setError(fraseDeError(e, 'pdf.error.apply'));
        return false;
      } finally {
        setTrabajando(false);
      }
    }
  };

  const anotar = (anots: AnotacionNueva[]) => {
    if (anots.length === 0) return;
    const quien = autor || actor;
    void aplicar(
      { op: 'anotar', datos: anots.map((a) => ({ ...a, autor: quien })) },
      t('pdf.saved.annotated'),
    );
  };

  const comprobarArchivo = async (): Promise<InformeArchivo> => {
    if (!motor || !abierto?.doc) throw new ErrorPDF('pdf_failed', 'pdf_failed');
    const r = await motor.herramienta(abierto.doc, { op: 'comprobar-archivo' });
    try {
      return JSON.parse(new TextDecoder().decode(r.datos)) as InformeArchivo;
    } finally {
      r.datos.fill(0);
    }
  };

  /* --- Ver: página, zoom y ajuste ------------------------------------------ */

  const contenido = abierto?.contenido;
  const irA = (n: number) => {
    if (!contenido) return;
    const destino = Math.min(Math.max(1, Math.round(n)), contenido.total);
    setPagina(destino);
    hojas.current
      ?.querySelector(`[data-pagina="${destino}"]`)
      ?.scrollIntoView?.({ block: 'start', behavior: 'smooth' });
  };
  useEffect(() => {
    const caja = hojas.current;
    const primera = contenido?.paginas[0];
    if (!caja || !primera || ajuste === 'libre') return;
    const ancho = Math.max(...contenido.paginas.map((p) => p.ancho)) * (dosPaginas ? 2 : 1);
    const z =
      ajuste === 'ancho'
        ? (caja.clientWidth - 64) / ancho
        : Math.min((caja.clientWidth - 64) / ancho, (caja.clientHeight - 64) / primera.alto);
    if (Number.isFinite(z) && z > 0) setZoom(Math.min(4, Math.max(0.25, z)));
  }, [ajuste, contenido, dosPaginas]);
  const alDesplazar = () => {
    const caja = hojas.current;
    if (!caja) return;
    const arriba = caja.getBoundingClientRect().top;
    for (const h of caja.querySelectorAll<HTMLElement>('[data-pagina]')) {
      if (h.getBoundingClientRect().bottom > arriba + 24) {
        setPagina(Number(h.dataset.pagina) || 1);
        return;
      }
    }
  };

  const apagados = motor ? [] : abierto ? [...CON_EL_MOTOR, ...SOLO_SERVIDOR] : SOLO_SERVIDOR;

  return (
    <div class="flex h-full min-h-0 w-full flex-col" data-estudio-boveda>
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
          {abierto && (
            <div class="flex flex-wrap items-center gap-2">
              {!motor && (
                <a
                  href={abierto.url}
                  download={abierto.pdf.nombre}
                  class="border-border bg-surface text-text hover:bg-bg-subtle sytue-focus-ring rounded-control inline-flex min-h-8 items-center border px-3 text-sm"
                >
                  {t('pdf.menu.download')}
                </a>
              )}
              <Button
                disabled={trabajando}
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
      ) : !abierto ? (
        <div class="mx-auto w-full max-w-[var(--container-content,1280px)] px-4 sm:px-6">
          <Button variant="secondary" onClick={() => void abrir()} loading={abriendo}>
            {t('pdf.device.open')}
          </Button>
        </div>
      ) : motor && contenido ? (
        <div class="flex min-h-0 flex-1 flex-col">
          <BarraHerramientas
            t={t}
            herramienta={herramienta}
            onHerramienta={setHerramienta}
            color={color}
            onColor={setColor}
            grosor={grosor}
            onGrosor={setGrosor}
            zoom={zoom}
            ajuste={ajuste}
            onZoom={(delta) => {
              setAjuste('libre');
              setZoom((z) => Math.min(4, Math.max(0.25, Math.round((z + delta) * 100) / 100)));
            }}
            onAjuste={setAjuste}
            dosPaginas={dosPaginas}
            onDosPaginas={setDosPaginas}
            leyendo={false}
            onLeer={() => undefined}
            pagina={pagina}
            total={contenido.total}
            onIrA={irA}
            onDialogo={(d) => {
              if (d === 'firmar') {
                setHecho(null);
                setFirmando(true);
              } else setDialogo(d);
            }}
            permisos={contenido.permisos}
            trabajando={trabajando}
            urlDescarga={abierto.url}
            nombreDescarga={abierto.pdf.nombre}
            onImprimir={() => undefined}
            panelAbierto={panelAbierto}
            onPanelAbierto={setPanelAbierto}
            noDisponible={{ ...NO_DISPONIBLE, motivo: t('pdf.vault.unavailable') }}
          />
          <div class="flex min-h-0 flex-1">
            <div
              ref={hojas}
              class="sytue-focus-ring-inner relative flex-1 overflow-auto px-6 py-6"
              data-lienzo-boveda
              data-herramienta={herramienta}
              role="region"
              aria-label={t('pdf.panel.pages')}
              // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
              tabIndex={0}
              onScroll={alDesplazar}
            >
              <div
                data-sin-tactil
                class={`mx-auto grid w-fit justify-center gap-6 ${
                  dosPaginas ? 'grid-cols-2 items-start' : 'grid-cols-1 items-center'
                }`}
              >
                {contenido.paginas.map((p) => (
                  <Hoja
                    key={`${abierto.doc}-${p.numero}`}
                    pagina={p}
                    svg={svgs[p.numero]}
                    zoom={zoom}
                    herramienta={herramienta}
                    color={color}
                    grosor={grosor}
                    anotaciones={(contenido.anotaciones ?? []).filter(
                      (a) => a.pagina === p.numero - 1,
                    )}
                    zonas={[]}
                    campos={[]}
                    enlaces={(contenido.enlaces ?? []).filter((e) => e.pagina === p.numero - 1)}
                    onIrA={irA}
                    onAnotar={anotar}
                    onCensurar={() => undefined}
                    onCampo={() => undefined}
                    onRecorte={() => undefined}
                    t={t}
                  />
                ))}
              </div>
            </div>
            <aside
              id="pdf-panel"
              class={cx(
                'border-border bg-surface w-72 shrink-0 overflow-y-auto border-l p-3 max-lg:absolute max-lg:inset-y-0 max-lg:right-0 max-lg:z-10 max-lg:shadow-lg',
                panelAbierto ? 'block' : 'hidden lg:block',
              )}
              aria-label={t('pdf.panel.fields')}
            >
              <h2 class="text-text mb-2 text-sm font-medium">{t('pdf.panel.fields')}</h2>
              <CamposLocales
                t={t}
                contenido={contenido}
                trabajando={trabajando}
                onAplicar={aplicar}
                onIrA={irA}
              />
            </aside>
          </div>
        </div>
      ) : (
        <div class="mx-auto flex w-full max-w-[var(--container-content,1280px)] flex-1 px-4 pb-6 sm:px-6">
          <iframe
            src={abierto.url}
            title={abierto.pdf.nombre}
            class="border-border bg-bg-subtle min-h-[60dvh] w-full flex-1 rounded-lg border"
            data-visor-boveda
          />
        </div>
      )}

      <div class="mx-auto flex w-full max-w-[var(--container-content,1280px)] shrink-0 flex-col gap-2 px-4 py-3 sm:px-6">
        <InlineHelp tone="info" class="max-w-2xl" live="off">
          <span id="pdf-vault-server-only">{t('pdf.vault.serverOnly')}</span>
        </InlineHelp>
        {apagados.length > 0 && (
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
        )}
      </div>

      {motor && contenido && (
        <Dialogos
          dialogo={dialogo}
          onCerrar={() => setDialogo(null)}
          t={t}
          contenido={contenido}
          path={path}
          onAplicar={aplicar}
          trabajando={trabajando}
          svgs={svgs}
          onRestaurada={() => undefined}
          locale={locale}
          local={{ comprobarArchivo }}
        />
      )}
      {firmando && drive && abierto && (
        <DialogoFirmar
          t={t}
          pdf={abierto.pdf}
          drive={drive}
          paginas={abierto.paginas}
          onCerrar={() => setFirmando(false)}
          onFirmado={(f) => {
            setFirmando(false);
            setHecho({
              texto: mensajeFirmado(t, f),
              ruta: `${carpeta}/${f.nombre}`,
              enlace: 'pdf.vault.openSigned',
            });
          }}
        />
      )}
      <DialogosDelEditor />
    </div>
  );
}

/**
 * Rellenar el formulario en el dispositivo: los mismos controles que el panel del
 * estudio (`CampoEditable`) y el mismo `rellenar`, que el motor local aplica.
 */
function CamposLocales({
  t,
  contenido,
  trabajando,
  onAplicar,
  onIrA,
}: {
  t: ReturnType<typeof docsT>;
  contenido: Contenido;
  trabajando: boolean;
  onAplicar: Aplicar;
  onIrA: (n: number) => void;
}) {
  const campos = (contenido.campos ?? []).filter((c) => c.tipo !== 'boton');
  const [valores, setValores] = useState<Record<string, string>>({});
  useEffect(() => {
    const inicial: Record<string, string> = {};
    for (const c of campos) inicial[c.nombre] = c.valor ?? '';
    setValores(inicial);
    // Solo al cambiar el documento: reescribir en cada tecleo perdería lo escrito.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contenido]);
  if (campos.length === 0) return <EmptyState compact title={t('pdf.fields.empty')} />;
  const puede = contenido.permisos.RellenarCampos || !contenido.permisos.SoloConClaveDeDueno;
  return (
    <form
      class="flex flex-col gap-2"
      data-campos-boveda
      onSubmit={(e) => {
        e.preventDefault();
        const cambiados: Record<string, string> = {};
        for (const c of campos)
          if ((valores[c.nombre] ?? '') !== (c.valor ?? ''))
            cambiados[c.nombre] = valores[c.nombre] ?? '';
        if (Object.keys(cambiados).length === 0) return;
        void onAplicar({ op: 'rellenar', datos: cambiados }, t('pdf.saved.filled'));
      }}
    >
      {!puede && (
        <InlineHelp tone="warning" live="off">
          {t('pdf.blocked.fill')}
        </InlineHelp>
      )}
      {campos.map((c) => (
        <CampoEditable
          key={c.nombre}
          campo={c}
          valor={valores[c.nombre] ?? ''}
          onValor={(v) => setValores((prev) => ({ ...prev, [c.nombre]: v }))}
          onIrA={() => onIrA(c.pagina + 1)}
          t={t}
        />
      ))}
      <Button type="submit" size="sm" disabled={trabajando || !puede}>
        {t('pdf.fields.save')}
      </Button>
    </form>
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
    />
  ) : (
    <PdfApp {...app} />
  );
}
