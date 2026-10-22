/**
 * TemplateWizard — alta guiada de un proyecto desde una plantilla del catálogo.
 *
 * Hasta aquí «usar una plantilla» solo pre-rellenaba el formulario de proyecto: el
 * usuario seguía teniendo que crear el proyecto, luego la base de datos, luego cada
 * variable, y descubrir por su cuenta que además había que desplegar. El
 * control-plane ya hace las cuatro cosas en UNA transacción
 * (POST /projects:from-template), así que esto es lo que faltaba para que sea de
 * verdad un «1 clic».
 *
 * Los pasos son los MÍNIMOS: elegir, nombrar + repositorio, revisar y listo. El
 * paso de preguntas solo existe si la plantilla declara variables `origin: "ask"`;
 * hoy ninguna del catálogo lo hace, así que el camino normal es de cero preguntas y
 * un paso vacío sería una pantalla puesta para parecer completa.
 *
 * Sobre los secretos se dice la verdad: los valores `generate` los crea y CIFRA el
 * servidor y no se devuelven en claro. Que el usuario no tenga que inventarse
 * ninguna contraseña es la ventaja; enseñarlas no es una promesa que podamos
 * cumplir. Lo único que se muestra una vez es el secreto del webhook, que existe
 * justo para que un humano lo pegue en su proveedor git.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  ToastProvider,
  Wizard,
  describedBy,
  useToast,
  type ReviewItem,
  type WizardStep,
  Checkbox,
} from '@sytue/ui';
import { formatList, type Locale } from '@sytue/i18n';
import type { TemplateApplied } from '@sytue/api-clients';
// Solo el TIPO: `lib/cloud.ts` es de servidor y un import de valor lo arrastraría
// al bundle del navegador (los `import type` se borran al compilar).
import type { StarterTemplate } from '../lib/cloud.js';
import { BffError, postBff } from '../lib/bff-client.js';
import { errorMessage, useT, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { CopyField } from './CopyField.js';

/** Etiqueta DNS: es el subdominio público, así que el formato no es negociable. */
const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

/** El control-plane acepta https:// o SSH scp-like (git@host:cuenta/repo). */
function validRepo(url: string): boolean {
  return url.startsWith('https://') || url.startsWith('git@');
}

/**
 * Propone la dirección a partir del nombre. Se quitan los acentos en vez de
 * rechazar el nombre: quien escribe «Mi Tienda Rápida» no tiene por qué saber que
 * un subdominio no admite tildes.
 */
function toSlug(name: string): string {
  return name
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .slice(0, 40)
    .replace(/^-+|-+$/g, '');
}

/** Nombres propios de stack: no se traducen. Solo «estático» es una palabra. */
const FRAMEWORK_NAME: Record<string, string> = {
  astro: 'Astro',
  next: 'Next.js',
  preact: 'Preact',
  node: 'Node',
  sveltekit: 'SvelteKit',
  go: 'Go',
};

function frameworkLabel(t: T, fw: string): string {
  if (!fw) return t('templates.autoDetect');
  return fw === 'static' ? t('framework.static') : (FRAMEWORK_NAME[fw] ?? fw);
}

function serviceLabel(t: T, kind: string): string {
  switch (kind) {
    case 'postgres':
      return t('service.postgres');
    case 'storage':
      return t('service.storage');
    case 'cache':
      return t('service.cache');
    case 'functions':
      return t('service.functions');
    case 'cron':
      return t('service.cron');
    case 'auth':
      return t('service.auth');
    default:
      return kind;
  }
}

