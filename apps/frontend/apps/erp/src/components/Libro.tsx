/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las dos pantallas de contabilidad: el libro diario y el sumas y saldos.
 *
 * No salen del catálogo como el resto, y no es una excepción caprichosa: un
 * asiento no es un registro con campos declarados, es una estructura con reglas
 * propias —se asienta, no se edita— y sus tablas viven aparte precisamente por
 * eso. Aquí solo se LEEN: corregir contabilidad se hace con otro asiento.
 *
 * Las dos se acotan por PERÍODO (periodo.ts) desde un selector junto al título: el
 * libro de un mes y el sumas y saldos de un trimestre son lo que se mira de verdad.
 *
 * El libro diario lleva además el CIERRE DEL EJERCICIO: hasta qué fecha está
 * cerrado, y para quien administra la contabilidad, cerrar y reabrir.
 */
import { useCallback, useEffect, useId, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import {
  Badge,
  Button,
  ConfirmDialog,
  FieldGroup,
  InlineHelp,
  Input,
  Modal,
  Spinner,
  usePrompt,
} from '@sytue/ui';
import { api, consulta, mensajeDeError } from '../lib/api.js';
import type { Asiento, EstadoCierre, Mayor, MovimientoCierre, RespuestaBalance } from './tipos.js';
import { formatearDinero, formatearFecha } from './formato.js';
import type { T } from '../i18n/index.js';
import {
  PERIODOS,
  ROTULO_PERIODO,
  ROTULO_PROPUESTA,
  propuestasDeCierre,
  rangoDePeriodo,
  type Periodo,
} from './periodo.js';
import { hoyISO } from './Seguimiento.js';

/** Importe en céntimos, o un guion si es cero: una columna de ceros no informa. */
function importe(centimos: number, locale: string): string {
  return centimos === 0 ? '—' : formatearDinero(centimos, locale);
}

/**
 * UN asiento con sus apuntes. Se saca aparte porque lo pintan dos sitios: el
 * libro diario y la ficha del documento que lo generó — y ver ahí el asiento
 * exacto que salió de una factura es lo que cierra el círculo entre «he
 * facturado» y «esto es lo que dice mi contabilidad».
 */
export function AsientoCompacto({
  asiento,
  locale,
  t,
}: {
  asiento: Asiento;
  locale: string;
  t: T;
}): JSX.Element {
  return (
    <div class="border-border bg-surface rounded-card border px-4 py-3">
      <header class="mb-2 flex flex-wrap items-baseline gap-2">
        <span class="text-text font-mono text-xs">{asiento.ref}</span>
        <span class="text-text-muted text-sm">{formatearFecha(asiento.fecha, locale)}</span>
        <span class="text-text mr-auto truncate text-sm font-medium">{asiento.concepto}</span>
        <Badge tone={asiento.estado === 'anulado' ? 'danger' : 'success'}>{asiento.estado}</Badge>
      </header>
      <table class="w-full text-sm">
        <thead class="text-text-subtle text-left">
          <tr>
            <th scope="col" class="py-1 font-medium">
              {t('ledger.account')}
            </th>
            <th scope="col" class="py-1 font-medium">
              {t('ledger.concept')}
            </th>
            <th scope="col" class="py-1 text-right font-medium">
              {t('ledger.debit')}
            </th>
            <th scope="col" class="py-1 text-right font-medium">
              {t('ledger.credit')}
            </th>
          </tr>
        </thead>
        <tbody>
          {(asiento.apuntes ?? []).map((ap, i) => (
            <tr key={`${asiento.id}-${i}`} class="border-border border-t">
              <td class="text-text py-1 font-mono text-xs">{ap.cuenta}</td>
              <td class="text-text-muted truncate py-1">{ap.concepto ?? ''}</td>
              <td class="text-text py-1 text-right tabular-nums">{importe(ap.debe, locale)}</td>
              <td class="text-text py-1 text-right tabular-nums">{importe(ap.haber, locale)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * El título de la pantalla con el selector de período a la derecha. El `<select>`
 * es el de la barra de lote (`text-xs`): más bajo que el título, no cambia el alto
 * de la fila.
 */
function CabeceraPeriodo({
  titulo,
  periodo,
  onPeriodo,
  t,
}: {
  titulo: string;
  periodo: Periodo;
  onPeriodo: (p: Periodo) => void;
  t: T;
}): JSX.Element {
  return (
    <header class="mb-5 flex flex-wrap items-center justify-between gap-3">
      <h1 class="text-text text-2xl font-semibold">{titulo}</h1>
      <select
        aria-label={t('ledger.period')}
        class="sytue-focus-ring border-border bg-surface text-text rounded px-2 py-1 text-xs"
        value={periodo}
        onChange={(e) => onPeriodo((e.target as HTMLSelectElement).value as Periodo)}
      >
        {PERIODOS.map((p) => (
          <option key={p} value={p}>
            {t(ROTULO_PERIODO[p])}
          </option>
        ))}
      </select>
    </header>
  );
}

/*
 * La cabecera con el selector se pinta SIEMPRE, también mientras carga y con un
 * error: devolver solo la rueda o solo el error desmontaba el `<select>` a cada
 * cambio de período —el foco caía al `body` y, con la flecha sobre el select
 * cerrado (que dispara `change` en cada paso), había que volver con Tab cada vez—
 * y, si un período fallaba, no quedaba forma de volver a «Todo».
 */
function Cargando(): JSX.Element {
  return (
    <div class="flex justify-center py-16">
      <Spinner size={22} />
    </div>
  );
}

function Fallo({ texto }: { texto: string }): JSX.Element {
  return <InlineHelp tone="danger">{texto}</InlineHelp>;
}

/** «Todavía no hay nada asentado», o «Nada asentado en este período» con un corte puesto. */
function textoVacio(periodo: Periodo, t: T): string {
  return periodo === 'todo' ? t('ledger.empty') : t('ledger.emptyPeriod');
}

// --- El cierre del ejercicio ------------------------------------------------------

/** El día de un `hasta` del historial, que llega como instante RFC 3339. */
function dia(hasta: string | null | undefined): string {
  return hasta ? hasta.slice(0, 10) : '';
}

/**
 * Hasta dónde queda cerrado el libro al REABRIR: el estado anterior al cierre vigente,
 * es decir, el movimiento más reciente del historial que dejaba el corte antes que el
 * de ahora. Sin ninguno —el primer cierre—, el libro se abre entero (`''`).
 *
 * Reabrir deshace UN paso, no todo el libro: quien reabre diciembre para corregir una
 * factura no quiere, de paso, abrir los tres ejercicios anteriores ya declarados.
 */
export function reaperturaHasta(estado: EstadoCierre): string {
  const actual = estado.cerrado_hasta;
  for (const m of (estado.historial ?? []).slice(1)) {
    const d = dia(m.hasta);
    if (d === '' || d < actual) return d;
  }
  return '';
}

/** El día siguiente a `iso`, en `YYYY-MM-DD`: el `min` del campo de fecha. */
function diaSiguiente(iso: string): string {
  const [a, m, d] = iso.split('-').map(Number);
  return hoyISO(new Date(a ?? 0, (m ?? 1) - 1, (d ?? 1) + 1, 12));
}

/** Qué le falta a la fecha elegida para poder cerrar; vacío, nada. */
function faltaParaCerrar(
  fecha: string,
  cerradoHasta: string,
  hoy: string,
  locale: string,
  t: T,
): string {
  if (fecha > hoy) return t('close.futureDate');
  if (cerradoHasta && fecha <= cerradoHasta)
    return t('close.notAfterCurrent', { fecha: formatearFecha(cerradoHasta, locale) });
  return '';
}

/** El texto de una fila del historial. */
function textoMovimiento(m: MovimientoCierre, locale: string, t: T): string {
  const d = dia(m.hasta);
  if (m.accion === 'cierre') return t('close.closedUntil', { fecha: formatearFecha(d, locale) });
  return d
    ? t('close.historyReopened', { fecha: formatearFecha(d, locale) })
    : t('close.historyReopenedAll');
}

/**
 * El diálogo de «Cerrar período…». Propone el fin del último mes, trimestre y año ya
 * acabados —lo que se cierra de verdad— y deja elegir otra fecha, nunca futura (el
 * `max` del campo es hoy, y aun tecleándola el botón no se enciende). Antes de
 * confirmar dice la CONSECUENCIA con la fecha elegida: es una acción que afecta a
 * todo el equipo y que no se deshace sin dar un motivo.
 */
function DialogoCerrar({
  abierto,
  cerradoHasta,
  locale,
  t,
  onCerrar,
  onHecho,
}: {
  abierto: boolean;
  cerradoHasta: string;
  locale: string;
  t: T;
  onCerrar: () => void;
  onHecho: () => void;
}): JSX.Element {
  const id = useId();
  const hoy = hoyISO();
  const propuestas = propuestasDeCierre().filter((p) => !cerradoHasta || p.hasta > cerradoHasta);
  const [fecha, setFecha] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');

  // Cada vez que se abre, empieza en la propuesta más reciente que avanza el corte.
  useEffect(() => {
    if (!abierto) return;
    setFecha(propuestas[0]?.hasta ?? '');
    setError('');
    setEnviando(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- solo al abrir
  }, [abierto]);

  const falta = fecha ? faltaParaCerrar(fecha, cerradoHasta, hoy, locale, t) : '';
  const cerrar = (): void => {
    if (!fecha || falta || enviando) return;
    setEnviando(true);
    setError('');
    api('contabilidad/cierre', { method: 'POST', body: JSON.stringify({ hasta: fecha }) })
      .then(onHecho)
      .catch((err: unknown) => {
        setEnviando(false);
        setError(mensajeDeError(err, t));
      });
  };

  return (
    <ConfirmDialog
      open={abierto}
      onClose={onCerrar}
      onConfirm={cerrar}
      title={t('close.closeTitle')}
      confirmLabel={t('close.closeConfirm')}
      confirmDisabled={!fecha || falta !== ''}
      busy={enviando}
    >
      <div class="space-y-4">
        {propuestas.length > 0 && (
          <div class="flex flex-wrap gap-2">
            {propuestas.map((p) => (
              <Button
                key={p.propuesta}
                type="button"
                size="sm"
                // Secundario también es relleno de marca: la elegida no se distinguiría.
                variant={fecha === p.hasta ? 'primary' : 'ghost'}
                aria-pressed={fecha === p.hasta}
                onClick={() => setFecha(p.hasta)}
              >
                {t(ROTULO_PROPUESTA[p.propuesta])} · {formatearFecha(p.hasta, locale)}
              </Button>
            ))}
          </div>
        )}
        <FieldGroup
          label={t('close.closeUpTo')}
          htmlFor={`${id}-hasta`}
          required
          {...(falta ? { error: falta } : {})}
        >
          <Input
            id={`${id}-hasta`}
            type="date"
            value={fecha}
            max={hoy}
            {...(cerradoHasta ? { min: diaSiguiente(cerradoHasta) } : {})}
            invalid={falta !== ''}
            onInput={(e) => setFecha(e.currentTarget.value)}
          />
        </FieldGroup>
        {fecha && !falta && (
          <p class="text-text text-sm leading-relaxed" data-consecuencia>
            {t('close.consequence', { fecha: formatearFecha(fecha, locale) })}
          </p>
        )}
        {error && <InlineHelp tone="danger">{error}</InlineHelp>}
      </div>
    </ConfirmDialog>
  );
}

/**
 * La franja del cierre bajo la cabecera del libro: la insignia «Cerrado hasta…», y a
 * quien administra la contabilidad, «Cerrar período…» y «Reabrir…». El historial se
 * despliega debajo. Sin nada que decir —abierto, sin historial y sin permiso—, no
 * ocupa sitio.
 */
export function CierreDelLibro({
  locale,
  t,
  puedeCerrar,
}: {
  locale: string;
  t: T;
  puedeCerrar: boolean;
}): JSX.Element | null {
  const id = useId();
  const [estado, setEstado] = useState<EstadoCierre | null>(null);
  const [error, setError] = useState('');
  const [cerrando, setCerrando] = useState(false);
  const [verHistorial, setVerHistorial] = useState(false);
  const { preguntar, dialogo } = usePrompt();

  const cargar = useCallback(() => {
    api<EstadoCierre>('contabilidad/cierre')
      .then((r) =>
        setEstado({ cerrado_hasta: r.cerrado_hasta ?? '', historial: r.historial ?? [] }),
      )
      // Si no se puede leer, la franja no sale: el libro sigue sirviendo para mirarlo,
      // y cerrar sin saber hasta dónde está cerrado sería a ciegas.
      .catch(() => setEstado(null));
  }, []);
  useEffect(cargar, [cargar]);

  if (!estado) return null;
  const cerradoHasta = estado.cerrado_hasta;
  const historial = estado.historial ?? [];
  if (!cerradoHasta && historial.length === 0 && !puedeCerrar) return null;

  const reabrir = async (): Promise<void> => {
    const hasta = reaperturaHasta(estado);
    const motivo = (
      await preguntar({
        titulo: hasta
          ? t('close.reopenTo', { fecha: formatearFecha(hasta, locale) })
          : t('close.reopenAll'),
        mensaje: t('close.reopenReason'),
        confirmLabel: t('close.reopenConfirm'),
      })
    )?.trim();
    if (!motivo) return;
    setError('');
    api('contabilidad/reapertura', { method: 'POST', body: JSON.stringify({ hasta, motivo }) })
      .then(cargar)
      .catch((err: unknown) => setError(mensajeDeError(err, t)));
  };

  return (
    <section class="mb-5 space-y-3">
      <div class="flex flex-wrap items-center gap-2">
        {cerradoHasta && (
          <Badge tone="info">
            {t('close.closedUntil', { fecha: formatearFecha(cerradoHasta, locale) })}
          </Badge>
        )}
        <span class="mr-auto" />
        {historial.length > 0 && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            aria-expanded={verHistorial}
            aria-controls={`${id}-historial`}
            onClick={() => setVerHistorial((v) => !v)}
          >
            {t('close.history')}
          </Button>
        )}
        {puedeCerrar && cerradoHasta && (
          <Button type="button" size="sm" variant="ghost" onClick={() => void reabrir()}>
            {t('close.reopen')}
          </Button>
        )}
        {puedeCerrar && (
          <Button type="button" size="sm" variant="secondary" onClick={() => setCerrando(true)}>
            {t('close.close')}
          </Button>
        )}
      </div>
      {error && <InlineHelp tone="danger">{error}</InlineHelp>}
      {verHistorial && (
        <ol
          id={`${id}-historial`}
          aria-label={t('close.history')}
          class="border-border bg-surface rounded-card divide-border divide-y border text-sm"
        >
          {historial.map((m, i) => (
            <li
              key={`${m.en}-${i}`}
              class="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-2"
            >
              <span class="text-text-muted tabular-nums">{formatearFecha(m.en, locale, true)}</span>
              <span class="text-text font-medium">{textoMovimiento(m, locale, t)}</span>
              {m.motivo && <span class="text-text-muted w-full italic sm:w-auto">{m.motivo}</span>}
            </li>
          ))}
        </ol>
      )}
      {puedeCerrar && (
        <DialogoCerrar
          abierto={cerrando}
          cerradoHasta={cerradoHasta}
          locale={locale}
          t={t}
          onCerrar={() => setCerrando(false)}
          onHecho={() => {
            setCerrando(false);
            cargar();
          }}
        />
      )}
      {dialogo}
    </section>
  );
}

export function LibroDiario({
  locale,
  t,
  puedeCerrar = false,
}: {
  locale: string;
  t: T;
  /** Administrador de contabilidad con el verbo `administrar`: lo que exige la ruta. */
  puedeCerrar?: boolean;
}): JSX.Element {
  const [asientos, setAsientos] = useState<Asiento[] | null>(null);
  const [error, setError] = useState('');
  const [periodo, setPeriodo] = useState<Periodo>('todo');

  useEffect(() => {
    let vivo = true;
    setAsientos(null);
    setError('');
    api<{ asientos: Asiento[] | null }>(
      `libro${consulta({ limite: 50, ...rangoDePeriodo(periodo) })}`,
    )
      .then((r) => vivo && setAsientos(r.asientos ?? []))
      .catch((err: unknown) => {
        if (vivo) setError(mensajeDeError(err, t));
      });
    return () => {
      vivo = false;
    };
  }, [t, periodo]);

  return (
    <div class="mx-auto max-w-5xl px-6 py-6">
      <CabeceraPeriodo titulo={t('nav.ledger')} periodo={periodo} onPeriodo={setPeriodo} t={t} />
      <CierreDelLibro locale={locale} t={t} puedeCerrar={puedeCerrar} />
      {error ? (
        <Fallo texto={error} />
      ) : !asientos ? (
        <Cargando />
      ) : asientos.length === 0 ? (
        <p class="text-text-muted py-16 text-center text-sm">{textoVacio(periodo, t)}</p>
      ) : (
        <ul class="space-y-3">
          {asientos.map((a) => (
            <li key={a.id}>
              <AsientoCompacto asiento={a} locale={locale} t={t} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * El libro MAYOR de una cuenta: sus apuntes asentados del período, cada uno con el saldo
 * que deja, entre el saldo con el que entra y el que sale. El sumas y saldos dice cuánto
 * hay en la 430; el mayor dice de dónde sale, que es lo siguiente que se pregunta. Se
 * abre desde la fila de la cuenta, con el mismo período.
 */
export function MayorCuenta({
  cuenta,
  periodo,
  locale,
  t,
  onCerrar,
}: {
  cuenta: string;
  periodo: Periodo;
  locale: string;
  t: T;
  onCerrar: () => void;
}): JSX.Element {
  const [datos, setDatos] = useState<Mayor | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    let vivo = true;
    api<Mayor>(`mayor${consulta({ cuenta, ...rangoDePeriodo(periodo) })}`)
      .then((r) => vivo && setDatos(r))
      .catch((err: unknown) => {
        if (vivo) setError(mensajeDeError(err, t));
      });
    return () => {
      vivo = false;
    };
  }, [cuenta, periodo, t]);
  return (
    <Modal
      open
      size="lg"
      onClose={onCerrar}
      title={t('ledger.accountTitle', { account: cuenta })}
      footer={<Button onClick={onCerrar}>{t('action.close')}</Button>}
    >
      {error ? (
        <Fallo texto={error} />
      ) : !datos ? (
        <Cargando />
      ) : (
        <div class="space-y-3" data-mayor={cuenta}>
          <p class="text-text-muted text-sm tabular-nums">
            {t('ledger.openingBalance', { amount: formatearDinero(datos.saldo_inicial, locale) })}
          </p>
          {datos.apuntes.length === 0 ? (
            <p class="text-text-muted py-6 text-center text-sm">{textoVacio(periodo, t)}</p>
          ) : (
            <div class="max-w-full overflow-x-auto">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-border text-text border-b text-left">
                    <th scope="col" class="sytue-overline px-2 py-2 font-bold">
                      {t('ledger.concept')}
                    </th>
                    <th scope="col" class="sytue-overline px-2 py-2 text-right font-bold">
                      {t('ledger.debit')}
                    </th>
                    <th scope="col" class="sytue-overline px-2 py-2 text-right font-bold">
                      {t('ledger.credit')}
                    </th>
                    <th scope="col" class="sytue-overline px-2 py-2 text-right font-bold">
                      {t('balance.balance')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {datos.apuntes.map((a, i) => (
                    <tr key={`${a.asiento_id}-${i}`} class="border-border border-b align-top">
                      <td class="px-2 py-1.5">
                        <span class="text-text block">{a.concepto || a.ref}</span>
                        <span class="text-text-subtle font-mono text-xs">
                          {formatearFecha(a.fecha, locale)} · {a.ref}
                        </span>
                      </td>
                      <td class="text-text px-2 py-1.5 text-right tabular-nums">
                        {importe(a.debe, locale)}
                      </td>
                      <td class="text-text px-2 py-1.5 text-right tabular-nums">
                        {importe(a.haber, locale)}
                      </td>
                      <td class="text-text px-2 py-1.5 text-right font-medium tabular-nums">
                        {formatearDinero(a.saldo, locale)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p class="text-text text-sm font-medium tabular-nums">
            {t('ledger.closingBalance', { amount: formatearDinero(datos.saldo_final, locale) })}
          </p>
          {datos.recortado ? <p class="text-text-subtle text-xs">{t('ai.truncated')}</p> : null}
        </div>
      )}
    </Modal>
  );
}

export function Balance({ locale, t }: { locale: string; t: T }): JSX.Element {
  const [datos, setDatos] = useState<RespuestaBalance | null>(null);
  const [error, setError] = useState('');
  const [periodo, setPeriodo] = useState<Periodo>('todo');
  /** La cuenta cuyo mayor está abierto. */
  const [mayor, setMayor] = useState<string | null>(null);

  useEffect(() => {
    let vivo = true;
    setDatos(null);
    setError('');
    api<RespuestaBalance>(`balance${consulta(rangoDePeriodo(periodo))}`)
      .then((r) => vivo && setDatos(r))
      .catch((err: unknown) => {
        if (vivo) setError(mensajeDeError(err, t));
      });
    return () => {
      vivo = false;
    };
  }, [t, periodo]);

  const cuentas = datos?.cuentas ?? [];
  // Que los dos lados coincidan es LO PRIMERO que mira quien abre esta pantalla.
  // Si no coinciden, algo va muy mal y hay que decirlo aquí y no en un registro.
  const cuadra = datos !== null && datos.total_debe === datos.total_haber;

  return (
    <div class="mx-auto max-w-4xl px-6 py-6">
      <CabeceraPeriodo titulo={t('nav.balance')} periodo={periodo} onPeriodo={setPeriodo} t={t} />
      {error ? (
        <Fallo texto={error} />
      ) : !datos ? (
        <Cargando />
      ) : cuentas.length === 0 ? (
        <p class="text-text-muted py-16 text-center text-sm">{textoVacio(periodo, t)}</p>
      ) : (
        <table class="w-full text-sm">
          <thead>
            <tr class="border-border text-text border-b text-left">
              <th scope="col" class="sytue-overline px-3 py-2 font-bold">
                {t('ledger.account')}
              </th>
              <th scope="col" class="sytue-overline px-3 py-2 text-right font-bold">
                {t('ledger.debit')}
              </th>
              <th scope="col" class="sytue-overline px-3 py-2 text-right font-bold">
                {t('ledger.credit')}
              </th>
              <th scope="col" class="sytue-overline px-3 py-2 text-right font-bold">
                {t('balance.balance')}
              </th>
            </tr>
          </thead>
          <tbody>
            {cuentas.map((c) => (
              <tr key={c.cuenta} class="border-border border-b">
                <td class="text-text px-3 py-2 font-mono text-xs">
                  <button
                    type="button"
                    class="sytue-focus-ring text-primary-text rounded-control hover:underline"
                    aria-label={t('ledger.openAccount', { account: c.cuenta })}
                    title={t('ledger.openAccount', { account: c.cuenta })}
                    onClick={() => setMayor(c.cuenta)}
                  >
                    {c.cuenta}
                  </button>
                </td>
                <td class="text-text px-3 py-2 text-right tabular-nums">
                  {importe(c.debe, locale)}
                </td>
                <td class="text-text px-3 py-2 text-right tabular-nums">
                  {importe(c.haber, locale)}
                </td>
                <td class="text-text px-3 py-2 text-right font-medium tabular-nums">
                  {formatearDinero(c.saldo, locale)}
                </td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr class="text-text font-medium">
              <td class="px-3 py-2">{t('balance.total')}</td>
              <td class="px-3 py-2 text-right tabular-nums">
                {formatearDinero(datos.total_debe, locale)}
              </td>
              <td class="px-3 py-2 text-right tabular-nums">
                {formatearDinero(datos.total_haber, locale)}
              </td>
              <td class="px-3 py-2 text-right">
                {cuadra ? (
                  <Badge tone="success">{t('balance.squares')}</Badge>
                ) : (
                  <Badge tone="danger">{t('balance.doesNotSquare')}</Badge>
                )}
              </td>
            </tr>
          </tfoot>
        </table>
      )}
      {mayor ? (
        <MayorCuenta
          cuenta={mayor}
          periodo={periodo}
          locale={locale}
          t={t}
          onCerrar={() => setMayor(null)}
        />
      ) : null}
    </div>
  );
}
