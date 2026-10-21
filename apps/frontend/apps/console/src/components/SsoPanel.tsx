/**
 * SsoPanel — inicio de sesión único (SSO / federación OIDC y SAML) de la organización
 * como página de AJUSTES.
 *
 * Dar de alta el IdP corporativo ya no es un muro de formulario: abre el asistente
 * guiado (ConnectIdpWizard), que explica en llano qué cambia para el equipo, dónde
 * encontrar cada dato en el proveedor, valida al vuelo, prueba la conexión y termina
 * dando los datos que el admin debe pegar en SU proveedor.
 *
 * Aquí quedan la lista de proveedores y su gestión: probar, editar y eliminar. El
 * client_secret solo se ESCRIBE (nunca vuelve al cliente). CRUD vía el BFF, solo
 * @sytue/ui. docs/CONVENTIONS.md §8.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Icon,
  InlineHelp,
  Modal,
  SettingsSection,
  SettingsShell,
  Switch,
  Table,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import ConnectIdpWizard, { CopyRow } from './ConnectIdpWizard.js';
import RegisterOidcAppWizard from './RegisterOidcAppWizard.js';
import { useT } from '../i18n/index.js';
import type { OidcApp, SamlSp, SsoProvider } from '../lib/types.js';

interface SsoPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  providers: SsoProvider[];
  /** Aplicaciones de terceros que entran CON Sytue (Sytue ID como proveedor). */
  apps: OidcApp[];
  /** Apps para las que Sytue ID hace de IdP SAML. Las da de alta el catálogo. */
  samlSps: SamlSp[];
  /**
   * false = la lista de SP no se pudo leer (Sytue ID caído, o un rol delegado con
   * `sso.manage` pero sin `apps.manage`, que es lo que exige ese endpoint). Va
   * aparte de `live` porque una lista vacía y una lista que no se pudo leer se
   * ven igual, y decir «no hay ninguno» cuando no se sabe es mentir.
   */
  samlSpsLive: boolean;
  /** false = sin backend vivo (Sytue ID no accesible); el panel avisa y no edita. */
  live: boolean;
}

/** La dirección que identifica al proveedor, sea OIDC o SAML. */
function providerAddress(p: SsoProvider): string {
  return p.type === 'saml' ? p.idpEntityId || p.idpSsoUrl : p.issuer;
}

