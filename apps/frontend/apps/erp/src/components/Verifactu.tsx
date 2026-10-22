/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VERI*FACTU en la interfaz, para quien no sabe de impuestos: en las facturas de
 * cliente, una tarjeta que dice qué es, lo activa y cuenta cómo van los envíos a
 * la Agencia Tributaria; en cada factura, qué contestó la Agencia y un botón para
 * volver a enviarla cuando algo estaba mal.
 *
 * Todo lo que decide (qué se registra, qué se puede cambiar, cuándo se envía) lo
 * decide el servicio: aquí solo se enseña y se pide.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, Icon, useConfirm, useToast, withBase } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';
import { api, baseDeLaApp, mensajeDeError } from '../lib/api.js';
import type { ErpMessageKey, useT } from '../i18n/index.js';
import type { EstadoVerifactu, MotivoVerifactu, RegistroVerifactu } from './tipos.js';

type Traducir = ReturnType<typeof useT>;

const MOTIVOS: Record<MotivoVerifactu, ErpMessageKey> = {
  sin_perfil: 'vf.reason.profile',
  pais: 'vf.reason.country',
  foral: 'vf.reason.foral',
  igic_ipsi: 'vf.reason.igic',
  nif: 'vf.reason.taxId',
  sin_certificado: 'vf.reason.certificate',
  declaracion: 'vf.reason.declaration',
};

const ESTADOS: Record<
  RegistroVerifactu['estado'],
  { rotulo: ErpMessageKey; tono: 'info' | 'success' | 'warning' | 'danger' }
> = {
  pendiente: { rotulo: 'vf.state.pending', tono: 'info' },
  correcto: { rotulo: 'vf.state.accepted', tono: 'success' },
  aceptado_con_errores: { rotulo: 'vf.state.acceptedWithErrors', tono: 'warning' },
  incorrecto: { rotulo: 'vf.state.rejected', tono: 'danger' },
};

const enlace = 'sytue-focus-ring text-text-muted hover:text-text rounded-sm text-xs underline';

/** La tarjeta de las facturas de cliente de una organización española. */
export function TarjetaVerifactu({
  administrador,
  locale,
  revision,
  t,
}: {
  /** Quien administra la organización es quien lo activa. */
  administrador: boolean;
  locale: Locale;
  /** Sube con cada escritura en la lista: los contadores se vuelven a pedir. */
  revision: number;
  t: Traducir;
}): JSX.Element | null {
  const [estado, setEstado] = useState<EstadoVerifactu | null>(null);
  const [activando, setActivando] = useState(false);
  const { confirmar, dialogo } = useConfirm();
  const aviso = useToast();

  useEffect(() => {
    let vivo = true;
    api<EstadoVerifactu>('verifactu')
      .then((e) => vivo && setEstado(e))
      // Sin estado no se enseña la tarjeta: las facturas se siguen pudiendo usar.
      .catch(() => undefined);
    return () => {
      vivo = false;
    };
  }, [revision]);

  if (!estado) return null;

  async function activar(): Promise<void> {
    const seguro = await confirmar({
      titulo: t('vf.confirmTitle'),
      mensaje: t('vf.confirmBody'),
      confirmLabel: t('vf.activate'),
    });
    if (!seguro) return;
    setActivando(true);
    try {
      setEstado(await api<EstadoVerifactu>('verifactu/activar', { method: 'POST' }));
      aviso.notify({ tone: 'success', title: t('vf.activated') });
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setActivando(false);
    }
  }

  const enCola = estado.pendientes + estado.con_errores + estado.rechazadas;
  return (
    <section class="border-border bg-surface rounded-card mx-4 mt-4 border px-4 py-3 md:mx-6">
      <div class="flex flex-wrap items-center gap-2">
        <Icon name="shield" size={16} />
        <h2 class="text-text text-sm font-semibold">VERI*FACTU</h2>
        {estado.activo && estado.desde ? (
          <Badge tone="success">
            {t('vf.activeSince', { date: formatDate(locale, estado.desde) })}
          </Badge>
        ) : null}
        {estado.entorno === 'pruebas' && (estado.activo || !estado.motivo) ? (
          <Badge tone="warning">{t('vf.testEnvironment')}</Badge>
        ) : null}
        <span class="flex-1" />
        <a
          href={withBase(baseDeLaApp(), '/api/verifactu/declaracion')}
          target="_blank"
          rel="noopener"
          class={enlace}
        >
          {t('vf.declaration')}
        </a>
        {estado.activo ? (
          <a href={withBase(baseDeLaApp(), '/api/verifactu/exportar')} class={enlace}>
            {t('vf.export')}
          </a>
        ) : null}
      </div>
      {estado.activo ? (
        <p class="text-text-muted mt-1 flex flex-wrap gap-x-3 text-xs">
          {enCola === 0 ? <span>{t('vf.allSent')}</span> : null}
          {estado.pendientes > 0 ? (
            <span>{t('vf.pending', { count: estado.pendientes })}</span>
          ) : null}
          {estado.con_errores > 0 ? (
            <span class="text-warning-text">
              {t('vf.withErrors', { count: estado.con_errores })}
            </span>
          ) : null}
          {estado.rechazadas > 0 ? (
            <span class="text-danger-text">{t('vf.rejected', { count: estado.rechazadas })}</span>
          ) : null}
          {estado.error_envio ? (
            <span class="text-danger-text">{t('vf.sendError', { error: estado.error_envio })}</span>
          ) : null}
        </p>
      ) : (
        <div class="mt-1 space-y-2">
          <p class="text-text-muted max-w-prose text-xs leading-relaxed">{t('vf.intro')}</p>
          {estado.motivo ? (
            <p class="text-text-subtle text-xs">{t(MOTIVOS[estado.motivo])}</p>
          ) : administrador ? (
            <Button size="xs" loading={activando} onClick={() => void activar()}>
              {t('vf.activate')}
            </Button>
          ) : (
            <p class="text-text-subtle text-xs">{t('vf.adminOnly')}</p>
          )}
        </div>
      )}
      {dialogo}
    </section>
  );
}

