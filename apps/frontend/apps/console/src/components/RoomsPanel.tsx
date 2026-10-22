/**
 * RoomsPanel — administración de los canales de Sytue Rooms de la organización.
 *
 * QUÉ RESUELVE. Rooms era el único producto de la suite sin administración: lo
 * que uno ve son los canales públicos y aquellos de los que es miembro, así que
 * un canal privado archivado por error no lo podía deshacer nadie de fuera, y no
 * había forma de responder «cuántos canales hay» sin abrir la base de datos.
 *
 * Los MENSAJES DIRECTOS no salen aquí, y no es un olvido: enseñarían con quién
 * habla cada cual —y cuándo— sin abrir un solo mensaje. Lo que un litigio
 * necesita se pide por Cumplimiento, con su retención legal y su traza de acceso.
 *
 * Y lo único que se puede cambiar es archivar/restaurar. El nombre, el tema y el
 * propósito los lleva el equipo dueño del canal, que es quien sabe de qué va.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  Input,
  SectionHeader,
  Switch,
  Table,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { AdminChannel, RoomsStats } from '../lib/rooms.js';
import { formatWhen } from '../lib/format.js';
import { contiene } from '@sytue/i18n';

interface Props {
  locale: Locale;
  channels: AdminChannel[];
  stats: RoomsStats | null;
  /** Motivo por el que no hay datos: 403 sin rol, 502 si Rooms no contesta. */
  problema?: number | undefined;
}

/** Una cifra del resumen. Se pinta aunque valga 0: «ninguno» también es dato. */
function Cifra({ valor, etiqueta }: { valor: number; etiqueta: string }) {
  return (
    <div class="border-border bg-surface-raised rounded-lg border px-4 py-3">
      <div class="text-2xl font-semibold tabular-nums">{valor}</div>
      <div class="text-text-muted text-xs">{etiqueta}</div>
    </div>
  );
}

function Inner({ channels, stats, problema, locale, t }: Props & { t: T }) {
  const toast = useToast();
  const [items, setItems] = useState(channels);
  const [verArchivados, setVerArchivados] = useState(false);
  const [filtro, setFiltro] = useState('');
  const [guardando, setGuardando] = useState<string | null>(null);

  // El filtro mira nombre, slug y equipo: quien busca «ventas» no sabe si es el
  // nombre del canal o el del equipo, y probar las dos cosas es trabajo suyo.
  const visibles = useMemo(() => {
    const q = filtro.trim().toLowerCase();
    return items.filter((c) => {
      if (!verArchivados && c.archived) return false;
      if (!q) return true;
      return contiene(c.name, q) || contiene(c.slug, q) || contiene(c.team_name, q);
    });
  }, [items, filtro, verArchivados]);

  if (problema !== undefined) {
    return (
      <Card>
        <CardBody>
          <EmptyState
            title={t(problema === 403 ? 'rooms.error.forbidden' : 'rooms.error.unavailable')}
            description={t(
              problema === 403 ? 'rooms.error.forbiddenBody' : 'rooms.error.unavailableBody',
            )}
          />
        </CardBody>
      </Card>
    );
  }

  async function alternarArchivado(c: AdminChannel) {
    setGuardando(c.id);
    const res = await fetch('/api/admin/rooms-channels', {
      method: 'PATCH',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id: c.id, archived: !c.archived }),
    });
    setGuardando(null);
    if (!res.ok) {
      toast.notify({ title: t('rooms.toast.failed'), tone: 'danger' });
      return;
    }
    // Se actualiza en memoria en vez de recargar: la lista puede ser larga y
    // volver a pedirla mueve el scroll justo después de tocar una fila.
    setItems((prev) => prev.map((x) => (x.id === c.id ? { ...x, archived: !c.archived } : x)));
    toast.notify({
      title: t(c.archived ? 'rooms.toast.restored' : 'rooms.toast.archived'),
      tone: 'success',
    });
  }

  return (
    <div class="flex flex-col gap-6">
      {stats && (
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Cifra valor={stats.channels} etiqueta={t('rooms.stat.channels')} />
          <Cifra valor={stats.private} etiqueta={t('rooms.stat.private')} />
          <Cifra valor={stats.archived} etiqueta={t('rooms.stat.archived')} />
          <Cifra valor={stats.teams} etiqueta={t('rooms.stat.teams')} />
        </div>
      )}

      <Card>
        <CardBody>
          <SectionHeader
            icon={<Icon name="users" size={20} />}
            title={t('rooms.channels.title')}
            subtitle={t('rooms.channels.subtitle')}
          />
          <div class="mt-4 mb-4 flex flex-wrap items-center gap-4">
            <Input
              value={filtro}
              placeholder={t('rooms.filter.placeholder')}
              aria-label={t('rooms.filter.placeholder')}
              class="max-w-xs"
              onInput={(e) => setFiltro((e.target as HTMLInputElement).value)}
            />
            <Switch
              label={t('rooms.showArchived')}
              checked={verArchivados}
              onChange={() => setVerArchivados((v) => !v)}
            />
          </div>

          {visibles.length === 0 ? (
            <EmptyState title={t('rooms.empty')} description={t('rooms.emptyBody')} />
          ) : (
            <Table<AdminChannel>
              rowKey={(c) => c.id}
              rows={visibles}
              caption={t('rooms.channels.title')}
              columns={[
                {
                  key: 'name',
                  header: t('rooms.col.channel'),
                  cell: (c) => (
                    <div class="flex flex-col">
                      <span class="font-medium">
                        {c.kind === 'private' ? '🔒 ' : '# '}
                        {c.name}
                      </span>
                      <span class="text-text-muted text-xs">{c.team_name}</span>
                    </div>
                  ),
                },
                {
                  key: 'kind',
                  header: t('rooms.col.kind'),
                  cell: (c) => (
                    <Badge tone={c.kind === 'private' ? 'warning' : 'neutral'}>
                      {t(c.kind === 'private' ? 'rooms.kind.private' : 'rooms.kind.public')}
                    </Badge>
                  ),
                },
                {
                  key: 'members',
                  header: t('rooms.col.members'),
                  cell: (c) => <span class="tabular-nums">{c.members}</span>,
                },
                {
                  key: 'activity',
                  header: t('rooms.col.lastActivity'),
                  cell: (c) => (
                    <span class="text-text-muted text-sm">
                      {formatWhen(locale, c.last_message_at)}
                    </span>
                  ),
                },
                {
                  key: 'state',
                  header: t('rooms.col.state'),
                  cell: (c) =>
                    c.archived ? (
                      <Badge tone="neutral">{t('rooms.state.archived')}</Badge>
                    ) : (
                      <Badge tone="success">{t('rooms.state.active')}</Badge>
                    ),
                },
                {
                  key: 'actions',
                  header: '',
                  cell: (c) => (
                    <Button
                      variant="ghost"
                      disabled={guardando === c.id}
                      onClick={() => void alternarArchivado(c)}
                    >
                      {t(c.archived ? 'rooms.action.restore' : 'rooms.action.archive')}
                    </Button>
                  ),
                },
              ]}
            />
          )}
        </CardBody>
      </Card>
    </div>
  );
}

export default function RoomsPanel(props: Props) {
  const t = useT(props.locale);
  return (
    <ToastProvider>
      <Inner {...props} t={t} />
    </ToastProvider>
  );
}
