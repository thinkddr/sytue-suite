/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Obligaciones — el calendario fiscal, contable, laboral y mercantil de la
 * organización, mes a mes, para quien no sabe de impuestos. Cada fila dice SU
 * fecha («tu fecha»: días hábiles antes del último día que da la administración,
 * para que haya margen si falta algo), qué es en una frase, qué le falta en el
 * ERP para presentarla (movimientos del banco, facturas, nóminas) y cómo se
 * hace, paso a paso. Para lo que el ERP puede calcular solo (el IVA del
 * trimestre, el pago fraccionado), «Preparar» saca la cifra del libro.
 *
 * La presentación en sí se hace en el portal de la administración —enlazado en
 * cada fila—: la firma con certificado y los servicios web de cada agencia son
 * un conector por país que no existe todavía, y fingir que se presenta desde
 * aquí sería peor que decirlo.
 *
 * Lo que queda de cada presentación (el justificante, las cuentas depositadas) se
 * sube en la propia fila, con lo que tiene que traer para que valga.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  Illustration,
  InlineHelp,
  Spinner,
  cx,
  withBase,
  useAhora,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';
import { api, baseDeLaApp, consulta, mensajeDeError } from '../lib/api.js';
import type { useT } from '../i18n/index.js';
import { formatearDinero } from './formato.js';
import { nombreDePais } from './Onboarding.js';
import { ProcesoObligacion } from './ProcesoObligacion.js';
import { DocumentosRegistro } from './Seguimiento.js';
import type {
  CalculoObligacion,
  Obligacion,
  Organizacion,
  Preparacion,
  RespuestaObligaciones,
  Sancion,
} from './tipos.js';

type Traducir = ReturnType<typeof useT>;

interface Props {
  locale: Locale;
  organizacion: Organizacion;
  onEditarPerfil: () => void;
  /** Abre el banco: es donde se resuelve lo que falta en casi todas. */
  onBanco: () => void;
  /** Con solo lectura en Obligaciones se ven los documentos, pero no se suben. */
  soloLectura: boolean;
  puedeLeer: (modulo: string) => boolean;
  puedeAccion: (modulo: string, verbo: string) => boolean;
  /** Subir un documento puede cerrar una tarea (el contador de la barra lateral). */
  onTareas: () => void;
  t: Traducir;
}

/** Días entre hoy y una fecha ISO, contando en UTC y por días enteros. */
export function diasHasta(iso: string, hoy = new Date()): number {
  const d = new Date(iso);
  const objetivo = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate());
  const ahora = Date.UTC(hoy.getFullYear(), hoy.getMonth(), hoy.getDate());
  return Math.round((objetivo - ahora) / 86_400_000);
}

/** Rótulo legible del periodo: «1T 2026», «marzo de 2026», «2026», «Pago 2, 2026». */
export function rotuloDePeriodo(periodo: string, locale: Locale, t: Traducir): string {
  const [anio = '', resto = ''] = periodo.split('-');
  if (/^T[1-4]$/.test(resto))
    return t('obligations.period.quarter', { n: resto[1] ?? '', year: anio });
  if (/^\d{2}$/.test(resto)) {
    return formatDate(locale, new Date(Date.UTC(Number(anio), Number(resto) - 1, 1)), {
      month: 'long',
      year: 'numeric',
      timeZone: 'UTC',
    });
  }
  if (resto === 'A') return t('obligations.period.year', { year: anio });
  const pago = /^(\d)P$/.exec(resto);
  if (pago) return t('obligations.period.instalment', { n: pago[1] ?? '', year: anio });
  return periodo;
}

/**
 * La sanción en una frase, compuesta de sus cifras en el idioma de quien mira.
 * `nota` (la letra pequeña, en el idioma del país) va aparte y plegada.
 */
