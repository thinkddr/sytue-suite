/**
 * ConnectionsPanel — credenciales con las que SYTUE se presenta ante cada
 * proveedor de terceros.
 *
 * QUÉ DESBLOQUEA. Con una app registrada por Sytue ante Google o Microsoft,
 * migrar el correo, el calendario y los ficheros de una empresa es UN CLIC: su
 * admin aprueba una pantalla de permisos y ya. Sin ella, cada organización tiene
 * que darse de alta como desarrollador ante el proveedor, declarar el
 * redirect_uri, esperar la revisión y pegar sus propias credenciales. Sigue
 * funcionando —y a veces es lo que un cliente quiere—, pero deja de ser algo que
 * se hace en una tarde.
 *
 * Esto configura la MITAD que es software. La otra mitad es trabajo de
 * operación: darse de alta ante cada proveedor, declarar los ámbitos y pasar su
 * revisión. Por eso cada fila enseña el redirect_uri que hay que registrar allí:
 * es el dato que más veces se pega mal y el que produce el error más opaco del
 * flujo («redirect_uri_mismatch», sin decir cuál esperaba).
 *
 * Los secretos son de ESCRITURA. Nunca vuelven del servidor: se ve si hay o no
 * hay. Un secreto que se puede leer de vuelta acaba en una captura de pantalla.
 */
import type { JSX } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  EmptyState,
  Input,
  Modal,
  Switch,
  Table,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import { guiaDe } from '../lib/guias.js';

/** Credencial registrada. `has_secret` y no el secreto: la API es write-only. */
export interface PlatformOAuthClient {
  template_key: string;
  client_id: string;
  has_secret: boolean;
  updated_by: string;
  updated_at: string;
}

/** Entrada del catálogo de conectores. */
export interface ConnectorTemplate {
  key: string;
  name?: string;
  enabled?: boolean;
}

interface Props {
  locale: Locale;
  clients: PlatformOAuthClient[];
  templates: ConnectorTemplate[];
  /** Motivo por el que no hay datos (403 sin MFA, 503 sin token de operador…). */
  problema?: number | undefined;
  /** El CATÁLOGO falló (no las credenciales). Sin él no se puede añadir, y hay
   *  que decirlo: un botón desactivado no distingue «ya están todos» de «el
   *  servicio devolvió un 500». */
  catalogoRoto?: number | undefined;
  /** Esta superficie no gestiona las conexiones; se enlaza a la que sí. */
  /** Origen de la superficie de plataforma, para el enlace. */
  /** Origen público del servicio de integraciones, para componer el redirect_uri. */
  publicUrl: string;
}

async function call(path: string, method: string, body?: unknown): Promise<Response> {
  return fetch(`/api/platform-connections${path}`, {
    method,
    ...(body
      ? { headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) }
      : {}),
  });
}

/**
 * El redirect_uri que hay que registrar ante el proveedor. Se compone del origen
 * público del servicio y es SIEMPRE el mismo para todos: el proveedor distingue
 * por `state`, no por la URL de vuelta.
 */
function redirectUri(publicUrl: string): string {
  return `${publicUrl.replace(/\/$/, '')}/api/v1/oauth/callback`;
}

