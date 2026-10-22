/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Obtener datos externos» (conector estilo Power Query para Sheets) — ASISTENTE
 * corto en vez de un muro de formulario: de dónde vienen los datos (una tarjeta por
 * origen, incluidos los ya conectados) → datos de acceso (solo si el origen es
 * nuevo) → qué traer y PROBARLO de verdad → resumen de lo que se va a insertar →
 * listo, con qué hacer a partir de ahora.
 *
 * Toda la lógica de datos (construir cuerpos, sanear, mapear, traducir errores a
 * acciones) vive en ../dataquery (pura y testeada); aquí solo hay estado de UI y
 * llamadas al BFF (GET/POST/PATCH /api/connections, POST /api/dataquery,
 * GET/POST/PATCH/DELETE /api/connection-queries). Las
 * CREDENCIALES viajan solo en el POST/PATCH de la conexión (navegador→backend por
 * HTTPS), se cifran at-rest y NUNCA vuelven al cliente ni se guardan en el
 * documento: por eso la pantalla lo dice y por eso renovarlas es un PATCH, no
 * rehacer la conexión. Materializar despacha el comando `extdata` al lienzo.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useT } from '../context';
import type { DocsT } from '../../i18n/index';
import type { DocsKey } from '../../i18n';
import type { ExtProvenance } from '../types';
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  DataFlow,
  FieldGroup,
  FieldHint,
  Input,
  InlineHelp,
  Modal,
  ReviewSummary,
  Select,
  Spinner,
  Textarea,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import { EditorIcon } from '../icons';
import { dispatchSheetCommand, readSheetState } from '../sheets/SheetCanvas';
import { parseRangeRef } from '../sheets/format';
import { TABLE_STYLES } from '../sheets/table';
import {
  buildConnectionBody,
  buildExecuteBody,
  buildQuerySpec,
  errorMessageFrom,
  gatewayQueryFields,
  parseConnection,
  parseConnections,
  parseExecuteResult,
  parseSavedQueries,
  queryErrorAdvice,
  querySpecToForm,
  resultToGrid,
  SSL_MODES,
  type ConnKind,
  type ConnectionForm,
  type DataConnection,
  type ExecuteResult,
  type GatewayConnConfig,
  type GatewayInstance,
  type PgConnConfig,
  type QueryAdvice,
  type QueryForm,
  type RestAuthType,
  type RestConnConfig,
  type SavedQuery,
} from '../dataquery';

const JSON_HEADERS = { 'content-type': 'application/json' } as const;

/** Mensaje de error legible (siempre una cadena, para las descripciones de toast). */
const errText = (e: unknown, t: DocsT): string =>
  e instanceof Error ? e.message : t('extdata.error.unexpected');

/** Máximo de filas mostradas en la previsualización (la hoja solo tiene 100). */
const PREVIEW_ROWS = 50;

const EMPTY_CONN_FORM: ConnectionForm = {
  name: '',
  kind: 'rest',
  instanceId: '',
  baseUrl: '',
  authType: 'none',
  apiKeyIn: 'header',
  apiKeyName: '',
  username: '',
  tokenUrl: '',
  clientId: '',
  scope: '',
  audience: '',
  host: '',
  port: '',
  database: '',
  user: '',
  sslmode: 'prefer',
  secret: '',
};

const EMPTY_QUERY_FORM: QueryForm = {
  kind: 'rest',
  method: 'GET',
  path: '',
  params: [],
  rootPath: '',
  sql: '',
  dataset: '',
  gatewayQuery: '',
};

/** Formas de acreditarse ante una API, contadas por lo que implican para el usuario. */
const AUTH_OPTIONS: {
  value: RestAuthType;
  labelKey: DocsKey | null;
  label?: string;
  hintKey: DocsKey;
}[] = [
  {
    value: 'oauth2_client_credentials',
    labelKey: null, // nombre del estándar: igual en los cinco idiomas
    label: 'OAuth2 (client credentials)',
    hintKey: 'extdata.auth.oauth2.hint',
  },
  {
    value: 'api_key',
    labelKey: 'extdata.auth.apiKey',
    hintKey: 'extdata.auth.apiKey.hint',
  },
  {
    value: 'bearer',
    labelKey: 'extdata.auth.bearer',
    hintKey: 'extdata.auth.bearer.hint',
  },
  {
    value: 'basic',
    labelKey: 'extdata.auth.basic',
    hintKey: 'extdata.auth.basic.hint',
  },
  { value: 'none', labelKey: 'extdata.auth.none', hintKey: 'extdata.auth.none.hint' },
];

/** Id de la instancia (app conectada) de una conexión por pasarela; '' si no lo es. */
function instanceIdOf(c: DataConnection | null): string {
  if (!c || c.kind !== 'gateway') return '';
  const id = (c.config as Partial<GatewayConnConfig>).instance_id;
  return typeof id === 'string' ? id : '';
}

/** Descripción de una conexión guardada, en lenguaje de humano. */
function describeConnection(c: DataConnection, instances: GatewayInstance[], t: DocsT): string {
  if (c.kind === 'gateway') {
    const inst = instances.find((i) => i.id === instanceIdOf(c));
    return inst ? `${inst.app} · ${inst.name}` : t('extdata.conn.gatewayGeneric');
  }
  if (c.kind === 'bases') return t('extdata.source.bases.desc');
  if (c.kind === 'rest')
    return (c.config as RestConnConfig).base_url || t('extdata.conn.restGeneric');
  const pg = c.config as PgConnConfig;
  return `${pg.user ?? ''}@${pg.host ?? ''}${pg.database ? `/${pg.database}` : ''}`;
}

