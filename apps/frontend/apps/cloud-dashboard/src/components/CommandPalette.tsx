/**
 * CommandPalette — paleta de comandos ⌘K (estilo Linear) de Sytue DevX. Abre con
 * ⌘K / Ctrl+K (o el disparador de la sidebar) y salta a NAVEGACIÓN (secciones),
 * PROYECTOS (GET /api/projects, carga perezosa a la primera apertura) y ACCIONES
 * reales (crear proyecto, volver a desplegar). Teclado completo (↑↓ mueve, Enter
 * ejecuta, Esc cierra), foco atrapado en el diálogo con retorno, patrón ARIA
 * combobox + listbox con aria-activedescendant (el foco NO abandona el input, así
 * se sigue escribiendo mientras se navega). Solo @sytue/ui + tokens --sytue-*.
 *
 * La capa (campo, lista, teclado, foco, la hoja del teléfono) es `PaletaDeComandos` de
 * @sytue/ui, la misma del ERP: aquí solo se dice qué comandos hay.
 */
import { useEffect, useState } from 'preact/hooks';
import { PaletaDeComandos, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { filterCommands, type PaletteCommand } from '../lib/palette.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';
import { useT } from '../i18n/index.js';

interface CommandPaletteProps {
  /** Idioma ya negociado en el servidor (llega por props desde CloudShell). */
  locale: Locale;
  /** Comandos de navegación (secciones), construidos por CloudShell. */
  nav: PaletteCommand[];
  /** Slug del proyecto activo (habilita las acciones de proyecto). */
  project?: string | undefined;
  /** Etiqueta del atajo según plataforma (⌘K / Ctrl K), calculada por CloudShell. */
  metaHint: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface SdkProjectRow {
  slug: string;
  name: string;
  repo?: string;
}

export default function CommandPalette({
  locale,
  nav,
  project,
  metaHint,
  open,
  onOpenChange,
}: CommandPaletteProps) {
  const t = useT(locale);
  const toast = useToast();
  const [query, setQuery] = useState('');
  const [projectCmds, setProjectCmds] = useState<PaletteCommand[]>([]);
  const [loadedProjects, setLoadedProjects] = useState(false);

  // Atajo global ⌘K / Ctrl+K: alterna la paleta desde cualquier punto de la app.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onOpenChange]);

  // Cada apertura empieza sin consulta (foco, Escape, flechas y el foco de vuelta los
  // lleva PaletaDeComandos).
  useEffect(() => {
    if (open) setQuery('');
  }, [open]);

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
      run: () => go('/?new=1'),
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
  const filtered = filterCommands(commands, query);

  return (
    <PaletaDeComandos
      open={open}
      onClose={() => onOpenChange(false)}
      etiqueta={t('palette.dialog')}
      rotuloCampo={t('palette.inputLabel')}
      placeholder={t('palette.placeholder')}
      rotuloResultados={t('palette.results')}
      sinResultados={(q) => t('palette.noResults', { query: q })}
      consulta={query}
      onConsulta={setQuery}
      opciones={filtered.map((c) => ({
        id: c.id,
        rotulo: c.label,
        grupo: c.group,
        icono: c.icon,
        ...(c.hint ? { pista: c.hint } : {}),
        comando: c,
      }))}
      onElegir={(o) => {
        onOpenChange(false);
        void o.comando.run();
      }}
      pie={
        <>
          <span>
            <kbd class="text-text-muted font-sans">↑↓</kbd> {t('palette.hint.move')}
          </span>
          <span>
            <kbd class="text-text-muted font-sans">↵</kbd> {t('palette.hint.open')}
          </span>
          <span>
            <kbd class="text-text-muted font-sans">esc</kbd> {t('palette.hint.close')}
          </span>
          <span class="ml-auto">{metaHint}</span>
        </>
      }
    />
  );
}
