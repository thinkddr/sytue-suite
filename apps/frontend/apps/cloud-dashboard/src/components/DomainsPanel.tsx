/**
 * DomainsPanel — dominios del proyecto (estilo Vercel): dominios de plataforma
 * (*.apps.sytue.eu) y dominios propios vía CNAME a ingress.sytue.eu con TLS
 * automático. Mutaciones vía POST /api/domains. Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  Button,
  Badge,
  Icon,
  Input,
  Modal,
  InlineHelp,
  useToast,
  EmptyState,
  ToastProvider,
  SectionHeader,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { Domain } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import DemoBanner from './DemoBanner.js';
import { CopyField } from './CopyField.js';

const tlsTone = { active: 'success', pending: 'warning', error: 'danger' } as const;

/**
 * Los dos registros DNS de un dominio pendiente, en llano y con botón de copiar.
 *
 * Cada uno hace una cosa distinta y por eso van los dos: el TXT PRUEBA que la zona
 * es del cliente (sin él nadie podría impedir que otro reclamase un dominio que ya
 * apunta aquí) y el CNAME manda el tráfico, que es lo que necesita Let's Encrypt
 * para emitir el certificado. Los valores los da el control-plane; aquí no se
 * inventa ninguno.
 */
function DnsInstructions({ t, domain }: { t: T; domain: Domain }) {
  return (
    <InlineHelp tone="warning" title={t('domains.dns.title')} class="mt-3">
      <p class="mb-3">{t('domains.dns.desc')}</p>
      <div class="flex flex-col gap-3">
        {domain.cname ? (
          <CopyField t={t} label={t('domains.dns.cname')} value={domain.cname} />
        ) : null}
        {domain.txtRecord ? (
          <CopyField t={t} label={t('domains.dns.txtName')} value={domain.txtRecord} />
        ) : null}
        {domain.txtValue ? (
          <CopyField t={t} label={t('domains.dns.txtValue')} value={domain.txtValue} />
        ) : null}
      </div>
      <p class="mt-3 text-sm">{t('domains.dns.after')}</p>
    </InlineHelp>
  );
}

function tlsLabel(t: T, tls: Domain['tls']): string {
  switch (tls) {
    case 'active':
      return t('domains.tls.active');
    case 'pending':
      return t('domains.tls.pending');
    case 'error':
      return t('domains.tls.error');
  }
}