function Inner({
  clients,
  templates,
  problema,
  catalogoRoto,
  publicUrl,
  t,
}: Props & { t: T }): JSX.Element {
  const toast = useToast();
  const [editando, setEditando] = useState<string | null>(null);
  const [nuevo, setNuevo] = useState(false);
  const [templateList, setTemplateList] = useState<ConnectorTemplate[]>(templates);

  async function toggleTemplateEnabled(key: string, next: boolean): Promise<void> {
    setTemplateList((prev) => prev.map((tp) => (tp.key === key ? { ...tp, enabled: next } : tp)));
    try {
      const res = await call(`/templates/${key}/enabled`, 'PUT', { enabled: next });
      if (!res.ok) throw new Error(String(res.status));
      toast.notify({
        title: next ? 'Integración habilitada' : 'Integración deshabilitada',
        tone: 'success',
      });
    } catch {
      setTemplateList((prev) =>
        prev.map((tp) => (tp.key === key ? { ...tp, enabled: !next } : tp)),
      );
      toast.notify({ title: t('conn.toast.failed'), tone: 'danger' });
    }
  }

  // Proveedores del catálogo que AÚN NO tienen credencial: son los que se pueden
  // añadir. Sin esta resta, la lista de «añadir» repetiría los ya configurados y
  // el desplegable dejaría elegir uno que sobrescribe otro sin avisar.
  const registrados = useMemo(() => new Set(clients.map((c) => c.template_key)), [clients]);
  const disponibles = useMemo(
    () => templateList.filter((tp) => !registrados.has(tp.key)),
    [templateList, registrados],
  );

  if (problema !== undefined) {
    return (
      <Card>
        <CardBody>
          <EmptyState
            title={t(problema === 503 ? 'conn.error.unavailable' : 'conn.error.forbidden')}
            description={t(
              problema === 503 ? 'conn.error.unavailableBody' : 'conn.error.forbiddenBody',
            )}
          />
        </CardBody>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('conn.title')}</CardTitle>
          <div class="flex items-center gap-2">
            {catalogoRoto ? (
              <span class="text-danger text-xs">{t('conn.error.catalog')}</span>
            ) : null}
            <Button
              variant="primary"
              disabled={disponibles.length === 0}
              onClick={() => setNuevo(true)}
            >
              {t('conn.add')}
            </Button>
          </div>
        </CardHeader>
        <CardBody>
          <p class="text-text-muted mb-4 text-sm">{t('conn.intro')}</p>
          <div class="border-border bg-surface-raised mb-4 rounded-lg border p-3">
            <span class="text-text-muted text-xs font-semibold uppercase">
              {t('conn.redirectUri')}
            </span>
            <code class="mt-1 block text-sm break-all">{redirectUri(publicUrl)}</code>
            <p class="text-text-muted mt-1 text-xs">{t('conn.redirectUriHelp')}</p>
          </div>

          {clients.length === 0 ? (
            <EmptyState title={t('conn.empty')} description={t('conn.emptyBody')} />
          ) : (
            <Table<PlatformOAuthClient>
              rowKey={(c) => c.template_key}
              rows={clients}
              caption={t('conn.title')}
              columns={[
                {
                  key: 'provider',
                  header: t('conn.col.provider'),
                  cell: (c) => <strong>{c.template_key}</strong>,
                },
                {
                  key: 'clientId',
                  header: 'client_id',
                  cell: (c) => <code class="text-sm break-all">{c.client_id}</code>,
                },
                {
                  key: 'secret',
                  header: t('conn.col.secret'),
                  cell: (c) => (
                    <Badge tone={c.has_secret ? 'success' : 'neutral'}>
                      {t(c.has_secret ? 'conn.secret.set' : 'conn.secret.public')}
                    </Badge>
                  ),
                },
                {
                  key: 'updated',
                  header: t('conn.col.updated'),
                  cell: (c) => (
                    <span class="text-text-muted text-sm">
                      {c.updated_at?.slice(0, 10)}
                      {c.updated_by ? ` · ${c.updated_by}` : ''}
                    </span>
                  ),
                },
                {
                  key: 'actions',
                  header: '',
                  cell: (c) => (
                    <div class="flex gap-2">
                      <Button variant="ghost" onClick={() => setEditando(c.template_key)}>
                        {t('conn.edit')}
                      </Button>
                      <Button variant="danger" onClick={() => void borrar(c.template_key)}>
                        {t('conn.remove')}
                      </Button>
                    </div>
                  ),
                },
              ]}
            />
          )}
        </CardBody>
      </Card>

      <Card class="mt-6">
        <CardHeader>
          <CardTitle>{t('conn.templates.title')}</CardTitle>
        </CardHeader>
        <CardBody>
          <p class="text-text-muted mb-4 text-sm">{t('conn.templates.intro')}</p>
          {templateList.length === 0 ? (
            <EmptyState
              title={t('conn.templates.empty')}
              description={t('conn.templates.emptyBody')}
            />
          ) : (
            <Table<ConnectorTemplate>
              rowKey={(tp) => tp.key}
              rows={templateList}
              caption={t('conn.templates.tableCaption')}
              columns={[
                {
                  key: 'name',
                  header: t('conn.col.provider'),
                  cell: (tp) => (
                    <div>
                      <strong>{tp.name || tp.key}</strong>
                      {tp.name && tp.name !== tp.key ? (
                        <span class="text-text-muted ml-2 font-mono text-xs">{tp.key}</span>
                      ) : null}
                    </div>
                  ),
                },
                {
                  key: 'status',
                  header: 'Estado en Suite',
                  cell: (tp) => (
                    <Badge tone={tp.enabled !== false ? 'success' : 'neutral'}>
                      {tp.enabled !== false ? 'Habilitada' : 'Deshabilitada'}
                    </Badge>
                  ),
                },
                {
                  key: 'toggle',
                  header: 'Disponible para todos',
                  cell: (tp) => (
                    <Switch
                      label={tp.enabled !== false ? 'Habilitada' : 'Deshabilitada'}
                      checked={tp.enabled !== false}
                      onChange={(e) =>
                        void toggleTemplateEnabled(tp.key, (e.target as HTMLInputElement).checked)
                      }
                    />
                  ),
                },
              ]}
            />
          )}
        </CardBody>
      </Card>

      {(nuevo || editando) && (
        <EditorCredencial
          t={t}
          templateKey={editando}
          disponibles={disponibles}
          templates={templates}
          publicUrl={publicUrl}
          onClose={() => {
            setNuevo(false);
            setEditando(null);
          }}
          onSaved={() => window.location.reload()}
          onError={() => toast.notify({ title: t('conn.toast.failed'), tone: 'danger' })}
        />
      )}
    </>
  );

  async function borrar(key: string): Promise<void> {
    const res = await call(`/oauth-clients/${key}`, 'DELETE');
    if (res.ok) {
      toast.notify({ title: t('conn.toast.removed'), tone: 'success' });
      window.location.reload();
    } else {
      toast.notify({ title: t('conn.toast.failed'), tone: 'danger' });
    }
  }
}

