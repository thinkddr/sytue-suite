/**
 * AutoMigrateWizard — «Trae todo lo que tengas» en un login.
 *
 * El asistente MANUAL (MigrateWizard) pide de qué cuenta a qué cuenta y qué
 * contenido. Este no pide nada de eso: se inicia sesión en el SaaS de origen y a
 * partir de ahí se le pregunta AL ORIGEN qué hay y a Sytue en qué cuenta va, con el
 * plan delante antes de tocar nada.
 *
 * Tres pasos y ya: de dónde → conectar (y, en Google, hasta dónde) → esto es lo que
 * traigo, ¿lo lanzo?
 *
 * HONESTIDAD: el plan enseña también lo que NO se trae y las cuentas del origen que
 * no tienen equivalente en Sytue. Un asistente que dice «todo» y se deja cosas por
 * el camino sin decirlo es peor que uno que pregunta.
 */
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  DataFlow,
  FieldHint,
  Icon,
  InlineHelp,
  Spinner,
  Textarea,
  Wizard,
} from '@sytue/ui';
import type { DataCategory, ReviewItem, WizardStep } from '@sytue/ui';
import { ReviewSummary } from '@sytue/ui';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { MigrationDomain, MigrationJob, MigrationPlan } from '../lib/gateway.js';

/** Orígenes que el asistente automático sabe migrar, y su plantilla del catálogo. */
export type AutoSourceKey = 'google' | 'github' | 'microsoft';

export interface AutoSource {
  key: AutoSourceKey;
  templateKey: string;
  /** Instancia ya creada en el catálogo de apps (si la hay). */
  instanceId?: string;
  /** La conexión ya está autorizada (hay credencial viva). */
  connected: boolean;
}

interface AutoMigrateWizardProps {
  t: T;
  sources: AutoSource[];
  /** Instancia que acaba de volver del consentimiento (?instance=…&oauth=connected). */
  justConnected?: string;
  onLaunched: (jobs: MigrationJob[]) => void;
  onCancel: () => void;
}

// Marcas comerciales: no se traducen.
const SOURCE_LABEL: Record<AutoSourceKey, string> = {
  google: 'Google',
  github: 'GitHub',
  microsoft: 'Microsoft 365',
};

const SOURCE_DESC: Record<AutoSourceKey, ConsoleMessageKey> = {
  google: 'wizard.auto.source.googleDesc',
  github: 'wizard.auto.source.githubDesc',
  microsoft: 'wizard.auto.source.microsoftDesc',
};

const DOMAIN_KEY: Record<MigrationDomain, ConsoleMessageKey> = {
  drive: 'wizard.auto.item.drive',
  contacts: 'wizard.auto.item.contacts',
  repos: 'wizard.auto.item.repos',
  gists: 'wizard.auto.item.gists',
  teams: 'wizard.auto.item.teams',
  team_files: 'wizard.auto.item.teamFiles',
  mail: 'wizard.auto.item.mail',
  calendar: 'wizard.auto.item.calendar',
  cloud: 'wizard.auto.item.cloud',
};

/** Qué es cada cosa del plan, dicho con las categorías del aviso de datos. */
const DOMAIN_DATA: Record<MigrationDomain, DataCategory> = {
  drive: 'files',
  contacts: 'contacts',
  repos: 'code',
  gists: 'code',
  teams: 'messages',
  team_files: 'files',
  mail: 'mail',
  calendar: 'events',
  cloud: 'cloud',
};

/**
 * Lo que entra de cada origen ANTES de conectar, cuando aún no hay plan: lo mismo
 * que ofrece migrate.Discover (services/integrations). Conectado, manda el plan.
 */
const INCOMING: Record<AutoSourceKey, MigrationDomain[]> = {
  google: ['drive', 'contacts', 'mail', 'calendar'],
  github: ['repos'],
  microsoft: ['drive', 'contacts', 'teams'],
};

