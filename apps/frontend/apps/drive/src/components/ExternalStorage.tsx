/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Añadir almacenamiento»: UNA sola puerta para todo el almacenamiento de terceros.
 *
 *  - Google Drive, OneDrive o Dropbox: un clic y el consentimiento del proveedor. Por
 *    defecto es una nube PERSONAL (PersonalClouds.tsx). Owner/admin puede marcar «Guardar
 *    aquí los archivos nuevos de la organización» y, a la vuelta, ve una sola pantalla:
 *    la cuenta, el aviso de responsabilidad y Guardar.
 *  - WebDAV, S3 y NAS (solo administración y con KEK): un formulario, el aviso y Guardar.
 *    El servicio prueba antes de sellar, así que no hay paso de «comprobar» aparte: si
 *    falla, su motivo sale aquí mismo. El NAS conserva su pantalla de comando y sondeo.
 *
 * El enlace profundo `?connect-storage=1` (tarjeta del portal, Hub nativo y vuelta del
 * consentimiento de organización con `&instance=…`) abre este mismo diálogo.
 */
import { useEffect, useRef, useState } from 'preact/hooks';
import type { ComponentChildren } from 'preact';
import {
  Button,
  Checkbox,
  DataFlow,
  FieldGroup,
  Icon,
  Input,
  Modal,
  ReviewSummary,
  Spinner,
  useSondeo,
  useToast,
  withBase,
} from '@sytue/ui';
import type { IconName } from '@sytue/ui';
import { useT as useCommonT } from '@sytue/i18n/preact';
import { isStorageOAuthKind, STORAGE_PROVIDERS } from '@sytue/api-clients';
import type { StorageOAuthKind } from '@sytue/api-clients';
import type { T } from '../i18n';
import type { CloudProvider } from './PersonalClouds';

const storageUrl = (path: string) => withBase(import.meta.env.BASE_URL ?? '/drive', path);

type ServerKind = 'webdav' | 's3' | 'nas';
type Kind = StorageOAuthKind | ServerKind;
export interface ExternalStorageState {
  connected: boolean;
  available: boolean;
  kind?: string;
  instance_id?: string;
  base_path?: string;
  endpoint?: string;
  bucket?: string;
  region?: string;
  label?: string;
  access_key?: string;
}
interface Account {
  id: string;
  template_key: string;
  name: string;
  connected: boolean;
  oauth_available?: boolean;
}
interface Nas {
  endpoint?: string;
  command?: string;
  online?: boolean;
  reachable?: boolean;
  error?: string;
}
type Description =
  | 'googleDescription'
  | 'onedriveDescription'
  | 'dropboxDescription'
  | 'webdavDescription'
  | 's3Description'
  | 'nasDescription';

export const PARTNER: Record<Kind, string> = {
  gdrive: 'Google Drive',
  onedrive: 'OneDrive',
  dropbox: 'Dropbox',
  webdav: 'WebDAV',
  s3: 'S3',
  nas: 'NAS',
};
/** Cada nube con su nombre en las dos rutas: `kind` de Drive y `provider` personal. */
const CLOUDS: { kind: StorageOAuthKind; cloud: CloudProvider; description: Description }[] = [
  { kind: 'gdrive', cloud: 'google-drive', description: 'googleDescription' },
  { kind: 'onedrive', cloud: 'onedrive', description: 'onedriveDescription' },
  { kind: 'dropbox', cloud: 'dropbox', description: 'dropboxDescription' },
];
const SERVERS: { kind: ServerKind; icon: IconName; description: Description }[] = [
  { kind: 'webdav', icon: 'globe', description: 'webdavDescription' },
  { kind: 's3', icon: 'bucket', description: 's3Description' },
  { kind: 'nas', icon: 'database', description: 'nasDescription' },
];
const EMPTY = {
  endpoint: '',
  base_path: '',
  region: '',
  bucket: '',
  access_key: '',
  secret_key: '',
  label: '',
};

function errorMessage(body: { error?: string | { message?: string } }): string {
  return typeof body.error === 'string' ? body.error : (body.error?.message ?? '');
}

