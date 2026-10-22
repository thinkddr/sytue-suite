/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Banco — cada movimiento del banco es una TAREA hasta que tiene su factura o su
 * explicación. Está pensado para quien no sabe de contabilidad: no pregunta por
 * cuentas ni asientos, pregunta «¿es esta factura?» y «¿qué es esto?», y el
 * asiento lo escribe el servicio.
 *
 * El extracto entra en formato Norma 43, que da cualquier banco español desde su
 * banca de empresas. Al subirlo, el servicio ya casa lo que no tiene duda (el
 * cobro que cita el número de factura) y justifica lo que se explica solo (una
 * comisión, un cargo de Hacienda o de la Seguridad Social): aquí queda lo demás.
 *
 * De cada cargo, Sytue AI apunta quién lo cobró y dónde está su factura: la página
 * desde la que se descarga si es un servicio en línea, o a quién pedírsela (con el
 * correo ya escrito en Sytue Mail) si no.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Modal, Spinner, cx, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';
import { appUrl, orgActual, rutaConOrg } from '@sytue/config';
import { api, consulta, mensajeDeError } from '../lib/api.js';
import type { useT } from '../i18n/index.js';
import { desglosar, formatearDinero } from './formato.js';
import { SelectorRelacion } from './SelectorRelacion.js';
import { ACEPTA_DOCUMENTOS, RequisitosDeFactura, hoyISO, subirDocumento } from './Seguimiento.js';
import type {
  FacturaCandidata,
  Organizacion,
  Pagina,
  Registro,
  RequisitosFactura,
  RespuestaBanco,
  ResultadoN43,
  TareaBanco,
} from './tipos.js';

type Traducir = ReturnType<typeof useT>;

/** Tipos de IVA que se ofrecen al crear la factura de un movimiento. */
const TIPOS_IVA = [21, 10, 4, 0] as const;

/** Solo enlaces https: la dirección la propuso un modelo o la tecleó alguien. */
const esHttps = (u?: string): u is string => !!u && /^https:\/\//i.test(u);