/** Lo que contestó la Agencia Tributaria a una factura, y volver a enviarla. */
export function EstadoVerifactuFactura({
  factura,
  estadoFactura,
  soloLectura,
  t,
}: {
  factura: string;
  /** El estado de la factura: al confirmarla o anularla aparece su registro. */
  estadoFactura: string;
  soloLectura: boolean;
  t: Traducir;
}): JSX.Element | null {
  const [datos, setDatos] = useState<{ registros: RegistroVerifactu[]; qr: string } | null>(null);
  const [enviando, setEnviando] = useState(false);
  const aviso = useToast();

  const cargar = useCallback(() => {
    api<{ registros: RegistroVerifactu[]; qr: string }>(`factura_venta/${factura}/verifactu`)
      .then(setDatos)
      .catch(() => setDatos(null));
  }, [factura]);
  // `estadoFactura` no se lee dentro: está para volver a pedirlo al cambiar.
  useEffect(cargar, [cargar, estadoFactura]);

  const ultimo = datos?.registros.at(-1);
  if (!ultimo) return null;

  async function reenviar(): Promise<void> {
    setEnviando(true);
    try {
      setDatos(
        await api<{ registros: RegistroVerifactu[]; qr: string }>(
          `factura_venta/${factura}/verifactu/subsanar`,
          { method: 'POST' },
        ),
      );
      aviso.notify({ tone: 'success', title: t('vf.resent') });
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setEnviando(false);
    }
  }

  const estado = ESTADOS[ultimo.estado];
  const corregible =
    !soloLectura && (ultimo.estado === 'incorrecto' || ultimo.estado === 'aceptado_con_errores');
  return (
    <section class="space-y-2">
      <h3 class="text-text-subtle text-xs font-semibold tracking-wide uppercase">VERI*FACTU</h3>
      <div class="flex flex-wrap items-center gap-2">
        <Badge tone={estado.tono}>
          {ultimo.tipo === 'anulacion' ? `${t('vf.cancellation')} · ` : ''}
          {t(estado.rotulo)}
        </Badge>
        {ultimo.csv ? (
          <span class="text-text-subtle font-mono text-xs">CSV {ultimo.csv}</span>
        ) : null}
        {datos?.qr ? (
          <a href={datos.qr} target="_blank" rel="noopener noreferrer" class={enlace}>
            {t('vf.check')}
          </a>
        ) : null}
      </div>
      {ultimo.descripcion_error ? (
        <p class="text-text-muted text-xs">
          {ultimo.codigo_error ? `${ultimo.codigo_error} · ` : ''}
          {ultimo.descripcion_error}
        </p>
      ) : null}
      {corregible ? (
        <div class="flex items-center gap-2">
          <p class="text-text-subtle flex-1 text-xs">{t('vf.fixHint')}</p>
          <Button size="xs" variant="secondary" loading={enviando} onClick={() => void reenviar()}>
            {t('vf.resend')}
          </Button>
        </div>
      ) : null}
    </section>
  );
}
