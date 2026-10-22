/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que cuelga de un registro además de sus campos: sus tareas con fecha y sus
 * documentos (la factura en PDF de un gasto, el justificante de una obligación).
 * Es el «chatter» de Odoo reducido a lo que usa una pyme: qué falta por hacer y
 * dónde está el papel.
 *
 * Subir el documento de una tarea «sube el documento» la da por hecha en el
 * servicio: no hay que acordarse de marcarla.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Badge, Button, Icon, Spinner, formatBytes, useToast, withBase } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import { api, baseDeLaApp, consulta, mensajeDeError } from '../lib/api.js';
import type { ErpMessageKey, useT } from '../i18n/index.js';
import { celda, formatearFecha } from './formato.js';
import type {
  Actividad,
  Adjunto,
  EntradaHistorial,
  Modelo,
  RequisitosFactura,
  ValorCampo,
} from './tipos.js';

type Traducir = ReturnType<typeof useT>;

/** Los tipos que se aceptan, los mismos que comprueba el servicio por contenido. */
export const ACEPTA_DOCUMENTOS =
  'application/pdf,image/jpeg,image/png,image/webp,.xml,application/xml';

/** Sube un adjunto usando la misma validación y permisos de todos los registros. */
export async function subirAdjunto(
  modelo: string,
  registro: string,
  fichero: File,
): Promise<{ adjunto: Adjunto; tareas_hechas: number }> {
  return api<{ adjunto: Adjunto; tareas_hechas: number }>(
    `adjuntos${consulta({ modelo, registro, nombre: fichero.name })}`,
    {
      method: 'POST',
      headers: { 'content-type': fichero.type || 'application/octet-stream' },
      body: await fichero.arrayBuffer(),
    },
  );
}

/** Sube un fichero a un registro y devuelve cuántas tareas cerró. */
export async function subirDocumento(
  modelo: string,
  registro: string,
  fichero: File,
): Promise<number> {
  return (await subirAdjunto(modelo, registro, fichero)).tareas_hechas;
}

/** Hoy en AAAA-MM-DD, en la hora de quien mira. */
export function hoyISO(ahora = new Date()): string {
  const d = new Date(ahora.getTime() - ahora.getTimezoneOffset() * 60_000);
  return d.toISOString().slice(0, 10);
}

/** Lo que tiene que tener una factura para valer, plegado. */
export function RequisitosDeFactura({
  requisitos,
  t,
}: {
  requisitos: RequisitosFactura | null;
  t: Traducir;
}): JSX.Element | null {
  if (!requisitos?.completa?.length) return null;
  const bloque = (titulo: string, lista?: string[]) =>
    lista?.length ? (
      <div>
        <p class="text-text text-xs font-semibold">{titulo}</p>
        <ul class="mt-1 list-disc space-y-0.5 pl-5">
          {lista.map((r) => (
            <li key={r}>{r}</li>
          ))}
        </ul>
      </div>
    ) : null;
  return (
    <details class="border-border rounded-md border px-3 py-2">
      <summary class="sytue-focus-ring text-text-muted hover:text-text cursor-pointer rounded-sm text-xs font-medium">
        {t('docs.invoiceValid')}
      </summary>
      <div class="text-text-muted mt-2 space-y-2 text-xs">
        {bloque(t('docs.invoiceFull'), requisitos.completa)}
        {bloque(t('docs.invoiceSimplified'), requisitos.simplificada)}
        {bloque(t('docs.invoiceForeign'), requisitos.extranjera)}
        {requisitos.fuente ? <p class="text-text-subtle">{requisitos.fuente}</p> : null}
      </div>
    </details>
  );
}

