/**
 * AppsPanel — Integraciones (Sytue Integration Gateway) como página de AJUSTES.
 *
 * Dos zonas: lo que ya está conectado (abrir, probar, sincronizar, desactivar) y el
 * catálogo de lo que puedes conectar. Dar de alta un SaaS ya no es un modal con
 * campos sueltos: abre el asistente guiado (ConnectAppWizard), que explica qué se
 * activa, dónde conseguir cada dato, prueba la conexión y resume antes de aplicar.
 *
 * El secreto solo se ESCRIBE (nunca vuelve). Solo @sytue/ui. Graphite & Brass.
 */
import { useState, useEffect } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  InlineHelp,
  Input,
  Modal,
  SettingsSection,
  SettingsShell,
  Switch,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import { formatDateTime, formatList, formatNumber, type Locale } from '@sytue/i18n';
import AppMap from './AppMap.js';
import AccesoConexion from './AccesoConexion.js';
import ConnectAppWizard, { capabilityLine, conectablesDesdeAdmin } from './ConnectAppWizard.js';
import { useT, type T } from '../i18n/index.js';
import { useTAcceso } from '../i18n/acceso/index.js';
import type { ConsoleMessageKey } from '../i18n/index.js';
import type { CatalogApp, ConnectedInstance, ProductChoice, SyncRun } from '../lib/gateway.js';
import type { DCRToken } from '../lib/admin.js';

interface AppsPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  catalog: CatalogApp[];
  instances: ConnectedInstance[];
  /** false = gateway no accesible; el panel avisa y no edita. */
  live: boolean;
  /** Estado inicial de «quién sirve cada producto» (SSR); el mapa lo refresca solo. */
  productChoices: ProductChoice[];
}

function statusBadge(status: ConnectedInstance['status'], t: T) {
  switch (status) {
    case 'active':
      return <Badge tone="success">{t('apps.status.active')}</Badge>;
    case 'error':
      return <Badge tone="danger">{t('apps.status.error')}</Badge>;
    case 'disabled':
      return <Badge tone="neutral">{t('apps.status.disabled')}</Badge>;
    default:
      return <Badge tone="warning">{t('apps.status.draft')}</Badge>;
  }
}

// CAPABILITY_KEY traduce la capacidad que devuelve el gateway a lo que significa para
// quien administra. Nada de «provisioning» ni de niveles L1-L5.
const CAPABILITY_KEY: Record<string, ConsoleMessageKey> = {
  provisioning: 'apps.cap.provisioning',
  data: 'apps.cap.data',
};

// capabilityNames extrae de la respuesta de «Probar» las capacidades que respondieron.
// Una conexión que solo hace SSO devuelve la lista vacía, y eso NO es un fallo.
function capabilityNames(payload: unknown, t: T): string[] {
  const results = (payload as { results?: { capability?: string; ok?: boolean }[] } | null)
    ?.results;
  if (!Array.isArray(results)) return [];
  return results
    .filter((r) => r.ok)
    .map((r) => {
      const key = CAPABILITY_KEY[r.capability ?? ''];
      return key ? t(key) : (r.capability ?? '');
    })
    .filter(Boolean);
}

/** Tile de marca de la aplicación (mismo encuadre en catálogo y conectadas). */
function AppIcon({ app, size = 40 }: { app: CatalogApp | undefined; size?: number }) {
  return (
    <span
      aria-hidden="true"
      class="grid shrink-0 place-items-center rounded-lg"
      style={{
        width: `${size}px`,
        height: `${size}px`,
        backgroundColor: app ? `color-mix(in srgb, ${app.color} 12%, transparent)` : undefined,
      }}
    >
      {app?.iconUrl ? (
        <img src={app.iconUrl} alt="" width={size / 2} height={size / 2} />
      ) : (
        <Icon name="globe" size={size / 2} class="text-text-subtle" />
      )}
    </span>
  );
}