export function ExternalDataModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}): JSX.Element | null {
  const toast = useToast();
  const t = useT();

  const [step, setStep] = useState(0);
  const [conns, setConns] = useState<DataConnection[]>([]);
  const [loadingConns, setLoadingConns] = useState(false);
  const [connError, setConnError] = useState<string | null>(null);
  /** Apps YA CONECTADAS de la organización que pueden servir datos (via pasarela). */
  const [instances, setInstances] = useState<GatewayInstance[]>([]);

  /** Elección del paso 1: id de una conexión guardada, o 'new:rest' / 'new:postgres' / 'new:gateway'. */
  const [choice, setChoice] = useState('');
  /** Conexión REALMENTE en uso (la elegida, o la recién creada). */
  const [selectedId, setSelectedId] = useState('');

  const [connForm, setConnForm] = useState<ConnectionForm>(EMPTY_CONN_FORM);
  const [savingConn, setSavingConn] = useState(false);

  const [query, setQuery] = useState<QueryForm>(EMPTY_QUERY_FORM);
  const [preview, setPreview] = useState<ExecuteResult | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [advice, setAdvice] = useState<QueryAdvice | null>(null);

  /** Renovación de la credencial in situ (PATCH): '' = el campo está oculto. */
  const [newSecret, setNewSecret] = useState<string | null>(null);
  const [savingSecret, setSavingSecret] = useState(false);

  /** Consultas GUARDADAS del origen en uso. Viven en el backend (no en el libro),
   *  así que se reutilizan en cualquier hoja de la organización. */
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>([]);
  /** Cuál de ellas está cargada en el formulario. '' = consulta nueva. */
  const [savedId, setSavedId] = useState('');
  const [queryName, setQueryName] = useState('');
  const [savingQuery, setSavingQuery] = useState(false);

  const [target, setTarget] = useState('A1');
  const [styleId, setStyleId] = useState(TABLE_STYLES[0]!.id);
  const [inserted, setInserted] = useState(false);

  const creatingNew = choice.startsWith('new:');
  const newKind: ConnKind = kindOfChoice(choice);
  const selected = conns.find((c) => c.id === selectedId) ?? null;
  const kind: ConnKind = selected?.kind ?? newKind;
  /** Driver de la app conectada en uso: decide qué pide la consulta. '' si no aplica. */
  const driver =
    kind === 'gateway'
      ? (instances.find((i) => i.id === (instanceIdOf(selected) || connForm.instanceId))?.driver ??
        '')
      : '';

  /** Lee las consultas guardadas del origen. Degrada a lista vacía: no poder
   *  listarlas no debe impedir escribir una consulta nueva a mano. */
  const loadQueries = useCallback(async (connId: string): Promise<SavedQuery[]> => {
    try {
      const res = await fetch(`/api/connection-queries?connection=${encodeURIComponent(connId)}`);
      const data: unknown = await res.json().catch(() => null);
      const list = res.ok ? parseSavedQueries(data) : [];
      setSavedQueries(list);
      return list;
    } catch {
      setSavedQueries([]);
      return [];
    }
  }, []);

  /** Carga una consulta guardada en el formulario (o lo vacía para una nueva). */
  const pickSavedQuery = (id: string): void => {
    const q = savedQueries.find((s) => s.id === id) ?? null;
    setSavedId(q?.id ?? '');
    setQueryName(q?.name ?? '');
    setQuery(q ? querySpecToForm(q.spec, kind, driver) : { ...EMPTY_QUERY_FORM, kind });
    setPreview(null);
    setAdvice(null);
  };

  /** Guarda la consulta probada: POST si es nueva, PATCH si se editó una guardada
   *  (así «guardar» no crea un duplicado cada vez que se retoca). Nunca viajan
   *  credenciales: la spec no las lleva y la conexión ya las tiene cifradas. */
  const saveQuery = async (): Promise<void> => {
    const spec = buildQuerySpec(query, driver);
    const name = queryName.trim();
    if (!spec || name === '' || selectedId === '') return;
    setSavingQuery(true);
    try {
      const conn = encodeURIComponent(selectedId);
      const nueva = savedId === '';
      const res = await fetch(
        nueva
          ? `/api/connection-queries?connection=${conn}`
          : `/api/connection-queries?connection=${conn}&query=${encodeURIComponent(savedId)}`,
        {
          method: nueva ? 'POST' : 'PATCH',
          headers: JSON_HEADERS,
          body: JSON.stringify({ name, spec }),
        },
      );
      if (!res.ok) {
        const data: unknown = await res.json().catch(() => null);
        throw new Error(errorMessageFrom(data, String(res.status)));
      }
      // Se recarga y se deja seleccionada: volver a pulsar «Guardar» actualiza
      // la misma consulta en vez de crear una segunda con el mismo nombre.
      const lista = await loadQueries(selectedId);
      setSavedId(lista.find((q) => q.name === name)?.id ?? savedId);
      toast.notify({ tone: 'success', title: t('extdata.query.savedOk') });
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('extdata.query.saveError'),
        description: errText(e, t),
      });
    } finally {
      setSavingQuery(false);
    }
  };

  /** Borra la consulta guardada en uso. La consulta del formulario se queda: se
   *  retira del catálogo del origen, no de la pantalla. */
  const deleteSavedQuery = async (): Promise<void> => {
    if (savedId === '' || selectedId === '') return;
    setSavingQuery(true);
    try {
      const res = await fetch(
        `/api/connection-queries?connection=${encodeURIComponent(selectedId)}&query=${encodeURIComponent(savedId)}`,
        { method: 'DELETE' },
      );
      if (!res.ok) {
        const data: unknown = await res.json().catch(() => null);
        throw new Error(errorMessageFrom(data, String(res.status)));
      }
      setSavedId('');
      await loadQueries(selectedId);
      toast.notify({ tone: 'success', title: t('extdata.query.deletedOk') });
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('extdata.query.deleteError'),
        description: errText(e, t),
      });
    } finally {
      setSavingQuery(false);
    }
  };

  const loadConnections = useCallback(async (): Promise<void> => {
    setLoadingConns(true);
    setConnError(null);
    try {
      const res = await fetch('/api/connections');
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) throw new Error(errorMessageFrom(data, String(res.status)));
      setConns(parseConnections(data));
    } catch (e) {
      setConnError(e instanceof Error ? e.message : t('extdata.error.loadSources'));
    } finally {
      setLoadingConns(false);
    }
  }, [t]);

  /** Elegir origen: fija la conexión (o la marca como «nueva») y limpia la consulta. */
  const chooseSource = (value: string): void => {
    setChoice(value);
    const conn = conns.find((c) => c.id === value) ?? null;
    const newKindOf = kindOfChoice(value);
    setSelectedId(conn?.id ?? '');
    setQuery({ ...EMPTY_QUERY_FORM, kind: conn?.kind ?? newKindOf });
    if (!conn) {
      setConnForm({
        ...EMPTY_CONN_FORM,
        kind: newKindOf,
        // PostgreSQL siempre pide contraseña; en REST, OAuth2 es el camino preferido.
        // La pasarela no pide ninguna: la credencial la guarda la organización.
        authType: newKindOf === 'rest' ? 'oauth2_client_credentials' : 'none',
      });
    }
    setPreview(null);
    setAdvice(null);
    setNewSecret(null);
  };

  /** Apps conectadas elegibles. Degrada a lista vacía: el paso lo dice en llano. */
  const loadInstances = useCallback(async (): Promise<void> => {
    try {
      const res = await fetch('/api/gateway-instances');
      const data = (await res.json().catch(() => null)) as { instances?: GatewayInstance[] } | null;
      setInstances(res.ok && Array.isArray(data?.instances) ? data.instances : []);
    } catch {
      setInstances([]);
    }
  }, []);

  // Al abrir: carga las conexiones y toma la celda ancla actual como destino.
  useEffect(() => {
    if (!open) return;
    setStep(0);
    setInserted(false);
    setPreview(null);
    setAdvice(null);
    setNewSecret(null);
    void loadConnections();
    void loadInstances();
    const sel = parseRangeRef(readSheetState()?.selection ?? '');
    if (sel) setTarget(colRow(sel.c1, sel.r1));
  }, [open, loadConnections, loadInstances]);

  // Las consultas guardadas cuelgan del ORIGEN, así que se recargan al cambiarlo
  // (un solo efecto cubre elegir uno guardado y acabar de crear uno nuevo).
  useEffect(() => {
    setSavedId('');
    setQueryName('');
    if (!open || selectedId === '') {
      setSavedQueries([]);
      return;
    }
    void loadQueries(selectedId);
  }, [open, selectedId, loadQueries]);

  /** Guarda la conexión nueva y sigue con ella seleccionada. */
  const saveConnection = async (): Promise<void> => {
    setSavingConn(true);
    try {
      const res = await fetch('/api/connections', {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(buildConnectionBody(connForm)),
      });
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) throw new Error(errorMessageFrom(data, String(res.status)));
      const created = parseConnection(data);
      if (!created) throw new Error(t('extdata.error.noCreated'));
      await loadConnections();
      // Ya es una conexión guardada como cualquier otra: el paso de acceso
      // desaparece del asistente (volver atrás y pulsar «Guardar» otra vez
      // habría creado un duplicado) y «Datos» pasa a ser el paso 2 de 4.
      setChoice(created.id);
      setSelectedId(created.id);
      setQuery({ ...EMPTY_QUERY_FORM, kind: created.kind });
      setStep(1);
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('extdata.error.saveSource'),
        description: errText(e, t),
      });
    } finally {
      setSavingConn(false);
    }
  };

  /** Sustituye SOLO la credencial de la conexión en uso (PATCH; la config no cambia). */
  const renewSecret = async (): Promise<void> => {
    if (!selected || !newSecret) return;
    setSavingSecret(true);
    try {
      const res = await fetch(`/api/connections?id=${encodeURIComponent(selected.id)}`, {
        method: 'PATCH',
        headers: JSON_HEADERS,
        body: JSON.stringify({
          name: selected.name,
          kind: selected.kind,
          config: selected.config,
          credentials: { secret: newSecret },
        }),
      });
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) throw new Error(errorMessageFrom(data, String(res.status)));
      setNewSecret(null);
      await loadConnections();
      toast.notify({ tone: 'success', title: 'Credencial actualizada' });
      await runPreview();
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('extdata.secret.updateError'),
        description: errText(e, t),
      });
    } finally {
      setSavingSecret(false);
    }
  };

  const deleteConnection = async (id: string): Promise<void> => {
    try {
      const res = await fetch(`/api/connections?id=${encodeURIComponent(id)}`, {
        method: 'DELETE',
      });
      if (!res.ok && res.status !== 204) {
        const data: unknown = await res.json().catch(() => null);
        throw new Error(errorMessageFrom(data, String(res.status)));
      }
      if (choice === id) chooseSource('');
      await loadConnections();
      toast.notify({ tone: 'success', title: 'Origen eliminado' });
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('extdata.source.deleteError'),
        description: errText(e, t),
      });
    }
  };

  /** Ejecuta la consulta contra la conexión en uso y deja el resultado o el consejo. */
  const runPreview = async (): Promise<void> => {
    const spec = buildQuerySpec(query, driver);
    if (!spec) {
      setAdvice({
        title: t('extdata.advice.incomplete.title'),
        action:
          kind === 'postgres'
            ? 'Escribe una sentencia SELECT con las columnas que quieres traer.'
            : 'Rellena los campos marcados de arriba y vuelve a probar.',
        fix: 'query',
      });
      return;
    }
    setPreviewing(true);
    setAdvice(null);
    try {
      const res = await fetch('/api/dataquery', {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(buildExecuteBody(selectedId, spec)),
      });
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) {
        setPreview(null);
        setAdvice(queryErrorAdvice(data, res.status, kind));
        return;
      }
      const result = parseExecuteResult(data);
      if (!result) {
        setPreview(null);
        setAdvice({
          title: t('extdata.advice.notTable.title'),
          action: t('extdata.adv.notTable.action'),
          fix: 'query',
        });
        return;
      }
      setPreview(result);
    } catch {
      setPreview(null);
      setAdvice({
        title: t('extdata.advice.offline.title'),
        action: t('extdata.adv.offline.action'),
        fix: 'retry',
      });
    } finally {
      setPreviewing(false);
    }
  };

  const materialize = (): void => {
    const spec = buildQuerySpec(query, driver);
    const rect = parseRangeRef(target.trim().toUpperCase());
    if (!spec || !preview || !rect || !selectedId) return;
    dispatchSheetCommand(
      'extdata',
      JSON.stringify({
        connectionId: selectedId,
        spec,
        anchor: colRow(rect.c1, rect.r1),
        columns: preview.columns,
        rows: preview.rows,
        provenance: preview.provenance, // Sytue Bases: se guarda con la tabla
        style: styleId,
      }),
    );
    setInserted(true);
    setStep((s) => s + 1);
  };

  const targetValid = parseRangeRef(target.trim().toUpperCase()) !== null;

  // ---- Pasos ----

  const stepSource: WizardStep = {
    id: 'source',
    title: t('extdata.step.source'),
    heading: t('extdata.source.heading'),
    description: t('extdata.source.intro'),
    canContinue: choice !== '',
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        {/* Las conexiones solo se consultan: la hoja no escribe en el origen. */}
        <DataFlow compact incoming={['records']} />
        {connError ? (
          <InlineHelp
            tone="danger"
            title={t('extdata.sourcesError')}
            action={
              <Button variant="secondary" size="sm" onClick={() => void loadConnections()}>
                {t('editor.error.retry')}
              </Button>
            }
          >
            {connError}
          </InlineHelp>
        ) : null}

        {loadingConns ? (
          <p class="text-text-muted flex items-center gap-2 text-sm">
            <Spinner size={14} /> {t('extdata.source.searching')}
          </p>
        ) : null}

        {conns.length > 0 ? (
          <fieldset class="flex flex-col gap-2">
            <legend class="text-text mb-1 text-sm font-semibold">
              {t('extdata.source.existing')}
            </legend>
            {conns.map((c) => (
              <ChoiceCard
                key={c.id}
                name="ext-source"
                value={c.id}
                icon={SOURCE_ICON[c.kind]}
                title={c.name}
                description={describeConnection(c, instances, t)}
                footnote={
                  c.kind === 'gateway'
                    ? t('extdata.source.foot.gateway')
                    : c.has_credentials
                      ? t('extdata.source.foot.hasCred')
                      : t('extdata.source.foot.noCred')
                }
                selected={choice === c.id}
                onSelect={() => chooseSource(c.id)}
              />
            ))}
          </fieldset>
        ) : null}

        <fieldset class="flex flex-col gap-2">
          <legend class="text-text mb-1 text-sm font-semibold">{t('extdata.connectNew')}</legend>
          {/* Primero lo que NO exige credencial: una app que la organización ya ha
              conectado (la pasarela entra con su permiso; aquí solo se elige cuál). */}
          {instances.length > 0 ? (
            <ChoiceCard
              name="ext-source"
              value="new:gateway"
              icon="cloud"
              title={t('extdata.connectedApp')}
              description={t('extdata.new.gateway.description', {
                apps: `${instances
                  .slice(0, 3)
                  .map((i) => i.app)
                  .join(', ')}${instances.length > 3 ? '…' : ''}`,
              })}
              footnote={t('extdata.new.gateway.footnote')}
              selected={choice === 'new:gateway'}
              onSelect={() => chooseSource('new:gateway')}
            />
          ) : null}
          {/* Una base de la propia organización: de solo lectura y con un token que concede
              y revoca quien es propietario de la base (Sytue Bases, «Conexiones»). */}
          <ChoiceCard
            name="ext-source"
            value="new:bases"
            icon="database"
            title={t('extdata.bases')}
            description={t('extdata.source.bases.desc')}
            footnote={t('extdata.source.bases.note')}
            selected={choice === 'new:bases'}
            onSelect={() => chooseSource('new:bases')}
          />
          <ChoiceCard
            name="ext-source"
            value="new:rest"
            icon="globe"
            title={t('extdata.api')}
            description={t('extdata.source.api.desc')}
            footnote={t('extdata.source.api.note')}
            selected={choice === 'new:rest'}
            onSelect={() => chooseSource('new:rest')}
          />
          <ChoiceCard
            name="ext-source"
            value="new:postgres"
            icon="database"
            title={t('extdata.postgres')}
            description={t('extdata.source.pg.desc')}
            footnote={t('extdata.source.pg.note')}
            selected={choice === 'new:postgres'}
            onSelect={() => chooseSource('new:postgres')}
          />
        </fieldset>

        {selected ? (
          <div class="flex flex-wrap items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => void deleteConnection(selected.id)}>
              <EditorIcon name="trash" size="sm" />
              {t('extdata.source.delete', { name: selected.name })}
            </Button>
            <span class="sytue-meta">{t('extdata.source.deleteWarning')}</span>
          </div>
        ) : null}
      </div>
    ),
  };

  const ACCESS_HEADING: Record<ConnKind, string> = {
    rest: t('extdata.access.heading.rest'),
    postgres: t('extdata.access.heading.postgres'),
    gateway: t('extdata.access.heading.gateway'),
    bases: t('extdata.access.heading.bases'),
  };

  const stepAccess: WizardStep = {
    id: 'access',
    title: newKind === 'gateway' ? t('extdata.step.app') : t('extdata.step.access'),
    heading: ACCESS_HEADING[newKind],
    description:
      newKind === 'gateway'
        ? t('extdata.access.description.gateway')
        : newKind === 'bases'
          ? t('extdata.access.description.bases')
          : t('extdata.access.description.credentials'),
    nextLabel: t('extdata.access.next'),
    canContinue: connFormComplete(connForm),
    onNext: () => void saveConnection(),
    content:
      newKind === 'gateway' ? (
        <GatewayInstanceFields form={connForm} instances={instances} onChange={setConnForm} />
      ) : newKind === 'bases' ? (
        <BasesTokenFields form={connForm} onChange={setConnForm} />
      ) : (
        <ConnectionFields form={connForm} onChange={setConnForm} />
      ),
  };

  const stepQuery: WizardStep = {
    id: 'query',
    title: t('extdata.step.data'),
    heading: t('extdata.query.heading'),
    description: t('extdata.query.description'),
    canContinue: preview !== null,
    content: (
      <div class="flex flex-col gap-4">
        {/* Consultas ya guardadas de ESTE origen: se eligen para no reescribir la
            ruta, los parámetros y el «camino a los datos» cada vez. Solo aparece
            si hay alguna: en el origen recién conectado no estorba. */}
        {savedQueries.length > 0 ? (
          <FieldGroup label={t('extdata.query.saved')} htmlFor="ext-saved-query">
            <div class="flex flex-wrap items-center gap-2">
              <Select
                id="ext-saved-query"
                class="min-w-52 flex-1"
                value={savedId}
                onChange={(e) => pickSavedQuery(e.currentTarget.value)}
              >
                <option value="">{t('extdata.query.savedNew')}</option>
                {savedQueries.map((q) => (
                  <option key={q.id} value={q.id}>
                    {q.name}
                  </option>
                ))}
              </Select>
              {savedId !== '' ? (
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={savingQuery}
                  onClick={() => void deleteSavedQuery()}
                >
                  <EditorIcon name="trash" size="sm" />
                  {t('extdata.query.deleteSaved')}
                </Button>
              ) : null}
            </div>
            <FieldHint>{t('extdata.query.savedHint')}</FieldHint>
          </FieldGroup>
        ) : null}

        {kind === 'bases' ? (
          <p class="sytue-meta">{t('extdata.bases.queryNote')}</p>
        ) : kind === 'gateway' ? (
          <GatewayQueryFields
            form={query}
            driver={driver}
            onChange={(f) => {
              setQuery(f);
              setPreview(null);
            }}
          />
        ) : kind === 'rest' ? (
          <RestQueryFields
            form={query}
            baseUrl={selected ? ((selected.config as RestConnConfig).base_url ?? '') : ''}
            onChange={(f) => {
              setQuery(f);
              setPreview(null);
            }}
          />
        ) : (
          <PostgresQueryFields
            form={query}
            onChange={(f) => {
              setQuery(f);
              setPreview(null);
            }}
          />
        )}

        <div class="flex flex-wrap items-center gap-3">
          <Button
            variant={preview ? 'secondary' : 'primary'}
            disabled={previewing}
            onClick={() => void runPreview()}
          >
            {previewing ? <Spinner size={14} /> : <EditorIcon name="search" size="sm" />}
            {preview ? t('extdata.query.retest') : t('extdata.query.test')}
          </Button>
          {!preview && !advice ? <p class="sytue-meta">{t('extdata.query.mustTest')}</p> : null}
        </div>

        {/* Guardar la consulta que YA ha funcionado, para reutilizarla en otra
            hoja. Solo tras probarla: guardar una que nadie sabe si funciona sería
            sembrar el catálogo del origen de consultas rotas. */}
        {preview ? (
          <div class="flex flex-wrap items-end gap-2">
            <FieldGroup
              label={t('extdata.field.name')}
              htmlFor="ext-query-name"
              class="min-w-52 flex-1"
            >
              <Input
                id="ext-query-name"
                value={queryName}
                placeholder={t('extdata.query.namePlaceholder')}
                onInput={(e) => setQueryName(e.currentTarget.value)}
              />
              <FieldHint>{t('extdata.query.saveHint')}</FieldHint>
            </FieldGroup>
            <Button
              variant="secondary"
              disabled={queryName.trim() === '' || savingQuery}
              onClick={() => void saveQuery()}
            >
              {savingQuery ? <Spinner size={14} /> : null}
              {savedId === '' ? t('extdata.query.save') : t('extdata.query.update')}
            </Button>
          </div>
        ) : null}

        {/* El fallo se cuenta como una ACCIÓN, no como el error crudo del proveedor. */}
        {advice ? (
          <InlineHelp
            tone="danger"
            title={advice.title}
            action={
              advice.fix === 'connection' ? (
                <Button variant="secondary" size="sm" onClick={() => setStep(0)}>
                  {t('extdata.advice.pickOther')}
                </Button>
              ) : advice.fix === 'session' ? (
                <Button variant="secondary" size="sm" onClick={() => location.reload()}>
                  {t('extdata.advice.signIn')}
                </Button>
              ) : null
            }
          >
            <p>{advice.action}</p>
            {advice.fix === 'credential' && selected ? (
              newSecret === null ? (
                <Button variant="secondary" size="sm" class="mt-2" onClick={() => setNewSecret('')}>
                  {t('extdata.secret.reenter')}
                </Button>
              ) : (
                <div class="mt-2 flex flex-wrap items-end gap-2">
                  <FieldGroup
                    label={t('extdata.secret.new')}
                    htmlFor="ext-renew-secret"
                    class="min-w-52 flex-1"
                  >
                    <Input
                      id="ext-renew-secret"
                      type="password"
                      autocomplete="new-password"
                      value={newSecret}
                      onInput={(e) => setNewSecret(e.currentTarget.value)}
                    />
                    <FieldHint>{t('extdata.secret.hint')}</FieldHint>
                  </FieldGroup>
                  <Button
                    size="sm"
                    disabled={newSecret.trim() === '' || savingSecret}
                    onClick={() => void renewSecret()}
                  >
                    {savingSecret ? <Spinner size={14} /> : null}
                    {t('extdata.secret.saveRetest')}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setNewSecret(null)}>
                    {t('action.cancel')}
                  </Button>
                </div>
              )
            ) : null}
          </InlineHelp>
        ) : null}

        {preview ? (
          <div class="flex flex-col gap-2">
            <InlineHelp tone="success" title={t('extdata.works')}>
              {t('extdata.preview.okBody')}
            </InlineHelp>
            <PreviewTable result={preview} />
            {preview.provenance ? <Procedencia p={preview.provenance} /> : null}
          </div>
        ) : null}
      </div>
    ),
  };

  // Índice del paso «Datos»: hay un paso de acceso de por medio solo si el origen
  // se está conectando ahora. Lo usa «Editar» del resumen.
  const queryStepIndex = creatingNew ? 2 : 1;

  const gwFields = gatewayQueryFields(driver);

  const summary: ReviewItem[] = [
    {
      label: t('extdata.summary.source'),
      value: selected ? `${selected.name} — ${describeConnection(selected, instances, t)}` : '—',
      onEdit: () => setStep(0),
    },
    {
      label: t('extdata.summary.what'),
      value:
        kind === 'bases'
          ? preview?.provenance
            ? textoDeProcedencia(preview.provenance, t)
            : t('extdata.summary.what.bases')
          : kind === 'gateway'
            ? [
                gwFields.dataset && query.dataset.trim()
                  ? `${gwFields.dataset.label}: ${query.dataset.trim()}`
                  : '',
                gwFields.query && query.gatewayQuery.trim()
                  ? `${gwFields.query.label}: ${query.gatewayQuery.trim()}`
                  : '',
              ]
                .filter(Boolean)
                .join(' · ') || '—'
            : kind === 'postgres'
              ? query.sql.trim() || '—'
              : `${query.method} ${query.path.trim() || '/'}${query.params.length > 0 ? ` · ${t('extdata.paramCount', { count: query.params.length })}` : ''}`,
      mono: kind === 'postgres',
      onEdit: () => setStep(queryStepIndex),
    },
    {
      label: t('layout.size'),
      value: preview
        ? t('extdata.summary.sizeValue', {
            rows: t('extdata.rows', { count: preview.rows.length }),
            columns: t('extdata.columns', { count: preview.columns.length }),
          })
        : t('extdata.summary.empty'),
    },
    {
      label: t('extdata.summary.credentials'),
      value:
        kind === 'bases'
          ? t('extdata.summary.cred.bases')
          : kind === 'gateway'
            ? t('extdata.summary.cred.gateway')
            : selected?.has_credentials
              ? t('extdata.summary.cred.stored')
              : t('extdata.summary.cred.none'),
    },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('extdata.step.review'),
    heading: t('extdata.review.heading'),
    description: t('extdata.review.description'),
    nextLabel: t('extdata.review.next'),
    canContinue: targetValid && preview !== null,
    onNext: materialize,
    content: (
      <div class="flex flex-col gap-4">
        <ReviewSummary items={summary} />
        <div class="flex flex-wrap items-start gap-3">
          <FieldGroup
            label={t('extdata.review.target')}
            htmlFor="ext-target"
            {...(targetValid ? {} : { error: t('extdata.review.targetError') })}
            class="w-52"
          >
            <Input
              id="ext-target"
              value={target}
              invalid={!targetValid}
              onInput={(e) => setTarget(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup label={t('extdata.review.style')} htmlFor="ext-style" class="w-52">
            <Select
              id="ext-style"
              value={styleId}
              onChange={(e) => setStyleId(e.currentTarget.value)}
            >
              {TABLE_STYLES.map((s) => (
                <option key={s.id} value={s.id}>
                  {t(s.labelKey)}
                </option>
              ))}
            </Select>
          </FieldGroup>
        </div>
        <InlineHelp tone="warning" title={t('extdata.overwrites')}>
          {t('extdata.review.overwriteBody', {
            rows: t('extdata.rows', { count: preview ? preview.rows.length + 1 : 0 }),
            columns: t('extdata.columns', { count: preview?.columns.length ?? 0 }),
          })}
        </InlineHelp>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('extdata.step.done'),
    heading: t('extdata.done.heading'),
    done: true,
    content: (
      <div class="flex flex-col gap-4">
        <InlineHelp tone="success" title={t('extdata.savedWithTable')}>
          {t('extdata.done.savedBody')}
        </InlineHelp>
        <section class="border-border bg-surface-raised rounded-lg border p-4">
          {/* 16 px: encabeza una lista cuyos títulos van a 13 y cuyo texto va a 14, o sea
              que iba MÁS PEQUEÑO que lo que encabeza. Queda un escalón por debajo del
              título del modal (18). */}
          <h3 class="text-text text-md font-semibold">{t('extdata.done.nextHeading')}</h3>
          <ol class="mt-3 flex flex-col gap-3">
            <NextStep n={1} title={t('extdata.refreshAnytime')}>
              {t('extdata.done.step1.body')}
            </NextStep>
            <NextStep n={2} title={t('extdata.keepOnlyWhatYouWant')}>
              {t('extdata.done.step2.body')}
            </NextStep>
            <NextStep n={3} title={t('extdata.useLikeAnyTable')}>
              {t('extdata.done.step3.body')}
            </NextStep>
          </ol>
        </section>
        <div class="border-border-subtle flex items-center gap-2 border-t pt-4">
          <Button onClick={onClose}>{t('action.close')}</Button>
        </div>
      </div>
    ),
  };

  const steps: WizardStep[] = creatingNew
    ? [stepSource, stepAccess, stepQuery, stepReview, stepDone]
    : [stepSource, stepQuery, stepReview, stepDone];

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('extdata.bringOutside')}
      size="lg"
      dismissable={!inserted}
    >
      <Wizard
        steps={steps}
        current={Math.min(step, steps.length - 1)}
        onCurrentChange={setStep}
        busy={savingConn}
        {...(inserted ? {} : { onCancel: onClose })}
      />
    </Modal>
  );
}

