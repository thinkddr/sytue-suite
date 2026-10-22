/**
 * EnvPanel — variables de entorno y secretos del proyecto (estilo Vercel). Lista
 * con valor enmascarado, ámbitos (producción/preview/desarrollo) y origen
 * (manual o inyectado por un servicio gestionado). Añadir, editar y eliminar
 * (con confirmación) vía POST /api/env — el borrado también por POST con
 * {delete:true} porque el checkOrigin CSRF de Astro bloquea DELETE. Los secretos
 * jamás se muestran en claro ni se guardan en git (Secret Manager). §10, §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  Table,
  Badge,
  Button,
  Icon,
  Input,
  Select,
  Checkbox,
  Modal,
  FieldGroup,
  FieldHint,
  describedBy,
  useToast,
  ToastProvider,
  SectionHeader,
  withBase,
} from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { EnvScope as ApiEnvScope } from '@sytue/api-clients';
import type { EnvVar, EnvScope } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import DemoBanner from './DemoBanner.js';
import EnvImport, { type ParsedVar } from './EnvImport.js';

function scopeLabel(t: T, scope: EnvScope): string {
  switch (scope) {
    case 'production':
      return t('env.scope.production');
    case 'preview':
      return t('env.scope.preview');
    case 'development':
      return t('env.scope.development');
  }
}

/** Origen de una variable inyectada por un servicio gestionado (nombre de producto). */
function sourceLabel(t: T, source: string): string {
  switch (source) {
    case 'postgres':
      return t('service.managed.postgres');
    case 'storage':
      return t('service.managed.storage');
    case 'cache':
      return t('service.managed.cache');
    case 'functions':
      return t('service.managed.functions');
    case 'cron':
      return t('service.managed.cron');
    case 'auth':
      return t('service.managed.auth');
    default:
      return source;
  }
}

const MASK = '••••••••••••';

/**
 * Ámbitos de VISTA que corresponden a cada ámbito de la API. Es lo que hace útil
 * el ámbito: `preview` deja poner una credencial DISTINTA en las previews de una
 * rama sin revisar, en vez de prestarle la de producción.
 */
const SCOPE_VIEW: Record<ApiEnvScope, EnvScope[]> = {
  all: ['production', 'preview'],
  production: ['production'],
  preview: ['preview'],
};

/** Explicación en llano de lo que implica cada ámbito (no su nombre técnico). */
function scopeHint(t: T, scope: ApiEnvScope): string {
  switch (scope) {
    case 'production':
      return t('env.scopeField.help.production');
    case 'preview':
      return t('env.scopeField.help.preview');
    case 'all':
      return t('env.scopeField.help.all');
  }
}

/** Clave válida: letras/dígitos/_, sin empezar por dígito (misma regla que el BFF). */
const KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

interface FieldErrors {
  key?: string;
  value?: string;
}

function validate(t: T, key: string, value: string): FieldErrors {
  const errs: FieldErrors = {};
  if (!key) errs.key = t('env.error.keyRequired');
  else if (!KEY_RE.test(key)) errs.key = t('env.error.keyFormat');
  if (!value.trim()) errs.value = t('env.error.valueRequired');
  return errs;
}

/** POST al BFF (/api/env). Devuelve live=false en modo demostración. */
async function postEnv(body: Record<string, unknown>): Promise<{ ok: boolean; live: boolean }> {
  return postBff('/api/env', body);
}

