/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bandeja de formularios de Sytue Pages (PAGES-PLAN §6.3, §10.6).
 *
 * Una bandeja, no una tabla de datos: se sabe qué es nuevo (leídas / sin leer, con
 * el recuento en cada formulario), se busca en todo lo que escribió la gente, se abre
 * una respuesta entera con la PRUEBA DEL CONSENTIMIENTO (el texto que aceptó y
 * cuándo, que es lo que hay que enseñar si alguien pregunta con qué base se guardan
 * sus datos) y se borra la de quien pide que se borren sus datos (RGPD, art. 17).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Icon,
  Input,
  Modal,
  Skeleton,
  Table,
  type TableColumn,
} from '@sytue/ui';
import { formatDate, formatDateTime, type Locale } from '@sytue/i18n';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { ruta } from '../lib/rutas.js';
import { correoPlausible, filtrarEnvios } from '../lib/listados.js';

export interface BandejaProps {
  siteId: string;
  locale: Locale;
}

interface FormField {
  id: string;
  kind: string;
  label?: Record<string, string>;
}

interface FormInbox {
  form_id: string;
  name: string;
  live: boolean;
  fields: FormField[];
  notify: string[];
  total: number;
  /** Respuestas sin marcar como leídas. */
  unread?: number;
}

interface FormSubmission {
  id: string;
  site_id: string;
  form_id: string;
  data: Record<string, unknown>;
  language: string;
  consent_text?: string;
  consent_at?: string;
  read_at?: string | null;
  created_at: string;
}

