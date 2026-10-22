/**
 * AppCatalogPanel — elegir varios productos open-source y que arranquen juntos y
 * conectados.
 *
 * La selección es MÚLTIPLE porque la promesa es componer, no instalar: un CMS y
 * su caché son una decisión, no dos. Y lo que se enseña antes de confirmar no es
 * un resumen del carrito, es QUÉ SE VA A CONECTAR CON QUÉ y el enlace a la
 * documentación oficial que lo respalda —sin eso, «se integran solas» es algo que
 * el usuario tiene que creerse—.
 *
 * La pantalla enseña además cuatro cosas que normalmente se esconden:
 *
 *  - **La licencia de cada app, y su código fuente.** Quien despliega software de
 *    otro tiene derecho a saber bajo qué términos lo hace; y con AGPL o GPL el
 *    enlace al fuente no es cortesía, es lo que la licencia obliga a ofrecer a
 *    quien usa el servicio (ADR 0032).
 *  - **Qué variables va a recibir tu app.** Es la respuesta a «y esto cómo lo
 *    uso»: si crear una caché no te dice que vas a tener `VALKEY_URL`, tienes que
 *    ir a buscarlo.
 *  - **Si tendrá dirección web o no.** Es la diferencia entre un producto que vas
 *    a usar y una pieza de apoyo para tu app, y descubrirlo después de crearla es
 *    tarde.
 *  - **Lo que está en camino.** Una entrada revisada cuya imagen aún no se ha
 *    replicado se enseña apagada en vez de ofrecerse: crearla fallaría después de
 *    haberte prometido el servicio.
 *
 * No hay campo para escribir una imagen, y no es un descuido: la imagen no viaja
 * nunca desde el cliente (ADR 0029). Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  InlineHelp,
  CardHeader,
  CardTitle,
  Modal,
  Select,
  ToastProvider,
  useToast,
  Checkbox,
} from '@sytue/ui';
import type { AppCatalog, CatalogApp } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import type { ManagedService } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';

interface Props {
  projectSlug: string;
  catalog: AppCatalog;
  /** Las que este proyecto ya tiene levantadas. */
  installed: ManagedService[];
  /**
   * El proyecto tiene un Sytue Postgres EN MARCHA. No basta con que exista: una
   * base a medio aprovisionar todavía no tiene credenciales, así que el
   * control-plane rechaza el alta igual y la tarjeta estaría mintiendo.
   */
  hayPostgres: boolean;
  live: boolean;
  locale: Locale;
}

export default function AppCatalogPanel(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} t={useT(props.locale)} />
    </ToastProvider>
  );
}

/**
 * Estado de un servicio ya levantado. Sale del catálogo como el resto de la
 * pantalla: el identificador de la API («running», «provisioning») es contrato,
 * no rótulo, y pintarlo crudo dejaba esta lista en inglés en los cinco idiomas.
 */
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

const categoryKey = {
  cache: 'apps.cat.cache',
  search: 'apps.cat.search',
  queue: 'apps.cat.queue',
  cms: 'apps.cat.cms',
  tools: 'apps.cat.tools',
  devtools: 'apps.cat.devtools',
  observability: 'apps.cat.observability',
  productivity: 'apps.cat.productivity',
  // El `satisfies` no es adorno: añadir una categoría en el backend y olvidar su
  // etiqueta aquí deja la tarjeta con la clave cruda a la vista. Así no compila.
} as const satisfies Record<CatalogApp['category'], string>;

