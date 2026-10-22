/**
 * ContactsApp — isla raíz de Sytue Contacts. Monta la Sytue Shell y, debajo, el
 * AppShell canónico (sidebar de libretas + lista filtrable de contactos + ficha).
 * Crear/editar/eliminar vía ContactModal -> BFF CardDAV (servicio Go Sytue
 * Contacts). Estética Sytue, modo claro/oscuro. docs/CONVENTIONS.md §1, §5.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { SytueShell, ImportData, LinkedServices } from '@sytue/shell';
import { useConnectivity } from '@sytue/offline/preact';
import {
  AppShell,
  Sidebar,
  SidebarSection,
  SidebarItem,
  Avatar,
  Button,
  Dropdown,
  FieldGroup,
  Input,
  Icon,
  MenuItem,
  Modal,
  Radio,
  Skeleton,
  Spinner,
  List,
  ListItem,
  VirtualList,
  EmptyState,
  SectionHeader,
  cx,
  describedBy,
  useConfirm,
  useToast,
  BulkActionBar,
  withBase,
  Illustration,
  MobileFab,
  esAtajoDeApp,
  Checkbox,
  Glifo,
} from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import { contiene, intlTag, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import { BirthdaysModal } from './BirthdaysModal.js';
import { proximosCumpleanos } from '../client/cumpleanos.js';
import {
  batchFavorite,
  batchGroupMembers,
  batchTrash,
  createAddressBook,
  createGroup,
  deleteAddressBook,
  deleteContact,
  deleteGroup,
  importCsv,
  listAddressBooks,
  listContacts,
  sincronizarDirectorio,
  listFavorites,
  listGroups,
  listTrash,
  photoUrl,
  purgeContact,
  renameAddressBook,
  restoreContact,
  saveContact,
  toggleFavorite,
  updateGroup,
  type AddressBook,
  type Contact,
  type ContactGroup,
  type ContactInput,
  type TrashItem,
} from '../client/contactsApi.js';
import { buildMergeInput, unionValues } from '../client/merge.js';
import { findDuplicateGroups } from '../client/duplicates.js';
import { downloadVcf, hasEmbeddedPhoto, importVcf } from '../client/vcard.js';
import { getAccessToken } from '@sytue/auth/client';
import { ContactModal, type EditingContact } from './ContactModal.js';
import { ContactDetail } from './ContactDetail.js';

interface Props {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma ya negociado en el servidor (middleware). Nunca se detecta aquí. */
  locale: Locale;
}

/* Casilla de selección: 16 px de dibujo dentro de una zona de toque de 36 px.
   A 16 px, en el móvil el dedo falla la casilla y lo que ocurre entonces es que
   se abre la ficha, que no era lo que se pedía (WCAG 2.5.8 pide 24 px; la HIG de
   Apple, 44). El margen negativo compensa el relleno para no mover nada de sitio,
   y el envoltorio es un <label> de verdad: es el navegador quien reenvía a la
   casilla lo que se toca en el margen, sin manejadores de más. */
const CHECKBOX_HIT = '-m-2.5 grid shrink-0 cursor-pointer place-items-center p-2.5';
const CHECKBOX_CLASS = 'border-border h-4 w-4 cursor-pointer accent-[var(--sytue-primary)]';

/**
 * Normaliza un UID a su forma desnuda (sin el prefijo urn:uuid:), que es como el
 * backend guarda contacts.uid y como devuelve los favoritos. La vCard servida por
 * CardDAV puede traer el UID con prefijo (p. ej. contactos importados de CSV):
 * así el cruce por UID entre la lista y el conjunto de favoritos es fiable.
 */
function cleanUid(uid: string): string {
  return uid.toLowerCase().startsWith('urn:uuid:') ? uid.slice(9) : uid;
}

