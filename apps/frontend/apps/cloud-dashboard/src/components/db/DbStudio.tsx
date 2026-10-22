/**
 * DbStudio — Sytue DB Studio: explorador de tablas + editor SQL + API
 * auto-generada/credenciales, en tres pestañas. Estilo Supabase con diseño
 * Sytue, embebido en el dashboard del proyecto. Provee ToastProvider local para
 * las acciones de copiar/ejecutar. docs/CONVENTIONS.md §14.
 */
import { useState } from 'preact/hooks';
import {
  ToastProvider,
  Icon,
  SectionHeader,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  EmptyState,
  Button,
  Input,
  Modal,
  InlineHelp,
  useToast,
} from '@sytue/ui';
import { Tabs } from '@sytue/ui';
import TableBrowser from './TableBrowser.js';
import SqlEditor from './SqlEditor.js';
import ApiPanel from './ApiPanel.js';
import RestorePanel from './RestorePanel.js';
import DemoBanner from '../DemoBanner.js';
import type { Locale } from '@sytue/i18n';
import { useT } from '../../i18n/index.js';
import { postBff, notifyDemoIfNeeded } from '../../lib/bff-client.js';
import type { DbApiInfo, DbRows, DbTable } from '../../lib/types.js';

interface DbStudioProps {
  /** Slug del proyecto (para las consultas al BFF de datos y SQL). */
  project: string;
  tables: DbTable[];
  rowsByTable: Record<string, DbRows>;
  api: DbApiInfo;
  live: boolean;
  /** ID del servicio Postgres gestionado (para restaurar backups). '' en demo. */
  serviceId: string;
  /** Nombre del servicio Postgres (confirmación del restore destructivo). */
  serviceName: string;
  /** Programación de la copia diaria; vacío = el defecto (3:00). */
  backupSchedule: string;
  /**
   * Si el proyecto TIENE una base de datos aprovisionada. Cuando no la tiene esta
   * pantalla no puede enseñar tablas de nadie: ofrece crearla, que es lo que hace
   * honesto que «Base de datos» esté en el menú.
   */
  provisioned: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function DbStudio({
  project,
  tables,
  rowsByTable,
  api,
  live,
  serviceId,
  serviceName,
  backupSchedule,
  provisioned,
  locale,
  error,
}: DbStudioProps) {
  const t = useT(locale);
  return (
    <ToastProvider>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="database" size={20} />}
        title="Sytue DB Studio"
        subtitle={t('db.subtitle')}
        actions={
          provisioned ? (
            <RestorePanel serviceId={serviceId} serviceName={serviceName} locale={locale} />
          ) : null
        }
      />
      <div class="cloud-content">
        {provisioned ? (
          <Tabs
            items={[
              {
                id: 'tables',
                label: t('db.tab.tables'),
                icon: <Icon name="table" size={16} />,
                content: (
                  <TableBrowser
                    project={project}
                    tables={tables}
                    rowsByTable={rowsByTable}
                    t={t}
                    locale={locale}
                  />
                ),
              },
              {
                id: 'sql',
                label: t('db.tab.sql'),
                icon: <Icon name="code" size={16} />,
                content: <SqlEditor project={project} t={t} />,
              },
              {
                id: 'api',
                label: t('db.tab.api'),
                icon: <Icon name="lock" size={16} />,
                content: <ApiPanel project={project} api={api} tables={tables} t={t} />,
              },
            ]}
          />
        ) : (
          <ProvisionDb project={project} locale={locale} />
        )}
        {provisioned && serviceId ? (
          <BackupSchedule serviceId={serviceId} schedule={backupSchedule} locale={locale} />
        ) : null}
        {provisioned && serviceId ? (
          <DropDb serviceId={serviceId} serviceName={serviceName} locale={locale} />
        ) : null}
      </div>
    </ToastProvider>
  );
}

/**
 * Baja de la base de datos del proyecto.
 *
 * Se podía crear desde aquí y no retirar: una base creada por error se quedaba
 * ocupando cuota del plan y facturando su volumen. Escribir el nombre no es
 * fricción decorativa —el volumen se va con ella y no hay deshacer—, y es la
 * misma confirmación de dos fases que ya pide el restore.
 */
