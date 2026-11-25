/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Derechos RGPD del lado del responsable (ADR 0054, enmienda del 2026-11-25): el
 * registro de solicitudes de la organización y la supresión física de una cuenta.
 *
 * Lo que tiene que quedar claro en la pantalla, más que en ningún otro sitio:
 *   - el plazo es el de la ley (un mes desde que LLEGÓ, prorrogable dos una vez) y
 *     lo calcula el servidor; aquí solo se pinta;
 *   - cerrar exige escribir qué se le contestó a la persona;
 *   - la supresión no se deshace, y lo que borra y lo que NO borra se dice antes del
 *     botón, no después.
 *
 * Los textos van en el catálogo de 26 idiomas `i18n/privacidad`; las claves se
 * escriben enteras (mapas de abajo) para que se vea qué se usa sin plantillas.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  FieldGroup,
  InlineHelp,
  Input,
  Modal,
  SegmentedControl,
  Select,
  SettingsSection,
  SettingsShell,
  Spinner,
  Stat,
  Table,
  Textarea,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import { formatDate, formatDateTime, type Locale } from '@sytue/i18n';
import { useTPrivacidad, type TPrivacidad } from '../i18n/privacidad/index.js';
import type { en } from '../i18n/privacidad/messages/en.js';
import { pedirReautenticacion } from '../lib/reautenticar.js';
import {
  CANALES,
  ESTADOS,
  TIPOS,
  prorrogable,
  urgencia,
  validarAlta,
  type AltaSolicitud,
  type CanalSolicitud,
  type DetalleRGPD,
  type EstadoSolicitud,
  type ListadoRGPD,
  type SolicitudRGPD,
  type TipoSolicitud,
} from '../lib/privacidad.js';

type Clave = keyof typeof en;

const ETIQUETA_TIPO: Record<TipoSolicitud, Clave> = {
  access: 'privacidad.tipo.access',
  rectification: 'privacidad.tipo.rectification',
  erasure: 'privacidad.tipo.erasure',
  restriction: 'privacidad.tipo.restriction',
  portability: 'privacidad.tipo.portability',
  objection: 'privacidad.tipo.objection',
};

const ETIQUETA_CANAL: Record<CanalSolicitud, Clave> = {
  email: 'privacidad.canal.email',
  web: 'privacidad.canal.web',
  letter: 'privacidad.canal.letter',
  phone: 'privacidad.canal.phone',
  in_person: 'privacidad.canal.in_person',
  other: 'privacidad.canal.other',
};

const ETIQUETA_ESTADO: Record<EstadoSolicitud, Clave> = {
  received: 'privacidad.estado.received',
  in_progress: 'privacidad.estado.in_progress',
  completed: 'privacidad.estado.completed',
  rejected: 'privacidad.estado.rejected',
};

const TONO_ESTADO: Record<EstadoSolicitud, BadgeTone> = {
  received: 'info',
  in_progress: 'primary',
  completed: 'success',
  rejected: 'neutral',
};

const ETIQUETA_EVENTO: Record<string, Clave> = {
  created: 'privacidad.evento.created',
  updated: 'privacidad.evento.updated',
  extended: 'privacidad.evento.extended',
  'erasure.started': 'privacidad.evento.erasureStarted',
  'erasure.completed': 'privacidad.evento.erasureCompleted',
};

const ERROR_ALTA: Record<'subject' | 'email' | 'date', Clave> = {
  subject: 'privacidad.alta.error.subject',
  email: 'privacidad.alta.error.email',
  date: 'privacidad.alta.error.date',
};

type Filtro = 'open' | 'overdue' | 'all';

/** Una persona del directorio, lo justo para elegirla. */
export interface PersonaDirectorio {
  id: string;
  email: string;
  fullName: string;
}

interface Props {
  locale: Locale;
  personas: PersonaDirectorio[];
  inicial: ListadoRGPD | null;
}

/** Lee el motivo que manda el BFF (código y mensaje de Sytue ID), si lo hay. */
async function motivoDe(res: Response | null): Promise<string> {
  const b = (await res?.json().catch(() => null)) as { message?: string } | null;
  return typeof b?.message === 'string' ? b.message : '';
}

