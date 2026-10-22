/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Button, Checkbox, Modal, Spinner, Wizard, withBase } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { api, baseDeLaApp, consulta, mensajeDeError } from '../lib/api.js';
import type {
  Adjunto,
  CalculoObligacion,
  Obligacion,
  Organizacion,
  Pagina,
  Registro,
  RespuestaObligaciones,
} from './tipos.js';
import { formatearDinero, nombreDe } from './formato.js';
import { Banco } from './Banco.js';
import { DocumentosRegistro } from './Seguimiento.js';

interface Progreso {
  fase: number;
  referencia: string;
}

/** Un paso, una tarea. Calcula y comprueba con el servicio; persiste el progreso
 * en la organización, no en el navegador. Nunca simula aceptación de AEAT. */
export function ProcesoObligacion({
  obligacion,
  organizacion,
  moneda,
  locale,
  t,
  soloLectura,
  puedeLeer,
  puedeAccion,
  onCerrar,
  onCambio,
}: {
  obligacion: Obligacion;
  organizacion: Organizacion;
  moneda: string;
  locale: Locale;
  t: T;
  soloLectura: boolean;
  puedeLeer: (modulo: string) => boolean;
  puedeAccion: (modulo: string, verbo: string) => boolean;
  onCerrar: () => void;
  onCambio: () => void;
}): JSX.Element {
  const [o, setO] = useState(obligacion);
  const [progreso, setProgreso] = useState<Progreso | null>(null);
  const [paso, setPaso] = useState(0);
  const [calculo, setCalculo] = useState<CalculoObligacion | null>(null);
  const [errorCalculo, setErrorCalculo] = useState('');
  const [revisionCalculo, setRevisionCalculo] = useState(0);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [banco, setBanco] = useState(false);
  const [revisado, setRevisado] = useState(false);
  const [presentado, setPresentado] = useState(false);
  const [referencia, setReferencia] = useState('');
  const [recibos, setRecibos] = useState(false);
  const [revision, setRevision] = useState(0);
  const enviandoRef = useRef(false);
  const referenciaEditadaRef = useRef(false);
  const ruta = `obligaciones/proceso${consulta({ clave: o.clave, periodo: o.periodo })}`;
  const documentos = useMemo(
    () =>
      o.documentos?.length
        ? o.documentos
        : [{ clave: 'justificante', nombre: t('process.receipt'), requisitos: [] }],
    [o.documentos, t],
  );
  const pendientes = Object.values(o.pendientes ?? {}).some((n) => n > 0);
  const faltaAcceso =
    ((o.pendientes?.banco ?? 0) > 0 && !puedeLeer('contabilidad')) ||
    ((o.pendientes?.facturas ?? 0) > 0 && !puedeLeer('facturacion')) ||
    ((o.pendientes?.nominas ?? 0) > 0 && !puedeLeer('nominas'));

  useEffect(() => {
    let vivo = true;
    api<Progreso>(ruta)
      .then((p) => {
        if (vivo) {
          setProgreso(p);
          setPaso(p.fase);
          if (!referenciaEditadaRef.current || p.fase === 4) setReferencia(p.referencia);
          setError('');
        }
      })
      .catch((e: unknown) => {
        if (vivo) setError(mensajeDeError(e, t));
      });
    return () => {
      vivo = false;
    };
  }, [ruta, revision, t]);

  useEffect(() => {
    let vivo = true;
    setRecibos(false);
    Promise.all(
      documentos.map((d) =>
        api<{ adjuntos: Adjunto[] }>(
          `adjuntos${consulta({ modelo: 'obligacion', registro: `${o.clave}:${o.periodo}:${d.clave}` })}`,
        ),
      ),
    )
      .then((todos) => {
        if (vivo) setRecibos(todos.every((r) => r.adjuntos.length > 0));
      })
      .catch((e: unknown) => {
        if (vivo) setError(mensajeDeError(e, t));
      });
    return () => {
      vivo = false;
    };
  }, [o.clave, o.periodo, documentos, revision, t]);

  async function comprobar(): Promise<Obligacion> {
    const calendario = await api<RespuestaObligaciones>(
      `obligaciones${consulta({ anio: Number(o.periodo.slice(0, 4)) })}`,
    );
    const actual = calendario.obligaciones?.find(
      (x) => x.clave === o.clave && x.periodo === o.periodo,
    );
    if (!actual) throw new Error(t('error.generic'));
    setO(actual);
    return actual;
  }

  function comprobarPendientes(): void {
    void comprobar().catch((e: unknown) => setError(mensajeDeError(e, t)));
    onCambio();
  }

  function documentosCambiaron(): void {
    setRevision((r) => r + 1);
    onCambio();
  }

  async function avanzar(): Promise<void> {
    if (enviandoRef.current || soloLectura) return;
    enviandoRef.current = true;
    setBusy(true);
    setError('');
    try {
      if (paso < 3) {
        const actual = await comprobar();
        if (Object.values(actual.pendientes ?? {}).some((n) => n > 0)) {
          setPaso(0);
          setError(t('process.resolve'));
          return;
        }
      }
      const p = await api<Progreso>(ruta, {
        method: 'POST',
        body: JSON.stringify({ fase: paso + 1, referencia }),
      });
      // El servidor puede haber avanzado más por otra persona de la organización.
      setProgreso(p);
      setPaso(p.fase);
      setReferencia(p.referencia);
      referenciaEditadaRef.current = false;
      onCambio();
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviandoRef.current = false;
      setBusy(false);
    }
  }

  useEffect(() => {
    if (paso !== 1 || !o.calculable) return;
    let vivo = true;
    setCalculo(null);
    setErrorCalculo('');
    setRevisado(false);
    api<CalculoObligacion>(
      `obligaciones/calculo${consulta({ clave: o.clave, periodo: o.periodo })}`,
    )
      .then((c) => {
        if (vivo) setCalculo(c);
      })
      .catch((e: unknown) => {
        if (vivo) setErrorCalculo(mensajeDeError(e, t));
      });
    return () => {
      vivo = false;
    };
  }, [paso, o.clave, o.periodo, o.calculable, revisionCalculo, t]);

  const justificantes = documentos.map((d) => (
    <DocumentosRegistro
      key={d.clave}
      modelo="obligacion"
      registro={`${o.clave}:${o.periodo}:${d.clave}`}
      titulo={d.nombre}
      requisitos={d.requisitos}
      soloLectura={soloLectura}
      locale={locale}
      t={t}
      onCambio={documentosCambiaron}
    />
  ));

  return (
    <Modal
      open
      size="lg"
      title={`${o.nombre} · ${o.periodo}`}
      onClose={() => {
        if (!busy) onCerrar();
      }}
      dismissable={!busy}
    >
      {error ? (
        <p role="alert" class="bg-danger-bg text-danger-text mb-4 rounded-md p-3">
          {error}
        </p>
      ) : null}
      {!progreso ? (
        <div class="space-y-3 py-8">
          {error ? (
            <Button variant="secondary" onClick={() => setRevision((r) => r + 1)}>
              {t('process.check')}
            </Button>
          ) : (
            <Spinner />
          )}
          <Button variant="ghost" onClick={onCerrar}>
            {t('action.close')}
          </Button>
        </div>
      ) : (
        <Wizard
          current={paso}
          onCurrentChange={(p) => {
            setPaso(p);
            setRevisado(false);
            setPresentado(false);
          }}
          busy={busy}
          onCancel={onCerrar}
          steps={[
            {
              id: 'preparar',
              title: t('process.prepare'),
              description: o.guia?.que ?? o.concepto_rotulo,
              nextLabel: t('obligations.prepare'),
              onNext: avanzar,
              canContinue: !soloLectura && !pendientes,
              content: (
                <div class="space-y-4">
                  <p class="text-text-muted">{t('process.prepareHint')}</p>
                  {Object.entries(o.pendientes ?? {})
                    .filter(([, n]) => n > 0)
                    .map(([k, n]) => (
                      <div
                        key={k}
                        class="border-border flex items-center justify-between gap-3 rounded-md border p-3"
                      >
                        <span>
                          {k === 'banco'
                            ? t('nav.bank')
                            : k === 'nominas'
                              ? t('process.payroll')
                              : t('process.invoices')}{' '}
                          <strong class="tabular-nums">{n}</strong>
                        </span>
                        {k === 'banco' && puedeLeer('contabilidad') ? (
                          <Button variant="secondary" onClick={() => setBanco((v) => !v)}>
                            {t('action.open')}
                          </Button>
                        ) : null}
                      </div>
                    ))}
                  {faltaAcceso ? (
                    <p class="text-warning-text">{t('process.accessNeeded')}</p>
                  ) : null}
                  {(o.pendientes?.facturas ?? 0) > 0 && puedeLeer('facturacion') ? (
                    <FacturasPendientes
                      o={o}
                      t={t}
                      locale={locale}
                      moneda={moneda}
                      puedeAsentar={puedeAccion('facturacion', 'asentar')}
                      onCambio={comprobarPendientes}
                    />
                  ) : null}
                  {(o.pendientes?.nominas ?? 0) > 0 && puedeLeer('nominas') ? (
                    <Button
                      variant="secondary"
                      href={withBase(baseDeLaApp(), '/?m=nomina')}
                      target="_blank"
                      rel="noopener"
                    >
                      {t('process.payroll')}
                    </Button>
                  ) : null}
                  {banco && puedeLeer('contabilidad') ? (
                    <Banco
                      locale={locale}
                      t={t}
                      organizacion={organizacion}
                      requisitos={null}
                      soloLectura={!puedeAccion('contabilidad', 'conciliar')}
                      onTareas={comprobarPendientes}
                    />
                  ) : null}
                  {!pendientes ? <p class="text-success-text">{t('obligations.ready')}</p> : null}
                  <Button variant="ghost" onClick={comprobarPendientes}>
                    {t('process.check')}
                  </Button>
                </div>
              ),
            },
            {
              id: 'revisar',
              title: t('onboarding.step.review'),
              nextLabel: t('process.review'),
              onNext: avanzar,
              canContinue: !soloLectura && revisado && (!o.calculable || calculo !== null),
              content: (
                <div class="space-y-4">
                  {o.calculable && !calculo ? (
                    errorCalculo ? (
                      <div class="space-y-2">
                        <p role="alert" class="text-danger-text">
                          {errorCalculo}
                        </p>
                        <Button
                          variant="secondary"
                          onClick={() => setRevisionCalculo((r) => r + 1)}
                        >
                          {t('process.check')}
                        </Button>
                      </div>
                    ) : (
                      <Spinner />
                    )
                  ) : null}
                  {calculo ? (
                    <>
                      <p class="text-text-muted">
                        {t('obligations.prepared', { from: calculo.desde, to: calculo.hasta })}
                      </p>
                      <dl class="divide-border divide-y">
                        {[
                          [t('process.outputTax'), calculo.haber_centimos],
                          [t('process.inputTax'), calculo.debe_centimos],
                          [t('obligations.result'), calculo.resultado_centimos],
                        ].map(([label, value]) => (
                          <div key={String(label)} class="flex justify-between gap-4 py-3">
                            <dt>{label}</dt>
                            <dd class="text-lg font-semibold tabular-nums">
                              {formatearDinero(Number(value), locale, moneda)}
                            </dd>
                          </div>
                        ))}
                      </dl>
                      {!calculo.asentado ? (
                        <p class="text-warning-text">{t('obligations.notPosted')}</p>
                      ) : null}
                    </>
                  ) : null}
                  {o.guia?.necesitas.length ? (
                    <ul class="space-y-2">
                      {o.guia.necesitas.map((n) => (
                        <li key={n}>{n}</li>
                      ))}
                    </ul>
                  ) : null}
                  <Checkbox
                    label={t('process.checked')}
                    disabled={soloLectura}
                    checked={revisado}
                    onChange={(e) => setRevisado(e.currentTarget.checked)}
                  />
                </div>
              ),
            },
            {
              id: 'presentar',
              title: t('process.file'),
              description: t('process.signature'),
              nextLabel: t('process.filed'),
              onNext: avanzar,
              canContinue: !soloLectura && presentado,
              content: (
                <div class="space-y-4">
                  {/* El nombre del portal debajo y no en el botón: «Abrir el portal ·
                      Agencia Tributaria — Sede electrónica» medía 375 en un cuerpo de 342 y
                      se salía del diálogo del teléfono. */}
                  <div class="space-y-1">
                    <Button href={o.portal.url} target="_blank" rel="noopener">
                      {t('obligations.portal')}
                    </Button>
                    <p class="sytue-meta">{o.portal.nombre}</p>
                  </div>
                  {o.guia?.pasos.length ? (
                    <ol class="list-decimal space-y-2 pl-5">
                      {o.guia.pasos.map((p) => (
                        <li key={p}>{p}</li>
                      ))}
                    </ol>
                  ) : null}
                  <Checkbox
                    label={t('process.filed')}
                    disabled={soloLectura}
                    checked={presentado}
                    onChange={(e) => setPresentado(e.currentTarget.checked)}
                  />
                </div>
              ),
            },
            {
              id: 'guardar',
              title: t('process.receipt'),
              nextLabel: t('process.finish'),
              onNext: avanzar,
              canContinue: !soloLectura && recibos && referencia.trim().length > 0,
              content: (
                <div class="space-y-5">
                  {justificantes}
                  <label class="grid gap-2" for="proceso-referencia">
                    {t('process.reference')}
                    <input
                      id="proceso-referencia"
                      name="referencia"
                      value={referencia}
                      maxLength={200}
                      required
                      readOnly={soloLectura}
                      onInput={(e) => {
                        referenciaEditadaRef.current = true;
                        setReferencia(e.currentTarget.value);
                      }}
                      class="border-border bg-bg text-text sytue-focus-ring rounded-md border p-3"
                    />
                  </label>
                </div>
              ),
            },
            {
              id: 'listo',
              title: t('onboarding.step.done'),
              done: true,
              content: (
                <div class="space-y-3">
                  <p class="text-success-text text-lg font-semibold">{t('process.archived')}</p>
                  <p>{progreso.referencia}</p>
                  <p class="text-text-muted">{t('process.acceptance')}</p>
                  {justificantes}
                  <Button onClick={onCerrar}>{t('action.close')}</Button>
                </div>
              ),
            },
          ]}
        />
      )}
    </Modal>
  );
}

