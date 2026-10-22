/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue AI en el ERP: crear registros pidiéndolos en una frase («apunta un gasto
 * de Claude Code de 180 €»), pedir un tablero o un Excel, ver los tableros
 * guardados y rellenar una ficha pegando un texto.
 *
 * La IA PROPONE y la persona decide: nada se guarda ni se descarga sin pulsar, y
 * lo que se enseña antes de guardar son los datos de verdad, no una maqueta.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  BarChart,
  Button,
  Card,
  cx,
  Icon,
  InlineHelp,
  Modal,
  Spinner,
  Table,
  Textarea,
  useConfirm,
  useToast,
} from '@sytue/ui';
import type { useT } from '../i18n/index.js';
import { api, ErrorApi, mensajeDeError } from '../lib/api.js';
import { celda, formatearDinero, formatearFecha } from './formato.js';
import type {
  Actividad,
  ColumnaInforme,
  Informe,
  Modelo,
  ParteInforme,
  PasoPlan,
  PlanIA,
  Registro,
  TablaInforme,
  Tablero,
} from './tipos.js';

type Traducir = ReturnType<typeof useT>;
type Celda = string | number | boolean | null | undefined;

/** Qué decir cuando Sytue AI no ha podido: el límite, no entenderlo o no responder. */
function mensajeIA(err: unknown, t: Traducir): string {
  const estado = err instanceof ErrorApi ? err.estado : 0;
  return t(estado === 429 ? 'ai.limit' : estado === 422 ? 'ai.notUnderstood' : 'ai.failed');
}

function valor(col: ColumnaInforme | undefined, v: Celda, locale: string): string {
  if (v === null || v === undefined) return '';
  switch (col?.tipo) {
    case 'dinero':
      return formatearDinero(Math.round(Number(v) * 100), locale);
    case 'numero':
      return new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(Number(v));
    case 'fecha':
      return formatearFecha(String(v), locale);
    case 'booleano':
      return v ? '✓' : '—';
  }
  // Un mes agrupado («2026-09») se lee como mes, no como código.
  if (typeof v === 'string' && /^\d{4}-\d{2}$/.test(v)) {
    const d = new Date(`${v}-01T00:00:00`);
    if (!Number.isNaN(d.getTime())) {
      return new Intl.DateTimeFormat(locale, { month: 'short', year: 'numeric' }).format(d);
    }
  }
  return String(v);
}

function Parte({
  parte,
  tabla,
  locale,
  t,
}: {
  parte: ParteInforme;
  tabla: TablaInforme | undefined;
  locale: string;
  t: Traducir;
}): JSX.Element {
  if (!tabla) return <Spinner size={16} />;
  if (tabla.error) {
    return (
      <InlineHelp tone="warning">
        {t(tabla.error === 'sin_permiso' ? 'ai.noPermission' : 'ai.unavailable')}
      </InlineHelp>
    );
  }
  const cols = tabla.columnas ?? [];
  const filas = tabla.filas ?? [];
  let cuerpo: JSX.Element;
  if (parte.vista === 'cifra') {
    cuerpo = (
      <p class="text-text text-3xl font-semibold tabular-nums">
        {valor(cols[0], filas[0]?.[0], locale)}
      </p>
    );
  } else if (parte.vista === 'barras') {
    cuerpo = (
      <BarChart
        points={filas.map((f) => ({
          label: valor(cols[0], f[0], locale),
          value: Number(f[1]) || 0,
        }))}
        seriesLabel={parte.titulo}
        emptyLabel={t('ai.empty')}
        tableHeaders={[cols[0]?.etiqueta ?? '', cols[1]?.etiqueta ?? '']}
      />
    );
  } else {
    cuerpo = (
      <Table
        columns={cols.map((c, i) => ({
          key: `${c.clave}-${i}`,
          header: c.etiqueta,
          cell: (f: Celda[]) => valor(c, f[i], locale),
          align: c.tipo === 'dinero' || c.tipo === 'numero' ? ('end' as const) : ('start' as const),
        }))}
        rows={filas}
        rowKey={(_, i) => String(i)}
        emptyContent={t('ai.empty')}
      />
    );
  }
  return (
    <>
      {cuerpo}
      {tabla.recortada ? <p class="text-text-subtle mt-2 text-xs">{t('ai.truncated')}</p> : null}
      {/* Procedencia de una vista de Sytue Bases: solo nombres propios y la fecha, sin
          palabras que traducir. Quién la concedió va en el `title`. */}
      {tabla.procedencia ? (
        <p class="text-text-subtle mt-2 text-xs" title={tabla.procedencia.concedida_por}>
          {tabla.procedencia.origen} · {tabla.procedencia.base} › {tabla.procedencia.tabla} ›{' '}
          {tabla.procedencia.vista} · v{tabla.procedencia.version} ·{' '}
          {tabla.procedencia.leida_el.slice(0, 16).replace('T', ' ')}
        </p>
      ) : null}
    </>
  );
}

