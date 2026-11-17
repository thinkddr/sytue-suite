/**
 * NewProjectWizard — «Nuevo proyecto» como asistente: origen → configurar →
 * variables → revisar → desplegar, con el log del primer build a la vista.
 *
 * Antes era un modal con siete campos a la vez que acababa en «Proyecto creado»:
 * las variables había que darlas de alta DESPUÉS, en otra pantalla, y el primer
 * despliegue no lo lanzaba nadie, así que la persona se quedaba ante un proyecto
 * vacío sin saber que faltaba pulsar «Desplegar» (ni que su app iba a romper al
 * arrancar por la variable que no estaba). Cada paso hace ahora una sola pregunta,
 * en el orden en que se sabe la respuesta: primero de dónde sale el código (y de
 * él se deduce casi todo lo demás), luego cómo se llama, luego qué secretos
 * necesita, y al final se ve todo junto antes de tocar nada.
 *
 * El último paso no es un «listo» de cortesía: crea el proyecto, guarda las
 * variables, lanza el despliegue y enseña el log del build hasta que queda en
 * línea o falla. Si algo falla a mitad, se dice QUÉ y «Reintentar» sigue donde se
 * paró (`runPipeline`), sin volver a crear lo ya creado.
 *
 * Solo @sytue/ui. docs/CONVENTIONS.md §14.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  FieldGroup,
  FieldHint,
  Icon,
  IconButton,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  Spinner,
  Textarea,
  ToastProvider,
  Wizard,
  describedBy,
  useToast,
  type ReviewItem,
  type WizardStep,
  EmptyState,
  CopyField,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { DeployStatus, LogLine, Project } from '../lib/types.js';
import { postBff } from '../lib/bff-client.js';
import { errorMessage, useT, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import {
  FRAMEWORK_NAME,
  INITIAL_PIPELINE,
  SLUG_RE,
  filledVars,
  isFinalStatus,
  mergeParsed,
  repoName,
  runPipeline,
  toSlug,
  validRepoUrl,
  varErrors,
  type PipelineError,
  type PipelineState,
  type TaskState,
  type VarError,
  type VarRow,
} from '../lib/nuevo-proyecto.js';
import { parseDotenv } from './EnvImport.js';
import { LogViewer } from './LogsPanel.js';

/** Frameworks con opción propia en el Select; otros (p. ej. go) se añaden al vuelo. */
const KNOWN_FRAMEWORKS = ['auto', 'astro', 'next', 'preact', 'static', 'node', 'sveltekit'];

function frameworkLabel(t: T, fw: string): string {
  if (fw === 'auto' || !fw) return t('framework.auto');
  return fw === 'static' ? t('framework.static') : (FRAMEWORK_NAME[fw] ?? fw);
}

/** Cada cuánto se pregunta por el build mientras corre. */
const POLL_MS = 3000;

/**
 * Generador de ids de fila a partir de las que ya hay: la identidad de la fila es
 * lo que conserva el foco al teclear, y la clave cambia mientras se escribe.
 */
function idFrom(rows: VarRow[]): () => number {
  let next = rows.reduce((max, r) => Math.max(max, r.id), 0) + 1;
  return () => next++;
}

/** GET JSON al BFF; null si contesta con error (se vuelve a preguntar luego). */
async function getJson<R>(url: string): Promise<R | null> {
  const res = await fetch(url);
  return res.ok ? ((await res.json()) as R) : null;
}

/** Pasos, por índice: el orden es el del flujo y el de la barra de progreso. */
const STEP = { origin: 0, configure: 1, variables: 2, review: 3, deploy: 4 } as const;