function Plazo({ s, t, locale }: { s: SolicitudRGPD; t: TPrivacidad; locale: Locale }) {
  const u = urgencia(s);
  return (
    <span class="flex flex-wrap items-center gap-1.5">
      <span class="tabular-nums">{formatDate(locale, s.due_at)}</span>
      {u === 'overdue' ? <Badge tone="danger">{t('privacidad.plazo.vencida')}</Badge> : null}
      {u === 'soon' ? <Badge tone="warning">{t('privacidad.plazo.pronto')}</Badge> : null}
      {s.extended ? <Badge tone="neutral">{t('privacidad.plazo.prorrogada')}</Badge> : null}
    </span>
  );
}

function Inner({ locale, personas, inicial }: Props): JSX.Element {
  const t = useTPrivacidad(locale);
  const toast = useToast();
  const [filtro, setFiltro] = useState<Filtro>('open');
  const [listado, setListado] = useState<ListadoRGPD | null>(inicial);
  const [cargando, setCargando] = useState(false);
  const [fallo, setFallo] = useState(inicial === null);
  const [alta, setAlta] = useState(false);
  const [abierta, setAbierta] = useState<string | null>(null);

  const cargar = useCallback(async (f: Filtro): Promise<void> => {
    setCargando(true);
    const q = f === 'open' ? '?status=open' : f === 'overdue' ? '?overdue=1' : '';
    const res = await fetch(`/api/admin/privacy/requests${q}`, { credentials: 'include' }).catch(
      () => null,
    );
    setCargando(false);
    if (!res?.ok) {
      setFallo(true);
      return;
    }
    setFallo(false);
    setListado((await res.json()) as ListadoRGPD);
  }, []);

  // La página ya trae las abiertas; las demás vistas se piden al servidor.
  const conInicial = inicial !== null;
  useEffect(() => {
    if (filtro !== 'open' || !conInicial) void cargar(filtro);
  }, [filtro, conInicial, cargar]);

  const filas = listado?.requests ?? [];
  return (
    <SettingsShell title={t('privacidad.titulo')} description={t('privacidad.descripcion')}>
      <SettingsSection title={t('privacidad.titulo')}>
        <div class="flex flex-col gap-4">
          <div class="grid grid-cols-2 gap-3 sm:max-w-md">
            <Stat label={t('privacidad.resumen.abiertas')} value={listado?.summary.open ?? '—'} />
            <Stat
              label={t('privacidad.resumen.vencidas')}
              value={listado?.summary.overdue ?? '—'}
              tone={(listado?.summary.overdue ?? 0) > 0 ? 'danger' : 'neutral'}
            />
          </div>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <SegmentedControl<Filtro>
              aria-label={t('privacidad.filtro')}
              size="sm"
              valor={filtro}
              onChange={setFiltro}
              opciones={[
                { valor: 'open', rotulo: t('privacidad.filtro.abiertas') },
                { valor: 'overdue', rotulo: t('privacidad.filtro.vencidas') },
                { valor: 'all', rotulo: t('privacidad.filtro.todas') },
              ]}
            />
            <Button variant="primary" onClick={() => setAlta(true)}>
              {t('privacidad.registrar')}
            </Button>
          </div>
          {cargando ? (
            <div class="flex justify-center py-8" aria-busy="true">
              <Spinner size={22} />
            </div>
          ) : fallo ? (
            <InlineHelp tone="danger" title={t('privacidad.errorCarga')} />
          ) : (
            <Table<SolicitudRGPD>
              rows={filas}
              rowKey={(s) => s.id}
              emptyContent={<p class="text-text-muted py-6 text-sm">{t('privacidad.vacia')}</p>}
              columns={[
                {
                  key: 'persona',
                  header: t('privacidad.col.persona'),
                  cell: (s) => (
                    <button
                      type="button"
                      class="sytue-focus-ring text-left"
                      onClick={() => setAbierta(s.id)}
                    >
                      <span class="text-text block font-medium">
                        {s.subject_name || s.subject_email}
                      </span>
                      {s.subject_name ? (
                        <span class="text-text-muted block text-xs">{s.subject_email}</span>
                      ) : null}
                    </button>
                  ),
                },
                {
                  key: 'tipo',
                  header: t('privacidad.col.tipo'),
                  cell: (s) => t(ETIQUETA_TIPO[s.kind]),
                },
                {
                  key: 'recibida',
                  header: t('privacidad.col.recibida'),
                  cell: (s) => formatDate(locale, s.received_at),
                },
                {
                  key: 'plazo',
                  header: t('privacidad.col.plazo'),
                  cell: (s) => <Plazo s={s} t={t} locale={locale} />,
                },
                {
                  key: 'estado',
                  header: t('privacidad.col.estado'),
                  cell: (s) => (
                    <Badge tone={TONO_ESTADO[s.status]}>{t(ETIQUETA_ESTADO[s.status])}</Badge>
                  ),
                },
              ]}
            />
          )}
        </div>
      </SettingsSection>
      {alta ? (
        <Alta
          t={t}
          personas={personas}
          onCerrar={() => setAlta(false)}
          onHecha={() => {
            setAlta(false);
            toast.notify({ tone: 'success', title: t('privacidad.alta.hecha') });
            void cargar(filtro);
          }}
        />
      ) : null}
      {abierta ? (
        <Detalle
          id={abierta}
          t={t}
          locale={locale}
          onCerrar={() => setAbierta(null)}
          onCambio={() => void cargar(filtro)}
        />
      ) : null}
    </SettingsShell>
  );
}

