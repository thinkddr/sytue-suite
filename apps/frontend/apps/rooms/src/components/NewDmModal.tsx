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
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Modal, Input, Avatar, Button, Icon, cx, EmptyState, Illustration, Glifo } from '@sytue/ui';
import { useShell } from '@sytue/shell';
import { adminUrl } from '@sytue/config';
import { openDirect, type Channel, type ChannelMember, type Persona } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { pareceIdentificador } from '../lib/personas.js';
import { BuscarPersona } from './BuscarPersona.js';
import { RemovableChip } from './RemovableChip.js';
import { contiene } from '@sytue/i18n';

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

/** Marca de verificación del checkbox (14-grid, stroke 3 para nitidez a 16px). */
function CheckMark(): JSX.Element {
  return <Glifo d="M20 6 9 17l-5-5" size={12} />;
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
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState<ChannelMember[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);

  useEffect(() => {
    if (open) {
      setQuery('');
      setSelected([]);
      setBusy(false);
      setError(false);
    }
  }, [open]);

  const selectedSubs = useMemo(() => new Set(selected.map((m) => m.sub)), [selected]);
  const atCap = selected.length >= MAX_OTHERS;

  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase();
    return members
      .filter((m) => m.sub !== selfId)
      .filter((m) => !q || contiene(m.name, q))
      .slice(0, 100);
  }, [members, selfId, query]);

  /** La persona que devolvió Sytue ID: a los chips, marcada si no es del equipo. */
  function addPersona(p: Persona): void {
    setQuery('');
    if (p.sub === selfId || selectedSubs.has(p.sub)) return;
    const delEquipo = members.find((m) => m.sub === p.sub);
    toggle(delEquipo ?? { ...p, external: true });
  }

  function toggle(m: ChannelMember): void {
    setError(false);
    setSelected((prev) => {
      if (prev.some((x) => x.sub === m.sub)) return prev.filter((x) => x.sub !== m.sub);
      if (prev.length >= MAX_OTHERS) return prev; // tope alcanzado
      return [...prev, m];
    });
  }

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
      .catch(() => setError(true))
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
        {selected.length > 0 ? (
          <ul class="flex flex-wrap gap-1.5" aria-label={t('dm.selectedPeople')}>
            {selected.map((m) => (
              <li key={m.sub}>
                <RemovableChip
                  label={m.external ? t('people.externalName', { name: m.name }) : m.name}
                  ariaLabel={t('action.removePerson', { name: m.name })}
                  onRemove={() => toggle(m)}
                  leading={<Avatar name={m.name} size="xs" />}
                />
              </li>
            ))}
          </ul>
        ) : null}

        <Input
          type="search"
          value={query}
          placeholder={t('dm.searchPlaceholder')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setQuery(e.currentTarget.value)}
          aria-label={t('dm.searchLabel')}
        />

        <BuscarPersona query={query} onFound={addPersona} disabled={busy || atCap} t={t} />

        {error ? (
          <p class="text-danger-text text-sm" role="alert">
            {t('dm.openFailed')}
          </p>
        ) : null}
        {atCap ? (
          <p class="text-text-muted text-xs" role="status">
            {t('dm.maxPeople', { count: MAX_OTHERS })}
          </p>
        ) : null}

        {candidates.length === 0 && pareceIdentificador(query) ? null : candidates.length === 0 ? (
          // «No hay nadie más» sin decir qué hacer dejaba la pantalla muerta: quien
          // puede dar de alta gente (owner/admin) ve el camino; el resto, al menos
          // entiende que no es un fallo suyo.
          <EmptyState
            class="py-4"
            title={query.trim() ? t('dm.emptyQuery') : t('dm.empty')}
            illustration={
              <Illustration name={query.trim() ? 'emptySearch' : 'emptyContacts'} size={96} />
            }
            action={
              !query.trim() && esAdmin ? (
                <Button variant="secondary" size="sm" href={adminUrl('users')}>
                  {t('dm.invite')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <ul
            class="flex max-h-[40dvh] flex-col gap-0.5 overflow-y-auto"
            role="listbox"
            aria-multiselectable="true"
            aria-label={t('dm.teamPeople')}
          >
            {candidates.map((m) => {
              const checked = selectedSubs.has(m.sub);
              return (
                <li key={m.sub}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={checked}
                    disabled={busy || (!checked && atCap)}
                    onClick={() => toggle(m)}
                    class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-center gap-3 rounded-md px-2 py-2 text-left transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    <span
                      aria-hidden="true"
                      class={cx(
                        'grid h-4 w-4 shrink-0 place-items-center rounded border',
                        checked
                          ? 'bg-primary border-transparent text-[var(--sytue-on-primary)]'
                          : 'border-border',
                      )}
                    >
                      {checked ? <CheckMark /> : null}
                    </span>
                    <Avatar name={m.name} size="sm" />
                    <span class="text-text min-w-0 flex-1 truncate text-base font-medium">
                      {m.name}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Modal>
  );
}