interface Props {
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function NewProjectWizard(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function Inner({ locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [step, setStep] = useState<number>(STEP.origin);

  // ---- Origen ----
  const [repoUrl, setRepoUrl] = useState('');
  /** Rama que se publica ('' = el defecto del control-plane, `main`). */
  const [productionRef, setProductionRef] = useState('');
  /** Carpeta de la app dentro del repo ('' = la raíz). Es lo que hace posible un monorepo. */
  const [rootDir, setRootDir] = useState('');
  /**
   * Lo que se ha visto DENTRO del repositorio. Es lo que permite no preguntar
   * framework, comandos y directorio de salida: están escritos ahí. `null` = no
   * se ha mirado, no se pudo o no se sacó nada en claro: entonces se pregunta.
   */
  const [detection, setDetection] = useState<{
    framework: string;
    evidence: string;
    confident: boolean;
  } | null>(null);
  const [inspecting, setInspecting] = useState(false);
  /** Pidió cambiar lo detectado: no se le vuelve a pisar la elección. */
  const [overrideDetection, setOverrideDetection] = useState(false);

  // ---- Configurar ----
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  /** Una vez editada a mano, la dirección deja de seguir al nombre. */
  const [slugEdited, setSlugEdited] = useState(false);
  const [slugStatus, setSlugStatus] = useState<'idle' | 'checking' | 'free' | 'taken' | 'unknown'>(
    'idle',
  );
  const [alternatives, setAlternatives] = useState<string[]>([]);
  const [suggesting, setSuggesting] = useState(false);
  const [framework, setFramework] = useState('auto');
  const [buildStrategy, setBuildStrategy] = useState('');
  const [buildCommand, setBuildCommand] = useState('');
  const [installCommand, setInstallCommand] = useState('');
  const [outputDir, setOutputDir] = useState('');
  /** Plantilla de la galería de la que se viene, si se viene de una. */
  const [templateName, setTemplateName] = useState('');

  // ---- Variables ----
  const [rows, setRows] = useState<VarRow[]>([]);
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');

  // ---- Alta y despliegue ----
  const [pipeline, setPipeline] = useState<PipelineState>(INITIAL_PIPELINE);
  const [busy, setBusy] = useState(false);
  const [deployStatus, setDeployStatus] = useState<DeployStatus | undefined>(undefined);
  const [deployUrl, setDeployUrl] = useState('');
  const [logs, setLogs] = useState<LogLine[]>([]);

  const cleanSlug = slug.trim().toLowerCase();
  const errors = useMemo(() => varErrors(rows), [rows]);
  const vars = useMemo(() => filledVars(rows), [rows]);

  // Deep-link de la galería de plantillas (`?template=…&framework=…`) o de la
  // paleta: pre-rellena la compilación y limpia la URL para que no reaparezca.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const keys = [
      'template',
      'framework',
      'build_strategy',
      'build_command',
      'install_command',
      'output_dir',
    ];
    if (!keys.some((k) => params.has(k))) return;
    const tpl = params.get('template');
    if (tpl) setTemplateName(tpl);
    const fw = params.get('framework');
    if (fw) setFramework(fw);
    setBuildStrategy(params.get('build_strategy') ?? '');
    setBuildCommand(params.get('build_command') ?? '');
    setInstallCommand(params.get('install_command') ?? '');
    setOutputDir(params.get('output_dir') ?? '');
    for (const k of keys) params.delete(k);
    const qs = params.toString();
    window.history.replaceState({}, '', qs ? `?${qs}` : window.location.pathname);
  }, []);

