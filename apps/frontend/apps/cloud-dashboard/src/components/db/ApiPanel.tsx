/**
 * ApiPanel — API auto-generada y credenciales de Sytue DB Studio. Muestra el
 * endpoint REST automático (PostgREST), claves (enmascaradas) y la cadena de
 * conexión Postgres, con ejemplos de uso (cURL / JS). Estilo Supabase con
 * diseño Sytue. Las claves reales se gestionan en Secrets — aquí solo previews.
 * docs/CONVENTIONS.md §10, §14.
 */
import { useState } from 'preact/hooks';
import { Card, CardBody, CardHeader, CardTitle, Badge, Button, useToast } from '@sytue/ui';
import type { DbApiInfo, DbTable } from '../../lib/types.js';
import type { T } from '../../i18n/index.js';
import { r } from '../../lib/rutas.js';
import { CopyField } from '../CopyField.js';

export default function ApiPanel({
  project,
  api,
  tables,
  t,
}: {
  project: string;
  api: DbApiInfo;
  tables: DbTable[];
  /** `t` ya atada al locale de la página (llega por props desde DbStudio). */
  t: T;
}) {
  // `tbl`, no `t`: `t` es la función de traducción y ensombrecerla en este filtro
  // es exactamente el fallo que deja un panel entero sin traducir sin avisar.
  const exposed = tables.filter((tbl) => tbl.apiExposed);
  const [example] = useState(exposed[0]?.name ?? 'clientes');
  const secretsHref = r(`/projects/${project}/env`);

  const curl = `curl '${api.restUrl}/${example}?select=*&limit=10' \\\n  -H "apikey: <SYTUE_DB_ANON_KEY>" \\\n  -H "Authorization: Bearer <SYTUE_DB_ANON_KEY>"`;
  // El ejemplo NO lleva URL ni clave a propósito: desde una app desplegada en
  // Sytue DevX no hacen falta —el control-plane ya las inyecta al desplegar—, y
  // enseñar aquí un `createClient(url, key)` empujaba a copiarlas a un fichero,
  // que es justo como acaban en un commit. Ver packages/cloud/README.md.
  const js = `import { sytue } from '@sytue/cloud';\n\nconst filas = await sytue.table('${example}').newest().limit(10).all();`;

  return (
    <div class="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('db.api.title')}</CardTitle>
          <Badge tone="accent" class="tabular-nums">
            {t('db.api.exposed', { count: exposed.length })}
          </Badge>
        </CardHeader>
        <CardBody class="flex flex-col gap-4">
          <p class="text-text-muted max-w-2xl text-base">{t('db.api.desc')}</p>
          <CopyField t={t} label={t('db.api.restUrl')} value={api.restUrl} />
          <div class="grid gap-4 sm:grid-cols-2">
            <CopyField
              t={t}
              label={t('db.api.anonKey')}
              value={api.anonKeyPreview}
              secret
              manageHref={secretsHref}
            />
            <CopyField
              t={t}
              label={t('db.api.serviceKey')}
              value={api.serviceKeyPreview}
              secret
              manageHref={secretsHref}
            />
          </div>
          <ConnectionString project={project} masked={api.connectionString} t={t} />
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('db.api.examples')}</CardTitle>
        </CardHeader>
        <CardBody class="flex flex-col gap-4">
          <div>
            <p class="text-text mb-1 text-sm font-semibold">cURL</p>
            <pre class="overflow-x-auto rounded-md bg-[var(--sytue-neutral-950)] p-3 font-mono text-sm leading-relaxed text-white/90">
              <code>{curl}</code>
            </pre>
          </div>
          <div>
            <p class="text-text mb-1 text-sm font-semibold">JavaScript</p>
            <pre class="overflow-x-auto rounded-md bg-[var(--sytue-neutral-950)] p-3 font-mono text-sm leading-relaxed text-white/90">
              <code>{js}</code>
            </pre>
          </div>
          <p class="text-text-muted text-sm">{t('db.api.secretsNote')}</p>
        </CardBody>
      </Card>
    </div>
  );
}

/**
 * La cadena de conexión, enmascarada hasta que se pide verla.
 *
 * Antes solo existía la máscara, y la contraseña únicamente dentro del clúster:
 * la base del cliente no se podía abrir con `psql`, ni con un cliente gráfico, ni
 * volcar con `pg_dump`. Son SUS datos, así que puede verlos; lo que se controla
 * es quién (admin/owner, solo humanos) y con qué rastro (queda en la auditoría).
 *
 * Se pide al pulsar y no al cargar la pantalla: así la contraseña no viaja —ni
 * acaba en una captura— cada vez que alguien entra a mirar sus tablas.
 */
function ConnectionString({ project, masked, t }: { project: string; masked: string; t: T }) {
  const toast = useToast();
  const [dsn, setDsn] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function reveal() {
    setBusy(true);
    try {
      const res = await fetch(`/api/db/credentials?project=${encodeURIComponent(project)}`);
      if (!res.ok) throw new Error(`bff_${res.status}`);
      const data = (await res.json()) as { connection_string: string };
      setDsn(data.connection_string);
    } catch (err) {
      const status = /bff_(\d+)/.exec(err instanceof Error ? err.message : '')?.[1];
      toast.notify({
        tone: 'danger',
        title: status === '403' ? t('db.api.reveal.forbidden') : t('db.api.reveal.fail'),
      });
    } finally {
      setBusy(false);
    }
  }

  // Revelada: es un valor real, así que se copia como cualquier otro campo (y ya
  // no se ofrece «gestionar en Secretos», que aquí no viene a cuento).
  if (dsn) {
    return <CopyField t={t} label={t('db.api.connectionString')} value={dsn} />;
  }
  return (
    <div class="flex flex-col gap-1">
      <span class="text-text text-sm font-semibold">{t('db.api.connectionString')}</span>
      <div class="flex items-center gap-2">
        <code class="border-border bg-surface-sunken text-text-muted min-w-0 flex-1 truncate rounded-md border px-3 py-2 font-mono text-sm">
          {masked}
        </code>
        <Badge tone="warning">{t('copy.secret')}</Badge>
        <Button
          variant="secondary"
          size="sm"
          class="shrink-0"
          loading={busy}
          onClick={() => void reveal()}
        >
          {t('db.api.reveal')}
        </Button>
      </div>
    </div>
  );
}