/* ---------- Sub-bloques ---------- */

/** Un paso de «qué hacer ahora» de la pantalla final. */
function NextStep({
  n,
  title,
  children,
}: {
  n: number;
  title: string;
  children: ComponentChildren;
}): JSX.Element {
  return (
    <li class="flex gap-3">
      <span
        aria-hidden="true"
        class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold"
      >
        {n}
      </span>
      <div class="min-w-0">
        <p class="text-text text-sm font-medium">{title}</p>
        <p class="text-text-muted mt-0.5 text-sm leading-snug">{children}</p>
      </div>
    </li>
  );
}

/** Tipo de origen que representa una tarjeta «conectar uno nuevo». */
function kindOfChoice(choice: string): ConnKind {
  if (choice === 'new:postgres') return 'postgres';
  if (choice === 'new:gateway') return 'gateway';
  if (choice === 'new:bases') return 'bases';
  return 'rest';
}

/** Icono de cada tipo de origen en la lista de «ya conectados». */
const SOURCE_ICON: Record<ConnKind, 'database' | 'globe' | 'cloud'> = {
  postgres: 'database',
  rest: 'globe',
  gateway: 'cloud',
  bases: 'database',
};

/** ¿Están los datos mínimos para que el backend acepte la conexión? */
function connFormComplete(form: ConnectionForm): boolean {
  if (form.name.trim() === '') return false;
  if (form.kind === 'gateway') return form.instanceId !== '';
  if (form.kind === 'bases') return form.secret.trim().startsWith('sbc_');
  if (form.kind === 'postgres')
    return form.host.trim() !== '' && form.database.trim() !== '' && form.user.trim() !== '';
  if (form.baseUrl.trim() === '') return false;
  if (form.authType === 'api_key') return form.apiKeyName.trim() !== '' && form.secret !== '';
  if (form.authType === 'oauth2_client_credentials')
    return form.tokenUrl.trim() !== '' && form.clientId.trim() !== '' && form.secret.trim() !== '';
  if (form.authType === 'bearer' || form.authType === 'basic') return form.secret !== '';
  return true;
}

