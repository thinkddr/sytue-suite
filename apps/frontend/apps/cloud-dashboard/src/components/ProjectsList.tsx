/**
 * ProjectsList — pantalla de proyectos (estilo Vercel): tarjetas con repo,
 * URL de producción, framework y servicios conectados. Buscador y CTA de nuevo
 * proyecto cableado a POST /api/projects. docs/CONVENTIONS.md §14.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Card,
  CARD_TONES,
  CardBody,
  Button,
  Input,
  Select,
  Badge,
  Icon,
  EmptyState,
  SectionHeader,
  Modal,
  FieldGroup,
  InlineHelp,
  describedBy,
  useToast,
  ToastProvider,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { Project } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { BffError, postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';
import { r } from '../lib/rutas.js';
import DemoBanner from './DemoBanner.js';
import { CopyField } from './CopyField.js';
import { contiene } from '@sytue/i18n';

/** Nombres propios de framework; solo «estático» es una palabra que se traduce. */
function frameworkLabel(t: T, fw: string): string {
  return fw === 'static' ? t('framework.static') : (FRAMEWORK_NAME[fw] ?? fw);
}

const FRAMEWORK_NAME: Record<string, string> = {
  astro: 'Astro',
  next: 'Next.js',
  preact: 'Preact',
  node: 'Node',
  sveltekit: 'SvelteKit',
  // Los devuelve la detección del repositorio aunque no estén en el desplegable:
  // sin nombre saldrían en minúscula y con pinta de erratas.
  go: 'Go',
  docker: 'Docker',
};

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

const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

/** Frameworks con opción propia en el Select; otros (p. ej. go) se añaden dinámicamente. */
const KNOWN_FRAMEWORKS = ['auto', 'astro', 'next', 'preact', 'static', 'node', 'sveltekit'];

/** Onboarding guiado (U1): qué ocurre al conectar un repositorio, en lenguaje llano. */
function onboardingSteps(t: T): Array<{ title: string; detail: string }> {
  return [
    { title: t('projects.step1.title'), detail: t('projects.step1.detail') },
    { title: t('projects.step2.title'), detail: t('projects.step2.detail') },
    { title: t('projects.step3.title'), detail: t('projects.step3.detail') },
  ];
}

