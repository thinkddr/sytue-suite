/**
 * ContactDetail — ficha de un contacto con el design system Sytue (Card, Avatar,
 * Button). Cabecera con identidad + acciones y tarjeta de datos ordenados.
 * Acciones cross-app: "Enviar correo" abre Sytue Mail con el destinatario
 * precargado (deep link). docs/design.md §9, integración §11.
 */
import type { JSX } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import {
  Avatar,
  AvatarPicker,
  Button,
  Card,
  CardHeader,
  CardTitle,
  Dropdown,
  EmptyState,
  Icon,
  MenuItem,
  useToast,
  Illustration,
  type ImageErrorCode,
  Glifo,
} from '@sytue/ui';
import { appUrl } from '@sytue/config';
import { intlTag, type Locale } from '@sytue/i18n';
import type { ContactsMessageKey, T } from '../i18n/index.js';
import {
  cleanUid,
  photoUrl,
  removePhoto,
  setPhoto,
  type Contact,
  type ContactGroup,
} from '../client/contactsApi.js';
import { parseVcardDetails, type PostalAddress, type TypedValue } from '../lib/vcard.js';
import { downloadVcf, exportFilename, hasEmbeddedPhoto } from '../client/vcard.js';

/** Código de fallo de imagen -> clave de catálogo. */
const PHOTO_ERROR_KEY: Record<ImageErrorCode, ContactsMessageKey> = {
  format: 'photo.errorFormat',
  process: 'photo.errorProcess',
  tooLarge: 'photo.errorTooLarge',
  read: 'photo.errorRead',
};

/** Etiqueta TYPE de la vCard -> clave de catálogo (las mismas que ofrece el editor). */
const TYPE_KEY: Record<string, ContactsMessageKey> = {
  home: 'type.home',
  work: 'type.work',
  cell: 'type.mobile',
  mobile: 'type.mobile',
  fax: 'type.fax',
  other: 'type.other',
};

/**
 * Rótulo de la etiqueta de un correo/teléfono/dirección. Un TYPE que no está en
 * el conjunto del editor (otros clientes CardDAV escriben «iPhone», «school»…)
 * se muestra TAL CUAL: inventarle una traducción sería mentir sobre el dato.
 */
function typeLabel(t: T, type: string, isAddress = false): string {
  const key = TYPE_KEY[type.toLowerCase()];
  if (isAddress && key === 'type.other') return t('type.otherAddress');
  return key ? t(key) : type;
}

/** Principal primero (orden estable): es el que abre «Enviar correo». */
function primaryFirst<V extends { primary?: boolean | undefined }>(list: V[]): V[] {
  return [...list].sort((a, b) => Number(b.primary ?? false) - Number(a.primary ?? false));
}

/** Dirección en líneas al estilo postal: calle / CP + ciudad / región / país. */
function addressLines(a: PostalAddress): string[] {
  return [a.street, [a.postalCode, a.city].filter(Boolean).join(' '), a.region, a.country]
    .map((line) => (line ?? '').trim())
    .filter(Boolean);
}

/**
 * Cumpleaños en el formato del idioma. La fecha se construye por partes y no con
 * `new Date('1990-05-04')`, que es medianoche UTC: al oeste de Greenwich eso se
 * pinta como el día 3.
 */