export function Bandeja({ siteId, locale }: BandejaProps): JSX.Element {
  const t = (k: ClaveMensaje, params?: Record<string, string | number>) =>
    translate(locale, k, params);

  const [formularios, setFormularios] = useState<FormInbox[]>([]);
  const [formIdActual, setFormIdActual] = useState<string>('');
  const [envios, setEnvios] = useState<FormSubmission[]>([]);
  const [cargando, setCargando] = useState(true);
  const [cargandoEnvios, setCargandoEnvios] = useState(false);
  const [errorCarga, setErrorCarga] = useState(false);
  const [busqueda, setBusqueda] = useState('');
  // Respuesta abierta en el detalle, y la que se va a borrar (confirmación aparte).
  const [abierta, setAbierta] = useState<FormSubmission | null>(null);
  const [aBorrar, setABorrar] = useState<FormSubmission | null>(null);
  const [borrando, setBorrando] = useState(false);
  const [aviso, setAviso] = useState<{ tono: 'exito' | 'error'; texto: string } | null>(null);

  // Avisos por correo para el formulario seleccionado
  const [avisados, setAvisados] = useState<string[]>([]);
  const [nuevoCorreo, setNuevoCorreo] = useState('');
  const [guardandoAvisos, setGuardandoAvisos] = useState(false);
  const [avisoGuardado, setAvisoGuardado] = useState(false);
  const [errorAvisos, setErrorAvisos] = useState('');

  const cargarFormularios = useCallback(async () => {
    setCargando(true);
    setErrorCarga(false);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/forms`));
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { forms?: FormInbox[] };
      const lista = data.forms ?? [];
      setFormularios(lista);
      if (lista.length > 0) {
        const primero = lista[0]?.form_id ?? '';
        setFormIdActual((prev) => (prev && lista.some((f) => f.form_id === prev) ? prev : primero));
      }
    } catch {
      setErrorCarga(true);
    } finally {
      setCargando(false);
    }
  }, [siteId]);

  useEffect(() => {
    void cargarFormularios();
  }, [cargarFormularios]);

  const formularioActual = formularios.find((f) => f.form_id === formIdActual);

  // Sincronizar lista de avisados al cambiar de formulario
  useEffect(() => {
    if (formularioActual) {
      setAvisados(formularioActual.notify ?? []);
      setAvisoGuardado(false);
      setErrorAvisos('');
      setNuevoCorreo('');
    }
  }, [formularioActual]);

  // Cargar envíos del formulario seleccionado
  const cargarEnvios = useCallback(
    async (fId: string) => {
      if (!fId) return;
      setCargandoEnvios(true);
      try {
        const res = await fetch(ruta(`/api/sites/${siteId}/forms/${fId}/submissions`));
        if (!res.ok) throw new Error(String(res.status));
        const data = (await res.json()) as { submissions?: FormSubmission[] };
        setEnvios(data.submissions ?? []);
      } catch {
        setEnvios([]);
      } finally {
        setCargandoEnvios(false);
      }
    },
    [siteId],
  );

  useEffect(() => {
    if (formIdActual) {
      setBusqueda('');
      void cargarEnvios(formIdActual);
    }
  }, [formIdActual, cargarEnvios]);

  const visibles = useMemo(() => filtrarEnvios(envios, busqueda), [envios, busqueda]);

  /** Ajusta el recuento de sin leer de un formulario sin volver a pedir la lista. */
  const restarSinLeer = (formId: string, delta: number) =>
    setFormularios((prev) =>
      prev.map((f) =>
        f.form_id === formId ? { ...f, unread: Math.max(0, (f.unread ?? 0) + delta) } : f,
      ),
    );

  /**
   * Marca respuestas como leídas (o sin leer). Sin ids, todas las del formulario.
   * Optimista: la bandeja cambia al pulsar y se deshace si el servicio dice que no.
   */
  async function marcar(ids: string[], leidas: boolean) {
    if (!formIdActual) return;
    const ahora = new Date().toISOString();
    const afectadas = new Set(
      ids.length > 0 ? ids : envios.filter((e) => !e.read_at).map((e) => e.id),
    );
    const antes = envios;
    const cambian = envios.filter(
      (e) => afectadas.has(e.id) && Boolean(e.read_at) !== leidas,
    ).length;
    setEnvios((prev) =>
      prev.map((e) =>
        afectadas.has(e.id) ? { ...e, read_at: leidas ? (e.read_at ?? ahora) : null } : e,
      ),
    );
    restarSinLeer(formIdActual, leidas ? -cambian : cambian);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/forms/${formIdActual}/submissions/read`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids, read: leidas }),
      });
      if (!res.ok) throw new Error(String(res.status));
    } catch {
      setEnvios(antes);
      restarSinLeer(formIdActual, leidas ? cambian : -cambian);
      setAviso({ tono: 'error', texto: t('forms.read_error') });
    }
  }

  /** Abre el detalle y, si estaba sin leer, la marca como leída: abrirla es leerla. */
  function abrir(e: FormSubmission) {
    setAbierta(e);
    if (!e.read_at) void marcar([e.id], true);
  }

  async function borrar() {
    if (!aBorrar || !formIdActual) return;
    setBorrando(true);
    try {
      const res = await fetch(
        ruta(`/api/sites/${siteId}/forms/${formIdActual}/submissions/${aBorrar.id}`),
        { method: 'DELETE' },
      );
      if (!res.ok && res.status !== 404) throw new Error(String(res.status));
      setEnvios((prev) => prev.filter((e) => e.id !== aBorrar.id));
      setFormularios((prev) =>
        prev.map((f) =>
          f.form_id === formIdActual
            ? {
                ...f,
                total: Math.max(0, f.total - 1),
                unread: Math.max(0, (f.unread ?? 0) - (aBorrar.read_at ? 0 : 1)),
              }
            : f,
        ),
      );
      setAbierta(null);
      setAviso({ tono: 'exito', texto: t('forms.deleted') });
    } catch {
      setAviso({ tono: 'error', texto: t('forms.delete_error') });
    } finally {
      setBorrando(false);
      setABorrar(null);
    }
  }

  const agregarAvisado = () => {
    const correo = nuevoCorreo.trim().toLowerCase();
    if (!correo) return;
    // Antes una dirección mal escrita se ignoraba en silencio: el campo no hacía
    // nada y no decía por qué.
    if (!correoPlausible(correo)) {
      setErrorAvisos(t('forms.notify_invalid'));
      return;
    }
    if (avisados.includes(correo)) {
      setNuevoCorreo('');
      return;
    }
    if (avisados.length >= 10) {
      setErrorAvisos(t('forms.notify_max'));
      return;
    }
    setAvisados((prev) => [...prev, correo]);
    setNuevoCorreo('');
    setAvisoGuardado(false);
    setErrorAvisos('');
  };

  const quitarAvisado = (correo: string) => {
    setAvisados((prev) => prev.filter((c) => c !== correo));
    setAvisoGuardado(false);
  };

  const guardarAvisos = async () => {
    if (!formIdActual) return;
    setGuardandoAvisos(true);
    setErrorAvisos('');
    setAvisoGuardado(false);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/forms/${formIdActual}/notify`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ notify: avisados }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setAvisoGuardado(true);
      // Actualizar en el estado local del formulario
      setFormularios((prev) =>
        prev.map((f) => (f.form_id === formIdActual ? { ...f, notify: avisados } : f)),
      );
    } catch {
      setErrorAvisos(t('forms.notify_save_error'));
    } finally {
      setGuardandoAvisos(false);
    }
  };

  function formatearValor(val: unknown): string {
    if (val === undefined || val === null || val === '') return '—';
    if (typeof val === 'string') return val;
    if (typeof val === 'number') return String(val);
    // Era un «Sí»/«No» escrito a mano: salía en castellano en los 26 idiomas.
    if (typeof val === 'boolean') return val ? t('forms.yes') : t('forms.no');
    return JSON.stringify(val);
  }

  const etiquetaDe = (c: FormField) => c.label?.[locale] ?? c.label?.es ?? c.label?.en ?? c.id;

  if (cargando) {
    return (
      <section class="space-y-4">
        <Skeleton class="h-8 w-48" />
        <Skeleton class="h-48 w-full" />
      </section>
    );
  }

  if (errorCarga) {
    return (
      <section class="space-y-4">
        <EmptyState title={t('forms.title')} description={t('forms.load_error')} />
        <div class="flex justify-center">
          <Button variant="secondary" onClick={() => void cargarFormularios()}>
            {t('forms.retry')}
          </Button>
        </div>
      </section>
    );
  }

  if (formularios.length === 0) {
    return (
      <section class="space-y-4">
        <EmptyState title={t('forms.empty')} description={t('forms.empty_help')} />
      </section>
    );
  }

  const sinLeer = formularioActual?.unread ?? 0;

  // Columnas de la tabla para el formulario actual
  const columnas: TableColumn<FormSubmission>[] = [
    {
      key: 'state',
      header: <span class="sr-only">{t('forms.col_state')}</span>,
      cell: (s) => (s.read_at ? null : <Badge tone="primary">{t('forms.new')}</Badge>),
    },
    {
      key: 'created_at',
      header: t('forms.col_date'),
      cell: (s) => (
        <span
          class={`text-sm whitespace-nowrap ${s.read_at ? 'text-text-muted' : 'text-text font-semibold'}`}
        >
          {formatDate(locale, s.created_at)}
        </span>
      ),
    },
    {
      key: 'language',
      header: t('forms.col_lang'),
      cell: (s) => (
        <span class="text-text-muted font-mono text-xs uppercase">{s.language || '—'}</span>
      ),
    },
  ];

  // Añadir columnas de los campos del formulario
  if (formularioActual?.fields) {
    for (const c of formularioActual.fields) {
      columnas.push({
        key: `campo_${c.id}`,
        header: etiquetaDe(c),
        cell: (s: FormSubmission) => (
          <span class="block max-w-xs truncate">{formatearValor(s.data[c.id])}</span>
        ),
      });
    }
  }
  columnas.push({
    key: 'actions',
    header: <span class="sr-only">{t('forms.col_actions')}</span>,
    align: 'end',
    cell: (s) => (
      <Button
        variant="ghost"
        size="sm"
        aria-label={t('forms.open_aria', { fecha: formatDateTime(locale, s.created_at) })}
        onClick={() => abrir(s)}
      >
        {t('forms.open')}
      </Button>
    ),
  });

  // Campos del detalle: los del formulario en su orden y, después, los que solo
  // traen respuestas de versiones anteriores, para no esconder nada de lo enviado.
  const camposDetalle = (e: FormSubmission): Array<[string, unknown]> => {
    const conocidos = new Set((formularioActual?.fields ?? []).map((c) => c.id));
    const filas: Array<[string, unknown]> = (formularioActual?.fields ?? []).map((c) => [
      etiquetaDe(c),
      e.data[c.id],
    ]);
    for (const [k, v] of Object.entries(e.data)) if (!conocidos.has(k)) filas.push([k, v]);
    return filas;
  };

  return (
    <section class="space-y-6" aria-labelledby="bandeja-formularios-titulo">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <h2 id="bandeja-formularios-titulo" class="sytue-h3 text-text">
            {t('forms.title')}
          </h2>
          <p class="text-text-muted mt-1 text-sm">
            {formularioActual
              ? t('forms.count', { count: formularioActual.total })
              : t('forms.submissions')}
            {sinLeer > 0 ? ` · ${t('forms.unread_count', { count: sinLeer })}` : ''}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          {sinLeer > 0 ? (
            <Button variant="ghost" size="sm" onClick={() => void marcar([], true)}>
              {t('forms.mark_all_read')}
            </Button>
          ) : null}
          {formularioActual && formularioActual.total > 0 ? (
            <Button
              variant="secondary"
              size="sm"
              href={ruta(
                `/api/sites/${siteId}/forms/${formularioActual.form_id}/submissions.csv?lang=${locale}`,
              )}
            >
              {t('forms.export_csv')}
            </Button>
          ) : null}
        </div>
      </div>

      {aviso ? (
        <p
          role={aviso.tono === 'error' ? 'alert' : 'status'}
          class={`text-sm font-medium ${aviso.tono === 'error' ? 'text-danger-text' : 'text-success-text'}`}
        >
          {aviso.texto}
        </p>
      ) : null}

      {/* Selector de formulario si hay más de uno */}
      {formularios.length > 1 ? (
        <div class="border-border flex flex-wrap gap-2 border-b pb-3">
          {formularios.map((f) => {
            const activo = f.form_id === formIdActual;
            return (
              <button
                key={f.form_id}
                type="button"
                aria-pressed={activo}
                onClick={() => setFormIdActual(f.form_id)}
                class={`sytue-focus-ring inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors pointer-coarse:min-h-[max(44px,2.75rem)] ${
                  activo
                    ? 'bg-primary text-on-primary'
                    : 'bg-surface text-text hover:bg-surface-raised border-border border'
                }`}
              >
                <span>{f.name || f.form_id}</span>
                <span
                  class={`rounded-full px-1.5 py-0.5 text-xs ${activo ? 'bg-white/20' : 'bg-surface-sunken'}`}
                >
                  {f.total}
                </span>
                {(f.unread ?? 0) > 0 ? (
                  <Badge tone="primary">{t('forms.unread_count', { count: f.unread ?? 0 })}</Badge>
                ) : null}
                {!f.live ? <Badge tone="warning">{t('forms.not_live')}</Badge> : null}
              </button>
            );
          })}
        </div>
      ) : null}

      {envios.length > 0 ? (
        <Input
          type="search"
          value={busqueda}
          class="w-72 max-w-full"
          leadingIcon={<Icon name="search" size={16} />}
          placeholder={t('forms.search')}
          aria-label={t('forms.search')}
          onInput={(e) => setBusqueda(e.currentTarget.value)}
        />
      ) : null}

      {/* Tabla de respuestas */}
      {cargandoEnvios ? (
        <div class="space-y-2">
          <Skeleton class="h-10 w-full" />
          <Skeleton class="h-10 w-full" />
          <Skeleton class="h-10 w-full" />
        </div>
      ) : visibles.length > 0 ? (
        <div class="border-border bg-surface overflow-x-auto rounded-xl border">
          <Table<FormSubmission>
            caption={formularioActual?.name || formularioActual?.form_id || t('forms.title')}
            columns={columnas}
            rows={visibles}
            rowKey={(s: FormSubmission): string => s.id}
          />
        </div>
      ) : envios.length > 0 ? (
        <p class="text-text-muted py-6 text-center text-sm">
          {t('forms.search_empty', { q: busqueda.trim() })}
        </p>
      ) : (
        <EmptyState title={t('forms.empty')} description={t('forms.empty_help')} />
      )}

      {/* Panel de avisos por correo */}
      {formularioActual ? (
        <div class="border-border bg-surface space-y-4 rounded-xl border p-4 sm:p-6">
          <div class="min-w-0">
            <h3 class="sytue-h4 text-text">{t('forms.notify_title')}</h3>
            <p class="text-text-muted mt-1 text-sm">{t('forms.notify_help')}</p>
          </div>

          <div class="flex flex-wrap gap-2">
            {avisados.map((correo) => (
              <span
                key={correo}
                class="bg-surface-raised border-border text-text inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm"
              >
                <span>{correo}</span>
                <button
                  type="button"
                  onClick={() => quitarAvisado(correo)}
                  class="sytue-focus-ring text-text-muted hover:text-danger inline-flex items-center justify-center rounded-full p-0.5 pointer-coarse:min-h-[max(44px,2.75rem)]"
                  aria-label={t('forms.notify_remove', { email: correo })}
                >
                  <Icon name="close" size={14} />
                </button>
              </span>
            ))}
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <Input
              type="email"
              value={nuevoCorreo}
              class="min-w-[240px]"
              placeholder="email@example.com"
              aria-label={t('forms.notify_add')}
              invalid={Boolean(errorAvisos)}
              onInput={(e) => {
                setNuevoCorreo(e.currentTarget.value);
                setErrorAvisos('');
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  agregarAvisado();
                }
              }}
            />
            <Button
              variant="secondary"
              size="sm"
              onClick={agregarAvisado}
              disabled={!nuevoCorreo.trim() || avisados.length >= 10}
            >
              {t('forms.notify_add')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => void guardarAvisos()}
              loading={guardandoAvisos}
            >
              {t('forms.notify_save')}
            </Button>
          </div>

          {avisoGuardado ? (
            <p class="text-success-text text-sm font-medium" role="status">
              {t('forms.notify_saved')}
            </p>
          ) : null}
          {errorAvisos ? (
            <p class="text-danger-text text-sm font-medium" role="alert">
              {errorAvisos}
            </p>
          ) : null}
        </div>
      ) : null}

      {/* Detalle de una respuesta, con la prueba del consentimiento. */}
      <Modal
        open={abierta !== null}
        onClose={() => setAbierta(null)}
        title={
          abierta
            ? t('forms.detail_title', { fecha: formatDateTime(locale, abierta.created_at) })
            : ''
        }
        size="md"
        footer={
          abierta ? (
            <>
              <Button
                variant="ghost"
                class="text-danger-text mr-auto"
                onClick={() => setABorrar(abierta)}
              >
                {t('forms.delete')}
              </Button>
              <Button
                variant="secondary"
                onClick={() => {
                  void marcar([abierta.id], false);
                  setAbierta(null);
                }}
              >
                {t('forms.mark_unread')}
              </Button>
              <Button variant="primary" onClick={() => setAbierta(null)}>
                {t('forms.close')}
              </Button>
            </>
          ) : null
        }
      >
        {abierta ? (
          <div class="flex flex-col gap-4">
            <dl class="divide-border border-border divide-y rounded-lg border">
              {camposDetalle(abierta).map(([rotulo, valor]) => (
                <div key={rotulo} class="grid gap-1 px-3 py-2 sm:grid-cols-[12rem_1fr]">
                  <dt class="text-text-muted text-sm">{rotulo}</dt>
                  <dd class="text-text text-sm break-words whitespace-pre-wrap">
                    {formatearValor(valor)}
                  </dd>
                </div>
              ))}
            </dl>
            <div class="bg-bg-subtle rounded-lg p-3">
              <p class="text-text text-sm font-semibold">{t('forms.consent_title')}</p>
              <p class="text-text-muted mt-1 text-xs">
                {abierta.consent_at
                  ? t('forms.consent_at', { fecha: formatDateTime(locale, abierta.consent_at) })
                  : t('forms.consent_none')}
                {abierta.language ? ` · ${abierta.language.toUpperCase()}` : ''}
              </p>
              {abierta.consent_text ? (
                <blockquote class="border-border text-text mt-2 border-l-2 pl-3 text-sm">
                  {abierta.consent_text}
                </blockquote>
              ) : null}
            </div>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={aBorrar !== null}
        onClose={() => {
          if (!borrando) setABorrar(null);
        }}
        onConfirm={() => {
          void borrar();
        }}
        title={t('forms.delete_title')}
        confirmLabel={t('forms.delete')}
        tone="danger"
        busy={borrando}
      >
        <p class="text-text text-sm">{t('forms.delete_confirm')}</p>
      </ConfirmDialog>
    </section>
  );
}

export default Bandeja;
