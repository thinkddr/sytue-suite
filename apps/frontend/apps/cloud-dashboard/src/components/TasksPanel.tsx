/**
 * TasksPanel — lo que el proyecto ejecuta SIN que nadie visite una URL: tareas
 * programadas (cron) y trabajos puntuales (jobs).
 *
 * Las dos existen en el control-plane desde la Ola 6 (CronJob y Job de Kubernetes,
 * uno por servicio) y no había ni una pantalla: se creaban con curl o no se
 * creaban. Van juntas y no en dos páginas porque son la misma idea —código que
 * corre solo— y se miran a la vez.
 *
 * # Un cron llama a tu app, o corre tu imagen
 *
 * El caso corriente es «toca esta ruta cada noche», y para eso el control-plane no
 * construye nada: la llamada la hace la imagen de plataforma. El segundo caso
 * —correr una imagen propia— es el mismo formulario con otro destino.
 *
 * # La programación no se teclea a ciegas
 *
 * Cinco campos separados por espacios son un examen, no una interfaz. Se ofrecen
 * las frecuencias que cubren casi todo y queda el campo libre para quien sepa
 * cron. El criterio de validez es el mismo que `domain.ValidCronSchedule`.
 *
 * Mutaciones vía POST /api/services/{create,delete}. Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Button,
  Badge,
  Icon,
  Input,
  Select,
  FieldGroup,
  Modal,
  EmptyState,
  ToastProvider,
  SectionHeader,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { BuiltImage } from '../lib/cloud.js';
import type { ManagedService } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import DemoBanner from './DemoBanner.js';
import ServiceLogsModal from './ServiceLogsModal.js';

/**
 * Frecuencias ofrecidas. Cubren lo que pide casi todo el mundo; `custom` abre el
 * campo libre para quien sepa escribir cron, que no tiene por qué ser la mayoría.
 */
const PRESETS = [
  { id: '@hourly', expr: '@hourly' },
  { id: '@daily', expr: '@daily' },
  { id: '@weekly', expr: '@weekly' },
  { id: '@monthly', expr: '@monthly' },
  { id: 'every15', expr: '*/15 * * * *' },
  { id: 'every5', expr: '*/5 * * * *' },
  { id: 'custom', expr: '' },
] as const;

type PresetId = (typeof PRESETS)[number]['id'];

/**
 * Mismo criterio que `domain.ValidCronSchedule`: macro conocida, `@every <algo>`
 * o cinco campos. Es una comprobación superficial a propósito —el parser de
 * verdad es el API server de Kubernetes— y sirve para no mandar un formulario que
 * va a volver con un 400.
 */
const MACROS = ['@hourly', '@daily', '@midnight', '@weekly', '@monthly', '@yearly', '@annually'];
function validSchedule(raw: string): boolean {
  const s = raw.trim();
  if (!s) return false;
  if (s.startsWith('@')) {
    if (MACROS.includes(s)) return true;
    return s.startsWith('@every ') && s.slice('@every '.length).trim() !== '';
  }
  return s.split(/\s+/).length === 5;
}

/** Nombre de servicio: se usa como nombre de objeto en Kubernetes. */
const TASK_NAME = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