/** Una opción del selector: un clic hace la acción, sin «Continuar». */
function Option({
  icon,
  title,
  description,
  disabled,
  onClick,
  value,
}: {
  icon: IconName;
  title: string;
  description: ComponentChildren;
  disabled: boolean;
  onClick: () => void;
  value: Kind;
}) {
  return (
    <button
      type="button"
      value={value}
      disabled={disabled}
      onClick={onClick}
      class="border-border hover:bg-bg-subtle sytue-focus-ring flex min-h-16 w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60"
    >
      <Icon name={icon} size={20} class="text-text-muted mt-0.5 shrink-0" />
      <span class="min-w-0">
        <span class="text-text block text-sm font-semibold">{title}</span>
        <span class="text-text-muted mt-0.5 block text-sm">{description}</span>
      </span>
    </button>
  );
}

export function StorageDialog({
  t,
  estado,
  personal,
  onPersonal,
  manage = false,
  onClose,
  onChange,
}: {
  t: T;
  /** Almacén de la organización; null si quien mira no lo administra. */
  estado: ExternalStorageState | null;
  /** Nubes personales que esta organización deja conectar. */
  personal: Partial<Record<CloudProvider, boolean>>;
  onPersonal: (provider: CloudProvider) => void;
  /** Abrir sobre el almacén ya conectado (desconectar, o el comando del NAS). */
  manage?: boolean;
  onClose: () => void;
  onChange: () => void;
}) {
  const { notify } = useToast();
  const common = useCommonT();
  const [query] = useState(
    () => new URLSearchParams(typeof location === 'undefined' ? '' : location.search),
  );
  const returned = query.get('connect-storage') === '1' ? query.get('instance') : null;
  const [view, setView] = useState<'pick' | 'confirm' | 'form' | 'nas' | 'manage'>(
    manage ? 'manage' : 'pick',
  );
  const [kind, setKind] = useState<Kind | null>(null);
  const [forOrg, setForOrg] = useState(Boolean(returned));
  const [accounts, setAccounts] = useState<Account[] | null>(null);
  const [orgProviders, setOrgProviders] = useState<Partial<Record<StorageOAuthKind, boolean>>>({});
  const [account, setAccount] = useState<Account | null>(null);
  const [form, setForm] = useState(EMPTY);
  const [accepted, setAccepted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(
    query.get('connect-storage') === '1' && query.has('oauth_error')
      ? t('storage.external.failed')
      : '',
  );
  const [nas, setNas] = useState<Nas | null>(null);
  const [copied, setCopied] = useState(false);
  const busyRef = useRef(false);
  const admin = estado !== null;
  const provider = kind ? PARTNER[kind] : '';

  // Las cuentas de organización ya autorizadas: así no se repite el consentimiento.
  useEffect(() => {
    if (!admin) return;
    let alive = true;
    void fetch(storageUrl('/api/storage-external-instances'), {
      credentials: 'include',
      signal: AbortSignal.timeout(20000),
    })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error('accounts'))))
      .then(
        (body: { items?: Account[]; providers?: Partial<Record<StorageOAuthKind, boolean>> }) => {
          if (!alive) return;
          const items = body.items ?? [];
          setAccounts(items);
          setOrgProviders(body.providers ?? {});
          if (!returned || query.has('oauth_error')) return;
          // Vuelta del consentimiento de organización: directos a la única pantalla que queda.
          const match = items.find((a) => a.id === returned && a.connected && a.oauth_available);
          const cloud = CLOUDS.find(
            (c) => STORAGE_PROVIDERS[c.kind].template === match?.template_key,
          );
          if (match && cloud) {
            setKind(cloud.kind);
            setAccount(match);
            setView('confirm');
          } else setError(t('storage.external.failed'));
        },
      )
      .catch(() => {
        if (alive) setAccounts([]);
      });
    return () => {
      alive = false;
    };
    // Solo al abrir (y si cambia quién administra): la URL de vuelta no cambia después.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [admin]);

  const run = async (task: () => Promise<void>) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError('');
    try {
      await task();
    } catch {
      setError(t('storage.external.failed'));
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };

  const consent = (next: StorageOAuthKind) =>
    run(async () => {
      const r = await fetch(storageUrl('/api/storage-external-consent'), {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ kind: next, new_account: true }),
        signal: AbortSignal.timeout(30000),
      });
      const body = (await r.json().catch(() => ({}))) as { authorize_url?: string; error?: string };
      if (body.error === 'oauth_provider_unavailable') {
        setError(t('storage.external.wizard.providerUnavailable', { provider: PARTNER[next] }));
        return;
      }
      if (!r.ok || !body.authorize_url) throw new Error('consent');
      location.assign(body.authorize_url);
    });

  const pickCloud = (next: (typeof CLOUDS)[number]) => {
    if (!forOrg) {
      onPersonal(next.cloud);
      return;
    }
    const ready = accounts?.find(
      (a) =>
        a.template_key === STORAGE_PROVIDERS[next.kind].template &&
        a.connected &&
        a.oauth_available,
    );
    if (!ready) {
      void consent(next.kind);
      return;
    }
    setKind(next.kind);
    setAccount(ready);
    setAccepted(false);
    setError('');
    setView('confirm');
  };

  const pickServer = (next: ServerKind) => {
    setKind(next);
    setAccount(null);
    setForm(EMPTY);
    setAccepted(false);
    setError('');
    setView('form');
  };

  const back = () => {
    setView('pick');
    setAccount(null);
    setError('');
  };

  const done = (name: string) => {
    notify({
      tone: 'success',
      title: t('storage.external.wizard.doneHeading', { provider: name }),
      description: t('storage.external.wizard.doneDescription'),
    });
    onChange();
    onClose();
  };

  const save = () =>
    run(async () => {
      if (!kind || !accepted) return;
      const isNas = kind === 'nas';
      const r = await fetch(
        storageUrl(isNas ? '/api/storage-external-nas' : '/api/storage-external'),
        {
          method: isNas ? 'POST' : 'PUT',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(
            isNas
              ? {
                  local_endpoint: form.endpoint,
                  user: form.access_key,
                  password: form.secret_key,
                  base_path: form.base_path,
                  label: form.label,
                  accept_terms: true,
                }
              : account
                ? { kind, instance_id: account.id, accept_terms: true }
                : { kind, ...form, accept_terms: true },
          ),
          signal: AbortSignal.timeout(60000),
        },
      );
      const body = (await r.json().catch(() => ({}))) as {
        endpoint?: string;
        command?: string;
        error?: string | { message?: string };
      };
      if (!r.ok) {
        // El servicio ya probó la conexión: su motivo es lo que hay que leer.
        const reason = errorMessage(body);
        setError(
          reason
            ? t('storage.external.checkFail', { error: reason })
            : t('storage.external.failed'),
        );
        return;
      }
      setForm((previous) => ({ ...previous, secret_key: '' }));
      if (!isNas) {
        done(form.label || account?.name || provider);
        return;
      }
      onChange();
      setNas({
        ...(body.endpoint ? { endpoint: body.endpoint } : {}),
        ...(body.command ? { command: body.command } : {}),
        online: false,
        reachable: false,
      });
      setView('nas');
    });

  const pollNas = async () => {
    try {
      const r = await fetch(storageUrl('/api/storage-external-nas'), {
        credentials: 'include',
        signal: AbortSignal.timeout(10000),
      });
      if (!r.ok) return;
      const state = (await r.json()) as Nas;
      setNas((previous) => ({ ...previous, ...state }));
      // Solo se da por conectado lo que se estaba esperando; abrir la gestión de un
      // NAS que ya responde no debe cerrarse solo.
      if (state.reachable) {
        if (view === 'nas') done(form.label || estado?.label || 'NAS');
      } else if (state.command) setView('nas');
    } catch {
      /* El comando sigue a la vista para el siguiente intento. */
    }
  };
  useEffect(() => {
    if (manage && estado?.kind === 'nas') void pollNas();
    // Una sola consulta al abrir la gestión; después sondea useSondeo.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useSondeo(() => void pollNas(), 5000, view === 'nas');

  const disconnect = () =>
    run(async () => {
      const r = await fetch(storageUrl('/api/storage-external'), {
        method: 'DELETE',
        credentials: 'include',
        signal: AbortSignal.timeout(30000),
      });
      if (r.ok) {
        onChange();
        onClose();
      } else
        setError(t(r.status === 409 ? 'storage.external.conflict' : 'storage.external.failed'));
    });

  const field = (
    key: keyof typeof EMPTY,
    label: string,
    options: { required?: boolean; type?: string; placeholder?: string; help?: string } = {},
  ) => (
    <FieldGroup
      htmlFor={`external-${key}`}
      label={label}
      {...(options.required ? { required: true } : {})}
      {...(options.help ? { help: options.help } : {})}
    >
      <Input
        id={`external-${key}`}
        name={key}
        class="min-h-[44px]"
        value={form[key]}
        onInput={(e) => {
          const value = e.currentTarget.value;
          setForm((previous) => ({ ...previous, [key]: value }));
          setError('');
        }}
        type={options.type ?? 'text'}
        required={options.required ?? false}
        {...(options.placeholder ? { placeholder: options.placeholder } : {})}
        {...(options.help ? { 'aria-describedby': `external-${key}-help` } : {})}
        autocomplete={
          key === 'access_key' && kind !== 's3'
            ? 'username'
            : key === 'secret_key'
              ? 'current-password'
              : 'off'
        }
        autocapitalize="off"
        spellcheck={false}
      />
    </FieldGroup>
  );

  const terms = (
    <>
      <DataFlow partner={provider} incoming={['files']} outgoing={['files']} />
      <label class="border-border bg-surface-sunken flex items-start gap-3 rounded-lg border p-4">
        <Checkbox
          name="accept_terms"
          checked={accepted}
          onChange={(e) => setAccepted(e.currentTarget.checked)}
          required
          disabled={busy}
          ringClass="mt-0.5"
        />
        <span class="text-text text-sm leading-relaxed">{t('storage.external.terms')}</span>
      </label>
    </>
  );

  let content: ComponentChildren;
  let footer: ComponentChildren = null;
  if (view === 'manage') {
    content = (
      <div class="flex flex-col gap-4">
        <ReviewSummary
          items={[
            {
              label: t('storage.external.wizard.provider'),
              value: PARTNER[estado?.kind as Kind] || estado?.kind || 'S3',
            },
            {
              label: t('storage.external.wizard.destination'),
              value:
                estado?.label ||
                estado?.bucket ||
                estado?.endpoint ||
                PARTNER[estado?.kind as Kind] ||
                '',
            },
          ]}
        />
        <p class="text-text-muted text-sm">{t('storage.external.wizard.reviewDescription')}</p>
      </div>
    );
    footer = (
      <Button variant="secondary" loading={busy} onClick={() => void disconnect()}>
        {t('storage.external.disconnect')}
      </Button>
    );
  } else if (view === 'nas') {
    content = (
      <div class="flex flex-col gap-4">
        <h3 class="sytue-h3">{t('storage.external.nas.command')}</h3>
        <p class="text-text-muted text-sm">{t('storage.external.nas.commandHelp')}</p>
        <pre class="bg-surface-sunken border-border max-w-full overflow-x-auto rounded-lg border p-4 text-xs select-all">
          {nas?.command}
        </pre>
        <Button
          variant="secondary"
          onClick={() => {
            void navigator.clipboard
              .writeText(nas?.command ?? '')
              .then(() => setCopied(true))
              .catch(() => setError(t('storage.external.wizard.copyFailed')));
          }}
        >
          {t(copied ? 'storage.external.wizard.copied' : 'storage.external.wizard.copy')}
        </Button>
        <p role="status" class="text-text-muted flex items-center gap-2 text-sm">
          <Spinner size={16} />
          {t(nas?.online ? 'storage.external.nas.online' : 'storage.external.nas.waiting')}
        </p>
        {nas?.error ? (
          <p role="alert" class="text-danger-text text-sm">
            {nas.error}
          </p>
        ) : null}
      </div>
    );
    footer = <Button onClick={onClose}>{t('storage.external.close')}</Button>;
  } else if (view === 'confirm' || view === 'form') {
    const saveLabel = t(
      busy
        ? 'storage.external.saving'
        : kind === 'nas'
          ? 'storage.external.nas.create'
          : 'storage.external.save',
    );
    content = (
      <form
        id="external-storage-form"
        method="post"
        aria-busy={busy}
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        <fieldset disabled={busy} class="flex min-w-0 flex-col gap-5">
          <legend class="sytue-h3 mb-1">
            {t('storage.external.wizard.accountHeading', { provider })}
          </legend>
          {view === 'confirm' ? (
            <ReviewSummary
              items={[{ label: t('storage.external.account'), value: account?.name ?? '' }]}
            />
          ) : (
            <>
              {kind === 'nas' ? (
                <p class="text-text-muted text-sm">{t('storage.external.nas.intro')}</p>
              ) : null}
              {field(
                'endpoint',
                t(kind === 'nas' ? 'storage.external.nas.local' : 'storage.external.endpoint'),
                {
                  required: true,
                  ...(kind !== 's3' ? { type: 'url' } : {}),
                  placeholder: t(
                    kind === 'nas'
                      ? 'storage.external.nas.localHint'
                      : kind === 'webdav'
                        ? 'storage.external.endpointHintWebdav'
                        : 'storage.external.endpointHint',
                  ),
                },
              )}
              {kind === 's3'
                ? field('bucket', t('storage.external.bucket'), { required: true })
                : null}
              {field(
                'access_key',
                t(kind === 's3' ? 'storage.external.accessKey' : 'storage.external.user'),
                { required: true },
              )}
              {field(
                'secret_key',
                t(kind === 's3' ? 'storage.external.secretKey' : 'storage.external.password'),
                { required: true, type: 'password', help: t('storage.external.secretHint') },
              )}
              <details class="border-border-subtle border-t pt-4">
                <summary class="sytue-focus-ring text-text-muted cursor-pointer rounded py-1 text-sm">
                  {t('storage.external.wizard.optional')}
                </summary>
                <div class="mt-4 flex flex-col gap-4">
                  {field('label', t('storage.external.label'))}
                  {kind === 's3'
                    ? field('region', t('storage.external.region'))
                    : field('base_path', t('storage.external.basePath'))}
                </div>
              </details>
            </>
          )}
          {terms}
        </fieldset>
      </form>
    );
    footer = (
      <>
        <Button variant="ghost" disabled={busy} onClick={back}>
          {common('ui.wizard.back')}
        </Button>
        {view === 'confirm' && isStorageOAuthKind(kind) ? (
          <Button variant="ghost" disabled={busy} onClick={() => void consent(kind)}>
            {t('storage.external.wizard.addAccount')}
          </Button>
        ) : null}
        <Button type="submit" form="external-storage-form" loading={busy} disabled={!accepted}>
          {saveLabel}
        </Button>
      </>
    );
  } else {
    // El selector también lleva pie, como cualquier diálogo: en el teléfono la lista
    // se desplaza y la salida queda fija abajo, no al final del scroll.
    footer = (
      <Button variant="secondary" onClick={onClose}>
        {t('storage.external.close')}
      </Button>
    );
    const loadingOrg = forOrg && accounts === null;
    content = (
      <div class="flex flex-col gap-3" aria-busy={busy || loadingOrg}>
        <p class="text-text-muted text-sm">{t('storage.external.wizard.chooseDescription')}</p>
        {CLOUDS.map((c) => {
          const enabled = forOrg ? orgProviders[c.kind] === true : personal[c.cloud] === true;
          return (
            <Option
              key={c.kind}
              value={c.kind}
              icon="cloud"
              title={PARTNER[c.kind]}
              disabled={busy || loadingOrg || !enabled}
              onClick={() => pickCloud(c)}
              description={
                enabled || loadingOrg
                  ? t(`storage.external.wizard.${c.description}`)
                  : t('storage.external.wizard.providerUnavailable', { provider: PARTNER[c.kind] })
              }
            />
          );
        })}
        {admin ? (
          <label class="flex items-start gap-3 py-1">
            <Checkbox
              name="for_org"
              checked={forOrg}
              onChange={(e) => setForOrg(e.currentTarget.checked)}
              disabled={busy}
              ringClass="mt-0.5"
            />
            <span class="text-text text-sm">{t('storage.external.forOrg')}</span>
          </label>
        ) : null}
        {estado?.available
          ? SERVERS.map((s) => (
              <Option
                key={s.kind}
                value={s.kind}
                icon={s.icon}
                title={PARTNER[s.kind]}
                disabled={busy}
                onClick={() => pickServer(s.kind)}
                description={t(`storage.external.wizard.${s.description}`)}
              />
            ))
          : null}
      </div>
    );
  }

  return (
    <Modal
      open
      title={(view === 'manage' && PARTNER[estado?.kind as Kind]) || t('storage.add')}
      size="md"
      dismissable={!busy}
      onClose={() => {
        if (!busyRef.current) onClose();
      }}
      footer={footer}
    >
      {content}
      {error ? (
        <p class="text-danger-text mt-4 flex items-start gap-2 text-sm" role="alert">
          <Icon name="danger" size={18} class="shrink-0" />
          {error}
        </p>
      ) : null}
    </Modal>
  );
}
