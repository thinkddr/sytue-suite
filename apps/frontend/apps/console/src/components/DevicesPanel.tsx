/**
 * DevicesPanel — Dispositivos de confianza (paridad Entra device-based Conditional
 * Access). Dos vistas: MIS dispositivos (marcar de confianza el actual, revocar) y los
 * dispositivos de la ORGANIZACIÓN (admin: revocar). Un dispositivo de confianza exime al
 * login de una política de Acceso Condicional con «requiere dispositivo de confianza».
 * El registro/identificación vive en el login de Sytue ID; esto es su plano self-service
 * y admin. Todo vía el BFF (cookie httpOnly, el token nunca está aquí). Solo @sytue/ui,
 * Graphite & Brass, denso, oscuro, WCAG AA.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  SectionHeader,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { Device } from '../lib/types.js';
import { formatWhen } from '../lib/format.js';

interface DevicesPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  myDevices: Device[];
  orgDevices: Device[];
  /** false = Sytue ID no accesible; el panel avisa y no edita. */
  live: boolean;
}

// Confirmación de revocado pendiente: distingue el ámbito (mío / admin) para el endpoint.
interface PendingRevoke {
  device: Device;
  scope: 'me' | 'admin';
}

function trustedBadge(d: Device, t: T) {
  return d.trusted ? (
    <Badge tone="success">{t('devices.trusted')}</Badge>
  ) : (
    <Badge tone="neutral">{t('devices.untrusted')}</Badge>
  );
}

