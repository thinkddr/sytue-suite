/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La cola de denuncias del OPERADOR (ADR 0073 y 0074, «Cola del operador»).
 *
 * Las denuncias de Sytue Cast y de Sytue Rooms llegan aquí y a la cola de quien
 * administra la organización dueña del contenido. Son las mismas filas: resolver
 * aquí la cierra allí, y allí queda que fue desde la plataforma y quién.
 *
 * Se ve solo lo que la denuncia guarda —el extracto, el mensaje acotado, el
 * nombre—: no hay enlace para entrar en el contenido del cliente. Quién denunció
 * no viaja nunca. Cada lectura y cada acción la deja Sytue ID en el registro de
 * plataforma y en el de la organización, porque cada servicio le pregunta antes.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  EmptyState,
  Table,
  Tabs,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type ConsoleMessageKey as MessageKey, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';

type Servicio = 'cast' | 'rooms';
type Estado = 'open' | 'resolved';

/** Una fila de cualquiera de las dos colas, ya en una forma. */
interface Fila {
  id: string;
  org: string;
  tipo: MessageKey;
  motivo: string;
  quien: string;
  contenido: string;
  detalle: string;
  creada: string;
  resolucion: string;
  desde: string;
  acciones: string[];
}

interface Props {
  locale: Locale;
  /** Por qué no se puede abrir la cola: 403 (sin elevar, sin factor) o 503. */
  problema?: number | undefined;
}

const MOTIVO: Record<string, MessageKey> = {
  harassment: 'mod.reason.harassment',
  sexual: 'mod.reason.sexual',
  violence: 'mod.reason.violence',
  spam: 'mod.reason.spam',
  copyright: 'mod.reason.copyright',
  other: 'mod.reason.other',
};

const TIPO_CAST: Record<string, MessageKey> = {
  video: 'mod.kind.video',
  comment: 'mod.kind.comment',
  chat: 'mod.kind.chat',
  person: 'mod.kind.person',
  channel: 'mod.kind.channel',
};

/** Acción de cada servicio → su rótulo. Rooms nombra la acción por el estado en que deja la denuncia. */
const ACCION: Record<string, MessageKey> = {
  dismiss: 'mod.action.dismiss',
  delete: 'mod.action.delete',
  unlist: 'mod.action.unlist',
  unpublish: 'mod.action.unpublish',
  block: 'mod.action.block',
  dismissed: 'mod.action.dismiss',
  message_deleted: 'mod.action.delete',
  member_removed: 'mod.action.remove',
};

interface DenunciaCast {
  id: string;
  tenant: string;
  kind: string;
  target_name?: string;
  excerpt?: string;
  reason: string;
  details?: string;
  status: string;
  resolution?: string;
  resolved_from?: string;
  created_at: string;
  actions: string[];
}

interface DenunciaRooms {
  id: string;
  tenant?: string;
  channel_name: string;
  channel_kind: string;
  target_name: string;
  message_id?: string;
  message_body?: string;
  reason: string;
  note?: string;
  status: string;
  resolved_from?: string;
  created_at: string;
}

function deCast(d: DenunciaCast): Fila {
  return {
    id: d.id,
    org: d.tenant,
    tipo: TIPO_CAST[d.kind] ?? 'mod.kind.video',
    motivo: d.reason,
    quien: d.target_name ?? '',
    contenido: d.excerpt ?? '',
    detalle: d.details ?? '',
    creada: d.created_at,
    resolucion: d.resolution ?? '',
    desde: d.resolved_from ?? '',
    acciones: d.actions,
  };
}

function deRooms(d: DenunciaRooms): Fila {
  const abierta = d.status === 'open';
  return {
    id: d.id,
    org: d.tenant ?? '',
    tipo: d.message_id ? 'mod.kind.message' : 'mod.kind.member',
    motivo: d.reason,
    quien: d.target_name,
    contenido: d.message_body ?? '',
    detalle: d.note ?? '',
    creada: d.created_at,
    resolucion: abierta ? '' : d.status,
    desde: d.resolved_from ?? '',
    // Las mismas que ofrece la cola de la organización (store.ResolveReport).
    acciones: abierta
      ? [
          'dismissed',
          ...(d.message_id ? ['message_deleted'] : []),
          ...(d.channel_kind === 'direct' ? [] : ['member_removed']),
        ]
      : [],
  };
}