/** Etiqueta del secreto según cómo se acredite (no todos son «una contraseña»). */
function secretLabel(form: ConnectionForm, t: DocsT): string {
  if (form.kind === 'postgres') return t('extdata.secret.password');
  switch (form.authType) {
    case 'basic':
      return t('extdata.secret.password');
    case 'bearer':
      return t('extdata.secret.token');
    case 'oauth2_client_credentials':
      return 'Client secret'; // nombre del estándar: igual en los cinco idiomas
    default:
      return t('extdata.secret.apiKey');
  }
}

/** Dónde encontrar ese secreto y qué permiso hace falta. */
function secretHint(form: ConnectionForm, t: DocsT): string {
  if (form.kind === 'postgres') return t('extdata.secret.hint.postgres');
  switch (form.authType) {
    case 'basic':
      return t('extdata.secret.hint.basic');
    case 'bearer':
      return t('extdata.secret.hint.bearer');
    case 'oauth2_client_credentials':
      return t('extdata.secret.hint.oauth2');
    default:
      return t('extdata.secret.hint.apiKey');
  }
}

/** La procedencia en una frase: base · tabla (vista) — quién la concedió, cuándo y versión. */
function textoDeProcedencia(p: ExtProvenance, t: DocsT): string {
  const fecha = p.grantedAt ? new Date(p.grantedAt) : null;
  return t('extdata.bases.provenanceValue', {
    base: p.baseName,
    table: p.viewName
      ? `${p.tableName} (${t('extdata.bases.provenanceView', { view: p.viewName })})`
      : p.tableName,
    who: p.grantedBy || '—',
    date: fecha && !Number.isNaN(fecha.getTime()) ? fecha.toLocaleDateString() : '—',
    version: String(p.version),
  });
}