function Alta({
  t,
  personas,
  onCerrar,
  onHecha,
}: {
  t: TPrivacidad;
  personas: PersonaDirectorio[];
  onCerrar: () => void;
  onHecha: () => void;
}): JSX.Element {
  const [deFuera, setDeFuera] = useState(personas.length === 0);
  const [userId, setUserId] = useState('');
  const [correo, setCorreo] = useState('');
  const [nombre, setNombre] = useState('');
  const [tipo, setTipo] = useState<TipoSolicitud>('access');
  const [canal, setCanal] = useState<CanalSolicitud>('email');
  const [recibida, setRecibida] = useState(() => new Date().toISOString().slice(0, 10));
  const [verificada, setVerificada] = useState(false);
  const [metodo, setMetodo] = useState('');
  const [notas, setNotas] = useState('');
  const [error, setError] = useState('');
  const [guardando, setGuardando] = useState(false);

  async function guardar(): Promise<void> {
    const a: AltaSolicitud = {
      kind: tipo,
      channel: canal,
      received_at: recibida,
      identity_verified: verificada,
      ...(deFuera ? { subject_email: correo.trim() } : { user_id: userId }),
      ...(deFuera && nombre.trim() ? { subject_name: nombre.trim() } : {}),
      ...(metodo.trim() ? { identity_method: metodo.trim() } : {}),
      ...(notas.trim() ? { notes: notas.trim() } : {}),
    };
    const invalido = validarAlta(a);
    if (invalido === 'subject' || invalido === 'email' || invalido === 'date') {
      setError(t(ERROR_ALTA[invalido]));
      return;
    }
    setGuardando(true);
    setError('');
    const res = await fetch('/api/admin/privacy/requests', {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(a),
    }).catch(() => null);
    setGuardando(false);
    if (res?.ok) {
      onHecha();
      return;
    }
    const motivo = await motivoDe(res);
    setError(
      motivo
        ? `${t('privacidad.alta.error.upstream')} ${motivo}`
        : t('privacidad.alta.error.upstream'),
    );
  }

  return (
    <Modal
      open
      onClose={onCerrar}
      title={t('privacidad.registrar')}
      placement="right"
      footer={
        <>
          <Button variant="secondary" onClick={onCerrar}>
            {t('privacidad.cancelar')}
          </Button>
          <Button variant="primary" loading={guardando} onClick={() => void guardar()}>
            {t('privacidad.registrar')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <SegmentedControl<'miembro' | 'fuera'>
          aria-label={t('privacidad.alta.quien')}
          size="sm"
          valor={deFuera ? 'fuera' : 'miembro'}
          onChange={(v) => setDeFuera(v === 'fuera')}
          opciones={[
            { valor: 'miembro', rotulo: t('privacidad.alta.miembro') },
            { valor: 'fuera', rotulo: t('privacidad.alta.fuera') },
          ]}
        />
        {deFuera ? (
          <>
            <FieldGroup label={t('privacidad.alta.correo')} htmlFor="rgpd-correo" required>
              <Input
                id="rgpd-correo"
                type="email"
                value={correo}
                onInput={(e) => setCorreo((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
            <FieldGroup label={t('privacidad.alta.nombre')} htmlFor="rgpd-nombre">
              <Input
                id="rgpd-nombre"
                value={nombre}
                onInput={(e) => setNombre((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
          </>
        ) : (
          <FieldGroup label={t('privacidad.alta.elegirMiembro')} htmlFor="rgpd-persona" required>
            <Select
              id="rgpd-persona"
              value={userId}
              onChange={(e) => setUserId((e.target as HTMLSelectElement).value)}
            >
              <option value="">—</option>
              {personas.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.fullName ? `${p.fullName} · ${p.email}` : p.email}
                </option>
              ))}
            </Select>
          </FieldGroup>
        )}
        <FieldGroup label={t('privacidad.alta.tipo')} htmlFor="rgpd-tipo" required>
          <Select
            id="rgpd-tipo"
            value={tipo}
            onChange={(e) => setTipo((e.target as HTMLSelectElement).value as TipoSolicitud)}
          >
            {TIPOS.map((k) => (
              <option key={k} value={k}>
                {t(ETIQUETA_TIPO[k])}
              </option>
            ))}
          </Select>
        </FieldGroup>
        <FieldGroup label={t('privacidad.alta.canal')} htmlFor="rgpd-canal" required>
          <Select
            id="rgpd-canal"
            value={canal}
            onChange={(e) => setCanal((e.target as HTMLSelectElement).value as CanalSolicitud)}
          >
            {CANALES.map((k) => (
              <option key={k} value={k}>
                {t(ETIQUETA_CANAL[k])}
              </option>
            ))}
          </Select>
        </FieldGroup>
        <FieldGroup
          label={t('privacidad.alta.recibida')}
          htmlFor="rgpd-recibida"
          help={t('privacidad.alta.recibidaAyuda')}
          required
        >
          <Input
            id="rgpd-recibida"
            type="date"
            value={recibida}
            onInput={(e) => setRecibida((e.target as HTMLInputElement).value)}
          />
        </FieldGroup>
        <Checkbox
          label={t('privacidad.alta.identidad')}
          checked={verificada}
          onChange={(e) => setVerificada((e.target as HTMLInputElement).checked)}
        />
        <FieldGroup label={t('privacidad.alta.metodo')} htmlFor="rgpd-metodo">
          <Input
            id="rgpd-metodo"
            value={metodo}
            onInput={(e) => setMetodo((e.target as HTMLInputElement).value)}
          />
        </FieldGroup>
        <FieldGroup label={t('privacidad.alta.notas')} htmlFor="rgpd-notas">
          <Textarea
            id="rgpd-notas"
            rows={3}
            value={notas}
            onInput={(e) => setNotas((e.target as HTMLTextAreaElement).value)}
          />
        </FieldGroup>
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}

function Detalle({
  id,
  t,
  locale,
  onCerrar,
  onCambio,
}: {
  id: string;
  t: TPrivacidad;
  locale: Locale;
  onCerrar: () => void;
  onCambio: () => void;
}): JSX.Element {
  const toast = useToast();
  const [d, setD] = useState<DetalleRGPD | null>(null);
  const [fallo, setFallo] = useState(false);
  const [estado, setEstado] = useState<EstadoSolicitud>('received');
  const [resolucion, setResolucion] = useState('');
  const [notas, setNotas] = useState('');
  const [motivoProrroga, setMotivoProrroga] = useState('');
  const [confirmacion, setConfirmacion] = useState('');
  const [ocupado, setOcupado] = useState<'' | 'guardar' | 'prorrogar' | 'suprimir'>('');
  const [error, setError] = useState('');

  const leer = useCallback(async (): Promise<void> => {
    const res = await fetch(`/api/admin/privacy/requests/${encodeURIComponent(id)}`, {
      credentials: 'include',
    }).catch(() => null);
    if (!res?.ok) {
      setFallo(true);
      return;
    }
    const detalle = (await res.json()) as DetalleRGPD;
    setD(detalle);
    setEstado(detalle.request.status);
    setResolucion(detalle.request.resolution ?? '');
    setNotas(detalle.request.notes ?? '');
  }, [id]);

  useEffect(() => {
    void leer();
  }, [leer]);

  async function enviar(
    que: 'guardar' | 'prorrogar' | 'suprimir',
    ruta: string,
    metodo: string,
    cuerpo: unknown,
    exito: string,
    errorGenerico: string,
  ): Promise<void> {
    setOcupado(que);
    setError('');
    const res = await fetch(ruta, {
      method: metodo,
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(cuerpo),
    }).catch(() => null);
    setOcupado('');
    if (res && (await pedirReautenticacion(res))) return;
    if (res?.ok) {
      toast.notify({ tone: 'success', title: exito });
      onCambio();
      await leer();
      return;
    }
    const motivo = await motivoDe(res);
    setError(motivo ? `${errorGenerico} ${motivo}` : errorGenerico);
  }

  const s = d?.request;
  const base = `/api/admin/privacy/requests/${encodeURIComponent(id)}`;
  return (
    <Modal
      open
      onClose={onCerrar}
      title={s ? s.subject_name || s.subject_email : ''}
      placement="right"
    >
      {fallo ? (
        <InlineHelp tone="danger" title={t('privacidad.errorCarga')} />
      ) : !s || !d ? (
        <div class="flex justify-center py-8" aria-busy="true">
          <Spinner size={22} />
        </div>
      ) : (
        <div class="flex flex-col gap-5">
          <div class="flex flex-wrap items-center gap-2">
            <Badge tone={TONO_ESTADO[s.status]}>{t(ETIQUETA_ESTADO[s.status])}</Badge>
            <span class="text-text text-sm">{t(ETIQUETA_TIPO[s.kind])}</span>
            <span class="text-text-muted text-sm">· {t(ETIQUETA_CANAL[s.channel])}</span>
          </div>
          <p class="text-text text-sm">
            {t('privacidad.detalle.plazo', { fecha: formatDate(locale, s.due_at) })}{' '}
            <Plazo s={s} t={t} locale={locale} />
          </p>
          {s.subject_name ? <p class="text-text-muted text-sm">{s.subject_email}</p> : null}

          {s.closed_at ? (
            <InlineHelp tone="info" title={t('privacidad.detalle.cerrada')}>
              {s.resolution}
            </InlineHelp>
          ) : (
            <div class="flex flex-col gap-3">
              <FieldGroup label={t('privacidad.col.estado')} htmlFor="rgpd-estado">
                <Select
                  id="rgpd-estado"
                  value={estado}
                  onChange={(e) =>
                    setEstado((e.target as HTMLSelectElement).value as EstadoSolicitud)
                  }
                >
                  {ESTADOS.map((k) => (
                    <option key={k} value={k}>
                      {t(ETIQUETA_ESTADO[k])}
                    </option>
                  ))}
                </Select>
              </FieldGroup>
              <FieldGroup
                label={t('privacidad.detalle.resolucion')}
                htmlFor="rgpd-resolucion"
                help={t('privacidad.detalle.resolucionAyuda')}
              >
                <Textarea
                  id="rgpd-resolucion"
                  rows={3}
                  value={resolucion}
                  onInput={(e) => setResolucion((e.target as HTMLTextAreaElement).value)}
                />
              </FieldGroup>
              <FieldGroup label={t('privacidad.alta.notas')} htmlFor="rgpd-notas-detalle">
                <Textarea
                  id="rgpd-notas-detalle"
                  rows={2}
                  value={notas}
                  onInput={(e) => setNotas((e.target as HTMLTextAreaElement).value)}
                />
              </FieldGroup>
              <div class="flex justify-end">
                <Button
                  variant="primary"
                  loading={ocupado === 'guardar'}
                  onClick={() =>
                    void enviar(
                      'guardar',
                      base,
                      'PATCH',
                      { status: estado, resolution: resolucion, notes: notas },
                      t('privacidad.detalle.guardado'),
                      t('privacidad.error.upstream'),
                    )
                  }
                >
                  {t('privacidad.detalle.guardar')}
                </Button>
              </div>
            </div>
          )}

          {prorrogable(s) ? (
            <section class="border-border flex flex-col gap-2 border-t pt-4">
              <h3 class="text-text text-sm font-semibold">{t('privacidad.detalle.prorroga')}</h3>
              <FieldGroup label={t('privacidad.detalle.prorrogaAyuda')} htmlFor="rgpd-prorroga">
                <Textarea
                  id="rgpd-prorroga"
                  rows={2}
                  value={motivoProrroga}
                  onInput={(e) => setMotivoProrroga((e.target as HTMLTextAreaElement).value)}
                />
              </FieldGroup>
              <div class="flex justify-end">
                <Button
                  variant="secondary"
                  disabled={!motivoProrroga.trim()}
                  loading={ocupado === 'prorrogar'}
                  onClick={() =>
                    void enviar(
                      'prorrogar',
                      `${base}/extend`,
                      'POST',
                      { reason: motivoProrroga.trim() },
                      t('privacidad.evento.extended'),
                      t('privacidad.error.upstream'),
                    )
                  }
                >
                  {t('privacidad.detalle.prorrogar')}
                </Button>
              </div>
            </section>
          ) : null}

          {s.kind === 'erasure' ? (
            <section class="border-border flex flex-col gap-2 border-t pt-4">
              <h3 class="text-text text-sm font-semibold">{t('privacidad.supresion.titulo')}</h3>
              {d.erasure.available ? (
                <>
                  <InlineHelp tone="warning" title={t('privacidad.supresion.titulo')}>
                    {t('privacidad.supresion.explica')}
                  </InlineHelp>
                  <FieldGroup
                    label={t('privacidad.supresion.confirmar', { correo: s.subject_email })}
                    htmlFor="rgpd-confirmar"
                  >
                    <Input
                      id="rgpd-confirmar"
                      value={confirmacion}
                      autoComplete="off"
                      onInput={(e) => setConfirmacion((e.target as HTMLInputElement).value)}
                    />
                  </FieldGroup>
                  <div class="flex justify-end">
                    <Button
                      variant="danger"
                      disabled={confirmacion.trim().toLowerCase() !== s.subject_email.toLowerCase()}
                      loading={ocupado === 'suprimir'}
                      onClick={() =>
                        void enviar(
                          'suprimir',
                          `${base}/erase`,
                          'POST',
                          { confirm: confirmacion.trim() },
                          t('privacidad.supresion.iniciada'),
                          t('privacidad.supresion.error'),
                        )
                      }
                    >
                      {t('privacidad.supresion.titulo')}
                    </Button>
                  </div>
                </>
              ) : (
                <p class="text-text-muted text-sm">
                  {d.erasure.reason === 'completed'
                    ? t('privacidad.supresion.hecha')
                    : d.erasure.reason === 'in_progress'
                      ? t('privacidad.supresion.enCurso')
                      : d.erasure.reason === 'not_member'
                        ? t('privacidad.supresion.noMiembro')
                        : d.erasure.reason === 'no_account'
                          ? t('privacidad.supresion.sinCuenta')
                          : t('privacidad.detalle.cerrada')}
                </p>
              )}
            </section>
          ) : null}

          {error ? (
            <p role="alert" class="text-danger-text text-sm">
              {error}
            </p>
          ) : null}

          <section class="border-border flex flex-col gap-2 border-t pt-4">
            <h3 class="text-text text-sm font-semibold">{t('privacidad.detalle.historia')}</h3>
            <ol class="flex flex-col gap-1.5">
              {d.events.map((e) => {
                const clave = ETIQUETA_EVENTO[e.action];
                return (
                  <li key={e.id} class="text-text-muted flex justify-between gap-3 text-sm">
                    <span>{clave ? t(clave) : e.action}</span>
                    <span class="tabular-nums">{formatDateTime(locale, e.at)}</span>
                  </li>
                );
              })}
            </ol>
          </section>
        </div>
      )}
    </Modal>
  );
}

export default function PrivacyPanel(props: Props): JSX.Element {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