interface Props {
  projects: Project[];
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

function ProjectsInner({ projects: initial, live: initialLive, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [projects, setProjects] = useState(initial);
  const [live] = useState(initialLive);
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const [repoUrl, setRepoUrl] = useState('');
  const [framework, setFramework] = useState<string>('auto');
  // Ajustes de compilación (pre-rellenados por una plantilla de arranque). '' = auto.
  const [buildStrategy, setBuildStrategy] = useState('');
  const [buildCommand, setBuildCommand] = useState('');
  const [installCommand, setInstallCommand] = useState('');
  const [outputDir, setOutputDir] = useState('');
  /** Rama que se publica ('' = el defecto del control-plane, `main`). */
  const [productionRef, setProductionRef] = useState('');
  /** Carpeta de la app dentro del repo ('' = la raíz). Es lo que hace posible un monorepo. */
  const [rootDir, setRootDir] = useState('');
  /**
   * Proyecto recién creado con sus credenciales de webhook. El control-plane
   * entrega el secreto UNA sola vez, así que mientras esto no sea null el modal
   * deja de ser un formulario y pasa a ser la única pantalla donde ese secreto
   * existe: redirigir sin enseñarlo lo perdía para siempre.
   */
  const [created, setCreated] = useState<Project | null>(null);
  /** Nombre de la plantilla activa (banner honesto dentro del modal), si vino de la galería. */
  const [templateName, setTemplateName] = useState('');
  /** Errores de validación por campo (inline en su FieldGroup, no como toast). */
  const [errors, setErrors] = useState<{ slug?: string; repo?: string }>({});
  const [suggesting, setSuggesting] = useState(false);
  /**
   * Disponibilidad del slug mientras se escribe. `unknown` NO es «libre»: sin
   * control-plane vivo nadie puede saberlo, y decir que sí sería mentir justo en el
   * dato por el que se pregunta.
   */
  const [slugStatus, setSlugStatus] = useState<'idle' | 'checking' | 'free' | 'taken' | 'unknown'>(
    'idle',
  );
  /** Nombres LIBRES derivados del que escribió el usuario, cuando el suyo está cogido. */
  const [alternatives, setAlternatives] = useState<string[]>([]);
  /**
   * Lo que se ha visto DENTRO del repositorio pegado. Es lo que permite dejar de
   * preguntar framework, comandos y directorio de salida: están escritos ahí.
   *
   * `null` = todavía no se ha mirado, no se pudo, o no se sacó nada en claro; en
   * los tres casos el formulario pregunta, como hacía siempre.
   */
  const [detection, setDetection] = useState<{
    framework: string;
    evidence: string;
    confident: boolean;
  } | null>(null);
  const [inspecting, setInspecting] = useState(false);
  /**
   * El usuario ha pedido cambiar lo detectado. Una vez pulsado no se vuelve a
   * pisar: sobreescribir su elección con la siguiente detección sería quitarle el
   * control justo después de dárselo.
   */
  const [overrideDetection, setOverrideDetection] = useState(false);

  function openModal() {
    setErrors({});
    setCreated(null);
    setOpen(true);
  }

  /** Cierra el modal y, si ya hay proyecto, entra en él. */
  function goToProject() {
    const target = created?.slug;
    setOpen(false);
    setCreated(null);
    if (target) window.location.href = r(`/projects/${target}`);
  }

  /**
   * Pide un nombre libre al control-plane. El backend lo COMPRUEBA contra el índice
   * global antes de devolverlo; si aun así no pudo confirmarlo, se dice, porque
   * prometer «libre» y luego chocar es peor que no prometer nada.
   */
  async function suggestSlug() {
    setSuggesting(true);
    try {
      const res = (await postBff('/api/suggest-slug', { lang: locale })) as unknown as {
        slug?: string;
        available?: boolean;
      };
      if (!res.slug) {
        toast.notify({ tone: 'warning', title: t('projects.suggest.failNow') });
        return;
      }
      setSlug(res.slug);
      setErrors((prev) => {
        const { slug: _quitado, ...resto } = prev;
        return resto;
      });
      if (!res.available) {
        toast.notify({
          tone: 'warning',
          title: t('projects.suggest.try', { slug: res.slug }),
          description: t('projects.suggest.tryDesc'),
        });
      }
    } catch {
      toast.notify({ tone: 'danger', title: t('projects.suggest.fail') });
    } finally {
      setSuggesting(false);
    }
  }

  /**
   * Disponibilidad EN VIVO mientras se escribe, como GitHub o Vercel: el slug es el
   * subdominio público y su índice es único en TODA la plataforma, así que sin esto
   * el usuario no se entera hasta el 409 de la creación —y ese 409 no puede decirle
   * que el nombre lo tiene otra organización—.
   *
   * Con debounce: una petición por tecla convertiría escribir «mi-tienda» en nueve
   * consultas cross-tenant. Se espera a que la mano pare. El `cancelado` de la
   * limpieza descarta las respuestas que llegan tarde: sin él, la comprobación de un
   * nombre anterior podría pintarse encima de la del nombre actual.
   */
  useEffect(() => {
    const s = slug.trim().toLowerCase();
    setAlternatives([]);
    // Un nombre con formato inválido ya lo dice la validación del campo; preguntar
    // por él sería un 400 seguro.
    if (!open || !s || !SLUG_RE.test(s)) {
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
  }, [slug, open, locale]);

  /**
   * Mira el repositorio pegado y rellena lo que se pueda deducir de él.
   *
   * Es la diferencia entre un formulario de siete decisiones y uno de dos. Las
   * respuestas no las tiene el usuario: las tiene su repositorio —el
   * `astro.config.mjs`, el lockfile, el script `build`—, así que preguntárselas es
   * pedirle que traduzca su propio proyecto a nuestro vocabulario.
   *
   * Tres reglas para no quitarle el control:
   *
   *  - **Nunca pisa lo que hay escrito.** Si el campo trae valor —lo puso él, o lo
   *    trajo una plantilla— se respeta.
   *  - **Nunca pisa después de que haya pedido cambiarlo** (`overrideDetection`).
   *  - **Un fallo no bloquea nada.** Sin detección, el formulario es el de antes.
   */
  useEffect(() => {
    const url = repoUrl.trim();
    if (!open || !url.startsWith('https://')) {
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
          // Rellenar SIN pisar: un valor ya escrito es una decisión, y la
          // detección es una sugerencia por bien fundada que esté.
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
    // `rootDir` entra porque en un monorepo la app vive en otra carpeta y lo que
    // hay en la raíz no dice nada de ella.
  }, [repoUrl, rootDir, open, overrideDetection]);

  // Deep-link ?new=1 (paleta ⌘K) o ?template=… (galería de plantillas): abre el
  // modal y, si vienen ajustes de plantilla, pre-rellena el formulario con ellos.
  // Limpia los parámetros para que no reaparezcan al recargar.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('new') === null && params.get('template') === null) return;
    setErrors({});
    // `tpl`, no `t`: `t` es la función de traducción de esta isla y llamar así a
    // una variable local la ensombrece dentro del efecto —el fallo que deja un
    // bloque sin traducir sin que el compilador diga nada—.
    const tpl = params.get('template');
    if (tpl) setTemplateName(tpl);
    const fw = params.get('framework');
    if (fw) setFramework(fw);
    const bs = params.get('build_strategy');
    if (bs) setBuildStrategy(bs);
    const bc = params.get('build_command');
    if (bc) setBuildCommand(bc);
    const ic = params.get('install_command');
    if (ic) setInstallCommand(ic);
    const od = params.get('output_dir');
    if (od) setOutputDir(od);
    setOpen(true);
    for (const k of [
      'new',
      'template',
      'framework',
      'build_strategy',
      'build_command',
      'install_command',
      'output_dir',
    ])
      params.delete(k);
    const qs = params.toString();
    window.history.replaceState({}, '', qs ? `?${qs}` : window.location.pathname);
  }, []);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return projects;
    return projects.filter((p) => contiene(p.name, q) || contiene(p.repo, q));
  }, [projects, query]);