function DropDb({
  serviceId,
  serviceName,
  locale,
}: {
  serviceId: string;
  serviceName: string;
  locale: Locale;
}) {
  const t = useT(locale);
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const confirmed = typed.trim() === serviceName;

  async function drop() {
    if (!confirmed) return;
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/delete', {
        service: serviceId,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      if (res.live) {
        // Ya no hay base: recargar deja la pantalla en el estado real (ofrecer
        // crearla) en vez de enseñar tablas de algo que se está retirando.
        window.location.reload();
        return;
      }
      setOpen(false);
      setTyped('');
    } catch {
      toast.notify({ tone: 'danger', title: t('db.drop.fail') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {/* `border-danger` y no el token `danger-border`, que no existe en
          theme.css, así que la clase no llegaba a generarse y la zona de
          peligro se quedaba con el pelo gris de cualquier otra tarjeta. */}
      <Card class="border-danger mt-6">
        <CardHeader>
          <CardTitle as="h2">
            <span class="flex items-center gap-2">
              <Icon name="warning" size={18} />
              {t('danger.title')}
            </span>
          </CardTitle>
        </CardHeader>
        <CardBody class="flex flex-wrap items-center justify-between gap-4">
          <div class="min-w-0">
            <p class="text-text font-medium">{t('db.drop.title')}</p>
            <p class="text-text-muted mt-1 text-sm">{t('db.drop.desc')}</p>
          </div>
          <Button variant="danger" class="shrink-0" onClick={() => setOpen(true)}>
            {t('db.drop.action')}
          </Button>
        </CardBody>
      </Card>

      <Modal
        open={open}
        onClose={() => {
          setOpen(false);
          setTyped('');
        }}
        title={t('db.drop.confirm.title')}
        size="sm"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setOpen(false);
                setTyped('');
              }}
              disabled={busy}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              loading={busy}
              disabled={!confirmed || busy}
              onClick={() => void drop()}
            >
              {t('db.drop.confirm.action')}
            </Button>
          </>
        }
      >
        <InlineHelp tone="danger" title={t('danger.confirm.warning')}>
          {t('db.drop.confirm.body')}
        </InlineHelp>
        <label for="drop-db-confirm" class="text-text mt-4 mb-1 block text-sm font-semibold">
          {t('danger.confirm.label', { slug: serviceName })}
        </label>
        <Input
          id="drop-db-confirm"
          value={typed}
          class="font-mono"
          autocomplete="off"
          placeholder={serviceName}
          onInput={(e) => setTyped(e.currentTarget.value)}
        />
      </Modal>
    </>
  );
}

/**
 * Alta de la base de datos del proyecto.
 *
 * El SDK ya sabía aprovisionar servicios gestionados y no lo llamaba nadie: el
 * menú del proyecto ofrecía «Base de datos» y, sin ninguna, esta pantalla enseñaba
 * las tablas del ejemplo como si fueran del usuario. Ahora o hay base de datos, o
 * se crea desde aquí.
 */
function ProvisionDb({ project, locale }: { project: string; locale: Locale }) {
  const t = useT(locale);
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function provision() {
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/create', {
        project,
        kind: 'postgres',
      });
      notifyDemoIfNeeded(t, toast, res.live);
      if (res.live) {
        // El aprovisionamiento lo hace el control-plane en segundo plano: se
        // recarga para que la pantalla lea el estado REAL del servicio en vez de
        // pintar una base de datos que quizá aún no está lista.
        window.location.reload();
        return;
      }
      toast.notify({ tone: 'info', title: t('db.provision.started') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('db.provision.fail'),
        description: t('common.retry'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardBody class="p-0">
        <EmptyState
          illustration={<Icon name="database" size={24} />}
          title={t('db.provision.title')}
          description={t('db.provision.desc')}
          action={
            <Button
              variant="primary"
              loading={busy}
              leadingIcon={<Icon name="plus" size={16} />}
              onClick={() => void provision()}
            >
              {t('db.provision.cta')}
            </Button>
          }
        />
      </CardBody>
    </Card>
  );
}

/**
 * Cuándo se hace la copia diaria.
 *
 * Estaba escrito a mano en el provisioner —las 3:00 para todo el mundo—, así que
 * una app con tráfico nocturno se llevaba el `pg_dump` en su hora punta. Ahora se
 * elige aquí, con las mismas frecuencias que las tareas programadas y campo libre
 * para quien sepa cron.
 */
function BackupSchedule({
  serviceId,
  schedule,
  locale,
}: {
  serviceId: string;
  schedule: string;
  locale: Locale;
}) {
  const t = useT(locale);
  const toast = useToast();
  const [value, setValue] = useState(schedule || '0 3 * * *');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  async function save(e: Event) {
    e.preventDefault();
    const cron = value.trim();
    // Mismo criterio superficial que el panel de tareas y que el control-plane:
    // macro conocida o cinco campos. El parser de verdad es el de Kubernetes.
    const ok = cron.startsWith('@')
      ? ['@hourly', '@daily', '@midnight', '@weekly', '@monthly', '@yearly', '@annually'].includes(
          cron,
        )
      : cron.split(/\s+/).length === 5;
    if (!ok) {
      setError(t('db.backup.schedule.invalid'));
      return;
    }
    setError(undefined);
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/update', {
        service: serviceId,
        config: { backup_schedule: cron },
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({ tone: 'success', title: t('db.backup.schedule.saved') });
    } catch {
      toast.notify({ tone: 'danger', title: t('db.backup.schedule.fail') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card class="mt-6">
      <CardHeader>
        <CardTitle as="h2">{t('db.backup.schedule.title')}</CardTitle>
      </CardHeader>
      <CardBody>
        <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void save(e)}>
          <div class="w-56">
            <label for="backup-schedule" class="text-text mb-1 block text-sm font-semibold">
              {t('db.backup.schedule.label')}
            </label>
            <Input
              id="backup-schedule"
              value={value}
              class="font-mono"
              invalid={Boolean(error)}
              onInput={(e) => setValue(e.currentTarget.value)}
            />
          </div>
          <Button type="submit" variant="secondary" class="shrink-0" loading={busy}>
            {t('common.save')}
          </Button>
          <p class="text-text-subtle w-full text-sm">{error ?? t('db.backup.schedule.help')}</p>
        </form>
      </CardBody>
    </Card>
  );
}