interface EditorProps {
  t: T;
  /** Clave a editar, o null si se está añadiendo una nueva. */
  templateKey: string | null;
  disponibles: ConnectorTemplate[];
  /** Catálogo completo: al editar, el proveedor ya no está en `disponibles` y aun
   *  así hay que poder decir su nombre y su guía. */
  templates: ConnectorTemplate[];
  publicUrl: string;
  onClose: () => void;
  onSaved: () => void;
  onError: () => void;
}

/**
 * Los cuatro pasos para conseguir el par de credenciales, con el enlace directo a
 * la consola del proveedor elegido y la URL de vuelta lista para copiar.
 *
 * Van DENTRO del diálogo y no en la documentación porque el momento de saberlo es
 * este: quien abre esto no tiene las credenciales todavía. Y la URL de vuelta se
 * repite aquí aunque esté en la tarjeta de detrás —tapada por el propio diálogo—
 * porque pegarla mal es el fallo más común y el que peor se explica luego:
 * el proveedor responde «invalid_client» o «redirect_uri_mismatch» sin decir cuál
 * esperaba.
 */
function Pasos({
  t,
  proveedor,
  nombre,
  publicUrl,
}: {
  t: T;
  proveedor: string;
  nombre: string;
  publicUrl: string;
}): JSX.Element {
  const guia = guiaDe(proveedor);
  const uri = redirectUri(publicUrl);
  const [copiado, setCopiado] = useState(false);

  return (
    <ol class="border-border bg-surface-raised flex list-decimal flex-col gap-3 rounded-lg border py-3 pr-3 pl-8 text-sm">
      <li>
        {t('conn.guide.step1').replace('{provider}', nombre)}
        {guia ? (
          <>
            {' '}
            <a
              class="text-accent underline"
              href={guia.consola}
              target="_blank"
              rel="noreferrer noopener"
            >
              {t('conn.guide.open')} ↗
            </a>
            <span class="text-text-muted mt-1 block text-xs">
              <code>{guia.donde}</code>
              {guia.tipo ? ` · ${t('conn.guide.type')}: ` : ''}
              {guia.tipo ? <code>{guia.tipo}</code> : null}
            </span>
          </>
        ) : null}
      </li>
      <li>
        {t('conn.guide.step2')}
        <span class="mt-1 flex items-center gap-2">
          <code class="bg-surface border-border flex-1 rounded border px-2 py-1 text-xs break-all">
            {uri}
          </code>
          <Button
            variant="ghost"
            onClick={() => {
              void navigator.clipboard?.writeText(uri).then(() => setCopiado(true));
            }}
          >
            {t(copiado ? 'conn.guide.copied' : 'conn.guide.copy')}
          </Button>
        </span>
      </li>
      <li>{t('conn.guide.step3')}</li>
      <li>{t('conn.guide.step4')}</li>
    </ol>
  );
}

