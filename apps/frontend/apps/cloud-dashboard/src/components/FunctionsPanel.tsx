/**
 * FunctionsPanel — Sytue Functions del proyecto.
 *
 * Una función es un contenedor HTTP con dirección propia y TLS
 * (`<proyecto>-<fn>.fn.<dominio>`), levantado desde una imagen que el proyecto YA
 * construyó y con las credenciales del proyecto inyectadas (base de datos,
 * almacenamiento). Eso es lo que hace el control-plane desde la Ola 7; esta
 * pantalla decía «hasta que el control-plane exponga funciones» y ofrecía irse a
 * Despliegues, así que la función estaba entregada y apagada.
 *
 * La imagen NO se teclea: se elige un commit ya construido. Una referencia OCI del
 * registro privado no es algo que nadie sepa de memoria, y equivocarse deja un
 * servicio en «provisioning» para siempre.
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
  InlineHelp,
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
 * Misma regla que `domain.ValidSlug` en el control-plane: el nombre ES la etiqueta
 * DNS del host público y del certificado. Se valida aquí para decirlo antes de
 * mandar, no para sustituir al servidor —que la vuelve a exigir—.
 */
const FN_NAME = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

interface Props {
  project: string;
  functions: ManagedService[];
  images: BuiltImage[];
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function FunctionsPanel(props: Props) {
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

function Inner({ project, functions, images, live, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<ManagedService[]>(functions);
  const [name, setName] = useState('');
  const [image, setImage] = useState(images[0]?.ref ?? '');
  const [command, setCommand] = useState('');
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<ManagedService | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function create(e: Event) {
    e.preventDefault();
    const clean = name.trim().toLowerCase();
    if (!FN_NAME.test(clean)) {
      setNameError(t('functions.error.name'));
      return;
    }
    setNameError(undefined);
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; service: ManagedService | null }>(
        '/api/services/create',
        {
          project,
          kind: 'functions',
          name: clean,
          config: { image, ...(command.trim() ? { command: command.trim() } : {}) },
        },
      );
      // La URL la compone el control-plane al aprovisionar: sin backend vivo no
      // hay ninguna que enseñar, y una inventada aquí sería una dirección que el
      // usuario llamaría y no respondería nunca.
      setItems((prev) => [
        ...prev,
        res.service ?? { kind: 'functions', name: clean, status: 'provisioning', image },
      ]);
      setName('');
      setCommand('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('functions.created', { name: clean }),
        ...demoDesc(t, res.live, t('functions.created.desc')),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', ...createErrorToast(t, err) });
    } finally {
      setBusy(false);
    }
  }

  async function confirmDelete() {
    const target = pendingDelete;
    if (!target?.id) return;
    setDeleting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/delete', {
        service: target.id,
      });
      setItems((prev) => prev.filter((s) => s.id !== target.id));
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('functions.deleted', { name: target.name }),
        ...demoDesc(t, res.live),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('functions.delete.fail') });
    } finally {
      setDeleting(false);
    }
  }

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="code" size={20} />}
        title={t('nav.functions')}
        subtitle={t('functions.subtitle')}
      />
      <div class="cloud-content">
        {/* Sin una imagen construida no hay función posible, y decirlo es más útil
            que enseñar un formulario que va a fallar al enviarse. */}
        {images.length === 0 ? (
          <Card class="mb-6">
            <CardBody class="p-0">
              <EmptyState
                illustration={<Icon name="code" size={24} />}
                title={t('functions.noImages.title')}
                description={t('functions.noImages.desc')}
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
          <Card class="mb-6">
            <CardHeader>
              <CardTitle as="h2">{t('functions.new')}</CardTitle>
            </CardHeader>
            <CardBody>
              <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void create(e)}>
                <FieldGroup
                  label={t('functions.field.name')}
                  htmlFor="fn-name"
                  class="w-56"
                  help={t('functions.field.name.hint')}
                  {...(nameError ? { error: nameError } : {})}
                >
                  <Input
                    id="fn-name"
                    value={name}
                    required
                    placeholder="api"
                    class="font-mono"
                    invalid={Boolean(nameError)}
                    onInput={(e) => setName(e.currentTarget.value)}
                  />
                </FieldGroup>
                <FieldGroup
                  label={t('functions.field.image')}
                  htmlFor="fn-image"
                  class="min-w-[18rem] flex-1"
                  help={t('functions.field.image.hint')}
                >
                  <Select
                    id="fn-image"
                    value={image}
                    onChange={(e) => setImage(e.currentTarget.value)}
                  >
                    {images.map((img) => (
                      <option key={img.ref} value={img.ref}>
                        {t('functions.image.option', {
                          commit: img.commit,
                          ref: img.gitRef,
                          date: new Date(img.createdAt).toLocaleDateString(locale),
                        })}
                      </option>
                    ))}
                  </Select>
                </FieldGroup>
                <FieldGroup
                  label={t('functions.field.command')}
                  htmlFor="fn-command"
                  class="min-w-[14rem] flex-1"
                  help={t('functions.field.command.hint')}
                >
                  <Input
                    id="fn-command"
                    value={command}
                    placeholder="node server.js"
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

        {items.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                illustration={<Icon name="code" size={24} />}
                title={t('functions.empty.title')}
                description={t('functions.empty.desc')}
              />
            </CardBody>
          </Card>
        ) : (
          <Card>
            <CardBody class="p-0">
              <ul>
                {items.map((fn) => (
                  <li
                    key={fn.id ?? fn.name}
                    class="border-border hover:bg-bg-subtle border-b px-4 py-3 transition-colors last:border-0"
                  >
                    <div class="flex flex-wrap items-center gap-3">
                      <span
                        class="border-border bg-bg-subtle text-text-subtle grid h-8 w-8 shrink-0 place-items-center rounded-md border"
                        aria-hidden="true"
                      >
                        <Icon name="code" size={16} />
                      </span>
                      <div class="min-w-0 flex-1">
                        <p class="text-text truncate font-medium">{fn.name}</p>
                        {fn.url ? (
                          <a
                            class="text-text-muted hover:text-text mt-0.5 block truncate font-mono text-sm underline-offset-2 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--sytue-focus-ring)]"
                            href={fn.url}
                            target="_blank"
                            rel="noreferrer"
                          >
                            {fn.url}
                          </a>
                        ) : (
                          // Mientras se aprovisiona no hay dirección todavía: se
                          // dice, en vez de dejar el hueco mudo.
                          <p class="text-text-subtle mt-0.5 text-sm">
                            {t('functions.url.pending')}
                          </p>
                        )}
                      </div>
                      <div class="ml-auto flex shrink-0 items-center gap-2">
                        <Badge tone={statusTone(fn.status)}>{statusLabel(t, fn.status)}</Badge>
                        {/* Sin esto, una función que no arranca es un callejón sin
                            salida: el estado se queda en «provisioning» y el motivo
                            solo existe dentro del clúster. */}
                        {fn.id ? (
                          <ServiceLogsModal
                            serviceId={fn.id}
                            serviceName={fn.name}
                            locale={locale}
                          />
                        ) : null}
                        {fn.id ? (
                          <Button
                            variant="ghost"
                            class="text-danger-text text-sm"
                            disabled={deleting}
                            onClick={() => setPendingDelete(fn)}
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
        )}

        <InlineHelp class="mt-4">{t('functions.credentials')}</InlineHelp>
      </div>

      {/* Confirmación: retirar una función tumba su dirección pública. */}
      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('functions.confirm.title')}
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
          {t('functions.confirm.body', { name: pendingDelete?.name ?? '' })}
        </p>
      </Modal>
    </>
  );
}

/** Código HTTP embebido por postBff en el mensaje del error (bff_<status>). */
function statusOf(e: unknown): number {
  const m = /bff_(\d+)/.exec(e instanceof Error ? e.message : String(e));
  return m ? Number(m[1]) : 0;
}

/**
 * El error del alta, dicho en sus términos. Los tres que el usuario puede
 * arreglar solo —no soy admin, se agotó la cuota del plan, ya existe una con ese
 * nombre— merecen su frase; el resto cae en el genérico.
 */
function createErrorToast(t: T, err: unknown): { title: string; description?: string } {
  switch (statusOf(err)) {
    case 403:
      return { title: t('functions.create.forbidden') };
    case 402:
      return { title: t('functions.create.quota') };
    case 409:
      return { title: t('functions.create.conflict') };
    default:
      return { title: t('functions.create.fail'), description: t('common.retry') };
  }
}