interface Props {
  templates: StarterTemplate[];
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function TemplateWizard(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function Inner({ templates, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();

  const [step, setStep] = useState(0);
  const [templateId, setTemplateId] = useState('');
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  /** Una vez editada a mano, la dirección deja de seguir al nombre. */
  const [slugEdited, setSlugEdited] = useState(false);
  const [repoUrl, setRepoUrl] = useState('');
  /**
   * `true` cuando el usuario no tiene repositorio y pide que se lo creemos.
   *
   * Es la tercera vía del alta, y vive aquí y no en un formulario aparte porque
   * este es el sitio donde ya se está eligiendo el stack: la pregunta «¿tienes
   * repositorio?» solo tiene sentido después de saber QUÉ se va a desplegar.
   */
  const [crearRepo, setCrearRepo] = useState(false);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  /**
   * Alta ya hecha. Mientras no sea null, el asistente es la ÚNICA pantalla donde
   * existe el secreto del webhook: el control-plane no lo entrega dos veces.
   */
  const [created, setCreated] = useState<TemplateApplied | null>(null);
  const [slugStatus, setSlugStatus] = useState<'idle' | 'checking' | 'free' | 'taken' | 'unknown'>(
    'idle',
  );
  const [alternatives, setAlternatives] = useState<string[]>([]);

  const template = useMemo(
    () => templates.find((x) => x.id === templateId),
    [templates, templateId],
  );
  // `ask` es lo ÚNICO que el usuario tiene que responder: `value` es literal del
  // manifiesto, `generate` lo crea el servidor y `from_service` lo inyecta el
  // controlador. Vacío = el asistente no tiene paso de preguntas.
  const questions = useMemo(
    () => template?.env.filter((e) => e.origin === 'ask') ?? [],
    [template],
  );

  /**
   * ¿Está libre la dirección? El slug es único en TODA la plataforma, así que sin
   * esto el usuario no se entera hasta el 409 —y ese 409 no puede decirle que el
   * nombre lo tiene otra organización—. Con debounce: una consulta por tecla
   * convertiría escribir «mi-tienda» en nueve peticiones cross-tenant.
   */
  useEffect(() => {
    const s = slug.trim().toLowerCase();
    setAlternatives([]);
    if (!s || !SLUG_RE.test(s)) {
      setSlugStatus('idle');
      return;
    }
    setSlugStatus('checking');
    let cancelado = false;
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const res = await postBff<{
            ok: boolean;
            live: boolean;
            available?: boolean;
            alternatives?: string[];
          }>('/api/check-slug', { slug: s, lang: locale });
          if (cancelado) return;
          // `unknown` NO es «libre»: sin control-plane nadie puede saberlo y decir
          // que sí sería mentir justo en el dato por el que se pregunta.
          if (!res.live) {
            setSlugStatus('unknown');
            return;
          }
          setSlugStatus(res.available ? 'free' : 'taken');
          setAlternatives(res.alternatives ?? []);
        } catch {
          if (!cancelado) setSlugStatus('unknown');
        }
      })();
    }, 400);
    return () => {
      cancelado = true;
      clearTimeout(timer);
    };
  }, [slug, locale]);

  function pickTemplate(tpl: StarterTemplate) {
    setTemplateId(tpl.id);
    // Las respuestas son de la plantilla elegida: arrastrar las de otra sería
    // mandar claves que la nueva no declara, y eso el control-plane lo rechaza.
    setAnswers({});
    // Y «créamelo» solo existe si la nueva trae esqueleto. Sin esto, cambiar de
    // plantilla dejaba la casilla marcada Y deshabilitada —imposible de quitar—
    // pidiendo un repositorio que nadie puede copiar.
    if (!tpl.hasStarter) setCrearRepo(false);
  }

  function changeName(value: string) {
    setName(value);
    if (!slugEdited) setSlug(toSlug(value));
  }

  const slugOk = SLUG_RE.test(slug.trim().toLowerCase()) && slugStatus !== 'taken';
  // Con «créamelo» no hay URL que validar: el repositorio todavía no existe, y
  // exigirla dejaría el botón apagado sin nada que el usuario pudiera hacer.
  const repoOk = crearRepo || validRepo(repoUrl.trim());
  const answersOk = questions.every((q) => (answers[q.key] ?? '').trim() !== '');
  /** Índice del paso terminal: con preguntas hay uno más por el medio. */
  const doneStepIndex = questions.length > 0 ? 4 : 3;

  /**
   * Pide que Sytue DevX cree el repositorio y manda a autorizarlo.
   *
   * Sale del panel a propósito y con `location.assign`: al otro lado hay una
   * pantalla de consentimiento de la forja, y eso no se pinta dentro de un iframe
   * ni se sigue por fetch. Se vuelve por `/api/forge/callback`, que redirige al
   * proyecto ya creado.
   */
  async function crearConRepo() {
    if (!template) return;
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; authorize_url?: string }>(
        '/api/projects/scaffold',
        {
          template_id: template.id,
          slug: slug.trim().toLowerCase(),
          name: name.trim() || slug.trim().toLowerCase(),
          repo_name: slug.trim().toLowerCase(),
        },
      );
      if (!res.live || !res.authorize_url) {
        toast.notify({ tone: 'info', title: t('tplw.demo'), description: t('tplw.demo.desc') });
        return;
      }
      window.location.assign(res.authorize_url);
    } catch (err) {
      const description =
        err instanceof BffError ? errorMessage(t, err.status, err.code) : t('common.retry');
      toast.notify({ tone: 'danger', title: t('tplw.create.fail'), description });
    } finally {
      setBusy(false);
    }
  }

  async function create() {
    if (!template) return;
    if (crearRepo) {
      await crearConRepo();
      return;
    }
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; result?: TemplateApplied }>(
        '/api/projects/from-template',
        {
          template_id: template.id,
          slug: slug.trim().toLowerCase(),
          name: name.trim() || slug.trim().toLowerCase(),
          repo_url: repoUrl.trim(),
          answers,
        },
      );
      // Sin control-plane NO se ha creado nada. Enseñar la pantalla de «listo»
      // aquí sería exactamente el dato inventado que no queremos: el usuario se
      // iría creyendo que tiene una app desplegada.
      if (!res.live || !res.result) {
        toast.notify({ tone: 'info', title: t('tplw.demo'), description: t('tplw.demo.desc') });
        return;
      }
      setCreated(res.result);
      setStep(doneStepIndex);
    } catch (err) {
      // Código REAL del control-plane traducido por código (409 nombre cogido, 402
      // cuota/gasto/suscripción, 403 rol, 400 respuesta no declarada…).
      const description =
        err instanceof BffError ? errorMessage(t, err.status, err.code) : t('common.retry');
      toast.notify({ tone: 'danger', title: t('tplw.create.fail'), description });
      // Nombre cogido entre la comprobación y el alta: se vuelve al campo que hay
      // que arreglar en vez de dejar al usuario en un resumen que no se puede crear.
      if (err instanceof BffError && err.status === 409) {
        setSlugStatus('taken');
        setStep(1);
      }
    } finally {
      setBusy(false);
    }
  }

  const chooseStep: WizardStep = {
    id: 'template',
    title: t('tplw.step.template'),
    heading: t('tplw.step.template.heading'),
    description: t('tplw.step.template.desc'),
    canContinue: Boolean(template),
    nextLabel: t('tplw.next'),
    content: (
      <>
        <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {templates.map((tpl) => (
            <ChoiceCard
              key={tpl.id}
              name="tplw-template"
              value={tpl.id}
              title={tpl.name}
              selected={tpl.id === templateId}
              onSelect={() => pickTemplate(tpl)}
              badge={<Badge tone="neutral">{frameworkLabel(t, tpl.framework)}</Badge>}
              footnote={[
                tpl.license ? t('tplw.license', { license: tpl.license }) : '',
                ...tpl.tags,
              ]
                .filter(Boolean)
                .join(' · ')}
              description={
                <>
                  {tpl.description}
                  <span class="text-text mt-1 block">
                    {tpl.suggestedServices.length > 0
                      ? t('tplw.provisions', {
                          services: formatList(
                            locale,
                            tpl.suggestedServices.map((s) => serviceLabel(t, s)),
                          ),
                        })
                      : t('tplw.provisions.none')}
                  </span>
                </>
              }
            />
          ))}
        </div>
        {/* Documentación OFICIAL del stack elegido: quien no sabe qué es «Astro»
          tiene que poder comprobarlo antes de decidir. Fuera de la tarjeta a
          propósito: un enlace dentro de un <label> con radio pelea con la
          selección. */}
        {template?.docsUrl ? (
          <a
            href={template.docsUrl}
            target="_blank"
            rel="noreferrer"
            aria-label={t('templates.docsLabel', { name: template.name })}
            class="text-text-muted hover:text-text mt-3 inline-flex items-center gap-1 text-sm"
          >
            {t('templates.docs')}
            <Icon name="globe" size={14} />
          </a>
        ) : null}
      </>
    ),
  };

  // Con el campo vacío se enseña el ejemplo, no un hueco: la ayuda tiene que
  // poder leerse como una dirección de verdad antes de escribir nada.
  const slugExample = t('tplw.field.slug.placeholder');
  const host = `${slug.trim().toLowerCase() || slugExample}.apps.sytue.eu`;
  const projectStep: WizardStep = {
    id: 'project',
    title: t('tplw.step.project'),
    heading: t('tplw.step.project.heading'),
    description: t('tplw.step.project.desc'),
    canContinue: slugOk && repoOk,
    nextLabel: t('tplw.next'),
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        <FieldGroup
          label={t('tplw.field.name')}
          htmlFor="tplw-name"
          help={t('tplw.field.name.help')}
          required
        >
          <Input
            id="tplw-name"
            value={name}
            placeholder={t('tplw.field.name.placeholder')}
            aria-describedby={describedBy('tplw-name', t('tplw.field.name.help'))}
            onInput={(e) => changeName(e.currentTarget.value)}
          />
        </FieldGroup>

        <FieldGroup label={t('tplw.field.slug')} htmlFor="tplw-slug" required>
          <Input
            id="tplw-slug"
            value={slug}
            placeholder={slugExample}
            invalid={slugStatus === 'taken'}
            aria-describedby="tplw-slug-state"
            onInput={(e) => {
              setSlugEdited(true);
              setSlug(e.currentTarget.value);
            }}
          />
          {/* El estado se ANUNCIA, no solo se pinta: un color y un icono no
              existen para un lector de pantalla, y aquí lo que hace falta oír es
              justo que el nombre está cogido. */}
          <div id="tplw-slug-state" class="mt-1.5" role="status" aria-live="polite">
            <FieldHint
              tone={
                slugStatus === 'free' ? 'success' : slugStatus === 'taken' ? 'warning' : 'neutral'
              }
            >
              {slugStatus === 'checking'
                ? t('projects.slug.checking')
                : slugStatus === 'free'
                  ? t('projects.slug.free', { slug: slug.trim().toLowerCase() })
                  : slugStatus === 'taken'
                    ? t('projects.slug.busy', { slug: slug.trim().toLowerCase() })
                    : slugStatus === 'unknown'
                      ? t('projects.slug.unknown')
                      : t('tplw.field.slug.help', { host })}
            </FieldHint>
            {alternatives.length > 0 ? (
              // Alternativas LIBRES derivadas de su nombre: un clic las acepta, no
              // hay que volver a teclear ni recordarlas.
              <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                <span class="text-text-muted text-sm">{t('projects.slug.alternatives')}</span>
                {alternatives.map((a) => (
                  <Button
                    key={a}
                    type="button"
                    variant="secondary"
                    size="sm"
                    aria-label={t('projects.slug.useName', { name: a })}
                    onClick={() => {
                      setSlugEdited(true);
                      setSlug(a);
                    }}
                  >
                    {a}
                  </Button>
                ))}
              </div>
            ) : null}
          </div>
        </FieldGroup>

        {/* «No tengo repositorio» es la puerta que faltaba: sin ella, quien empieza
          de cero se topa con un campo obligatorio que no puede rellenar. Solo se
          ofrece si la plantilla trae esqueleto; si no, se dice por qué en vez de
          esconder la opción, que es indistinguible de que no exista. */}
        <FieldGroup label={t('tplw.repoChoice')} htmlFor="tplw-crear-repo">
          <label class="flex items-start gap-3 text-sm">
            <Checkbox
              ringClass="mt-0.5"
              id="tplw-crear-repo"
              class="accent-primary size-4 shrink-0"
              checked={crearRepo}
              disabled={!template?.hasStarter}
              onChange={(e) => setCrearRepo(e.currentTarget.checked)}
            />
            <span>
              {t('tplw.createRepo')}
              <span class="text-text-subtle block">
                {template?.hasStarter
                  ? t('tplw.createRepo.help')
                  : t('tplw.createRepo.unavailable')}
              </span>
            </span>
          </label>
        </FieldGroup>

        {crearRepo ? null : (
          <FieldGroup
            label={t('tplw.field.repo')}
            htmlFor="tplw-repo"
            help={t('tplw.field.repo.help')}
            required
            {...(repoUrl.trim() && !repoOk ? { error: t('tplw.field.repo.error') } : {})}
          >
            <Input
              id="tplw-repo"
              type="url"
              value={repoUrl}
              invalid={Boolean(repoUrl.trim()) && !repoOk}
              aria-describedby={describedBy(
                'tplw-repo',
                t('tplw.field.repo.help'),
                repoUrl.trim() && !repoOk ? t('tplw.field.repo.error') : undefined,
              )}
              onInput={(e) => setRepoUrl(e.currentTarget.value)}
            />
          </FieldGroup>
        )}
      </div>
    ),
  };

  // Solo existe si la plantilla PREGUNTA algo. Un paso vacío «para que haya tres»
  // es un clic que no le sirve a nadie.
  const questionsStep: WizardStep = {
    id: 'answers',
    title: t('tplw.step.answers'),
    heading: t('tplw.step.answers.heading'),
    description: t('tplw.step.answers.desc'),
    canContinue: answersOk,
    nextLabel: t('tplw.next'),
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        {questions.map((q) => (
          <FieldGroup key={q.key} label={q.prompt ?? q.key} htmlFor={`tplw-a-${q.key}`} required>
            <Input
              id={`tplw-a-${q.key}`}
              type={q.secret ? 'password' : 'text'}
              value={answers[q.key] ?? ''}
              autoComplete="off"
              aria-describedby={`tplw-a-${q.key}-hint`}
              onInput={(e) => setAnswers((prev) => ({ ...prev, [q.key]: e.currentTarget.value }))}
            />
            <FieldHint id={`tplw-a-${q.key}-hint`}>
              {q.secret ? t('tplw.answer.secret', { key: q.key }) : q.key}
            </FieldHint>
          </FieldGroup>
        ))}
      </div>
    ),
  };

  // Contraseñas que genera y cifra la PLATAFORMA: el usuario no inventa ninguna.
  const generated = template?.env.filter((e) => e.origin === 'generate').length ?? 0;
  const reviewItems: ReviewItem[] = template
    ? [
        { label: t('tplw.review.template'), value: template.name, onEdit: () => setStep(0) },
        { label: t('tplw.review.project'), value: name.trim() || slug, onEdit: () => setStep(1) },
        { label: t('tplw.review.address'), value: host, mono: true, onEdit: () => setStep(1) },
        {
          label: t('tplw.review.repo'),
          value: repoUrl.trim(),
          mono: true,
          onEdit: () => setStep(1),
        },
        {
          label: t('tplw.review.services'),
          value:
            template.suggestedServices.length > 0
              ? formatList(
                  locale,
                  template.suggestedServices.map((s) => serviceLabel(t, s)),
                )
              : t('tplw.review.services.none'),
        },
        {
          label: t('tplw.review.secrets'),
          value:
            generated > 0
              ? t('tplw.review.secrets.value', { count: generated })
              : t('tplw.review.secrets.none'),
        },
        { label: t('tplw.review.deploy'), value: t('tplw.review.deploy.value') },
        ...questions.map((q): ReviewItem => ({
          label: q.prompt ?? q.key,
          value: q.secret ? t('tplw.review.answer.secret') : (answers[q.key] ?? ''),
          onEdit: () => setStep(2),
        })),
      ]
    : [];

  const reviewStep: WizardStep = {
    id: 'review',
    title: t('tplw.step.review'),
    heading: t('tplw.step.review.heading'),
    description: t('tplw.step.review.desc'),
    nextLabel: t('tplw.create'),
    onNext: () => void create(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={reviewItems} editLabel={t('common.edit')} />
        <InlineHelp tone="info" title={t('tplw.review.atomic')}>
          {t('tplw.review.atomic.desc')}
        </InlineHelp>
      </div>
    ),
  };

  const doneStep: WizardStep = {
    id: 'done',
    title: t('tplw.step.done'),
    heading: t('tplw.step.done.heading', { name: created?.project.name ?? '' }),
    done: true,
    content: created ? <Done t={t} locale={locale} applied={created} /> : null,
  };

  const steps: WizardStep[] = [
    chooseStep,
    projectStep,
    ...(questions.length > 0 ? [questionsStep] : []),
    reviewStep,
    doneStep,
  ];

  return (
    <Wizard
      steps={steps}
      current={Math.min(step, steps.length - 1)}
      onCurrentChange={setStep}
      busy={busy}
      labels={{
        back: t('common.back'),
        next: t('common.continue'),
        cancel: t('common.cancel'),
        progress: t('wizard.progress'),
        goToStep: (n) => t('wizard.goToStep', { n }),
        stepOf: (n, total) => t('wizard.stepOf', { n, total }),
      }}
      {...(created ? {} : { onCancel: () => window.location.assign('/') })}
    />
  );
}