  /**
   * Mira el repositorio y rellena lo que se pueda deducir de él, SIN pisar lo ya
   * escrito (lo puso él o lo trajo una plantilla) ni lo que pidió cambiar. Un
   * fallo no bloquea nada: sin detección se pregunta, como siempre.
   */
  useEffect(() => {
    const url = repoUrl.trim();
    if (!validRepoUrl(url)) {
      setDetection(null);
      return;
    }
    setInspecting(true);
    let cancelado = false;
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const res = await postBff<{
            ok: boolean;
            live: boolean;
            detected: boolean;
            detection?: {
              framework: string;
              build_strategy?: string;
              install_command?: string;
              build_command?: string;
              output_dir?: string;
              confident: boolean;
              evidence?: string;
            };
          }>('/api/inspect-repo', { repo_url: url, root_dir: rootDir.trim() });
          if (cancelado) return;
          const d = res.detected ? res.detection : undefined;
          if (!d || overrideDetection) {
            setDetection(null);
            return;
          }
          setDetection({
            framework: d.framework,
            evidence: d.evidence ?? '',
            confident: d.confident,
          });
          if (d.framework && d.framework !== 'auto')
            setFramework((v) => (v === 'auto' ? d.framework : v));
          if (d.build_strategy) setBuildStrategy((v) => v || d.build_strategy!);
          if (d.install_command) setInstallCommand((v) => v || d.install_command!);
          if (d.build_command) setBuildCommand((v) => v || d.build_command!);
          if (d.output_dir) setOutputDir((v) => v || d.output_dir!);
        } catch {
          if (!cancelado) setDetection(null);
        } finally {
          if (!cancelado) setInspecting(false);
        }
      })();
    }, 600);
    return () => {
      cancelado = true;
      setInspecting(false);
      clearTimeout(timer);
    };
    // `rootDir` entra porque en un monorepo lo que hay en la raíz no dice nada de la app.
  }, [repoUrl, rootDir, overrideDetection]);

  /**
   * ¿Está libre la dirección? El slug es único en TODA la plataforma: sin esto el
   * usuario no se entera hasta el 409. Con debounce, y descartando respuestas
   * tardías de un nombre anterior. `unknown` NO es «libre».
   */
  useEffect(() => {
    setAlternatives([]);
    if (!cleanSlug || !SLUG_RE.test(cleanSlug)) {
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
          }>('/api/check-slug', { slug: cleanSlug, lang: locale });
          if (cancelado) return;
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
  }, [cleanSlug, locale]);

  /**
   * Una vez lanzado el despliegue, se pregunta por su estado y por el log del build
   * hasta que acaba. El log solo se pinta si es REAL (`live`): sin build todavía,
   * el BFF devolvería el de demostración, y enseñarlo sería inventarse un build.
   */
  const createdSlug = pipeline.created?.slug ?? '';
  useEffect(() => {
    if (pipeline.deploy !== 'done' || !createdSlug) return;
    let cancelado = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const q = encodeURIComponent(createdSlug);
    const tick = async () => {
      let final = false;
      try {
        const [dep, log] = await Promise.all([
          getJson<{ deployment?: { status: DeployStatus; url?: string } }>(
            `/api/deployments?project=${q}`,
          ),
          getJson<{ live?: boolean; logs?: LogLine[] }>(`/api/logs?project=${q}&source=build`),
        ]);
        if (cancelado) return;
        const d = dep?.deployment;
        if (d) {
          setDeployStatus(d.status);
          if (d.url) setDeployUrl(d.url);
          final = isFinalStatus(d.status);
        }
        if (log?.live && log.logs) setLogs(log.logs);
      } catch {
        // Un fallo de red al preguntar no cambia nada del despliegue: se vuelve a
        // preguntar en el siguiente turno.
      }
      if (!cancelado && !final) timer = setTimeout(() => void tick(), POLL_MS);
    };
    void tick();
    return () => {
      cancelado = true;
      if (timer) clearTimeout(timer);
    };
  }, [pipeline.deploy, createdSlug]);

  function changeName(value: string) {
    setName(value);
    if (!slugEdited) setSlug(toSlug(value));
  }

  /** Pide un nombre libre ya comprobado contra el índice global. */
  async function suggestSlug() {
    setSuggesting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; slug?: string; available?: boolean }>(
        '/api/suggest-slug',
        { lang: locale },
      );
      if (!res.slug) {
        toast.notify({ tone: 'warning', title: t('projects.suggest.failNow') });
        return;
      }
      setSlugEdited(true);
      setSlug(res.slug);
    } catch {
      toast.notify({ tone: 'danger', title: t('projects.suggest.fail') });
    } finally {
      setSuggesting(false);
    }
  }

  function updateRow(id: number, patch: Partial<VarRow>) {
    setRows((prev) => prev.map((row) => (row.id === id ? { ...row, ...patch } : row)));
  }

  const parsedImport = useMemo(() => parseDotenv(importText), [importText]);

  function applyImport() {
    setRows((prev) => mergeParsed(prev, parsedImport.vars, idFrom(prev)));
    setImportText('');
    setImportOpen(false);
  }

  /** Lo que se manda al crear. En un reintento el proyecto ya existe y no se usa. */
  function projectBody() {
    return {
      slug: cleanSlug,
      name: name.trim() || cleanSlug,
      repo_url: repoUrl.trim(),
      framework,
      build_strategy: buildStrategy,
      build_command: buildCommand.trim(),
      install_command: installCommand.trim(),
      output_dir: outputDir.trim(),
      production_ref: productionRef.trim(),
      root_dir: rootDir.trim().replace(/^\/+|\/+$/g, ''),
    };
  }

  /** Lanza el alta o la reanuda donde se paró (`runPipeline` se salta lo hecho). */
  async function run(): Promise<PipelineState> {
    setBusy(true);
    try {
      return await runPipeline(
        { project: projectBody(), vars },
        pipeline,
        (url, body) => postBff(url, body),
        setPipeline,
      );
    } finally {
      setBusy(false);
    }
  }

  async function start() {
    const s = await run();
    if (s.project === 'done') {
      setStep(STEP.deploy);
      return;
    }
    // Nombre cogido entre la comprobación y el alta: se vuelve al campo que hay
    // que arreglar, que es donde el error tiene arreglo.
    if (s.error?.phase === 'project' && s.error.status === 409) {
      setSlugStatus('taken');
      setStep(STEP.configure);
    }
  }

  const repoInvalid = repoUrl.trim() !== '' && !validRepoUrl(repoUrl);
  const slugFormatBad = cleanSlug !== '' && !SLUG_RE.test(cleanSlug);
  const slugOk = SLUG_RE.test(cleanSlug) && slugStatus !== 'taken';
  const host = `${cleanSlug || t('tplw.field.slug.placeholder')}.apps.sytue.eu`;

  const originStep: WizardStep = {
    id: 'origin',
    title: t('npw.step.origin'),
    heading: t('npw.step.origin.heading'),
    description: t('npw.step.origin.desc'),
    canContinue: validRepoUrl(repoUrl),
    onNext: () => {
      // El nombre del repositorio es casi siempre el del proyecto: se propone, y
      // con él la dirección, sin pisar lo que ya hubiera escrito.
      if (!name.trim()) changeName(repoName(repoUrl));
      setStep(STEP.configure);
    },
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        {templateName ? (
          <InlineHelp tone="info">
            {t('projects.modal.template', { name: templateName })}
          </InlineHelp>
        ) : null}
        <FieldGroup
          label={t('projects.field.repo')}
          htmlFor="npw-repo"
          help={t('tplw.field.repo.help')}
          required
          {...(repoInvalid ? { error: t('projects.field.repo.error') } : {})}
        >
          <Input
            id="npw-repo"
            type="url"
            value={repoUrl}
            required
            placeholder="https://git.sytue.eu/org/repo"
            class="font-mono text-sm"
            autoComplete="off"
            invalid={repoInvalid}
            aria-describedby={describedBy(
              'npw-repo',
              t('tplw.field.repo.help'),
              repoInvalid ? t('projects.field.repo.error') : undefined,
            )}
            onInput={(e) => setRepoUrl(e.currentTarget.value)}
          />
        </FieldGroup>
        {/* Lo que se ha visto y CON QUÉ prueba, en vez de un desplegable que el
          usuario tiene que rellenar con datos de su propio repositorio. */}
        <div role="status" aria-live="polite">
          {inspecting ? (
            <FieldHint>{t('projects.detecting')}</FieldHint>
          ) : detection?.confident && !overrideDetection ? (
            <InlineHelp
              tone="success"
              title={t('projects.detected.title')}
              action={
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setOverrideDetection(true)}
                >
                  {t('projects.detected.change')}
                </Button>
              }
            >
              {t('projects.detected.desc', {
                framework: frameworkLabel(t, framework),
                evidence: detection.evidence,
              })}
            </InlineHelp>
          ) : detection && !detection.confident ? (
            <FieldHint tone="warning">
              {t('projects.detected.unsure', { evidence: detection.evidence })}
            </FieldHint>
          ) : null}
        </div>
        {/* Sin estas dos NO se puede publicar un repo cuya rama principal no se
          llame «main», ni una app que viva dentro de un monorepo. */}
        <FieldGroup
          label={t('projects.field.branch')}
          htmlFor="npw-branch"
          help={t('projects.field.branch.help')}
        >
          <Input
            id="npw-branch"
            value={productionRef}
            placeholder="main"
            class="font-mono text-sm"
            aria-describedby={describedBy('npw-branch', t('projects.field.branch.help'))}
            onInput={(e) => setProductionRef(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup
          label={t('projects.field.rootDir')}
          htmlFor="npw-root"
          help={t('projects.field.rootDir.help')}
        >
          <Input
            id="npw-root"
            value={rootDir}
            placeholder="apps/web"
            class="font-mono text-sm"
            aria-describedby={describedBy('npw-root', t('projects.field.rootDir.help'))}
            onInput={(e) => setRootDir(e.currentTarget.value)}
          />
        </FieldGroup>
      </div>
    ),
  };

  const configureStep: WizardStep = {
    id: 'configure',
    title: t('npw.step.configure'),
    heading: t('npw.step.configure.heading'),
    description: t('npw.step.configure.desc'),
    canContinue: slugOk,
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        <FieldGroup
          label={t('tplw.field.name')}
          htmlFor="npw-name"
          help={t('tplw.field.name.help')}
        >
          <Input
            id="npw-name"
            value={name}
            placeholder={t('tplw.field.name.placeholder')}
            aria-describedby={describedBy('npw-name', t('tplw.field.name.help'))}
            onInput={(e) => changeName(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup
          label={t('tplw.field.slug')}
          htmlFor="npw-slug"
          required
          {...(slugFormatBad ? { error: t('projects.field.slug.format') } : {})}
        >
          <Input
            id="npw-slug"
            value={slug}
            required
            placeholder={t('tplw.field.slug.placeholder')}
            class="font-mono text-sm"
            invalid={slugFormatBad || slugStatus === 'taken'}
            // El error de formato y el estado de la comprobación, los dos: quien no
            // ve el campo tiene que oír por qué no puede seguir.
            aria-describedby={slugFormatBad ? 'npw-slug-error npw-slug-state' : 'npw-slug-state'}
            onInput={(e) => {
              setSlugEdited(true);
              setSlug(e.currentTarget.value);
            }}
          />
          {/* El estado se ANUNCIA, no solo se pinta: un color no existe para un
            lector de pantalla, y aquí lo que hace falta oír es que está cogido. */}
          <div id="npw-slug-state" class="mt-1.5" role="status" aria-live="polite">
            <FieldHint
              tone={
                slugStatus === 'free' ? 'success' : slugStatus === 'taken' ? 'warning' : 'neutral'
              }
            >
              {slugStatus === 'checking'
                ? t('projects.slug.checking')
                : slugStatus === 'free'
                  ? t('projects.slug.free', { slug: cleanSlug })
                  : slugStatus === 'taken'
                    ? t('projects.slug.busy', { slug: cleanSlug })
                    : slugStatus === 'unknown'
                      ? t('projects.slug.unknown')
                      : t('tplw.field.slug.help', { host })}
            </FieldHint>
            {alternatives.length > 0 ? (
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
          <Button
            type="button"
            variant="ghost"
            size="sm"
            class="mt-1 self-start"
            loading={suggesting}
            disabled={suggesting}
            onClick={() => void suggestSlug()}
          >
            {slug ? t('projects.slug.suggestAnother') : t('projects.slug.suggest')}
          </Button>
        </FieldGroup>
        <FieldGroup label={t('projects.field.framework')} htmlFor="npw-fw">
          <Select
            id="npw-fw"
            value={framework}
            onChange={(e) => setFramework(e.currentTarget.value)}
          >
            <option value="auto">{t('framework.auto')}</option>
            <option value="astro">Astro</option>
            <option value="next">Next.js</option>
            <option value="preact">Preact</option>
            <option value="static">{t('framework.static')}</option>
            <option value="node">Node</option>
            <option value="sveltekit">SvelteKit</option>
            {/* La plantilla o la detección pueden traer un stack sin opción propia. */}
            {!KNOWN_FRAMEWORKS.includes(framework) ? (
              <option value={framework}>{frameworkLabel(t, framework)}</option>
            ) : null}
          </Select>
        </FieldGroup>
        {/* Compilación: plegada salvo que ya traiga algo (detectado o de plantilla),
          porque entonces el usuario tiene que poder ver qué se va a ejecutar. */}
        <details
          class="group"
          open={Boolean(buildCommand || installCommand || outputDir || buildStrategy)}
        >
          <summary class="text-text-muted hover:text-text flex cursor-pointer items-center gap-1.5 text-sm select-none">
            <Icon name="chevronRight" size={14} class="transition-transform group-open:rotate-90" />
            {t('projects.build.summary')}
          </summary>
          <div class="mt-3 flex flex-col gap-4">
            <FieldGroup
              label={t('projects.build.strategy')}
              htmlFor="npw-strategy"
              help={t('projects.build.strategy.help')}
            >
              <Select
                id="npw-strategy"
                value={buildStrategy}
                onChange={(e) => setBuildStrategy(e.currentTarget.value)}
              >
                <option value="">{t('projects.build.strategy.auto')}</option>
                <option value="dockerfile">Dockerfile</option>
                <option value="buildpacks">Buildpacks</option>
              </Select>
            </FieldGroup>
            <FieldGroup label={t('projects.build.install')} htmlFor="npw-install">
              <Input
                id="npw-install"
                value={installCommand}
                placeholder="npm install"
                class="font-mono text-sm"
                onInput={(e) => setInstallCommand(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup label={t('projects.build.command')} htmlFor="npw-build">
              <Input
                id="npw-build"
                value={buildCommand}
                placeholder="npm run build"
                class="font-mono text-sm"
                onInput={(e) => setBuildCommand(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup label={t('projects.build.output')} htmlFor="npw-output">
              <Input
                id="npw-output"
                value={outputDir}
                placeholder="dist"
                class="font-mono text-sm"
                onInput={(e) => setOutputDir(e.currentTarget.value)}
              />
            </FieldGroup>
          </div>
        </details>
      </div>
    ),
  };

  const variablesStep: WizardStep = {
    id: 'variables',
    title: t('npw.step.variables'),
    heading: t('npw.step.variables.heading'),
    description: t('npw.step.variables.desc'),
    canContinue: errors.size === 0,
    nextLabel: vars.length === 0 ? t('npw.vars.skip') : t('common.continue'),
    content: (
      <div class="flex max-w-3xl flex-col gap-4">
        {rows.length === 0 ? (
          <EmptyState compact title={t('npw.vars.empty')} />
        ) : (
          <ul class="flex flex-col gap-3" aria-label={t('env.caption')}>
            {rows.map((row, i) => (
              <VarRowEditor
                key={row.id}
                t={t}
                row={row}
                n={i + 1}
                error={errors.get(row.id)}
                onChange={(patch) => updateRow(row.id, patch)}
                onRemove={() => setRows((prev) => prev.filter((x) => x.id !== row.id))}
              />
            ))}
          </ul>
        )}
        <div class="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="secondary"
            leadingIcon={<Icon name="plus" size={16} />}
            onClick={() =>
              setRows((prev) => [
                ...prev,
                { id: idFrom(prev)(), key: '', value: '', secret: false },
              ])
            }
          >
            {t('npw.vars.add')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            leadingIcon={<Icon name="upload" size={16} />}
            aria-expanded={importOpen}
            aria-controls="npw-import"
            onClick={() => setImportOpen((v) => !v)}
          >
            {t('env.import.open')}
          </Button>
        </div>
        {importOpen ? (
          <div
            id="npw-import"
            class="border-border bg-bg-subtle flex flex-col gap-3 rounded-md border p-3"
          >
            <FieldGroup
              label={t('env.import.paste')}
              htmlFor="npw-import-text"
              help={t('env.import.paste.help')}
            >
              <Textarea
                id="npw-import-text"
                rows={6}
                value={importText}
                class="font-mono text-sm"
                spellcheck={false}
                aria-describedby={describedBy('npw-import-text', t('env.import.paste.help'))}
                onInput={(e) => setImportText(e.currentTarget.value)}
              />
            </FieldGroup>
            {importText.trim() ? (
              <FieldHint tone={parsedImport.vars.length > 0 ? 'neutral' : 'warning'}>
                {parsedImport.vars.length > 0
                  ? t('env.import.preview', {
                      count: parsedImport.vars.length,
                      skipped: parsedImport.skipped,
                    })
                  : t('env.import.nothing')}
              </FieldHint>
            ) : null}
            <div>
              <Button
                type="button"
                variant="primary"
                disabled={parsedImport.vars.length === 0}
                onClick={applyImport}
              >
                {t('env.import.action', { count: parsedImport.vars.length })}
              </Button>
            </div>
          </div>
        ) : null}
        <FieldHint>{t('npw.vars.scope')}</FieldHint>
      </div>
    ),
  };

  const projectError = pipeline.error?.phase === 'project' ? pipeline.error : undefined;
  const reviewItems: ReviewItem[] = [
    {
      label: t('tplw.review.repo'),
      value: repoUrl.trim(),
      mono: true,
      onEdit: () => setStep(STEP.origin),
    },
    {
      label: t('projects.field.branch'),
      value: productionRef.trim() || 'main',
      mono: true,
      onEdit: () => setStep(STEP.origin),
    },
    ...(rootDir.trim()
      ? [
          {
            label: t('projects.field.rootDir'),
            value: rootDir.trim(),
            mono: true,
            onEdit: () => setStep(STEP.origin),
          },
        ]
      : []),
    {
      label: t('tplw.review.project'),
      value: name.trim() || cleanSlug,
      onEdit: () => setStep(STEP.configure),
    },
    {
      label: t('tplw.review.address'),
      value: host,
      mono: true,
      onEdit: () => setStep(STEP.configure),
    },
    {
      label: t('projects.field.framework'),
      value: frameworkLabel(t, framework),
      onEdit: () => setStep(STEP.configure),
    },
    {
      label: t('npw.review.vars'),
      value:
        vars.length > 0 ? (
          <>
            {t('env.count', { count: vars.length })}
            <span class="text-text-muted block font-mono text-sm break-all">
              {vars.map((v) => v.key).join(', ')}
            </span>
          </>
        ) : (
          t('npw.review.vars.none')
        ),
      onEdit: () => setStep(STEP.variables),
    },
    { label: t('tplw.review.deploy'), value: t('npw.review.deploy.value') },
  ];

  const reviewStep: WizardStep = {
    id: 'review',
    title: t('tplw.step.review'),
    heading: t('tplw.step.review.heading'),
    description: t('tplw.step.review.desc'),
    nextLabel: t('tplw.create'),
    onNext: () => void start(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={reviewItems} editLabel={t('common.edit')} />
        {/* El error del alta se queda en el paso que lo causa y dice lo que
          contestó el control-plane (cuota, permiso, nombre cogido…). */}
        {projectError ? (
          <InlineHelp tone="danger" title={t('projects.create.fail')}>
            {describeError(t, projectError)}
          </InlineHelp>
        ) : !pipeline.live ? (
          <InlineHelp tone="info" title={t('tplw.demo')}>
            {t('tplw.demo.desc')}
          </InlineHelp>
        ) : (
          <InlineHelp tone="info" title={t('npw.review.order')}>
            {t('npw.review.order.desc')}
          </InlineHelp>
        )}
      </div>
    ),
  };

  const deployStep: WizardStep = {
    id: 'deploy',
    title: t('npw.step.deploy'),
    heading: t('tplw.step.done.heading', { name: pipeline.created?.name ?? name }),
    done: true,
    content: pipeline.created ? (
      <DeployProgress
        t={t}
        project={pipeline.created}
        pipeline={pipeline}
        totalVars={vars.length}
        status={deployStatus}
        url={deployUrl}
        logs={logs}
        busy={busy}
        onRetry={() => void run()}
      />
    ) : null,
  };

  return (
    <Wizard
      steps={[originStep, configureStep, variablesStep, reviewStep, deployStep]}
      current={step}
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
      {...(pipeline.created ? {} : { onCancel: () => window.location.assign(r('/')) })}
    />
  );
}

/** Frase para un fallo del alta, con el código REAL del control-plane. */
function describeError(t: T, e: PipelineError): string {
  return e.status ? errorMessage(t, e.status, e.code) : t('common.retry');
}

const VAR_ERROR_KEY: Record<
  VarError,
  'env.error.keyFormat' | 'env.error.valueRequired' | 'npw.vars.duplicate'
> = {
  keyFormat: 'env.error.keyFormat',
  valueRequired: 'env.error.valueRequired',
  duplicate: 'npw.vars.duplicate',
};

function VarRowEditor({
  t,
  row,
  n,
  error,
  onChange,
  onRemove,
}: {
  t: T;
  row: VarRow;
  n: number;
  error: VarError | undefined;
  onChange: (patch: Partial<VarRow>) => void;
  onRemove: () => void;
}) {
  const errId = `npw-var-${row.id}-err`;
  return (
    <li class="flex flex-col gap-1.5">
      <div class="grid grid-cols-1 items-center gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto_auto]">
        <Input
          value={row.key}
          placeholder="API_KEY"
          aria-label={t('npw.vars.keyLabel', { n })}
          class="font-mono text-sm"
          autoComplete="off"
          spellcheck={false}
          invalid={error === 'keyFormat' || error === 'duplicate'}
          aria-describedby={error ? errId : undefined}
          onInput={(e) => onChange({ key: e.currentTarget.value })}
        />
        <Input
          value={row.value}
          type={row.secret ? 'password' : 'text'}
          placeholder={t('env.field.value.placeholder')}
          aria-label={t('npw.vars.valueLabel', { n })}
          class="font-mono text-sm"
          autoComplete="off"
          spellcheck={false}
          invalid={error === 'valueRequired'}
          aria-describedby={error ? errId : undefined}
          onInput={(e) => onChange({ value: e.currentTarget.value })}
        />
        <label class="text-text-muted flex items-center gap-2 text-sm">
          <Checkbox
            checked={row.secret}
            onChange={(e) => onChange({ secret: e.currentTarget.checked })}
          />
          {t('env.secret')}
        </label>
        <IconButton label={t('npw.vars.remove', { n })} tone="danger" onClick={onRemove}>
          <Icon name="trash" size={16} />
        </IconButton>
      </div>
      {error ? (
        <FieldHint id={errId} tone="danger">
          {t(VAR_ERROR_KEY[error])}
        </FieldHint>
      ) : null}
    </li>
  );
}

/** Estado de una tarea del alta, para la lista de progreso. */
function TaskRow({ t, state, label }: { t: T; state: TaskState; label: string }) {
  return (
    <li class="flex items-center gap-2.5 text-sm">
      <span class="grid h-5 w-5 shrink-0 place-items-center" aria-hidden="true">
        {state === 'done' ? (
          <Icon name="check" size={16} class="text-success-text" />
        ) : state === 'running' ? (
          <Spinner size={14} />
        ) : state === 'failed' ? (
          <Icon name="danger" size={16} class="text-danger-text" />
        ) : (
          <Icon name="circle" size={14} class="text-text-subtle" />
        )}
      </span>
      <span class={state === 'pending' ? 'text-text-subtle' : 'text-text'}>{label}</span>
      <span class="sr-only">{t(`npw.task.state.${state}`)}</span>
    </li>
  );
}

/** Estado del build a partir del despliegue: lo que la lista dice en su última fila. */
function buildTask(deploy: TaskState, status: DeployStatus | undefined): TaskState {
  if (deploy !== 'done') return 'pending';
  if (status === 'ready') return 'done';
  if (status === 'error' || status === 'canceled') return 'failed';
  return 'running';
}

function DeployProgress({
  t,
  project,
  pipeline,
  totalVars,
  status,
  url,
  logs,
  busy,
  onRetry,
}: {
  t: T;
  project: Project;
  pipeline: PipelineState;
  totalVars: number;
  status: DeployStatus | undefined;
  url: string;
  logs: LogLine[];
  busy: boolean;
  onRetry: () => void;
}) {
  const err = pipeline.error;
  const build = buildTask(pipeline.deploy, status);
  const liveUrl = url || project.productionUrl;
  return (
    <div class="flex max-w-3xl flex-col gap-4">
      <section class="border-border bg-surface-raised rounded-lg border p-4" aria-live="polite">
        <ol class="flex flex-col gap-2">
          <TaskRow t={t} state={pipeline.project} label={t('npw.task.project')} />
          {totalVars > 0 ? (
            <TaskRow
              t={t}
              state={pipeline.vars}
              label={t('npw.task.vars', { done: pipeline.varsSaved, total: totalVars })}
            />
          ) : null}
          <TaskRow t={t} state={pipeline.deploy} label={t('npw.task.deploy')} />
          <TaskRow
            t={t}
            state={build}
            label={
              status
                ? t('npw.task.build.status', { status: t(`deploy.status.${status}`) })
                : t('npw.task.build')
            }
          />
        </ol>
      </section>

      {/* Lo que falló, en el paso donde se ve, con lo que dijo el control-plane y
        la salida: «Reintentar» sigue donde se paró, sin volver a crear nada. */}
      {err ? (
        <InlineHelp
          tone="danger"
          title={
            err.phase === 'vars' ? t('npw.fail.vars', { key: err.key ?? '' }) : t('npw.fail.deploy')
          }
          action={
            <Button type="button" variant="secondary" size="sm" loading={busy} onClick={onRetry}>
              {t('common.tryAgain')}
            </Button>
          }
        >
          {describeError(t, err)} {t('npw.fail.keep')}
        </InlineHelp>
      ) : null}

      {pipeline.deploy === 'done' ? (
        <section class="flex flex-col gap-2">
          <h3 class="text-text text-sm font-semibold">{t('npw.logs.title')}</h3>
          <LogViewer t={t} lines={logs} query="" stick empty={t('npw.logs.waiting')} />
        </section>
      ) : null}

      {status === 'ready' ? (
        <InlineHelp tone="success" title={t('npw.result.ready')}>
          {/* Sin dirección conocida, «Responde en .» sería peor que nada. */}
          {liveUrl ? t('npw.result.ready.desc', { url: liveUrl.replace('https://', '') }) : null}
        </InlineHelp>
      ) : status === 'error' ? (
        <InlineHelp tone="danger" title={t('npw.result.error')}>
          {t('npw.result.error.desc')}
        </InlineHelp>
      ) : null}

      {project.webhookSecret ? <WebhookPanel t={t} project={project} /> : null}

      <div class="border-border-subtle flex flex-wrap items-center gap-3 border-t pt-4">
        <Button
          variant="primary"
          onClick={() => window.location.assign(r(`/projects/${project.slug}`))}
        >
          {t('tplw.done.open')}
        </Button>
        {status === 'ready' && liveUrl ? (
          <a
            href={liveUrl}
            target="_blank"
            rel="noreferrer"
            class="text-text-muted hover:text-text inline-flex items-center gap-1 text-sm"
          >
            {liveUrl.replace('https://', '')}
            <Icon name="arrowUpRight" size={14} />
          </a>
        ) : null}
      </div>
    </div>
  );
}

/**
 * Lo que hay que hacer con un proyecto recién creado: registrar el webhook en el
 * proveedor git. Dirección y secreto los devuelve el control-plane al crear y NO
 * se pueden volver a leer, así que esta es la única oportunidad de copiarlos.
 */
function WebhookPanel({ t, project }: { t: T; project: Project }) {
  return (
    <InlineHelp tone="warning" live="off" title={t('projects.hook.once')} titleAs="h3">
      <p>{t('projects.hook.once.desc')}</p>
      <div class="mt-3 flex flex-col gap-3">
        <CopyField
          labelWidth="stacked"
          label={t('projects.hook.url')}
          value={project.webhookUrl ?? ''}
        />
        <CopyField
          labelWidth="stacked"
          label={t('projects.hook.secret')}
          value={project.webhookSecret ?? ''}
        />
      </div>
      <ol class="mt-4 flex flex-col gap-3">
        {[
          { title: t('projects.hook.step1'), detail: t('projects.hook.step1.desc') },
          { title: t('projects.hook.step2'), detail: t('projects.hook.step2.desc') },
          { title: t('projects.hook.step3'), detail: t('projects.hook.step3.desc') },
        ].map((s, i) => (
          <li key={s.title} class="flex gap-3">
            <span
              aria-hidden="true"
              class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold"
            >
              {i + 1}
            </span>
            <div class="min-w-0">
              <p class="text-text text-sm font-medium">{s.title}</p>
              <p class="text-text-muted mt-0.5 text-sm leading-snug">{s.detail}</p>
            </div>
          </li>
        ))}
      </ol>
    </InlineHelp>
  );
}
