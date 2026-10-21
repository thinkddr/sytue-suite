/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import type { JSX } from 'preact';
import { useContext, useRef, useState } from 'preact/hooks';
import { Button, ChoiceCard, Modal, Wizard, formatBytes, withBase } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { api, baseDeLaApp, ErrorApi, mensajeDeError } from '../lib/api.js';
import { AltasContext, type Altas } from './AltasContext.js';
import { formatearDinero, formatearFecha, nombreDe } from './formato.js';
import { DocumentosRegistro, hoyISO, subirDocumento } from './Seguimiento.js';
import type { Modelo, Registro, ValorCampo } from './tipos.js';
import { EstadoVerifactuFactura } from './Verifactu.js';

export interface FormularioFacturaAsistida {
  modelo: Modelo;
  semilla: Registro['data'];
  etiquetas: Record<string, string>;
  onCreado: (registro: Registro) => void;
  onCancelar: () => void;
}
export interface AsistenteFacturasProps {
  t: T;
  locale: Locale;
  modelos: Modelo[];
  puedeCrear: (clave: string) => boolean;
  puedeAccion: (modulo: string, verbo: string) => boolean;
  onCerrar: () => void;
  onAbrir: (modelo: Modelo, registro: Registro) => void;
  renderFormulario: (props: FormularioFacturaAsistida) => JSX.Element;
}
interface LecturaFactura {
  data: Registro['data'];
  etiquetas?: Record<string, string>;
  contacto?: Record<string, unknown>;
  numero_origen?: string;
  existente?: Registro;
}
const PDF_MAX = 10 * 1024 * 1024;
const XML_MAX = 8 * 1024 * 1024;
const FACE = 'https://proveedores.face.gob.es/inicio';

// Identidad estable: abrir el alta de una relación no desmonta la ficha padre.
function FormularioCompartido({
  renderFormulario,
  ...formulario
}: FormularioFacturaAsistida & {
  renderFormulario: AsistenteFacturasProps['renderFormulario'];
}): JSX.Element {
  return renderFormulario(formulario);
}

/** Lee y propone. Solo el formulario común crea la factura; adjuntar y asentar
 * tienen botones propios y conservan el registro creadoRef para sus reintentos. */