interface Props {
  vars: EnvVar[];
  project: string;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function EnvPanel(props: Props) {
  return (
    <ToastProvider>
      <EnvInner {...props} />
    </ToastProvider>
  );
}

function EnvInner({ vars, project, live, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<EnvVar[]>(vars);
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [secret, setSecret] = useState(true);
  /** Ámbito elegido. `all` es el mismo defecto que aplica el control-plane. */
  const [scope, setScope] = useState<ApiEnvScope>('all');
  /** Clave en edición (la clave no se puede cambiar: es el identificador). */
  const [editing, setEditing] = useState<string | null>(null);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<EnvVar | null>(null);
  const [deleting, setDeleting] = useState(false);

  /**
   * Recoge en la tabla lo que acaba de importarse. Las que ya existían se
   * actualizan en su sitio y las nuevas entran arriba: recargar la página para
   * verlo sería perder el formulario a medio escribir.
   */
  function onImported(nuevas: ParsedVar[], esSecreto: boolean, ambito: ApiEnvScope) {
    if (nuevas.length === 0) return;
    const now = new Date().toISOString();
    const scopes = SCOPE_VIEW[ambito];
    setItems((prev) => {
      const out = [...prev];
      for (const v of nuevas) {
        const fila: EnvVar = {
          key: v.key,
          preview: esSecreto ? MASK : v.value,
          secret: esSecreto,
          scopes,
          source: 'manual',
          updatedAt: now,
        };
        const i = out.findIndex((x) => x.key === v.key);
        if (i >= 0) out[i] = fila;
        else out.unshift(fila);
      }
      return out;
    });
  }

  function resetForm() {
    setEditing(null);
    setKey('');
    setValue('');
    setSecret(true);
    setScope('all');
    setErrors({});
  }

  function startEdit(v: EnvVar) {
    setEditing(v.key);
    setKey(v.key);
    setValue('');
    setSecret(v.secret);
    // Se reedita con el ámbito que la variable YA tiene: guardarla sin tocarlo no
    // debe ampliarla en silencio a los dos entornos.
    setScope(
      v.scopes.length === 1 && (v.scopes[0] === 'production' || v.scopes[0] === 'preview')
        ? v.scopes[0]
        : 'all',
    );
    setErrors({});
    requestAnimationFrame(() => document.getElementById('env-value')?.focus());
  }

  async function submit(e: Event) {
    e.preventDefault();
    const k = (editing ?? key).trim();
    const errs = validate(t, k, value);
    setErrors(errs);
    if (errs.key || errs.value) return;

    setBusy(true);
    try {
      const res = await postEnv({ project, key: k, value, secret, scope });
      const preview = secret ? MASK : value;
      const now = new Date().toISOString();
      // Los ámbitos de la fila salen del ámbito que se acaba de GUARDAR; antes se
      // escribían a mano como «producción y preview» pasara lo que pasara, así que
      // una credencial acotada a preview se anunciaba como si también estuviera en
      // producción —justo al revés de lo que se había pedido—.
      const scopes = SCOPE_VIEW[scope];
      setItems((prev) =>
        prev.some((i) => i.key === k)
          ? prev.map((i) => (i.key === k ? { ...i, preview, secret, scopes, updatedAt: now } : i))
          : [{ key: k, preview, secret, scopes, source: 'manual', updatedAt: now }, ...prev],
      );
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: editing ? t('env.updated') : t('env.saved'),
        ...demoDesc(t, res.live, t('env.saved.desc', { key: k })),
      });
      resetForm();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('env.save.fail'),
        description: t('common.retry'),
      });
    } finally {
      setBusy(false);
    }
  }

  async function confirmDelete() {
    if (!pendingDelete) return;
    const k = pendingDelete.key;
    setDeleting(true);
    try {
      const res = await postEnv({ project, key: k, delete: true });
      setItems((prev) => prev.filter((i) => i.key !== k));
      if (editing === k) resetForm();
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('env.deleted'),
        ...demoDesc(t, res.live, t('env.deleted.desc', { key: k })),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('env.delete.fail'),
        description: t('common.retry'),
      });
    } finally {
      setDeleting(false);
    }
  }

  const columns: TableColumn<EnvVar>[] = [
    {
      key: 'key',
      header: t('env.field.key'),
      cell: (v) => (
        <span class="flex items-center gap-1.5 font-mono text-sm">
          {v.secret ? (
            <Icon name="lock" size={16} label={t('env.secretLabel')} class="text-text-subtle" />
          ) : null}
          {v.key}
        </span>
      ),
    },
    {
      key: 'preview',
      header: t('env.field.value'),
      cell: (v) => (
        <span
          class="text-text-muted block max-w-[220px] truncate font-mono text-sm"
          title={v.secret ? undefined : v.preview}
        >
          {v.preview}
        </span>
      ),
    },
    {
      key: 'scopes',
      header: t('env.col.scopes'),
      cell: (v) => (
        <span class="flex flex-wrap gap-1">
          {v.scopes.map((s) => (
            <Badge key={s} tone="neutral">
              {scopeLabel(t, s)}
            </Badge>
          ))}
        </span>
      ),
    },
    {
      key: 'source',
      header: t('env.col.source'),
      cell: (v) =>
        v.source === 'manual' ? (
          <span class="text-text-muted text-sm">{t('env.source.manual')}</span>
        ) : (
          <Badge tone="accent">{sourceLabel(t, v.source)}</Badge>
        ),
    },
    {
      key: 'actions',
      header: <span class="sr-only">{t('env.col.actions')}</span>,
      align: 'end',
      // Las inyectadas por un servicio gestionado las gobierna el control-plane:
      // ni se editan ni se borran desde aquí.
      cell: (v) =>
        v.source !== 'manual' ? null : (
          <span class="flex justify-end gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => startEdit(v)}
              aria-label={t('env.editVar', { key: v.key })}
            >
              {t('common.edit')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="text-danger-text"
              onClick={() => setPendingDelete(v)}
              aria-label={t('env.deleteVar', { key: v.key })}
            >
              {t('common.delete')}
            </Button>
          </span>
        ),
    },
  ];

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="lock" size={20} />}
        title={t('nav.env')}
        subtitle={t('env.subtitle')}
      />
      <div class="cloud-content">
        {/* Traer un .env entero: la primera fricción de quien llega de otro
            sitio con veinte variables y solo puede darlas de alta de una en una. */}
        <div class="mb-4 flex flex-wrap items-center gap-2">
          <EnvImport
            project={project}
            t={t}
            existing={items.map((i) => i.key)}
            onImported={onImported}
          />
          {/* Y la vuelta: sin descarga, arrancar el proyecto en local significaba
              copiar los valores a mano de una pantalla que además oculta los
              secretos. Enlace normal (no fetch): el navegador ya sabe guardar un
              fichero, y así funciona también con el botón derecho: el guardado
              lo fuerza el `Content-Disposition` del servidor, no un atributo
              `download` (que además no existe en un <button>). El backend exige
              admin/owner y lo deja escrito en la auditoría. */}
          <Button
            variant="secondary"
            href={withBase(
              import.meta.env.BASE_URL,
              `/api/env/export?project=${encodeURIComponent(project)}`,
            )}
          >
            {t('env.export')}
          </Button>
        </div>
        <Card class="mb-6">
          <CardBody>
            <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void submit(e)}>
              <FieldGroup
                label={t('env.field.key')}
                htmlFor="env-key"
                class="min-w-0 flex-1"
                {...(errors.key ? { error: errors.key } : {})}
              >
                <Input
                  id="env-key"
                  value={key}
                  placeholder="API_TOKEN"
                  class="font-mono"
                  disabled={editing !== null}
                  invalid={Boolean(errors.key)}
                  aria-describedby={describedBy('env-key', undefined, errors.key)}
                  onInput={(e) => setKey(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup
                label={t('env.field.value')}
                htmlFor="env-value"
                class="min-w-0 flex-1"
                {...(errors.value ? { error: errors.value } : {})}
                {...(editing && !errors.value ? { help: t('env.field.value.help') } : {})}
              >
                <Input
                  id="env-value"
                  type={secret ? 'password' : 'text'}
                  value={value}
                  placeholder={t('env.field.value.placeholder')}
                  class="font-mono"
                  invalid={Boolean(errors.value)}
                  aria-describedby={describedBy(
                    'env-value',
                    editing ? t('env.field.value.help') : undefined,
                    errors.value,
                  )}
                  onInput={(e) => setValue(e.currentTarget.value)}
                />
              </FieldGroup>
              {/* Poner una credencial distinta en preview es lo que hace segura la
                  preview de una rama sin revisar: si comparte la de producción,
                  cualquier cambio sin revisar corre con acceso a los datos reales. */}
              <FieldGroup label={t('env.scopeField')} htmlFor="env-scope" class="min-w-0 basis-56">
                <Select
                  id="env-scope"
                  value={scope}
                  aria-describedby="env-scope-hint"
                  onChange={(e) => setScope(e.currentTarget.value as ApiEnvScope)}
                >
                  <option value="all">{t('env.scopeField.all')}</option>
                  <option value="production">{t('env.scopeField.production')}</option>
                  <option value="preview">{t('env.scopeField.preview')}</option>
                </Select>
                <FieldHint id="env-scope-hint" class="mt-1.5">
                  {scopeHint(t, scope)}
                </FieldHint>
              </FieldGroup>
              <Checkbox
                label={t('env.secret')}
                checked={secret}
                onChange={(e) => setSecret(e.currentTarget.checked)}
              />
              <span class="flex shrink-0 gap-2">
                {editing ? (
                  <Button type="button" variant="ghost" disabled={busy} onClick={resetForm}>
                    {t('common.cancel')}
                  </Button>
                ) : null}
                <Button
                  type="submit"
                  variant="primary"
                  loading={busy}
                  {...(editing ? {} : { leadingIcon: <Icon name="plus" size={16} /> })}
                >
                  {editing ? t('common.saveChanges') : t('common.add')}
                </Button>
              </span>
            </form>
          </CardBody>
        </Card>

        <div class="mb-3 flex items-baseline justify-between gap-3">
          <h2 class="sytue-h4 text-text">{t('env.listTitle')}</h2>
          <span class="text-text-muted text-sm tabular-nums">
            {t('env.count', { count: items.length })}
          </span>
        </div>
        <Table
          columns={columns}
          rows={items}
          rowKey={(v) => v.key}
          caption={t('env.caption')}
          emptyContent={t('env.empty')}
        />
      </div>

      {/* Confirmación de borrado (acción destructiva: nunca directa). */}
      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('env.confirm.title')}
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
          {t('env.confirm.body', { key: pendingDelete?.key ?? '' })}
        </p>
      </Modal>
    </>
  );
}
