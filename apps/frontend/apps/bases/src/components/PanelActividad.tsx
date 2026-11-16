/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La actividad de una base, para su propietario: la traza de auditoría del servicio
 * contada en frases («Ana · Renombró un campo · Importe · Pedidos»), de la más reciente
 * a la más antigua, en el panel lateral del sistema. Se pide por páginas: «Cargar más»
 * trae las anteriores a la última que se ve (`before` = su id).
 *
 * Los permisos se piden a la vez para ponerle nombre a quien hizo cada cosa; si fallan,
 * la actividad se ve igual y las personas que no son quien mira salen como «Otra
 * persona» (nunca su identificador).
 *
 * También el diálogo para editar la descripción de la base.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, FieldGroup, Modal, Skeleton, Textarea, EmptyState } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { BasesAuditEntry, BasesPermission, BasesTable } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { MAX_DESCRIPCION, PAGINA_ACTIVIDAD, contarEntrada } from '../lib/actividad.js';

export function PanelActividad(p: {
  t: T;
  locale: Locale;
  baseId: string;
  nombre: string;
  /** El `sub` de quien mira. */
  yo: string;
  tablas: readonly BasesTable[];
  onCerrar: () => void;
}): JSX.Element {
  const { t } = p;
  const [entradas, setEntradas] = useState<BasesAuditEntry[] | null>(null);
  const [permisos, setPermisos] = useState<BasesPermission[]>([]);
  /** Si la última página vino llena, puede haber más. */
  const [hayMas, setHayMas] = useState(false);
  const [cargando, setCargando] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let vivo = true;
    void (async () => {
      const [es, ps] = await Promise.allSettled([
        cliente().listAudit(p.baseId, { limit: PAGINA_ACTIVIDAD }),
        cliente().listPermissions(p.baseId),
      ]);
      if (!vivo) return;
      if (ps.status === 'fulfilled') setPermisos(ps.value);
      if (es.status === 'fulfilled') {
        setEntradas(es.value);
        setHayMas(es.value.length >= PAGINA_ACTIVIDAD);
      } else {
        setEntradas([]);
        setError(mensajeDeError(es.reason, t));
      }
    })();
    return () => {
      vivo = false;
    };
  }, [p.baseId, t]);

  async function cargarMas(): Promise<void> {
    const ultima = entradas?.at(-1);
    if (!ultima || cargando) return;
    setCargando(true);
    setError('');
    try {
      const mas = await cliente().listAudit(p.baseId, {
        before: ultima.id,
        limit: PAGINA_ACTIVIDAD,
      });
      setEntradas((es) => [...(es ?? []), ...mas]);
      setHayMas(mas.length >= PAGINA_ACTIVIDAD);
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      setCargando(false);
    }
  }

  const ctx = { yo: p.yo, tablas: p.tablas, permisos };
  return (
    <Modal
      open
      placement="right"
      onClose={p.onCerrar}
      title={t('activity.title', { name: p.nombre })}
    >
      <div class="flex flex-col gap-4">
        {entradas === null ? (
          <Skeleton height="6rem" />
        ) : entradas.length === 0 && !error ? (
          <EmptyState compact title={t('activity.empty')} />
        ) : (
          <ol aria-label={t('activity.listLabel')} class="flex flex-col">
            {entradas.map((e) => {
              const c = contarEntrada(e, ctx, t);
              return (
                <li
                  key={c.id}
                  class="border-border-subtle flex flex-col gap-0.5 border-b py-3 last:border-b-0"
                >
                  <span class="text-text text-sm">
                    <span class="font-semibold">{c.quien}</span> · {c.que}
                  </span>
                  {c.sobre ? (
                    <span class="text-text-muted truncate text-sm" title={c.sobre}>
                      {c.sobre}
                    </span>
                  ) : null}
                  <time class="text-text-muted text-xs" dateTime={c.at}>
                    {formatDateTime(p.locale, c.at)}
                  </time>
                </li>
              );
            })}
          </ol>
        )}
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
        {hayMas ? (
          <div>
            <Button variant="secondary" loading={cargando} onClick={() => void cargarMas()}>
              {t('activity.more')}
            </Button>
          </div>
        ) : null}
      </div>
    </Modal>
  );
}

/** Editar la descripción de la base. Vacía es válida: se quita. */
export function DialogoDescripcion(p: {
  t: T;
  valor: string;
  ocupado: boolean;
  error: string;
  onGuardar: (descripcion: string) => void;
  onCerrar: () => void;
}): JSX.Element {
  const [texto, setTexto] = useState(p.valor);
  const cambiada = texto.trim() !== p.valor.trim();
  return (
    <Modal
      open
      onClose={p.onCerrar}
      title={p.t('base.editDescription')}
      footer={
        <>
          <Button variant="ghost" onClick={p.onCerrar}>
            {p.t('common.cancel')}
          </Button>
          <Button
            loading={p.ocupado}
            disabled={!cambiada}
            onClick={() => p.onGuardar(texto.trim())}
          >
            {p.t('common.save')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-2">
        <FieldGroup
          label={p.t('base.descriptionLabel')}
          htmlFor="base-descripcion"
          help={p.t('base.descriptionHelp')}
        >
          <Textarea
            id="base-descripcion"
            aria-describedby="base-descripcion-help"
            rows={4}
            maxLength={MAX_DESCRIPCION}
            value={texto}
            onInput={(e) => setTexto(e.currentTarget.value)}
          />
        </FieldGroup>
        {p.error ? (
          <p role="alert" class="text-danger-text text-sm">
            {p.error}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