export function ContactsApp({ user, hubBaseUrl, locale }: Props): JSX.Element {
  const t = useT(locale);
  const offline = useConnectivity() === 'offline';
  const [books, setBooks] = useState<AddressBook[]>([]);
  const [activeBook, setActiveBook] = useState<AddressBook | null>(null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [selected, setSelected] = useState<Contact | null>(null);
  const [query, setQuery] = useState('');
  const [loadingBooks, setLoadingBooks] = useState(true);
  const [loadingContacts, setLoadingContacts] = useState(false);
  const [booksError, setBooksError] = useState(false);
  const [contactsError, setContactsError] = useState(false);
  // Contador de reintentos: relanza el efecto de carga de libretas conservando
  // su guarda de desmontaje.
  const [booksAttempt, setBooksAttempt] = useState(0);
  const [modal, setModal] = useState<{ open: boolean; editing: EditingContact | null }>({
    open: false,
    editing: null,
  });
  // Grupos (etiquetas) de la libreta activa y filtro de lista por etiqueta.
  const [groups, setGroups] = useState<ContactGroup[]>([]);
  const [groupsError, setGroupsError] = useState(false);
  const [loadingGroups, setLoadingGroups] = useState(false);
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  // Alta o cambio de nombre de una etiqueta: `editing` null = alta.
  const [labelModal, setLabelModal] = useState<{ open: boolean; editing: ContactGroup | null }>({
    open: false,
    editing: null,
  });
  // Alta o cambio de nombre de una LIBRETA: `editing` null = alta. El servicio
  // soportaba crear/renombrar/borrar libretas desde el primer día (igual que las
  // etiquetas); la interfaz solo sabía listar la que ya hubiera.
  const [addressBookModal, setAddressBookModal] = useState<{
    open: boolean;
    editing: AddressBook | null;
  }>({ open: false, editing: null });
  // Superviviente de una fusión «Combinar con…» en curso (abre la MergeModal).
  const [mergeFor, setMergeFor] = useState<Contact | null>(null);
  // Candidatos acotados a un grupo de duplicados (null = toda la libreta).
  const [mergeScope, setMergeScope] = useState<Contact[] | null>(null);
  // Modal de revisión de duplicados abierto.
  const [reviewingDups, setReviewingDups] = useState(false);
  // UID de los contactos destacados del usuario (cross-libreta) + filtro activo.
  const [favorites, setFavorites] = useState<Set<string>>(new Set());
  const [favFilter, setFavFilter] = useState(false);
  // Modal de papelera abierto.
  const [trashOpen, setTrashOpen] = useState(false);
  // Próximos cumpleaños, del vCard de cada contacto; el contador es a 30 días.
  const [birthdaysOpen, setBirthdaysOpen] = useState(false);
  const cumpleanos = useMemo(() => proximosCumpleanos(contacts, new Date()), [contacts]);
  const cumpleanosPronto = cumpleanos.filter((c) => c.dias <= 30).length;
  // Multiselección: href de los contactos marcados (checkbox por fila). El
  // ancla de shift-click recuerda la última fila pulsada para seleccionar rangos.
  const [selection, setSelection] = useState<Set<string>>(new Set());
  const anchorRef = useRef<number | null>(null);
  // Contacto pedido por enlace (/contacts/{id}, el del buscador global de la suite):
  // su libreta se abre en vez de la primera y su ficha, al cargar la lista.
  const enlace = useRef<{ book: string; uid: string } | null>(null);

  // La conectividad puede caer con una edición abierta. Contacts degrada a
  // lectura, así que cierra todos los flujos que escriben y limpia su selección.
  useEffect(() => {
    if (!offline) return;
    setModal({ open: false, editing: null });
    setLabelModal({ open: false, editing: null });
    setAddressBookModal({ open: false, editing: null });
    setMergeFor(null);
    setMergeScope(null);
    setReviewingDups(false);
    setTrashOpen(false);
    setSelection(new Set());
    anchorRef.current = null;
  }, [offline]);

  useEffect(() => {
    let active = true;
    setLoadingBooks(true);
    setBooksError(false);
    // El enlace sale de la URL al leerlo: refrescar no lo reaplica, y un reintento de
    // la carga lo conserva en la ref.
    const pedido = new URLSearchParams(window.location.search);
    const uid = pedido.get('uid');
    if (uid) {
      enlace.current = { book: pedido.get('book') ?? '', uid };
      window.history.replaceState(null, '', window.location.pathname);
    }
    // La libreta «Organización» se pone al día ANTES de listar, para que quien
    // acaba de entrar en la empresa aparezca sin que nadie haga nada. Si falla
    // —el directorio caído, sin permiso— no se dice nada y se sigue: la agenda
    // propia tiene que abrirse igual.
    void sincronizarDirectorio()
      .then(() => listAddressBooks())
      .then(({ addressBooks }) => {
        if (!active) return;
        setBooks(addressBooks);
        const slug = enlace.current?.book;
        setActiveBook(
          addressBooks.find((b) => slug && b.url.replace(/\/+$/, '').endsWith(`/${slug}`)) ??
            addressBooks[0] ??
            null,
        );
      })
      .catch(() => active && setBooksError(true))
      .finally(() => active && setLoadingBooks(false));
    return () => {
      active = false;
    };
  }, [booksAttempt]);

  function clearSelection(): void {
    setSelection(new Set());
    anchorRef.current = null;
  }

  function reload(): void {
    if (!activeBook) return;
    clearSelection(); // una recarga de la lista invalida la selección por href
    setLoadingContacts(true);
    setContactsError(false);
    listContacts(activeBook.url)
      .then(({ contacts: list }) => {
        // El orden alfabético depende del idioma (diéresis, ñ, acentos): se
        // compara con el locale del usuario, no con uno fijo.
        const sorted = [...list].sort((a, b) =>
          a.fullName.localeCompare(b.fullName, intlTag(locale)),
        );
        setContacts(sorted);
        const enlazado = enlace.current && sorted.find((c) => c.uid === enlace.current?.uid);
        enlace.current = null;
        // La ficha sigue al dato fresco (etag incluido) tras editar/importar/
        // combinar; si el contacto ya no existe, se deselecciona.
        setSelected(
          (sel) => enlazado || (sel ? (sorted.find((c) => c.href === sel.href) ?? null) : null),
        );
      })
      .catch(() => {
        setContacts([]);
        setContactsError(true);
      })
      .finally(() => setLoadingContacts(false));
  }

  function reloadGroups(): void {
    if (!activeBook) {
      setGroups([]);
      return;
    }
    setLoadingGroups(true);
    setGroupsError(false);
    listGroups(activeBook.url)
      .then(({ groups: list }) => setGroups(list))
      .catch(() => {
        setGroups([]);
        setGroupsError(true);
      })
      .finally(() => setLoadingGroups(false));
  }

  function reloadFavorites(): void {
    void listFavorites()
      .then(({ uids }) => setFavorites(new Set(uids)))
      .catch(() => setFavorites(new Set()));
  }

  // Los destacados son del USUARIO, no de la libreta: `listFavorites()` no recibe
  // la libreta activa. Iban en el efecto de abajo, que corre dos veces al arrancar
  // —una con `activeBook` aún a null y otra cuando la libreta resuelve—, y como
  // `reloadFavorites` no tiene la guarda `if (!activeBook) return` de `reload` y
  // `reloadGroups`, GET /api/favorites salía dos veces idéntico (medido en prod:
  // t+331 ms y t+387 ms, la única petición duplicada de las 18 del arranque).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => reloadFavorites(), []);

  useEffect(() => {
    setSelected(null);
    setActiveGroupId(null); // las etiquetas son por libreta
    reload();
    reloadGroups();
    // Solo se recarga al cambiar la libreta activa (su URL), no por la
    // identidad de `reload`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeBook?.url]);

  // «n» crea un contacto: el atajo que pulsa el menú nativo de la app de
  // escritorio («Contacto → Nuevo contacto ⌘N»).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'n' || !esAtajoDeApp(e)) return;
      e.preventDefault();
      openCreate();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
    // `openCreate` es una declaración de función del componente: estable por render.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /**
   * Destaca o quita el destacado de un contacto (por UID). Actualización
   * optimista del conjunto con reversión; el error lo notifica la ficha.
   */
  async function toggleFav(contact: Contact): Promise<void> {
    const uid = cleanUid(contact.uid);
    const has = favorites.has(uid);
    const next = new Set(favorites);
    if (has) next.delete(uid);
    else next.add(uid);
    setFavorites(next);
    try {
      await toggleFavorite(uid, !has);
    } catch (err) {
      setFavorites(favorites); // revertir
      throw err;
    }
  }

  /**
   * Asigna o quita la etiqueta al contacto (toggle sobre members, MEMBER por
   * UID). Actualización optimista con reversión; el error lo notifica la ficha
   * (useToast solo está disponible bajo la Sytue Shell).
   */
  async function toggleGroupMembership(group: ContactGroup, contact: Contact): Promise<void> {
    const has = group.members.includes(contact.uid);
    const members = has
      ? group.members.filter((uid) => uid !== contact.uid)
      : [...group.members, contact.uid];
    const prev = groups;
    setGroups((gs) => gs.map((g) => (g.id === group.id ? { ...g, members } : g)));
    try {
      const saved = await updateGroup({ id: group.id, name: group.name, members });
      setGroups((gs) => gs.map((g) => (g.id === group.id ? { ...g, ...saved } : g)));
    } catch (err) {
      setGroups(prev);
      throw err;
    }
  }

  /**
   * Fusiona `loser` dentro de `survivor` (ambos de la libreta activa). Orden
   * ATÓMICO, NUNCA invertido:
   *  1) saveContact del superviviente (PUT con If-Match). Si falla —p.ej. 412
   *     por etag viejo, que el BFF reenvía como error genérico—, se recarga el
   *     estado fresco del servidor y se reintenta UNA vez con el etag nuevo,
   *     re-uniendo también los datos frescos de ambas tarjetas.
   *  2) Re-apuntar las etiquetas del perdedor al superviviente (MEMBER por
   *     UID, updateGroup) ANTES de borrar, para no dejar membresías huérfanas.
   *  3) deleteContact del perdedor.
   * Si (2)/(3) fallan tras el PUT, el perdedor queda como duplicado (estado
   * seguro, sin pérdida de datos): se devuelve 'duplicate' para avisar con
   * toast. Si (1) falla, se propaga el error y no se ha tocado nada.
   */
  async function performMerge(
    survivor: Contact,
    loser: Contact,
    merged: ContactInput,
  ): Promise<'merged' | 'duplicate'> {
    if (!activeBook) throw new Error('sin libreta activa');
    const bookUrl = activeBook.url;
    try {
      try {
        await saveContact({
          addressBookUrl: bookUrl,
          // baseVcard = tarjeta del superviviente: conserva sus campos no
          // proyectados (PHOTO/BDAY/ADR/X-*…) al reconstruir la vCard fusionada.
          contactInput: { ...merged, baseVcard: survivor.vcard },
          href: survivor.href,
          ...(survivor.etag ? { etag: survivor.etag } : {}),
        });
      } catch {
        const { contacts: fresh } = await listContacts(bookUrl);
        const freshSurvivor = fresh.find((c) => c.uid === survivor.uid);
        if (!freshSurvivor) throw new Error('el contacto superviviente ya no existe');
        const freshLoser = fresh.find((c) => c.uid === loser.uid) ?? loser;
        await saveContact({
          addressBookUrl: bookUrl,
          contactInput: {
            ...merged,
            emails: unionValues(merged.emails, freshSurvivor.emails, freshLoser.emails),
            phones: unionValues(merged.phones, freshSurvivor.phones, freshLoser.phones),
            baseVcard: freshSurvivor.vcard,
          },
          href: freshSurvivor.href,
          ...(freshSurvivor.etag ? { etag: freshSurvivor.etag } : {}),
        });
      }
      try {
        for (const g of groups) {
          if (!g.members.includes(loser.uid)) continue;
          const members = g.members.filter((uid) => uid !== loser.uid);
          if (!members.includes(survivor.uid)) members.push(survivor.uid);
          await updateGroup({ id: g.id, name: g.name, members });
        }
        await deleteContact(loser.href);
      } catch {
        return 'duplicate';
      }
      return 'merged';
    } finally {
      // Refleja el resultado real en lista, ficha y etiquetas pase lo que pase.
      reload();
      reloadGroups();
    }
  }

  const activeGroup = groups.find((g) => g.id === activeGroupId) ?? null;

  const filtered = useMemo(() => {
    const memberSet = activeGroup ? new Set(activeGroup.members) : null;
    const q = query.trim().toLowerCase();
    // El teléfono se busca por DÍGITOS: nadie teclea «+34 600 00 00 00» tal y
    // como está guardado, y en una libreta importada conviven los dos formatos.
    // Desde 3 dígitos: con menos, «1» devolvería media libreta.
    const qDigits = q.replace(/\D/g, '');
    const byPhone = qDigits.length >= 3;
    if (!q && !memberSet && !favFilter) return contacts;
    return contacts.filter(
      (c) =>
        (!favFilter || favorites.has(cleanUid(c.uid))) &&
        (!memberSet || memberSet.has(c.uid)) &&
        (!q ||
          contiene(c.fullName, q) ||
          c.emails.some((e) => contiene(e, q)) ||
          contiene(c.organization ?? '', q) ||
          (byPhone && c.phones.some((p) => p.replace(/\D/g, '').includes(qDigits)))),
    );
  }, [contacts, query, activeGroup, favFilter, favorites]);

  // Clave estable por fila para el virtualizador (es su clave de caché de
  // medidas: si cambia en cada render, remide todo y el scroll salta).
  const rowKey = useCallback((index: number) => filtered[index]?.href ?? index, [filtered]);

  // Grupos de posibles duplicados en la libreta cargada (nombre/correo/teléfono).
  const duplicateGroups = useMemo(() => findDuplicateGroups(contacts), [contacts]);

  // Contactos seleccionados (por href) y sus UID (para las acciones en lote).
  const selectedContacts = useMemo(
    () => contacts.filter((c) => selection.has(c.href)),
    [contacts, selection],
  );
  const selectedUids = useMemo(
    () => selectedContacts.map((c) => cleanUid(c.uid)),
    [selectedContacts],
  );
  // Estado del «seleccionar todo» (sobre lo filtrado): todo / parcial (indeterminate).
  const allFilteredSelected = filtered.length > 0 && filtered.every((c) => selection.has(c.href));
  const someFilteredSelected = !allFilteredSelected && filtered.some((c) => selection.has(c.href));

  /** Marca/desmarca una fila; con shift extiende el rango desde el ancla previo. */
  function toggleRow(index: number, shiftKey: boolean): void {
    setSelection((prev) => {
      const next = new Set(prev);
      if (shiftKey && anchorRef.current !== null) {
        const a = anchorRef.current;
        const [lo, hi] = a < index ? [a, index] : [index, a];
        for (let i = lo; i <= hi; i += 1) {
          const h = filtered[i]?.href;
          if (h) next.add(h);
        }
      } else {
        const h = filtered[index]?.href;
        if (h) {
          if (next.has(h)) next.delete(h);
          else next.add(h);
        }
        anchorRef.current = index;
      }
      return next;
    });
  }

  /** Selecciona (o deselecciona) todas las filas visibles del filtro actual. */
  function toggleSelectAll(): void {
    setSelection((prev) => {
      const next = new Set(prev);
      if (allFilteredSelected) filtered.forEach((c) => next.delete(c.href));
      else filtered.forEach((c) => next.add(c.href));
      return next;
    });
    anchorRef.current = null;
  }

  /** URL del avatar si la vCard trae foto embebida; si no, undefined (iniciales). */
  function avatarSrc(c: Contact): string | undefined {
    return hasEmbeddedPhoto(c.vcard) ? photoUrl(cleanUid(c.uid), c.etag) : undefined;
  }

  /** Ejecuta una acción en lote y refresca lista/estado; el error lo avisa BatchBar. */
  async function runBatch(fn: () => Promise<unknown>): Promise<void> {
    await fn();
    reload();
    reloadFavorites();
    reloadGroups();
  }

  // Abre la fusión sobre un grupo de duplicados: superviviente = 1.º, candidatos
  // acotados al resto del grupo. Reutiliza la MergeModal existente.
  function pickDuplicateGroup(group: Contact[]): void {
    setReviewingDups(false);
    setMergeScope(group.slice(1));
    setMergeFor(group[0] ?? null);
  }

  function openCreate(): void {
    setModal({ open: true, editing: { href: null, fullName: '', emails: [], phones: [] } });
  }

  function openEdit(c: Contact): void {
    setModal({
      open: true,
      editing: {
        href: c.href,
        etag: c.etag,
        uid: c.uid,
        fullName: c.fullName,
        emails: c.emails,
        phones: c.phones,
        organization: c.organization,
        vcard: c.vcard,
      },
    });
  }

  const favCount = useMemo(
    () => contacts.reduce((n, c) => (favorites.has(cleanUid(c.uid)) ? n + 1 : n), 0),
    [contacts, favorites],
  );

  // Recuento de la lista. Se compone con claves de catálogo (plural por
  // `Intl.PluralRules`, nunca `n === 1 ? … : …`) y el nombre de la etiqueta
  // activa entra por interpolación: concatenar «X de Y» con « · etiqueta» a
  // mano no sobrevive a un cambio de orden de palabras.
  // El filtro de destacados cuenta como filtro: sin él, la vista «Destacados»
  // decía «1.234 contactos» mientras enseñaba cinco.
  const countSummary =
    query || activeGroup || favFilter
      ? t('list.countFiltered', { shown: filtered.length, count: contacts.length })
      : t('list.count', { count: contacts.length });

  const sidebar = (
    <Sidebar
      header={
        <Button
          fullWidth
          variant="primary"
          leadingIcon={<Icon name="plus" />}
          disabled={!activeBook || offline}
          onClick={openCreate}
        >
          {t('nav.newContact')}
        </Button>
      }
      // «Trae tus contactos»: importar un CSV ya estaba, pero traerlos directamente de
      // Google o Microsoft —que es lo que casi todo el mundo quiere— solo se podía desde
      // la consola de administración.
      footer={
        <div class="space-y-2">
          <LinkedServices
            service="contacts"
            basePath={import.meta.env.BASE_URL}
            labels={{
              create: t('nav.newContact'),
              title: t('form.fullName'),
              text: t('field.note'),
              organization: t('field.organization'),
              email: t('field.email'),
              phone: t('field.phone'),
            }}
          />
          <ImportData domains={['contacts']} onFinished={reload} />
        </div>
      }
    >
      <SidebarSection title={t('nav.views')}>
        <SidebarItem
          label={t('nav.starred')}
          {...(favCount ? { count: favCount } : {})}
          active={favFilter}
          icon={<StarGlyph filled={favFilter} />}
          // Toggle: pulsar «Destacados» activo quita el filtro.
          onClick={() => setFavFilter((v) => !v)}
        />
        <SidebarItem
          label={t('nav.birthdays')}
          {...(cumpleanosPronto ? { count: cumpleanosPronto } : {})}
          icon={<CakeGlyph />}
          onClick={() => setBirthdaysOpen(true)}
        />
        {!offline ? (
          <SidebarItem
            label={t('nav.trash')}
            icon={<TrashGlyph />}
            onClick={() => setTrashOpen(true)}
          />
        ) : null}
      </SidebarSection>
      <SidebarSection title={t('nav.addressBooks')}>
        {loadingBooks ? (
          <div class="flex justify-center py-4">
            <Spinner class="text-sidebar-fg-muted" />
          </div>
        ) : booksError ? (
          <div class="text-sidebar-fg-muted px-2.5 py-1 text-sm">
            <p>{t('nav.booksError')}</p>
            <button
              type="button"
              class="text-sidebar-fg sytue-focus-ring-chrome mt-1 rounded-sm font-semibold underline-offset-2 hover:underline focus-visible:outline-none"
              onClick={() => setBooksAttempt((n) => n + 1)}
            >
              {t('common.retry')}
            </button>
          </div>
        ) : (
          <>
            {books.length === 0 ? (
              <p class="text-sidebar-fg-muted px-2.5 py-1 text-sm">{t('nav.booksEmpty')}</p>
            ) : (
              books.map((b) => (
                <SidebarItem
                  key={b.url}
                  label={b.displayName}
                  active={activeBook?.url === b.url}
                  icon={<Icon name="user" size={18} />}
                  onClick={() => setActiveBook(b)}
                />
              ))
            )}
            {!offline ? (
              <SidebarItem
                label={t('nav.newAddressBook')}
                icon={<Icon name="plus" size={18} />}
                onClick={() => setAddressBookModal({ open: true, editing: null })}
              />
            ) : null}
          </>
        )}
      </SidebarSection>
      {activeBook ? (
        <SidebarSection title={t('nav.labels')}>
          {loadingGroups ? (
            <div class="flex justify-center py-4">
              <Spinner class="text-sidebar-fg-muted" />
            </div>
          ) : groupsError ? (
            <div class="text-sidebar-fg-muted px-2.5 py-1 text-sm">
              <p>{t('nav.labelsError')}</p>
              <button
                type="button"
                class="text-sidebar-fg sytue-focus-ring-chrome mt-1 rounded-sm font-semibold underline-offset-2 hover:underline focus-visible:outline-none"
                onClick={reloadGroups}
              >
                {t('common.retry')}
              </button>
            </div>
          ) : (
            <>
              {groups.length === 0 ? (
                <p class="text-sidebar-fg-muted px-2.5 py-1 text-sm">{t('nav.labelsEmpty')}</p>
              ) : (
                groups.map((g) => (
                  <SidebarItem
                    key={g.id}
                    label={g.name}
                    count={g.members.length}
                    active={activeGroupId === g.id}
                    icon={<TagGlyph />}
                    // Toggle: pulsar la etiqueta activa quita el filtro.
                    onClick={() => setActiveGroupId((id) => (id === g.id ? null : g.id))}
                  />
                ))
              )}
              {!offline ? (
                <SidebarItem
                  label={t('nav.newLabel')}
                  icon={<Icon name="plus" size={18} />}
                  onClick={() => setLabelModal({ open: true, editing: null })}
                />
              ) : null}
            </>
          )}
        </SidebarSection>
      ) : null}
    </Sidebar>
  );

  const listPanel = (
    <section class="flex h-full flex-col" aria-label={t('list.region')}>
      {/* Cabecera de lista en BLANCO (SectionHeader, estilo Mattermost): el color
          del producto vive en la topbar y el menú lateral, no sobre el contenido. */}
      <SectionHeader
        // Titula la vista entera: es el h1 de la pagina (la ficha va en h2).
        level={1}
        title={t('list.title')}
        actions={
          <div class="flex shrink-0 items-center gap-1.5">
            {/* Renombrar y eliminar la etiqueta, junto a la vista que filtra por
                ella: el servicio y el BFF lo servían desde el primer día y la
                interfaz solo sabía crearlas. */}
            {!offline && activeGroup ? (
              <LabelActions
                t={t}
                group={activeGroup}
                onRename={() => setLabelModal({ open: true, editing: activeGroup })}
                onDeleted={() => {
                  setActiveGroupId(null);
                  reloadGroups();
                }}
              />
            ) : null}
            {!offline && activeBook ? (
              <BookActions
                t={t}
                book={activeBook}
                onRename={() => setAddressBookModal({ open: true, editing: activeBook })}
                onDeleted={() => setBooksAttempt((n) => n + 1)}
              />
            ) : null}
            {!offline && duplicateGroups.length > 0 ? (
              <Button variant="secondary" size="sm" onClick={() => setReviewingDups(true)}>
                {t('list.reviewDuplicates', { count: duplicateGroups.length })}
              </Button>
            ) : null}
            <VcardToolbar
              t={t}
              contacts={contacts}
              addressBookUrl={activeBook?.url ?? null}
              addressBookId={activeBook?.id ?? null}
              onImported={reload}
              offline={offline}
            />
          </div>
        }
      />
      <div class="border-border border-b p-3">
        <Input
          type="search"
          placeholder={t('list.search')}
          aria-label={t('list.search')}
          leadingIcon={<Icon name="search" size={16} />}
          value={query}
          onInput={(e) => setQuery(e.currentTarget.value)}
        />
        {!loadingContacts && contacts.length > 0 ? (
          // mt-3 y no mt-2: la zona de toque de la casilla se estira 10 px hacia
          // arriba y con 8 px de hueco pisaba el borde del buscador.
          <div class="mt-3 flex items-center gap-2.5 px-0.5">
            <label class={CHECKBOX_HIT}>
              <Checkbox
                class={CHECKBOX_CLASS}
                aria-label={t('list.selectAll')}
                checked={allFilteredSelected}
                indeterminate={someFilteredSelected}
                onChange={toggleSelectAll}
              />
            </label>
            {/* role=status: al escribir en el buscador, quien usa lector de
                pantalla no tenía forma de saber cuántos quedaban. */}
            <p class="sytue-meta tabular-nums" role="status">
              {activeGroup
                ? t('list.countWithLabel', { summary: countSummary, label: activeGroup.name })
                : countSummary}
            </p>
          </div>
        ) : null}
      </div>
      {!offline && selectedContacts.length > 0 ? (
        <BatchBar
          t={t}
          count={selectedContacts.length}
          uids={selectedUids}
          groups={groups}
          onRun={runBatch}
          onClear={clearSelection}
        />
      ) : null}
      {/* Sin scroll propio: quien desplaza es la VirtualList de dentro (necesita
          ser ella el contenedor de scroll para saber qué ventana pintar). Las
          otras ramas —esqueleto, error, vacío— caben de sobra en el panel. */}
      <div class="flex min-h-0 flex-1 flex-col" aria-busy={loadingContacts}>
        {loadingContacts ? (
          <ContactListSkeleton />
        ) : contactsError || booksError ? (
          <EmptyState
            class="h-full"
            title={t('list.errorTitle')}
            description={t('list.errorDescription')}
            illustration={<Icon name="warning" size={40} />}
            action={
              <Button
                variant="secondary"
                size="sm"
                onClick={() => (booksError ? setBooksAttempt((n) => n + 1) : reload())}
              >
                {t('common.retry')}
              </Button>
            }
          />
        ) : filtered.length === 0 ? (
          // Icono del DS en lugar de las ilustraciones SVG externas: dentro de
          // <img> sus var(--sytue-*) no se resuelven y quedaban invisibles.
          // Cada filtro explica lo SUYO y ofrece su salida: en «Destacados» sin
          // destacados se leía «tu libreta está vacía» —con la libreta llena— y
          // el único botón creaba un contacto que tampoco iba a salir ahí.
          <EmptyState
            class="h-full"
            title={t('list.emptyTitle')}
            description={
              query
                ? t('list.emptySearch')
                : activeGroup
                  ? t('list.emptyLabel', { label: activeGroup.name })
                  : favFilter
                    ? t('list.emptyStarred')
                    : t('list.emptyBook')
            }
            illustration={<Illustration name={query ? 'emptySearch' : 'emptyContacts'} />}
            action={
              query ? (
                <Button variant="secondary" size="sm" onClick={() => setQuery('')}>
                  {t('list.clearSearch')}
                </Button>
              ) : activeGroup ? (
                <Button variant="secondary" size="sm" onClick={() => setActiveGroupId(null)}>
                  {t('list.clearFilter')}
                </Button>
              ) : favFilter ? (
                <Button variant="secondary" size="sm" onClick={() => setFavFilter(false)}>
                  {t('list.clearFilter')}
                </Button>
              ) : offline ? undefined : (
                <Button
                  variant="primary"
                  size="sm"
                  leadingIcon={<Icon name="plus" />}
                  onClick={openCreate}
                >
                  {t('nav.newContact')}
                </Button>
              )
            }
          />
        ) : (
          // VIRTUALIZADA. Una libreta corporativa son miles de tarjetas y aquí se
          // pintaba UNA FILA POR CONTACTO: 2.000 contactos medían 25.601 elementos
          // de DOM y 2.000 <img> de avatar en el primer render. Con VirtualList
          // solo existe la ventana visible + overscan, así que el coste deja de
          // depender del tamaño de la libreta. Es la misma receta de la casa que
          // ya usan el directorio de la consola, Drive, Docs y Rooms.
          <VirtualList
            items={filtered}
            // Alto real de la fila: 32 px de avatar + py-2 (8+8). El
            // virtualizador mide el DOM de todas formas; esto es la estimación
            // de arranque.
            estimateSize={48}
            getItemKey={rowKey}
            aria-label={t('list.title')}
            class="min-h-0 flex-1"
            renderItem={(c, index) => {
              const isSelected = selected?.href === c.href;
              const subtitle = c.emails[0] ?? c.organization;
              return (
                <div
                  class={cx(
                    'flex items-center gap-3 border-l-2 px-3 py-2 text-base transition-colors duration-150',
                    isSelected
                      ? 'border-l-primary bg-selected'
                      : 'hover:bg-bg-subtle border-l-transparent',
                  )}
                >
                  {/* La casilla va SUELTA del botón que abre la ficha (patrón del
                      directorio de la consola): antes iba dentro de la fila
                      clicable y hacía falta un data-select y dos
                      stopPropagation para que marcar a alguien no abriera además
                      su ficha. Como hermanos, el problema no existe. */}
                  <label class={CHECKBOX_HIT}>
                    <Checkbox
                      class={CHECKBOX_CLASS}
                      checked={selection.has(c.href)}
                      aria-label={t('list.select', { name: c.fullName })}
                      onClick={(e) => toggleRow(index, (e as unknown as MouseEvent).shiftKey)}
                    />
                  </label>
                  {/* <button> de verdad: Enter y Espacio los da el navegador, sin
                      manejador de teclado propio. Anillo de foco interior (DS
                      §3.7): el global de 3 px lo recorta el overflow del panel. */}
                  <button
                    type="button"
                    class="sytue-focus-ring-inner flex min-w-0 flex-1 items-center gap-2.5 rounded-md text-left"
                    aria-current={isSelected ? 'true' : undefined}
                    onClick={() => setSelected(c)}
                  >
                    <Avatar name={c.fullName} src={avatarSrc(c)} size="sm" />
                    <span class="flex min-w-0 flex-1 flex-col">
                      <span class="text-text truncate font-medium" title={c.fullName}>
                        {c.fullName}
                      </span>
                      {subtitle ? (
                        <span class="text-text-muted truncate text-sm" title={subtitle}>
                          {subtitle}
                        </span>
                      ) : null}
                    </span>
                    {favorites.has(cleanUid(c.uid)) ? (
                      <span class="text-warning-text shrink-0" title={t('list.starredTitle')}>
                        <StarGlyph filled size={14} />
                      </span>
                    ) : null}
                  </button>
                </div>
              );
            }}
          />
        )}
      </div>
    </section>
  );

  return (
    <SytueShell
      currentApp="contacts"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      {/* La topbar global ya la pinta la Sytue Shell: el AppShell ocupa el resto del viewport. */}
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        sidebar={sidebar}
        // En <768px la lista y la ficha no caben a la vez: la shell muestra una
        // u otra, y sin contacto elegido se queda en la lista.
        detailOpen={Boolean(selected)}
        onBack={() => setSelected(null)}
        title={activeBook?.displayName ?? t('list.title')}
        secondaryPanelResize={{
          label: activeBook?.displayName ?? t('list.title'),
          storageKey: 'sytue.contacts.panel-width',
        }}
        secondaryPanel={listPanel}
      >
        <ContactDetail
          t={t}
          locale={locale}
          contact={selected}
          groups={groups}
          onToggleGroup={offline ? undefined : toggleGroupMembership}
          onEdit={offline ? undefined : openEdit}
          onMerge={!offline && contacts.length > 1 ? setMergeFor : undefined}
          onCreate={!offline && activeBook ? openCreate : undefined}
          isFavorite={selected ? favorites.has(cleanUid(selected.uid)) : false}
          onToggleFavorite={offline ? undefined : toggleFav}
          onChanged={offline ? undefined : reload}
          readOnly={offline}
          org={user.tenant}
        />
      </AppShell>
      {/* Nuevo contacto al alcance del pulgar: en móvil el menú lateral es un cajón cerrado. */}
      {offline ? null : (
        <MobileFab
          label={t('nav.newContact')}
          icon={<Icon name="plus" size={24} />}
          onClick={openCreate}
        />
      )}

      <ContactModal
        t={t}
        open={modal.open}
        addressBookUrl={activeBook?.url ?? null}
        editing={modal.editing}
        onClose={() => setModal({ open: false, editing: null })}
        onSaved={reload}
      />
      <LabelModal
        t={t}
        open={labelModal.open}
        editing={labelModal.editing}
        addressBookUrl={activeBook?.url ?? null}
        onClose={() => setLabelModal({ open: false, editing: null })}
        onSaved={reloadGroups}
      />
      <AddressBookModal
        t={t}
        open={addressBookModal.open}
        editing={addressBookModal.editing}
        onClose={() => setAddressBookModal({ open: false, editing: null })}
        onSaved={() => setBooksAttempt((n) => n + 1)}
      />
      <MergeModal
        t={t}
        survivor={mergeFor}
        candidates={
          mergeFor ? (mergeScope ?? contacts.filter((c) => c.href !== mergeFor.href)) : []
        }
        onClose={() => {
          setMergeFor(null);
          setMergeScope(null);
        }}
        onMerge={performMerge}
      />
      <DuplicatesModal
        t={t}
        open={reviewingDups}
        groups={duplicateGroups}
        onClose={() => setReviewingDups(false)}
        onPick={pickDuplicateGroup}
      />
      <BirthdaysModal
        t={t}
        locale={locale}
        open={birthdaysOpen}
        cumpleanos={cumpleanos}
        onClose={() => setBirthdaysOpen(false)}
        onOpen={(c) => {
          setBirthdaysOpen(false);
          setSelected(c);
        }}
      />
      <TrashModal
        t={t}
        open={trashOpen}
        onClose={() => setTrashOpen(false)}
        onChanged={() => {
          reload();
          reloadFavorites();
        }}
      />
    </SytueShell>
  );
}

/**
 * Acciones de libreta: Importar/Exportar en vCard y CSV (Google Contacts /
 * Outlook). Vive dentro de la Sytue Shell para poder usar useToast.
 *  - Importar vCard: parte el .vcf y manda cada tarjeta entera al servicio.
 *  - Importar CSV: envía el texto al BFF; el servicio Go detecta el dialecto,
 *    salta filas inválidas y deduplica por correo (una sola llamada).
 *  - Exportar vCard: descarga la libreta cargada como .vcf multi-tarjeta.
 *  - Exportar CSV: descarga TODOS los contactos del usuario (GET /api/export-csv).
 */
function VcardToolbar({
  t,
  contacts,
  addressBookUrl,
  addressBookId,
  onImported,
  offline,
}: {
  t: T;
  contacts: Contact[];
  addressBookUrl: string | null;
  /** Id REST de la libreta: el alta de una tarjeta entera va por la API, no por CardDAV. */
  addressBookId: string | null;
  onImported: () => void;
  offline: boolean;
}): JSX.Element {
  const toast = useToast();
  const vcfRef = useRef<HTMLInputElement>(null);
  const csvRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);
  // Avance REAL de la importación de vCard: el total se conoce antes de la
  // primera escritura. Antes solo había un spinner mudo durante los cinco
  // minutos que tardan 2.000 tarjetas, sin forma de saber si seguía viva.
  // El CSV no lo necesita: va en UNA petición al servicio Go.
  const [vcfProgress, setVcfProgress] = useState<{ done: number; total: number } | null>(null);

  function reportImport(imported: number, skipped: number, kind: 'vCard' | 'CSV'): void {
    if (imported > 0) {
      toast.notify({
        tone: 'success',
        title: t('io.imported', { count: imported }),
        ...(skipped ? { description: t('io.skipped', { count: skipped }) } : {}),
      });
      onImported();
    } else {
      toast.notify({
        tone: 'danger',
        title: t('io.noneImported'),
        description: kind === 'CSV' ? t('io.noneImportedCsv') : t('io.noneImportedVcard'),
      });
    }
  }

  async function onVcf(input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    input.value = ''; // permite reimportar el mismo fichero
    if (!file) return;
    // Sin id REST no hay alta (el listado REST de libretas falló): se dice, en vez de
    // no hacer nada al elegir el fichero.
    if (!addressBookId) {
      reportImport(0, 0, 'vCard');
      return;
    }
    setImporting(true);
    try {
      const { imported, failed } = await importVcf(
        addressBookId,
        await file.text(),
        (done, total) => setVcfProgress({ done, total }),
      );
      reportImport(imported, failed, 'vCard');
    } catch {
      toast.notify({ tone: 'danger', title: t('io.vcardReadFailed') });
    } finally {
      setImporting(false);
      setVcfProgress(null);
    }
  }

  async function onCsv(input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    input.value = '';
    if (!file || !addressBookUrl) return;
    setImporting(true);
    try {
      const { imported, skipped } = await importCsv(addressBookUrl, await file.text());
      reportImport(imported, skipped, 'CSV');
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('io.csvFailed'),
        description: t('io.csvFailedHint'),
      });
    } finally {
      setImporting(false);
    }
  }

  return (
    <>
      <input
        ref={vcfRef}
        type="file"
        accept=".vcf,text/vcard"
        class="hidden"
        aria-hidden="true"
        tabindex={-1}
        onChange={(e) => void onVcf(e.currentTarget)}
      />
      <input
        ref={csvRef}
        type="file"
        accept=".csv,text/csv"
        class="hidden"
        aria-hidden="true"
        tabindex={-1}
        onChange={(e) => void onCsv(e.currentTarget)}
      />
      <Dropdown
        label={t('io.menuLabel')}
        align="end"
        trigger={(props) => (
          <Button
            variant="secondary"
            size="sm"
            loading={importing}
            disabled={importing}
            trailingIcon={<Icon name="chevronDown" size={14} />}
            {...props}
          >
            {/* El propio botón lleva la cuenta: es donde mira quien acaba de
                pulsarlo, y el `aria-busy` del Button ya está puesto. role=status
                para que el avance también se anuncie sin robar el foco. */}
            {vcfProgress ? (
              <span class="tabular-nums" role="status">
                {t('io.importProgress', vcfProgress)}
              </span>
            ) : (
              t('io.button')
            )}
          </Button>
        )}
      >
        <MenuItem disabled={offline || !addressBookUrl} onClick={() => vcfRef.current?.click()}>
          {t('io.importVcard')}
        </MenuItem>
        <MenuItem disabled={offline || !addressBookUrl} onClick={() => csvRef.current?.click()}>
          {t('io.importCsv')}
        </MenuItem>
        <MenuItem
          disabled={contacts.length === 0}
          onClick={() =>
            downloadVcf(
              t('io.exportFilename'),
              contacts.map((c) => c.vcard),
            )
          }
        >
          {t('io.exportVcard')}
        </MenuItem>
        <MenuItem
          disabled={offline}
          onClick={() =>
            window.location.assign(withBase(import.meta.env.BASE_URL, '/api/export-csv'))
          }
        >
          {t('io.exportCsv')}
        </MenuItem>
      </Dropdown>
    </>
  );
}

