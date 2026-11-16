/**
 * ComandosDeCloud — lo que Sytue Cloud ofrece en ⌘K: NAVEGACIÓN (secciones), PROYECTOS
 * (GET /api/projects, carga perezosa a la primera apertura) y ACCIONES reales (crear
 * proyecto, volver a desplegar).
 *
 * Cloud tenía su propia paleta con el mismo atajo que la de la suite; ahora la paleta es
 * la de la shell (`PaletaDeLaSuite` de @sytue/shell), que busca además en toda la suite,
 * y aquí solo se registra la capa de Cloud. No pinta nada.
 */
import { useEffect, useState } from 'preact/hooks';
import { useToast } from '@sytue/ui';
import { useCapaDePaleta, usePaletaDeLaSuite, type ComandoDeApp } from '@sytue/shell';
import type { Locale } from '@sytue/i18n';
import type { PaletteCommand } from '../lib/palette.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';
import { useT } from '../i18n/index.js';

interface ComandosDeCloudProps {
  /** Idioma ya negociado en el servidor (llega por props desde CloudShell). */
  locale: Locale;
  /** Comandos de navegación (secciones), construidos por CloudShell. */
  nav: PaletteCommand[];
  /** Slug del proyecto activo (habilita las acciones de proyecto). */
  project?: string | undefined;
}

interface SdkProjectRow {
  slug: string;
  name: string;
  repo?: string;
}

export default function ComandosDeCloud({ locale, nav, project }: ComandosDeCloudProps): null {
  const t = useT(locale);
  const toast = useToast();
  const { abierta: open } = usePaletaDeLaSuite();
  const [projectCmds, setProjectCmds] = useState<PaletteCommand[]>([]);
  const [loadedProjects, setLoadedProjects] = useState(false);

  // Carga perezosa de proyectos a la primera apertura (el SDK es solo-servidor;
  // se pasa por el BFF). Sin backend vivo la paleta sigue con navegación+acciones.
  useEffect(() => {
    if (!open || loadedProjects) return;
    let alive = true;
    void (async () => {
      try {
        const res = await fetch('/api/projects', { headers: { accept: 'application/json' } });
        if (!res.ok) return;
        const data = (await res.json()) as { projects?: SdkProjectRow[] };
        if (!alive) return;
        setProjectCmds(
          (data.projects ?? []).map((p) => ({
            id: `project:${p.slug}`,
            group: t('palette.group.projects'),
            label: p.name,
            icon: 'grid',
            hint: p.slug,
            keywords: `${p.slug} ${p.repo ?? ''}`,
            run: () => go(`/projects/${p.slug}`),
          })),
        );
        setLoadedProjects(true);
      } catch {
        /* sin backend: se omite el grupo Proyectos */
      }
    })();
    return () => {
      alive = false;
    };
  }, [open, loadedProjects, t]);

  function go(href: string) {
    window.location.href = href;
  }

  async function redeploy() {
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/deploy', { project });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('toast.deployQueued'),
        description: res.live ? t('palette.deployQueued.desc') : t('toast.demoNotDeployed'),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('toast.deployFailed'),
        description: t('common.retry'),
      });
    }
  }

  const actions: PaletteCommand[] = [
    {
      id: 'action:new-project',
      group: t('palette.group.actions'),
      label: t('palette.createProject'),
      icon: 'plus',
      keywords: t('palette.createProject.keywords'),
      run: () => go('/new'),
    },
  ];
  if (project) {
    actions.push({
      id: 'action:redeploy',
      group: t('palette.group.actions'),
      label: t('palette.redeploy'),
      icon: 'rocket',
      hint: project,
      keywords: t('palette.redeploy.keywords'),
      run: () => void redeploy(),
    });
  }

  const commands = [...nav, ...actions, ...projectCmds];
  // Se registra en cada render: los `run` cierran sobre el proyecto y el registro solo
  // avisa a la paleta, no a esta capa, así que no hay bucle.
  useCapaDePaleta('cloud', {
    comandos: commands.map((c): ComandoDeApp => ({
      id: c.id,
      rotulo: c.label,
      grupo: c.group,
      icono: c.icon,
      ...(c.hint ? { pista: c.hint } : {}),
      ...(c.keywords ? { palabras: c.keywords } : {}),
      ejecutar: c.run,
    })),
  });
  return null;
}
