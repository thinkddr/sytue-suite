/**
 * NewDmModal — «Nuevo mensaje» (DM 1:1 o de grupo, paridad Mattermost/Slack). Modal
 * con selección MÚLTIPLE de miembros del team: búsqueda por nombre, chips de los
 * elegidos y una lista con checkbox. Al confirmar abre/crea el DM (POST /api/dm,
 * idempotente por CONJUNTO en el backend) y avisa al padre para seleccionarlo. El
 * nombre a mostrar lo resuelve el backend per-visor (los otros participantes).
 *
 * Y a gente de OTRA organización (ADR 0040): escribiendo su correo o su @usuario se
 * la busca en Sytue One y entra en los chips como cualquiera del equipo.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Modal, Button } from '@sytue/ui';
import { useShell } from '@sytue/shell';
import { ChatHttpError, openDirect, type Channel, type ChannelMember } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { SelectorDePersonas } from './SelectorDePersonas.js';

/** Tope de OTROS participantes (backend: 2..8 en total, contando al usuario actual). */
const MAX_OTHERS = 7;

interface Props {
  open: boolean;
  onClose: () => void;
  /** Miembros del team (sin filtrar); se excluye al propio usuario. */
  members: ChannelMember[];
  selfId: string;
  /** Llamado con el canal directo abierto/creado (para seleccionarlo). */
  onOpened: (channel: Channel) => void;
  /** Traductor ya atado al idioma. */
  t: T;
}

export function NewDmModal({
  open,
  onClose,
  members,
  selfId,
  onOpened,
  t,
}: Props): JSX.Element | null {
  const { user } = useShell();
  const esAdmin = user.roles.some((r) => r === 'owner' || r === 'admin');
  const [selected, setSelected] = useState<ChannelMember[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<boolean | 'externo'>(false);

  useEffect(() => {
    if (open) {
      setSelected([]);
      setBusy(false);
      setError(false);
    }
  }, [open]);

  function confirm(): void {
    if (selected.length === 0 || busy) return;
    setBusy(true);
    setError(false);
    openDirect(selected.map((m) => m.sub))
      // Display inmediato: nombre compuesto = nombres de los elegidos; peers marca el
      // group DM en la UI. El backend lo resuelve igual per-visor al recargar.
      .then(({ channel }) =>
        onOpened({
          ...channel,
          name: selected.map((m) => m.name).join(', '),
          peers: selected.map((m) => m.name),
        }),
      )
      .catch((e: unknown) =>
        setError(
          e instanceof ChatHttpError && e.code === 'rooms_external_closed' ? 'externo' : true,
        ),
      )
      .finally(() => setBusy(false));
  }

  if (!open) return null;

  const cta =
    selected.length > 1 ? t('dm.createGroup', { count: selected.length }) : t('dm.openChat');

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('dm.title')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('action.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={confirm}
            disabled={selected.length === 0 || busy}
            loading={busy}
          >
            {cta}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-3">
        <SelectorDePersonas
          members={members}
          selfId={selfId}
          selected={selected}
          onChange={(next) => {
            setError(false);
            setSelected(next);
          }}
          max={MAX_OTHERS}
          disabled={busy}
          canInvite={esAdmin}
          t={t}
        />
        {error ? (
          <p class="text-danger-text text-sm" role="alert">
            {error === 'externo' ? t('dm.externalClosed') : t('dm.openFailed')}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
