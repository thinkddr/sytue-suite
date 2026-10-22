/**
 * ContactModal — crear/editar un contacto con el design system Sytue (Modal +
 * Form de @sytue/ui). Editor completo (paridad Google Contacts): N correos y N
 * teléfonos con selector de etiqueta y marca de principal, direcciones postales
 * y cumpleaños. Guarda vía BFF CardDAV (el servidor genera la vCard) y los
 * campos tipados se leen de la vCard cruda del contacto en edición para no
 * perder datos en el round-trip. docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  AvatarPicker,
  Button,
  FieldGroup,
  Input,
  Textarea,
  Select,
  Radio,
  Icon,
  Modal,
  describedBy,
  useToast,
  type ImageErrorCode,
} from '@sytue/ui';
import {
  saveContact,
  deleteContact,
  setPhoto,
  removePhoto,
  cleanUid,
  photoUrl,
  type ContactInput,
  type TypedValue,
  type PostalAddress,
} from '../client/contactsApi.js';
import { parseVcardDetails } from '../lib/vcard.js';
import { hasEmbeddedPhoto } from '../client/vcard.js';
import { AsistenteFicha } from './AsistenteFicha.js';
import type { FichaPropuesta } from '../lib/ai.js';
import type { ContactsMessageKey, T } from '../i18n/index.js';

/** Código de fallo de imagen -> clave de catálogo (mismo mapa que ContactDetail). */
const PHOTO_ERROR_KEY: Record<ImageErrorCode, ContactsMessageKey> = {
  format: 'photo.errorFormat',
  process: 'photo.errorProcess',
  tooLarge: 'photo.errorTooLarge',
  read: 'photo.errorRead',
};

export interface EditingContact {
  href: string | null;
  etag?: string | undefined;
  uid?: string | undefined;
  fullName: string;
  emails: string[];
  phones: string[];
  organization?: string | undefined;
  /** vCard cruda del contacto en edición: base para conservar campos no proyectados. */
  vcard?: string | undefined;
}

interface Props {
  /** `t` ya atada al idioma negociado en el servidor; llega desde la isla raíz. */
  t: T;
  open: boolean;
  addressBookUrl: string | null;
  editing: EditingContact | null;
  onClose: () => void;
  onSaved: () => void;
}

interface ValueRow {
  value: string;
  type: string;
}

interface AddrRow {
  type: string;
  street: string;
  city: string;
  region: string;
  postalCode: string;
  country: string;
}

/**
 * Opción de etiqueta: el VALOR es el de la vCard (no se traduce nunca, viaja al
 * servidor) y `key` es la clave de catálogo con la que se pinta.
 */
interface TypeOption {
  value: string;
  key: ContactsMessageKey;
}

/**
 * Añade valores nuevos a las filas existentes sin duplicar ni perder lo tecleado.
 * Las filas vacías se van: son el hueco que deja el formulario al crear.
 */
function fusionar(filas: ValueRow[], nuevos: string[], tipo: string): ValueRow[] {
  const utiles = filas.filter((f) => f.value.trim() !== '');
  const ya = new Set(utiles.map((f) => f.value.trim().toLowerCase()));
  const salida = [...utiles];
  for (const v of nuevos) {
    if (ya.has(v.toLowerCase())) continue;
    ya.add(v.toLowerCase());
    salida.push({ value: v, type: tipo });
  }
  return salida.length > 0 ? salida : filas;
}

const EMAIL_TYPES: TypeOption[] = [
  { value: 'home', key: 'type.home' },
  { value: 'work', key: 'type.work' },
  { value: 'other', key: 'type.other' },
];

const PHONE_TYPES: TypeOption[] = [
  { value: 'cell', key: 'type.mobile' },
  { value: 'home', key: 'type.home' },
  { value: 'work', key: 'type.work' },
  { value: 'fax', key: 'type.fax' },
  { value: 'other', key: 'type.other' },
];

// «Otra» y no «Otro»: en español el adjetivo concuerda con «dirección».
const ADDRESS_TYPES: TypeOption[] = [
  { value: 'home', key: 'type.home' },
  { value: 'work', key: 'type.work' },
  { value: 'other', key: 'type.otherAddress' },
];

/** Ajusta una etiqueta leída de la vCard al conjunto de opciones (o al fallback). */
function normalizeType(type: string, options: TypeOption[], fallback: string): string {
  return options.some((o) => o.value === type) ? type : fallback;
}