export function describirSancion(
  s: Sancion | undefined,
  moneda: string,
  locale: string,
  t: Traducir,
): string {
  if (!s) return '';
  const divisa = s.moneda ?? moneda;
  const partes: string[] = [];
  if (s.tipo === 'fija' && s.fija_centimos) {
    partes.push(
      t('obligations.penaltyFixed', { amount: formatearDinero(s.fija_centimos, locale, divisa) }),
    );
  }
  if (s.tipo === 'recargo') {
    if (s.recargo_base_pct)
      partes.push(t('obligations.penaltyBase', { pct: String(s.recargo_base_pct) }));
    if (s.recargo_pct_mes)
      partes.push(t('obligations.penaltyPerMonth', { pct: String(s.recargo_pct_mes) }));
    if (s.recargo_max_pct)
      partes.push(t('obligations.penaltyMax', { pct: String(s.recargo_max_pct) }));
    if (s.fija_centimos) {
      partes.push(
        t('obligations.penaltyMin', { amount: formatearDinero(s.fija_centimos, locale, divisa) }),
      );
    }
    if (s.con_requerimiento_pct && s.con_requerimiento_pct.length === 2) {
      partes.push(
        t('obligations.penaltyDemand', {
          min: String(s.con_requerimiento_pct[0]),
          max: String(s.con_requerimiento_pct[1]),
        }),
      );
    }
  }
  return partes.join(' · ');
}

/** Lo que costaría UN mes de retraso sobre una cuota, con la regla general de la sanción. */
export function estimarUnMes(s: Sancion | undefined, importe: number): number | null {
  if (!s || importe <= 0) return null;
  if (s.tipo === 'fija') return s.fija_centimos ?? null;
  if (s.tipo !== 'recargo') return null;
  let pct = (s.recargo_base_pct ?? 0) + (s.recargo_pct_mes ?? 0);
  if (s.recargo_max_pct && pct > s.recargo_max_pct) pct = s.recargo_max_pct;
  return Math.max(Math.round((importe * pct) / 100), s.fija_centimos ?? 0);
}

/** Lo que le falta a una obligación en el ERP, en frases; vacío = lista. */
export function faltas(o: Obligacion, t: Traducir): { clave: Preparacion; texto: string }[] {
  const frases: Record<Preparacion, (n: number) => string> = {
    banco: (count) => t('obligations.missingBank', { count }),
    facturas: (count) => t('obligations.missingInvoices', { count }),
    nominas: (count) => t('obligations.missingPayslips', { count }),
  };
  return (Object.entries(o.pendientes ?? {}) as [Preparacion, number][])
    .filter(([, n]) => n > 0)
    .map(([clave, n]) => ({ clave, texto: frases[clave](n) }));
}

