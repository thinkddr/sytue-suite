/**
 * ProjectsList — pantalla de proyectos (estilo Vercel): tarjetas con repo,
 * URL de producción, framework y servicios conectados. Buscador y CTA de nuevo
 * proyecto, que lleva al asistente de `/new`. docs/CONVENTIONS.md §14.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Card,
  CARD_TONES,
  CardBody,
  Button,
  Input,
  Badge,
  Icon,
  EmptyState,
  SectionHeader,
  ToastProvider,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { Project } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { FRAMEWORK_NAME } from '../lib/nuevo-proyecto.js';
import DemoBanner from './DemoBanner.js';
import { contiene } from '@sytue/i18n';

/** Nombres propios de framework; solo «estático» es una palabra que se traduce. */
function frameworkLabel(t: T, fw: string): string {
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
  const [projects] = useState(initial);
  const [live] = useState(initialLive);
  const [query, setQuery] = useState('');
  /**
   * Los enlaces de siempre (`?new=1` de la paleta y de marcadores, `?template=…`
   * de la galería) abrían aquí un modal de alta. El alta es ahora el asistente de
   * `/new`: se reenvían allí con sus parámetros para que ningún enlace guardado
   * se quede en una lista sin formulario.
   */
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('new') === null && params.get('template') === null) return;
    params.delete('new');
    const qs = params.toString();
    window.location.replace(r(`/new${qs ? `?${qs}` : ''}`));
  }, []);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return projects;
    return projects.filter((p) => contiene(p.name, q) || contiene(p.repo, q));
  }, [projects, query]);

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
              onClick={() => window.location.assign(r('/new'))}
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
                icon="search"
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
                icon="rocket"
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
                        onClick={() => window.location.assign(r('/new'))}
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
    </>
  );
}

export default function ProjectsList(props: Props) {
  return (
    <ToastProvider>
      <ProjectsInner {...props} />
    </ToastProvider>
  );
}