/** Índice del primer elemento marcado como principal, o 0. */
function primaryIndex(list: TypedValue[]): number {
  const i = list.findIndex((v) => v.primary);
  return i >= 0 ? i : 0;
}

/** Filas con valor -> valores tipados con exactamente un principal. */
function toDetailed(rows: ValueRow[], primary: number): TypedValue[] {
  const kept = rows.map((r, i) => ({ r, i })).filter(({ r }) => r.value.trim());
  const detailed = kept.map(({ r, i }) => ({
    value: r.value.trim(),
    type: r.type,
    primary: i === primary,
  }));
  if (detailed.length > 0 && !detailed.some((d) => d.primary)) {
    detailed[0]!.primary = true;
  }
  return detailed;
}

export function ContactModal({
  t,
  open,
  addressBookUrl,
  editing,
  onClose,
  onSaved,
}: Props): JSX.Element | null {
  const toast = useToast();
  const [fullName, setFullName] = useState('');
  const [emails, setEmails] = useState<ValueRow[]>([]);
  const [primaryEmail, setPrimaryEmail] = useState(0);
  const [phones, setPhones] = useState<ValueRow[]>([]);
  const [primaryPhone, setPrimaryPhone] = useState(0);
  const [addresses, setAddresses] = useState<AddrRow[]>([]);
  const [birthday, setBirthday] = useState('');
  // Si la vCard trae un BDAY parcial (sin año) que <input type=date> no
  // representa, birthdayManaged=false: se conserva verbatim salvo que el
  // usuario elija una fecha nueva (que pasa a gestionarlo el editor).
  const [birthdayManaged, setBirthdayManaged] = useState(true);
  const [organization, setOrganization] = useState('');
  const [note, setNote] = useState('');
  // Avatar elegido en esta sesión del formulario (predefinido o subido), pendiente
  // de aplicarse hasta guardar: al crear, el contacto no tiene UID todavía, así
  // que la foto no puede subirse hasta que saveContact() devuelva uno.
  const [photoDataUri, setPhotoDataUri] = useState<string | null>(null);
  const [photoRemoved, setPhotoRemoved] = useState(false);
  const [hasExistingPhoto, setHasExistingPhoto] = useState(false);
  // Acción en curso (guardar o eliminar): permite spinners independientes y
  // deshabilitar el resto de acciones mientras tanto.
  const [busy, setBusy] = useState<'save' | 'delete' | null>(null);
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  // Índice de la primera fila de correo con formato inválido, o null.
  const [emailErrorIdx, setEmailErrorIdx] = useState<number | null>(null);
  // Eliminación en dos pasos: el primer clic pide confirmación en el propio botón.
  const [confirmDelete, setConfirmDelete] = useState(false);

  /**
   * Vuelca lo que propuso el asistente en el formulario. NO GUARDA.
   *
   * LOS CORREOS Y TELÉFONOS SE AÑADEN, no sustituyen: quien ya tenía una fila
   * escrita a mano y pega una firma quiere las dos cosas, no perder la primera.
   * Las filas vacías —la que sale por defecto al crear— sí se descartan, que si
   * no la ficha queda con un hueco en medio.
   */
  const aplicarPropuesta = (f: FichaPropuesta): void => {
    setFullName(f.nombre);
    setNameError(undefined);
    if (f.organizacion !== undefined) setOrganization(f.organizacion);
    if (f.nota !== undefined) setNote((n) => (n.trim() === '' ? f.nota! : `${n}\n${f.nota!}`));
    setEmails((filas) => fusionar(filas, f.correos, 'work'));
    setEmailErrorIdx(null);
    setPhones((filas) => fusionar(filas, f.telefonos, 'cell'));
  };

  // Al (re)abrir, sincroniza el formulario con el contacto en edición. Los
  // campos tipados (etiquetas, principal, direcciones, cumpleaños, nota) se
  // extraen de la vCard cruda; al crear se parte de una fila vacía.
  useEffect(() => {
    if (!open) return;
    setFullName(editing?.fullName ?? '');
    const details = editing?.vcard ? parseVcardDetails(editing.vcard) : null;

    const parsedEmails =
      details?.emails ??
      (editing?.emails ?? []).map((value) => ({ value, type: 'home', primary: false }));
    const parsedPhones =
      details?.phones ??
      (editing?.phones ?? []).map((value) => ({ value, type: 'cell', primary: false }));

    setEmails(
      parsedEmails.length > 0
        ? parsedEmails.map((e) => ({
            value: e.value,
            type: normalizeType(e.type, EMAIL_TYPES, 'other'),
          }))
        : [{ value: '', type: 'home' }],
    );
    setPhones(
      parsedPhones.length > 0
        ? parsedPhones.map((p) => ({
            value: p.value,
            type: normalizeType(p.type, PHONE_TYPES, 'other'),
          }))
        : [{ value: '', type: 'cell' }],
    );
    setPrimaryEmail(primaryIndex(parsedEmails));
    setPrimaryPhone(primaryIndex(parsedPhones));
    setAddresses(
      (details?.addresses ?? []).map((a) => ({
        type: normalizeType(a.type, ADDRESS_TYPES, 'other'),
        street: a.street ?? '',
        city: a.city ?? '',
        region: a.region ?? '',
        postalCode: a.postalCode ?? '',
        country: a.country ?? '',
      })),
    );
    setBirthday(details?.birthday ?? '');
    setBirthdayManaged(details?.birthdayManaged ?? true);
    setOrganization(editing?.organization ?? '');
    setNote(details?.note ?? '');
    setNameError(undefined);
    setEmailErrorIdx(null);
    setConfirmDelete(false);
    setPhotoDataUri(null);
    setPhotoRemoved(false);
    setHasExistingPhoto(Boolean(editing?.vcard && hasEmbeddedPhoto(editing.vcard)));
  }, [open, editing]);

  const isEdit = Boolean(editing?.href);

  async function handleSave(): Promise<void> {
    if (!addressBookUrl) {
      toast.notify({ tone: 'danger', title: t('form.selectBook') });
      return;
    }
    const name = fullName.trim();
    const nameErr = name ? undefined : t('form.nameRequired');
    // Comprobación mínima de forma (usuario@dominio); la validación real la hace el servidor.
    let badEmail: number | null = null;
    emails.forEach((e, i) => {
      const v = e.value.trim();
      if (v && !/^\S+@\S+\.\S+$/.test(v) && badEmail === null) badEmail = i;
    });
    setNameError(nameErr);
    setEmailErrorIdx(badEmail);
    if (nameErr || badEmail !== null) return;

    const emailsDetailed = toDetailed(emails, primaryEmail);
    const phonesDetailed = toDetailed(phones, primaryPhone);
    const addressesOut: PostalAddress[] = addresses
      .map((a) => ({
        type: a.type,
        street: a.street.trim(),
        city: a.city.trim(),
        region: a.region.trim(),
        postalCode: a.postalCode.trim(),
        country: a.country.trim(),
      }))
      .filter((a) => a.street || a.city || a.region || a.postalCode || a.country);

    setBusy('save');
    const contactInput: ContactInput = {
      fullName: name,
      emails: emailsDetailed.map((e) => e.value),
      phones: phonesDetailed.map((p) => p.value),
      emailsDetailed,
      phonesDetailed,
      addresses: addressesOut,
      ...(birthdayManaged ? { birthday } : {}),
      ...(editing?.uid ? { uid: editing.uid } : {}),
      ...(organization.trim() ? { organization: organization.trim() } : {}),
      ...(note.trim() ? { note } : {}),
      // Al editar, conserva PHOTO/X-*… (y ADR/BDAY que el editor no gestione).
      ...(editing?.vcard ? { baseVcard: editing.vcard } : {}),
    };
    try {
      const saved = await saveContact({
        addressBookUrl,
        contactInput,
        ...(editing?.href ? { href: editing.href } : {}),
        ...(editing?.etag ? { etag: editing.etag } : {}),
      });
      // La foto va aparte porque la sube un endpoint propio (PUT /contacts/photo,
      // por UID); al crear, ese UID no existe hasta que saveContact() responde.
      // Un fallo aquí no deshace la ficha ya guardada: se avisa por separado.
      if (saved.uid && (photoDataUri || (photoRemoved && hasExistingPhoto))) {
        try {
          if (photoDataUri) await setPhoto(saved.uid, photoDataUri);
          else await removePhoto(saved.uid);
        } catch {
          toast.notify({ tone: 'danger', title: t('photo.updateFailed') });
        }
      }
      toast.notify({
        tone: 'success',
        title: isEdit ? t('form.updated') : t('form.created'),
      });
      onSaved();
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('form.saveFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(null);
    }
  }

  async function handleDelete(): Promise<void> {
    if (!editing?.href) return;
    setBusy('delete');
    try {
      await deleteContact(editing.href);
      // El borrado del servicio es SUAVE: decir solo «eliminado» hacía que la
      // gente diera por perdido lo que está a un clic en la papelera.
      toast.notify({
        tone: 'success',
        title: t('form.deleted'),
        description: t('form.deletedHint'),
      });
      onSaved();
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('form.deleteFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(null);
      setConfirmDelete(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isEdit ? t('form.editTitle') : t('form.newTitle')}
      size="md"
      dismissable={busy === null}
      footer={
        <>
          {/* «Eliminar» a la izquierda en escritorio (`md:mr-auto`); en el teléfono, sola
              arriba y Cancelar | Guardar debajo (el pie del Modal). */}
          {isEdit ? (
            <Button
              variant="danger"
              class="md:mr-auto"
              onClick={() => (confirmDelete ? void handleDelete() : setConfirmDelete(true))}
              loading={busy === 'delete'}
              disabled={busy !== null}
            >
              {confirmDelete ? t('form.confirmDelete') : t('common.delete')}
            </Button>
          ) : null}
          <Button variant="secondary" onClick={onClose} disabled={busy !== null}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => void handleSave()}
            loading={busy === 'save'}
            disabled={busy !== null}
          >
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        {/* Vista previa en vivo: refleja el nombre/organización mientras se escribe. */}
        <div class="border-border flex items-center gap-3 border-b pb-4">
          <AvatarPicker
            name={fullName.trim() || t('form.newTitle')}
            size="lg"
            hasPhoto={Boolean(photoDataUri) || (hasExistingPhoto && !photoRemoved)}
            src={
              photoRemoved
                ? undefined
                : (photoDataUri ??
                  (hasExistingPhoto && editing?.uid
                    ? photoUrl(cleanUid(editing.uid), editing.etag)
                    : undefined))
            }
            onPick={(dataUri) => {
              setPhotoDataUri(dataUri);
              setPhotoRemoved(false);
            }}
            onRemove={() => {
              setPhotoDataUri(null);
              setPhotoRemoved(true);
            }}
            onError={(description) =>
              toast.notify({ tone: 'danger', title: t('photo.updateFailed'), description })
            }
            labels={{
              change: t('photo.change'),
              add: t('photo.add'),
              presets: t('photo.presets'),
              upload: t('photo.upload'),
              remove: t('photo.remove'),
              error: (code) => t(PHOTO_ERROR_KEY[code]),
            }}
          />
          <div class="min-w-0">
            <p
              class="text-text truncate text-base font-semibold"
              title={fullName.trim() || undefined}
            >
              {fullName.trim() || t('form.newTitle')}
            </p>
            {organization.trim() ? (
              <p class="text-text-muted truncate text-sm" title={organization.trim()}>
                {organization.trim()}
              </p>
            ) : (
              <p class="text-text-subtle text-sm">{t('form.preview')}</p>
            )}
          </div>
        </div>

        {/* El asistente va ARRIBA porque es el atajo a la ficha entera: se pega
            la firma, se rellenan los campos y se revisa hacia abajo. */}
        <AsistenteFicha t={t} onPropuesta={aplicarPropuesta} />

        <FieldGroup
          label={t('form.fullName')}
          htmlFor="c-name"
          required
          {...(nameError ? { error: nameError } : {})}
        >
          <Input
            id="c-name"
            value={fullName}
            placeholder={t('form.fullNamePlaceholder')}
            autocomplete="off"
            invalid={Boolean(nameError)}
            aria-describedby={describedBy('c-name', undefined, nameError)}
            onInput={(e) => {
              setFullName(e.currentTarget.value);
              setNameError(undefined);
            }}
          />
        </FieldGroup>

        <ValueList
          t={t}
          legend={t('field.email')}
          idPrefix="c-email"
          rows={emails}
          setRows={setEmails}
          primary={primaryEmail}
          setPrimary={setPrimaryEmail}
          typeOptions={EMAIL_TYPES}
          inputType="email"
          placeholder={t('form.emailPlaceholder')}
          addLabel={t('form.addEmail')}
          typeLabelKey="form.emailTypeLabel"
          valueLabelKey="form.emailLabel"
          removeLabelKey="form.removeEmail"
          errorIndex={emailErrorIdx}
          error={emailErrorIdx !== null ? t('form.emailInvalid') : undefined}
          onEditRow={() => setEmailErrorIdx(null)}
        />

        <ValueList
          t={t}
          legend={t('field.phone')}
          idPrefix="c-phone"
          rows={phones}
          setRows={setPhones}
          primary={primaryPhone}
          setPrimary={setPrimaryPhone}
          typeOptions={PHONE_TYPES}
          inputType="tel"
          placeholder={t('form.phonePlaceholder')}
          addLabel={t('form.addPhone')}
          typeLabelKey="form.phoneTypeLabel"
          valueLabelKey="form.phoneLabel"
          removeLabelKey="form.removePhone"
        />

        <AddressList t={t} addresses={addresses} setAddresses={setAddresses} />

        <FieldGroup
          label={t('field.birthday')}
          htmlFor="c-bday"
          {...(!birthdayManaged ? { help: t('form.birthdayPartial') } : {})}
        >
          <Input
            id="c-bday"
            type="date"
            value={birthday}
            aria-describedby={birthdayManaged ? undefined : 'c-bday-help'}
            onInput={(e) => {
              setBirthday(e.currentTarget.value);
              setBirthdayManaged(true);
            }}
          />
        </FieldGroup>

        <FieldGroup label={t('field.organization')} htmlFor="c-org">
          <Input
            id="c-org"
            value={organization}
            placeholder={t('form.organizationPlaceholder')}
            autocomplete="off"
            onInput={(e) => setOrganization(e.currentTarget.value)}
          />
        </FieldGroup>

        <FieldGroup label={t('field.note')} htmlFor="c-note">
          <Textarea
            id="c-note"
            rows={3}
            value={note}
            placeholder={t('form.notePlaceholder')}
            onInput={(e) => setNote(e.currentTarget.value)}
          />
        </FieldGroup>
      </div>
    </Modal>
  );
}

/**
 * Lista editable de valores con etiqueta y principal (correos o teléfonos):
 * filas con selector de etiqueta + valor, radio «Principal» (cuando hay más de
 * una) y botón de quitar; botón para añadir filas. fieldset+legend para el
 * grupo (accesible), aria-label por control.
 */
function ValueList({
  t,
  legend,
  idPrefix,
  rows,
  setRows,
  primary,
  setPrimary,
  typeOptions,
  inputType,
  placeholder,
  addLabel,
  typeLabelKey,
  valueLabelKey,
  removeLabelKey,
  errorIndex,
  error,
  onEditRow,
}: {
  t: T;
  legend: string;
  idPrefix: string;
  rows: ValueRow[];
  setRows: (rows: ValueRow[]) => void;
  primary: number;
  setPrimary: (i: number) => void;
  typeOptions: TypeOption[];
  inputType: 'email' | 'tel';
  placeholder: string;
  addLabel: string;
  /* Etiquetas de lector de pantalla como CLAVES y no derivadas de `legend` con
     `toLowerCase()`: en alemán los sustantivos van en mayúscula y bajarlas es
     una falta de ortografía. */
  typeLabelKey: ContactsMessageKey;
  valueLabelKey: ContactsMessageKey;
  removeLabelKey: ContactsMessageKey;
  errorIndex?: number | null;
  error?: string | undefined;
  onEditRow?: (() => void) | undefined;
}): JSX.Element {
  function update(i: number, patch: Partial<ValueRow>): void {
    setRows(rows.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
    onEditRow?.();
  }
  function remove(i: number): void {
    setRows(rows.filter((_, idx) => idx !== i));
    // Reajusta el índice principal tras eliminar.
    if (primary === i) setPrimary(0);
    else if (primary > i) setPrimary(primary - 1);
    onEditRow?.();
  }
  function add(): void {
    setRows([...rows, { value: '', type: typeOptions[0]!.value }]);
  }

  return (
    <fieldset class="flex flex-col gap-2">
      <legend class="text-text mb-1 text-sm font-semibold">{legend}</legend>
      {rows.map((row, i) => (
        <div key={i} class="flex flex-wrap items-center gap-2">
          {/* `w-28`: en `w-24`, con el campo de 44 del dedo, «Trabajo» se leía «Trabaj». */}
          <div class="w-28 shrink-0">
            <Select
              aria-label={t(typeLabelKey, { index: i + 1 })}
              value={row.type}
              onChange={(e) => update(i, { type: e.currentTarget.value })}
            >
              {typeOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.key)}
                </option>
              ))}
            </Select>
          </div>
          <div class="min-w-0 grow basis-48">
            <Input
              id={`${idPrefix}-${i}`}
              type={inputType}
              value={row.value}
              placeholder={placeholder}
              autocomplete="off"
              invalid={errorIndex === i}
              aria-label={t(valueLabelKey, { index: i + 1 })}
              onInput={(e) => update(i, { value: e.currentTarget.value })}
            />
          </div>
          {rows.length > 1 ? (
            <Radio
              class="shrink-0 whitespace-nowrap"
              name={`primary-${idPrefix}`}
              label={t('form.primary')}
              checked={primary === i}
              onChange={() => setPrimary(i)}
            />
          ) : null}
          <Button
            variant="ghost"
            size="sm"
            iconOnly
            class="shrink-0"
            aria-label={t(removeLabelKey, { index: i + 1 })}
            leadingIcon={<Icon name="close" size={16} />}
            onClick={() => remove(i)}
          />
        </div>
      ))}
      {error ? (
        <p class="text-danger-text flex items-center gap-1 text-xs" role="alert">
          <Icon name="danger" size={14} />
          {error}
        </p>
      ) : null}
      <div>
        <Button
          variant="ghost"
          size="sm"
          leadingIcon={<Icon name="plus" size={14} />}
          onClick={add}
        >
          {addLabel}
        </Button>
      </div>
    </fieldset>
  );
}