export function Obligaciones({
  locale,
  organizacion,
  onEditarPerfil,
  onBanco,
  soloLectura,
  puedeLeer,
  puedeAccion,
  onTareas,
  t,
}: Props): JSX.Element {
  const [proceso, setProceso] = useState<Obligacion | null>(null);
  const [revision, setRevision] = useState(0);
  const ahora = useAhora();
  const [anio, setAnio] = useState(() => new Date().getFullYear());
  const [datos, setDatos] = useState<RespuestaObligaciones | null>(null);
  const [cargando, setCargando] = useState(true);
  const [error, setError] = useState('');
  const [ambito, setAmbito] = useState('');
  const [calculos, setCalculos] = useState<
    Record<string, CalculoObligacion | 'cargando' | 'error'>
  >({});

  useEffect(() => {
    let vivo = true;
    setCargando(true);
    api<RespuestaObligaciones>(`obligaciones${consulta({ anio })}`)
      .then((r) => {
        if (!vivo) return;
        setDatos(r);
        setError('');
      })
      .catch((err) => vivo && setError(mensajeDeError(err, t)))
      .finally(() => vivo && setCargando(false));
    return () => {
      vivo = false;
    };
  }, [anio, t, revision]);

  // Por «tu fecha», que es la que se enseña y la que manda para quien presenta.
  const lista = useMemo(
    () =>
      (datos?.obligaciones ?? [])
        .filter((o) => !ambito || o.ambito === ambito)
        .sort((a, b) => a.recomendada.localeCompare(b.recomendada)),
    [datos, ambito],
  );
  const provisional = lista.some((o) => o.calendario_provisional);
  const ambitos = useMemo(() => {
    const vistos = new Map<string, string>();
    for (const o of datos?.obligaciones ?? []) vistos.set(o.ambito, o.ambito_rotulo);
    return [...vistos.entries()];
  }, [datos]);

  // Agrupado por el mes de «tu fecha».
  const agruparPorMes = useCallback(
    (filas: Obligacion[]) => {
      const grupos: { clave: string; titulo: string; filas: Obligacion[] }[] = [];
      for (const o of filas) {
        const d = new Date(o.recomendada);
        const clave = `${d.getUTCFullYear()}-${d.getUTCMonth()}`;
        const ultimo = grupos[grupos.length - 1];
        if (ultimo && ultimo.clave === clave) {
          ultimo.filas.push(o);
          continue;
        }
        grupos.push({
          clave,
          titulo: formatDate(locale, d, { month: 'long', year: 'numeric', timeZone: 'UTC' }),
          filas: [o],
        });
      }
      return grupos;
    },
    [locale],
  );
  // En el año en curso, lo que ya venció va al final y plegado: en septiembre,
  // abrir la vista y encontrar ocho meses de «vencida hace 195 días» antes de
  // la primera fecha que importa era leer el calendario al revés. Un año
  // pasado se enseña entero y en orden: ahí todo venció.
  const separar = anio >= ahora.getFullYear();
  const mesesProximos = useMemo(
    () =>
      agruparPorMes(
        lista.filter((o) => o.proceso?.fase !== 4 && (!separar || diasHasta(o.limite) >= 0)),
      ),
    [lista, separar, agruparPorMes],
  );
  const mesesVencidos = useMemo(
    () =>
      separar
        ? agruparPorMes(lista.filter((o) => o.proceso?.fase !== 4 && diasHasta(o.limite) < 0))
        : [],
    [lista, separar, agruparPorMes],
  );
  const nVencidas = mesesVencidos.reduce((n, m) => n + m.filas.length, 0);
  const mesesArchivados = useMemo(
    () => agruparPorMes(lista.filter((o) => o.proceso?.fase === 4)),
    [lista, agruparPorMes],
  );

  async function preparar(o: Obligacion): Promise<void> {
    const clave = `${o.clave}|${o.periodo}`;
    setCalculos((prev) => ({ ...prev, [clave]: 'cargando' }));
    try {
      const c = await api<CalculoObligacion>(
        `obligaciones/calculo${consulta({ clave: o.clave, periodo: o.periodo })}`,
      );
      setCalculos((prev) => ({ ...prev, [clave]: c }));
    } catch {
      setCalculos((prev) => ({ ...prev, [clave]: 'error' }));
    }
  }

  const moneda = datos?.moneda ?? 'EUR';
  const fecha = (iso: string) =>
    formatDate(locale, new Date(iso), { day: 'numeric', month: 'short', timeZone: 'UTC' });

  const pintarMeses = (grupos: typeof mesesProximos, prefijo = 'mes'): JSX.Element[] =>
    grupos.map((mes) => (
      <section key={mes.clave} aria-labelledby={`${prefijo}-${mes.clave}`} class="space-y-2">
        <h2 id={`${prefijo}-${mes.clave}`} class="sytue-overline text-text-subtle">
          {mes.titulo}
        </h2>
        <ul class="border-border divide-border rounded-card divide-y border">
          {mes.filas.map((o) => {
            const archivada = o.proceso?.fase === 4;
            const dias = diasHasta(o.limite);
            const diasTu = diasHasta(o.recomendada);
            const claveCalc = `${o.clave}|${o.periodo}`;
            const calc = calculos[claveCalc];
            const sancion = describirSancion(o.sancion, moneda, locale, t);
            const falta = faltas(o, t);
            return (
              <li
                key={claveCalc}
                class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:gap-4"
              >
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="text-text text-sm font-semibold">{o.nombre}</span>
                    <span class="text-text-muted text-sm">
                      · {rotuloDePeriodo(o.periodo, locale, t)}
                    </span>
                    <Badge tone="neutral">{o.ambito_rotulo}</Badge>
                    {archivada ? <Badge tone="success">{t('process.archived')}</Badge> : null}
                  </div>
                  <p class="text-text-muted mt-0.5 text-xs">{o.guia?.que ?? o.concepto_rotulo}</p>
                  <ol
                    class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs"
                    aria-label={t('obligations.steps')}
                  >
                    {(
                      [
                        'process.prepare',
                        'onboarding.step.review',
                        'process.file',
                        'process.receipt',
                      ] as const
                    ).map((titulo, i) => {
                      const fase = o.proceso?.fase ?? 0;
                      return (
                        <li
                          key={titulo}
                          aria-current={fase === i ? 'step' : undefined}
                          class={cx(
                            'flex items-center gap-1',
                            fase > i
                              ? 'text-success-text'
                              : fase === i
                                ? 'text-text font-semibold'
                                : 'text-text-subtle',
                          )}
                        >
                          {fase > i ? (
                            <Icon name="check" size={14} />
                          ) : (
                            <span aria-hidden="true">{i + 1}.</span>
                          )}
                          {t(titulo)}
                        </li>
                      );
                    })}
                  </ol>
                  {o.pendientes && dias >= 0 && !archivada ? (
                    falta.length === 0 ? (
                      <p class="text-success-text mt-1 flex items-center gap-1 text-xs">
                        <Icon name="check" size={12} />
                        {t('obligations.ready')}
                      </p>
                    ) : (
                      <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                        {falta.map((f) => (
                          <span key={f.clave} class="text-warning-text flex items-center gap-1">
                            <Icon name="warning" size={12} />
                            {f.texto}
                          </span>
                        ))}
                        {falta.some((f) => f.clave === 'banco') ? (
                          <button
                            type="button"
                            onClick={onBanco}
                            class="sytue-focus-ring text-text hover:text-text-muted rounded-sm font-medium underline"
                          >
                            {t('obligations.goToBank')}
                          </button>
                        ) : null}
                      </div>
                    )
                  ) : null}
                  <Button class="mt-2" variant="secondary" size="sm" onClick={() => setProceso(o)}>
                    {t(
                      archivada
                        ? 'process.viewReceipt'
                        : (o.proceso?.fase ?? 0) > 0
                          ? 'process.resume'
                          : 'process.start',
                    )}
                  </Button>
                  {o.guia ? (
                    <details class="mt-1">
                      <summary class="sytue-focus-ring text-text-muted hover:text-text cursor-pointer rounded-sm text-xs font-medium">
                        {t('obligations.howTo')}
                      </summary>
                      <div class="text-text-muted mt-2 max-w-prose space-y-2 text-sm">
                        {o.guia.necesitas.length > 0 ? (
                          <div>
                            <p class="text-text text-xs font-semibold">{t('obligations.need')}</p>
                            <ul class="mt-1 list-disc space-y-0.5 pl-5">
                              {o.guia.necesitas.map((n) => (
                                <li key={n}>{n}</li>
                              ))}
                            </ul>
                          </div>
                        ) : null}
                        <div>
                          <p class="text-text text-xs font-semibold">{t('obligations.steps')}</p>
                          <ol class="mt-1 list-decimal space-y-0.5 pl-5">
                            {o.guia.pasos.map((p) => (
                              <li key={p}>{p}</li>
                            ))}
                          </ol>
                        </div>
                        <div>
                          <p class="text-text text-xs font-semibold">{t('obligations.ifLate')}</p>
                          <p class="mt-1">{o.guia.si_no}</p>
                        </div>
                      </div>
                    </details>
                  ) : null}
                  <DocumentosObligacion
                    obligacion={o}
                    soloLectura={soloLectura}
                    locale={locale}
                    t={t}
                    onCambio={() => {
                      setRevision((r) => r + 1);
                      onTareas();
                    }}
                  />
                  {sancion ? (
                    <p class="text-text-subtle mt-1 text-xs">
                      <span class="font-medium">{t('obligations.penalty')}:</span> {sancion}
                      {o.sancion?.nota ? (
                        <details class="mt-0.5 inline-block">
                          <summary class="sytue-focus-ring cursor-pointer rounded-sm underline decoration-dotted">
                            {t('obligations.penaltyNote')}
                          </summary>
                          <span class="text-text-subtle block max-w-prose">{o.sancion.nota}</span>
                        </details>
                      ) : null}
                    </p>
                  ) : null}
                  {calc && calc !== 'cargando' && calc !== 'error' ? (
                    <div class="bg-bg-subtle mt-2 rounded-md px-3 py-2 text-xs">
                      <p class="text-text-muted">
                        {t('obligations.prepared', { from: calc.desde, to: calc.hasta })}
                      </p>
                      {calc.asentado ? (
                        <>
                          <p class="text-text mt-0.5 text-sm font-semibold tabular-nums">
                            {t('obligations.result')}:{' '}
                            {formatearDinero(calc.resultado_centimos, locale, moneda)}
                          </p>
                          {estimarUnMes(o.sancion, calc.resultado_centimos) !== null ? (
                            <p class="text-text-subtle mt-0.5">
                              {t('obligations.penaltyEstimate', {
                                amount: formatearDinero(
                                  estimarUnMes(o.sancion, calc.resultado_centimos) ?? 0,
                                  locale,
                                  o.sancion?.moneda ?? moneda,
                                ),
                              })}
                            </p>
                          ) : null}
                        </>
                      ) : (
                        <p class="text-text-muted mt-0.5">{t('obligations.notPosted')}</p>
                      )}
                    </div>
                  ) : null}
                </div>
                <div class="flex shrink-0 flex-col items-start gap-1 text-xs sm:items-end">
                  {!archivada ? (
                    <span class="text-text flex items-center gap-1.5 text-sm font-semibold">
                      {t('obligations.yourDate')} {fecha(o.recomendada)}
                      <Badge
                        tone={
                          dias < 0 || diasTu < 0 ? 'danger' : diasTu <= 7 ? 'warning' : 'neutral'
                        }
                      >
                        {dias < 0
                          ? t('obligations.overdue', { count: -dias })
                          : diasTu === 0
                            ? t('obligations.today')
                            : diasTu < 0
                              ? dias === 0
                                ? t('obligations.lastDay')
                                : t('obligations.lastDays', { count: dias })
                              : t('obligations.daysLeft', { count: diasTu })}
                      </Badge>
                    </span>
                  ) : null}
                  {o.limite_domiciliacion && !archivada ? (
                    <span class="text-text-subtle">
                      {t('obligations.directDebit', { date: fecha(o.limite_domiciliacion) })}
                    </span>
                  ) : null}
                  <span class="text-text-subtle">
                    {t('obligations.legalDeadline', { date: fecha(o.limite) })}
                  </span>
                  <span class="mt-1 flex flex-wrap gap-1">
                    {o.calculable ? (
                      <Button
                        size="xs"
                        variant="secondary"
                        loading={calc === 'cargando'}
                        onClick={() => void preparar(o)}
                      >
                        {t('obligations.prepare')}
                      </Button>
                    ) : null}
                    <a
                      href={o.portal.url}
                      target="_blank"
                      rel="noopener"
                      class="text-text-muted hover:text-text sytue-focus-ring rounded-control inline-flex h-6 items-center gap-1 px-2 text-xs font-medium"
                    >
                      {t('obligations.portal')}
                      <Icon name="arrowUpRight" size={12} />
                    </a>
                  </span>
                </div>
              </li>
            );
          })}
        </ul>
      </section>
    ));

  return (
    <div class="mx-auto max-w-5xl space-y-6 px-6 py-6">
      {proceso ? (
        <ProcesoObligacion
          obligacion={proceso}
          organizacion={organizacion}
          moneda={moneda}
          locale={locale}
          t={t}
          soloLectura={soloLectura}
          puedeLeer={puedeLeer}
          puedeAccion={puedeAccion}
          onCerrar={() => setProceso(null)}
          onCambio={() => {
            setRevision((r) => r + 1);
            onTareas();
          }}
        />
      ) : null}
      <header class="border-border flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h1 class="text-text text-lg font-semibold tracking-tight">{t('obligations.title')}</h1>
            {datos ? (
              <Badge tone={datos.verificado ? 'success' : 'neutral'}>
                {datos.verificado ? t('obligations.verified') : t('obligations.indicative')}
              </Badge>
            ) : null}
          </div>
          <p class="text-text-muted mt-1 text-sm">
            {t('obligations.subtitle', { country: nombreDePais(organizacion.pais, locale) })}
          </p>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <div
            class="border-border inline-flex items-center rounded-md border"
            role="group"
            aria-label={t('obligations.year')}
          >
            <button
              type="button"
              class="sytue-focus-ring text-text-muted hover:text-text px-2 py-1"
              aria-label={String(anio - 1)}
              onClick={() => setAnio((a) => a - 1)}
            >
              <Icon name="chevronLeft" size={14} />
            </button>
            <span class="text-text px-1 text-sm font-medium tabular-nums">{anio}</span>
            <button
              type="button"
              class="sytue-focus-ring text-text-muted hover:text-text px-2 py-1"
              aria-label={String(anio + 1)}
              onClick={() => setAnio((a) => a + 1)}
            >
              <Icon name="chevronRight" size={14} />
            </button>
          </div>
          <Button
            variant="secondary"
            size="sm"
            title={t('obligations.icsHint')}
            href={withBase(baseDeLaApp(), `/api/obligaciones/ics${consulta({ anio })}`)}
          >
            <Icon name="calendar" size={14} class="mr-1" />
            {t('obligations.ics')}
          </Button>
          <Button variant="ghost" size="sm" onClick={onEditarPerfil}>
            {t('obligations.editProfile')}
          </Button>
        </div>
      </header>

      {datos?.territorio ? (
        <InlineHelp tone="warning" title={datos.territorio.nombre}>
          {datos.territorio.nota}
        </InlineHelp>
      ) : null}
      <p class="text-text-subtle text-xs">
        {t('obligations.yourDateHint')} {provisional ? t('obligations.provisional') : ''}
      </p>

      {ambitos.length > 1 ? (
        <div
          class="flex flex-wrap items-center gap-1.5"
          role="group"
          aria-label={t('list.filterByStatus')}
        >
          {[['', t('obligations.scope.all')] as [string, string], ...ambitos].map(
            ([clave, rotulo]) => (
              <button
                key={clave}
                type="button"
                aria-pressed={ambito === clave}
                onClick={() => setAmbito(clave)}
                class={cx(
                  'sytue-focus-ring rounded-control px-2.5 py-1 text-xs transition-colors',
                  ambito === clave
                    ? 'bg-selected text-text font-semibold'
                    : 'text-text-muted hover:text-text',
                )}
              >
                {rotulo}
              </button>
            ),
          )}
        </div>
      ) : null}

      {error ? (
        <p class="bg-danger-bg text-danger-text rounded-md px-4 py-3 text-sm">{error}</p>
      ) : null}

      {cargando && !datos ? (
        <div class="flex justify-center py-12">
          <Spinner size={24} />
        </div>
      ) : mesesProximos.length + mesesVencidos.length + mesesArchivados.length === 0 ? (
        <EmptyState
          title={t('obligations.empty', { year: String(anio) })}
          illustration={<Illustration name="emptyCalendar" size={96} />}
        />
      ) : (
        <>
          {pintarMeses(mesesProximos)}
          {mesesVencidos.length > 0 ? (
            <details class="border-border rounded-card border">
              <summary class="sytue-focus-ring text-text-muted hover:text-text rounded-card cursor-pointer px-4 py-3 text-sm font-medium">
                {t('obligations.overdueSection', { count: nVencidas })}
              </summary>
              <div class="space-y-6 px-4 pb-4">{pintarMeses(mesesVencidos, 'vencidos')}</div>
            </details>
          ) : null}
          {mesesArchivados.length > 0 ? (
            <details class="border-border rounded-card border">
              <summary class="sytue-focus-ring text-text-muted hover:text-text rounded-card cursor-pointer px-4 py-3 text-sm font-medium">
                {t('process.archivedList')}
              </summary>
              <div class="space-y-6 px-4 pb-4">{pintarMeses(mesesArchivados, 'archivados')}</div>
            </details>
          ) : null}
        </>
      )}
    </div>
  );
}