/** Los documentos de un registro: subir, abrir y borrar. */
export function DocumentosRegistro({
  modelo,
  registro,
  titulo,
  requisitos,
  soloLectura,
  locale,
  t,
  onCambio,
}: {
  modelo: string;
  registro: string;
  /** Rótulo del bloque; por defecto, «Documentos». */
  titulo?: string;
  /** Qué tiene que traer el documento, si se sabe. */
  requisitos?: string[];
  soloLectura?: boolean;
  locale: Locale;
  t: Traducir;
  onCambio?: () => void;
}): JSX.Element {
  const aviso = useToast();
  const [lista, setLista] = useState<Adjunto[] | null>(null);
  const [subiendo, setSubiendo] = useState(false);
  const selector = useRef<HTMLInputElement>(null);

  const cargar = useCallback(() => {
    api<{ adjuntos: Adjunto[] }>(`adjuntos${consulta({ modelo, registro })}`)
      .then((r) => setLista(r.adjuntos ?? []))
      .catch(() => setLista([]));
  }, [modelo, registro]);
  useEffect(cargar, [cargar]);

  async function subir(fichero: File): Promise<void> {
    setSubiendo(true);
    try {
      const cerradas = await subirDocumento(modelo, registro, fichero);
      aviso.notify({
        tone: 'success',
        title: t('docs.uploaded'),
        ...(cerradas > 0 ? { description: t('docs.taskDone') } : {}),
      });
      onCambio?.();
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

  async function borrar(a: Adjunto): Promise<void> {
    try {
      await api(`adjuntos/${a.id}`, { method: 'DELETE' });
      cargar();
      onCambio?.();
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    }
  }

  return (
    <section class="space-y-2">
      <div class="flex items-center justify-between gap-2">
        <h3 class="text-text-subtle text-xs font-semibold tracking-wide uppercase">
          {titulo ?? t('docs.title')}
        </h3>
        {soloLectura ? null : (
          <>
            <input
              ref={selector}
              type="file"
              class="hidden"
              accept={ACEPTA_DOCUMENTOS}
              onChange={(e) => {
                const input = e.target as HTMLInputElement;
                const f = input.files?.[0];
                input.value = '';
                if (f) void subir(f);
              }}
            />
            <Button
              size="xs"
              variant="secondary"
              type="button"
              loading={subiendo}
              onClick={() => selector.current?.click()}
            >
              <Icon name="upload" size={12} class="mr-1" />
              {t('docs.upload')}
            </Button>
          </>
        )}
      </div>
      {requisitos?.length ? (
        <ul class="text-text-muted list-disc space-y-0.5 pl-5 text-xs">
          {requisitos.map((r) => (
            <li key={r}>{r}</li>
          ))}
        </ul>
      ) : null}
      {lista === null ? (
        <Spinner size={14} />
      ) : lista.length === 0 ? (
        <p class="text-text-subtle text-xs">{t('docs.empty')}</p>
      ) : (
        <ul class="border-border divide-border divide-y rounded-md border">
          {lista.map((a) => (
            <li key={a.id} class="flex items-center justify-between gap-2 px-3 py-2 text-sm">
              <a
                href={withBase(baseDeLaApp(), `/api/adjuntos/${a.id}?ver=1`)}
                target="_blank"
                rel="noopener"
                class="sytue-focus-ring text-text hover:text-text-muted min-w-0 truncate rounded-sm underline"
              >
                {a.nombre}
              </a>
              <span class="text-text-subtle flex shrink-0 items-center gap-2 text-xs tabular-nums">
                {formatBytes(locale, a.tamano)} ·{' '}
                {formatDate(locale, new Date(a.creado_en), { day: 'numeric', month: 'short' })}
                {soloLectura ? null : (
                  <Button
                    size="xs"
                    variant="ghost"
                    type="button"
                    aria-label={`${t('action.delete')} ${a.nombre}`}
                    onClick={() => void borrar(a)}
                  >
                    <Icon name="trash" size={12} />
                  </Button>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const TIPOS: Actividad['tipo'][] = [
  'subir_documento',
  'pedir_documento',
  'llamada',
  'correo',
  'reunion',
  'tarea',
];

/** El rótulo de un tipo de tarea. Un mapa de claves literales y no una plantilla:
 *  así el catálogo comprueba que existen las seis. */
export function rotuloTipo(tipo: Actividad['tipo'], t: Traducir): string {
  const claves = {
    subir_documento: 'tasks.kind.upload',
    pedir_documento: 'tasks.kind.request',
    llamada: 'tasks.kind.call',
    correo: 'tasks.kind.email',
    reunion: 'tasks.kind.meeting',
    tarea: 'tasks.kind.todo',
  } as const;
  return t(claves[tipo]);
}

/** Las tareas de un registro: las pendientes, marcar hecha y planificar otra. */
export function TareasRegistro({
  modelo,
  registro,
  soloLectura,
  locale,
  revision,
  t,
}: {
  modelo: string;
  registro: string;
  soloLectura?: boolean;
  locale: Locale;
  /** Cambia cuando algo de fuera (subir un documento) puede haber cerrado una. */
  revision?: number;
  t: Traducir;
}): JSX.Element {
  const aviso = useToast();
  const [lista, setLista] = useState<Actividad[] | null>(null);
  const [abierta, setAbierta] = useState(false);
  const [nueva, setNueva] = useState({
    tipo: 'tarea' as Actividad['tipo'],
    resumen: '',
    vence: hoyISO(),
    /** Vacío: quien la crea. A otra persona, le llega el aviso a su campana. */
    responsable: '',
  });
  const [equipo, setEquipo] = useState<{ sub: string; nombre: string; correo: string }[]>([]);
  /** Quien planifica: va primero como «Yo (su nombre)» y no se repite en la lista. */
  const [yo, setYo] = useState('');
  useEffect(() => {
    if (!abierta || equipo.length > 0) return;
    api<{ personas: { sub: string; nombre: string; correo: string }[]; yo?: string }>('equipo')
      .then((r) => {
        setEquipo(r.personas ?? []);
        setYo(r.yo ?? '');
      })
      .catch(() => undefined); // sin directorio, la tarea es de quien la crea, como antes
  }, [abierta, equipo.length]);
  const mio = equipo.find((p) => p.sub === yo);

  const cargar = useCallback(() => {
    api<{ actividades: Actividad[] }>(
      `actividades${consulta({ modelo, registro, estado: 'pendiente' })}`,
    )
      .then((r) => setLista(r.actividades ?? []))
      .catch(() => setLista([]));
  }, [modelo, registro]);
  useEffect(cargar, [cargar, revision]);

  async function hecha(a: Actividad): Promise<void> {
    try {
      await api(`actividades/${a.id}/hecha`, { method: 'POST', body: '{}' });
      cargar();
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    }
  }

  async function planificar(): Promise<void> {
    if (!nueva.resumen.trim()) return;
    try {
      const { responsable, ...resto } = nueva;
      const quien = equipo.find((p) => p.sub === responsable);
      await api('actividades', {
        method: 'POST',
        body: JSON.stringify({
          ...resto,
          modelo,
          registro,
          ...(quien
            ? { responsable_sub: quien.sub, responsable_nombre: quien.nombre || quien.correo }
            : {}),
        }),
      });
      setNueva({ tipo: 'tarea', resumen: '', vence: hoyISO(), responsable: '' });
      setAbierta(false);
      cargar();
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    }
  }

  // El editor vive dentro del formulario de la ficha: Intro aquí guarda la tarea, no la ficha.
  const alIntro = (e: KeyboardEvent) => {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    void planificar();
  };

  const hoy = hoyISO();
  return (
    <section class="space-y-2">
      <div class="flex items-center justify-between gap-2">
        <h3 class="text-text-subtle text-xs font-semibold tracking-wide uppercase">
          {t('tasks.title')}
        </h3>
        {soloLectura ? null : (
          <Button size="xs" variant="ghost" type="button" onClick={() => setAbierta(!abierta)}>
            <Icon name="plus" size={12} class="mr-1" />
            {t('tasks.plan')}
          </Button>
        )}
      </div>
      {abierta ? (
        <div class="border-border flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-end">
          <label class="text-text-muted flex flex-col gap-1 text-xs">
            {t('tasks.type')}
            <select
              value={nueva.tipo}
              onChange={(e) =>
                setNueva({
                  ...nueva,
                  tipo: (e.target as HTMLSelectElement).value as Actividad['tipo'],
                })
              }
              class="sytue-focus-ring border-border bg-bg text-text rounded-md border px-2 py-1.5 text-sm"
            >
              {TIPOS.map((x) => (
                <option key={x} value={x}>
                  {rotuloTipo(x, t)}
                </option>
              ))}
            </select>
          </label>
          <label class="text-text-muted flex min-w-0 flex-1 flex-col gap-1 text-xs">
            {t('tasks.summary')}
            <input
              type="text"
              onKeyDown={alIntro}
              value={nueva.resumen}
              maxLength={300}
              onInput={(e) => setNueva({ ...nueva, resumen: (e.target as HTMLInputElement).value })}
              class="sytue-focus-ring border-border bg-bg text-text rounded-md border px-2 py-1.5 text-sm"
            />
          </label>
          <label class="text-text-muted flex flex-col gap-1 text-xs">
            {t('tasks.due')}
            <input
              type="date"
              onKeyDown={alIntro}
              value={nueva.vence}
              onInput={(e) => setNueva({ ...nueva, vence: (e.target as HTMLInputElement).value })}
              class="sytue-focus-ring border-border bg-bg text-text rounded-md border px-2 py-1.5 text-sm"
            />
          </label>
          {equipo.length > 0 ? (
            <label class="text-text-muted flex flex-col gap-1 text-xs">
              {t('tasks.owner')}
              <select
                value={nueva.responsable}
                onChange={(e) =>
                  setNueva({ ...nueva, responsable: (e.target as HTMLSelectElement).value })
                }
                class="sytue-focus-ring border-border bg-bg text-text rounded-md border px-2 py-1.5 text-sm"
              >
                <option value="">
                  {mio
                    ? t('tasks.ownerMeNamed', { name: mio.nombre || mio.correo })
                    : t('tasks.ownerMe')}
                </option>
                {equipo
                  .filter((p) => p.sub !== yo)
                  .map((p) => (
                    <option key={p.sub} value={p.sub}>
                      {p.nombre || p.correo}
                    </option>
                  ))}
              </select>
            </label>
          ) : null}
          <Button
            size="sm"
            variant="primary"
            type="button"
            disabled={!nueva.resumen.trim()}
            onClick={() => void planificar()}
          >
            {t('action.save')}
          </Button>
        </div>
      ) : null}
      {lista === null ? (
        <Spinner size={14} />
      ) : lista.length === 0 ? (
        <p class="text-text-subtle text-xs">{t('tasks.none')}</p>
      ) : (
        <ul class="space-y-1">
          {lista.map((a) => (
            <li key={a.id} class="flex items-center justify-between gap-2 text-sm">
              <span class="min-w-0">
                <span class="text-text block truncate">{a.resumen}</span>
                <span class="text-text-subtle text-xs">
                  {rotuloTipo(a.tipo, t)} · {a.responsable_nombre}
                </span>
              </span>
              <span class="flex shrink-0 items-center gap-2">
                <Badge tone={a.vence < hoy ? 'danger' : a.vence === hoy ? 'warning' : 'neutral'}>
                  {formatDate(locale, new Date(`${a.vence}T00:00:00`), {
                    day: 'numeric',
                    month: 'short',
                  })}
                </Badge>
                {soloLectura ? null : (
                  <Button size="xs" variant="secondary" type="button" onClick={() => void hecha(a)}>
                    <Icon name="check" size={12} class="mr-1" />
                    {t('tasks.done')}
                  </Button>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const SUCESOS: Record<'creado' | 'borrado' | 'restaurado', ErpMessageKey> = {
  creado: 'history.created',
  borrado: 'history.deleted',
  restaurado: 'history.restored',
};

/**
 * El historial del registro, como el de Odoo: quién lo creó, qué cambió quién y
 * cuándo, y las notas del equipo. Lo escribe la base con cada cambio, venga de la
 * ficha, de un lote, de una importación o del banco.
 */
export function HistorialRegistro({
  modelo,
  registro,
  soloLectura,
  locale,
  etiquetas,
  revision,
  t,
}: {
  modelo: Modelo;
  registro: string;
  soloLectura?: boolean;
  locale: Locale;
  /** El nombre legible de las relaciones que ya conoce la ficha. */
  etiquetas: Record<string, string>;
  /** Cambia cuando algo del registro cambia desde la ficha: se vuelve a pedir. */
  revision?: string;
  t: Traducir;
}): JSX.Element {
  const aviso = useToast();
  const [lista, setLista] = useState<EntradaHistorial[] | null>(null);
  const [nota, setNota] = useState('');
  const [enviando, setEnviando] = useState(false);

  const cargar = useCallback(() => {
    api<{ historial: EntradaHistorial[] }>(`${modelo.clave}/${registro}/historial`)
      .then((r) => setLista(r.historial ?? []))
      .catch(() => setLista([]));
  }, [modelo.clave, registro]);
  useEffect(cargar, [cargar, revision]);

  async function anotar(): Promise<void> {
    const texto = nota.trim();
    if (!texto) return;
    setEnviando(true);
    try {
      await api(`${modelo.clave}/${registro}/historial`, {
        method: 'POST',
        body: JSON.stringify({ texto }),
      });
      setNota('');
      cargar();
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setEnviando(false);
    }
  }

  const rotulo = (clave: string): string =>
    clave === 'estado'
      ? t('history.status')
      : (modelo.campos.find((c) => c.clave === clave)?.etiqueta ?? clave);
  const valor = (clave: string, v: ValorCampo): string => {
    if (clave === 'estado') {
      return modelo.estados?.find((e) => e.valor === v)?.etiqueta ?? String(v ?? '—');
    }
    const campo = modelo.campos.find((c) => c.clave === clave);
    return campo ? celda(campo, v, locale, etiquetas) : String(v ?? '—');
  };

  return (
    <section class="space-y-2">
      <h3 class="text-text-subtle text-xs font-semibold tracking-wide uppercase">
        {t('history.title')}
      </h3>
      {soloLectura ? null : (
        <div class="flex items-end gap-2">
          <textarea
            value={nota}
            rows={2}
            maxLength={5000}
            placeholder={t('history.notePlaceholder')}
            aria-label={t('history.notePlaceholder')}
            onInput={(e) => setNota((e.target as HTMLTextAreaElement).value)}
            class="sytue-focus-ring border-border bg-bg text-text min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
          />
          <Button
            size="sm"
            variant="secondary"
            type="button"
            loading={enviando}
            disabled={!nota.trim()}
            onClick={() => void anotar()}
          >
            {t('history.addNote')}
          </Button>
        </div>
      )}
      {lista === null ? (
        <Spinner size={14} />
      ) : lista.length === 0 ? (
        <p class="text-text-subtle text-xs">{t('history.empty')}</p>
      ) : (
        <ol class="space-y-2">
          {lista.map((e) => (
            <li key={e.id} class="text-sm">
              <p class="text-text-subtle text-xs">
                <span class="text-text font-medium">
                  {e.nombre || (e.sub ? t('history.someone') : 'Sytue')}
                </span>
                {' · '}
                {formatearFecha(e.creado_en, locale, true)}
              </p>
              {e.tipo === 'nota' ? (
                <p class="text-text break-words whitespace-pre-line">{e.texto}</p>
              ) : e.tipo === 'cambio' ? (
                <ul class="text-text-muted text-xs break-words">
                  {Object.entries(e.cambios ?? {}).map(([clave, c]) => (
                    <li key={clave}>
                      {rotulo(clave)}: {valor(clave, c.antes)} → {valor(clave, c.despues)}
                    </li>
                  ))}
                </ul>
              ) : (
                <p class="text-text-muted text-xs">{t(SUCESOS[e.tipo])}</p>
              )}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