/**
 * LabelActions — cambiar el nombre y eliminar la etiqueta por la que se está
 * filtrando. Eliminarla NO tiene vuelta atrás (no hay papelera de etiquetas), así
 * que pregunta; los contactos no se tocan y el diálogo lo dice. Vive bajo la
 * Sytue Shell para poder usar useToast (mismo motivo que VcardToolbar).
 */
function LabelActions({
  t,
  group,
  onRename,
  onDeleted,
}: {
  t: T;
  group: ContactGroup;
  onRename: () => void;
  onDeleted: () => void;
}): JSX.Element {
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [busy, setBusy] = useState(false);

  async function remove(): Promise<void> {
    const ok = await confirmar({
      titulo: t('label.delete'),
      mensaje: t('label.deleteConfirm', { label: group.name }),
      confirmLabel: t('label.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    setBusy(true);
    try {
      await deleteGroup(group.id);
      toast.notify({ tone: 'success', title: t('label.deleted') });
      onDeleted();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('label.deleteFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Dropdown
        label={t('label.actions')}
        align="end"
        trigger={({ onClick, ...aria }) => (
          <Button
            {...aria}
            onClick={onClick}
            variant="ghost"
            size="sm"
            iconOnly
            disabled={busy}
            aria-label={t('label.actions')}
            title={t('label.actions')}
            leadingIcon={<TagGlyph />}
          />
        )}
      >
        {(close) => (
          <>
            <MenuItem
              onClick={() => {
                close();
                onRename();
              }}
            >
              {t('label.rename')}
            </MenuItem>
            <MenuItem
              danger
              onClick={() => {
                close();
                void remove();
              }}
            >
              {t('label.delete')}
            </MenuItem>
          </>
        )}
      </Dropdown>
      {dialogo}
    </>
  );
}

/**
 * BookActions — cambiar el nombre y eliminar la libreta activa. Sin `id` (el
 * BFF no pudo emparejar el id REST con la libreta CardDAV) no se ofrece nada:
 * mejor no mostrar el botón que mostrarlo y que falle. La libreta por defecto
 * tampoco se puede borrar (el servicio la protege); si lo es, solo se ofrece
 * renombrar. Mismo patrón que LabelActions.
 */
function BookActions({
  t,
  book,
  onRename,
  onDeleted,
}: {
  t: T;
  book: AddressBook;
  onRename: () => void;
  onDeleted: () => void;
}): JSX.Element | null {
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [busy, setBusy] = useState(false);

  if (!book.id) return null;
  const id = book.id;

  async function remove(): Promise<void> {
    const ok = await confirmar({
      titulo: t('book.delete'),
      mensaje: t('book.deleteConfirm', { book: book.displayName }),
      confirmLabel: t('book.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    setBusy(true);
    try {
      await deleteAddressBook(id);
      toast.notify({ tone: 'success', title: t('book.deleted') });
      onDeleted();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('book.deleteFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Dropdown
        label={t('book.actions')}
        align="end"
        trigger={({ onClick, ...aria }) => (
          <Button
            {...aria}
            onClick={onClick}
            variant="ghost"
            size="sm"
            iconOnly
            disabled={busy}
            aria-label={t('book.actions')}
            title={t('book.actions')}
            leadingIcon={<Icon name="user" size={16} />}
          />
        )}
      >
        {(close) => (
          <>
            <MenuItem
              onClick={() => {
                close();
                onRename();
              }}
            >
              {t('book.rename')}
            </MenuItem>
            {!book.isDefault ? (
              <MenuItem
                danger
                onClick={() => {
                  close();
                  void remove();
                }}
              >
                {t('book.delete')}
              </MenuItem>
            ) : null}
          </>
        )}
      </Dropdown>
      {dialogo}
    </>
  );
}

/** Icono de etiqueta (24-grid, envoltorio `Glifo`): @sytue/ui aún no expone un glifo
 *  «tag», así que va inline (mismo patrón que VideoGlyph en Chat). */
function TagGlyph(): JSX.Element {
  return (
    <Glifo size={18}>
      <path d="M20.6 13.4 12 22l-9-9V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z" />
      <path d="M7.5 7.5h.01" />
    </Glifo>
  );
}

/** Estrella «destacado» (24-grid, envoltorio `Glifo`). Rellena si `filled`. Glifo inline:
 *  @sytue/ui no expone «star» (mismo patrón que TagGlyph). */
/** Tarta con una vela: mismo lenguaje que los otros glifos (rejilla 24, envoltorio `Glifo`). */
function CakeGlyph({ size = 18 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M4 20h16M4 20v-7a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v7M4 15.5c1.5 1.5 3 1.5 4.5 0s3-1.5 4.5 0 3 1.5 4.5 0 2-1 2.5-.5" />
      <path d="M12 11V8M12 5.5a1 1 0 1 0 0-.01" />
    </Glifo>
  );
}

function StarGlyph({ filled, size = 18 }: { filled: boolean; size?: number }): JSX.Element {
  return (
    <Glifo
      d="M12 2.5l2.9 5.9 6.5.95-4.7 4.58 1.11 6.47L12 17.9l-5.81 3.06 1.11-6.47-4.7-4.58 6.5-.95L12 2.5z"
      size={size}
      fill={filled ? 'currentColor' : undefined}
    />
  );
}

/** Papelera (24-grid, envoltorio `Glifo`). Glifo inline (@sytue/ui no expone «trash»). */
function TrashGlyph({ size = 18 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M3 6h18" />
      <path d="M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2" />
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
      <path d="M10 11v6M14 11v6" />
    </Glifo>
  );
}

/**
 * LabelModal — alta de una etiqueta (grupo) en la libreta activa y cambio de
 * nombre de una existente (`editing`). El PUT del servicio reemplaza también la
 * pertenencia, así que al renombrar se reenvían SUS miembros: mandar [] dejaría
 * la etiqueta vacía. Vive bajo la Sytue Shell para usar useToast (mismo motivo
 * que VcardToolbar).
 */
function LabelModal({
  t,
  open,
  editing,
  addressBookUrl,
  onClose,
  onSaved,
}: {
  t: T;
  open: boolean;
  editing: ContactGroup | null;
  addressBookUrl: string | null;
  onClose: () => void;
  onSaved: () => void;
}): JSX.Element {
  const toast = useToast();
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // Al (re)abrir: el nombre actual si se renombra, vacío si es un alta.
  useEffect(() => {
    if (!open) return;
    setName(editing?.name ?? '');
    setError(undefined);
  }, [open, editing]);

  async function handleSave(): Promise<void> {
    const trimmed = name.trim();
    if (!trimmed) {
      setError(t('label.nameRequired'));
      return;
    }
    if (!editing && !addressBookUrl) return;
    setSaving(true);
    try {
      if (editing) {
        await updateGroup({ id: editing.id, name: trimmed, members: editing.members });
        toast.notify({ tone: 'success', title: t('label.renamed') });
      } else if (addressBookUrl) {
        await createGroup(addressBookUrl, trimmed);
        toast.notify({ tone: 'success', title: t('label.created') });
      }
      onSaved();
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: editing ? t('label.renameFailed') : t('label.createFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? t('label.renameTitle') : t('label.newTitle')}
      size="sm"
      dismissable={!saving}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => void handleSave()}
            loading={saving}
            disabled={saving}
          >
            {editing ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <FieldGroup label={t('common.name')} htmlFor="g-name" required {...(error ? { error } : {})}>
        <Input
          id="g-name"
          value={name}
          placeholder={t('label.namePlaceholder')}
          autocomplete="off"
          invalid={Boolean(error)}
          aria-describedby={describedBy('g-name', undefined, error)}
          onInput={(e) => {
            setName(e.currentTarget.value);
            setError(undefined);
          }}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              void handleSave();
            }
          }}
        />
      </FieldGroup>
    </Modal>
  );
}

/**
 * AddressBookModal — alta de una libreta y cambio de nombre de una existente
 * (`editing`). Mismo patrón que LabelModal: el servicio soportaba
 * crear/renombrar/borrar libretas (`/api/v1/address-books`) desde el primer
 * día; la interfaz solo sabía listar la que ya hubiera.
 */
function AddressBookModal({
  t,
  open,
  editing,
  onClose,
  onSaved,
}: {
  t: T;
  open: boolean;
  editing: AddressBook | null;
  onClose: () => void;
  onSaved: () => void;
}): JSX.Element {
  const toast = useToast();
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  useEffect(() => {
    if (!open) return;
    setName(editing?.displayName ?? '');
    setError(undefined);
  }, [open, editing]);

  async function handleSave(): Promise<void> {
    const trimmed = name.trim();
    if (!trimmed) {
      setError(t('book.nameRequired'));
      return;
    }
    setSaving(true);
    try {
      if (editing) {
        if (!editing.id) throw new Error('sin id REST');
        await renameAddressBook(editing.id, trimmed);
        toast.notify({ tone: 'success', title: t('book.renamed') });
      } else {
        await createAddressBook(trimmed);
        toast.notify({ tone: 'success', title: t('book.created') });
      }
      onSaved();
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: editing ? t('book.renameFailed') : t('book.createFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? t('book.renameTitle') : t('book.newTitle')}
      size="sm"
      dismissable={!saving}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => void handleSave()}
            loading={saving}
            disabled={saving}
          >
            {editing ? t('common.save') : t('common.create')}
          </Button>
        </>
      }
    >
      <FieldGroup label={t('common.name')} htmlFor="b-name" required {...(error ? { error } : {})}>
        <Input
          id="b-name"
          value={name}
          placeholder={t('book.namePlaceholder')}
          autocomplete="off"
          invalid={Boolean(error)}
          aria-describedby={describedBy('b-name', undefined, error)}
          onInput={(e) => {
            setName(e.currentTarget.value);
            setError(undefined);
          }}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              void handleSave();
            }
          }}
        />
      </FieldGroup>
    </Modal>
  );
}

/**
 * DuplicatesModal — lista los grupos de posibles duplicados detectados en la
 * libreta (fichas que comparten nombre, correo o teléfono). «Combinar» en un
 * grupo abre la MergeModal existente acotada a ese grupo. Vive bajo la Sytue
 * Shell (mismo patrón que las demás modales de esta app).
 */
function DuplicatesModal({
  t,
  open,
  groups,
  onClose,
  onPick,
}: {
  t: T;
  open: boolean;
  groups: Contact[][];
  onClose: () => void;
  onPick: (group: Contact[]) => void;
}): JSX.Element {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('duplicates.title')}
      size="md"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      {groups.length === 0 ? (
        <p class="text-text-muted py-6 text-center text-sm">{t('duplicates.empty')}</p>
      ) : (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-sm">{t('duplicates.description')}</p>
          <ul class="flex flex-col gap-2">
            {groups.map((group) => (
              <li
                key={group.map((c) => c.href).join('|')}
                class="border-border flex items-center justify-between gap-3 rounded-md border p-3"
              >
                <div class="flex min-w-0 items-center gap-2.5">
                  <div class="flex -space-x-2">
                    {group.slice(0, 3).map((c) => (
                      <Avatar key={c.href} name={c.fullName} size="sm" />
                    ))}
                  </div>
                  <div class="min-w-0">
                    <p
                      class="text-text truncate text-sm font-medium"
                      title={group.map((c) => c.fullName).join(', ')}
                    >
                      {group.map((c) => c.fullName).join(', ')}
                    </p>
                    <p class="sytue-meta truncate">
                      {group[0]?.emails[0] ??
                        group[0]?.phones[0] ??
                        group[0]?.organization ??
                        t('duplicates.cards', { count: group.length })}
                    </p>
                  </div>
                </div>
                <Button variant="secondary" size="sm" onClick={() => onPick(group)}>
                  {t('duplicates.merge')}
                </Button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Modal>
  );
}

/**
 * MergeModal — «Combinar con…»: fusiona dos contactos de la MISMA libreta
 * (candidates ya viene acotado a la libreta activa, sin el superviviente).
 * Paso 1: buscador + lista para elegir el 2.º contacto (el que se eliminará).
 * Paso 2: confirmación con elección de nombre/organización si difieren
 * (superviviente por defecto) y resumen del resultado. Vive bajo la Sytue
 * Shell para poder usar useToast (mismo motivo que VcardToolbar/LabelModal).
 */
function MergeModal({
  t,
  survivor,
  candidates,
  onClose,
  onMerge,
}: {
  t: T;
  survivor: Contact | null;
  candidates: Contact[];
  onClose: () => void;
  onMerge: (
    survivor: Contact,
    loser: Contact,
    merged: ContactInput,
  ) => Promise<'merged' | 'duplicate'>;
}): JSX.Element {
  const toast = useToast();
  const [loser, setLoser] = useState<Contact | null>(null);
  const [search, setSearch] = useState('');
  const [keepName, setKeepName] = useState<'survivor' | 'loser'>('survivor');
  const [keepOrg, setKeepOrg] = useState<'survivor' | 'loser'>('survivor');
  const [busy, setBusy] = useState(false);

  const open = survivor !== null;

  // Al (re)abrir, vuelta al paso de selección con las elecciones por defecto.
  useEffect(() => {
    if (!open) return;
    setLoser(null);
    setSearch('');
    setKeepName('survivor');
    setKeepOrg('survivor');
  }, [open]);

  const q = search.trim().toLowerCase();
  const options = q
    ? candidates.filter((c) => contiene(c.fullName, q) || c.emails.some((e) => contiene(e, q)))
    : candidates;

  const merged =
    survivor && loser
      ? buildMergeInput(survivor, loser, {
          ...(keepName === 'loser' ? { fullName: loser.fullName } : {}),
          ...(keepOrg === 'loser' && loser.organization
            ? { organization: loser.organization }
            : {}),
        })
      : null;

  async function handleMerge(): Promise<void> {
    if (!survivor || !loser || !merged) return;
    setBusy(true);
    try {
      const outcome = await onMerge(survivor, loser, merged);
      if (outcome === 'merged') {
        toast.notify({
          tone: 'success',
          title: t('merge.done'),
          description: t('merge.doneDescription', {
            loser: loser.fullName,
            survivor: merged.fullName,
          }),
        });
      } else {
        toast.notify({
          tone: 'warning',
          title: t('merge.partial'),
          description: t('merge.partialDescription', {
            loser: loser.fullName,
            survivor: merged.fullName,
          }),
        });
      }
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('merge.failed'),
        description: t('merge.failedHint'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={loser ? t('merge.confirmTitle') : t('merge.selectTitle')}
      size="md"
      dismissable={!busy}
      footer={
        loser ? (
          <>
            <Button variant="secondary" onClick={() => setLoser(null)} disabled={busy}>
              {t('common.back')}
            </Button>
            <Button
              variant="primary"
              onClick={() => void handleMerge()}
              loading={busy}
              disabled={busy}
            >
              {t('merge.confirm')}
            </Button>
          </>
        ) : (
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
        )
      }
    >
      {survivor && loser && merged ? (
        <div class="flex flex-col gap-4">
          {/* Frase completa desde el catálogo: el orden de «X se añade a Y» no
              es el mismo en las cinco lenguas, así que no se concatena. */}
          <p class="text-text text-base">
            {t('merge.explain', { loser: loser.fullName, survivor: survivor.fullName })}
          </p>
          {survivor.fullName.trim() !== loser.fullName.trim() ? (
            <fieldset class="flex flex-col gap-1.5">
              <legend class="text-text-muted mb-1.5 text-sm font-medium">
                {t('merge.keepName')}
              </legend>
              <Radio
                name="merge-name"
                label={survivor.fullName}
                checked={keepName === 'survivor'}
                onChange={() => setKeepName('survivor')}
              />
              <Radio
                name="merge-name"
                label={loser.fullName}
                checked={keepName === 'loser'}
                onChange={() => setKeepName('loser')}
              />
            </fieldset>
          ) : null}
          {survivor.organization &&
          loser.organization &&
          survivor.organization !== loser.organization ? (
            <fieldset class="flex flex-col gap-1.5">
              <legend class="text-text-muted mb-1.5 text-sm font-medium">
                {t('merge.keepOrganization')}
              </legend>
              <Radio
                name="merge-org"
                label={survivor.organization}
                checked={keepOrg === 'survivor'}
                onChange={() => setKeepOrg('survivor')}
              />
              <Radio
                name="merge-org"
                label={loser.organization}
                checked={keepOrg === 'loser'}
                onChange={() => setKeepOrg('loser')}
              />
            </fieldset>
          ) : null}
          <p class="border-border text-text-muted border-t pt-3 text-sm">
            {t('merge.result', {
              emails: t('merge.resultEmails', { count: merged.emails.length }),
              phones: t('merge.resultPhones', { count: merged.phones.length }),
            })}
          </p>
        </div>
      ) : (
        <div class="flex flex-col gap-3">
          <Input
            type="search"
            placeholder={t('merge.searchPlaceholder')}
            aria-label={t('merge.searchLabel')}
            leadingIcon={<Icon name="search" size={16} />}
            value={search}
            onInput={(e) => setSearch(e.currentTarget.value)}
          />
          {options.length === 0 ? (
            <p class="text-text-muted py-6 text-center text-sm">
              {candidates.length === 0 ? t('merge.noCandidates') : t('merge.noMatches')}
            </p>
          ) : (
            <div class="border-border max-h-80 overflow-y-auto rounded-md border">
              <List selectable label={t('merge.listLabel')}>
                {options.map((c, index) => (
                  <ListItem
                    key={c.href}
                    index={index}
                    class="sytue-focus-ring-inner"
                    onClick={() => setLoser(c)}
                    onKeyDown={(e: KeyboardEvent) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        setLoser(c);
                      }
                    }}
                    leading={<Avatar name={c.fullName} size="sm" />}
                    title={<span title={c.fullName}>{c.fullName}</span>}
                    subtitle={
                      (c.emails[0] ?? c.organization) ? (
                        <span title={c.emails[0] ?? c.organization}>
                          {c.emails[0] ?? c.organization}
                        </span>
                      ) : undefined
                    }
                  />
                ))}
              </List>
            </div>
          )}
        </div>
      )}
    </Modal>
  );
}

/**
 * TrashModal — papelera del usuario: lista los contactos borrados y permite
 * RESTAURAR o ELIMINAR DEFINITIVAMENTE (con confirmación en línea). Reusa el
 * soft-delete existente del backend. Vive bajo la Sytue Shell (useToast).
 */
function TrashModal({
  t,
  open,
  onClose,
  onChanged,
}: {
  t: T;
  open: boolean;
  onClose: () => void;
  onChanged: () => void;
}): JSX.Element {
  const toast = useToast();
  const [items, setItems] = useState<TrashItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmId, setConfirmId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setConfirmId(null);
    setLoading(true);
    setError(false);
    listTrash()
      .then((res) => setItems(res.items))
      .catch(() => setError(true))
      .finally(() => setLoading(false));
  }, [open]);

  async function restore(it: TrashItem): Promise<void> {
    setBusyId(it.id);
    try {
      await restoreContact(it.id);
      setItems((xs) => xs.filter((x) => x.id !== it.id));
      onChanged();
      toast.notify({ tone: 'success', title: t('trash.restored') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('trash.restoreFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function purge(it: TrashItem): Promise<void> {
    setBusyId(it.id);
    try {
      await purgeContact(it.id);
      setItems((xs) => xs.filter((x) => x.id !== it.id));
      setConfirmId(null);
      onChanged();
      toast.notify({ tone: 'success', title: t('trash.purged') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('trash.purgeFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('trash.title')}
      size="md"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div class="flex justify-center py-8">
          <Spinner class="text-text-muted" label={t('trash.loading')} />
        </div>
      ) : error ? (
        <p class="text-text-muted py-6 text-center text-sm" role="alert">
          {t('trash.loadFailed')}
        </p>
      ) : items.length === 0 ? (
        <p class="text-text-muted py-6 text-center text-sm">{t('trash.empty')}</p>
      ) : (
        <div class="flex flex-col gap-2">
          <p class="text-text-muted text-sm">{t('trash.description')}</p>
          <ul class="flex flex-col gap-2">
            {items.map((it) => (
              <li
                key={it.id}
                // En el teléfono, la persona entera arriba y las acciones debajo: en una
                // fila, «Restaurar» y «Eliminar» dejaban el nombre en 102 px y la
                // confirmación lo hacía desaparecer (no se veía a quién se borraba).
                class="border-border flex items-center justify-between gap-3 rounded-md border p-3 max-md:flex-wrap max-md:gap-2"
              >
                <div class="flex min-w-0 items-center gap-2.5 max-md:basis-full">
                  <Avatar name={it.formatted_name} size="sm" />
                  <div class="min-w-0">
                    <p class="text-text truncate text-sm font-medium" title={it.formatted_name}>
                      {it.formatted_name}
                    </p>
                    <p class="sytue-meta truncate">{it.primary_email ?? it.organization ?? '—'}</p>
                  </div>
                </div>
                {confirmId === it.id ? (
                  <div class="flex shrink-0 items-center gap-1.5 max-md:w-full">
                    <span class="text-text-muted text-xs max-md:mr-auto">
                      {t('trash.confirmPurge')}
                    </span>
                    <Button
                      variant="danger"
                      size="sm"
                      id="trash-purge-confirm"
                      loading={busyId === it.id}
                      disabled={busyId !== null}
                      onClick={() => void purge(it)}
                    >
                      {t('trash.confirmYes')}
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      disabled={busyId !== null}
                      onClick={() => {
                        setConfirmId(null);
                        requestAnimationFrame(() =>
                          document.getElementById(`trash-del-${it.id}`)?.focus(),
                        );
                      }}
                    >
                      {t('trash.confirmNo')}
                    </Button>
                  </div>
                ) : (
                  <div class="flex shrink-0 items-center gap-1.5 max-md:w-full max-md:justify-end">
                    <Button
                      variant="secondary"
                      size="sm"
                      loading={busyId === it.id}
                      disabled={busyId !== null}
                      onClick={() => void restore(it)}
                    >
                      {t('trash.restore')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      id={`trash-del-${it.id}`}
                      disabled={busyId !== null}
                      onClick={() => {
                        setConfirmId(it.id);
                        requestAnimationFrame(() =>
                          document.getElementById('trash-purge-confirm')?.focus(),
                        );
                      }}
                    >
                      {t('common.delete')}
                    </Button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </Modal>
  );
}

/**
 * BatchBar — barra contextual de acciones EN LOTE sobre los contactos marcados
 * (multiselección). Aparece solo con selección; anuncia el conteo (aria-live) y
 * ofrece papelera, destacar/quitar destacado y añadir/quitar de etiqueta en lote.
 * Cada acción llama al BFF (una consulta `uid = ANY` en el servicio Go). Vive bajo
 * la Sytue Shell para usar useToast.
 */
function BatchBar({
  t,
  count,
  uids,
  groups,
  onRun,
  onClear,
}: {
  t: T;
  count: number;
  uids: string[];
  groups: ContactGroup[];
  onRun: (fn: () => Promise<unknown>) => Promise<void>;
  onClear: () => void;
}): JSX.Element {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function act(fn: () => Promise<unknown>, okTitle: string): Promise<void> {
    setBusy(true);
    try {
      await onRun(fn);
      toast.notify({ tone: 'success', title: okTitle });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('common.actionFailed'),
        description: t('common.tryAgainSoon'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    // `BulkActionBar` de @sytue/ui y no una barra propia: la de aquí era un
    // `role="region"` con cada acción como parada de tabulación suelta —seis para
    // salir de ella con el teclado— y sin forma de soltar la selección con
    // Escape. La compartida es una `Toolbar`: una sola parada, flechas para
    // moverse dentro y Escape para vaciar, que es como ya funcionaba Sytue Mail.
    // El recuento sigue saliendo de una clave con formas de plural: en inglés y
    // alemán el sustantivo no lleva «s» final y el participio no concuerda.
    <BulkActionBar
      count={count}
      countLabel={t('batch.selected', { count })}
      label={t('batch.region')}
      onClear={onClear}
    >
      <div class="flex-1" />
      <Button
        variant="secondary"
        size="sm"
        disabled={busy}
        onClick={() => void act(() => batchFavorite(uids, true), t('batch.starred', { count }))}
      >
        {t('batch.star')}
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={busy}
        onClick={() => void act(() => batchFavorite(uids, false), t('batch.unstarred'))}
      >
        {t('batch.unstar')}
      </Button>
      {groups.length > 0 ? (
        <>
          <Dropdown
            label={t('batch.addToLabel')}
            align="end"
            trigger={(props) => (
              <Button
                variant="secondary"
                size="sm"
                disabled={busy}
                trailingIcon={<Icon name="chevronDown" size={14} />}
                {...props}
              >
                {t('batch.addToLabel')}
              </Button>
            )}
          >
            {groups.map((g) => (
              <MenuItem
                key={g.id}
                onClick={() =>
                  void act(
                    () => batchGroupMembers(g.id, uids, true),
                    t('batch.addedTo', { label: g.name }),
                  )
                }
              >
                <span class="truncate">{g.name}</span>
              </MenuItem>
            ))}
          </Dropdown>
          <Dropdown
            label={t('batch.removeFromLabel')}
            align="end"
            trigger={(props) => (
              <Button
                variant="secondary"
                size="sm"
                disabled={busy}
                trailingIcon={<Icon name="chevronDown" size={14} />}
                {...props}
              >
                {t('batch.removeFromLabel')}
              </Button>
            )}
          >
            {groups.map((g) => (
              <MenuItem
                key={g.id}
                onClick={() =>
                  void act(
                    () => batchGroupMembers(g.id, uids, false),
                    t('batch.removedFrom', { label: g.name }),
                  )
                }
              >
                <span class="truncate">{g.name}</span>
              </MenuItem>
            ))}
          </Dropdown>
        </>
      ) : null}
      <Button
        variant="danger"
        size="sm"
        disabled={busy}
        onClick={() => void act(() => batchTrash(uids), t('batch.trashed', { count }))}
      >
        {t('batch.toTrash')}
      </Button>
    </BulkActionBar>
  );
}

/** Esqueleto de carga con el mismo ritmo (avatar 32px + dos líneas) que los ítems reales. */
function ContactListSkeleton(): JSX.Element {
  return (
    <div aria-hidden="true">
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} class="flex items-center gap-3 px-3 py-2">
          <Skeleton circle width="2rem" height="2rem" />
          <div class="flex min-w-0 flex-1 flex-col gap-1.5 py-0.5">
            <Skeleton width={i % 2 ? '55%' : '70%'} height="0.75rem" />
            <Skeleton width={i % 2 ? '80%' : '65%'} height="0.625rem" />
          </div>
        </div>
      ))}
    </div>
  );
}