function formatBirthday(iso: string, locale: Locale): string {
  const [y, m, d] = iso.split('-').map(Number);
  if (!y || !m || !d) return iso;
  return new Intl.DateTimeFormat(intlTag(locale), {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(new Date(y, m - 1, d));
}

interface Props {
  /** `t` ya atada al idioma negociado en el servidor; llega desde la isla raíz. */
  t: T;
  /** Idioma negociado: formatea la fecha del cumpleaños. */
  locale: Locale;
  contact: Contact | null;
  /** Etiquetas (grupos) de la libreta activa. */
  groups: ContactGroup[];
  /** Asigna/quita la etiqueta al contacto; rechaza si el servidor falla. */
  onToggleGroup?: ((group: ContactGroup, contact: Contact) => Promise<void>) | undefined;
  onEdit?: ((contact: Contact) => void) | undefined;
  /** Abre el flujo «Combinar con…» (fusión de duplicados de la misma libreta);
   *  ausente si la libreta no tiene otro contacto con el que combinar. */
  onMerge?: ((contact: Contact) => void) | undefined;
  /** Abre el modal de creación desde el estado vacío (ausente si no hay libreta activa). */
  onCreate?: (() => void) | undefined;
  /** Si el contacto está destacado (para pintar la estrella). */
  isFavorite?: boolean;
  /** Destaca/quita el destacado del contacto; rechaza si el servidor falla. */
  onToggleFavorite?: ((contact: Contact) => Promise<void>) | undefined;
  /** Recarga la lista tras cambiar la foto (refresca vCard/etag del contacto). */
  onChanged?: (() => void) | undefined;
  /** Sin red la ficha es explícitamente de consulta: no ofrece mutaciones que fallarían. */
  readOnly?: boolean;
  /**
   * La organización activa de la sesión (`user.tenant`): encabeza los enlaces a
   * Mail y Calendar (ADR 0056) para que aterricen canónicos, sin el salto del
   * middleware. Ausente, salen sin org y la añade el 302 de la app de destino.
   */
  org?: string | undefined;
}

/* Enlaces de la ficha (mailto/tel): subrayado en hover y anillo de foco visible
   (AA), mismo patrón de anillo que los controles del design system. */
const linkClass =
  'break-words rounded-sm underline-offset-2 hover:underline focus-visible:outline-none ' +
  'sytue-focus-ring-inset';

export function ContactDetail({
  t,
  locale,
  contact,
  groups,
  onToggleGroup,
  onEdit,
  onMerge,
  onCreate,
  isFavorite = false,
  onToggleFavorite,
  onChanged,
  readOnly = false,
  org,
}: Props): JSX.Element {
  const toast = useToast();
  // Dirección, cumpleaños, nota y la etiqueta de cada correo/teléfono viven en la
  // vCard cruda: el proyectado del SDK solo trae los valores planos, así que sin
  // leerla aquí la ficha no enseñaba nada de lo que el editor sí deja escribir.
  const details = useMemo(
    () => (contact ? parseVcardDetails(contact.vcard) : null),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [contact?.vcard],
  );

  if (!contact || !details) {
    return (
      // Icono del DS en lugar de la ilustración SVG externa: dentro de <img>
      // sus var(--sytue-*) no se resuelven y quedaba invisible.
      <EmptyState
        class="h-full"
        title={t('detail.emptyTitle')}
        description={t('detail.emptyDescription')}
        illustration={<Illustration name="emptyContacts" />}
        action={
          onCreate ? (
            <Button
              variant="primary"
              size="sm"
              leadingIcon={<Icon name="plus" />}
              onClick={onCreate}
            >
              {t('nav.newContact')}
            </Button>
          ) : undefined
        }
      />
    );
  }

  // Copiar al portapapeles sin traerse una utilidad entera: `writeText` es lo
  // único que hace falta, y si el navegador lo niega (contexto no seguro) se
  // dice, en vez de fallar en silencio.
  const copiar = async (texto: string): Promise<void> => {
    try {
      await navigator.clipboard.writeText(texto);
      toast.notify({ tone: 'success', title: t('detail.emailCopied') });
    } catch {
      toast.notify({ tone: 'danger', title: t('detail.copyFailed') });
    }
  };

  const emails = primaryFirst(details.emails);
  const phones = primaryFirst(details.phones);
  const primaryEmail = emails[0]?.value ?? contact.emails[0];
  // `appUrl` y no `appDomains` a pelo: bajo el host de la organización cada app
  // vive tras su prefijo, y la raíz DESNUDA redirige a él PERDIENDO la query.
  // Medido en producción el 2026-09-09: `mail.sytue.eu/?to=…` acababa en
  // `mail.sytue.eu/mail` sin destinatario, así que «Enviar correo» abría el
  // redactor vacío. Con la org de la sesión (ADR 0056) la URL sale canónica y
  // aterriza sin el 302 que de otro modo añade el middleware de la app destino.
  const mailHref = primaryEmail
    ? `${appUrl('mail', '', org)}/?to=${encodeURIComponent(primaryEmail)}`
    : undefined;
  /**
   * «Crear una reunión»: abre Sytue Calendar con esta persona ya invitada y el
   * asunto puesto. Es el intent de creación que Calendar ya sabe leer
   * (`?action=create&title=&attendees=`), el mismo que usa el Hub; sin él había
   * que copiar la dirección a mano y buscarla otra vez en el otro lado.
   */
  const reunionHref = primaryEmail
    ? `${appUrl('calendar', '', org)}/?action=create&attendees=${encodeURIComponent(primaryEmail)}` +
      `&title=${encodeURIComponent(t('detail.meetingTitle', { name: contact.fullName }))}`
    : undefined;

  return (
    <div class="mx-auto w-full max-w-3xl p-4 sm:p-6 lg:p-8">
      {/* Cabecera de identidad: avatar + nombre + organización + acciones. */}
      <header class="flex flex-col gap-4 sm:flex-row sm:items-center">
        <ContactPhoto t={t} contact={contact} onChanged={onChanged} editable={!readOnly} />
        <div class="min-w-0 flex-1">
          {/* h2: el h1 de la pagina es la cabecera de la lista (ContactsApp). */}
          <h2 class="sytue-h2 text-text truncate" title={contact.fullName}>
            {contact.fullName}
          </h2>
          {contact.organization ? (
            <p class="text-text-muted mt-0.5 truncate text-base" title={contact.organization}>
              {contact.organization}
            </p>
          ) : null}
        </div>
        {/* Dos acciones frecuentes con nombre y el resto en un menú «⋯»: cuatro
            botones de texto seguidos ya envolvían a dos líneas en cuanto la
            etiqueta crecía («Zusammenführen mit…»), y competían todos entre sí. */}
        <div class="flex shrink-0 items-center gap-2">
          {onToggleFavorite ? (
            <FavoriteToggle
              t={t}
              contact={contact}
              active={isFavorite}
              onToggle={onToggleFavorite}
            />
          ) : null}
          {mailHref ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => window.open(mailHref, '_blank', 'noopener')}
            >
              {t('detail.sendEmail')}
            </Button>
          ) : null}
          {onEdit ? (
            <Button variant="primary" size="sm" onClick={() => onEdit(contact)}>
              {t('common.edit')}
            </Button>
          ) : null}
          <Dropdown
            label={t('detail.moreActions')}
            align="end"
            trigger={({ onClick, ...aria }) => (
              <Button
                {...aria}
                onClick={onClick}
                variant="ghost"
                size="sm"
                iconOnly
                aria-label={t('detail.moreActions')}
                title={t('detail.moreActions')}
                leadingIcon={<Icon name="ellipsis" size={16} />}
              />
            )}
          >
            {(close) => (
              <>
                {reunionHref ? (
                  <MenuItem
                    onClick={() => {
                      close();
                      window.open(reunionHref, '_blank', 'noopener');
                    }}
                  >
                    {t('detail.meeting')}
                  </MenuItem>
                ) : null}
                {primaryEmail ? (
                  <MenuItem
                    onClick={() => {
                      close();
                      void copiar(primaryEmail);
                    }}
                  >
                    {t('detail.copyEmail')}
                  </MenuItem>
                ) : null}
                <MenuItem
                  onClick={() => {
                    close();
                    downloadVcf(exportFilename(contact.fullName), [contact.vcard]);
                  }}
                >
                  {t('detail.export')}
                </MenuItem>
                {onMerge ? (
                  <MenuItem
                    onClick={() => {
                      close();
                      onMerge(contact);
                    }}
                  >
                    {t('detail.merge')}
                  </MenuItem>
                ) : null}
              </>
            )}
          </Dropdown>
        </div>
      </header>

      <Card class="mt-6">
        <CardHeader>
          <CardTitle>{t('detail.infoTitle')}</CardTitle>
        </CardHeader>
        <dl class="divide-border divide-y">
          <Field label={t('field.email')}>
            {emails.length ? (
              <div class="flex flex-col gap-1">
                {emails.map((e, i) => (
                  <TypedRow key={`${e.value}-${i}`} t={t} value={e}>
                    <a href={`mailto:${e.value}`} class={`${linkClass} text-primary-text`}>
                      {e.value}
                    </a>
                  </TypedRow>
                ))}
              </div>
            ) : (
              <span class="text-text-subtle">—</span>
            )}
          </Field>
          <Field label={t('field.phone')}>
            {phones.length ? (
              <div class="flex flex-col gap-1">
                {phones.map((p, i) => (
                  <TypedRow key={`${p.value}-${i}`} t={t} value={p}>
                    {/* tel: en una sola línea: un número partido no se marca. */}
                    <a href={`tel:${p.value}`} class={`${linkClass} text-text whitespace-nowrap`}>
                      {p.value}
                    </a>
                  </TypedRow>
                ))}
              </div>
            ) : (
              <span class="text-text-subtle">—</span>
            )}
          </Field>
          <Field label={t('field.organization')}>
            <span class={contact.organization ? 'text-text' : 'text-text-subtle'}>
              {contact.organization ?? '—'}
            </span>
          </Field>
          {/* Dirección, cumpleaños y nota SOLO si los hay: una ficha con tres
              guiones de más no informa de nada (Apple/Google tampoco los pintan). */}
          {details.addresses.length ? (
            <Field label={t('field.address')}>
              <div class="flex flex-col gap-2">
                {details.addresses.map((a, i) => (
                  <div key={i}>
                    <p class="sytue-meta">{typeLabel(t, a.type, true)}</p>
                    <p class="text-text">
                      {addressLines(a).map((line, li) => (
                        <span key={li} class="block">
                          {line}
                        </span>
                      ))}
                    </p>
                  </div>
                ))}
              </div>
            </Field>
          ) : null}
          {details.birthday ? (
            <Field label={t('field.birthday')}>
              <span class="text-text">{formatBirthday(details.birthday, locale)}</span>
            </Field>
          ) : null}
          {details.note.trim() ? (
            <Field label={t('field.note')}>
              {/* La nota se escribe en varias líneas: se respetan (pre-line). */}
              <p class="text-text break-words whitespace-pre-line">{details.note}</p>
            </Field>
          ) : null}
          <Field label={t('field.labels')}>
            <GroupChips t={t} contact={contact} groups={groups} onToggle={onToggleGroup} />
          </Field>
        </dl>
      </Card>
    </div>
  );
}