/** De dónde vienen los datos de una tabla de Sytue Bases, junto a su vista previa. */
function Procedencia({ p }: { p: ExtProvenance }): JSX.Element {
  const t = useT();
  return (
    <p class="sytue-meta">
      <strong>{t('extdata.bases.provenance')}:</strong> {textoDeProcedencia(p, t)}
    </p>
  );
}

/**
 * Conexión con Sytue Bases: el nombre con el que aparecerá el origen y el token de la
 * conexión, que crea quien es propietario de la base. Nada más: qué tabla o qué vista se
 * lee lo dice el propio token.
 */
function BasesTokenFields({
  form,
  onChange,
}: {
  form: ConnectionForm;
  onChange: (f: ConnectionForm) => void;
}): JSX.Element {
  const t = useT();
  return (
    <div class="flex max-w-xl flex-col gap-5">
      <FieldGroup label={t('extdata.field.name')} htmlFor="bf-name" required>
        <Input
          id="bf-name"
          value={form.name}
          placeholder={t('extdata.bases')}
          onInput={(e) => onChange({ ...form, name: e.currentTarget.value })}
        />
      </FieldGroup>
      <FieldGroup label={t('extdata.bases.token')} htmlFor="bf-token" required>
        <Input
          id="bf-token"
          type="password"
          autocomplete="off"
          value={form.secret}
          onInput={(e) => onChange({ ...form, secret: e.currentTarget.value })}
        />
        <FieldHint>{t('extdata.bases.tokenHint')}</FieldHint>
      </FieldGroup>
    </div>
  );
}