/** Las partes de un informe, en rejilla: las cifras y las barras de dos en dos. */
export function PartesInforme({
  informe,
  datos,
  locale,
  t,
}: {
  informe: Informe;
  datos: TablaInforme[] | null;
  locale: string;
  t: Traducir;
}): JSX.Element {
  return (
    <div class="grid gap-4 md:grid-cols-2">
      {informe.partes.map((p, i) => (
        <Card key={`${p.titulo}-${i}`} class={p.vista === 'tabla' ? 'p-4 md:col-span-2' : 'p-4'}>
          <h3 class="text-text-muted mb-3 text-sm font-medium">{p.titulo}</h3>
          <Parte parte={p} tabla={datos?.[i]} locale={locale} t={t} />
        </Card>
      ))}
    </div>
  );
}

/** Descarga el informe como Excel. El nombre del fichero es el título. */
async function descargarExcel(informe: Informe): Promise<void> {
  const resp = await fetch('/api/informes/excel', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ informe }),
  });
  if (!resp.ok) throw new ErrorApi(`HTTP ${resp.status}`, resp.status);
  const url = URL.createObjectURL(await resp.blob());
  const enlace = document.createElement('a');
  enlace.href = url;
  enlace.download = `${informe.titulo.replace(/[\\/:*?"<>|]+/g, '-').trim() || 'Sytue AI'}.xlsx`;
  enlace.click();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

async function datosDe(informe: Informe): Promise<TablaInforme[]> {
  const r = await api<{ datos: TablaInforme[] }>('informes/datos', {
    method: 'POST',
    body: JSON.stringify({ informe }),
  });
  return r.datos;
}