async function cargar(servicio: Servicio, estado: Estado): Promise<Fila[] | number> {
  const ruta =
    servicio === 'cast'
      ? `/api/platform-moderation/cast/reports?status=${estado}&limit=200`
      : `/api/platform-moderation/rooms/reports${estado === 'resolved' ? '?all=1' : ''}`;
  try {
    const res = await fetch(ruta);
    if (!res.ok) return res.status;
    if (servicio === 'cast') {
      const cuerpo = (await res.json()) as { reports?: DenunciaCast[] };
      return (cuerpo.reports ?? []).map(deCast);
    }
    const cuerpo = (await res.json()) as { items?: DenunciaRooms[] };
    return (cuerpo.items ?? [])
      .filter((d) => (estado === 'open') === (d.status === 'open'))
      .map(deRooms);
  } catch {
    return 503;
  }
}

function resolverRuta(servicio: Servicio, id: string): string {
  return servicio === 'cast'
    ? `/api/platform-moderation/cast/reports/${id}`
    : `/api/platform-moderation/rooms/reports/${id}/resolve`;
}

function Aviso({ codigo, t }: { codigo: number; t: T }): JSX.Element {
  return (
    <EmptyState
      title={t(codigo === 503 ? 'mod.error.unavailable' : 'mod.error.forbidden')}
      description={t(codigo === 503 ? 'mod.error.unavailableBody' : 'mod.error.forbiddenBody')}
      action={
        codigo === 503 ? undefined : (
          <a class="text-primary underline" href={r('/platform')}>
            {t('nav.platform')}
          </a>
        )
      }
    />
  );
}