function EditorCredencial({
  t,
  templateKey,
  disponibles,
  templates,
  publicUrl,
  onClose,
  onSaved,
  onError,
}: EditorProps): JSX.Element {
  const editando = templateKey !== null;
  const [key, setKey] = useState(templateKey ?? disponibles[0]?.key ?? '');
  const nombre = templates.find((tp) => tp.key === key)?.name ?? key;
  const [clientId, setClientId] = useState('');
  const [secret, setSecret] = useState('');
  const [guardando, setGuardando] = useState(false);

  async function guardar(): Promise<void> {
    if (!key.trim() || !clientId.trim()) return;
    setGuardando(true);
    // keep_secret cuando se edita y no se escribe secret nuevo: conserva el
    // guardado. Varios proveedores enseñan el secret UNA vez, así que corregir
    // una errata del client_id no puede obligar a rotarlo.
    const res = await call(`/oauth-clients/${key.trim()}`, 'PUT', {
      client_id: clientId.trim(),
      client_secret: secret,
      keep_secret: editando && secret === '',
    });
    setGuardando(false);
    if (res.ok) onSaved();
    else onError();
  }

  return (
    <Modal open onClose={onClose} title={t(editando ? 'conn.editTitle' : 'conn.addTitle')}>
      <div class="flex flex-col gap-4">
        {editando ? (
          <p class="text-sm">
            <strong>{key}</strong>
          </p>
        ) : (
          <label class="flex flex-col gap-1 text-sm">
            {t('conn.field.provider')}
            <select
              class="border-border bg-surface rounded-md border px-3 py-2"
              value={key}
              onChange={(e) => setKey((e.target as HTMLSelectElement).value)}
            >
              {disponibles.map((tp) => (
                <option key={tp.key} value={tp.key}>
                  {tp.name ?? tp.key}
                </option>
              ))}
            </select>
          </label>
        )}
        <Pasos t={t} proveedor={key} nombre={nombre} publicUrl={publicUrl} />
        <label class="flex flex-col gap-1 text-sm">
          client_id
          <Input
            value={clientId}
            required
            onInput={(e) => setClientId((e.target as HTMLInputElement).value)}
          />
        </label>
        <label class="flex flex-col gap-1 text-sm">
          client_secret
          <Input
            type="password"
            value={secret}
            autocomplete="off"
            onInput={(e) => setSecret((e.target as HTMLInputElement).value)}
          />
        </label>
        <p class="text-text-muted text-xs">
          {t(editando ? 'conn.field.secretKeepHelp' : 'conn.field.secretHelp')}
        </p>
        {/* El diagnóstico de los dos errores que devuelve el proveedor, dicho
            aquí y no en un manual: los dos significan que algo de esta pantalla
            no coincide con lo de allí, y ninguno de los dos lo dice. */}
        <p class="text-text-muted text-xs">{t('conn.guide.troubleshoot')}</p>
        <div class="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            disabled={guardando || !key.trim() || !clientId.trim()}
            onClick={() => void guardar()}
          >
            {t('common.save')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/**
 * El ToastProvider va aquí y no en la página: la isla se monta sola y necesita su
 * propio contexto de avisos, igual que PlatformPanel. El idioma llega por props
 * porque dentro de una isla no se detecta —lo negocia el servidor con las
 * cabeceras de la petición—.
 */
export default function ConnectionsPanel(props: Props): JSX.Element {
  const t = useT(props.locale);
  return (
    <ToastProvider>
      <Inner {...props} t={t} />
    </ToastProvider>
  );
}