/**
 * Elección de la aplicación conectada (origen por pasarela): una tarjeta por
 * instancia, más el nombre con el que aparecerá en la lista de orígenes (se
 * rellena solo con el de la aplicación). Sin credenciales: las guarda la
 * organización en la conexión de la pasarela.
 */
function GatewayInstanceFields({
  form,
  instances,
  onChange,
}: {
  form: ConnectionForm;
  instances: GatewayInstance[];
  onChange: (f: ConnectionForm) => void;
}): JSX.Element {
  const t = useT();
  return (
    <div class="flex max-w-xl flex-col gap-5">
      <fieldset class="flex flex-col gap-2">
        <legend class="text-text mb-1 text-sm font-semibold">{t('extdata.connectedApps')}</legend>
        {instances.map((i) => (
          <ChoiceCard
            key={i.id}
            name="ext-instance"
            value={i.id}
            icon="cloud"
            title={i.name}
            description={i.app}
            selected={form.instanceId === i.id}
            onSelect={() =>
              onChange({
                ...form,
                instanceId: i.id,
                // El nombre del origen se propone solo; sigue siendo editable.
                name: form.name.trim() === '' ? i.name : form.name,
              })
            }
          />
        ))}
      </fieldset>

      <FieldGroup label={t('extdata.field.name')} htmlFor="gw-name" required>
        <Input
          id="gw-name"
          value={form.name}
          placeholder={t('extdata.supportTickets')}
          aria-describedby="gw-name-hint"
          onInput={(e) => onChange({ ...form, name: e.currentTarget.value })}
        />
        <FieldHint id="gw-name-hint">{t('extdata.field.name.hint')}</FieldHint>
      </FieldGroup>

      <InlineHelp tone="info" title={t('extdata.noCredentials')}>
        {t('extdata.gw.noCredBody')}
      </InlineHelp>
    </div>
  );
}

/**
 * Campos de la consulta a una aplicación conectada. Lo que se pide lo decide el
 * DRIVER de esa aplicación (una colección OData no es una consulta JQL), así que
 * las etiquetas y los ejemplos vienen de `gatewayQueryFields`.
 */
function GatewayQueryFields({
  form,
  driver,
  onChange,
}: {
  form: QueryForm;
  driver: string;
  onChange: (f: QueryForm) => void;
}): JSX.Element {
  const fields = gatewayQueryFields(driver);
  return (
    <div class="flex max-w-xl flex-col gap-4">
      {fields.dataset ? (
        <FieldGroup
          label={fields.dataset.label}
          htmlFor="gw-dataset"
          required={fields.dataset.required}
        >
          <Input
            id="gw-dataset"
            value={form.dataset}
            placeholder={fields.dataset.placeholder}
            aria-describedby="gw-dataset-hint"
            onInput={(e) => onChange({ ...form, dataset: e.currentTarget.value })}
          />
          <FieldHint id="gw-dataset-hint">{fields.dataset.hint}</FieldHint>
        </FieldGroup>
      ) : null}

      {fields.query ? (
        <FieldGroup label={fields.query.label} htmlFor="gw-query" required={fields.query.required}>
          {fields.query.code ? (
            <Textarea
              id="gw-query"
              rows={4}
              value={form.gatewayQuery}
              placeholder={fields.query.placeholder}
              class="font-mono"
              aria-describedby="gw-query-hint"
              onInput={(e) => onChange({ ...form, gatewayQuery: e.currentTarget.value })}
            />
          ) : (
            <Input
              id="gw-query"
              value={form.gatewayQuery}
              placeholder={fields.query.placeholder}
              aria-describedby="gw-query-hint"
              onInput={(e) => onChange({ ...form, gatewayQuery: e.currentTarget.value })}
            />
          )}
          <FieldHint id="gw-query-hint">{fields.query.hint}</FieldHint>
        </FieldGroup>
      ) : null}
    </div>
  );
}