interface Props {
  project: string;
  crons: ManagedService[];
  jobs: ManagedService[];
  workers: ManagedService[];
  images: BuiltImage[];
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function TasksPanel(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function statusTone(status: ManagedService['status']): 'success' | 'warning' {
  return status === 'running' ? 'success' : 'warning';
}

function statusLabel(t: T, status: ManagedService['status']): string {
  switch (status) {
    case 'running':
      return t('service.status.running');
    case 'provisioning':
      return t('service.status.provisioning');
    case 'paused':
      return t('service.status.paused');
  }
}

/** Código HTTP embebido por postBff en el mensaje del error (bff_<status>). */
function statusOf(e: unknown): number {
  const m = /bff_(\d+)/.exec(e instanceof Error ? e.message : String(e));
  return m ? Number(m[1]) : 0;
}

function createErrorToast(t: T, err: unknown): { title: string; description?: string } {
  switch (statusOf(err)) {
    case 403:
      return { title: t('tasks.create.forbidden') };
    case 402:
      return { title: t('tasks.create.quota') };
    case 409:
      return { title: t('tasks.create.conflict') };
    default:
      return { title: t('tasks.create.fail'), description: t('common.retry') };
  }
}

function Inner({ project, crons, jobs, workers, images, live, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [cronItems, setCronItems] = useState<ManagedService[]>(crons);
  const [jobItems, setJobItems] = useState<ManagedService[]>(jobs);
  const [workerItems, setWorkerItems] = useState<ManagedService[]>(workers);
  const [pendingDelete, setPendingDelete] = useState<ManagedService | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function confirmDelete() {
    const target = pendingDelete;
    if (!target?.id) return;
    setDeleting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/delete', {
        service: target.id,
      });
      setCronItems((prev) => prev.filter((s) => s.id !== target.id));
      setJobItems((prev) => prev.filter((s) => s.id !== target.id));
      setWorkerItems((prev) => prev.filter((s) => s.id !== target.id));
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('tasks.deleted', { name: target.name }),
        ...demoDesc(t, res.live),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('tasks.delete.fail') });
    } finally {
      setDeleting(false);
    }
  }

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="clock" size={20} />}
        title={t('nav.tasks')}
        subtitle={t('tasks.subtitle')}
      />
      <div class="cloud-content flex flex-col gap-8">
        <CronSection
          project={project}
          items={cronItems}
          images={images}
          t={t}
          locale={locale}
          toast={toast}
          onCreated={(s) => setCronItems((prev) => [...prev, s])}
          onDelete={setPendingDelete}
          deleting={deleting}
        />
        <JobSection
          project={project}
          items={jobItems}
          images={images}
          t={t}
          locale={locale}
          toast={toast}
          onCreated={(s) => setJobItems((prev) => [...prev, s])}
          onDelete={setPendingDelete}
          deleting={deleting}
        />
        <WorkerSection
          project={project}
          items={workerItems}
          images={images}
          t={t}
          locale={locale}
          toast={toast}
          onCreated={(s) => setWorkerItems((prev) => [...prev, s])}
          onDelete={setPendingDelete}
          deleting={deleting}
        />
      </div>

      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('tasks.confirm.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)} disabled={deleting}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={deleting} onClick={() => void confirmDelete()}>
              {t('common.retire')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base">
          {t('tasks.confirm.body', { name: pendingDelete?.name ?? '' })}
        </p>
      </Modal>
    </>
  );
}

type Toast = ReturnType<typeof useToast>;

interface SectionProps {
  project: string;
  items: ManagedService[];
  images: BuiltImage[];
  t: T;
  /** Idioma ya negociado en el servidor; baja hasta el visor de registro. */
  locale: Locale;
  toast: Toast;
  onCreated: (s: ManagedService) => void;
  onDelete: (s: ManagedService) => void;
  deleting: boolean;
}

// ---- Tareas programadas (cron) --------------------------------------------

