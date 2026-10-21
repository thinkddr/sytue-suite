/**
 * SourcePanel — de dónde sale el código y cómo llega solo: repositorio, rama de
 * producción y despliegue en cada push.
 *
 * Las dos mitades faltaban por motivos distintos:
 *
 *  - **Repositorio y rama** se fijan en el alta y no se podían cambiar después:
 *    equivocarse de URL o querer pasar de `master` a `main` obligaba a crear otro
 *    proyecto, aunque `PATCH /projects/{id}` acepta los dos campos desde el
 *    principio.
 *  - **El comando de release** —lo que corre ANTES de arrancar la versión nueva,
 *    donde va una migración de base de datos— no existía en ninguna pantalla ni
 *    en la API.
 *  - **El webhook** (lo que hace que un push despliegue) solo se enseñaba en el
 *    aviso del alta. Cerrabas la pestaña y el secreto no volvía a aparecer: el
 *    control-plane lo devuelve en `GET /projects/{id}` a un admin/owner, pero
 *    ninguna pantalla lo pedía.
 *
 * Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Button,
  Icon,
  Input,
  FieldGroup,
  InlineHelp,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { PushDeployInfo } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import { CopyField } from './CopyField.js';

/** Nombre de rama/etiqueta: mismo criterio que el BFF (REF_RE). */
const REF_RE = /^(?!.*\.\.)[A-Za-z0-9][A-Za-z0-9._\-/]{0,254}$/;