/** Campos de la conexión nueva (API REST o PostgreSQL), con ayuda por campo. */
function ConnectionFields({
  form,
  onChange,
}: {
  form: ConnectionForm;
  onChange: (f: ConnectionForm) => void;
}): JSX.Element {
  const t = useT();
  const set = (patch: Partial<ConnectionForm>): void => onChange({ ...form, ...patch });
  const showSecret = form.kind === 'postgres' || form.authType !== 'none';
  const auth = AUTH_OPTIONS.find((a) => a.value === form.authType);
  const urlLooksWrong = form.baseUrl.trim() !== '' && !/^https?:\/\/.+/i.test(form.baseUrl.trim());
  const tokenUrlLooksWrong =
    form.tokenUrl.trim() !== '' && !/^https?:\/\/.+/i.test(form.tokenUrl.trim());

  return (
    <div class="flex max-w-xl flex-col gap-5">
      <FieldGroup label={t('extdata.field.name')} htmlFor="cf-name" required>
        <Input
          id="cf-name"
          value={form.name}
          placeholder={
            form.kind === 'postgres'
              ? t('extdata.field.name.placeholder.postgres')
              : t('extdata.field.name.placeholder.rest')
          }
          aria-describedby="cf-name-hint"
          onInput={(e) => set({ name: e.currentTarget.value })}
        />
        <FieldHint id="cf-name-hint">{t('extdata.field.name.hint')}</FieldHint>
      </FieldGroup>

      {form.kind === 'rest' ? (
        <>
          <FieldGroup label={t('extdata.field.baseUrl')} htmlFor="cf-url" required>
            <Input
              id="cf-url"
              value={form.baseUrl}
              placeholder={t('extdata.field.baseUrl.placeholder')}
              invalid={urlLooksWrong}
              aria-describedby="cf-url-hint"
              onInput={(e) => set({ baseUrl: e.currentTarget.value })}
            />
            <FieldHint id="cf-url-hint" tone={urlLooksWrong ? 'danger' : 'neutral'}>
              {urlLooksWrong
                ? t('extdata.field.baseUrl.hint.invalid')
                : t('extdata.field.baseUrl.hint')}
            </FieldHint>
          </FieldGroup>

          <FieldGroup label={t('extdata.field.auth')} htmlFor="cf-auth">
            <Select
              id="cf-auth"
              value={form.authType}
              aria-describedby="cf-auth-hint"
              onChange={(e) => set({ authType: e.currentTarget.value as RestAuthType })}
            >
              {AUTH_OPTIONS.map((a) => (
                <option key={a.value} value={a.value}>
                  {a.labelKey ? t(a.labelKey) : a.label}
                </option>
              ))}
            </Select>
            <FieldHint id="cf-auth-hint">{auth ? t(auth.hintKey) : ''}</FieldHint>
          </FieldGroup>

          {form.authType === 'api_key' ? (
            <div class="flex flex-wrap gap-3">
              <FieldGroup label={t('extdata.field.apiKeyIn')} htmlFor="cf-in" class="w-44">
                <Select
                  id="cf-in"
                  value={form.apiKeyIn}
                  onChange={(e) => set({ apiKeyIn: e.currentTarget.value as 'header' | 'query' })}
                >
                  <option value="header">{t('extdata.apiKeyIn.header')}</option>
                  <option value="query">{t('extdata.apiKeyIn.query')}</option>
                </Select>
              </FieldGroup>
              <FieldGroup
                label={
                  form.apiKeyIn === 'header'
                    ? t('extdata.field.apiKeyName.header')
                    : t('extdata.field.apiKeyName.query')
                }
                htmlFor="cf-keyname"
                required
                class="min-w-52 flex-1"
              >
                <Input
                  id="cf-keyname"
                  value={form.apiKeyName}
                  placeholder={form.apiKeyIn === 'header' ? 'X-API-Key' : 'api_key'}
                  aria-describedby="cf-keyname-hint"
                  onInput={(e) => set({ apiKeyName: e.currentTarget.value })}
                />
                <FieldHint id="cf-keyname-hint">{t('extdata.field.apiKeyName.hint')}</FieldHint>
              </FieldGroup>
            </div>
          ) : null}

          {form.authType === 'basic' ? (
            <FieldGroup label={t('extdata.field.username')} htmlFor="cf-user">
              <Input
                id="cf-user"
                value={form.username}
                onInput={(e) => set({ username: e.currentTarget.value })}
              />
            </FieldGroup>
          ) : null}

          {form.authType === 'oauth2_client_credentials' ? (
            <>
              <FieldGroup label={t('extdata.field.tokenUrl')} htmlFor="cf-tokenurl" required>
                <Input
                  id="cf-tokenurl"
                  value={form.tokenUrl}
                  placeholder={t('extdata.field.tokenUrl.placeholder')}
                  invalid={tokenUrlLooksWrong}
                  aria-describedby="cf-tokenurl-hint"
                  onInput={(e) => set({ tokenUrl: e.currentTarget.value })}
                />
                <FieldHint id="cf-tokenurl-hint" tone={tokenUrlLooksWrong ? 'danger' : 'neutral'}>
                  {tokenUrlLooksWrong
                    ? t('extdata.field.tokenUrl.hint.invalid')
                    : t('extdata.field.tokenUrl.hint')}
                </FieldHint>
              </FieldGroup>
              <FieldGroup label={t('extdata.field.clientId')} htmlFor="cf-clientid" required>
                <Input
                  id="cf-clientid"
                  value={form.clientId}
                  aria-describedby="cf-clientid-hint"
                  onInput={(e) => set({ clientId: e.currentTarget.value })}
                />
                <FieldHint id="cf-clientid-hint">{t('extdata.field.clientId.hint')}</FieldHint>
              </FieldGroup>
              <div class="flex flex-wrap gap-3">
                <FieldGroup
                  label={t('extdata.field.scope')}
                  htmlFor="cf-scope"
                  class="min-w-52 flex-1"
                >
                  <Input
                    id="cf-scope"
                    value={form.scope}
                    placeholder="read:data"
                    aria-describedby="cf-scope-hint"
                    onInput={(e) => set({ scope: e.currentTarget.value })}
                  />
                  <FieldHint id="cf-scope-hint">{t('extdata.field.scope.hint')}</FieldHint>
                </FieldGroup>
                <FieldGroup
                  label={t('extdata.field.audience')}
                  htmlFor="cf-audience"
                  class="min-w-52 flex-1"
                >
                  <Input
                    id="cf-audience"
                    value={form.audience}
                    aria-describedby="cf-audience-hint"
                    onInput={(e) => set({ audience: e.currentTarget.value })}
                  />
                  <FieldHint id="cf-audience-hint">
                    Opcional. Solo lo piden algunos proveedores (Auth0, por ejemplo).
                  </FieldHint>
                </FieldGroup>
              </div>
            </>
          ) : null}
        </>
      ) : (
        <>
          <div class="flex flex-wrap gap-3">
            <FieldGroup
              label={t('extdata.field.host')}
              htmlFor="cf-host"
              required
              class="min-w-52 flex-1"
            >
              <Input
                id="cf-host"
                value={form.host}
                placeholder={t('extdata.field.host.placeholder')}
                aria-describedby="cf-host-hint"
                onInput={(e) => set({ host: e.currentTarget.value })}
              />
              <FieldHint id="cf-host-hint">{t('extdata.field.host.hint')}</FieldHint>
            </FieldGroup>
            <FieldGroup label={t('extdata.field.port')} htmlFor="cf-port" class="w-28">
              <Input
                id="cf-port"
                type="number"
                value={form.port}
                placeholder="5432"
                onInput={(e) => set({ port: e.currentTarget.value })}
              />
            </FieldGroup>
          </div>
          <div class="flex flex-wrap gap-3">
            <FieldGroup
              label={t('extdata.field.database')}
              htmlFor="cf-db"
              required
              class="min-w-52 flex-1"
            >
              <Input
                id="cf-db"
                value={form.database}
                onInput={(e) => set({ database: e.currentTarget.value })}
              />
            </FieldGroup>
            <FieldGroup
              label={t('extdata.field.username')}
              htmlFor="cf-pguser"
              required
              class="min-w-52 flex-1"
            >
              <Input
                id="cf-pguser"
                value={form.user}
                aria-describedby="cf-pguser-hint"
                onInput={(e) => set({ user: e.currentTarget.value })}
              />
              <FieldHint id="cf-pguser-hint">{t('extdata.field.user.hint')}</FieldHint>
            </FieldGroup>
          </div>
          <FieldGroup label={t('extdata.field.sslmode')} htmlFor="cf-ssl" class="w-52">
            <Select
              id="cf-ssl"
              value={form.sslmode}
              aria-describedby="cf-ssl-hint"
              onChange={(e) => set({ sslmode: e.currentTarget.value })}
            >
              {SSL_MODES.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </Select>
            <FieldHint id="cf-ssl-hint">{t('extdata.field.sslmode.hint')}</FieldHint>
          </FieldGroup>
        </>
      )}

      {showSecret ? (
        <FieldGroup label={secretLabel(form, t)} htmlFor="cf-secret" required>
          <Input
            id="cf-secret"
            type="password"
            autocomplete="new-password"
            value={form.secret}
            aria-describedby="cf-secret-hint"
            onInput={(e) => set({ secret: e.currentTarget.value })}
          />
          <FieldHint id="cf-secret-hint">{secretHint(form, t)}</FieldHint>
          <FieldHint tone="info">{t('extdata.secret.stored')}</FieldHint>
        </FieldGroup>
      ) : null}
    </div>
  );
}

/** Campos de consulta REST: método, ruta, parámetros y ruta-raíz. */
function RestQueryFields({
  form,
  baseUrl,
  onChange,
}: {
  form: QueryForm;
  baseUrl: string;
  onChange: (f: QueryForm) => void;
}): JSX.Element {
  const t = useT();
  const set = (patch: Partial<QueryForm>): void => onChange({ ...form, ...patch });
  const setParam = (i: number, patch: Partial<{ key: string; value: string }>): void =>
    set({ params: form.params.map((p, j) => (j === i ? { ...p, ...patch } : p)) });

  return (
    <div class="flex flex-col gap-4">
      <div class="flex flex-wrap gap-3">
        <FieldGroup label={t('extdata.field.method')} htmlFor="q-method" class="w-28">
          <Select
            id="q-method"
            value={form.method}
            onChange={(e) => set({ method: e.currentTarget.value as 'GET' | 'POST' })}
          >
            <option value="GET">GET</option>
            <option value="POST">POST</option>
          </Select>
        </FieldGroup>
        <FieldGroup label={t('extdata.field.path')} htmlFor="q-path" class="min-w-52 flex-1">
          <Input
            id="q-path"
            value={form.path}
            placeholder={t('extdata.field.path.placeholder')}
            aria-describedby="q-path-hint"
            onInput={(e) => set({ path: e.currentTarget.value })}
          />
          <FieldHint id="q-path-hint">
            {baseUrl
              ? t('extdata.field.path.hint.preview', {
                  method: form.method,
                  url: `${baseUrl}${form.path}`,
                })
              : t('extdata.field.path.hint')}
          </FieldHint>
        </FieldGroup>
      </div>

      <div class="flex flex-col gap-2">
        <span class="text-text text-sm font-semibold">{t('query.parameters')}</span>
        <p class="sytue-meta -mt-1">
          Los filtros que la API espera en la URL: por ejemplo <code>desde</code> = 2026-01-01.
        </p>
        {form.params.map((p, i) => (
          <div key={i} class="flex items-center gap-2">
            <Input
              value={p.key}
              placeholder={t('extdata.param.name.placeholder')}
              aria-label={t('extdata.param.name', { n: i + 1 })}
              onInput={(e) => setParam(i, { key: e.currentTarget.value })}
            />
            <Input
              value={p.value}
              placeholder={t('extdata.param.value.placeholder')}
              aria-label={t('extdata.param.value', { n: i + 1 })}
              onInput={(e) => setParam(i, { value: e.currentTarget.value })}
            />
            <Button
              variant="ghost"
              size="sm"
              iconOnly
              aria-label={t('extdata.param.remove', { n: i + 1 })}
              onClick={() => set({ params: form.params.filter((_, j) => j !== i) })}
            >
              <EditorIcon name="close" size="sm" />
            </Button>
          </div>
        ))}
        <div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => set({ params: [...form.params, { key: '', value: '' }] })}
          >
            <EditorIcon name="plus" size="sm" />
            {t('extdata.param.add')}
          </Button>
        </div>
      </div>

      <FieldGroup label={t('extdata.field.rootPath')} htmlFor="q-root">
        <Input
          id="q-root"
          value={form.rootPath}
          placeholder="data.items"
          aria-describedby="q-root-hint"
          onInput={(e) => set({ rootPath: e.currentTarget.value })}
        />
        <FieldHint id="q-root-hint">{t('extdata.field.rootPath.hint')}</FieldHint>
      </FieldGroup>
    </div>
  );
}

