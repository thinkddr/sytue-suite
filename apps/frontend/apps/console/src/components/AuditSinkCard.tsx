/**
 * AuditSinkCard — a dónde se ENVÍA la auditoría de esta organización.
 *
 * El servicio empuja los eventos al SIEM del cliente (Splunk HEC, Sentinel,
 * Elastic, Datadog) desde la 0035, firmados y con cursor persistente. Lo que
 * faltaba era la pantalla: había que configurarlo por API, así que en la
 * práctica no lo usaba nadie.
 *
 * Los secretos van SOLO de ida. El servicio nunca los devuelve —solo si HAY uno
 * guardado— y por eso el formulario los deja vacíos: escribirlos otra vez
 * significa cambiarlos, y dejarlos en blanco, conservarlos. Un token del SIEM
 * que se pudiera releer desde el navegador es un token que se filtra en la
 * primera captura de pantalla.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Input,
  Switch,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { AuditSink } from '../lib/admin.js';

type Toast = ReturnType<typeof useToast>;

function Inner({ sink, locale, t }: { sink: AuditSink | null; locale: Locale; t: T }) {
  const toast: Toast = useToast();
  const [url, setUrl] = useState(sink?.url ?? '');
  const [auth, setAuth] = useState('');
  const [secreto, setSecreto] = useState('');
  const [activo, setActivo] = useState(sink?.enabled ?? true);
  const [guardando, setGuardando] = useState(false);
  const [borrando, setBorrando] = useState(false);
  // El servicio exige https y rechaza direcciones internas (cliente SSRF-safe);
  // aquí solo se evita el viaje obvio, sin duplicar esa decisión.
  const invalid = !url.trim().startsWith('https://');

  async function guardar() {
    if (invalid || guardando) return;
    setGuardando(true);
    const res = await fetch('/api/admin/audit-sink', {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        url: url.trim(),
        // Vacíos = conservar. No se mandan como cadena vacía para que el
        // servicio distinga «no lo toques» de «bórralo».
        ...(auth.trim() ? { auth_header: auth.trim() } : {}),
        ...(secreto.trim() ? { signing_secret: secreto.trim() } : {}),
        enabled: activo,
      }),
    });
    setGuardando(false);
    if (!res.ok) {
      toast.notify({ tone: 'danger', title: t('sink.toast.failed') });
      return;
    }
    toast.notify({ tone: 'success', title: t('sink.toast.saved') });
    window.location.reload();
  }

  async function borrar() {
    if (borrando) return;
    setBorrando(true);
    const res = await fetch('/api/admin/audit-sink', { method: 'DELETE' });
    setBorrando(false);
    if (!res.ok) {
      toast.notify({ tone: 'danger', title: t('sink.toast.failed') });
      return;
    }
    toast.notify({ tone: 'success', title: t('sink.toast.removed') });
    window.location.reload();
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('sink.title')}</CardTitle>
        {sink ? (
          <Badge tone={sink.last_error ? 'danger' : sink.enabled ? 'success' : 'neutral'}>
            {sink.last_error
              ? t('sink.state.failing')
              : sink.enabled
                ? t('sink.state.on')
                : t('sink.state.off')}
          </Badge>
        ) : null}
      </CardHeader>
      <CardBody>
        <p class="text-text-muted mb-3 text-sm">{t('sink.intro')}</p>

        {/*
          El último error se enseña TAL CUAL lo devolvió el destino. Traducirlo o
          resumirlo a «error de envío» borraría justo lo que hace falta para
          arreglarlo: casi siempre es un 401 del SIEM o un certificado caducado.
        */}
        {sink?.last_error ? (
          <p class="bg-danger-bg text-danger-text mb-3 rounded-md px-3 py-2 text-sm">
            {sink.last_error}
          </p>
        ) : null}
        {sink?.last_sent_at ? (
          <p class="sytue-meta mb-3">
            {t('sink.lastSent', { when: formatDateTime(locale, sink.last_sent_at) })}
          </p>
        ) : null}

        <div class="flex flex-col gap-3">
          <label class="flex flex-col gap-1 text-sm">
            {t('sink.url')}
            <Input
              type="url"
              value={url}
              placeholder="https://http-inputs-acme.splunkcloud.com/services/collector"
              onInput={(e) => setUrl(e.currentTarget.value)}
            />
          </label>
          <label class="flex flex-col gap-1 text-sm">
            {t('sink.authHeader')}
            <Input
              type="password"
              value={auth}
              autocomplete="off"
              placeholder={sink?.has_auth_header ? t('sink.keepSecret') : 'Splunk <token>'}
              onInput={(e) => setAuth(e.currentTarget.value)}
            />
          </label>
          <label class="flex flex-col gap-1 text-sm">
            {t('sink.signingSecret')}
            <Input
              type="password"
              value={secreto}
              autocomplete="off"
              placeholder={sink?.has_signing_secret ? t('sink.keepSecret') : ''}
              onInput={(e) => setSecreto(e.currentTarget.value)}
            />
            <small class="sytue-meta">{t('sink.signingHint')}</small>
          </label>
          <Switch label={t('sink.enabled')} checked={activo} onChange={() => setActivo(!activo)} />
          <div class="flex flex-wrap gap-2">
            <Button disabled={invalid || guardando} onClick={() => void guardar()}>
              {t('common.save')}
            </Button>
            {sink ? (
              <Button variant="secondary" disabled={borrando} onClick={() => void borrar()}>
                {t('sink.remove')}
              </Button>
            ) : null}
          </div>
        </div>
      </CardBody>
    </Card>
  );
}

export default function AuditSinkCard({
  sink,
  locale,
}: {
  sink: AuditSink | null;
  locale: Locale;
}) {
  const t = useT(locale);
  return (
    <ToastProvider>
      <Inner sink={sink} locale={locale} t={t} />
    </ToastProvider>
  );
}