function AppCard({
  app,
  t,
  busy,
  installed,
  selected,
  hayPostgres,
  onToggle,
}: {
  app: CatalogApp;
  t: T;
  busy: boolean;
  /** Ya levantada en este proyecto: no se puede elegir dos veces. */
  installed: boolean;
  selected: boolean;
  /** El proyecto ya tiene un Sytue Postgres listo (lo exigen algunas apps). */
  hayPostgres: boolean;
  onToggle: () => void;
}) {
  const id = `app-${app.id}`;
  return (
    <Card>
      <CardBody class="flex h-full flex-col gap-3">
        <div class="flex items-start justify-between gap-3">
          <div class="flex min-w-0 items-start gap-3">
            {/* Casilla nativa: es un multiselect de verdad, no una lista de
                botones que se pulsan de uno en uno. */}
            <Checkbox
              ringClass="mt-1"
              id={id}
              class="accent-primary size-4 shrink-0"
              checked={selected}
              disabled={busy || installed}
              onChange={onToggle}
            />
            <div class="min-w-0">
              <label for={id} class="text-base font-medium">
                {app.name}
              </label>
              <p class="text-text-subtle text-sm">
                {t(categoryKey[app.category])} · {app.version}
              </p>
            </div>
          </div>
          {/* La licencia no es letra pequeña: es la razón de que esta app esté
              aquí y otra no. */}
          <Badge tone="neutral">{app.license}</Badge>
        </div>
        <p class="text-text-subtle text-sm">{app.description}</p>

        {app.exports.length > 0 ? (
          <p class="sytue-meta">
            {t('apps.injects')}{' '}
            <code class="font-mono">{app.exports.map((e) => e.key).join(', ')}</code>
          </p>
        ) : null}

        {/* Publicable o no: es la diferencia entre «tendrás una URL» y «solo la
            usa tu app», y decirlo después de crearla es tarde. */}
        <p class="sytue-meta">{app.web_port ? t('apps.public') : t('apps.internal')}</p>

        {/* Lo que necesita para arrancar. En rojo cuando el proyecto no lo tiene:
            el alta lo rechaza, y enterarse al pulsar «crear» —o peor, viendo un
            contenedor reiniciarse— es tarde. */}
        {app.needs?.includes('postgres') ? (
          <p class={hayPostgres ? 'sytue-meta' : 'text-danger-text text-sm'}>
            {hayPostgres ? t('apps.needs.db.ok') : t('apps.needs.db.missing')}
          </p>
        ) : null}

        <div class="mt-auto flex flex-wrap items-center gap-x-4 gap-y-1 pt-2">
          <a
            href={app.homepage}
            target="_blank"
            rel="noreferrer noopener"
            class="text-text-subtle text-sm underline"
          >
            {t('apps.homepage')}
          </a>
          {/* El fuente va al lado de la licencia, no escondido: con AGPL o GPL es
              lo que la licencia exige ofrecer a quien usa el servicio. */}
          {app.source ? (
            <a
              href={app.source}
              target="_blank"
              rel="noreferrer noopener"
              class="text-text-subtle text-sm underline"
            >
              {t('apps.source')}
            </a>
          ) : null}
          {installed ? <Badge tone="success">{t('apps.alreadyInstalled')}</Badge> : null}
        </div>
      </CardBody>
    </Card>
  );
}

/**
 * Las integraciones que se van a activar con lo que hay elegido.
 *
 * Es lo que convierte un multiselect en una composición: se calcula sobre la
 * selección MÁS lo ya instalado, porque el control-plane conecta lo que conviva
 * en el proyecto, no lo que se marcó en esta pantalla. Enseñarlo solo de lo
 * marcado prometería de menos justo en el caso que más importa —añadir una caché
 * a un proyecto que ya tiene el CMS puesto—.
 */
function integracionesActivas(
  apps: CatalogApp[],
  presentes: Set<string>,
): { from: CatalogApp; to: CatalogApp; docs: string; keys: string[] }[] {
  const porId = new Map(apps.map((a) => [a.id, a]));
  const out: { from: CatalogApp; to: CatalogApp; docs: string; keys: string[] }[] = [];
  for (const id of presentes) {
    const app = porId.get(id);
    if (!app) continue;
    for (const i of app.integrations ?? []) {
      const otra = porId.get(i.app);
      if (!otra || !presentes.has(i.app)) continue;
      out.push({ from: app, to: otra, docs: i.docs, keys: i.env.map((e) => e.key) });
    }
  }
  return out;
}