  async function create(e: Event) {
    e.preventDefault();
    const s = slug.trim().toLowerCase();
    // Validación inline: el error se ancla a su campo (FieldGroup), no a un toast
    // transitorio desligado del input que falla. El toast queda para el resultado de red.
    const errs: { slug?: string; repo?: string } = {};
    if (!SLUG_RE.test(s)) errs.slug = t('projects.field.slug.format');
    // Ya sabemos que está cogido: mandarlo sería pedir un 409 a propósito. Si la
    // comprobación no pudo hacerse (`unknown`) NO se bloquea nada: quien decide es
    // el control-plane, no una suposición del navegador.
    else if (slugStatus === 'taken') errs.slug = t('projects.field.slug.taken');
    if (!repoUrl.trim().startsWith('https://')) errs.repo = t('projects.field.repo.error');
    setErrors(errs);
    if (errs.slug || errs.repo) return;
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; project?: Project }>(
        '/api/projects',
        {
          slug: s,
          name: name.trim() || s,
          repo_url: repoUrl.trim(),
          framework,
          build_strategy: buildStrategy,
          build_command: buildCommand.trim(),
          install_command: installCommand.trim(),
          output_dir: outputDir.trim(),
          production_ref: productionRef.trim(),
          root_dir: rootDir.trim(),
        },
      );
      if (res.project) {
        setProjects((prev) => [res.project!, ...prev.filter((p) => p.slug !== res.project!.slug)]);
        notifyDemoIfNeeded(t, toast, res.live);
        toast.notify({
          tone: 'success',
          title: t('projects.created'),
          description: res.live
            ? t('projects.created.desc', { slug: s })
            : t('projects.created.demo', { slug: s }),
        });
        // Con secreto de webhook el modal se queda abierto enseñándolo: es la
        // ÚNICA vez que el control-plane lo entrega y sin él nadie puede conectar
        // «git push → despliegue». Sin secreto (demostración, o token de API) el
        // comportamiento es el de siempre.
        if (res.project.webhookSecret) {
          setCreated(res.project);
        } else {
          setOpen(false);
          if (res.live) window.location.href = r(`/projects/${s}`);
        }
      }
    } catch (e) {
      // Un 403 NO se reintenta: es un permiso que falta, no un fallo pasajero.
      // Medido en producción el 2026-08-13 con una cuenta employee: el alta
      // devolvía 403 y el aviso decía «Reintenta en unos segundos», así que la
      // persona reintenta algo que no va a funcionar nunca y no se entera de
      // que lo que necesita es que un administrador le dé acceso.
      const sinPermiso = e instanceof BffError && (e.status === 403 || e.status === 401);
      toast.notify({
        tone: 'danger',
        title: sinPermiso ? t('error.forbidden') : t('projects.create.fail'),
        description: sinPermiso ? t('demo.denied') : t('common.retry'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="grid" size={20} />}
        title={t('nav.projects')}
        subtitle={t('projects.subtitle')}
        actions={
          <>
            <Input
              type="search"
              value={query}
              placeholder={t('projects.searchLabel')}
              aria-label={t('projects.searchLabel')}
              onInput={(e) => setQuery(e.currentTarget.value)}
              // El marcador es el rótulo corto, no la frase larga: «Search
              // project or repository» no cabía en la caja y ensancharla para
              // que cupiera echaba los dos botones fuera de la pantalla.
              class="w-64 min-w-0"
            />
            <Button
              variant="secondary"
              class="shrink-0 whitespace-nowrap"
              leadingIcon={<Icon name="rocket" size={16} />}
              onClick={() => {
                window.location.href = r('/templates');
              }}
            >
              {t('nav.templates')}
            </Button>
            <Button
              variant="primary"
              class="shrink-0 whitespace-nowrap"
              leadingIcon={<Icon name="plus" size={16} />}
              onClick={openModal}
            >
              {t('projects.new')}
            </Button>
          </>
        }
      />
      <div class="cloud-content">
        {filtered.length === 0 ? (
          <Card class="mt-2">
            {query.trim() ? (
              <EmptyState
                illustration={<Icon name="search" size={24} />}
                title={t('projects.noResults.title')}
                description={t('projects.noResults.desc', { query: query.trim() })}
                action={
                  <Button variant="secondary" onClick={() => setQuery('')}>
                    {t('projects.noResults.clear')}
                  </Button>
                }
              />
            ) : (
              <EmptyState
                illustration={<Icon name="rocket" size={24} />}
                title={t('projects.empty.title')}
                description={t('projects.empty.desc')}
                action={
                  <div class="flex flex-col items-stretch gap-5">
                    <ol class="mx-auto flex max-w-md flex-col gap-3 text-left">
                      {onboardingSteps(t).map((s, i) => (
                        <li key={s.title} class="flex items-start gap-3">
                          <span class="bg-selected text-primary-text mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full text-sm font-semibold tabular-nums">
                            {i + 1}
                          </span>
                          <span class="min-w-0">
                            <span class="text-text block font-medium">{s.title}</span>
                            <span class="text-text-muted block text-sm">{s.detail}</span>
                          </span>
                        </li>
                      ))}
                    </ol>
                    <div class="flex justify-center">
                      <Button
                        variant="primary"
                        leadingIcon={<Icon name="plus" size={16} />}
                        onClick={openModal}
                      >
                        {t('projects.empty.cta')}
                      </Button>
                    </div>
                  </div>
                }
              />
            )}
          </Card>
        ) : (
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {filtered.map((p, index) => (
              <a key={p.slug} href={r(`/projects/${p.slug}`)} class="group block rounded-lg">
                <Card
                  tone={CARD_TONES[index % CARD_TONES.length] ?? 'neutral'}
                  class="group-hover:border-border-strong h-full transition-colors duration-150 ease-[var(--ease-standard)]"
                >
                  <CardBody class="flex h-full flex-col gap-3">
                    <div class="flex items-center gap-2.5">
                      <span
                        class="bg-selected text-primary-text grid h-9 w-9 shrink-0 place-items-center rounded-md"
                        aria-hidden="true"
                      >
                        <Icon name="grid" size={18} />
                      </span>
                      <div class="min-w-0">
                        <p class="text-text truncate font-semibold" title={p.name}>
                          {p.name}
                        </p>
                        <p class="text-text-muted truncate text-sm" title={p.repo}>
                          {p.repo}
                        </p>
                      </div>
                    </div>
                    <p class="text-text-muted truncate text-sm" title={p.productionUrl}>
                      {p.productionUrl.replace('https://', '')}
                    </p>
                    <div class="mt-auto flex flex-wrap items-center gap-1.5">
                      <Badge tone="primary">{frameworkLabel(t, p.framework)}</Badge>
                      <Badge tone="neutral">{p.region}</Badge>
                      {p.services.map((s) => (
                        <Badge key={s} tone="accent">
                          {serviceLabel(t, s)}
                        </Badge>
                      ))}
                    </div>
                  </CardBody>
                </Card>
              </a>
            ))}
          </div>
        )}
      </div>

      <Modal
        open={open}
        // Con el secreto en pantalla, cerrar es la última oportunidad de copiarlo:
        // se sale por el botón explícito, no por un descarte accidental.
        onClose={() => (created ? goToProject() : setOpen(false))}
        title={
          created ? t('projects.hook.title', { name: created.name }) : t('projects.modal.title')
        }
        // Las acciones en el pie del diálogo, no al final del formulario: a todo el
        // ancho en el teléfono y siempre a la vista aunque el cuerpo se desplace.
        footer={
          created ? (
            <Button variant="primary" onClick={goToProject}>
              {t('projects.hook.done')}
            </Button>
          ) : (
            <>
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" form="proyecto-nuevo" variant="primary" loading={busy}>
                {t('projects.create')}
              </Button>
            </>
          )
        }
      >
        {created ? (
          <ProjectCreatedPanel t={t} project={created} />
        ) : (
          <form id="proyecto-nuevo" class="flex flex-col gap-4" onSubmit={(e) => void create(e)}>
            {templateName ? (
              <div class="border-border bg-bg-subtle text-text-muted flex items-start gap-2 rounded-md border px-3 py-2 text-sm">
                <Icon name="info" size={16} class="text-text-subtle mt-0.5 shrink-0" />
                <span>{t('projects.modal.template', { name: templateName })}</span>
              </div>
            ) : null}
            <FieldGroup
              label={t('projects.field.slug')}
              htmlFor="proj-slug"
              help={t('projects.field.slug.help')}
              required
              {...(errors.slug ? { error: errors.slug } : {})}
            >
              <Input
                id="proj-slug"
                value={slug}
                required
                placeholder={t('projects.field.slug.placeholder')}
                invalid={Boolean(errors.slug)}
                aria-describedby={describedBy(
                  'proj-slug',
                  t('projects.field.slug.help'),
                  errors.slug,
                )}
                onInput={(e) => setSlug(e.currentTarget.value)}
              />
              {/* Resultado de la comprobación en vivo, alternativas incluidas.
                `aria-live` porque un color y un icono no existen para un lector de
                pantalla: el estado se anuncia, no solo se pinta —y con él la salida,
                que es lo que de verdad hace falta oír cuando el nombre está cogido—.
                `polite` para no interrumpir mientras se teclea. */}
              <div class="mt-1.5 min-h-5 text-sm" role="status" aria-live="polite">
                {slugStatus === 'checking' ? (
                  <span class="text-text-muted">{t('projects.slug.checking')}</span>
                ) : null}
                {slugStatus === 'free' ? (
                  <span class="text-success-text flex items-center gap-1.5">
                    <Icon name="check" size={14} aria-hidden="true" />
                    {t('projects.slug.free', { slug: slug.trim().toLowerCase() })}
                  </span>
                ) : null}
                {slugStatus === 'taken' ? (
                  <span class="text-warning-text flex items-center gap-1.5">
                    <Icon name="warning" size={14} aria-hidden="true" />
                    {t('projects.slug.busy', { slug: slug.trim().toLowerCase() })}
                  </span>
                ) : null}
                {slugStatus === 'unknown' ? (
                  <span class="text-text-muted flex items-center gap-1.5">
                    <Icon name="info" size={14} aria-hidden="true" />
                    {t('projects.slug.unknown')}
                  </span>
                ) : null}
                {alternatives.length > 0 ? (
                  // Alternativas LIBRES derivadas del nombre que eligió el usuario. Un
                  // clic las acepta: no hay que volver a teclear ni recordarlas.
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="text-text-muted">{t('projects.slug.alternatives')}</span>
                    {alternatives.map((a) => (
                      <Button
                        key={a}
                        type="button"
                        variant="secondary"
                        size="sm"
                        aria-label={t('projects.slug.useName', { name: a })}
                        onClick={() => setSlug(a)}
                      >
                        {a}
                      </Button>
                    ))}
                  </div>
                ) : null}
              </div>
            </FieldGroup>
            {/* El slug es único en TODA la plataforma, así que quien se inventa uno
                choca con un 409 sin poder saber por qué —lo tiene otra
                organización— ni qué probar después. Esto propone uno ya
                comprobado: crear un proyecto pasa a ser aceptar lo que hay. Va fuera
                del grupo: dentro quedaba entre el campo y su ayuda. */}
            <div class="-mt-2">
              <Button
                type="button"
                variant="ghost"
                loading={suggesting}
                disabled={suggesting}
                onClick={() => void suggestSlug()}
              >
                {slug ? t('projects.slug.suggestAnother') : t('projects.slug.suggest')}
              </Button>
            </div>
            <FieldGroup label={t('projects.field.name')} htmlFor="proj-name">
              <Input
                id="proj-name"
                value={name}
                placeholder={t('projects.field.name.placeholder')}
                onInput={(e) => setName(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup
              label={t('projects.field.repo')}
              htmlFor="proj-repo"
              required
              {...(errors.repo ? { error: errors.repo } : {})}
            >
              <Input
                id="proj-repo"
                value={repoUrl}
                required
                placeholder="https://git.sytue.eu/org/repo"
                class="font-mono text-sm"
                invalid={Boolean(errors.repo)}
                aria-describedby={describedBy('proj-repo', undefined, errors.repo)}
                onInput={(e) => setRepoUrl(e.currentTarget.value)}
              />
            </FieldGroup>
            {/* Detectado y comprobable: se dice lo que se ha visto y CON QUÉ prueba,
              en vez de un desplegable que el usuario tiene que rellenar con datos
              de su propio repositorio. El botón devuelve el control sin esconderlo:
              «Astro» acertado el 95 % de las veces sigue siendo un 5 % de gente
              atrapada si no hay forma de cambiarlo. */}
            {detection?.confident && !overrideDetection ? (
              <InlineHelp tone="success" title={t('projects.detected.title')}>
                <p class="text-sm">
                  {t('projects.detected.desc', {
                    framework: frameworkLabel(t, framework),
                    evidence: detection.evidence,
                  })}
                </p>
                <Button
                  type="button"
                  variant="ghost"
                  class="mt-2"
                  onClick={() => setOverrideDetection(true)}
                >
                  {t('projects.detected.change')}
                </Button>
              </InlineHelp>
            ) : (
              <FieldGroup
                label={t('projects.field.framework')}
                htmlFor="proj-fw"
                {...(inspecting ? { help: t('projects.detecting') } : {})}
                {...(detection && !detection.confident
                  ? { help: t('projects.detected.unsure', { evidence: detection.evidence }) }
                  : {})}
              >
                <Select
                  id="proj-fw"
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
                  {/* La plantilla o la detección pueden traer un stack que aún no
                    listamos (p. ej. go, docker). */}
                  {!KNOWN_FRAMEWORKS.includes(framework) ? (
                    <option value={framework}>
                      {framework.charAt(0).toUpperCase() + framework.slice(1)}
                    </option>
                  ) : null}
                </Select>
              </FieldGroup>
            )}
            {/* Opciones avanzadas: rama, carpeta y compilación. Opcionales y, salvo
              las que ya trae una plantilla, plegadas. Nativo <details> para no
              cargar el modal cuando no se usan. */}
            <details
              class="group"
              open={Boolean(
                buildCommand || installCommand || outputDir || productionRef || rootDir,
              )}
            >
              <summary class="text-text-muted hover:text-text flex cursor-pointer items-center gap-1.5 text-sm select-none">
                <Icon
                  name="chevronRight"
                  size={14}
                  class="transition-transform group-open:rotate-90"
                />
                {t('projects.build.summary')}
              </summary>
              <div class="mt-3 flex flex-col gap-4">
                {/* Sin estas dos NO se puede publicar un repo cuya rama principal no
                  se llame «main», ni una app que viva dentro de un monorepo. */}
                <FieldGroup
                  label={t('projects.field.branch')}
                  htmlFor="proj-branch"
                  help={t('projects.field.branch.help')}
                >
                  <Input
                    id="proj-branch"
                    value={productionRef}
                    placeholder="main"
                    class="font-mono text-sm"
                    aria-describedby={describedBy('proj-branch', t('projects.field.branch.help'))}
                    onInput={(e) => setProductionRef(e.currentTarget.value)}
                  />
                </FieldGroup>
                <FieldGroup
                  label={t('projects.field.rootDir')}
                  htmlFor="proj-root"
                  help={t('projects.field.rootDir.help')}
                >
                  <Input
                    id="proj-root"
                    value={rootDir}
                    placeholder="apps/web"
                    class="font-mono text-sm"
                    aria-describedby={describedBy('proj-root', t('projects.field.rootDir.help'))}
                    onInput={(e) => setRootDir(e.currentTarget.value)}
                  />
                </FieldGroup>
                <FieldGroup
                  label={t('projects.build.strategy')}
                  htmlFor="proj-strategy"
                  help={t('projects.build.strategy.help')}
                >
                  <Select
                    id="proj-strategy"
                    value={buildStrategy}
                    onChange={(e) => setBuildStrategy(e.currentTarget.value)}
                  >
                    <option value="">{t('projects.build.strategy.auto')}</option>
                    <option value="dockerfile">Dockerfile</option>
                    <option value="buildpacks">Buildpacks</option>
                  </Select>
                </FieldGroup>
                <FieldGroup label={t('projects.build.install')} htmlFor="proj-install">
                  <Input
                    id="proj-install"
                    value={installCommand}
                    placeholder="npm install"
                    class="font-mono text-sm"
                    onInput={(e) => setInstallCommand(e.currentTarget.value)}
                  />
                </FieldGroup>
                <FieldGroup label={t('projects.build.command')} htmlFor="proj-build">
                  <Input
                    id="proj-build"
                    value={buildCommand}
                    placeholder="npm run build"
                    class="font-mono text-sm"
                    onInput={(e) => setBuildCommand(e.currentTarget.value)}
                  />
                </FieldGroup>
                <FieldGroup label={t('projects.build.output')} htmlFor="proj-output">
                  <Input
                    id="proj-output"
                    value={outputDir}
                    placeholder="dist"
                    class="font-mono text-sm"
                    onInput={(e) => setOutputDir(e.currentTarget.value)}
                  />
                </FieldGroup>
              </div>
            </details>
          </form>
        )}
      </Modal>
    </>
  );
}

/**
 * Lo que hay que hacer AHORA con un proyecto recién creado: registrar el webhook
 * en el proveedor git. La dirección y el secreto los devuelve el control-plane en
 * la respuesta de creación y NO se pueden volver a leer, así que esta pantalla es
 * la única oportunidad de copiarlos —y lo dice—.
 */
function ProjectCreatedPanel({ t, project }: { t: T; project: Project }) {
  return (
    <div class="flex flex-col gap-4">
      <InlineHelp tone="success" title={t('projects.hook.ready')}>
        {t('projects.hook.ready.desc')}
      </InlineHelp>

      <section class="border-warning bg-warning-bg rounded-lg border p-4">
        <h3 class="text-warning-text text-sm font-semibold">{t('projects.hook.once')}</h3>
        <p class="text-text-muted mt-1 text-sm leading-relaxed">{t('projects.hook.once.desc')}</p>
        <div class="mt-3 flex flex-col gap-3">
          <CopyField t={t} label={t('projects.hook.url')} value={project.webhookUrl ?? ''} />
          <CopyField t={t} label={t('projects.hook.secret')} value={project.webhookSecret ?? ''} />
        </div>
      </section>

      <section class="border-border bg-surface-raised rounded-lg border p-4">
        <h3 class="text-text text-sm font-semibold">{t('projects.hook.next')}</h3>
        <ol class="mt-3 flex flex-col gap-3">
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
      </section>
    </div>
  );
}

export default function ProjectsList(props: Props) {
  return (
    <ToastProvider>
      <ProjectsInner {...props} />
    </ToastProvider>
  );
}