/**
 * Lista editable de direcciones postales (ADR): cada dirección con selector de
 * etiqueta y campos calle/ciudad/región/CP/país; botón de quitar por bloque y
 * botón para añadir. Las direcciones son opcionales (se empieza sin ninguna).
 */
function AddressList({
  t,
  addresses,
  setAddresses,
}: {
  t: T;
  addresses: AddrRow[];
  setAddresses: (a: AddrRow[]) => void;
}): JSX.Element {
  function update(i: number, patch: Partial<AddrRow>): void {
    setAddresses(addresses.map((a, idx) => (idx === i ? { ...a, ...patch } : a)));
  }
  function remove(i: number): void {
    setAddresses(addresses.filter((_, idx) => idx !== i));
  }
  function add(): void {
    setAddresses([
      ...addresses,
      { type: 'home', street: '', city: '', region: '', postalCode: '', country: '' },
    ]);
  }

  return (
    <fieldset class="flex flex-col gap-2">
      <legend class="text-text mb-1 text-sm font-semibold">{t('field.address')}</legend>
      {addresses.map((a, i) => (
        <div key={i} class="border-border flex flex-col gap-2 rounded-md border p-3">
          <div class="flex items-center gap-2">
            <Select
              class="w-24 shrink-0"
              aria-label={t('form.addressTypeLabel', { index: i + 1 })}
              value={a.type}
              onChange={(e) => update(i, { type: e.currentTarget.value })}
            >
              {ADDRESS_TYPES.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.key)}
                </option>
              ))}
            </Select>
            <span class="grow" />
            <Button
              variant="ghost"
              size="sm"
              iconOnly
              class="shrink-0"
              aria-label={t('form.removeAddress', { index: i + 1 })}
              leadingIcon={<Icon name="close" size={16} />}
              onClick={() => remove(i)}
            />
          </div>
          <Input
            aria-label={t('form.streetLabel', { index: i + 1 })}
            placeholder={t('form.street')}
            autocomplete="off"
            value={a.street}
            onInput={(e) => update(i, { street: e.currentTarget.value })}
          />
          <div class="grid grid-cols-2 gap-2">
            <Input
              aria-label={t('form.cityLabel', { index: i + 1 })}
              placeholder={t('form.city')}
              autocomplete="off"
              value={a.city}
              onInput={(e) => update(i, { city: e.currentTarget.value })}
            />
            <Input
              aria-label={t('form.regionLabel', { index: i + 1 })}
              placeholder={t('form.region')}
              autocomplete="off"
              value={a.region}
              onInput={(e) => update(i, { region: e.currentTarget.value })}
            />
          </div>
          <div class="grid grid-cols-2 gap-2">
            <Input
              aria-label={t('form.postalCodeLabel', { index: i + 1 })}
              placeholder={t('form.postalCode')}
              autocomplete="off"
              value={a.postalCode}
              onInput={(e) => update(i, { postalCode: e.currentTarget.value })}
            />
            <Input
              aria-label={t('form.countryLabel', { index: i + 1 })}
              placeholder={t('form.country')}
              autocomplete="off"
              value={a.country}
              onInput={(e) => update(i, { country: e.currentTarget.value })}
            />
          </div>
        </div>
      ))}
      <div>
        <Button
          variant="ghost"
          size="sm"
          leadingIcon={<Icon name="plus" size={14} />}
          onClick={add}
        >
          {t('form.addAddress')}
        </Button>
      </div>
    </fieldset>
  );
}