/** Campo de consulta PostgreSQL: una sentencia SELECT. */
function PostgresQueryFields({
  form,
  onChange,
}: {
  form: QueryForm;
  onChange: (f: QueryForm) => void;
}): JSX.Element {
  const t = useT();
  const looksUnsafe = form.sql.trim() !== '' && !/^\s*(select|with)\b/i.test(form.sql);
  return (
    <FieldGroup label={t('extdata.field.sql')} htmlFor="q-sql" required>
      <Textarea
        id="q-sql"
        rows={5}
        value={form.sql}
        placeholder={t('extdata.field.sqlExample')}
        class="font-mono"
        invalid={looksUnsafe}
        aria-describedby="q-sql-hint"
        onInput={(e) => onChange({ ...form, sql: e.currentTarget.value })}
      />
      <FieldHint id="q-sql-hint" tone={looksUnsafe ? 'danger' : 'neutral'}>
        {looksUnsafe ? t('extdata.field.sql.hint.invalid') : t('extdata.field.sql.hint')}
      </FieldHint>
    </FieldGroup>
  );
}

/** Tabla de previsualización (cabecera + primeras filas). */
function PreviewTable({ result }: { result: ExecuteResult }): JSX.Element {
  const grid = resultToGrid(result);
  const header = grid[0] ?? [];
  const rows = grid.slice(1, PREVIEW_ROWS + 1);
  const total = result.rows.length;
  return (
    <div class="flex flex-col gap-1.5">
      <div class="border-border max-h-64 overflow-auto rounded-md border">
        <table class="w-full border-collapse text-xs">
          <thead class="bg-bg-subtle sticky top-0">
            <tr>
              {header.map((h, i) => (
                <th
                  key={i}
                  class="border-border text-text truncate border-b px-2 py-1 text-left font-semibold"
                >
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, r) => (
              <tr key={r} class="odd:bg-bg-subtle/40">
                {header.map((_, c) => (
                  <td
                    key={c}
                    class="border-border/60 text-text-muted max-w-52 truncate border-b px-2 py-1"
                  >
                    {row[c] ?? ''}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p class="sytue-meta">
        {total} fila(s) · {header.length} columna(s)
        {total > PREVIEW_ROWS ? ` · se muestran las primeras ${PREVIEW_ROWS}` : ''}
      </p>
    </div>
  );
}

/** Ref A1 «C7» a partir de índices 0-based (evita depender de helpers internos). */
function colRow(c: number, r: number): string {
  let s = '';
  let n = c;
  do {
    s = String.fromCharCode(65 + (n % 26)) + s;
    n = Math.floor(n / 26) - 1;
  } while (n >= 0);
  return `${s}${r + 1}`;
}