// Nombres de marca de las integraciones de serie: no se traducen.
const BUILTIN_NAMES: Record<string, string> = {
  'google-drive': 'Google Drive',
  onedrive: 'OneDrive',
  dropbox: 'Dropbox',
  slack: 'Slack',
  'microsoft-teams': 'Microsoft Teams',
  'google-chat': 'Google Chat',
  'google-meet': 'Google Meet',
  gmail: 'Gmail',
  microsoft: 'Microsoft Outlook',
  yahoo: 'Yahoo Mail',
  imap: 'IMAP',
  pop3: 'POP3',
};

/**
 * Integraciones de serie: cada persona las conecta desde la app con solo el
 * consentimiento. Aquí solo se apagan, una a una; encendidas por defecto.
 */
function BuiltinIntegrations({ t }: { t: T }) {
  const toast = useToast();
  const [items, setItems] = useState<{ provider: string; enabled: boolean }[]>([]);
  useEffect(() => {
    void fetch('/api/admin/apps/integrations')
      .then((res) => (res.ok ? res.json() : null))
      .then((body: { items?: { provider: string; enabled: boolean }[] } | null) =>
        setItems(body?.items ?? []),
      )
      .catch(() => setItems([]));
  }, []);
  async function toggle(provider: string, enabled: boolean) {
    const res = await fetch(
      `/api/admin/apps/integrations?provider=${encodeURIComponent(provider)}`,
      {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ enabled }),
      },
    ).catch(() => null);
    if (!res?.ok) {
      toast.notify({ tone: 'danger', title: t('apps.builtin.failed') });
      return;
    }
    setItems((prev) => prev.map((it) => (it.provider === provider ? { ...it, enabled } : it)));
  }
  if (items.length === 0) return null;
  return (
    <SettingsSection
      plain
      title={t('apps.builtin.title')}
      description={t('apps.builtin.description')}
    >
      <Card>
        <CardBody>
          <ul class="flex flex-col gap-3">
            {items.map((it) => (
              <li key={it.provider}>
                <Switch
                  label={BUILTIN_NAMES[it.provider] ?? it.provider}
                  checked={it.enabled}
                  onChange={(e) => void toggle(it.provider, (e.target as HTMLInputElement).checked)}
                />
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>
    </SettingsSection>
  );
}

function Inner({ locale, catalog, instances, live, productChoices }: AppsPanelProps) {
  const t = useT(locale);
  const tAcceso = useTAcceso(locale);
  const toast = useToast();
  // Conexión cuya lista de «quién consulta» está abierta (NEG-06).
  const [acceso, setAcceso] = useState<ConnectedInstance | null>(null);
  const [items, setItems] = useState<ConnectedInstance[]>(instances);
  const [choices, setChoices] = useState<ProductChoice[]>(productChoices);
  const [wizard, setWizard] = useState<{ open: boolean; app?: CatalogApp }>({ open: false });
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmDel, setConfirmDel] = useState<ConnectedInstance | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<ConnectedInstance | null>(null);
  // Token de registro dinámico. Se guarda en estado porque el servicio lo
  // devuelve UNA vez —solo persiste su hash—, así que si esta pantalla no lo
  // enseña ahora, ya no lo enseña nadie.
  const [tokenDCR, setTokenDCR] = useState<DCRToken | null>(null);
  const [pidiendoToken, setPidiendoToken] = useState(false);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const params = new URLSearchParams(window.location.search);
    const oauth = params.get('oauth');
    const oauthError = params.get('oauth_error');
    const instId = params.get('instance');
    if (oauth === 'connected' || oauth === 'authorized') {
      if (instId) {
        setItems((prev) => prev.map((it) => (it.id === instId ? { ...it, status: 'active' } : it)));
      }
      const instName = instId ? (items.find((x) => x.id === instId)?.name ?? '') : '';
      toast.notify({
        tone: 'success',
        title: t('apps.toast.oauthConnected'),
        ...(instName
          ? { description: t('apps.toast.oauthConnectedHint', { name: instName }) }
          : {}),
      });
      const newUrl = window.location.pathname;
      window.history.replaceState({}, '', newUrl);
    } else if (oauthError) {
      toast.notify({
        tone: 'danger',
        title: t('apps.toast.actionFailed'),
        description: oauthError,
      });
      const newUrl = window.location.pathname;
      window.history.replaceState({}, '', newUrl);
    }
  }, [items, t, toast]);

  async function pedirTokenDCR() {
    if (pidiendoToken) return;
    setPidiendoToken(true);
    try {
      const res = await fetch('/api/admin/apps/dcr-token', { method: 'POST' });
      if (!res.ok) throw new Error(String(res.status));
      setTokenDCR((await res.json()) as DCRToken);
    } catch {
      toast.notify({ tone: 'danger', title: t('apps.dcr.failed') });
    } finally {
      setPidiendoToken(false);
    }
  }

  async function deleteInstance(inst: ConnectedInstance) {
    setBusyId(inst.id);
    try {
      const res = await fetch(`/api/admin/apps/instances/${inst.id}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.filter((x) => x.id !== inst.id));
      setConfirmDelete(null);
      toast.notify({
        tone: 'success',
        title: t('apps.toast.deleted'),
        description: t('apps.toast.deletedHint', { name: inst.name }),
      });
      void onChoicesChanged();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('apps.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }
  const [runs, setRuns] = useState<Record<string, SyncRun | 'loading' | 'error'>>({});

  // Tras cambiar quién sirve un producto se relee el mapa entero en vez de parchear el
  // estado local: activar o desactivar una conexión también lo altera, y reconstruirlo
  // a mano acabaría divergiendo del servidor en algún camino.
  const onChoicesChanged = async (): Promise<void> => {
    try {
      const res = await fetch('/api/admin/apps/substitutions');
      if (!res.ok) return;
      const body = (await res.json()) as { products?: ProductChoice[] };
      if (Array.isArray(body.products)) setChoices(body.products);
    } catch {
      // Un fallo al releer no revierte lo aplicado: el cambio ya está en el servidor y
      // el toast lo ha confirmado. La próxima carga lo mostrará.
    }
  };

  const byKey = new Map(catalog.map((a) => [a.key, a]));
  /** Una conexión viva por plantilla (los borradores no cuentan como conectadas). */
  const connectedList = items.filter((i) => i.status !== 'draft');
  const connectedNames: Record<string, string> = {};
  for (const i of connectedList) connectedNames[i.templateKey] = i.name;
  // Lo que se conecta desde Drive, Rooms o Mail no se ofrece aquí (PLANTILLAS_DE_PRODUCTO).
  const available = conectablesDesdeAdmin(catalog).filter((a) => !connectedNames[a.key]);

  function upsert(inst: ConnectedInstance) {
    setItems((prev) => {
      const i = prev.findIndex((x) => x.id === inst.id);
      if (i < 0) return [inst, ...prev];
      const copy = prev.slice();
      copy[i] = inst;
      return copy;
    });
  }

  async function runAction(inst: ConnectedInstance, action: 'test' | 'sync' | 'deactivate') {
    setBusyId(inst.id);
    try {
      const res = await fetch(`/api/admin/apps/instances/${inst.id}/${action}`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: '{}',
      });
      if (!res.ok) throw new Error(String(res.status));
      const payload: unknown = action === 'test' ? await res.json().catch(() => null) : null;
      if (action === 'deactivate') {
        upsert({ ...inst, status: 'disabled' });
        setConfirmDel(null);
        toast.notify({
          tone: 'success',
          title: t('apps.toast.deactivated'),
          description: t('apps.toast.deactivatedHint', { name: inst.name }),
        });
      } else if (action === 'test') {
        upsert({ ...inst, status: 'active' });
        // La prueba verifica CADA capacidad por separado (aprovisionar, traer datos),
        // porque cada una usa su propio driver y su propia credencial. Se dice cuál ha
        // respondido en vez de un «responde» genérico que no distingue.
        const probadas = capabilityNames(payload, t);
        toast.notify({
          tone: 'success',
          title: t('apps.toast.testOk'),
          description: probadas.length
            ? t('apps.toast.testOkHint', {
                name: inst.name,
                what: formatList(locale, probadas),
              })
            : t('apps.toast.testSsoOnly', { name: inst.name }),
        });
      } else {
        toast.notify({
          tone: 'success',
          title: t('apps.toast.syncStarted'),
          description: t('apps.toast.syncStartedHint'),
        });
      }
    } catch (e) {
      if (action === 'test') upsert({ ...inst, status: 'error' });
      const status = e instanceof Error ? Number(e.message) : 0;
      toast.notify({
        tone: 'danger',
        title: t(action === 'test' ? 'apps.toast.testFailed' : 'apps.toast.actionFailed'),
        description: status === 502 ? t('apps.toast.gatewayDown') : t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function loadRuns(inst: ConnectedInstance) {
    setRuns((r) => ({ ...r, [inst.id]: 'loading' }));
    try {
      const res = (await fetch(`/api/admin/apps/instances/${inst.id}/sync-runs?limit=1`).then(
        (r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))),
      )) as { sync_runs?: SyncRun[] };
      const last = res.sync_runs?.[0];
      setRuns((r) => ({ ...r, [inst.id]: last ?? 'error' }));
    } catch {
      setRuns((r) => ({ ...r, [inst.id]: 'error' }));
    }
  }

  if (wizard.open) {
    return (
      <SettingsShell title={t('apps.connect')} description={t('apps.wizard.description')}>
        <ConnectAppWizard
          t={t}
          locale={locale}
          catalog={catalog}
          connected={connectedNames}
          initialApp={wizard.app}
          onCancel={() => setWizard({ open: false })}
          onConnected={upsert}
          onDone={() => setWizard({ open: false })}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell
      title={t('page.apps')}
      description={t('apps.subtitle')}
      actions={
        <div class="flex flex-wrap gap-2">
          {/*
            Registro dinámico (RFC 7591). Es para que la app se registre SOLA
            —un despliegue automatizado, un cliente que se instala— en vez de
            que un humano rellene el formulario. Va al lado y no dentro del
            asistente porque es el camino alternativo, no un paso suyo.
          */}
          <Button
            variant="secondary"
            disabled={!live || pidiendoToken}
            onClick={() => void pedirTokenDCR()}
          >
            {t('apps.dcr.action')}
          </Button>
          <Button
            variant="primary"
            disabled={!live}
            leadingIcon={<Icon name="plus" size={16} />}
            onClick={() => setWizard({ open: true })}
          >
            {t('apps.connect')}
          </Button>
        </div>
      }
      banner={
        live ? undefined : (
          <InlineHelp tone="warning" title={t('common.offlineGatewayTitle')}>
            {t('apps.offlineBody')}
          </InlineHelp>
        )
      }
    >
      {/* El mapa va PRIMERO: «quién sirve cada producto» es la decisión de más alto
          nivel de esta página. Debajo quedan las conexiones y el catálogo, que son el
          medio para poder tomarla. */}
      {choices.length > 0 ? (
        <SettingsSection plain title={t('apps.map.title')} description={t('apps.map.description')}>
          <Card>
            <CardBody>
              <AppMap
                t={t}
                choices={choices}
                live={live}
                canEdit={live}
                onChanged={() => void onChoicesChanged()}
              />
            </CardBody>
          </Card>
        </SettingsSection>
      ) : null}

      {live ? <BuiltinIntegrations t={t} /> : null}

      <SettingsSection
        plain
        title={t('apps.connected.title')}
        description={t('apps.connected.description')}
      >
        {connectedList.length === 0 ? (
          <Card>
            <EmptyState
              title={t('apps.empty.title')}
              description={t('apps.empty.description')}
              illustration={<Icon name="globe" size={28} />}
              action={
                <Button
                  variant="primary"
                  disabled={!live}
                  onClick={() => setWizard({ open: true })}
                >
                  {t('apps.empty.action')}
                </Button>
              }
            />
          </Card>
        ) : (
          <ul class="grid gap-3 lg:grid-cols-2">
            {connectedList.map((inst) => {
              const app = byKey.get(inst.templateKey);
              const busy = busyId === inst.id;
              const run = runs[inst.id];
              const launchUrl = inst.launchUrl;
              return (
                <li key={inst.id}>
                  <Card class="flex h-full flex-col">
                    <CardBody class="flex flex-1 gap-3">
                      <AppIcon app={app} />
                      <div class="min-w-0 flex-1">
                        <div class="flex flex-wrap items-center gap-2">
                          <h3 class="text-text truncate font-semibold">{inst.name}</h3>
                          {statusBadge(inst.status, t)}
                        </div>
                        <p class="text-text-muted mt-0.5 text-sm leading-snug">
                          {app ? capabilityLine(app, t, locale) : t('apps.configured')}
                        </p>
                        {run === 'loading' ? (
                          <p class="sytue-meta mt-1">{t('common.loading')}</p>
                        ) : run === 'error' ? (
                          <p class="sytue-meta mt-1">{t('apps.runs.none')}</p>
                        ) : run ? (
                          <p class="sytue-meta mt-1">
                            {t('apps.runs.last', {
                              created: formatNumber(locale, run.counts.created),
                              updated: formatNumber(locale, run.counts.updated),
                              deactivated: formatNumber(locale, run.counts.deactivated),
                            })}
                            {run.counts.errors
                              ? ` · ${t('apps.runs.errors', { count: run.counts.errors })}`
                              : ''}
                          </p>
                        ) : null}
                      </div>
                    </CardBody>
                    <div class="border-border flex flex-wrap items-center gap-1 border-t px-3 py-2">
                      {launchUrl ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          leadingIcon={<Icon name="globe" size={14} />}
                          onClick={() => window.open(launchUrl, '_blank', 'noopener,noreferrer')}
                        >
                          {t('apps.open')}
                        </Button>
                      ) : null}
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!live || busy}
                        onClick={() => void runAction(inst, 'test')}
                      >
                        {t('apps.test')}
                      </Button>
                      {app?.hasData ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={!live}
                          onClick={() => setAcceso(inst)}
                        >
                          {tAcceso('acceso.boton')}
                        </Button>
                      ) : null}
                      {app?.provisioning === 'scim' ? (
                        <>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={!live || busy}
                            onClick={() => void runAction(inst, 'sync')}
                          >
                            {t('apps.sync')}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={!live || run === 'loading'}
                            onClick={() => void loadRuns(inst)}
                          >
                            {t('apps.syncStatus')}
                          </Button>
                        </>
                      ) : null}
                      <div class="ml-auto flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={!live || busy}
                          onClick={() => setConfirmDel(inst)}
                        >
                          {t('apps.deactivate')}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={!live || busy}
                          onClick={() => setConfirmDelete(inst)}
                        >
                          {t('apps.delete')}
                        </Button>
                      </div>
                    </div>
                  </Card>
                </li>
              );
            })}
          </ul>
        )}
      </SettingsSection>

      <SettingsSection
        plain
        title={t('apps.catalog.title')}
        description={t('apps.catalog.description')}
      >
        {catalog.length === 0 ? (
          <Card>
            <EmptyState
              title={t('apps.catalog.emptyTitle')}
              description={t('apps.catalog.emptyDescription')}
              illustration={<Icon name="globe" size={28} />}
            />
          </Card>
        ) : available.length === 0 ? (
          <p class="text-text-muted text-sm">{t('apps.catalog.allConnected')}</p>
        ) : (
          <ul class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {available.map((app) => (
              <li key={app.key}>
                <Card class="flex h-full flex-col">
                  <CardBody class="flex flex-1 gap-3">
                    <AppIcon app={app} size={36} />
                    <div class="min-w-0 flex-1">
                      <h3 class="text-text truncate font-semibold">{app.name}</h3>
                      <p class="text-text-muted mt-0.5 text-sm leading-snug">
                        {capabilityLine(app, t, locale)}
                      </p>
                      {app.category ? <p class="sytue-meta mt-1">{app.category}</p> : null}
                    </div>
                  </CardBody>
                  <div class="border-border flex items-center border-t px-3 py-2">
                    <Button
                      variant="secondary"
                      size="sm"
                      class="ml-auto"
                      disabled={!live}
                      onClick={() => setWizard({ open: true, app })}
                    >
                      {t('apps.connectOne')}
                    </Button>
                  </div>
                </Card>
              </li>
            ))}
          </ul>
        )}
      </SettingsSection>

      {acceso ? (
        <AccesoConexion locale={locale} conexion={acceso} onClose={() => setAcceso(null)} />
      ) : null}

      <ConfirmDialog
        open={Boolean(confirmDel)}
        onClose={() => setConfirmDel(null)}
        onConfirm={() => {
          if (confirmDel) void runAction(confirmDel, 'deactivate');
        }}
        title={t('apps.deactivate.title')}
        confirmLabel={t('apps.deactivate')}
        tone="danger"
        busy={confirmDel ? busyId === confirmDel.id : false}
      >
        {confirmDel ? (
          <p class="text-text-muted text-sm leading-relaxed">
            {t('apps.deactivate.body', { name: confirmDel.name })}
          </p>
        ) : null}
      </ConfirmDialog>

      <ConfirmDialog
        open={Boolean(confirmDelete)}
        onClose={() => setConfirmDelete(null)}
        onConfirm={() => {
          if (confirmDelete) void deleteInstance(confirmDelete);
        }}
        title={t('apps.delete.title')}
        confirmLabel={t('apps.delete')}
        tone="danger"
        busy={confirmDelete ? busyId === confirmDelete.id : false}
      >
        {confirmDelete ? (
          <p class="text-text-muted text-sm leading-relaxed">
            {t('apps.delete.body', { name: confirmDelete.name })}
          </p>
        ) : null}
      </ConfirmDialog>

      {/*
        Token de registro dinámico (RFC 7591). Se enseña UNA vez y se dice: el
        servicio guarda solo su hash, así que cerrar esto sin copiarlo significa
        pedir otro — mejor avisarlo antes que dejar que se descubra después.
      */}
      {tokenDCR && (
        <Modal open title={t('apps.dcr.title')} onClose={() => setTokenDCR(null)}>
          <div class="flex flex-col gap-3">
            <InlineHelp tone="warning" title={t('apps.dcr.onceTitle')}>
              {t('apps.dcr.onceBody')}
            </InlineHelp>
            <label class="flex flex-col gap-1 text-sm">
              {t('apps.dcr.token')}
              <Input readOnly value={tokenDCR.initial_access_token} class="font-mono" />
            </label>
            <label class="flex flex-col gap-1 text-sm">
              {t('apps.dcr.endpoint')}
              <Input readOnly value={tokenDCR.registration_endpoint} class="font-mono" />
            </label>
            <p class="sytue-meta">
              {t('apps.dcr.expires', { when: formatDateTime(locale, tokenDCR.expires_at) })}
            </p>
            <div class="flex justify-end gap-2">
              <Button
                variant="secondary"
                onClick={() => {
                  void navigator.clipboard.writeText(tokenDCR.initial_access_token);
                  toast.notify({ tone: 'success', title: t('apps.dcr.copied') });
                }}
              >
                {t('apps.dcr.copy')}
              </Button>
              <Button onClick={() => setTokenDCR(null)}>{t('common.close')}</Button>
            </div>
          </div>
        </Modal>
      )}
    </SettingsShell>
  );
}

export default function AppsPanel(props: AppsPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