/**
 * Los documentos de una presentación. Se piden al desplegar: con cuarenta filas en
 * el año, pedirlos todos al abrir la vista serían cuarenta consultas para nada.
 */
function DocumentosObligacion({
  obligacion: o,
  soloLectura,
  locale,
  t,
  onCambio,
}: {
  obligacion: Obligacion;
  soloLectura: boolean;
  locale: Locale;
  t: Traducir;
  onCambio: () => void;
}): JSX.Element | null {
  const [abierto, setAbierto] = useState(false);
  if (!o.documentos?.length) return null;
  return (
    <details class="mt-1" onToggle={(e) => setAbierto(e.currentTarget.open)}>
      <summary class="sytue-focus-ring text-text-muted hover:text-text cursor-pointer rounded-sm text-xs font-medium">
        {t('obligations.documents')}
      </summary>
      {abierto ? (
        <div class="mt-2 max-w-prose space-y-4">
          {o.documentos.map((d) => (
            <DocumentosRegistro
              key={d.clave}
              modelo="obligacion"
              registro={`${o.clave}:${o.periodo}:${d.clave}`}
              titulo={d.nombre}
              requisitos={d.requisitos}
              soloLectura={soloLectura}
              locale={locale}
              t={t}
              onCambio={onCambio}
            />
          ))}
        </div>
      ) : null}
    </details>
  );
}