/**
 * GroupChips — etiquetas del contacto como chips (quitar con la ✕) y menú
 * «Añadir» con las etiquetas restantes de la libreta. La mutación es optimista
 * (la aplica ContactsApp sobre el estado de grupos); aquí solo se serializa el
 * toggle en curso y se notifica el fallo.
 */
function GroupChips({
  t,
  contact,
  groups,
  onToggle,
}: {
  t: T;
  contact: Contact;
  groups: ContactGroup[];
  onToggle?: ((group: ContactGroup, contact: Contact) => Promise<void>) | undefined;
}): JSX.Element {
  const toast = useToast();
  // ID del grupo con mutación en vuelo: deshabilita sus controles.
  const [busyId, setBusyId] = useState<string | null>(null);

  const memberOf = groups.filter((g) => g.members.includes(contact.uid));
  const available = groups.filter((g) => !g.members.includes(contact.uid));

  async function toggle(group: ContactGroup): Promise<void> {
    if (!onToggle) return;
    setBusyId(group.id);
    try {
      await onToggle(group, contact);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('detail.labelFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div class="flex flex-wrap items-center gap-1.5">
      {memberOf.length === 0 ? <span class="text-text-subtle">{t('detail.noLabels')}</span> : null}
      {memberOf.map((g) => (
        <span
          key={g.id}
          class="border-border bg-bg-subtle text-text inline-flex items-center gap-1 rounded-full border py-0.5 pr-1 pl-2.5 text-sm"
        >
          <span class="max-w-48 truncate" title={g.name}>
            {g.name}
          </span>
          {onToggle ? (
            <button
              type="button"
              aria-label={t('detail.removeLabel', { label: g.name })}
              disabled={busyId !== null}
              class="text-text-muted hover:bg-border hover:text-text sytue-focus-ring rounded-control grid h-5 w-5 place-items-center transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50"
              onClick={() => void toggle(g)}
            >
              <Icon name="close" size={12} />
            </button>
          ) : null}
        </span>
      ))}
      {onToggle ? (
        <Dropdown
          label={t('detail.addLabel')}
          align="start"
          trigger={(props) => (
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="plus" size={14} />}
              disabled={busyId !== null}
              {...props}
            >
              {t('common.add')}
            </Button>
          )}
        >
          {available.length === 0 ? (
            <p class="text-text-muted px-3 py-2 text-sm">
              {groups.length === 0 ? t('detail.noLabelsAvailable') : t('detail.allLabelsAssigned')}
            </p>
          ) : (
            available.map((g) => (
              <MenuItem key={g.id} onClick={() => void toggle(g)}>
                <span class="truncate">{g.name}</span>
              </MenuItem>
            ))
          )}
        </Dropdown>
      ) : null}
    </div>
  );
}

/**
 * FavoriteToggle — estrella «destacar» (paridad Google Contacts). Botón icónico
 * con aria-pressed; la mutación es optimista en ContactsApp, aquí solo se
 * serializa la llamada en vuelo y se notifica el fallo. Sobrio: sin relleno de
 * color salvo el trazo/relleno de la propia estrella.
 */
function FavoriteToggle({
  t,
  contact,
  active,
  onToggle,
}: {
  t: T;
  contact: Contact;
  active: boolean;
  onToggle: (contact: Contact) => Promise<void>;
}): JSX.Element {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function handle(): Promise<void> {
    setBusy(true);
    try {
      await onToggle(contact);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('detail.starFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <button
      type="button"
      aria-pressed={active}
      aria-label={active ? t('detail.unstar') : t('detail.star')}
      disabled={busy}
      class={`sytue-focus-ring-inset rounded-control grid h-9 w-9 place-items-center border transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50 ${
        active ? 'border-border text-warning-text' : 'border-border text-text-muted hover:text-text'
      }`}
      onClick={() => void handle()}
    >
      <StarGlyph filled={active} />
    </button>
  );
}

/** Estrella (24-grid, envoltorio `Glifo`). Rellena cuando el contacto está destacado; solo
 *  trazo cuando no. @sytue/ui no expone glifo «star», va inline (patrón TagGlyph). */
function StarGlyph({ filled }: { filled: boolean }): JSX.Element {
  return (
    <Glifo
      d="M12 2.5l2.9 5.9 6.5.95-4.7 4.58 1.11 6.47L12 17.9l-5.81 3.06 1.11-6.47-4.7-4.58 6.5-.95L12 2.5z"
      size={18}
      fill={filled ? 'currentColor' : undefined}
    />
  );
}

/**
 * ContactPhoto — avatar del contacto con control de FOTO (subir/cambiar/quitar).
 * La imagen se redimensiona en el navegador a un cuadrado pequeño y se sube al
 * BFF, que la valida (raster + tamaño) e incrusta en la PHOTO de la vCard. El
 * avatar se sirve por bytes (cacheable). Fallback a iniciales si no hay foto.
 */
function ContactPhoto({
  t,
  contact,
  onChanged,
  editable,
}: {
  t: T;
  contact: Contact;
  onChanged?: (() => void) | undefined;
  editable: boolean;
}): JSX.Element {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const uid = cleanUid(contact.uid);
  const has = hasEmbeddedPhoto(contact.vcard);
  const src = has ? photoUrl(uid, contact.etag) : undefined;

  async function pick(dataUri: string): Promise<void> {
    setBusy(true);
    try {
      await setPhoto(uid, dataUri);
      toast.notify({ tone: 'success', title: t('photo.updated') });
      onChanged?.();
    } catch {
      toast.notify({ tone: 'danger', title: t('photo.updateFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function remove(): Promise<void> {
    setBusy(true);
    try {
      await removePhoto(uid);
      toast.notify({ tone: 'success', title: t('photo.removed') });
      onChanged?.();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('photo.removeFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(false);
    }
  }

  if (!editable) return <Avatar name={contact.fullName} size="xl" src={src} />;

  return (
    <AvatarPicker
      name={contact.fullName}
      src={src}
      hasPhoto={has}
      busy={busy}
      onPick={pick}
      onRemove={remove}
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
  );
}

/**
 * Valor con su etiqueta al lado («Móvil», «Trabajo»). Sin ella, dos números
 * seguidos en la ficha son indistinguibles y hay que abrir el editor para saber
 * a cuál se llama.
 */
function TypedRow({
  t,
  value,
  children,
}: {
  t: T;
  value: TypedValue;
  children: JSX.Element;
}): JSX.Element {
  return (
    <span class="flex flex-wrap items-baseline gap-x-2">
      {children}
      <span class="sytue-meta">{typeLabel(t, value.type)}</span>
    </span>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: JSX.Element | JSX.Element[];
}): JSX.Element {
  return (
    <div class="grid grid-cols-[6.5rem_1fr] items-start gap-4 px-4 py-3 sm:grid-cols-[8rem_1fr] sm:px-6">
      <dt class="text-text-muted text-sm font-medium">{label}</dt>
      <dd class="min-w-0 text-base">{children}</dd>
    </div>
  );
}