interface Props {
  project: string;
  repo: string;
  productionRef: string;
  /** Comando previo al arranque de la versión nueva; vacío = no hay. */
  releaseCommand: string;
  /** Si las previews del proyecto ya piden contraseña. */
  previewProtected: boolean;
  push: PushDeployInfo;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function SourcePanel(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

/** Código HTTP embebido por postBff en el mensaje del error (bff_<status>). */
function statusOf(e: unknown): number {
  const m = /bff_(\d+)/.exec(e instanceof Error ? e.message : String(e));
  return m ? Number(m[1]) : 0;
}

function saveErrorTitle(t: T, err: unknown): string {
  return statusOf(err) === 403 ? t('source.save.forbidden') : t('source.save.fail');
}

function Inner({
  project,
  repo,
  productionRef,
  releaseCommand,
  previewProtected,
  push,
  live,
  locale,
}: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [repoUrl, setRepoUrl] = useState(repo);
  const [ref, setRef] = useState(productionRef);
  const [release, setRelease] = useState(releaseCommand);
  const [errors, setErrors] = useState<{ repo?: string; ref?: string }>({});
  const [busy, setBusy] = useState(false);
  // El secreto vivo: el que llegó del servidor o el que acaba de emitirse.
  const [secret, setSecret] = useState(push.webhookSecret);
  const [rotating, setRotating] = useState(false);
  const [confirmRotate, setConfirmRotate] = useState(false);
  // Protección de previews: estado vivo + la credencial recién emitida, que solo
  // existe en esta pantalla y hasta que se recargue.
  const [guarded, setGuarded] = useState(previewProtected);
  const [creds, setCreds] = useState<{ username: string; password: string } | null>(null);
  const [guarding, setGuarding] = useState(false);

  const dirty =
    repoUrl.trim() !== repo || ref.trim() !== productionRef || release.trim() !== releaseCommand;

  async function save(e: Event) {
    e.preventDefault();
    const nextRepo = repoUrl.trim();
    const nextRef = ref.trim();
    const next: typeof errors = {};
    if (!nextRepo.startsWith('https://')) next.repo = t('source.error.repo');
    if (!REF_RE.test(nextRef)) next.ref = t('source.error.ref');
    setErrors(next);
    if (Object.keys(next).length > 0) return;

    setBusy(true);
    try {
      // Solo viaja lo que ha cambiado: la API trata lo ausente como «no lo
      // toques», y mandar el par entero reescribiría el campo que nadie editó.
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/project-runtime', {
        project,
        ...(nextRepo !== repo ? { repo_url: nextRepo } : {}),
        ...(nextRef !== productionRef ? { production_ref: nextRef } : {}),
        // Vacío es un valor válido (quitar el comando), así que se manda cuando
        // CAMBIA, no cuando tiene contenido.
        ...(release.trim() !== releaseCommand ? { release_command: release.trim() } : {}),
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('source.saved'),
        ...demoDesc(t, res.live, t('source.saved.desc')),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', title: saveErrorTitle(t, err) });
    } finally {
      setBusy(false);
    }
  }

  async function rotate() {
    setRotating(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; secret?: string }>(
        '/api/webhook-secret',
        { project },
      );
      notifyDemoIfNeeded(t, toast, res.live);
      if (res.secret) setSecret(res.secret);
      setConfirmRotate(false);
      toast.notify({
        tone: 'success',
        title: t('source.push.rotated'),
        ...demoDesc(t, res.live, t('source.push.rotated.desc')),
      });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title:
          statusOf(err) === 403 ? t('source.push.rotate.forbidden') : t('source.push.rotate.fail'),
      });
    } finally {
      setRotating(false);
    }
  }

  async function togglePreviewGuard(disable: boolean) {
    setGuarding(true);
    try {
      const res = await postBff<{
        ok: boolean;
        live: boolean;
        protected: boolean;
        username?: string;
        password?: string;
      }>('/api/preview-protection', { project, ...(disable ? { disable: true } : {}) });
      notifyDemoIfNeeded(t, toast, res.live);
      setGuarded(res.protected);
      setCreds(
        res.username && res.password ? { username: res.username, password: res.password } : null,
      );
      toast.notify({
        tone: 'success',
        title: disable ? t('source.preview.opened') : t('source.preview.closed'),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: statusOf(err) === 403 ? t('source.preview.forbidden') : t('source.preview.fail'),
      });
    } finally {
      setGuarding(false);
    }
  }

  return (
    <>
      <Card class="mt-6">
        <CardHeader>
          <CardTitle as="h2">{t('source.title')}</CardTitle>
        </CardHeader>
        <CardBody>
          <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void save(e)}>
            <FieldGroup
              label={t('source.field.repo')}
              htmlFor="source-repo"
              class="min-w-[20rem] flex-1"
              help={t('source.field.repo.help')}
              {...(errors.repo ? { error: errors.repo } : {})}
            >
              <Input
                id="source-repo"
                type="url"
                value={repoUrl}
                class="font-mono"
                invalid={Boolean(errors.repo)}
                onInput={(e) => setRepoUrl(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup
              label={t('source.field.ref')}
              htmlFor="source-ref"
              class="w-48"
              help={t('source.field.ref.help')}
              {...(errors.ref ? { error: errors.ref } : {})}
            >
              <Input
                id="source-ref"
                value={ref}
                class="font-mono"
                invalid={Boolean(errors.ref)}
                onInput={(e) => setRef(e.currentTarget.value)}
              />
            </FieldGroup>
            <Button
              type="submit"
              variant="primary"
              class="shrink-0"
              loading={busy}
              disabled={!dirty}
            >
              {t('common.save')}
            </Button>
            {/* El paso de release va aquí y no en «Escala» porque forma parte de
                QUÉ se despliega, no de cuánta máquina se le pone. En ancho
                completo: un comando cabe mal en un tercio de fila. */}
            <FieldGroup
              label={t('source.field.release')}
              htmlFor="source-release"
              class="w-full"
              help={t('source.field.release.help')}
            >
              <Input
                id="source-release"
                value={release}
                placeholder="npm run migrate"
                class="font-mono"
                onInput={(e) => setRelease(e.currentTarget.value)}
              />
            </FieldGroup>
          </form>
          <p class="text-text-subtle mt-2 text-sm">{t('source.applyNote')}</p>
        </CardBody>
      </Card>

      <Card class="mt-6">
        <CardHeader>
          <CardTitle as="h2">
            <span class="flex items-center gap-2">
              <Icon name="shield" size={18} />
              {t('source.preview.title')}
            </span>
          </CardTitle>
        </CardHeader>
        <CardBody>
          <p class="text-text-muted mb-3 text-sm">{t('source.preview.desc')}</p>
          {/* La credencial recién emitida vive SOLO aquí: no se puede volver a
              leer, así que se enseña hasta que alguien recargue la página. */}
          {creds ? (
            <div class="mb-3 grid gap-4 sm:grid-cols-2">
              <CopyField t={t} label={t('source.preview.user')} value={creds.username} />
              <CopyField t={t} label={t('source.preview.password')} value={creds.password} />
            </div>
          ) : null}
          {creds ? (
            <InlineHelp tone="warning" class="mb-3">
              {t('source.preview.once')}
            </InlineHelp>
          ) : null}
          <div class="flex flex-wrap items-center gap-2">
            <Button
              variant={guarded ? 'secondary' : 'primary'}
              loading={guarding}
              onClick={() => void togglePreviewGuard(false)}
            >
              {guarded ? t('source.preview.rotate') : t('source.preview.enable')}
            </Button>
            {guarded ? (
              <Button
                variant="ghost"
                class="text-danger-text"
                disabled={guarding}
                onClick={() => void togglePreviewGuard(true)}
              >
                {t('source.preview.disable')}
              </Button>
            ) : null}
            <span class="text-text-muted text-sm">
              {guarded ? t('source.preview.state.closed') : t('source.preview.state.open')}
            </span>
          </div>
        </CardBody>
      </Card>

      <Card class="mt-6">
        <CardHeader>
          <CardTitle as="h2">
            <span class="flex items-center gap-2">
              <Icon name="rocket" size={18} />
              {t('source.push.title')}
            </span>
          </CardTitle>
        </CardHeader>
        <CardBody>
          {push.webhookUrl && secret ? (
            <>
              <p class="text-text-muted mb-3 text-sm">{t('source.push.desc')}</p>
              <div class="grid gap-4 sm:grid-cols-2">
                <CopyField t={t} label={t('source.push.url')} value={push.webhookUrl} />
                {/* Copiable a propósito, y por eso NO va con `secret`: ese modo
                    enseña un preview truncado y enlaza a «Variables y secretos»,
                    donde este valor no vive. La única razón de existir del
                    secreto del webhook es pegarlo en el proveedor git. */}
                <CopyField t={t} label={t('source.push.secret')} value={secret} />
              </div>
              <InlineHelp class="mt-4" title={t('source.push.how')}>
                {t('source.push.steps')}
              </InlineHelp>
              {/* Rotar: hasta ahora un secreto filtrado no tenía arreglo salvo
                  rehacer el proyecto entero. Se avisa de la consecuencia ANTES,
                  porque el de antes deja de valer en el acto. */}
              <div class="mt-4">
                {confirmRotate ? (
                  <InlineHelp tone="warning" title={t('source.push.rotate.title')}>
                    <p>{t('source.push.rotate.body')}</p>
                    <div class="mt-3 flex gap-2">
                      <Button variant="danger" loading={rotating} onClick={() => void rotate()}>
                        {t('source.push.rotate.confirm')}
                      </Button>
                      <Button
                        variant="ghost"
                        disabled={rotating}
                        onClick={() => setConfirmRotate(false)}
                      >
                        {t('common.cancel')}
                      </Button>
                    </div>
                  </InlineHelp>
                ) : (
                  <Button variant="secondary" onClick={() => setConfirmRotate(true)}>
                    {t('source.push.rotate')}
                  </Button>
                )}
              </div>
            </>
          ) : (
            // Sin rol para verlo no es un fallo: es la respuesta correcta del
            // control-plane, y decirlo evita que alguien lo lea como avería.
            <InlineHelp tone={live ? 'info' : 'warning'}>
              {live ? t('source.push.needsAdmin') : t('source.push.noBackend')}
            </InlineHelp>
          )}
        </CardBody>
      </Card>

      {push.webhookUrl ? (
        <DeployHookCard project={project} base={push.webhookUrl} live={live} t={t} />
      ) : null}
    </>
  );
}