/**
 * Lo que ha quedado creado y lo único que hay que hacer ahora: guardar el secreto
 * del webhook. El control-plane lo entrega UNA vez, así que esta pantalla es la
 * única oportunidad de copiarlo —y lo dice—. Los secretos que generó la plataforma
 * NO se enseñan aquí: viven cifrados y no vuelven, que es justo la garantía.
 */
function Done({ t, locale, applied }: { t: T; locale: Locale; applied: TemplateApplied }) {
  const services = applied.services.map((s) => serviceLabel(t, s.kind));
  const secrets = applied.env.filter((e) => e.is_secret).length;
  const url = applied.deployment.url;
  return (
    <div class="flex max-w-2xl flex-col gap-4">
      <InlineHelp tone="success" title={t('tplw.done.building')}>
        {t('tplw.done.building.desc')}
      </InlineHelp>

      <section class="border-border bg-surface-raised rounded-lg border p-4">
        <h3 class="text-text text-sm font-semibold">{t('tplw.done.created')}</h3>
        <ul class="text-text-muted mt-2 flex flex-col gap-1.5 text-sm">
          <li class="flex items-start gap-2">
            <Icon name="check" size={14} class="text-success-text mt-1 shrink-0" />
            <span>{t('tplw.done.project', { name: applied.project.name })}</span>
          </li>
          {services.length > 0 ? (
            <li class="flex items-start gap-2">
              <Icon name="check" size={14} class="text-success-text mt-1 shrink-0" />
              <span>{t('tplw.done.services', { services: formatList(locale, services) })}</span>
            </li>
          ) : null}
          {secrets > 0 ? (
            <li class="flex items-start gap-2">
              <Icon name="check" size={14} class="text-success-text mt-1 shrink-0" />
              <span>{t('tplw.done.secrets', { count: secrets })}</span>
            </li>
          ) : null}
          <li class="flex items-start gap-2">
            <Icon name="check" size={14} class="text-success-text mt-1 shrink-0" />
            <span>{t('tplw.done.deploy')}</span>
          </li>
        </ul>
      </section>

      {applied.webhook_secret ? (
        <section class="border-warning bg-warning-bg rounded-lg border p-4">
          <h3 class="text-warning-text text-sm font-semibold">{t('projects.hook.once')}</h3>
          <p class="text-text-muted mt-1 text-sm leading-relaxed">{t('projects.hook.once.desc')}</p>
          <div class="mt-3 flex flex-col gap-3">
            <CopyField t={t} label={t('projects.hook.url')} value={applied.webhook_url ?? ''} />
            <CopyField t={t} label={t('projects.hook.secret')} value={applied.webhook_secret} />
          </div>
        </section>
      ) : null}

      <div class="border-border-subtle flex flex-wrap items-center gap-3 border-t pt-4">
        <Button
          variant="primary"
          // `r()`, no la ruta a mano: esta app vive bajo `/cloud` y una ruta profunda
          // escrita desde la raíz se salta el prefijo (mismo 404 documentado en
          // `../lib/rutas.ts`).
          onClick={() => window.location.assign(r(`/projects/${applied.project.slug}`))}
        >
          {t('tplw.done.open')}
        </Button>
        {url ? (
          <a
            href={url}
            target="_blank"
            rel="noreferrer"
            class="text-text-muted hover:text-text inline-flex items-center gap-1 text-sm"
          >
            {url.replace('https://', '')}
            <Icon name="globe" size={14} />
          </a>
        ) : null}
      </div>
    </div>
  );
}