interface Props {
  project: string;
  domains: Domain[];
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function DomainsPanel(props: Props) {
  return (
    <ToastProvider>
      <DomainsInner {...props} />
    </ToastProvider>
  );
}

function DomainsInner({ project, domains, live, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<Domain[]>(domains);
  const [host, setHost] = useState('');
  const [busy, setBusy] = useState(false);
  // Borrado destructivo (rompe tráfico/TLS): nunca directo, se gatea con Modal.
  const [pendingDelete, setPendingDelete] = useState<Domain | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function confirmDelete() {
    const target = pendingDelete;
    if (!target?.id) return;
    setDeleting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/domains', {
        project,
        domain_id: target.id,
        delete: true,
      });
      setItems((prev) => prev.filter((d) => d.id !== target.id));
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('domains.deleted'),
        ...demoDesc(t, res.live, t('domains.deleted.desc', { host: target.host })),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('domains.delete.fail') });
    } finally {
      setDeleting(false);
    }
  }

  async function setPrimary(domain: Domain) {
    if (!domain.id) return;
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/domains', {
        project,
        domain_id: domain.id,
        set_primary: true,
      });
      setItems((prev) => prev.map((d) => ({ ...d, primary: d.id === domain.id })));
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('domains.primaryUpdated'),
        ...demoDesc(t, res.live, t('domains.primaryUpdated.desc', { host: domain.host })),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('domains.primary.fail') });
    } finally {
      setBusy(false);
    }
  }

  async function add(e: Event) {
    e.preventDefault();
    const clean = host.trim().toLowerCase();
    if (!clean) return;
    setBusy(true);
    try {
      const res = await postBff<{
        ok: boolean;
        live: boolean;
        domain?: { id?: string; cname?: string; txtRecord?: string; txtValue?: string };
      }>('/api/domains', { project, hostname: clean });
      // Los registros los CALCULA el control-plane (el destino del CNAME sale de su
      // configuración y el TXT lleva el token que acaba de emitir): se pintan tal
      // cual llegan. Sin backend vivo no hay registros y la fila lo refleja: no se
      // inventa un token que no existe, porque copiarlo no verificaría nada.
      const dns = res.domain ?? {};
      setItems((prev) => [
        ...prev,
        {
          host: clean,
          kind: 'custom',
          verified: false,
          tls: 'pending',
          ...(dns.id ? { id: dns.id } : {}),
          ...(dns.cname ? { cname: dns.cname } : {}),
          ...(dns.txtRecord ? { txtRecord: dns.txtRecord } : {}),
          ...(dns.txtValue ? { txtValue: dns.txtValue } : {}),
        },
      ]);
      setHost('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'info',
        title: t('domains.added'),
        // Dos registros y cada uno hace una cosa distinta: el TXT PRUEBA que la zona
        // es tuya (sin él no se verifica, para que nadie reclame un dominio ajeno
        // que ya apunte aquí) y el CNAME manda el tráfico, que es lo que necesita
        // Let's Encrypt para emitir el certificado.
        description: t('domains.added.desc', { host: clean }),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('domains.add.fail'),
        description: t('common.retry'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="globe" size={20} />}
        title={t('nav.domains')}
        subtitle={t('domains.subtitle')}
      />
      <div class="cloud-content">
        <Card class="mb-6">
          <CardBody>
            <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void add(e)}>
              <div class="min-w-0 flex-1">
                <label for="domain-host" class="text-text mb-1 block text-sm font-semibold">
                  {t('domains.addLabel')}
                </label>
                <Input
                  id="domain-host"
                  value={host}
                  required
                  placeholder={t('domains.addPlaceholder')}
                  class="font-mono"
                  onInput={(e) => setHost(e.currentTarget.value)}
                />
              </div>
              <Button
                type="submit"
                variant="primary"
                class="shrink-0"
                loading={busy}
                leadingIcon={<Icon name="plus" size={16} />}
              >
                {t('domains.add')}
              </Button>
            </form>
          </CardBody>
        </Card>

        {items.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                illustration={<Icon name="globe" size={24} />}
                title={t('domains.empty.title')}
                description={t('domains.empty.desc')}
              />
            </CardBody>
          </Card>
        ) : (
          <Card>
            <CardBody class="p-0">
              <ul>
                {items.map((d) => (
                  <li
                    key={d.host}
                    class="border-border hover:bg-bg-subtle border-b px-4 py-3 transition-colors last:border-0"
                  >
                    <div class="flex flex-wrap items-center gap-3">
                      <span
                        class="border-border bg-bg-subtle text-text-subtle grid h-8 w-8 shrink-0 place-items-center rounded-md border"
                        aria-hidden="true"
                      >
                        <Icon name="globe" size={16} />
                      </span>
                      <div class="min-w-0 flex-1">
                        <p class="text-text flex items-center gap-2 font-medium">
                          <span class="truncate" title={d.host}>
                            {d.host}
                          </span>
                          {d.primary ? <Badge tone="primary">{t('domains.primary')}</Badge> : null}
                          {d.kind === 'platform' ? (
                            <Badge tone="neutral">{t('domains.platform')}</Badge>
                          ) : null}
                        </p>
                        {d.cname ? (
                          <p class="text-text-muted mt-0.5 truncate text-sm">
                            CNAME →{' '}
                            <code class="bg-bg-subtle text-text-muted rounded-sm px-1.5 py-0.5 font-mono text-xs">
                              {d.cname}
                            </code>
                          </p>
                        ) : null}
                      </div>
                      <div class="ml-auto flex shrink-0 items-center gap-2">
                        {d.id && d.kind === 'custom' ? (
                          <>
                            {!d.primary ? (
                              <Button
                                variant="ghost"
                                class="text-sm"
                                disabled={busy || deleting}
                                onClick={() => void setPrimary(d)}
                              >
                                {t('domains.setPrimary')}
                              </Button>
                            ) : null}
                            <Button
                              variant="ghost"
                              class="text-danger-text text-sm"
                              disabled={busy || deleting}
                              onClick={() => setPendingDelete(d)}
                            >
                              {t('common.delete')}
                            </Button>
                          </>
                        ) : null}
                        <Badge tone={d.verified ? 'success' : 'warning'}>
                          {d.verified ? t('domains.verified') : t('domains.unverified')}
                        </Badge>
                        <Badge tone={tlsTone[d.tls]}>{tlsLabel(t, d.tls)}</Badge>
                      </div>
                    </div>
                    {/* Mientras esté sin verificar, los registros que hay que crear
                        siguen a la vista: el aviso de alta se va con el toast y sin
                        esto el dominio se quedaba pendiente para siempre. */}
                    {!d.verified && d.kind === 'custom' && (d.txtValue || d.cname) ? (
                      <DnsInstructions t={t} domain={d} />
                    ) : null}
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>
        )}
      </div>

      {/* Confirmación de borrado (acción destructiva: rompe tráfico/TLS). */}
      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('domains.confirm.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)} disabled={deleting}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={deleting} onClick={() => void confirmDelete()}>
              {t('common.delete')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base">
          {t('domains.confirm.body', { host: pendingDelete?.host ?? '' })}
        </p>
      </Modal>
    </>
  );
}