function Inner({ projectSlug, catalog, installed, hayPostgres, live, t }: Props & { t: T }) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  // La lista de instaladas es estado: retirar una tiene que quitarla de aquí, y
  // también del conjunto que decide qué integraciones se anuncian abajo.
  const [puestas, setPuestas] = useState<ManagedService[]>(installed);
  const [pendingDelete, setPendingDelete] = useState<ManagedService | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Filtro por categoría. El catálogo empezó con tres entradas y una rejilla
  // plana bastaba; con productos completos dentro esto crece, y buscar «la wiki»
  // entre veinte tarjetas iguales no es buscar, es rastrear.
  const [categoria, setCategoria] = useState<string>('');
  const todas = catalog.apps;
  // Solo las categorías que EXISTEN, no las ocho del dominio: un desplegable con
  // opciones que no filtran nada es una promesa incumplida por partida doble.
  const categorias = [...new Set(todas.map((a) => a.category))].sort();
  const apps = categoria ? todas.filter((a) => a.category === categoria) : todas;
  const yaPuestas = new Set(puestas.map((s) => s.appId).filter(Boolean) as string[]);
  // Lo que habrá en el proyecto si se confirma: lo instalado más lo marcado. Es
  // el conjunto sobre el que el control-plane resolverá las integraciones.
  const presentes = new Set([...yaPuestas, ...selected]);
  const integraciones = integracionesActivas(apps, presentes);

  async function confirmDelete() {
    const target = pendingDelete;
    if (!target?.id) return;
    setDeleting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/delete', {
        service: target.id,
      });
      setPuestas((prev) => prev.filter((s) => s.id !== target.id));
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('apps.removed', { name: target.name }),
        ...demoDesc(t, res.live),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('apps.remove.fail') });
    } finally {
      setDeleting(false);
    }
  }

  function toggle(id: string) {
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  }

  /**
   * Crea todas las elegidas.
   *
   * En serie y no en paralelo, a propósito: cada alta comprueba la cuota de
   * servicios del plan, y lanzarlas a la vez haría que varias leyeran el mismo
   * recuento y se colaran por encima del límite. Son dos o tres peticiones.
   *
   * No hay endpoint de lote porque no hace falta uno: lo que haría un lote
   * —dejarlas conectadas entre sí— ya no depende del orden ni de la atomicidad,
   * lo resuelve el control-plane reencolando lo que se integra con lo que va
   * apareciendo.
   */
  async function createSelected() {
    if (selected.length === 0) return;
    setBusy(true);
    try {
      let demo = false;
      for (const id of selected) {
        const app = apps.find((a) => a.id === id);
        if (!app) continue;
        const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/create', {
          project: projectSlug,
          kind: 'app',
          app_id: app.id,
          name: app.name,
        });
        demo = demo || !res.live;
      }
      if (demo) {
        notifyDemoIfNeeded(t, toast, false);
        setSelected([]);
        return;
      }
      // Las levanta el control-plane en segundo plano: se recarga para leer el
      // estado REAL en vez de pintar servicios que quizá aún no están listos.
      window.location.reload();
    } catch {
      toast.notify({ tone: 'danger', title: t('apps.createFailed') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div class="flex flex-col gap-4">
      {puestas.length > 0 ? (
        <Card>
          <CardHeader>
            {/* Título y subtítulo apilados dentro del CardHeader, que es
              `justify-between` y está pensado para título + acción. Antes esto era
              un SectionHeader DENTRO del CardBody: trae su propio `bg-surface` y
              `border-b px-4`, así que sobre el `surface-raised` de la Card salía una
              banda de otro tono metida 16px, con el hairline muriendo a 16px de cada
              borde. El patrón bueno estaba al lado, en StoragePanel. */}
            <div class="min-w-0">
              <CardTitle as="h2">{t('apps.installed.title')}</CardTitle>
              <p class="text-text-muted mt-0.5 text-sm">{t('apps.installed.desc')}</p>
            </div>
          </CardHeader>
          <CardBody>
            <ul class="flex flex-col gap-2">
              {puestas.map((s) => (
                <li key={s.id ?? s.name} class="flex items-center justify-between gap-3 text-sm">
                  {/* Con URL pública, el nombre ES el enlace: llegar a un CMS que
                      acabas de levantar no puede exigir copiar un hostname. */}
                  {s.webUrl ? (
                    <a
                      href={s.webUrl}
                      target="_blank"
                      rel="noreferrer noopener"
                      class="truncate underline"
                    >
                      {s.name}
                    </a>
                  ) : (
                    <span class="truncate">{s.name}</span>
                  )}
                  <span class="flex shrink-0 items-center gap-2">
                    <Badge tone={s.status === 'running' ? 'success' : 'warning'}>
                      {statusLabel(t, s.status)}
                    </Badge>
                    {/* Retirar una app la borra del clúster con sus datos: hasta
                        ahora solo se podía instalar, así que una elegida por error
                        se quedaba puesta y contando para la cuota del plan. */}
                    {s.id ? (
                      <Button
                        variant="ghost"
                        class="text-danger-text text-sm"
                        disabled={deleting}
                        onClick={() => setPendingDelete(s)}
                        aria-label={t('apps.remove.aria', { name: s.name })}
                      >
                        {t('common.delete')}
                      </Button>
                    ) : null}
                  </span>
                </li>
              ))}
            </ul>
          </CardBody>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          {/* Título y subtítulo apilados dentro del CardHeader, que es
            `justify-between` y está pensado para título + acción. Antes esto era
            un SectionHeader DENTRO del CardBody: trae su propio `bg-surface` y
            `border-b px-4`, así que sobre el `surface-raised` de la Card salía una
            banda de otro tono metida 16px, con el hairline muriendo a 16px de cada
            borde. El patrón bueno estaba al lado, en StoragePanel. */}
          <div class="min-w-0">
            <CardTitle as="h2">{t('apps.title')}</CardTitle>
            <p class="text-text-muted mt-0.5 text-sm">{t('apps.desc')}</p>
          </div>
        </CardHeader>
        <CardBody>
          <InlineHelp tone="info" title={t('apps.why.title')}>
            <ul class="list-disc pl-5 text-sm">
              <li>{t('apps.why.curated')}</li>
              <li>{t('apps.why.licenses')}</li>
              <li>{t('apps.why.private')}</li>
            </ul>
          </InlineHelp>
        </CardBody>
      </Card>

      {!live || (apps.length === 0 && catalog.pending.length === 0) ? (
        <Card>
          <CardBody>
            <EmptyState
              illustration={<Icon name="grid" size={32} />}
              title={t('apps.empty.title')}
              description={t('apps.empty.desc')}
            />
          </CardBody>
        </Card>
      ) : null}

      {todas.length > 1 && categorias.length > 1 ? (
        <div class="mb-4 flex flex-wrap items-center gap-2">
          <Select
            aria-label={t('apps.filter')}
            value={categoria}
            class="w-64"
            onChange={(e) => setCategoria(e.currentTarget.value)}
          >
            <option value="">{t('apps.filter.all', { count: todas.length })}</option>
            {categorias.map((c) => (
              <option key={c} value={c}>
                {t(categoryKey[c])}
              </option>
            ))}
          </Select>
          {categoria ? (
            <Button variant="ghost" size="sm" onClick={() => setCategoria('')}>
              {t('apps.filter.clear')}
            </Button>
          ) : null}
        </div>
      ) : null}

      {apps.length > 0 ? (
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {apps.map((app: CatalogApp) => (
            <AppCard
              key={app.id}
              app={app}
              t={t}
              busy={busy}
              installed={yaPuestas.has(app.id)}
              hayPostgres={hayPostgres}
              selected={selected.includes(app.id)}
              onToggle={() => toggle(app.id)}
            />
          ))}
        </div>
      ) : null}

      {/* La barra de confirmación. Aparece solo con algo elegido: una barra fija
          vacía es ruido permanente por una acción que casi nunca se está haciendo. */}
      {selected.length > 0 ? (
        <Card>
          <CardHeader>
            <div class="min-w-0">
              <CardTitle as="h2">
                {t('apps.selected.title', { count: String(selected.length) })}
              </CardTitle>
              <p class="text-text-muted mt-0.5 text-sm">{t('apps.selected.desc')}</p>
            </div>
          </CardHeader>
          <CardBody class="flex flex-col gap-4">
            {/* Lo que hace especial a esto: qué se va a conectar con qué, dicho
                ANTES de crear nada y con el enlace a la documentación oficial que
                lo respalda. Sin esta lista, «se integran solas» es una promesa que
                el usuario tiene que creerse. */}
            {integraciones.length > 0 ? (
              <InlineHelp tone="success" title={t('apps.integrations.title')}>
                <ul class="flex flex-col gap-1 text-sm">
                  {integraciones.map((i) => (
                    <li key={`${i.from.id}-${i.to.id}`}>
                      {t('apps.integrations.line', { from: i.from.name, to: i.to.name })}{' '}
                      <code class="font-mono">{i.keys.join(', ')}</code>{' '}
                      <a href={i.docs} target="_blank" rel="noreferrer noopener" class="underline">
                        {t('apps.integrations.docs')}
                      </a>
                    </li>
                  ))}
                </ul>
              </InlineHelp>
            ) : null}

            <div class="flex flex-wrap items-center gap-3">
              <Button variant="primary" onClick={() => void createSelected()} disabled={busy}>
                {busy
                  ? t('apps.creating')
                  : t('apps.createSelected', { count: String(selected.length) })}
              </Button>
              <Button variant="secondary" onClick={() => setSelected([])} disabled={busy}>
                {t('apps.clearSelection')}
              </Button>
            </div>
          </CardBody>
        </Card>
      ) : null}

      {/* Lo que viene. Enseñarlo es honesto; esconderlo haría que nadie echara de
          menos la imagen que falta por replicar. */}
      {catalog.pending.length > 0 ? (
        <Card>
          <CardBody>
            <p class="text-text-subtle flex flex-wrap items-center gap-2 text-sm">
              <Icon name="info" size={16} />
              {t('apps.pending', { list: catalog.pending.join(', ') })}
            </p>
          </CardBody>
        </Card>
      ) : null}

      {/* Confirmación: retirar una app se lleva sus datos con ella. */}
      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('apps.remove.title')}
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
          {t('apps.remove.body', { name: pendingDelete?.name ?? '' })}
        </p>
      </Modal>
    </div>
  );
}