export function AsistenteFacturas({
  t,
  locale,
  modelos,
  puedeCrear,
  puedeAccion,
  onCerrar,
  onAbrir,
  renderFormulario,
}: AsistenteFacturasProps): JSX.Element {
  const altas = useContext(AltasContext);
  const [claveModelo, setClaveModelo] = useState(() =>
    modelos.some((m) => m.clave === 'factura_compra' && puedeCrear(m.clave))
      ? 'factura_compra'
      : 'factura_venta',
  );
  const [paso, setPaso] = useState(0);
  const [editando, setEditando] = useState(false);
  const [fichero, setFichero] = useState<File | null>(null);
  const [semilla, setSemilla] = useState<Registro['data']>({});
  const [etiquetas, setEtiquetas] = useState<Record<string, string>>({});
  const [contacto, setContacto] = useState<Record<string, ValorCampo> | null>(null);
  const [numeroOrigen, setNumeroOrigen] = useState('');
  const [registro, setRegistro] = useState<Registro | null>(null);
  const [asiento, setAsiento] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const enviandoRef = useRef(false);
  const creadoRef = useRef<Registro | null>(null);
  const modelo = modelos.find((m) => m.clave === claveModelo);
  const recibida = claveModelo === 'factura_compra';
  const cerrar = () => {
    if (!enviandoRef.current) onCerrar();
  };

  function elegir(clave: string): void {
    if (enviandoRef.current || creadoRef.current || !puedeCrear(clave)) return;
    setClaveModelo(clave);
    setFichero(null);
    setSemilla({});
    setEtiquetas({});
    setContacto(null);
    setNumeroOrigen('');
    setError('');
  }
  function manual(): void {
    if (enviandoRef.current || creadoRef.current) return;
    setFichero(null);
    setSemilla({ fecha: hoyISO() });
    setEtiquetas({});
    setContacto(null);
    setNumeroOrigen('');
    setError('');
    setPaso(2);
  }
  async function preparar(): Promise<void> {
    if (enviandoRef.current || creadoRef.current || !modelo || !puedeCrear(modelo.clave)) return;
    if (!fichero) {
      manual();
      return;
    }
    const pdf = fichero.type === 'application/pdf' || /\.pdf$/i.test(fichero.name);
    const xml =
      !pdf &&
      (['application/xml', 'text/xml'].includes(fichero.type) || /\.xml$/i.test(fichero.name));
    if (!pdf && (!xml || !recibida)) {
      setError(t('invoiceAssist.invalidFile'));
      return;
    }
    const limite = pdf ? PDF_MAX : XML_MAX;
    if (fichero.size > limite) {
      setError(t('invoiceAssist.fileTooLarge', { limit: formatBytes(locale, limite) }));
      return;
    }
    enviandoRef.current = true;
    setBusy(true);
    setError('');
    try {
      const lectura = await api<LecturaFactura>(
        pdf ? `${modelo.clave}/ia/rellenar` : 'factura_compra/asistencia/leer',
        {
          method: 'POST',
          headers: { 'content-type': pdf ? 'application/pdf' : 'application/xml' },
          body: await fichero.arrayBuffer(),
        },
      );
      const data = { ...lectura.data };
      if (lectura.existente) {
        creadoRef.current = lectura.existente;
        setRegistro(lectura.existente);
        setNumeroOrigen(lectura.numero_origen ?? '');
        setPaso(3);
        return;
      }
      // Una factura NUEVA no hereda identidad ni marcas de emisión del documento
      // de apoyo. Los XML solo entran por la lectura de facturas de proveedor.
      if (!recibida) {
        for (const campo of [
          'externo_id',
          'tpv_operacion',
          'tpv_huella',
          'sesion',
          'medio_pago',
          'rectifica',
        ])
          delete data[campo];
      }
      setSemilla(data);
      setEtiquetas(lectura.etiquetas ?? {});
      const propuesta = Object.fromEntries(
        Object.entries(lectura.contacto ?? {}).filter(
          ([, v]) => v === null || ['string', 'number', 'boolean'].includes(typeof v),
        ),
      ) as Record<string, ValorCampo>;
      setContacto(recibida && Object.keys(propuesta).length ? propuesta : null);
      setNumeroOrigen(lectura.numero_origen ?? '');
      setPaso(2);
    } catch (e) {
      setError(
        pdf && e instanceof ErrorApi && e.estado === 422
          ? t('ai.pdfUnreadable')
          : mensajeDeError(e, t),
      );
    } finally {
      enviandoRef.current = false;
      setBusy(false);
    }
  }
  function facturaCreada(r: Registro): void {
    if (creadoRef.current) return;
    creadoRef.current = r;
    setRegistro(r);
    setEditando(false);
    setError('');
    setPaso(fichero ? 3 : 4);
  }
  async function adjuntar(): Promise<void> {
    if (enviandoRef.current || !registro || !modelo || !fichero) return;
    enviandoRef.current = true;
    setBusy(true);
    setError('');
    try {
      await subirDocumento(modelo.clave, registro.id, fichero);
      setPaso(4);
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviandoRef.current = false;
      setBusy(false);
    }
  }
  async function asentar(): Promise<void> {
    if (
      enviandoRef.current ||
      !registro ||
      !modelo ||
      asiento ||
      registro.estado !== 'borrador' ||
      !puedeAccion(modelo.modulo, 'asentar')
    )
      return;
    enviandoRef.current = true;
    setBusy(true);
    setError('');
    try {
      const a = await api<{ ref: string }>(`${modelo.clave}/${registro.id}/asentar`, {
        method: 'POST',
        body: '{}',
      });
      setAsiento(a.ref);
      setRegistro({ ...registro, estado: 'confirmado' });
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviandoRef.current = false;
      setBusy(false);
    }
  }

  // El selector de relaciones conserva su alta habitual. Solo se le aporta la
  // semilla del proveedor leído; cancelar esa alta deja el formulario de factura
  // abierto, sin otro Modal por detrás ni un callback de cancelación inventado.
  const altasFormulario: Altas | null = altas
    ? {
        puedeCrear: altas.puedeCrear,
        crear: (clave, onCreado, propuesta) =>
          altas.crear(
            clave,
            (r) => {
              if (clave === 'contacto' && contacto && modelo) {
                const relacion = modelo.campos.find(
                  (c) => c.tipo === 'relacion' && c.modelo === 'contacto',
                );
                if (relacion) setSemilla((s) => ({ ...s, [relacion.clave]: r.id }));
                setEtiquetas((e) => ({ ...e, [r.id]: nombreDe(r) }));
                setContacto(null);
              }
              onCreado(r);
            },
            clave === 'contacto' && contacto ? { ...contacto, ...propuesta } : propuesta,
          ),
      }
    : null;

  if (editando && modelo)
    return (
      <AltasContext.Provider value={altasFormulario}>
        <FormularioCompartido
          renderFormulario={renderFormulario}
          modelo={modelo}
          semilla={semilla}
          etiquetas={etiquetas}
          onCreado={facturaCreada}
          onCancelar={() => {
            setEditando(false);
            setError('');
            setPaso(2);
          }}
        />
      </AltasContext.Provider>
    );

  const contenidoRevision = (
    <div class="space-y-4">
      <p class="text-text-muted text-sm">{t('ai.filled')}</p>
      {modelo ? (
        <dl class="divide-border border-border divide-y border-y text-sm">
          {numeroOrigen ? (
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-text-muted">{t('invoiceAssist.source')}</dt>
              <dd>{numeroOrigen}</dd>
            </div>
          ) : null}
          {modelo.campos
            .filter(
              (c) =>
                ['proveedor', 'cliente', 'fecha', 'base', 'iva', 'total'].includes(c.clave) &&
                semilla[c.clave] !== undefined &&
                semilla[c.clave] !== null,
            )
            .map((c) => {
              const valor = semilla[c.clave];
              const texto =
                c.tipo === 'dinero' && Number.isFinite(Number(valor))
                  ? formatearDinero(Number(valor), locale)
                  : c.tipo === 'fecha'
                    ? formatearFecha(String(valor), locale)
                    : c.tipo === 'relacion'
                      ? (etiquetas[String(valor)] ?? t('relation.choose'))
                      : String(valor);
              return (
                <div key={c.clave} class="flex justify-between gap-4 py-2">
                  <dt class="text-text-muted">{c.etiqueta}</dt>
                  <dd class="text-right tabular-nums">{texto}</dd>
                </div>
              );
            })}
        </dl>
      ) : null}
      {contacto ? (
        <div class="border-border space-y-2 rounded-md border p-3">
          <p class="font-semibold">{String(contacto.nombre ?? '')}</p>
          {contacto.nif ? <p class="text-text-muted text-sm">{String(contacto.nif)}</p> : null}
          <p class="text-text-muted text-sm">{t('invoiceAssist.contactHint')}</p>
        </div>
      ) : null}
    </div>
  );

  return (
    <Modal open onClose={cerrar} dismissable={!busy} title={t('invoiceAssist.title')} size="lg">
      {error ? (
        <p role="alert" class="bg-danger-bg text-danger-text mb-4 rounded-md p-3">
          {error}
        </p>
      ) : null}
      <Wizard
        current={paso}
        busy={busy}
        onCancel={cerrar}
        onCurrentChange={(n) => {
          if (!creadoRef.current && !enviandoRef.current) {
            setError('');
            setPaso(n);
          }
        }}
        steps={[
          {
            id: 'elegir',
            title: t('invoiceAssist.kind'),
            canContinue: Boolean(modelo && puedeCrear(modelo.clave)),
            content: (
              <div class="space-y-3">
                {[
                  { clave: 'factura_compra', titulo: t('invoiceAssist.received') },
                  { clave: 'factura_venta', titulo: t('invoiceAssist.issued') },
                ].map((opcion) => (
                  <ChoiceCard
                    key={opcion.clave}
                    name="asistente-tipo-factura"
                    value={opcion.clave}
                    title={opcion.titulo}
                    description={modelos.find((m) => m.clave === opcion.clave)?.etiqueta}
                    selected={claveModelo === opcion.clave}
                    disabled={
                      !modelos.some((m) => m.clave === opcion.clave) || !puedeCrear(opcion.clave)
                    }
                    onSelect={() => elegir(opcion.clave)}
                  />
                ))}
              </div>
            ),
          },
          {
            id: 'origen',
            title: t('invoiceAssist.source'),
            description: t(
              recibida ? 'invoiceAssist.sourceHint' : 'invoiceAssist.issuedSourceHint',
            ),
            nextLabel: t(fichero ? 'process.prepare' : 'invoiceAssist.manual'),
            onNext: preparar,
            canContinue: Boolean(modelo && puedeCrear(modelo.clave)),
            content: (
              <div class="space-y-3">
                <label class="block space-y-2" for="asistente-factura-original">
                  <span class="text-sm font-semibold">{t('docs.title')}</span>
                  <input
                    id="asistente-factura-original"
                    type="file"
                    disabled={busy}
                    accept={
                      recibida
                        ? 'application/pdf,.pdf,application/xml,text/xml,.xml'
                        : 'application/pdf,.pdf'
                    }
                    class="sytue-focus-ring border-border block w-full rounded-md border p-3 text-base"
                    onChange={(e) => {
                      setFichero(e.currentTarget.files?.[0] ?? null);
                      setError('');
                    }}
                  />
                </label>
                <p class="text-text-muted text-xs">
                  PDF · {formatBytes(locale, PDF_MAX)}
                  {recibida ? ` / XML · ${formatBytes(locale, XML_MAX)}` : ''}
                </p>
                {fichero ? (
                  <Button variant="ghost" disabled={busy} onClick={manual}>
                    {t('invoiceAssist.manual')}
                  </Button>
                ) : null}
              </div>
            ),
          },
          {
            id: 'revisar',
            title: t('invoiceAssist.review'),
            content: contenidoRevision,
            nextLabel: t('invoiceAssist.review'),
            canContinue: Boolean(modelo && puedeCrear(modelo.clave)),
            onNext: () => {
              setError('');
              setEditando(true);
            },
          },
          {
            id: 'adjuntar',
            title: t('docs.title'),
            description: t('invoiceAssist.attachHint'),
            done: true,
            content: (
              <div class="space-y-4">
                <p role="status" class="font-semibold">
                  {registro ? nombreDe(registro) : ''}
                  {registro?.ref ? ` · ${registro.ref}` : ''}
                </p>
                <p class="text-text-muted break-words">{fichero?.name}</p>
                <div class="flex flex-wrap gap-2">
                  <Button loading={busy} onClick={() => void adjuntar()}>
                    {t('docs.upload')}
                  </Button>
                  {modelo && registro ? (
                    <Button
                      variant="secondary"
                      disabled={busy}
                      onClick={() => onAbrir(modelo, registro)}
                    >
                      {t('action.open')}
                    </Button>
                  ) : null}
                  <Button variant="ghost" disabled={busy} onClick={cerrar}>
                    {t('action.close')}
                  </Button>
                </div>
              </div>
            ),
          },
          {
            id: 'final',
            title: t('action.post'),
            done: true,
            content:
              modelo && registro ? (
                <div class="space-y-5">
                  <p role="status" class="font-semibold">
                    {nombreDe(registro)}
                    {registro.ref ? ` · ${registro.ref}` : ''}
                  </p>
                  {asiento ? (
                    <p class="text-success-text">{t('form.posted', { ref: asiento })}</p>
                  ) : registro.estado !== 'borrador' ? (
                    <p>{modelo.estados?.find((e) => e.valor === registro.estado)?.etiqueta}</p>
                  ) : (
                    <>
                      <p class="text-text-muted text-sm">
                        {t(recibida ? 'invoiceAssist.postHint' : 'invoiceAssist.issueHint')}
                      </p>
                      {modelo.contable && puedeAccion(modelo.modulo, 'asentar') ? (
                        <Button loading={busy} onClick={() => void asentar()}>
                          {t('action.post')}
                        </Button>
                      ) : null}
                    </>
                  )}
                  <DocumentosRegistro
                    modelo={modelo.clave}
                    registro={registro.id}
                    locale={locale}
                    t={t}
                    soloLectura={!puedeCrear(modelo.clave)}
                  />
                  {!recibida ? (
                    <section class="border-border space-y-3 border-t pt-4">
                      <EstadoVerifactuFactura
                        factura={registro.id}
                        estadoFactura={registro.estado ?? ''}
                        t={t}
                        soloLectura={!puedeAccion(modelo.modulo, 'asentar')}
                      />
                      <p class="text-text-muted text-sm">{t('sales.facturaeHint')}</p>
                      <div class="flex flex-wrap gap-2">
                        <Button
                          variant="secondary"
                          href={withBase(
                            baseDeLaApp(),
                            `/api/factura_venta/${registro.id}/facturae`,
                          )}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {t('form.downloadFacturae')}
                        </Button>
                        <Button
                          variant="ghost"
                          href={FACE}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          FACe
                        </Button>
                      </div>
                    </section>
                  ) : null}
                  <div class="border-border flex flex-wrap gap-2 border-t pt-4">
                    <Button
                      variant="secondary"
                      disabled={busy}
                      onClick={() => onAbrir(modelo, registro)}
                    >
                      {t('action.open')}
                    </Button>
                    <Button variant="ghost" disabled={busy} onClick={cerrar}>
                      {t('action.close')}
                    </Button>
                  </div>
                </div>
              ) : null,
          },
        ]}
      />
    </Modal>
  );
}