/** Un paso del plan: qué se crea (o qué ya existía), con sus datos y lo que falta. */
function PasoDelPlan({
  paso,
  modelos,
  pasos,
  locale,
  t,
}: {
  paso: PasoPlan;
  modelos: Modelo[];
  pasos: PasoPlan[];
  locale: string;
  t: Traducir;
}): JSX.Element {
  const modelo = modelos.find((m) => m.clave === paso.modelo);
  const campos = (modelo?.campos ?? []).filter(
    (c) =>
      paso.enlaces?.[c.clave] || (paso.data[c.clave] !== undefined && paso.data[c.clave] !== ''),
  );
  const nombreDePaso = (ref: string) => {
    const otro = pasos.find((p) => p.ref === ref);
    const titulo = otro?.existente?.etiqueta ?? String(otro?.data.nombre ?? otro?.etiqueta ?? ref);
    return titulo;
  };
  return (
    <li class="border-border space-y-2 rounded-md border p-3">
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-text text-sm font-semibold">{paso.etiqueta ?? modelo?.etiqueta}</span>
        {paso.existente ? (
          <Badge tone="neutral">{t('ai.actExisting', { name: paso.existente.etiqueta })}</Badge>
        ) : (
          <Badge tone="info">{t('ai.actNew')}</Badge>
        )}
      </div>
      {paso.existente ? null : (
        <dl class="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-2">
          {campos.map((c) => (
            <div key={c.clave} class="flex min-w-0 gap-1">
              <dt class="text-text-subtle shrink-0">{c.etiqueta}:</dt>
              <dd class="text-text min-w-0 truncate">
                {paso.enlaces?.[c.clave]
                  ? nombreDePaso(paso.enlaces[c.clave] ?? '')
                  : celda(c, paso.data[c.clave], locale, paso.etiquetas ?? {})}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {paso.faltan?.length ? (
        <InlineHelp tone="danger">
          {t('ai.actMissing', {
            fields: paso.faltan
              .map((f) => modelo?.campos.find((c) => c.clave === f)?.etiqueta ?? f)
              .join(', '),
          })}
        </InlineHelp>
      ) : null}
      {paso.avisos?.map((a) => (
        <p key={a} class="text-warning-text text-xs">
          {a}
        </p>
      ))}
      {paso.tarea && !paso.existente ? (
        <p class="text-text-muted flex items-center gap-1 text-xs">
          <Icon name="clock" size={12} />
          {t('ai.actTask', { task: paso.tarea })}
        </p>
      ) : null}
    </li>
  );
}

/**
 * Pedir en una frase que se cree algo. Sytue AI propone los registros (con lo que
 * se puede comprobar en internet ya comprobado), la persona los ve y los crea de
 * una vez: o entran todos, o ninguno.
 */
function CrearConIA({
  modelos,
  locale,
  t,
  onCreado,
}: {
  modelos: Modelo[];
  locale: string;
  t: Traducir;
  onCreado: (tareas: Actividad[]) => void;
}): JSX.Element {
  const aviso = useToast();
  const [peticion, setPeticion] = useState('');
  const [plan, setPlan] = useState<PlanIA | null>(null);
  const [ocupado, setOcupado] = useState<'' | 'proponer' | 'crear'>('');
  const [fallo, setFallo] = useState('');

  async function proponer(): Promise<void> {
    if (!peticion.trim() || ocupado) return;
    setOcupado('proponer');
    setFallo('');
    setPlan(null);
    try {
      setPlan(
        await api<PlanIA>('ia/acciones', { method: 'POST', body: JSON.stringify({ peticion }) }),
      );
    } catch (err) {
      setFallo(mensajeIA(err, t));
    } finally {
      setOcupado('');
    }
  }

  async function crear(): Promise<void> {
    if (!plan || ocupado) return;
    setOcupado('crear');
    setFallo('');
    try {
      const r = await api<{ creados: Registro[]; actividades: Actividad[] }>('ia/acciones/crear', {
        method: 'POST',
        body: JSON.stringify({ pasos: plan.pasos }),
      });
      aviso.notify({
        tone: 'success',
        title: t('ai.actCreated', { count: r.creados.length }),
        ...(r.actividades.length > 0
          ? { description: t('ai.actTaskCreated', { task: r.actividades[0]?.resumen ?? '' }) }
          : {}),
      });
      setPlan(null);
      setPeticion('');
      onCreado(r.actividades);
    } catch (err) {
      setFallo(mensajeDeError(err, t));
    } finally {
      setOcupado('');
    }
  }

  const incompleto = plan?.pasos.some((p) => !p.existente && (p.faltan?.length ?? 0) > 0) ?? false;
  return (
    <div class="space-y-4">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void proponer();
        }}
      >
        <Textarea
          value={peticion}
          rows={3}
          maxLength={2000}
          placeholder={t('ai.actPlaceholder')}
          aria-label={t('ai.actAsk')}
          onInput={(e) => setPeticion(e.currentTarget.value)}
        />
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <Button
            type="submit"
            variant="primary"
            loading={ocupado === 'proponer'}
            disabled={!peticion.trim() || !!ocupado}
          >
            {t('ai.actPrepare')}
          </Button>
          <p class="text-text-subtle text-xs">{t('ai.actHint')}</p>
        </div>
      </form>
      {fallo ? <InlineHelp tone="danger">{fallo}</InlineHelp> : null}
      {plan ? (
        <section class="space-y-3" aria-live="polite">
          <p class="text-text text-sm">{plan.resumen}</p>
          {plan.avisos?.length ? (
            <InlineHelp tone="warning">
              <ul class="list-disc space-y-0.5 pl-4">
                {plan.avisos.map((a) => (
                  <li key={a}>{a}</li>
                ))}
              </ul>
            </InlineHelp>
          ) : null}
          <ol class="space-y-2">
            {plan.pasos.map((p) => (
              <PasoDelPlan
                key={p.ref}
                paso={p}
                modelos={modelos}
                pasos={plan.pasos}
                locale={locale}
                t={t}
              />
            ))}
          </ol>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <Button variant="ghost" onClick={() => setPlan(null)} disabled={!!ocupado}>
              {t('action.close')}
            </Button>
            <Button
              variant="primary"
              loading={ocupado === 'crear'}
              disabled={incompleto || !!ocupado}
              onClick={() => void crear()}
            >
              {t('ai.actCreate')}
            </Button>
          </div>
        </section>
      ) : null}
    </div>
  );
}

/**
 * El panel de Sytue AI: crear registros pidiéndolos, o pedir un tablero o un
 * Excel. Lo que propone se enseña antes de guardar, crear o descargar.
 */
export function PanelIA({
  modelo,
  modelos,
  locale,
  t,
  onCerrar,
  onGuardado,
  onCreado,
}: {
  /** La lista abierta, si hay una: la petición se entiende sobre ella. */
  modelo: Modelo | null;
  /** Todos los modelos del catálogo, para pintar lo que se va a crear. */
  modelos: Modelo[];
  locale: string;
  t: Traducir;
  onCerrar: () => void;
  onGuardado: (tablero: Tablero) => void;
  onCreado: (tareas: Actividad[]) => void;
}): JSX.Element {
  const [pestana, setPestana] = useState<'crear' | 'informe'>('crear');
  return (
    <Modal open onClose={onCerrar} title="Sytue AI" size="lg">
      <div
        class="border-border mb-4 inline-flex rounded-md border"
        role="tablist"
        aria-label="Sytue AI"
      >
        {(
          [
            { valor: 'crear', rotulo: t('ai.tabCreate') },
            { valor: 'informe', rotulo: t('ai.tabReport') },
          ] as const
        ).map((p) => (
          <button
            key={p.valor}
            type="button"
            role="tab"
            aria-selected={pestana === p.valor}
            onClick={() => setPestana(p.valor)}
            class={cx(
              'sytue-focus-ring px-3 py-1.5 text-sm',
              pestana === p.valor
                ? 'bg-selected text-text font-semibold'
                : 'text-text-muted hover:text-text',
            )}
          >
            {p.rotulo}
          </button>
        ))}
      </div>
      {pestana === 'crear' ? (
        <CrearConIA modelos={modelos} locale={locale} t={t} onCreado={onCreado} />
      ) : (
        <InformeIA modelo={modelo} locale={locale} t={t} onGuardado={onGuardado} />
      )}
    </Modal>
  );
}

/** Pedir un tablero o un Excel en una frase. */
function InformeIA({
  modelo,
  locale,
  t,
  onGuardado,
}: {
  modelo: Modelo | null;
  locale: string;
  t: Traducir;
  onGuardado: (tablero: Tablero) => void;
}): JSX.Element {
  const aviso = useToast();
  const [peticion, setPeticion] = useState('');
  const [para, setPara] = useState<'tablero' | 'excel'>('tablero');
  const [informe, setInforme] = useState<Informe | null>(null);
  const [datos, setDatos] = useState<TablaInforme[] | null>(null);
  const [ocupado, setOcupado] = useState<'' | 'proponer' | 'guardar' | 'excel'>('');
  const [fallo, setFallo] = useState('');

  async function proponer(destino: 'tablero' | 'excel'): Promise<void> {
    if (!peticion.trim() || ocupado) return;
    setPara(destino);
    setOcupado('proponer');
    setFallo('');
    setInforme(null);
    setDatos(null);
    try {
      const r = await api<{ informe: Informe }>('ia/informe', {
        method: 'POST',
        body: JSON.stringify({ peticion, para: destino, modelo: modelo?.clave ?? '' }),
      });
      setInforme(r.informe);
      setDatos(await datosDe(r.informe));
    } catch (err) {
      setFallo(mensajeIA(err, t));
    } finally {
      setOcupado('');
    }
  }

  async function guardar(): Promise<void> {
    if (!informe || ocupado) return;
    setOcupado('guardar');
    try {
      const r = await api<{ tablero: Tablero }>('tableros', {
        method: 'POST',
        body: JSON.stringify({ informe }),
      });
      aviso.notify({ tone: 'success', title: t('dashboard.saved') });
      onGuardado(r.tablero);
    } catch {
      setFallo(t('error.generic'));
    } finally {
      setOcupado('');
    }
  }

  async function excel(): Promise<void> {
    if (!informe || ocupado) return;
    setOcupado('excel');
    try {
      await descargarExcel(informe);
    } catch {
      setFallo(t('error.generic'));
    } finally {
      setOcupado('');
    }
  }

  const listo = peticion.trim() !== '' && !ocupado;
  return (
    <>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void proponer('tablero');
        }}
      >
        <Textarea
          value={peticion}
          rows={3}
          maxLength={2000}
          placeholder={t('ai.placeholder')}
          aria-label={t('ai.ask')}
          onInput={(e) => setPeticion(e.currentTarget.value)}
        />
        {modelo ? (
          <p class="text-text-subtle mt-1 text-xs">{t('ai.about', { model: modelo.etiqueta })}</p>
        ) : null}
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <Button
            type="submit"
            variant="primary"
            loading={ocupado === 'proponer' && para === 'tablero'}
            disabled={!listo}
          >
            {t('ai.dashboard')}
          </Button>
          <Button
            type="button"
            variant="secondary"
            loading={ocupado === 'proponer' && para === 'excel'}
            disabled={!listo}
            onClick={() => void proponer('excel')}
          >
            {t('ai.excel')}
          </Button>
        </div>
        <p class="text-text-subtle mt-3 text-xs">{t('ai.privacy')}</p>
      </form>

      {fallo ? (
        <div class="mt-4">
          <InlineHelp tone="danger">{fallo}</InlineHelp>
        </div>
      ) : null}

      {informe ? (
        <section class="mt-6 space-y-4" aria-live="polite">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h3 class="text-text text-base font-semibold">{informe.titulo}</h3>
            <div class="flex flex-wrap gap-2">
              <Button
                variant={para === 'excel' ? 'primary' : 'secondary'}
                loading={ocupado === 'excel'}
                disabled={!!ocupado && ocupado !== 'excel'}
                onClick={() => void excel()}
              >
                {t('ai.download')}
              </Button>
              <Button
                variant={para === 'excel' ? 'secondary' : 'primary'}
                loading={ocupado === 'guardar'}
                disabled={!!ocupado && ocupado !== 'guardar'}
                onClick={() => void guardar()}
              >
                {t('ai.save')}
              </Button>
            </div>
          </div>
          <PartesInforme informe={informe} datos={datos} locale={locale} t={t} />
        </section>
      ) : null}
    </>
  );
}

/** Un tablero guardado: se abre con los datos de ahora, no con los de cuando se guardó. */
export function VistaTablero({
  tablero,
  locale,
  t,
  onBorrado,
}: {
  tablero: Tablero;
  locale: string;
  t: Traducir;
  onBorrado: (id: string) => void;
}): JSX.Element {
  const aviso = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [datos, setDatos] = useState<TablaInforme[] | null>(null);

  useEffect(() => {
    let vivo = true;
    setDatos(null);
    datosDe(tablero.informe)
      .then((d) => vivo && setDatos(d))
      .catch(
        () =>
          vivo &&
          setDatos(
            tablero.informe.partes.map(() => ({
              columnas: null,
              filas: null,
              error: 'no_disponible',
            })),
          ),
      );
    return () => {
      vivo = false;
    };
  }, [tablero]);

  async function borrar(): Promise<void> {
    const seguro = await confirmar({
      titulo: t('dashboard.delete'),
      mensaje: t('dashboard.deleteConfirm', { nombre: tablero.titulo }),
      confirmLabel: t('dashboard.delete'),
      tone: 'danger',
    });
    if (!seguro) return;
    try {
      await api(`tableros/${tablero.id}`, { method: 'DELETE' });
      aviso.notify({ tone: 'success', title: t('dashboard.deleted') });
      onBorrado(tablero.id);
    } catch {
      aviso.notify({ tone: 'danger', title: t('error.generic') });
    }
  }

  return (
    <div class="mx-auto max-w-6xl space-y-5 px-6 py-6">
      <header class="flex flex-wrap items-center justify-between gap-3">
        <h1 class="text-text flex items-center gap-2 text-2xl font-semibold">
          <Icon name="chart" size={20} />
          {tablero.titulo}
        </h1>
        <div class="flex flex-wrap gap-2">
          <Button
            variant="secondary"
            onClick={() =>
              void descargarExcel(tablero.informe).catch(() =>
                aviso.notify({ tone: 'danger', title: t('error.generic') }),
              )
            }
          >
            {t('ai.download')}
          </Button>
          <Button variant="ghost" onClick={() => void borrar()}>
            {t('dashboard.delete')}
          </Button>
        </div>
      </header>
      <PartesInforme informe={tablero.informe} datos={datos} locale={locale} t={t} />
      {dialogo}
    </div>
  );
}

/** Pegar un texto (un correo, un pedido) y que Sytue AI rellene la ficha. */
export function RellenarIA({
  modelo,
  t,
  onCerrar,
  onRellenado,
}: {
  modelo: Modelo;
  t: Traducir;
  onCerrar: () => void;
  onRellenado: (data: Registro['data'], etiquetas: Record<string, string>) => void;
}): JSX.Element {
  const [texto, setTexto] = useState('');
  const [ocupado, setOcupado] = useState(false);
  const [fallo, setFallo] = useState('');

  // Con el PDF de la factura, el servicio lo lee con Sytue PDF y sigue igual que
  // con un texto pegado.
  async function rellenar(pdf?: File): Promise<void> {
    if ((!pdf && !texto.trim()) || ocupado) return;
    setOcupado(true);
    setFallo('');
    try {
      const r = await api<{ data: Registro['data']; etiquetas: Record<string, string> }>(
        `${modelo.clave}/ia/rellenar`,
        pdf
          ? {
              method: 'POST',
              headers: { 'content-type': 'application/pdf' },
              body: await pdf.arrayBuffer(),
            }
          : { method: 'POST', body: JSON.stringify({ texto }) },
      );
      onRellenado(r.data ?? {}, r.etiquetas ?? {});
    } catch (err) {
      setFallo(
        pdf && err instanceof ErrorApi && err.estado === 422
          ? t('ai.pdfUnreadable')
          : mensajeIA(err, t),
      );
      setOcupado(false);
    }
  }

  return (
    <Modal
      open
      onClose={onCerrar}
      title={t('ai.fill')}
      footer={
        <>
          <Button variant="ghost" onClick={onCerrar}>
            {t('action.close')}
          </Button>
          <Button
            variant="primary"
            loading={ocupado}
            disabled={!texto.trim()}
            onClick={() => void rellenar()}
          >
            {t('ai.fillAction')}
          </Button>
        </>
      }
    >
      <p class="text-text-muted mb-3 text-sm">{t('ai.fillHint', { model: modelo.etiqueta })}</p>
      <Textarea
        value={texto}
        rows={8}
        maxLength={20000}
        aria-label={t('ai.fill')}
        onInput={(e) => setTexto(e.currentTarget.value)}
      />
      <label class="text-text-muted mt-3 flex flex-wrap items-center gap-2 text-sm">
        <Icon name="upload" size={14} />
        {t('ai.fillFromPdf')}
        <input
          type="file"
          accept="application/pdf"
          disabled={ocupado}
          class="text-xs"
          onChange={(e) => {
            const fichero = e.currentTarget.files?.[0];
            if (fichero) void rellenar(fichero);
          }}
        />
      </label>
      {fallo ? (
        <div class="mt-3">
          <InlineHelp tone="danger">{fallo}</InlineHelp>
        </div>
      ) : null}
    </Modal>
  );
}