function Cola({ servicio, locale, t }: { servicio: Servicio; locale: Locale; t: T }): JSX.Element {
  const toast = useToast();
  const [estado, setEstado] = useState<Estado>('open');
  const [filas, setFilas] = useState<Fila[] | number | null>(null);
  const [vuelta, setVuelta] = useState(0);
  const [pendiente, setPendiente] = useState<{ fila: Fila; accion: string } | null>(null);
  const [enviando, setEnviando] = useState(false);

  useEffect(() => {
    let vivo = true;
    void cargar(servicio, estado).then((x) => {
      if (vivo) setFilas(x);
    });
    return () => {
      vivo = false;
    };
  }, [servicio, estado, vuelta]);

  async function aplicar(): Promise<void> {
    if (!pendiente) return;
    setEnviando(true);
    try {
      const res = await fetch(resolverRuta(servicio, pendiente.fila.id), {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action: pendiente.accion }),
      });
      toast.notify(
        res.ok
          ? { title: t('mod.toast.done'), tone: 'success' }
          : { title: t('mod.toast.failed'), tone: 'danger' },
      );
    } catch {
      toast.notify({ title: t('mod.toast.failed'), tone: 'danger' });
    } finally {
      setEnviando(false);
      setPendiente(null);
      setVuelta((n) => n + 1);
    }
  }

  const rotulo = (a: string): string => t(ACCION[a] ?? 'mod.action.dismiss');

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(servicio === 'cast' ? 'mod.service.cast' : 'mod.service.rooms')}</CardTitle>
        <div class="flex items-center gap-2">
          {(['open', 'resolved'] as const).map((e) => (
            <Button
              key={e}
              variant={estado === e ? 'primary' : 'secondary'}
              aria-pressed={estado === e}
              onClick={() => setEstado(e)}
            >
              {t(e === 'open' ? 'mod.status.open' : 'mod.status.resolved')}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardBody>
        {filas === null ? (
          <p class="sy-hint">{t('mod.loading')}</p>
        ) : typeof filas === 'number' ? (
          <Aviso codigo={filas} t={t} />
        ) : (
          <Table<Fila>
            rowKey={(f) => f.id}
            rows={filas}
            caption={t(servicio === 'cast' ? 'mod.service.cast' : 'mod.service.rooms')}
            emptyContent={<EmptyState title={t('mod.empty')} />}
            columns={[
              {
                key: 'when',
                header: t('mod.col.date'),
                cell: (f) => new Date(f.creada).toLocaleString(locale),
              },
              { key: 'org', header: t('mod.col.org'), cell: (f) => f.org },
              {
                key: 'what',
                header: t('mod.col.what'),
                cell: (f) => (
                  <span>
                    {t(f.tipo)}
                    {f.quien ? <span class="sy-hint block">{f.quien}</span> : null}
                  </span>
                ),
              },
              {
                key: 'reason',
                header: t('mod.col.reason'),
                cell: (f) => t(MOTIVO[f.motivo] ?? 'mod.reason.other'),
              },
              {
                key: 'content',
                header: t('mod.col.content'),
                cell: (f) => (
                  <span class="break-words whitespace-pre-wrap">
                    {f.contenido || '—'}
                    {f.detalle ? <span class="sy-hint block">{f.detalle}</span> : null}
                  </span>
                ),
              },
              {
                key: 'actions',
                header: t('mod.col.actions'),
                cell: (f) =>
                  f.acciones.length > 0 ? (
                    <div class="flex flex-wrap gap-1">
                      {f.acciones.map((a) => (
                        <Button
                          key={a}
                          variant={a === 'dismiss' || a === 'dismissed' ? 'secondary' : 'danger'}
                          onClick={() => setPendiente({ fila: f, accion: a })}
                        >
                          {rotulo(a)}
                        </Button>
                      ))}
                    </div>
                  ) : (
                    <span class="flex flex-wrap items-center gap-1">
                      {f.resolucion ? rotulo(f.resolucion) : '—'}
                      {f.desde ? (
                        <Badge tone={f.desde === 'platform' ? 'info' : 'neutral'}>
                          {t(f.desde === 'platform' ? 'mod.from.platform' : 'mod.from.org')}
                        </Badge>
                      ) : null}
                    </span>
                  ),
              },
            ]}
          />
        )}
      </CardBody>
      <ConfirmDialog
        open={pendiente !== null}
        onClose={() => setPendiente(null)}
        onConfirm={() => void aplicar()}
        title={t('mod.confirm.title', { action: pendiente ? rotulo(pendiente.accion) : '' })}
        confirmLabel={pendiente ? rotulo(pendiente.accion) : ''}
        tone="danger"
        confirmDisabled={enviando}
      >
        <p class="sy-hint">{t('mod.confirm.body', { org: pendiente?.fila.org ?? '' })}</p>
      </ConfirmDialog>
    </Card>
  );
}

function Inner({ locale, problema, t }: Props & { t: T }): JSX.Element {
  if (problema !== undefined) {
    return (
      <Card>
        <CardBody>
          <Aviso codigo={problema} t={t} />
        </CardBody>
      </Card>
    );
  }
  return (
    <>
      <p class="sy-hint mb-4">
        {t('mod.intro')}{' '}
        <a class="text-primary underline" href={r('/platform')}>
          {t('mod.suspend')}
        </a>
      </p>
      <Tabs
        items={[
          {
            id: 'cast',
            label: t('mod.service.cast'),
            content: <Cola servicio="cast" locale={locale} t={t} />,
          },
          {
            id: 'rooms',
            label: t('mod.service.rooms'),
            content: <Cola servicio="rooms" locale={locale} t={t} />,
          },
        ]}
      />
    </>
  );
}

export default function PlatformModeration(props: Props): JSX.Element {
  const t = useT(props.locale);
  return (
    <ToastProvider>
      <Inner {...props} t={t} />
    </ToastProvider>
  );
}
