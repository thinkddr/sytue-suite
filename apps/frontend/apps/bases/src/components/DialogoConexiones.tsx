/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Conexiones de SOLO LECTURA de una base (solo el propietario): conceder a Sytue Calcus la
 * lectura de una tabla o de una vista, ver quién lee y cuánto, rotar el token y revocar. El
 * token solo se enseña al crearlo o al rotarlo: el servicio guarda solo su huella.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, FieldGroup, Input, Modal, Select, useConfirm } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import {
  BASES_CONSUMERS,
  type BasesConnection,
  type BasesConsumer,
  type BasesTable,
  type BasesView,
} from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';

/** Los nombres de producto son marca: no se traducen. */
const PRODUCTO: Record<BasesConsumer, string> = {
  calcus: 'Sytue Calcus',
  drive: 'Sytue Drive',
  erp: 'SyERP',
  web: 'Sytue Web',
};

function estadoDe(c: BasesConnection): 'revoked' | 'expired' | 'active' {
  if (c.revoked_at) return 'revoked';
  return new Date(c.expires_at).getTime() <= Date.now() ? 'expired' : 'active';
}

export function DialogoConexiones(p: {
  t: T;
  locale: Locale;
  baseId: string;
  tabla: BasesTable;
  /** Las vistas de la tabla activa que pueden acotar una conexión (no los formularios). */
  vistas: BasesView[];
  onCerrar: () => void;
}): JSX.Element {
  const { t } = p;
  const { confirmar, dialogo } = useConfirm();
  const [conexiones, setConexiones] = useState<BasesConnection[]>([]);
  const [nombre, setNombre] = useState('');
  const [vista, setVista] = useState('');
  const [consumidor, setConsumidor] = useState<BasesConsumer>('calcus');
  const [dias, setDias] = useState('90');
  const [token, setToken] = useState('');
  const [copiado, setCopiado] = useState(false);
  const [error, setError] = useState('');

  const cargar = useCallback(async () => {
    try {
      setConexiones(await cliente().listConnections(p.baseId));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }, [p.baseId, t]);
  useEffect(() => {
    void cargar();
  }, [cargar]);

  async function crear(e: Event): Promise<void> {
    e.preventDefault();
    setError('');
    const n = Number.parseInt(dias, 10);
    if (!Number.isInteger(n) || n < 1 || n > 365) {
      setError(t('conn.daysInvalid'));
      return;
    }
    // Solo Calcus lee una tabla entera; el servicio lo vuelve a exigir.
    if (consumidor !== 'calcus' && !vista) {
      setError(t('conn.consumerNeedsView'));
      return;
    }
    try {
      const c = await cliente().createConnection(p.tabla.id, {
        name: nombre.trim(),
        days: n,
        consumer: consumidor,
        ...(vista ? { view_id: vista } : {}),
      });
      setToken(c.token);
      setCopiado(false);
      setNombre('');
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  async function revocar(id: string): Promise<void> {
    try {
      await cliente().revokeConnection(id);
      setToken('');
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  async function rotar(id: string): Promise<void> {
    if (!(await confirmar({ mensaje: t('conn.rotateConfirm'), confirmLabel: t('conn.rotate') })))
      return;
    try {
      const r = await cliente().rotateConnection(id);
      setToken(r.token);
      setCopiado(false);
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  return (
    <Modal open onClose={p.onCerrar} title={t('conn.title')} size="lg">
      <div class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('conn.help')}</p>
        <form class="flex flex-col gap-3" onSubmit={(e) => void crear(e)}>
          <FieldGroup label={t('conn.name')} htmlFor="conn-nombre" required>
            <Input
              id="conn-nombre"
              value={nombre}
              maxLength={120}
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup label={t('conn.consumer')} htmlFor="conn-consumidor">
            <Select
              id="conn-consumidor"
              value={consumidor}
              onChange={(e) => setConsumidor(e.currentTarget.value as BasesConsumer)}
            >
              {BASES_CONSUMERS.map((c) => (
                <option key={c} value={c}>
                  {PRODUCTO[c]}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <div class="flex flex-wrap items-end gap-3">
            <FieldGroup label={t('conn.scope')} htmlFor="conn-alcance" class="min-w-0 flex-1">
              <Select
                id="conn-alcance"
                value={vista}
                onChange={(e) => setVista(e.currentTarget.value)}
              >
                <option value="">{t('conn.scopeTable', { table: p.tabla.name })}</option>
                {p.vistas.map((v) => (
                  <option key={v.id} value={v.id}>
                    {t('conn.scopeView', { view: v.name })}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <FieldGroup label={t('conn.days')} htmlFor="conn-dias">
              <Input
                id="conn-dias"
                inputMode="numeric"
                value={dias}
                onInput={(e) => setDias(e.currentTarget.value)}
              />
            </FieldGroup>
            <Button type="submit" disabled={!nombre.trim()}>
              {t('conn.create')}
            </Button>
          </div>
        </form>
        {token ? (
          <div class="flex flex-col gap-2" role="status">
            <p class="text-text text-sm">{t('conn.once')}</p>
            <div class="flex flex-wrap items-center gap-2">
              <Input
                aria-label={t('conn.token')}
                readOnly
                value={token}
                class="min-w-0 flex-1"
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  void navigator.clipboard?.writeText(token).then(() => setCopiado(true));
                }}
              >
                {copiado ? t('conn.copied') : t('conn.copy')}
              </Button>
            </div>
          </div>
        ) : null}
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
        {conexiones.length === 0 ? (
          <p class="text-text-muted text-sm">{t('conn.none')}</p>
        ) : (
          <ul class="flex flex-col gap-3">
            {conexiones.map((c) => {
              const st = estadoDe(c);
              return (
                <li
                  key={c.id}
                  class="border-border flex flex-wrap items-center justify-between gap-2 border-b pb-3 text-sm last:border-b-0"
                >
                  <span class="flex min-w-0 flex-col gap-1">
                    <span class="text-text flex flex-wrap items-center gap-2 font-semibold">
                      {c.name}
                      <Badge tone="neutral">
                        {PRODUCTO[c.consumer as BasesConsumer] ?? c.consumer}
                      </Badge>
                      <Badge tone={st === 'active' ? 'success' : 'neutral'}>
                        {t(`conn.state.${st}`)}
                      </Badge>
                    </span>
                    <span class="text-text-muted">
                      {t('conn.version', { n: c.version })} ·{' '}
                      {t('conn.expires', { date: formatDate(p.locale, c.expires_at) })} ·{' '}
                      {c.last_read_at
                        ? `${t('conn.reads', { n: c.reads })} · ${t('conn.lastRead', { date: formatDate(p.locale, c.last_read_at) })}`
                        : t('conn.never')}
                    </span>
                  </span>
                  {st === 'active' ? (
                    <span class="flex gap-2">
                      <Button variant="ghost" size="sm" onClick={() => void rotar(c.id)}>
                        {t('conn.rotate')}
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => void revocar(c.id)}>
                        {t('conn.revoke')}
                      </Button>
                    </span>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </div>
      {dialogo}
    </Modal>
  );
}