function Inner({ locale, providers, apps, samlSps, samlSpsLive, live }: SsoPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<SsoProvider[]>(providers);
  const [wizard, setWizard] = useState<{ open: boolean; provider?: SsoProvider }>({ open: false });
  // Aplicaciones que entran CON Sytue (sentido contrario a los proveedores).
  const [appItems, setAppItems] = useState<OidcApp[]>(apps);
  const [appWizard, setAppWizard] = useState(false);
  const [appBusyId, setAppBusyId] = useState<string | null>(null);
  const [confirmAppDel, setConfirmAppDel] = useState<OidcApp | null>(null);
  /** Secreto recién rotado: se enseña una vez y se olvida al cerrar. */
  const [newSecret, setNewSecret] = useState<{ app: OidcApp; secret: string } | null>(null);
  // SP SAML: mismo patrón que las apps OIDC (apagar sin borrar, borrar con confirmación).
  const [spItems, setSpItems] = useState<SamlSp[]>(samlSps);
  const [spBusyId, setSpBusyId] = useState<string | null>(null);
  const [confirmSpDel, setConfirmSpDel] = useState<SamlSp | null>(null);
  const [spDetails, setSpDetails] = useState<SamlSp | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const [confirmDel, setConfirmDel] = useState<SsoProvider | null>(null);
  const [deleting, setDeleting] = useState(false);

  function upsert(p: SsoProvider) {
    setItems((prev) => {
      const i = prev.findIndex((x) => x.id === p.id);
      if (i < 0) return [p, ...prev];
      const copy = prev.slice();
      copy[i] = p;
      return copy;
    });
  }

  /** Encender/apagar el proveedor sin pasar por el asistente (PATCH solo del estado). */
  async function toggleEnabled(p: SsoProvider) {
    setTogglingId(p.id);
    try {
      const res = await fetch(`/api/admin/sso/providers/${encodeURIComponent(p.id)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ enabled: !p.enabled }),
      });
      if (!res.ok) throw new Error(String(res.status));
      upsert((await res.json()) as SsoProvider);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('common.toggleFailed'),
        description: t('common.toggleFailedHint', { name: p.name }),
      });
    } finally {
      setTogglingId(null);
    }
  }

  async function runTest(p: SsoProvider) {
    if (testingId) return;
    setTestingId(p.id);
    try {
      const res = await fetch(`/api/admin/sso/providers/${encodeURIComponent(p.id)}/test`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error(String(res.status));
      const out = (await res.json()) as { provider: SsoProvider; pkce?: boolean };
      upsert(out.provider);
      toast.notify({
        tone: 'success',
        title: t('sso.toast.testOk'),
        description:
          p.type === 'saml'
            ? t('sso.toast.testOkSaml')
            : t(out.pkce ? 'sso.toast.testOkOidcPkce' : 'sso.toast.testOkOidc'),
      });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('sso.toast.testFailed'),
        description:
          code === '400'
            ? t(p.type === 'saml' ? 'sso.toast.testBadSaml' : 'sso.toast.testBadOidc')
            : t('sso.toast.idpDown'),
      });
    } finally {
      setTestingId(null);
    }
  }

  async function remove(p: SsoProvider) {
    setDeleting(true);
    try {
      const res = await fetch(`/api/admin/sso/providers/${encodeURIComponent(p.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.filter((x) => x.id !== p.id));
      setConfirmDel(null);
      toast.notify({ tone: 'success', title: t('sso.toast.providerDeleted'), description: p.name });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('sso.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setDeleting(false);
    }
  }

  async function rotateAppSecret(app: OidcApp) {
    setAppBusyId(app.clientId);
    try {
      const res = await fetch(
        `/api/admin/apps/oidc-clients/${encodeURIComponent(app.clientId)}/rotate-secret`,
        { method: 'POST', headers: { 'content-type': 'application/json' }, body: '{}' },
      );
      if (!res.ok) throw new Error(String(res.status));
      const updated = (await res.json()) as OidcApp;
      if (updated.secret) setNewSecret({ app: updated, secret: updated.secret });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('sso.toast.rotateFailed'),
        description: t('sso.toast.rotateFailedHint', { name: app.name }),
      });
    } finally {
      setAppBusyId(null);
    }
  }

  async function removeApp(app: OidcApp) {
    setAppBusyId(app.clientId);
    try {
      const res = await fetch(`/api/admin/apps/oidc-clients/${encodeURIComponent(app.clientId)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setAppItems((prev) => prev.filter((x) => x.clientId !== app.clientId));
      setConfirmAppDel(null);
      toast.notify({
        tone: 'success',
        title: t('sso.toast.appDeleted'),
        description: t('sso.toast.appDeletedHint', { name: app.name }),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('sso.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setAppBusyId(null);
    }
  }

  /** Copia al portapapeles avisando del resultado (los datos del IdP se pegan en el SP). */
  function copyValue(value: string) {
    void navigator.clipboard
      .writeText(value)
      .then(() => toast.notify({ tone: 'success', title: t('common.copied') }))
      .catch(() =>
        toast.notify({
          tone: 'danger',
          title: t('sso.secret.copyFailed'),
          description: t('sso.secret.copyFailedHint'),
        }),
      );
  }

  /** Corta o restablece la federación con un SP sin perder su configuración. */
  async function toggleSp(sp: SamlSp) {
    setSpBusyId(sp.id);
    try {
      const res = await fetch(`/api/admin/apps/saml-sps/${encodeURIComponent(sp.id)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ enabled: !sp.enabled }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const updated = (await res.json()) as SamlSp;
      setSpItems((prev) => prev.map((x) => (x.id === sp.id ? updated : x)));
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('sso.saml.toast.toggleFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setSpBusyId(null);
    }
  }

  async function removeSp(sp: SamlSp) {
    setSpBusyId(sp.id);
    try {
      const res = await fetch(`/api/admin/apps/saml-sps/${encodeURIComponent(sp.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setSpItems((prev) => prev.filter((x) => x.id !== sp.id));
      setConfirmSpDel(null);
      toast.notify({ tone: 'success', title: t('sso.saml.toast.deleted') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('sso.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setSpBusyId(null);
    }
  }

  const spColumns: TableColumn<SamlSp>[] = [
    {
      key: 'name',
      header: t('sso.saml.col.app'),
      cell: (sp) => (
        <span class="flex items-center gap-2">
          <Icon name="lock" size={14} class="text-text-subtle shrink-0" />
          <span class="min-w-0">
            <span class="flex items-center gap-2">
              <span class="text-text truncate font-medium">{sp.name}</span>
              {sp.configured ? null : <Badge tone="warning">{t('sso.saml.incomplete')}</Badge>}
            </span>
            <span class="sytue-meta block truncate font-mono">{sp.entityId}</span>
          </span>
        </span>
      ),
    },
    {
      key: 'acs',
      header: t('sso.saml.col.acs'),
      cell: (sp) => (
        <span class="text-text-muted font-mono text-xs break-all">{sp.acsUrl || '—'}</span>
      ),
    },
    {
      key: 'status',
      header: t('sso.col.status'),
      cell: (sp) => (
        <Switch
          label={t(sp.enabled ? 'common.enabled' : 'common.disabled')}
          checked={sp.enabled}
          disabled={!live || spBusyId === sp.id}
          aria-label={t('common.switchAria', {
            state: t(sp.enabled ? 'common.enabled' : 'common.disabled'),
            name: sp.name,
          })}
          onChange={() => void toggleSp(sp)}
        />
      ),
    },
    {
      key: 'actions',
      header: '',
      align: 'end',
      cell: (sp) => (
        <div class="flex justify-end gap-1">
          <Button
            variant="ghost"
            size="sm"
            aria-label={t('sso.saml.detailsAria', { name: sp.name })}
            onClick={() => setSpDetails(sp)}
          >
            {t('sso.saml.details')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={!live || spBusyId !== null}
            aria-label={t('sso.deleteAria', { name: sp.name })}
            onClick={() => setConfirmSpDel(sp)}
          >
            {t('sso.delete')}
          </Button>
        </div>
      ),
    },
  ];

  const appColumns: TableColumn<OidcApp>[] = [
    {
      key: 'name',
      header: t('sso.apps.col.app'),
      cell: (a) => (
        <span class="flex items-center gap-2">
          <Icon name="globe" size={14} class="text-text-subtle shrink-0" />
          <span class="min-w-0">
            <span class="text-text block truncate font-medium">{a.name}</span>
            <span class="sytue-meta block truncate font-mono">{a.clientId}</span>
          </span>
        </span>
      ),
    },
    {
      key: 'kind',
      header: t('sso.apps.col.runsOn'),
      cell: (a) => (
        <span class="text-text-muted text-sm">
          {t(a.type === 'public' ? 'sso.apps.runsPublic' : 'sso.apps.runsConfidential')}
        </span>
      ),
    },
    {
      key: 'redirects',
      header: t('sso.apps.col.redirect'),
      cell: (a) =>
        a.redirectUris.length ? (
          <span class="text-text-muted font-mono text-xs break-all">
            {a.redirectUris[0]}
            {a.redirectUris.length > 1 ? ` +${a.redirectUris.length - 1}` : ''}
          </span>
        ) : (
          <span class="text-text-subtle text-sm">—</span>
        ),
    },
    {
      key: 'actions',
      header: '',
      align: 'end',
      cell: (a) => (
        <div class="flex justify-end gap-1">
          {a.hasSecret ? (
            <Button
              variant="ghost"
              size="sm"
              disabled={!live || appBusyId !== null}
              loading={appBusyId === a.clientId}
              aria-label={t('sso.apps.rotateAria', { name: a.name })}
              onClick={() => void rotateAppSecret(a)}
            >
              {t('sso.apps.rotate')}
            </Button>
          ) : null}
          <Button
            variant="ghost"
            size="sm"
            disabled={!live || appBusyId !== null}
            aria-label={t('sso.deleteAria', { name: a.name })}
            onClick={() => setConfirmAppDel(a)}
          >
            {t('sso.delete')}
          </Button>
        </div>
      ),
    },
  ];

  const columns: TableColumn<SsoProvider>[] = [
    {
      key: 'name',
      header: t('sso.col.provider'),
      cell: (p) => (
        <button
          type="button"
          class="group text-text sytue-focus-ring-inset rounded-control -mx-1 flex items-center gap-2 px-1 py-0.5 text-left font-medium"
          onClick={() => setWizard({ open: true, provider: p })}
        >
          <Icon name="lock" size={14} class="text-text-subtle shrink-0" />
          <span class="min-w-0">
            <span class="flex items-center gap-2">
              <span class="truncate group-hover:underline">{p.name}</span>
              {p.configured ? null : <Badge tone="warning">{t('sso.unverified')}</Badge>}
            </span>
            <span class="sytue-meta block truncate font-mono font-normal">
              {providerAddress(p)}
            </span>
          </span>
        </button>
      ),
    },
    {
      key: 'type',
      header: t('sso.col.connection'),
      cell: (p) => (
        <span class="text-text-muted text-sm">{p.type === 'saml' ? 'SAML 2.0' : 'OIDC'}</span>
      ),
    },
    {
      key: 'domains',
      header: t('sso.col.whoCanSignIn'),
      cell: (p) =>
        p.allowedEmailDomains.length ? (
          <span class="text-text-muted font-mono text-sm">{p.allowedEmailDomains.join(', ')}</span>
        ) : (
          <span class="text-text-subtle text-sm">{t('sso.anyEmail')}</span>
        ),
    },
    {
      key: 'status',
      header: t('sso.col.status'),
      cell: (p) => (
        <Switch
          label={t(p.enabled ? 'common.enabled' : 'common.disabled')}
          checked={p.enabled}
          disabled={!live || togglingId === p.id}
          aria-label={t('common.switchAria', {
            state: t(p.enabled ? 'common.enabled' : 'common.disabled'),
            name: p.name,
          })}
          onChange={() => void toggleEnabled(p)}
        />
      ),
    },
    {
      key: 'actions',
      header: '',
      align: 'end',
      cell: (p) => (
        <div class="flex justify-end gap-1">
          <Button
            variant="ghost"
            size="sm"
            disabled={!live || testingId !== null}
            loading={testingId === p.id}
            aria-label={t('sso.testAria', { name: p.name })}
            onClick={() => void runTest(p)}
          >
            {t('sso.test')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={!live}
            leadingIcon={<Icon name="settings" size={14} />}
            aria-label={t('common.editName', { name: p.name })}
            onClick={() => setWizard({ open: true, provider: p })}
          >
            {t('common.edit')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={!live}
            aria-label={t('sso.deleteAria', { name: p.name })}
            onClick={() => setConfirmDel(p)}
          >
            {t('sso.delete')}
          </Button>
        </div>
      ),
    },
  ];

  if (appWizard) {
    return (
      <SettingsShell title={t('sso.apps.register')} description={t('sso.apps.wizardDescription')}>
        <RegisterOidcAppWizard
          t={t}
          onCreated={(app) => setAppItems((prev) => [app, ...prev])}
          onCancel={() => setAppWizard(false)}
          onDone={() => setAppWizard(false)}
        />
      </SettingsShell>
    );
  }

  if (wizard.open) {
    return (
      <SettingsShell
        title={
          wizard.provider
            ? t('common.editName', { name: wizard.provider.name })
            : t('sso.connectProvider')
        }
        description={t('sso.wizardDescription')}
      >
        <ConnectIdpWizard
          t={t}
          editing={wizard.provider}
          takenSlugs={items.map((p) => p.slug)}
          onSaved={upsert}
          onCancel={(discardedId) => {
            if (discardedId) setItems((prev) => prev.filter((x) => x.id !== discardedId));
            setWizard({ open: false });
          }}
          onDone={() => setWizard({ open: false })}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell
      title={t('page.sso')}
      description={t('sso.subtitle')}
      actions={
        <Button
          variant="primary"
          disabled={!live}
          leadingIcon={<Icon name="plus" size={16} />}
          onClick={() => setWizard({ open: true })}
        >
          {t('sso.connectProvider')}
        </Button>
      }
      banner={
        live ? undefined : (
          <InlineHelp tone="warning" title={t('common.offlineIdTitle')}>
            {t('sso.offlineBody')}
          </InlineHelp>
        )
      }
    >
      <SettingsSection
        plain
        title={t('sso.providers.title')}
        description={t('sso.providers.description')}
      >
        {items.length === 0 ? (
          <Card>
            <EmptyState
              title={t('sso.providers.emptyTitle')}
              description={t('sso.providers.emptyDescription')}
              illustration={<Icon name="lock" size={28} />}
              action={
                <Button
                  variant="primary"
                  disabled={!live}
                  onClick={() => setWizard({ open: true })}
                >
                  {t('sso.providers.emptyAction')}
                </Button>
              }
            />
          </Card>
        ) : (
          <Table
            columns={columns}
            rows={items}
            rowKey={(p) => p.id}
            caption={t('sso.providers.caption')}
          />
        )}
      </SettingsSection>

      <SettingsSection
        plain
        title={t('sso.apps.title')}
        description={t('sso.apps.description')}
        actions={
          appItems.length > 0 ? (
            <Button
              variant="secondary"
              size="sm"
              disabled={!live}
              leadingIcon={<Icon name="plus" size={14} />}
              onClick={() => setAppWizard(true)}
            >
              {t('sso.apps.register')}
            </Button>
          ) : null
        }
      >
        {appItems.length === 0 ? (
          <Card>
            <EmptyState
              title={t('sso.apps.emptyTitle')}
              description={t('sso.apps.emptyDescription')}
              illustration={<Icon name="globe" size={28} />}
              action={
                <Button variant="primary" disabled={!live} onClick={() => setAppWizard(true)}>
                  {t('sso.apps.emptyAction')}
                </Button>
              }
            />
          </Card>
        ) : (
          <Table
            columns={appColumns}
            rows={appItems}
            rowKey={(a) => a.clientId}
            caption={t('sso.apps.caption')}
          />
        )}
      </SettingsSection>

      <SettingsSection plain title={t('sso.saml.title')} description={t('sso.saml.description')}>
        {!samlSpsLive ? (
          <Card>
            <InlineHelp tone="warning" title={t('common.offlineIdTitle')}>
              {t('sso.offlineBody')}
            </InlineHelp>
          </Card>
        ) : spItems.length === 0 ? (
          <Card>
            <EmptyState
              title={t('sso.saml.emptyTitle')}
              description={t('sso.saml.emptyDescription')}
              illustration={<Icon name="lock" size={28} />}
            />
          </Card>
        ) : (
          <Table
            columns={spColumns}
            rows={spItems}
            rowKey={(sp) => sp.id}
            caption={t('sso.saml.caption')}
          />
        )}
      </SettingsSection>

      <Modal
        open={Boolean(spDetails)}
        onClose={() => setSpDetails(null)}
        title={t('sso.saml.details')}
        size="md"
      >
        {spDetails ? (
          <div class="flex flex-col gap-3">
            <p class="text-text-muted text-sm leading-relaxed">
              {t('sso.saml.detailsBody', { name: spDetails.name })}
            </p>
            <CopyRow
              label={t('sso.saml.entityId')}
              value={spDetails.idpEntityId}
              copyLabel={t('common.copy')}
              onCopy={copyValue}
            />
            <CopyRow
              label={t('sso.saml.metadataUrl')}
              value={spDetails.idpMetadataUrl}
              copyLabel={t('common.copy')}
              onCopy={copyValue}
            />
            <CopyRow
              label={t('sso.saml.ssoUrl')}
              value={spDetails.idpSsoUrl}
              copyLabel={t('common.copy')}
              onCopy={copyValue}
            />
            <CopyRow
              label={t('sso.saml.sloUrl')}
              value={spDetails.idpSloUrl}
              copyLabel={t('common.copy')}
              onCopy={copyValue}
            />
            {/* Solo si el SP acepta SSO iniciado por el IdP: si no, esa URL no lleva a
                ninguna parte y enseñarla invita a pegar algo que no funciona. */}
            {spDetails.idpInitiated ? (
              <CopyRow
                label={t('sso.saml.initiateUrl')}
                value={spDetails.initiateUrl}
                copyLabel={t('common.copy')}
                onCopy={copyValue}
              />
            ) : null}
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={Boolean(confirmSpDel)}
        onClose={() => setConfirmSpDel(null)}
        onConfirm={() => {
          if (confirmSpDel) void removeSp(confirmSpDel);
        }}
        title={t('sso.saml.deleteTitle')}
        confirmLabel={t('sso.delete')}
        tone="danger"
        busy={confirmSpDel ? spBusyId === confirmSpDel.id : false}
      >
        {confirmSpDel ? (
          <p class="text-text-muted text-sm leading-relaxed">
            {t('sso.saml.deleteBody', { name: confirmSpDel.name })}
          </p>
        ) : null}
      </ConfirmDialog>

      <Modal
        open={Boolean(newSecret)}
        onClose={() => setNewSecret(null)}
        title={t('sso.secret.title')}
        size="sm"
      >
        {newSecret ? (
          <div class="flex flex-col gap-4">
            <p class="text-text-muted text-sm leading-relaxed">
              {t('sso.secret.body', { name: newSecret.app.name })}
            </p>
            <CopyRow
              label={t('sso.secret.label')}
              value={newSecret.secret}
              copyLabel={t('common.copy')}
              onCopy={(v) => {
                void navigator.clipboard
                  .writeText(v)
                  .then(() => toast.notify({ tone: 'success', title: t('sso.secret.copied') }))
                  .catch(() =>
                    toast.notify({
                      tone: 'danger',
                      title: t('sso.secret.copyFailed'),
                      description: t('sso.secret.copyFailedHint'),
                    }),
                  );
              }}
            />
            <div class="flex justify-end">
              <Button variant="primary" onClick={() => setNewSecret(null)}>
                {t('sso.secret.done')}
              </Button>
            </div>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={Boolean(confirmAppDel)}
        onClose={() => setConfirmAppDel(null)}
        onConfirm={() => {
          if (confirmAppDel) void removeApp(confirmAppDel);
        }}
        title={t('sso.apps.deleteTitle')}
        confirmLabel={t('sso.delete')}
        tone="danger"
        busy={confirmAppDel ? appBusyId === confirmAppDel.clientId : false}
      >
        {confirmAppDel ? (
          <p class="text-text-muted text-sm leading-relaxed">
            {t('sso.apps.deleteBody', { name: confirmAppDel.name })}
          </p>
        ) : null}
      </ConfirmDialog>

      <Modal
        open={Boolean(confirmDel)}
        onClose={() => setConfirmDel(null)}
        title={t('sso.deleteTitle')}
        size="sm"
      >
        {confirmDel ? (
          <div class="flex flex-col gap-4">
            <p class="text-text-muted text-sm leading-relaxed">
              {t('sso.deleteBody', { name: confirmDel.name })}
            </p>
            <div class="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setConfirmDel(null)}>
                {t('common.cancel')}
              </Button>
              <Button variant="danger" loading={deleting} onClick={() => void remove(confirmDel)}>
                {t('sso.delete')}
              </Button>
            </div>
          </div>
        ) : null}
      </Modal>
    </SettingsShell>
  );
}

export default function SsoPanel(props: SsoPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