function FacturasPendientes({
  o,
  t,
  locale,
  moneda,
  puedeAsentar,
  onCambio,
}: {
  o: Obligacion;
  t: T;
  locale: Locale;
  moneda: string;
  puedeAsentar: boolean;
  onCambio: () => void;
}): JSX.Element {
  const [pagina, setPagina] = useState<Pagina | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [mas, setMas] = useState(false);
  const [revision, setRevision] = useState(0);
  const enviandoRef = useRef(false);
  const ruta = `obligaciones/proceso/pendientes${consulta({ clave: o.clave, periodo: o.periodo })}`;

  useEffect(() => {
    let vivo = true;
    setPagina(null);
    setError('');
    api<Pagina>(ruta)
      .then((p) => {
        if (vivo) setPagina(p);
      })
      .catch((e: unknown) => {
        if (vivo) setError(mensajeDeError(e, t));
      });
    return () => {
      vivo = false;
    };
  }, [ruta, revision, t]);

  async function cargarMas(): Promise<void> {
    if (!pagina?.siguiente || enviandoRef.current) return;
    enviandoRef.current = true;
    setMas(true);
    setError('');
    try {
      const p = await api<Pagina>(
        `obligaciones/proceso/pendientes${consulta({ clave: o.clave, periodo: o.periodo, cursor: pagina.siguiente })}`,
      );
      setPagina((anterior) => ({
        ...p,
        registros: [...(anterior?.registros ?? []), ...(p.registros ?? [])],
      }));
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviandoRef.current = false;
      setMas(false);
    }
  }

  async function asentar(r: Registro): Promise<void> {
    if (enviandoRef.current || !puedeAsentar) return;
    enviandoRef.current = true;
    setBusy(r.id);
    setError('');
    try {
      await api(`${r.modelo}/${r.id}/asentar`, { method: 'POST' });
      setRevision((n) => n + 1);
      onCambio();
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviandoRef.current = false;
      setBusy('');
    }
  }

  return (
    <div class="space-y-3">
      {error ? (
        <p role="alert" class="text-danger-text">
          {error}
        </p>
      ) : null}
      {pagina === null ? (
        error ? (
          <Button variant="secondary" onClick={() => setRevision((r) => r + 1)}>
            {t('process.check')}
          </Button>
        ) : (
          <Spinner />
        )
      ) : (
        (pagina.registros ?? []).map((r) => (
          <div key={r.id} class="border-border rounded-md border p-3">
            <p class="font-semibold">
              {nombreDe(r)} ·{' '}
              {formatearDinero(Number(r.data.total ?? 0), locale, String(r.data.moneda ?? moneda))}
            </p>
            <div class="mt-2 flex flex-wrap gap-2">
              <Button
                variant="secondary"
                href={withBase(baseDeLaApp(), `/?m=${r.modelo}&r=${r.id}`)}
                target="_blank"
                rel="noopener"
              >
                {t('onboarding.step.review')}
              </Button>
              {puedeAsentar ? (
                <Button
                  disabled={Boolean(busy) || mas}
                  loading={busy === r.id}
                  onClick={() => void asentar(r)}
                >
                  {t('action.post')}
                </Button>
              ) : null}
            </div>
          </div>
        ))
      )}
      {pagina?.siguiente ? (
        <Button
          variant="secondary"
          loading={mas}
          disabled={Boolean(busy)}
          onClick={() => void cargarMas()}
        >
          {t('action.more')}
        </Button>
      ) : null}
    </div>
  );
}
