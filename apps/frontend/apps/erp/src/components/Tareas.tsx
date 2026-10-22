/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tareas — lo que toca hacer, por fecha: las vencidas, las de hoy y las que
 * vienen. La más común la deja Sytue AI al apuntar un gasto («sube la factura en
 * PDF») y se resuelve aquí mismo, subiendo el fichero: no hace falta ir a buscar
 * la factura en su lista.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Badge, Button, EmptyState, Icon, Illustration, Spinner, cx, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';
import { api, consulta, mensajeDeError } from '../lib/api.js';
import type { useT } from '../i18n/index.js';
import { ACEPTA_DOCUMENTOS, hoyISO, rotuloTipo, subirDocumento } from './Seguimiento.js';
import type { Actividad } from './tipos.js';

type Traducir = ReturnType<typeof useT>;

export function Tareas({
  locale,
  t,
  onAbrir,
  onCambio,
}: {
  locale: Locale;
  t: Traducir;
  /** Abre el registro del que cuelga una tarea. */
  onAbrir: (modelo: string, id: string) => void;
  /** Algo cambió (para el contador de la barra lateral). */
  onCambio: () => void;
}): JSX.Element {
  const aviso = useToast();
  const [mias, setMias] = useState(true);
  const [lista, setLista] = useState<Actividad[] | null>(null);
  const [ocupada, setOcupada] = useState('');
  const selector = useRef<HTMLInputElement>(null);
  const paraSubir = useRef<Actividad | null>(null);

  const cargar = useCallback(() => {
    api<{ actividades: Actividad[] }>(
      `actividades${consulta({ estado: 'pendiente', mias: mias ? '1' : '' })}`,
    )
      .then((r) => setLista(r.actividades ?? []))
      .catch(() => setLista([]));
  }, [mias]);
  useEffect(cargar, [cargar]);

  async function cerrar(a: Actividad, como: 'hecha' | 'cancelar'): Promise<void> {
    setOcupada(a.id);
    try {
      await api(`actividades/${a.id}/${como}`, { method: 'POST', body: '{}' });
      onCambio();
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setOcupada('');
      cargar();
    }
  }

  async function subir(f: File): Promise<void> {
    const a = paraSubir.current;
    if (!a) return;
    setOcupada(a.id);
    try {
      await subirDocumento(a.modelo, a.registro, f);
      aviso.notify({ tone: 'success', title: t('docs.uploaded'), description: t('docs.taskDone') });
      onCambio();
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setOcupada('');
      cargar();
    }
  }

  const hoy = hoyISO();
  const grupos = [
    {
      clave: 'vencidas',
      titulo: t('tasks.overdue'),
      filas: (lista ?? []).filter((a) => a.vence < hoy),
    },
    { clave: 'hoy', titulo: t('tasks.today'), filas: (lista ?? []).filter((a) => a.vence === hoy) },
    {
      clave: 'proximas',
      titulo: t('tasks.upcoming'),
      filas: (lista ?? []).filter((a) => a.vence > hoy),
    },
  ].filter((g) => g.filas.length > 0);

  return (
    <div class="mx-auto max-w-5xl space-y-6 px-6 py-6">
      <header class="border-border flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <h1 class="text-text text-lg font-semibold tracking-tight">{t('tasks.title')}</h1>
          <p class="text-text-muted mt-1 max-w-prose text-sm">{t('tasks.subtitle')}</p>
        </div>
        <div
          class="border-border inline-flex shrink-0 items-center rounded-md border"
          role="group"
          aria-label={t('tasks.title')}
        >
          {[
            { valor: true, rotulo: t('tasks.mine') },
            { valor: false, rotulo: t('tasks.all') },
          ].map(({ valor, rotulo }) => (
            <button
              key={String(valor)}
              type="button"
              aria-pressed={mias === valor}
              onClick={() => setMias(valor)}
              class={cx(
                'sytue-focus-ring px-3 py-1 text-sm',
                mias === valor
                  ? 'bg-selected text-text font-semibold'
                  : 'text-text-muted hover:text-text',
              )}
            >
              {rotulo}
            </button>
          ))}
        </div>
      </header>

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

      {lista === null ? (
        <div class="flex justify-center py-12">
          <Spinner size={24} />
        </div>
      ) : grupos.length === 0 ? (
        <EmptyState
          title={t('tasks.allDone')}
          description={t('tasks.allDoneHint')}
          illustration={<Illustration name="emptyCalendar" size={96} />}
        />
      ) : (
        grupos.map((g) => (
          <section key={g.clave} aria-labelledby={`tareas-${g.clave}`} class="space-y-2">
            <h2 id={`tareas-${g.clave}`} class="sytue-overline text-text-subtle">
              {g.titulo}
            </h2>
            <ul class="border-border divide-border rounded-card divide-y border">
              {g.filas.map((a) => (
                <li
                  key={a.id}
                  class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div class="min-w-0">
                    <p class="text-text text-sm font-medium">{a.resumen}</p>
                    <p class="text-text-subtle text-xs">
                      {rotuloTipo(a.tipo, t)}
                      {a.registro && a.modelo && a.modelo !== 'obligacion' ? (
                        <>
                          {' · '}
                          <button
                            type="button"
                            onClick={() => onAbrir(a.modelo, a.registro)}
                            class="sytue-focus-ring hover:text-text rounded-sm underline"
                          >
                            {a.registro_etiqueta || t('tasks.open')}
                          </button>
                        </>
                      ) : null}
                      {!mias && a.responsable_nombre ? ` · ${a.responsable_nombre}` : ''}
                    </p>
                  </div>
                  <div class="flex shrink-0 flex-wrap items-center gap-1.5">
                    <Badge
                      tone={a.vence < hoy ? 'danger' : a.vence === hoy ? 'warning' : 'neutral'}
                    >
                      {formatDate(locale, new Date(`${a.vence}T00:00:00`), {
                        day: 'numeric',
                        month: 'short',
                      })}
                    </Badge>
                    {a.tipo === 'subir_documento' && a.registro ? (
                      <Button
                        size="xs"
                        variant="primary"
                        loading={ocupada === a.id}
                        onClick={() => {
                          paraSubir.current = a;
                          selector.current?.click();
                        }}
                      >
                        <Icon name="upload" size={12} class="mr-1" />
                        {t('docs.upload')}
                      </Button>
                    ) : null}
                    <Button
                      size="xs"
                      variant="secondary"
                      loading={ocupada === a.id}
                      onClick={() => void cerrar(a, 'hecha')}
                    >
                      <Icon name="check" size={12} class="mr-1" />
                      {t('tasks.done')}
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      disabled={ocupada === a.id}
                      onClick={() => void cerrar(a, 'cancelar')}
                    >
                      {t('tasks.cancel')}
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          </section>
        ))
      )}
    </div>
  );
}