/**
 * URL de despliegue: una dirección secreta que publica el proyecto al recibir un
 * POST, sin cabeceras ni sesión.
 *
 * Es para lo que no puede mandar un `Authorization`: el botón «publicar» de un
 * CMS, un cron de un tercero, una automatización de las que solo piden «pega
 * aquí una URL». Con la API sola era imposible.
 *
 * Por debajo NO hay nada nuevo: es un token de despliegue confinado a este
 * proyecto (los dos ejes ya existían), presentado como URL. Por eso se revoca
 * donde se revocan los demás, en Tokens, y se dice aquí para que nadie lo busque
 * en esta pantalla.
 */
function DeployHookCard({
  project,
  base,
  live,
  t,
}: {
  project: string;
  /** URL pública del webhook git, de donde se deriva la del hook (mismo host). */
  base: string;
  live: boolean;
  t: T;
}) {
  const toast = useToast();
  const [url, setUrl] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function create() {
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; token: string }>(
        '/api/projects/deploy-hook',
        { project },
      );
      // El host lo pone el backend en la URL del webhook; aquí solo se cambia la
      // ruta. Así el panel no tiene que saber dónde vive el control-plane —que en
      // el clúster no es la dirección pública— ni queda una segunda copia que
      // mantener cuando cambie el dominio.
      setUrl(base.replace(/\/webhooks\/git$/, '/hooks/deploy/') + res.token);
    } catch (err) {
      const status = /bff_(\d+)/.exec(err instanceof Error ? err.message : '')?.[1];
      toast.notify({
        tone: 'danger',
        title: status === '403' ? t('source.hook.forbidden') : t('source.hook.fail'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card class="mt-6">
      <CardHeader>
        <CardTitle as="h2">
          <span class="flex items-center gap-2">
            <Icon name="globe" size={18} />
            {t('source.hook.title')}
          </span>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <p class="text-text-muted mb-3 text-sm">{t('source.hook.desc')}</p>
        {url ? (
          <>
            {/* Copiable, no enmascarada: su única razón de existir es pegarla en
                otro sitio, y no se vuelve a poder leer. */}
            <CopyField t={t} label={t('source.hook.url')} value={url} />
            <InlineHelp tone="warning" class="mt-3">
              {t('source.hook.once')}
            </InlineHelp>
          </>
        ) : (
          <Button variant="secondary" loading={busy} disabled={!live} onClick={() => void create()}>
            {t('source.hook.action')}
          </Button>
        )}
      </CardBody>
    </Card>
  );
}