/** Lo que NO se trae, por origen. Se enseña ANTES de lanzar, no en el informe. */
const LIMITS: Record<AutoSourceKey, ConsoleMessageKey[]> = {
  google: [
    'wizard.migrate.nots.modifiedDate',
    'wizard.migrate.nots.permissions',
    'wizard.auto.nots.cloudDatabases',
    'wizard.auto.nots.cloudImages',
  ],
  github: [
    'wizard.auto.nots.actions',
    'wizard.auto.nots.forgeAccount',
    'wizard.auto.nots.settings',
  ],
  microsoft: [
    'wizard.auto.nots.teamsHistory',
    'wizard.auto.nots.teamsGuests',
    'wizard.migrate.nots.modifiedDate',
    'wizard.migrate.nots.permissions',
    'wizard.migrate.nots.mailCalendar',
  ],
};

export default function AutoMigrateWizard({
  t,
  sources,
  justConnected,
  onLaunched,
  onCancel,
}: AutoMigrateWizardProps) {
  const initial =
    sources.find((s) => s.instanceId && s.instanceId === justConnected) ?? sources[0] ?? null;
  const [chosen, setChosen] = useState<AutoSource | null>(initial);
  // 'mine' = lo de la cuenta que inicia sesión; 'org' = todo el personal del origen.
  const [scope, setScope] = useState<'mine' | 'org'>('mine');
  const [step, setStep] = useState(justConnected ? 2 : 0);
  const [plan, setPlan] = useState<MigrationPlan | null>(null);
  const [loadingPlan, setLoadingPlan] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [serviceKey, setServiceKey] = useState('');

  const ids = ['origin', 'connect', 'review'];
  const go = (id: string) => setStep(ids.indexOf(id));

  // El plan se pide en cuanto hay una conexión autorizada: es lo que convierte el
  // login en «esto es lo que tienes» sin que nadie rellene un formulario.
  async function refreshPlan(instanceId: string) {
    setLoadingPlan(true);
    setError('');
    try {
      const res = await fetch(
        `/api/admin/migrations/plan?instance_id=${encodeURIComponent(instanceId)}`,
      );
      if (!res.ok) throw new Error(String(res.status));
      const out = (await res.json()) as { plan: MigrationPlan };
      setPlan(out.plan);
    } catch (e) {
      setPlan(null);
      // Un 400 aquí es «esta conexión todavía no está autorizada», que es el estado
      // NORMAL antes de iniciar sesión: se ofrece el botón, no un error en rojo.
      if (Number(e instanceof Error ? e.message : 0) !== 400) setError(t('wizard.auto.error.plan'));
    } finally {
      setLoadingPlan(false);
    }
  }

  // Solo al elegir origen o al volver del proveedor. refreshPlan queda FUERA de las
  // dependencias a propósito: se recrea en cada render y meterla dispararía una
  // consulta al origen por cada tecla que se pulse en el paso de la clave.
  useEffect(() => {
    // Basta con que haya conexión creada: quien dice si está autorizada es el ORIGEN
    // al responder el plan, no el estado que tenga guardado la instancia.
    if (chosen?.instanceId) void refreshPlan(chosen.instanceId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chosen?.instanceId, chosen?.connected]);

  /** Crea la conexión si aún no existe y manda a la persona a autorizar. */
  async function connect() {
    if (!chosen || busy) return;
    setBusy(true);
    setError('');
    // Qué se estaba haciendo al fallar: el mismo 400 significa cosas distintas al
    // crear la conexión (plantilla ausente del catálogo) que al pedir consentimiento
    // (Sytue no tiene app registrada ante el proveedor), y solo la segunda tiene una
    // salida que quien mira la pantalla puede tomar.
    let phase: 'create' | 'consent' = 'create';
    try {
      let instanceId = chosen.instanceId;
      if (!instanceId) {
        const created = (await fetch('/api/admin/apps/instances', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({
            template_key: chosen.templateKey,
            name: SOURCE_LABEL[chosen.key],
          }),
        }).then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))) as {
          id?: string;
        };
        if (!created.id) throw new Error('bad_shape');
        instanceId = created.id;
      }
      phase = 'consent';
      const start = (await fetch(
        `/api/admin/apps/instances/${encodeURIComponent(instanceId)}/oauth-start`,
        {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ return_to: 'migrations' }),
        },
      ).then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))) as {
        authorize_url?: string;
      };
      if (!start.authorize_url) throw new Error('bad_shape');
      // A partir de aquí navega la persona: vuelve a /migrations ya conectada.
      window.location.assign(start.authorize_url);
    } catch (e) {
      const status = e instanceof Error ? e.message : '';
      // El gateway rechaza el consentimiento por dos motivos que NO se arreglan
      // reintentando, y decir «vuelve a intentarlo en unos segundos» ante ellos deja
      // a quien mira la pantalla dando clics para siempre.
      const key: ConsoleMessageKey =
        phase !== 'consent'
          ? 'wizard.auto.error.connect'
          : status === '400'
            ? 'wizard.auto.error.noProviderApp'
            : status === '503'
              ? 'wizard.auto.error.noKek'
              : 'wizard.auto.error.connect';
      setError(t(key, { source: SOURCE_LABEL[chosen.key] }));
      setBusy(false);
    }
  }

  /** Guarda la clave de la cuenta de servicio (solo alcance «toda la organización»). */
  async function saveServiceKey() {
    if (!chosen?.instanceId || !serviceKey.trim() || busy) return;
    setBusy(true);
    setError('');
    try {
      const res = await fetch(
        `/api/admin/apps/instances/${encodeURIComponent(chosen.instanceId)}/secrets`,
        {
          // PATCH y no PUT: añade la clave SIN borrar el token del consentimiento
          // que esta misma conexión ya tiene guardado.
          method: 'PATCH',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ secrets: { service_account_key: serviceKey.trim() } }),
        },
      );
      if (!res.ok) throw new Error(String(res.status));
      setServiceKey('');
      await refreshPlan(chosen.instanceId);
    } catch {
      setError(t('wizard.auto.error.serviceKey'));
    } finally {
      setBusy(false);
    }
  }

  async function launch() {
    if (!chosen?.instanceId || busy) return;
    setBusy(true);
    setError('');
    try {
      const res = await fetch('/api/admin/migrations/auto', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ instance_id: chosen.instanceId }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const out = (await res.json()) as { migrations?: MigrationJob[] };
      onLaunched(out.migrations ?? []);
    } catch {
      setError(t('wizard.auto.error.launch'));
    } finally {
      setBusy(false);
    }
  }

  // ---- Paso 1: de dónde ----

  const stepOrigin: WizardStep = {
    id: 'origin',
    title: t('wizard.auto.step.origin'),
    heading: t('wizard.auto.origin.heading'),
    description: t('wizard.auto.origin.description'),
    canContinue: Boolean(chosen),
    content: (
      <ul class="flex max-w-2xl flex-col gap-3">
        {sources.map((s) => (
          <li key={s.key}>
            <ChoiceCard
              name="sytue-auto-source"
              value={s.key}
              icon={s.key === 'github' ? 'code' : s.key === 'microsoft' ? 'grid' : 'globe'}
              title={SOURCE_LABEL[s.key]}
              description={t(SOURCE_DESC[s.key])}
              badge={
                s.connected ? <Badge tone="success">{t('wizard.auto.connected')}</Badge> : null
              }
              selected={chosen?.key === s.key}
              onSelect={() => {
                setChosen(s);
                setPlan(null);
              }}
            />
          </li>
        ))}
      </ul>
    ),
  };

  // ---- Paso 2: conectar (y, en Google, hasta dónde) ----

  const isGoogle = chosen?.key === 'google';
  // Google y Microsoft ofrecen el mismo par de alcances; lo que cambia es CÓMO se
  // amplía: Google pide una cuenta de servicio con delegación, Microsoft solo que
  // un administrador apruebe una pantalla.
  const hasScopes = isGoogle || chosen?.key === 'microsoft';
  const connected = Boolean(plan);
  const orgReady = plan?.scope === 'organization';

  const stepConnect: WizardStep = {
    id: 'connect',
    title: t('wizard.auto.step.connect'),
    heading: t('wizard.auto.connect.heading', { source: SOURCE_LABEL[chosen?.key ?? 'google'] }),
    description: t('wizard.auto.connect.description'),
    canContinue: connected && (!hasScopes || scope === 'mine' || orgReady),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        {chosen ? (
          <DataFlow
            partner={SOURCE_LABEL[chosen.key]}
            incoming={[
              ...new Set(
                (plan?.items.map((i) => i.domain) ?? INCOMING[chosen.key]).map(
                  (d) => DOMAIN_DATA[d],
                ),
              ),
            ]}
          />
        ) : null}
        {hasScopes ? (
          <ul class="flex flex-col gap-3">
            <li>
              <ChoiceCard
                name="sytue-auto-scope"
                value="mine"
                icon="user"
                title={t('wizard.auto.scope.mine')}
                description={t('wizard.auto.scope.mineHint')}
                badge={<Badge tone="neutral">{t('wizard.auto.scope.oneClick')}</Badge>}
                selected={scope === 'mine'}
                onSelect={() => setScope('mine')}
              />
            </li>
            <li>
              <ChoiceCard
                name="sytue-auto-scope"
                value="org"
                icon="users"
                title={t('wizard.auto.scope.org')}
                description={t(
                  isGoogle ? 'wizard.auto.scope.orgHint' : 'wizard.auto.scope.orgHintMicrosoft',
                )}
                selected={scope === 'org'}
                onSelect={() => setScope('org')}
              />
            </li>
          </ul>
        ) : null}

        {connected ? (
          <InlineHelp tone="success" title={t('wizard.auto.connect.doneTitle')}>
            {t('wizard.auto.connect.doneBody', {
              account: plan?.connectedAs ?? '',
              source: SOURCE_LABEL[chosen?.key ?? 'google'],
            })}
          </InlineHelp>
        ) : (
          <div class="flex flex-col gap-2">
            <Button variant="primary" loading={busy} onClick={() => void connect()}>
              {t('wizard.auto.connect.action', { source: SOURCE_LABEL[chosen?.key ?? 'google'] })}
            </Button>
            <FieldHint>
              {t(
                chosen?.key === 'github'
                  ? 'wizard.auto.connect.readOnlyGithub'
                  : 'wizard.auto.connect.readOnly',
              )}
            </FieldHint>
          </div>
        )}

        {loadingPlan ? (
          <p class="text-text-muted flex items-center gap-2 text-sm">
            <Spinner size={16} /> {t('wizard.auto.connect.checking')}
          </p>
        ) : null}

        {/* Ampliar a toda la organización: el paso extra del proveedor, con el
            enlace a SU pantalla ya relleno para no ir buscándola. En Microsoft es
            solo aprobar; en Google hay además una clave que aportar. */}
        {hasScopes && scope === 'org' && connected && !orgReady ? (
          <section class="border-border bg-surface-raised flex flex-col gap-3 rounded-lg border p-4">
            <h3 class="text-text text-sm font-semibold">{t('wizard.auto.delegation.title')}</h3>
            {isGoogle ? (
              <ol class="text-text-muted flex list-decimal flex-col gap-1.5 pl-5 text-sm leading-snug">
                <li>{t('wizard.auto.delegation.step1')}</li>
                <li>{t('wizard.auto.delegation.step2')}</li>
                <li>{t('wizard.auto.delegation.step3')}</li>
              </ol>
            ) : (
              <p class="text-text-muted text-sm leading-snug">
                {t('wizard.auto.delegation.microsoftBody')}
              </p>
            )}
            {isGoogle ? (
              <Textarea
                rows={4}
                value={serviceKey}
                spellcheck={false}
                class="font-mono text-xs"
                aria-label={t('wizard.auto.delegation.keyLabel')}
                placeholder='{"type":"service_account", …}'
                onInput={(e) => setServiceKey(e.currentTarget.value)}
              />
            ) : null}
            <div class="flex flex-wrap gap-2">
              {isGoogle ? (
                <Button
                  variant="secondary"
                  disabled={!serviceKey.trim() || busy}
                  onClick={() => void saveServiceKey()}
                >
                  {t('wizard.auto.delegation.save')}
                </Button>
              ) : null}
              {plan?.delegationSetupUrl ? (
                <Button
                  variant="ghost"
                  trailingIcon={<Icon name="globe" size={14} />}
                  onClick={() => window.open(plan.delegationSetupUrl, '_blank', 'noopener')}
                >
                  {t('wizard.auto.delegation.open')}
                </Button>
              ) : null}
              <Button
                variant="ghost"
                disabled={loadingPlan || !chosen?.instanceId}
                onClick={() => chosen?.instanceId && void refreshPlan(chosen.instanceId)}
              >
                {t('wizard.auto.delegation.recheck')}
              </Button>
            </div>
            <FieldHint>
              {t(isGoogle ? 'wizard.auto.delegation.why' : 'wizard.auto.delegation.whyMicrosoft')}
            </FieldHint>
          </section>
        ) : null}

        {isGoogle && scope === 'org' && orgReady ? (
          <InlineHelp tone="success" title={t('wizard.auto.delegation.readyTitle')}>
            {t('wizard.auto.delegation.readyBody', { count: plan?.people.length ?? 0 })}
          </InlineHelp>
        ) : null}

        {error ? (
          <InlineHelp tone="danger" title={t('wizard.auto.error.title')}>
            {error}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  // ---- Paso 3: esto es lo que traigo ----

  const limits = LIMITS[chosen?.key ?? 'google'];
  const summary: ReviewItem[] = plan
    ? [
        {
          label: t('wizard.auto.review.account'),
          value: plan.connectedAs || '—',
          onEdit: () => go('connect'),
        },
        {
          label: t('wizard.auto.review.who'),
          value:
            plan.people.length === 1
              ? (plan.people[0]?.targetUser ?? '—')
              : t('wizard.auto.review.people', { count: plan.people.length }),
        },
        {
          label: t('wizard.auto.review.what'),
          value:
            plan.items
              .map((i) =>
                i.count >= 0 ? `${t(DOMAIN_KEY[i.domain])} (${i.count})` : t(DOMAIN_KEY[i.domain]),
              )
              .join(' · ') || '—',
        },
        {
          label: t('wizard.auto.review.jobs'),
          value: t('wizard.auto.review.jobsValue', { count: plan.jobs }),
        },
      ]
    : [];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.auto.step.review'),
    heading: t('wizard.auto.review.heading'),
    description: t('wizard.auto.review.description'),
    canContinue: Boolean(plan && plan.jobs > 0),
    nextLabel: t('wizard.auto.review.launch'),
    onNext: () => void launch(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        {loadingPlan ? (
          <p class="text-text-muted flex items-center gap-2 text-sm">
            <Spinner size={16} /> {t('wizard.auto.connect.checking')}
          </p>
        ) : null}

        {plan ? <ReviewSummary items={summary} /> : null}

        {plan && plan.unmatched.length > 0 ? (
          <InlineHelp tone="warning" title={t('wizard.auto.unmatched.title')}>
            {t('wizard.auto.unmatched.body', { count: plan.unmatched.length })}
            <span class="mt-2 block font-mono text-xs break-words">
              {plan.unmatched.slice(0, 12).join(', ')}
            </span>
          </InlineHelp>
        ) : null}

        <section class="border-warning bg-warning-bg rounded-lg border p-4">
          <h3 class="text-warning-text text-sm font-semibold">{t('wizard.auto.limits.title')}</h3>
          <ul class="mt-2 flex flex-col gap-1.5">
            {limits.map((k) => (
              <li key={k} class="text-text-muted text-sm leading-snug">
                {t(k)}
              </li>
            ))}
          </ul>
        </section>

        <InlineHelp tone="info" title={t('wizard.auto.safe.title')}>
          {t('wizard.auto.safe.body')}
        </InlineHelp>

        {error ? (
          <InlineHelp tone="danger" title={t('wizard.auto.error.title')}>
            {error}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const byId: Record<string, WizardStep> = {
    origin: stepOrigin,
    connect: stepConnect,
    review: stepReview,
  };

  return (
    <Wizard
      steps={ids.map((id) => byId[id] as WizardStep)}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}
