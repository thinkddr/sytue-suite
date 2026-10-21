/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bandeja de formularios de Sytue Pages (PAGES-PLAN §6.3, §10.6).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, EmptyState, Skeleton, Table, type TableColumn } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { ruta } from '../lib/rutas.js';

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
}

interface FormSubmission {
  id: string;
  site_id: string;
  form_id: string;
  data: Record<string, unknown>;
  language: string;
  consent_text?: string;
  consent_at?: string;
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
      void cargarEnvios(formIdActual);
    }
  }, [formIdActual, cargarEnvios]);

  const agregarAvisado = () => {
    const correo = nuevoCorreo.trim().toLowerCase();
    if (!correo) return;
    if (!correo.includes('@') || !correo.includes('.')) {
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
      setErrorAvisos('Error al guardar');
    } finally {
      setGuardandoAvisos(false);
    }
  };

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

  // Columnas de la tabla para el formulario actual
  const columnas: TableColumn<FormSubmission>[] = [
    {
      key: 'created_at',
      header: t('forms.col_date'),
      cell: (s) => (
        <span class="text-text-muted text-sm whitespace-nowrap">
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

  function formatearValor(val: unknown): string {
    if (val === undefined || val === null || val === '') {
      return '—';
    }
    if (typeof val === 'string') {
      return val;
    }
    if (typeof val === 'number') {
      return String(val);
    }
    if (typeof val === 'boolean') {
      return val ? 'Sí' : 'No';
    }
    return JSON.stringify(val);
  }

  // Añadir columnas de los campos del formulario
  if (formularioActual?.fields) {
    for (const c of formularioActual.fields) {
      const etiqueta = c.label?.[locale] ?? c.label?.es ?? c.label?.en ?? c.id;
      columnas.push({
        key: `campo_${c.id}`,
        header: etiqueta,
        cell: (s: FormSubmission) => {
          const val = s.data[c.id];
          return <span class="block max-w-xs truncate">{formatearValor(val)}</span>;
        },
      });
    }
  }

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
          </p>
        </div>
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

      {/* Selector de formulario si hay más de uno */}
      {formularios.length > 1 ? (
        <div class="border-border flex flex-wrap gap-2 border-b pb-3">
          {formularios.map((f) => {
            const activo = f.form_id === formIdActual;
            return (
              <button
                key={f.form_id}
                type="button"
                onClick={() => setFormIdActual(f.form_id)}
                class={`inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors pointer-coarse:min-h-[max(44px,2.75rem)] ${
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
                {!f.live ? <Badge tone="warning">{t('forms.not_live')}</Badge> : null}
              </button>
            );
          })}
        </div>
      ) : null}

      {/* Tabla de respuestas */}
      {cargandoEnvios ? (
        <div class="space-y-2">
          <Skeleton class="h-10 w-full" />
          <Skeleton class="h-10 w-full" />
          <Skeleton class="h-10 w-full" />
        </div>
      ) : envios.length > 0 ? (
        <div class="border-border bg-surface overflow-x-auto rounded-xl border">
          <Table<FormSubmission>
            caption={formularioActual?.name || formularioActual?.form_id || t('forms.title')}
            columns={columnas}
            rows={envios}
            rowKey={(s: FormSubmission): string => s.id}
          />
        </div>
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
                  class="text-text-muted hover:text-danger inline-flex items-center justify-center p-0.5 pointer-coarse:min-h-[max(44px,2.75rem)]"
                  aria-label={t('forms.notify_remove', { email: correo })}
                >
                  &times;
                </button>
              </span>
            ))}
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <input
              type="email"
              value={nuevoCorreo}
              onInput={(e) => setNuevoCorreo((e.target as HTMLInputElement).value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  agregarAvisado();
                }
              }}
              placeholder="email@example.com"
              class="border-border bg-surface text-text placeholder:text-text-muted focus:border-primary min-w-[240px] rounded-lg border px-3 py-2 text-sm focus:outline-none pointer-coarse:min-h-[max(44px,2.75rem)]"
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
            <p class="text-success text-sm font-medium">{t('forms.notify_saved')}</p>
          ) : null}
          {errorAvisos ? <p class="text-danger text-sm font-medium">{errorAvisos}</p> : null}
        </div>
      ) : null}
    </section>
  );
}

export default Bandeja;