function CronSection({
  project,
  items,
  images,
  t,
  locale,
  toast,
  onCreated,
  onDelete,
  deleting,
}: SectionProps) {
  const [name, setName] = useState('');
  const [preset, setPreset] = useState<PresetId>('@daily');
  const [custom, setCustom] = useState('');
  const [mode, setMode] = useState<'url' | 'image'>('url');
  const [url, setUrl] = useState('');
  const [image, setImage] = useState(images[0]?.ref ?? '');
  const [command, setCommand] = useState('');
  const [errors, setErrors] = useState<{ name?: string; schedule?: string; url?: string }>({});
  const [busy, setBusy] = useState(false);

  const schedule =
    preset === 'custom' ? custom : (PRESETS.find((p) => p.id === preset)?.expr ?? '');

  async function create(e: Event) {
    e.preventDefault();
    const clean = name.trim().toLowerCase();
    const next: typeof errors = {};
    if (!TASK_NAME.test(clean)) next.name = t('tasks.error.name');
    if (!validSchedule(schedule)) next.schedule = t('tasks.error.schedule');
    if (mode === 'url') {
      // Forma nada más: que la dirección no sea interna lo decide el control-plane
      // (mismo filtro anti-SSRF que un webhook), y ese criterio no se duplica aquí.
      try {
        const p = new URL(url.trim()).protocol;
        if (p !== 'https:' && p !== 'http:') next.url = t('tasks.error.url');
      } catch {
        next.url = t('tasks.error.url');
      }
    }
    setErrors(next);
    if (Object.keys(next).length > 0) return;

    setBusy(true);
    try {
      const config =
        mode === 'url'
          ? { schedule, url: url.trim() }
          : { schedule, image, ...(command.trim() ? { command: command.trim() } : {}) };
      const res = await postBff<{ ok: boolean; live: boolean; service: ManagedService | null }>(
        '/api/services/create',
        { project, kind: 'cron', name: clean, config },
      );
      onCreated(
        res.service ?? {
          kind: 'cron',
          name: clean,
          status: 'provisioning',
          schedule,
          ...(mode === 'url' ? { targetUrl: url.trim() } : { image }),
        },
      );
      setName('');
      setUrl('');
      setCommand('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('tasks.cron.created', { name: clean }),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', ...createErrorToast(t, err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-labelledby="cron-heading">
      <div class="mb-1 flex items-baseline justify-between gap-3">
        <h2 id="cron-heading" class="sytue-h4 text-text">
          {t('tasks.cron.heading')}
        </h2>
        <span class="text-text-muted text-sm tabular-nums">
          {t('tasks.count', { count: items.length })}
        </span>
      </div>
      <p class="text-text-muted mb-3 text-sm">{t('tasks.cron.desc')}</p>

      <Card class="mb-4">
        <CardHeader>
          <CardTitle as="h3">{t('tasks.cron.new')}</CardTitle>
        </CardHeader>
        <CardBody>
          <form class="flex flex-col gap-3" onSubmit={(e) => void create(e)}>
            <div class="flex flex-wrap items-end gap-3">
              <FieldGroup
                label={t('common.name')}
                htmlFor="cron-name"
                class="w-52"
                {...(errors.name ? { error: errors.name } : {})}
              >
                <Input
                  id="cron-name"
                  value={name}
                  required
                  placeholder="limpieza-nocturna"
                  class="font-mono"
                  invalid={Boolean(errors.name)}
                  onInput={(e) => setName(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup label={t('tasks.field.frequency')} htmlFor="cron-preset" class="w-52">
                <Select
                  id="cron-preset"
                  value={preset}
                  onChange={(e) => setPreset(e.currentTarget.value as PresetId)}
                >
                  {PRESETS.map((p) => (
                    <option key={p.id} value={p.id}>
                      {t(`tasks.freq.${p.id}`)}
                    </option>
                  ))}
                </Select>
              </FieldGroup>
              {preset === 'custom' ? (
                <FieldGroup
                  label={t('tasks.field.expression')}
                  htmlFor="cron-custom"
                  class="w-56"
                  help={t('tasks.field.expression.help')}
                  {...(errors.schedule ? { error: errors.schedule } : {})}
                >
                  <Input
                    id="cron-custom"
                    value={custom}
                    placeholder="0 3 * * *"
                    class="font-mono"
                    invalid={Boolean(errors.schedule)}
                    onInput={(e) => setCustom(e.currentTarget.value)}
                  />
                </FieldGroup>
              ) : null}
              <FieldGroup label={t('tasks.field.what')} htmlFor="cron-mode" class="w-56">
                <Select
                  id="cron-mode"
                  value={mode}
                  onChange={(e) => setMode(e.currentTarget.value as 'url' | 'image')}
                >
                  <option value="url">{t('tasks.mode.url')}</option>
                  <option value="image" disabled={images.length === 0}>
                    {t('tasks.mode.image')}
                  </option>
                </Select>
              </FieldGroup>
            </div>

            <div class="flex flex-wrap items-end gap-3">
              {mode === 'url' ? (
                <FieldGroup
                  label={t('tasks.field.url')}
                  htmlFor="cron-url"
                  class="min-w-[18rem] flex-1"
                  help={t('tasks.field.url.help')}
                  {...(errors.url ? { error: errors.url } : {})}
                >
                  <Input
                    id="cron-url"
                    type="url"
                    value={url}
                    placeholder="https://mi-app.apps.sytue.eu/tareas/limpiar"
                    class="font-mono"
                    invalid={Boolean(errors.url)}
                    onInput={(e) => setUrl(e.currentTarget.value)}
                  />
                </FieldGroup>
              ) : (
                <>
                  <FieldGroup
                    label={t('functions.field.image')}
                    htmlFor="cron-image"
                    class="min-w-[16rem] flex-1"
                  >
                    <Select
                      id="cron-image"
                      value={image}
                      onChange={(e) => setImage(e.currentTarget.value)}
                    >
                      {images.map((img) => (
                        <option key={img.ref} value={img.ref}>
                          {t('functions.image.option', {
                            commit: img.commit,
                            ref: img.gitRef,
                            date: img.createdAt.slice(0, 10),
                          })}
                        </option>
                      ))}
                    </Select>
                  </FieldGroup>
                  <FieldGroup
                    label={t('tasks.field.command')}
                    htmlFor="cron-command"
                    class="min-w-[14rem] flex-1"
                  >
                    <Input
                      id="cron-command"
                      value={command}
                      placeholder="node scripts/limpiar.js"
                      class="font-mono"
                      onInput={(e) => setCommand(e.currentTarget.value)}
                    />
                  </FieldGroup>
                </>
              )}
              <Button
                type="submit"
                variant="primary"
                class="shrink-0"
                loading={busy}
                leadingIcon={<Icon name="plus" size={16} />}
              >
                {t('common.create')}
              </Button>
            </div>
          </form>
        </CardBody>
      </Card>

      <TaskList
        items={items}
        t={t}
        locale={locale}
        emptyTitle={t('tasks.cron.empty.title')}
        emptyDesc={t('tasks.cron.empty.desc')}
        onDelete={onDelete}
        deleting={deleting}
      />
    </section>
  );
}

// ---- Trabajos puntuales (jobs) ---------------------------------------------

function JobSection({
  project,
  items,
  images,
  t,
  locale,
  toast,
  onCreated,
  onDelete,
  deleting,
}: SectionProps) {
  const [name, setName] = useState('');
  const [image, setImage] = useState(images[0]?.ref ?? '');
  const [command, setCommand] = useState('');
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  async function create(e: Event) {
    e.preventDefault();
    const clean = name.trim().toLowerCase();
    if (!TASK_NAME.test(clean)) {
      setNameError(t('tasks.error.name'));
      return;
    }
    setNameError(undefined);
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; service: ManagedService | null }>(
        '/api/services/create',
        {
          project,
          kind: 'job',
          name: clean,
          config: { image, ...(command.trim() ? { command: command.trim() } : {}) },
        },
      );
      onCreated(res.service ?? { kind: 'job', name: clean, status: 'provisioning', image });
      setName('');
      setCommand('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('tasks.job.created', { name: clean }),
        ...demoDesc(t, res.live, t('tasks.job.created.desc')),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', ...createErrorToast(t, err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-labelledby="jobs-heading">
      <div class="mb-1 flex items-baseline justify-between gap-3">
        <h2 id="jobs-heading" class="sytue-h4 text-text">
          {t('tasks.job.heading')}
        </h2>
        <span class="text-text-muted text-sm tabular-nums">
          {t('tasks.count', { count: items.length })}
        </span>
      </div>
      <p class="text-text-muted mb-3 text-sm">{t('tasks.job.desc')}</p>

      {/* Sin imagen construida no hay job posible: se dice, en vez de ofrecer un
          formulario que el control-plane rechazaría. */}
      {images.length === 0 ? (
        <Card class="mb-4">
          <CardBody class="p-0">
            <EmptyState
              illustration={<Icon name="clock" size={24} />}
              title={t('functions.noImages.title')}
              description={t('tasks.job.noImages.desc')}
              action={
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    window.location.href = r(`/projects/${project}/deployments`);
                  }}
                >
                  {t('functions.empty.cta')}
                </Button>
              }
            />
          </CardBody>
        </Card>
      ) : (
        <Card class="mb-4">
          <CardHeader>
            <CardTitle as="h3">{t('tasks.job.new')}</CardTitle>
          </CardHeader>
          <CardBody>
            <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void create(e)}>
              <FieldGroup
                label={t('common.name')}
                htmlFor="job-name"
                class="w-52"
                {...(nameError ? { error: nameError } : {})}
              >
                <Input
                  id="job-name"
                  value={name}
                  required
                  placeholder="migracion-2026-08"
                  class="font-mono"
                  invalid={Boolean(nameError)}
                  onInput={(e) => setName(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup
                label={t('functions.field.image')}
                htmlFor="job-image"
                class="min-w-[16rem] flex-1"
              >
                <Select
                  id="job-image"
                  value={image}
                  onChange={(e) => setImage(e.currentTarget.value)}
                >
                  {images.map((img) => (
                    <option key={img.ref} value={img.ref}>
                      {t('functions.image.option', {
                        commit: img.commit,
                        ref: img.gitRef,
                        date: img.createdAt.slice(0, 10),
                      })}
                    </option>
                  ))}
                </Select>
              </FieldGroup>
              <FieldGroup
                label={t('tasks.field.command')}
                htmlFor="job-command"
                class="min-w-[14rem] flex-1"
              >
                <Input
                  id="job-command"
                  value={command}
                  placeholder="node scripts/migrar.js"
                  class="font-mono"
                  onInput={(e) => setCommand(e.currentTarget.value)}
                />
              </FieldGroup>
              <Button
                type="submit"
                variant="primary"
                class="shrink-0"
                loading={busy}
                leadingIcon={<Icon name="plus" size={16} />}
              >
                {t('common.create')}
              </Button>
            </form>
          </CardBody>
        </Card>
      )}

      <TaskList
        items={items}
        t={t}
        locale={locale}
        emptyTitle={t('tasks.job.empty.title')}
        emptyDesc={t('tasks.job.empty.desc')}
        onDelete={onDelete}
        deleting={deleting}
      />
    </section>
  );
}

// ---- Lista compartida por las dos secciones --------------------------------

/**
 * «Ejecutar ahora»: lo que faltaba para poder PROBAR una tarea.
 *
 * Un cron recién escrito solo se podía comprobar esperando a su hora o
 * cambiándole el horario para que cayera pronto —y acordándose de devolverlo—; un
 * job terminado, borrándolo y creándolo otra vez. Corre exactamente la misma
 * imagen y el mismo comando que la ejecución automática, así que probarlo aquí
 * prueba lo que va a pasar de madrugada. Lo que escriba se lee con el botón de al
 * lado.
 */
function RunNowButton({ service, name, t }: { service: string; name: string; t: T }) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/run', { service });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('tasks.run.done', { name }),
        ...demoDesc(t, res.live, t('tasks.run.done.desc')),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('tasks.run.fail', { name }) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Button
      variant="ghost"
      class="text-sm"
      loading={busy}
      disabled={busy}
      aria-label={t('tasks.run.aria', { name })}
      onClick={() => void run()}
    >
      {t('tasks.run.action')}
    </Button>
  );
}

function TaskList({
  items,
  t,
  locale,
  emptyTitle,
  emptyDesc,
  onDelete,
  deleting,
}: {
  items: ManagedService[];
  t: T;
  locale: Locale;
  emptyTitle: string;
  emptyDesc: string;
  onDelete: (s: ManagedService) => void;
  deleting: boolean;
}) {
  if (items.length === 0) {
    return (
      <Card>
        <CardBody class="p-0">
          <EmptyState
            illustration={<Icon name="clock" size={24} />}
            title={emptyTitle}
            description={emptyDesc}
          />
        </CardBody>
      </Card>
    );
  }
  return (
    <Card>
      <CardBody class="p-0">
        <ul>
          {items.map((s) => (
            <li
              key={s.id ?? s.name}
              class="border-border hover:bg-bg-subtle border-b px-4 py-3 transition-colors last:border-0"
            >
              <div class="flex flex-wrap items-center gap-3">
                <span
                  class="border-border bg-bg-subtle text-text-subtle grid h-8 w-8 shrink-0 place-items-center rounded-md border"
                  aria-hidden="true"
                >
                  <Icon name="clock" size={16} />
                </span>
                <div class="min-w-0 flex-1">
                  <p class="text-text flex items-center gap-2 font-medium">
                    <span class="truncate">{s.name}</span>
                    {s.schedule ? (
                      <code class="bg-bg-subtle text-text-muted rounded-sm px-1.5 py-0.5 font-mono text-xs">
                        {s.schedule}
                      </code>
                    ) : null}
                  </p>
                  {/* Qué ejecuta: la dirección que llama, o la imagen que corre.
                      Sin esto dos tareas del mismo proyecto son indistinguibles. */}
                  <p class="text-text-muted mt-0.5 truncate text-sm" title={s.targetUrl ?? s.image}>
                    {s.targetUrl ?? s.image ?? t('tasks.target.unknown')}
                  </p>
                </div>
                <div class="ml-auto flex shrink-0 items-center gap-2">
                  <Badge tone={statusTone(s.status)}>{statusLabel(t, s.status)}</Badge>
                  {/* Una tarea que falla cada noche solo se puede depurar leyendo
                      lo que escribió, y eso no estaba en ninguna pantalla. */}
                  {s.id ? <RunNowButton service={s.id} name={s.name} t={t} /> : null}
                  {s.id ? (
                    <ServiceLogsModal serviceId={s.id} serviceName={s.name} locale={locale} />
                  ) : null}
                  {s.id ? (
                    <Button
                      variant="ghost"
                      class="text-danger-text text-sm"
                      disabled={deleting}
                      onClick={() => onDelete(s)}
                    >
                      {t('common.delete')}
                    </Button>
                  ) : null}
                </div>
              </div>
            </li>
          ))}
        </ul>
      </CardBody>
    </Card>
  );
}

// ---- Procesos permanentes (workers) ----------------------------------------

/**
 * Un worker es lo que consume una cola: no atiende peticiones y no termina.
 *
 * Vive aquí y no en Funciones porque lo que comparte con un cron y un job es lo
 * importante —código del proyecto que corre SIN que nadie visite una URL—; lo que
 * comparte con una función es solo que no se para.
 */
function WorkerSection({
  project,
  items,
  images,
  t,
  locale,
  toast,
  onCreated,
  onDelete,
  deleting,
}: SectionProps) {
  const [name, setName] = useState('');
  const [image, setImage] = useState(images[0]?.ref ?? '');
  const [command, setCommand] = useState('');
  const [replicas, setReplicas] = useState('1');
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  async function create(e: Event) {
    e.preventDefault();
    const clean = name.trim().toLowerCase();
    if (!TASK_NAME.test(clean)) {
      setNameError(t('tasks.error.name'));
      return;
    }
    setNameError(undefined);
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; service: ManagedService | null }>(
        '/api/services/create',
        {
          project,
          kind: 'worker',
          name: clean,
          config: {
            image,
            replicas,
            ...(command.trim() ? { command: command.trim() } : {}),
          },
        },
      );
      onCreated(res.service ?? { kind: 'worker', name: clean, status: 'provisioning', image });
      setName('');
      setCommand('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('tasks.worker.created', { name: clean }),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', ...createErrorToast(t, err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-labelledby="workers-heading">
      <div class="mb-1 flex items-baseline justify-between gap-3">
        <h2 id="workers-heading" class="sytue-h4 text-text">
          {t('tasks.worker.heading')}
        </h2>
        <span class="text-text-muted text-sm tabular-nums">
          {t('tasks.count', { count: items.length })}
        </span>
      </div>
      <p class="text-text-muted mb-3 text-sm">{t('tasks.worker.desc')}</p>

      {images.length === 0 ? (
        <Card class="mb-4">
          <CardBody class="p-0">
            <EmptyState
              illustration={<Icon name="clock" size={24} />}
              title={t('functions.noImages.title')}
              description={t('tasks.worker.noImages.desc')}
              action={
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    window.location.href = r(`/projects/${project}/deployments`);
                  }}
                >
                  {t('functions.empty.cta')}
                </Button>
              }
            />
          </CardBody>
        </Card>
      ) : (
        <Card class="mb-4">
          <CardHeader>
            <CardTitle as="h3">{t('tasks.worker.new')}</CardTitle>
          </CardHeader>
          <CardBody>
            <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void create(e)}>
              <FieldGroup
                label={t('common.name')}
                htmlFor="worker-name"
                class="w-52"
                {...(nameError ? { error: nameError } : {})}
              >
                <Input
                  id="worker-name"
                  value={name}
                  required
                  placeholder="consumidor-cola"
                  class="font-mono"
                  invalid={Boolean(nameError)}
                  onInput={(e) => setName(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup
                label={t('functions.field.image')}
                htmlFor="worker-image"
                class="min-w-[16rem] flex-1"
              >
                <Select
                  id="worker-image"
                  value={image}
                  onChange={(e) => setImage(e.currentTarget.value)}
                >
                  {images.map((img) => (
                    <option key={img.ref} value={img.ref}>
                      {t('functions.image.option', {
                        commit: img.commit,
                        ref: img.gitRef,
                        date: img.createdAt.slice(0, 10),
                      })}
                    </option>
                  ))}
                </Select>
              </FieldGroup>
              <FieldGroup
                label={t('tasks.field.command')}
                htmlFor="worker-command"
                class="min-w-[12rem] flex-1"
              >
                <Input
                  id="worker-command"
                  value={command}
                  placeholder="node worker.js"
                  class="font-mono"
                  onInput={(e) => setCommand(e.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup
                label={t('tasks.worker.replicas')}
                htmlFor="worker-replicas"
                class="w-28"
                help={t('tasks.worker.replicas.help')}
              >
                <Input
                  id="worker-replicas"
                  type="number"
                  min="0"
                  max="20"
                  value={replicas}
                  onInput={(e) => setReplicas(e.currentTarget.value)}
                />
              </FieldGroup>
              <Button
                type="submit"
                variant="primary"
                class="shrink-0"
                loading={busy}
                leadingIcon={<Icon name="plus" size={16} />}
              >
                {t('common.create')}
              </Button>
            </form>
          </CardBody>
        </Card>
      )}

      <TaskList
        items={items}
        t={t}
        locale={locale}
        emptyTitle={t('tasks.worker.empty.title')}
        emptyDesc={t('tasks.worker.empty.desc')}
        onDelete={onDelete}
        deleting={deleting}
      />
    </section>
  );
}
