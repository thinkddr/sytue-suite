/**
 * ResourcesModal — gestión de salas y recursos reservables (alta/edición/baja). Para
 * administradores del tenant: el servicio Go verifica el rol (alta) y que seas gestor
 * (edición/baja). Cada recurso tiene su propio calendario de reservas; el anti
 * doble-reserva y la disponibilidad viven en Go. Reservar un recurso se hace desde el
 * EventModal (aquí solo se gestiona el catálogo).
 *
 * Sobrio (design system Sytue: Modal + FieldGroup + Input/Select/Switch de @sytue/ui,
 * hairlines, sin degradados/emojis). Accesible: foco/teclado del Modal, errores por
 * aria-describedby, el tipo y el estado se muestran con TEXTO además del tono (WCAG AA).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  EmptyState,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  Modal,
  SettingsSection,
  Switch,
  useToast,
} from '@sytue/ui';
import {
  createResource,
  deleteResource,
  listResources,
  updateResource,
  type Resource,
  type ResourceKind,
} from '../client/calendarApi.js';
import type { CalendarMessageKey, T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  open: boolean;
  onClose: () => void;
}

const KIND_LABEL: Record<ResourceKind, CalendarMessageKey> = {
  room: 'resources.kindRoom',
  equipment: 'resources.kindEquipment',
};

interface FormState {
  id: string | null; // null = alta; id = edición
  kind: ResourceKind;
  name: string;
  capacity: string;
  location: string;
  email: string;
  autoAccept: boolean;
}

const EMPTY_FORM: FormState = {
  id: null,
  kind: 'room',
  name: '',
  capacity: '',
  location: '',
  email: '',
  autoAccept: true,
};

export function ResourcesModal({ t, open, onClose }: Props): JSX.Element {
  const toast = useToast();
  const [resources, setResources] = useState<Resource[]>([]);
  const [loading, setLoading] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [nameError, setNameError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setForm(EMPTY_FORM);
    setNameError(null);
    setLoading(true);
    let stale = false;
    void listResources()
      .then(({ resources: list }) => {
        if (!stale) setResources(list ?? []);
      })
      .catch(() => {
        if (!stale) toast.notify({ tone: 'danger', title: t('toast.resourcesLoadFailed') });
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [open, t, toast]);

  async function reload(): Promise<void> {
    const { resources: list } = await listResources();
    setResources(list ?? []);
  }

  function edit(r: Resource): void {
    setForm({
      id: r.id,
      kind: r.kind,
      name: r.name,
      capacity: r.capacity > 0 ? String(r.capacity) : '',
      location: r.location ?? '',
      email: r.email ?? '',
      autoAccept: r.auto_accept,
    });
    setNameError(null);
  }

  async function save(): Promise<void> {
    if (!form.name.trim()) {
      setNameError(t('resources.nameRequired'));
      document.getElementById('res-name')?.focus();
      return;
    }
    setBusy(true);
    const capacity = Math.max(0, parseInt(form.capacity, 10) || 0);
    try {
      if (form.id) {
        await updateResource(form.id, {
          name: form.name.trim(),
          capacity,
          location: form.location.trim(),
          email: form.email.trim(),
          auto_accept: form.autoAccept,
        });
        toast.notify({ tone: 'success', title: t('toast.resourceUpdated') });
      } else {
        await createResource({
          kind: form.kind,
          name: form.name.trim(),
          capacity,
          location: form.location.trim(),
          email: form.email.trim(),
          auto_accept: form.autoAccept,
        });
        toast.notify({ tone: 'success', title: t('toast.resourceCreated') });
      }
      setForm(EMPTY_FORM);
      await reload();
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.resourceSaveFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function remove(r: Resource): Promise<void> {
    try {
      await deleteResource(r.id);
      setResources((prev) => prev.filter((x) => x.id !== r.id));
      if (form.id === r.id) setForm(EMPTY_FORM);
      toast.notify({ tone: 'success', title: t('toast.resourceRemoved') });
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.resourceRemoveFailed') });
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('resources.title')}
      size="lg"
      footer={
        <Button variant="secondary" onClick={onClose}>
          {t('common.done')}
        </Button>
      }
    >
      <div class="flex flex-col gap-6">
        <InlineHelp tone="info" title={t('resources.helpTitle')}>
          {t('resources.helpBody')}
        </InlineHelp>

        {/* Alta / edición */}
        <SettingsSection
          plain
          title={t(form.id ? 'resources.formEditTitle' : 'resources.formNewTitle')}
          description={t(form.id ? 'resources.formEditDesc' : 'resources.formNewDesc')}
        >
          {form.id ? (
            // El tipo no se puede cambiar tras el alta (antes: un desplegable
            // deshabilitado); se muestra para que se sepa qué se está editando.
            <p class="text-text-muted flex items-center gap-2 text-sm">
              <Icon name={form.kind === 'room' ? 'building' : 'grid'} size={16} />
              {t('resources.kindFixed', { kind: t(KIND_LABEL[form.kind]) })}
            </p>
          ) : (
            <fieldset class="m-0 flex flex-col gap-2 border-0 p-0">
              <legend class="text-text mb-1 p-0 text-sm font-semibold">
                {t('resources.legend')}
              </legend>
              <ChoiceCard
                dense
                name="res-kind"
                value="room"
                title={t('resources.roomTitle')}
                description={t('resources.roomDesc')}
                icon="building"
                selected={form.kind === 'room'}
                onSelect={() => setForm((f) => ({ ...f, kind: 'room' }))}
              />
              <ChoiceCard
                dense
                name="res-kind"
                value="equipment"
                title={t('resources.equipmentTitle')}
                description={t('resources.equipmentDesc')}
                icon="grid"
                selected={form.kind === 'equipment'}
                onSelect={() => setForm((f) => ({ ...f, kind: 'equipment' }))}
              />
            </fieldset>
          )}
          {/* El error va en el FieldHint (mismo sitio que la ayuda), no duplicado. */}
          <FieldGroup label={t('resources.name')} htmlFor="res-name">
            <Input
              id="res-name"
              value={form.name}
              placeholder={t('resources.namePlaceholder')}
              invalid={Boolean(nameError)}
              aria-describedby="res-name-hint"
              onInput={(e) => {
                setForm((f) => ({ ...f, name: e.currentTarget.value }));
                if (nameError) setNameError(null);
              }}
            />
            <FieldHint id="res-name-hint" tone={nameError ? 'danger' : 'neutral'}>
              {nameError ?? t('resources.nameHint')}
            </FieldHint>
          </FieldGroup>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FieldGroup label={t('resources.capacity')} htmlFor="res-capacity">
              <Input
                id="res-capacity"
                type="number"
                min={0}
                value={form.capacity}
                placeholder={t('resources.capacityPlaceholder')}
                aria-describedby="res-capacity-hint"
                onInput={(e) => setForm((f) => ({ ...f, capacity: e.currentTarget.value }))}
              />
              <FieldHint id="res-capacity-hint">{t('resources.capacityHint')}</FieldHint>
            </FieldGroup>
            <FieldGroup label={t('resources.location')} htmlFor="res-location">
              <Input
                id="res-location"
                value={form.location}
                placeholder={t('resources.locationPlaceholder')}
                aria-describedby="res-location-hint"
                onInput={(e) => setForm((f) => ({ ...f, location: e.currentTarget.value }))}
              />
              <FieldHint id="res-location-hint">{t('resources.locationHint')}</FieldHint>
            </FieldGroup>
          </div>
          <FieldGroup label={t('resources.email')} htmlFor="res-email">
            <Input
              id="res-email"
              type="email"
              value={form.email}
              placeholder={t('resources.emailPlaceholder')}
              autoComplete="off"
              aria-describedby="res-email-hint"
              onInput={(e) => setForm((f) => ({ ...f, email: e.currentTarget.value }))}
            />
            <FieldHint id="res-email-hint">{t('resources.emailHint')}</FieldHint>
          </FieldGroup>
          <div class="border-border rounded-md border px-3 py-2.5">
            <Switch
              checked={form.autoAccept}
              onChange={() => setForm((f) => ({ ...f, autoAccept: !f.autoAccept }))}
              label={t('resources.autoAccept')}
            />
            <p class="text-text-muted mt-1 text-xs leading-snug">
              {t(form.autoAccept ? 'resources.autoAcceptOn' : 'resources.autoAcceptOff')}
            </p>
          </div>
          <div class="flex justify-end gap-2">
            {form.id ? (
              <Button variant="secondary" onClick={() => setForm(EMPTY_FORM)}>
                {t('common.cancel')}
              </Button>
            ) : null}
            <Button variant="primary" onClick={() => void save()} loading={busy}>
              {t(form.id ? 'common.save' : 'resources.create')}
            </Button>
          </div>
        </SettingsSection>

        {/* Catálogo */}
        <SettingsSection
          plain
          title={t('resources.catalogueTitle')}
          description={t('resources.catalogueDesc')}
          class="border-border border-t pt-5"
        >
          {loading ? (
            <p class="text-text-muted text-sm">{t('common.loading')}</p>
          ) : resources.length === 0 ? (
            <EmptyState
              title={t('resources.emptyTitle')}
              description={t('resources.emptyDescription')}
              illustration={<Icon name="building" size={32} />}
              action={
                <Button
                  variant="secondary"
                  onClick={() => document.getElementById('res-name')?.focus()}
                >
                  {t('resources.emptyAction')}
                </Button>
              }
            />
          ) : (
            <ul class="flex flex-col gap-1">
              {resources.map((r) => (
                <li
                  key={r.id}
                  class="border-border flex items-center gap-2 rounded-md border px-2.5 py-2"
                >
                  <Icon
                    name={r.kind === 'room' ? 'building' : 'grid'}
                    size={16}
                    class="text-text-muted shrink-0"
                  />
                  {/* El nombre y, debajo, aforo y ubicación: en una sola línea, en el
                      teléfono quedaban 145 px y se perdía todo lo que no era el nombre. */}
                  <span class="flex min-w-0 flex-1 flex-col">
                    <span class="text-text truncate text-sm">{r.name}</span>
                    {r.capacity > 0 || r.location ? (
                      <span class="text-text-muted truncate text-xs">
                        {[
                          r.capacity > 0 ? t('resources.people', { count: r.capacity }) : '',
                          r.location,
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                      </span>
                    ) : null}
                  </span>
                  <Badge tone="neutral" class="shrink-0">
                    {t(KIND_LABEL[r.kind])}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="sm"
                    iconOnly
                    aria-label={t('resources.edit', { name: r.name })}
                    title={t('common.edit')}
                    onClick={() => edit(r)}
                    class="shrink-0"
                    leadingIcon={<Icon name="settings" size={16} />}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    iconOnly
                    aria-label={t('resources.remove', { name: r.name })}
                    title={t('resources.removeTitle')}
                    onClick={() => void remove(r)}
                    class="shrink-0"
                    leadingIcon={<Icon name="close" size={16} />}
                  />
                </li>
              ))}
            </ul>
          )}
        </SettingsSection>
      </div>
    </Modal>
  );
}