export function Banco({
  locale,
  t,
  organizacion,
  requisitos,
  soloLectura,
  onTareas,
}: {
  locale: Locale;
  t: Traducir;
  organizacion: Organizacion | null;
  requisitos: RequisitosFactura | null;
  /** Con solo lectura en contabilidad se ve el banco, pero no se toca. */
  soloLectura: boolean;
  /** Se creó una tarea (el contador de la barra lateral). */
  onTareas: () => void;
}): JSX.Element {
  const aviso = useToast();
  const [datos, setDatos] = useState<RespuestaBanco | null>(null);
  const [error, setError] = useState('');
  const [subiendo, setSubiendo] = useState(false);
  const [ocupada, setOcupada] = useState('');
  const [creando, setCreando] = useState<TareaBanco | null>(null);
  const selector = useRef<HTMLInputElement>(null);

  const cargar = useCallback(() => {
    api<RespuestaBanco>('banco')
      .then((r) => {
        setDatos(r);
        setError('');
      })
      .catch((err) => setError(mensajeDeError(err, t)));
  }, [t]);
  useEffect(cargar, [cargar]);

  // Los cargos que aún no tienen pista se le pasan a Sytue AI una vez por visita;
  // si encontró algo, se vuelve a pintar (y a mirar, por si quedaban más de un lote).
  const pistasPedidasRef = useRef(false);
  useEffect(() => {
    if (!datos || soloLectura || pistasPedidasRef.current) return;
    if (!datos.pendientes.some((m) => m.importe < 0 && !m.empresa)) return;
    pistasPedidasRef.current = true;
    api<{ buscadas: number }>('banco/pistas', { method: 'POST', body: '{}' })
      .then((r) => {
        if (r.buscadas === 0) return;
        pistasPedidasRef.current = false;
        cargar();
      })
      .catch(() => undefined);
  }, [datos, soloLectura, cargar]);

  async function subir(ficheros: File[]): Promise<void> {
    setSubiendo(true);
    try {
      for (const fichero of ficheros) {
        // Los bancos escriben el Norma 43 en ISO-8859-1: se manda en bruto y el
        // servicio lo lee como tal. Pasarlo por `text()` rompería cada «Ñ».
        const res = await api<ResultadoN43>('linea_extracto/importar', {
          method: 'POST',
          headers: { 'content-type': 'text/plain' },
          body: await fichero.arrayBuffer(),
        });
        aviso.notify({
          tone: 'success',
          title: t('bank.imported', { count: res.nuevos }),
          description: t('bank.importedDetail', {
            matched: res.casados,
            explained: res.explicados,
            pending: res.pendientes,
          }),
        });
      }
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setSubiendo(false);
      cargar();
    }
  }

  /** Una acción sobre un movimiento: la ejecuta, avisa si falla y recarga. */
  async function hacer(id: string, ruta: string, cuerpo?: object): Promise<boolean> {
    setOcupada(id);
    try {
      await api(`banco/${id}/${ruta}`, { method: 'POST', body: JSON.stringify(cuerpo ?? {}) });
      return true;
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
      return false;
    } finally {
      setOcupada('');
      cargar();
    }
  }

  const dinero = (c: number) => formatearDinero(c, locale);
  const fecha = (iso: string) =>
    formatDate(locale, new Date(iso), { day: 'numeric', month: 'short', timeZone: 'UTC' });
  const rotuloMotivo = (clave?: string) =>
    datos?.motivos.find((m) => m.clave === clave)?.rotulo ?? clave ?? '';

  return (
    <div class="mx-auto max-w-5xl space-y-6 px-6 py-6">
      <header class="border-border flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <h1 class="text-text text-lg font-semibold tracking-tight">{t('bank.title')}</h1>
          <p class="text-text-muted mt-1 max-w-prose text-sm">{t('bank.subtitle')}</p>
        </div>
        <div class={cx('flex shrink-0 items-center gap-2', soloLectura && 'hidden')}>
          <input
            ref={selector}
            type="file"
            multiple
            class="hidden"
            accept=".n43,.q43,.aeb,.csb,.txt,text/plain"
            onChange={(e) => {
              const input = e.target as HTMLInputElement;
              const ficheros = [...(input.files ?? [])];
              input.value = '';
              if (ficheros.length > 0) void subir(ficheros);
            }}
          />
          <Button
            variant="primary"
            size="sm"
            loading={subiendo}
            onClick={() => selector.current?.click()}
          >
            <Icon name="upload" size={14} class="mr-1" />
            {t('bank.upload')}
          </Button>
        </div>
      </header>

      <details class="border-border rounded-card border">
        <summary class="sytue-focus-ring text-text-muted hover:text-text rounded-card cursor-pointer px-4 py-3 text-sm font-medium">
          {t('bank.howTo')}
        </summary>
        <p class="text-text-muted max-w-prose px-4 pb-4 text-sm">{t('bank.howToBody')}</p>
      </details>

      {error ? (
        <p class="bg-danger-bg text-danger-text rounded-md px-4 py-3 text-sm">{error}</p>
      ) : null}

      {!datos ? (
        <div class="flex justify-center py-12">
          <Spinner size={24} />
        </div>
      ) : datos.pendientes.length === 0 ? (
        <EmptyState
          title={t('bank.allDone')}
          description={t('bank.allDoneHint')}
          illustration={<Illustration name="emptyCalendar" size={96} />}
        />
      ) : (
        <section aria-labelledby="banco-pendientes" class="space-y-2">
          <h2 id="banco-pendientes" class="sytue-overline text-text-subtle">
            {t('bank.pendingTitle', { count: datos.total_pendientes })}
          </h2>
          <ul class="border-border divide-border rounded-card divide-y border">
            {datos.pendientes.map((tarea) => (
              <Tarea
                key={tarea.id}
                tarea={tarea}
                motivos={datos.motivos}
                ocupada={ocupada === tarea.id}
                dinero={dinero}
                fecha={fecha}
                organizacion={organizacion}
                soloLectura={soloLectura}
                onCasar={(factura) => void hacer(tarea.id, 'conciliar', { factura })}
                onExplicar={(motivo) => void hacer(tarea.id, 'explicar', { motivo })}
                onCrear={() => setCreando(tarea)}
                t={t}
              />
            ))}
          </ul>
        </section>
      )}

      {datos && datos.hechas.length > 0 ? (
        <details class="border-border rounded-card border">
          <summary class="sytue-focus-ring text-text-muted hover:text-text rounded-card cursor-pointer px-4 py-3 text-sm font-medium">
            {t('bank.done')}
          </summary>
          <ul class="divide-border divide-y px-4 pb-2">
            {datos.hechas.map((m) => (
              <li key={m.id} class="flex items-center justify-between gap-3 py-2 text-sm">
                <div class="min-w-0">
                  <span class="text-text block truncate">{m.descripcion}</span>
                  <span class="text-text-subtle block truncate text-xs">
                    {fecha(m.fecha)} ·{' '}
                    {m.factura_ref
                      ? t('bank.matched', { ref: m.factura_ref })
                      : rotuloMotivo(m.motivo)}
                  </span>
                </div>
                <div class="flex shrink-0 items-center gap-2">
                  <Importe centimos={m.importe} dinero={dinero} t={t} />
                  {soloLectura ? null : (
                    <Button
                      size="xs"
                      variant="ghost"
                      loading={ocupada === m.id}
                      onClick={() => void hacer(m.id, 'reabrir')}
                    >
                      {t('bank.undo')}
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </details>
      ) : null}

      {creando ? (
        <FacturaDeMovimiento
          tarea={creando}
          locale={locale}
          requisitos={requisitos}
          t={t}
          onCerrar={() => setCreando(null)}
          onCreada={async (facturaId) => {
            const tarea = creando;
            setCreando(null);
            await hacer(tarea.id, 'conciliar', { factura: facturaId });
            onTareas();
          }}
        />
      ) : null}
    </div>
  );
}

function Importe({
  centimos,
  dinero,
  t,
}: {
  centimos: number;
  dinero: (c: number) => string;
  t: Traducir;
}): JSX.Element {
  return (
    <span
      class={cx(
        'text-sm font-semibold tabular-nums',
        centimos > 0 ? 'text-success-text' : 'text-text',
      )}
      aria-label={`${centimos > 0 ? t('bank.in') : t('bank.out')} ${dinero(Math.abs(centimos))}`}
    >
      {centimos > 0 ? '+' : '−'}
      {dinero(Math.abs(centimos))}
    </span>
  );
}

function Tarea({
  tarea,
  motivos,
  ocupada,
  dinero,
  fecha,
  organizacion,
  soloLectura,
  onCasar,
  onExplicar,
  onCrear,
  t,
}: {
  tarea: TareaBanco;
  motivos: RespuestaBanco['motivos'];
  ocupada: boolean;
  dinero: (c: number) => string;
  fecha: (iso: string) => string;
  organizacion: Organizacion | null;
  soloLectura: boolean;
  onCasar: (facturaId: string) => void;
  onExplicar: (motivo: string) => void;
  onCrear: () => void;
  t: Traducir;
}): JSX.Element {
  const [modo, setModo] = useState<'' | 'motivo' | 'otra'>('');
  const [motivo, setMotivo] = useState(tarea.sugerencia ?? '');
  const [otra, setOtra] = useState({ id: '', etiqueta: '' });
  const entra = tarea.importe > 0;
  const modeloFactura = entra ? 'factura_venta' : 'factura_compra';

  return (
    <li class="space-y-2 px-4 py-3">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="text-text text-sm font-medium break-words">{tarea.descripcion}</p>
          <p class="text-text-subtle text-xs">{fecha(tarea.fecha)}</p>
        </div>
        <Importe centimos={tarea.importe} dinero={dinero} t={t} />
      </div>

      {!entra && (tarea.empresa || esHttps(tarea.url_factura)) ? (
        <p class="text-text-muted flex flex-wrap items-center gap-x-1.5 text-xs">
          <Icon name="ai" size={12} />
          {tarea.empresa ? <span class="text-text font-medium">{tarea.empresa}</span> : null}
          {esHttps(tarea.url_factura) ? (
            <a
              href={tarea.url_factura}
              target="_blank"
              rel="noopener noreferrer"
              class="sytue-focus-ring text-text rounded-sm underline"
            >
              {t('bank.hintDownload')}
            </a>
          ) : tarea.email_facturas ? (
            <a
              href={correoPidiendoFactura(tarea, organizacion, dinero, fecha, t)}
              target="_blank"
              rel="noopener"
              class="sytue-focus-ring text-text rounded-sm underline"
            >
              {t('bank.hintRequest', { email: tarea.email_facturas })}
            </a>
          ) : (
            <span>
              {t('bank.hintAsk')}
              {esHttps(tarea.web) ? (
                <>
                  {' · '}
                  <a
                    href={tarea.web}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="sytue-focus-ring rounded-sm underline"
                  >
                    {t('bank.hintWeb')}
                  </a>
                </>
              ) : null}
            </span>
          )}
        </p>
      ) : null}

      {tarea.candidatas.length > 0 ? (
        <div class="bg-bg-subtle space-y-1 rounded-md px-3 py-2">
          <p class="text-text-muted text-xs font-medium">{t('bank.isThisIt')}</p>
          {tarea.candidatas.map((f: FacturaCandidata) => (
            <div key={f.id} class="flex flex-wrap items-center justify-between gap-2 text-sm">
              <span class="text-text min-w-0 truncate">
                {f.ref} · {f.tercero || '—'} · {dinero(f.total)}
              </span>
              <Button
                size="xs"
                variant={f.segura ? 'primary' : 'secondary'}
                loading={ocupada}
                disabled={soloLectura}
                onClick={() => onCasar(f.id)}
              >
                {t('bank.yesThisOne')}
              </Button>
            </div>
          ))}
        </div>
      ) : null}

      {entra ? <p class="text-text-muted text-sm">{t('bank.customerPaymentHint')}</p> : null}

      <div class={cx('flex flex-wrap items-center gap-1.5', soloLectura && 'hidden')}>
        <Button size="xs" variant="secondary" onClick={onCrear}>
          <Icon name="plus" size={12} class="mr-1" />
          {t('bank.addInvoice')}
        </Button>
        <Button
          size="xs"
          variant="ghost"
          aria-expanded={modo === 'otra'}
          onClick={() => setModo(modo === 'otra' ? '' : 'otra')}
        >
          {t('bank.otherInvoice')}
        </Button>
        <Button
          size="xs"
          variant="ghost"
          aria-expanded={modo === 'motivo'}
          onClick={() => setModo(modo === 'motivo' ? '' : 'motivo')}
        >
          {t('bank.noInvoice')}
        </Button>
      </div>

      {modo === 'otra' ? (
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
          <div class="min-w-0 flex-1">
            <SelectorRelacion
              id={`otra-${tarea.id}`}
              modelo={modeloFactura}
              valor={otra.id}
              etiqueta={otra.etiqueta}
              onCambio={(id, etiqueta) => setOtra({ id, etiqueta })}
              t={t}
            />
          </div>
          <Button
            size="sm"
            variant="primary"
            disabled={!otra.id}
            loading={ocupada}
            onClick={() => onCasar(otra.id)}
          >
            {t('bank.yesThisOne')}
          </Button>
        </div>
      ) : null}

      {modo === 'motivo' ? (
        <div class="space-y-1">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label class="text-text-muted text-xs font-medium" for={`motivo-${tarea.id}`}>
              {t('bank.reason')}
            </label>
            <select
              id={`motivo-${tarea.id}`}
              value={motivo}
              onChange={(e) => setMotivo((e.target as HTMLSelectElement).value)}
              class="sytue-focus-ring border-border bg-bg text-text min-w-0 flex-1 rounded-md border px-3 py-2 text-sm"
            >
              <option value="" disabled>
                {t('relation.choose')}
              </option>
              {motivos.map((m) => (
                <option key={m.clave} value={m.clave}>
                  {m.rotulo}
                  {m.clave === tarea.sugerencia ? ` · ${t('bank.suggested')}` : ''}
                </option>
              ))}
            </select>
            <Button
              size="sm"
              variant="primary"
              disabled={!motivo}
              loading={ocupada}
              onClick={() => onExplicar(motivo)}
            >
              {t('action.save')}
            </Button>
          </div>
          {entra && motivo === 'otro' ? (
            <p class="text-text-subtle text-xs">{t('bank.otherIncomeHint')}</p>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}

/**
 * El correo que pide una factura, ya escrito, en el compositor de Sytue Mail: con
 * el cargo (fecha, importe, concepto) y los datos fiscales que la factura tiene
 * que llevar. La persona lo revisa y lo manda.
 */
function correoPidiendoFactura(
  tarea: TareaBanco,
  organizacion: Organizacion | null,
  dinero: (c: number) => string,
  fecha: (iso: string) => string,
  t: Traducir,
): string {
  // La org de la URL vista (ADR 0056) en el enlace a Mail, que sale canónico
  // (`/mail/<org>/?action=compose`) y aterriza sin el 302 del middleware de Mail.
  // Sin org también funciona. `appUrl` y no el `href` del catálogo a propósito:
  // ahí Mail es su dominio desnudo, y la org tiene que ir TRAS el prefijo
  // `/mail`, no en la raíz del host.
  const u = new URL(
    rutaConOrg(appUrl('mail'), orgActual(import.meta.env.BASE_URL), '/'),
    window.location.href,
  );
  const importe = dinero(Math.abs(tarea.importe));
  u.searchParams.set('action', 'compose');
  u.searchParams.set('to', tarea.email_facturas ?? '');
  u.searchParams.set(
    'subject',
    t('bank.requestSubject', { amount: importe, date: fecha(tarea.fecha) }),
  );
  u.searchParams.set(
    'body',
    t('bank.requestBody', {
      amount: importe,
      date: fecha(tarea.fecha),
      concept: tarea.descripcion,
      company: organizacion?.nombre ?? '',
      nif: organizacion?.nif ?? '',
      address: [organizacion?.direccion, organizacion?.cp, organizacion?.ciudad]
        .filter(Boolean)
        .join(', '),
    }),
  );
  return u.toString();
}

/**
 * Crear la factura de un movimiento sin salir del banco: quién, qué IVA lleva y
 * listo. La fecha y el total salen del propio movimiento. Lo que no cabe aquí
 * (varios tipos de IVA, retenciones) se crea en su lista y se elige con «Otra
 * factura».
 */
function FacturaDeMovimiento({
  tarea,
  locale,
  requisitos,
  t,
  onCerrar,
  onCreada,
}: {
  tarea: TareaBanco;
  locale: Locale;
  requisitos: RequisitosFactura | null;
  t: Traducir;
  onCerrar: () => void;
  onCreada: (facturaId: string) => Promise<void>;
}): JSX.Element {
  const entra = tarea.importe > 0;
  const total = Math.abs(tarea.importe);
  const [nombre, setNombre] = useState(tarea.empresa ?? '');
  const [pdf, setPdf] = useState<File | null>(null);
  const [sugeridos, setSugeridos] = useState<Registro[]>([]);
  const [fechaFactura, setFechaFactura] = useState(tarea.fecha.slice(0, 10));
  const [tipo, setTipo] = useState<number>(21);
  const [guardando, setGuardando] = useState(false);
  const [fallo, setFallo] = useState('');
  const { base, iva } = desglosar(total, tipo);

  // Sugerencias de contactos mientras se escribe, con la lista nativa del
  // navegador: si no está, se crea con ese nombre al guardar.
  useEffect(() => {
    if (nombre.trim().length < 2) return;
    let vivo = true;
    const temporizador = setTimeout(() => {
      api<Pagina>(`contacto${consulta({ q: nombre.trim(), limite: 8 })}`)
        .then((p) => vivo && setSugeridos(p.registros ?? []))
        .catch(() => vivo && setSugeridos([]));
    }, 180);
    return () => {
      vivo = false;
      clearTimeout(temporizador);
    };
  }, [nombre]);

  async function guardar(): Promise<void> {
    const limpio = nombre.trim();
    if (!limpio) return;
    setGuardando(true);
    setFallo('');
    try {
      let contacto = sugeridos.find((c) => c.etiqueta.toLowerCase() === limpio.toLowerCase())?.id;
      if (!contacto) {
        const nuevo = await api<Registro>('contacto', {
          method: 'POST',
          body: JSON.stringify({ data: { nombre: limpio, tipo: 'organizacion' } }),
        });
        contacto = nuevo.id;
      }
      const modeloFactura = entra ? 'factura_venta' : 'factura_compra';
      const factura = await api<Registro>(modeloFactura, {
        method: 'POST',
        // La API guarda lo que va en `data`, como la ficha: sin el envoltorio
        // la factura llegaba vacía y la validación la rechazaba.
        body: JSON.stringify({
          data: {
            [entra ? 'cliente' : 'proveedor']: contacto,
            fecha: fechaFactura,
            base,
            iva,
            total,
            notas: tarea.descripcion,
          },
        }),
      });
      // Con el PDF, la factura queda completa. Sin él, queda la tarea de subirlo hoy:
      // una factura sin su papel no sirve ante Hacienda.
      if (pdf) {
        await subirDocumento(modeloFactura, factura.id, pdf);
      } else {
        await api('actividades', {
          method: 'POST',
          body: JSON.stringify({
            tipo: 'subir_documento',
            resumen: t('bank.uploadTask', { company: limpio }),
            vence: hoyISO(),
            modelo: modeloFactura,
            registro: factura.id,
          }),
        }).catch(() => undefined);
      }
      await onCreada(factura.id);
    } catch (err) {
      setFallo(mensajeDeError(err, t));
      setGuardando(false);
    }
  }

  const dinero = (c: number) => formatearDinero(c, locale);
  return (
    <Modal
      open
      onClose={onCerrar}
      title={entra ? t('bank.invoiceTitleIn') : t('bank.invoiceTitleOut')}
      footer={
        <>
          <Button variant="ghost" size="sm" onClick={onCerrar}>
            {t('action.close')}
          </Button>
          <Button
            variant="primary"
            size="sm"
            loading={guardando}
            disabled={!nombre.trim()}
            onClick={() => void guardar()}
          >
            {t('bank.saveAndMatch')}
          </Button>
        </>
      }
    >
      <div class="space-y-4">
        <p class="text-text-muted text-sm">
          {tarea.descripcion} · <span class="tabular-nums">{dinero(total)}</span>
        </p>
        <div class="space-y-1">
          <label class="text-text text-sm font-medium" for="factura-tercero">
            {entra ? t('bank.whoIn') : t('bank.whoOut')}
          </label>
          <input
            id="factura-tercero"
            type="text"
            list="factura-tercero-lista"
            value={nombre}
            autocomplete="off"
            onInput={(e) => setNombre((e.target as HTMLInputElement).value)}
            class="sytue-focus-ring border-border bg-bg text-text w-full rounded-md border px-3 py-2 text-sm"
          />
          <datalist id="factura-tercero-lista">
            {sugeridos.map((c) => (
              <option key={c.id} value={c.etiqueta} />
            ))}
          </datalist>
          <p class="text-text-subtle text-xs">{t('bank.whoHint')}</p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-1">
            <label class="text-text text-sm font-medium" for="factura-fecha">
              {t('bank.date')}
            </label>
            <input
              id="factura-fecha"
              type="date"
              value={fechaFactura}
              onInput={(e) => setFechaFactura((e.target as HTMLInputElement).value)}
              class="sytue-focus-ring border-border bg-bg text-text w-full rounded-md border px-3 py-2 text-sm"
            />
          </div>
          <fieldset class="space-y-1">
            <legend class="text-text text-sm font-medium">{t('bank.vat')}</legend>
            <div class="flex flex-wrap gap-1.5">
              {TIPOS_IVA.map((x) => (
                <button
                  key={x}
                  type="button"
                  aria-pressed={tipo === x}
                  onClick={() => setTipo(x)}
                  class={cx(
                    'sytue-focus-ring rounded-control border px-3 py-1 text-sm',
                    tipo === x
                      ? 'border-border-strong bg-selected text-text font-semibold'
                      : 'border-border text-text-muted hover:text-text',
                  )}
                >
                  {x === 0 ? t('bank.vatNone') : `${x} %`}
                </button>
              ))}
            </div>
          </fieldset>
        </div>
        <p class="text-text-muted text-xs tabular-nums">
          {t('bank.breakdown', { base: dinero(base), vat: dinero(iva), total: dinero(total) })}
        </p>
        <div class="space-y-1">
          <label class="text-text text-sm font-medium" for="factura-pdf">
            {t('bank.invoiceFile')}
          </label>
          <input
            id="factura-pdf"
            type="file"
            accept={ACEPTA_DOCUMENTOS}
            onChange={(e) => setPdf((e.target as HTMLInputElement).files?.[0] ?? null)}
            class="text-text-muted block w-full text-sm"
          />
          <p class="text-text-subtle text-xs">
            {pdf ? t('bank.invoiceHint') : t('bank.invoiceFileHint')}
          </p>
        </div>
        <RequisitosDeFactura requisitos={requisitos} t={t} />
        {fallo ? <p class="text-danger-text text-sm">{fallo}</p> : null}
      </div>
    </Modal>
  );
}