function Inner({ locale, myDevices, orgDevices, live }: DevicesPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [mine, setMine] = useState<Device[]>(myDevices);
  const [org, setOrg] = useState<Device[]>(orgDevices);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [pending, setPending] = useState<PendingRevoke | null>(null);

  async function trust(d: Device) {
    setBusyId(d.id);
    try {
      const res = await fetch(`/api/me/devices/${encodeURIComponent(d.id)}/trust`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error(String(res.status));
      const saved = (await res.json()) as Device;
      setMine((prev) => prev.map((x) => (x.id === d.id ? { ...x, trusted: saved.trusted } : x)));
      setOrg((prev) => prev.map((x) => (x.id === d.id ? { ...x, trusted: saved.trusted } : x)));
      toast.notify({ tone: 'success', title: t('devices.toast.trusted'), description: d.name });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('devices.toast.trustFailed'),
        description:
          code === '400' || code === '403'
            ? t('devices.toast.trustOnlyCurrent')
            : t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function revoke(p: PendingRevoke) {
    setBusyId(p.device.id);
    try {
      const url =
        p.scope === 'admin'
          ? `/api/admin/devices/${encodeURIComponent(p.device.id)}`
          : `/api/me/devices/${encodeURIComponent(p.device.id)}`;
      const res = await fetch(url, { method: 'DELETE' });
      if (!res.ok) throw new Error(String(res.status));
      if (p.scope === 'admin') setOrg((prev) => prev.filter((x) => x.id !== p.device.id));
      else setMine((prev) => prev.filter((x) => x.id !== p.device.id));
      setPending(null);
      toast.notify({
        tone: 'success',
        title: t('devices.toast.revoked'),
        description: p.device.name,
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('devices.toast.revokeFailed'),
        description: p.device.name,
      });
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div>
      <SectionHeader level={1} title={t('page.devices')} subtitle={t('devices.subtitle')} />

      {!live ? <p class="text-warning-text px-4 pt-3 text-xs">{t('devices.offline')}</p> : null}

      {/* --- Mis dispositivos --- */}
      <div class="px-4 py-4">
        <h2 class="text-text mb-2 text-sm font-semibold">{t('devices.mine.title')}</h2>
        {mine.length === 0 ? (
          <EmptyState
            title={t('devices.mine.emptyTitle')}
            description={t('devices.mine.emptyDescription')}
            illustration={<Icon name="lock" size={28} />}
          />
        ) : (
          <div class="border-border bg-surface overflow-hidden rounded-lg border">
            <ul>
              {mine.map((d) => (
                <li
                  key={d.id}
                  class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                >
                  <span class="min-w-0 flex-1">
                    <span class="text-text flex items-center gap-2 font-medium">
                      <span class="truncate" title={d.userAgent}>
                        {d.name}
                      </span>
                      {d.current ? <Badge tone="info">{t('devices.thisDevice')}</Badge> : null}
                    </span>
                    <span class="sytue-meta block truncate">
                      {d.lastIp ? `IP ${d.lastIp} · ` : ''}
                      {t('devices.lastActivity', { when: formatWhen(locale, d.lastSeen) })}
                    </span>
                  </span>
                  <span class="shrink-0">{trustedBadge(d, t)}</span>
                  <span class="flex shrink-0 items-center gap-1">
                    {d.current && !d.trusted ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={!live || busyId === d.id}
                        leadingIcon={<Icon name="check" size={14} />}
                        onClick={() => void trust(d)}
                      >
                        {t('devices.trust')}
                      </Button>
                    ) : null}
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!live || busyId === d.id}
                      aria-label={t('devices.revokeAria', { name: d.name })}
                      onClick={() => setPending({ device: d, scope: 'me' })}
                    >
                      {t('devices.revoke')}
                    </Button>
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {/* --- Dispositivos de la organización (admin) --- */}
      <div class="px-4 pb-4">
        <h2 class="text-text mb-2 text-sm font-semibold">{t('devices.org.title')}</h2>
        {org.length === 0 ? (
          <EmptyState
            title={t('devices.org.emptyTitle')}
            description={t('devices.org.emptyDescription')}
            illustration={<Icon name="users" size={28} />}
          />
        ) : (
          <div class="border-border bg-surface overflow-hidden rounded-lg border">
            <div
              aria-hidden="true"
              class="border-border bg-surface-sunken text-text-muted sytue-overline flex items-center gap-3 border-b px-3 py-2.5"
            >
              <span class="min-w-0 flex-1">{t('devices.col.userDevice')}</span>
              <span class="w-32 shrink-0 text-center">{t('devices.col.trust')}</span>
              <span class="w-24 shrink-0 text-right">{t('devices.col.manage')}</span>
            </div>
            <ul>
              {org.map((d) => (
                <li
                  key={d.id}
                  class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                >
                  <span class="min-w-0 flex-1">
                    <span class="text-text block truncate font-medium">
                      {d.userEmail || d.userName || d.userId || t('devices.fallbackUser')}
                    </span>
                    <span class="sytue-meta block truncate" title={d.userAgent}>
                      {d.name} ·{' '}
                      {t('devices.lastActivity', { when: formatWhen(locale, d.lastSeen) })}
                    </span>
                  </span>
                  <span class="w-32 shrink-0 text-center">{trustedBadge(d, t)}</span>
                  <span class="flex w-24 shrink-0 items-center justify-end">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!live || busyId === d.id}
                      aria-label={t('devices.revokeOfAria', {
                        user: d.userEmail || d.userName || t('devices.fallbackUser'),
                      })}
                      onClick={() => setPending({ device: d, scope: 'admin' })}
                    >
                      {t('devices.revoke')}
                    </Button>
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {/* Confirmar revocado */}
      <ConfirmDialog
        open={Boolean(pending)}
        onClose={() => setPending(null)}
        onConfirm={() => {
          if (pending) void revoke(pending);
        }}
        title={t('devices.revokeTitle')}
        confirmLabel={t('devices.revoke')}
        tone="danger"
        busy={pending ? busyId === pending.device.id : false}
      >
        {pending ? (
          <p class="text-text-muted text-sm">
            {t('devices.revokeConfirm', { name: pending.device.name })}
          </p>
        ) : null}
      </ConfirmDialog>
    </div>
  );
}

export default function DevicesPanel(props: DevicesPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
