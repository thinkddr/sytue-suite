/**
 * SelectorDePersonas — elegir VARIAS personas: las elegidas en chips que se quitan, una
 * caja que filtra a la gente del equipo por nombre, la fila «Buscar en Sytue One» cuando
 * lo tecleado es un correo o un @usuario (ADR 0040) y la lista con casilla.
 *
 * Nació dentro de «Nuevo mensaje» (NewDmModal) y salió de ahí cuando «Crear canal»
 * necesitó lo mismo para nacer ya con personas: un solo selector para que las dos
 * puertas se comporten igual (tope, gente de fuera, búsqueda sin acentos).
 *
 * La consulta es estado propio: el padre solo sabe a quién se ha elegido.
 */
import type { JSX } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import { Input, Avatar, Button, Icon, cx, EmptyState, Glifo } from '@sytue/ui';
import { adminUrl } from '@sytue/config';
import { contiene } from '@sytue/i18n';
import type { ChannelMember, Persona } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { pareceIdentificador } from '../lib/personas.js';
import { BuscarPersona } from './BuscarPersona.js';
import { RemovableChip } from './RemovableChip.js';

interface Props {
  /** Miembros del team (sin filtrar); se excluye al propio usuario. */
  members: ChannelMember[];
  selfId: string;
  selected: ChannelMember[];
  onChange: (next: ChannelMember[]) => void;
  /** Cuántas personas se pueden elegir como mucho. */
  max: number;
  disabled?: boolean;
  /** Para asociar la caja de búsqueda a una etiqueta (FieldGroup) y a su ayuda. */
  inputId?: string;
  describedBy?: string | undefined;
  /**
   * Cómo se dice que no hay nadie: 'ilustrado' (el estado vacío grande, con «Invitar
   * personas» si `canInvite`) cuando el selector ES el diálogo; 'texto' (una línea)
   * cuando es un campo más de un formulario y la ilustración se comería el diálogo.
   */
  empty?: 'ilustrado' | 'texto';
  /** ¿Puede quien mira dar de alta gente (owner/admin)? Enseña el camino al vacío. */
  canInvite?: boolean;
  /** Traductor ya atado al idioma. */
  t: T;
}

/** Marca de verificación del checkbox (14-grid, stroke 3 para nitidez a 16px). */
function CheckMark(): JSX.Element {
  return <Glifo d="M20 6 9 17l-5-5" size={12} />;
}

export function SelectorDePersonas({
  members,
  selfId,
  selected,
  onChange,
  max,
  disabled = false,
  inputId,
  describedBy,
  empty = 'ilustrado',
  canInvite = false,
  t,
}: Props): JSX.Element {
  const [query, setQuery] = useState('');

  const selectedSubs = useMemo(() => new Set(selected.map((m) => m.sub)), [selected]);
  const atCap = selected.length >= max;

  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase();
    return members
      .filter((m) => m.sub !== selfId)
      .filter((m) => !q || contiene(m.name, q))
      .slice(0, 100);
  }, [members, selfId, query]);

  function toggle(m: ChannelMember): void {
    if (selected.some((x) => x.sub === m.sub)) {
      onChange(selected.filter((x) => x.sub !== m.sub));
    } else if (selected.length < max) {
      onChange([...selected, m]);
    }
  }

  /** La persona que devolvió Sytue ID: a los chips, marcada si no es del equipo. */
  function addPersona(p: Persona): void {
    setQuery('');
    if (p.sub === selfId || selectedSubs.has(p.sub)) return;
    const delEquipo = members.find((m) => m.sub === p.sub);
    toggle(delEquipo ?? { ...p, external: true });
  }

  const emptyTitle = query.trim() ? t('dm.emptyQuery') : t('dm.empty');

  return (
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
        {...(inputId ? { id: inputId } : { 'aria-label': t('dm.searchLabel') })}
        {...(describedBy ? { 'aria-describedby': describedBy } : {})}
        value={query}
        placeholder={t('dm.searchPlaceholder')}
        leadingIcon={<Icon name="search" size={16} />}
        onInput={(e) => setQuery(e.currentTarget.value)}
      />

      <BuscarPersona query={query} onFound={addPersona} disabled={disabled || atCap} t={t} />

      {atCap ? (
        <p class="text-text-muted text-xs" role="status">
          {t('dm.maxPeople', { count: max })}
        </p>
      ) : null}

      {candidates.length === 0 && pareceIdentificador(query) ? null : candidates.length === 0 ? (
        empty === 'texto' ? (
          <p class="text-text-muted px-2 text-sm">{emptyTitle}</p>
        ) : (
          // «No hay nadie más» sin decir qué hacer dejaba la pantalla muerta: quien
          // puede dar de alta gente (owner/admin) ve el camino; el resto, al menos
          // entiende que no es un fallo suyo.
          <EmptyState
            class="py-4"
            title={emptyTitle}
            spot={query.trim() ? 'emptySearch' : 'emptyContacts'}
            action={
              !query.trim() && canInvite ? (
                <Button variant="secondary" size="sm" href={adminUrl('users')}>
                  {t('dm.invite')}
                </Button>
              ) : undefined
            }
          />
        )
      ) : (
        <ul
          class={cx(
            'flex flex-col gap-0.5 overflow-y-auto',
            empty === 'texto' ? 'max-h-[30dvh]' : 'max-h-[40dvh]',
          )}
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
                  disabled={disabled || (!checked && atCap)}
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
  );
}
