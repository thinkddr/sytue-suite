/**
 * UsersPanel — directorio de usuarios (API de administración de Sytue ID). Tabla
 * con estado, roles, MFA y aprovisionamiento por servicio Sytue (Drive/Chat/Mail).
 * Buscador, filtro por organización y detalle en panel lateral (Modal) con
 * acciones de administración. A escala de cientos de miles de usuarios las filas
 * van VIRTUALIZADAS con VirtualList de @sytue/ui (solo se pinta la ventana
 * visible + overscan), cabecera de columnas fija fuera del scroll (mismo patrón
 * que Sytue Drive) y el directorio se avanza por páginas keyset con «Cargar más»
 * (cursor opaco vía /api/admin/users del BFF). docs/CONVENTIONS.md §8.
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Icon,
  Input,
  Select,
  Avatar,
  Modal,
  Switch,
  SectionHeader,
  ToastProvider,
  VirtualList,
  useToast,
} from '@sytue/ui';
import { formatDate, formatDateTime, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import UserMailboxes from './UserMailboxes.js';
import type { DirectoryUser, SytueRole, SytueService, Tenant } from '../lib/types.js';
import type { PendingInvitation, UserImpact } from '../lib/admin.js';
import { pedirReautenticacion } from '../lib/reautenticar.js';
import {
  formatWhen,
  provisionLabelKey,
  provisionTone,
  roleLabelKey,
  roleTone,
} from '../lib/format.js';

/** Query de una página del directorio: término de búsqueda y cursor keyset. */
function paginaQuery(q: string, cursor: string): string {
  const p = new URLSearchParams();
  if (q) p.set('q', q);
  if (cursor) p.set('cursor', cursor);
  return p.toString();
}

// Nombres de producto: no se traducen (son marcas).
const SERVICES: { key: SytueService; label: string }[] = [
  { key: 'drive', label: 'Drive' },
  { key: 'chat', label: 'Chat' },
  { key: 'mail', label: 'Mail' },
];

const ROLES: SytueRole[] = ['owner', 'admin', 'member', 'billing'];

/** Fila del directorio (la pinta y mide VirtualList). Misma retícula que la cabecera. */
function UserRow({
  user: u,
  onSelect,
  marcado,
  onMarcar,
  t,
}: {
  user: DirectoryUser;
  onSelect: (u: DirectoryUser) => void;
  marcado: boolean;
  onMarcar: (id: string, marcado: boolean) => void;
  t: T;
}) {
  return (
    <div class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 transition-colors">
      {/*
        La casilla va SUELTA del botón que abre la ficha: anidarla dentro haría
        que marcar a alguien abriera su ficha, y elegir a treinta personas se
        convertiría en abrir y cerrar treinta paneles.
      */}
      <Checkbox
        label=""
        aria-label={t('users.bulk.select', { name: u.fullName })}
        checked={marcado}
        class="shrink-0"
        onChange={(e) => onMarcar(u.id, e.currentTarget.checked)}
      />
      <span class="min-w-0 flex-1">
        <button
          type="button"
          class="group sytue-focus-ring -mx-1 flex w-full items-center gap-2.5 rounded-md px-1 py-0.5 text-left"
          onClick={() => onSelect(u)}
          aria-haspopup="dialog"
          aria-label={t('users.openRecord', { name: u.fullName })}
        >
          <Avatar name={u.fullName} size="sm" />
          <span class="min-w-0">
            <span
              class="text-text block truncate font-medium group-hover:underline"
              title={u.fullName}
            >
              {u.fullName}
            </span>
            <span class="text-text-muted block truncate text-sm" title={u.email}>
              {u.email}
            </span>
          </span>
        </button>
      </span>
      <span class="hidden w-44 shrink-0 flex-wrap gap-1 lg:flex">
        {u.roles.map((r) => (
          <Badge key={r} tone={roleTone[r]}>
            {t(roleLabelKey[r])}
          </Badge>
        ))}
      </span>
      <span class="text-text-muted hidden w-36 shrink-0 truncate font-mono text-sm md:block">
        org/{u.tenant}
      </span>
      <span class="hidden w-20 shrink-0 sm:block">
        {u.mfaEnabled ? (
          <Badge tone="success">{t('users.mfaOn')}</Badge>
        ) : (
          <Badge tone="warning">{t('users.mfaOff')}</Badge>
        )}
      </span>
      <span class="hidden w-44 shrink-0 flex-wrap gap-1 xl:flex">
        {SERVICES.map((e) => (
          <Badge
            key={e.key}
            tone={provisionTone[u.provisioning[e.key]]}
            title={`${e.label}: ${t(provisionLabelKey[u.provisioning[e.key]])}`}
          >
            {e.label}
          </Badge>
        ))}
      </span>
      <span class="w-24 shrink-0">
        <Badge tone={u.enabled ? 'success' : 'neutral'}>
          {u.enabled ? t('users.statusActive') : t('users.statusDisabled')}
        </Badge>
      </span>
    </div>
  );
}

/**
 * Qué se lleva por delante la acción, con números del servidor.
 *
 * Sin esto la confirmación decía «¿seguro?», que no aporta nada que quien la
 * pulsa no supiera ya. Lo que hace parar a tiempo es «se cerrarán 4 sesiones» o
 * «es el único propietario». Mientras el dato no llega no se enseña nada: un
 * «0 sesiones» provisional que luego cambia a 4 es peor que esperar medio
 * segundo, porque el primero se lee como un hecho.
 */
function ImpactoLista({ impacto, t }: { impacto: UserImpact | null; t: T }) {
  if (!impacto) return null;
  return (
    <ul class="text-text-muted flex list-disc flex-col gap-0.5 pl-5 text-sm">
      <li>{t('users.impact.sessions', { count: impacto.active_sessions })}</li>
      {impacto.groups > 0 ? <li>{t('users.impact.groups', { count: impacto.groups })}</li> : null}
      {impacto.last_owner ? <li class="text-warning-text">{t('users.impact.lastOwner')}</li> : null}
    </ul>
  );
}

interface UsersPanelProps {
  users: DirectoryUser[];
  tenants: Tenant[];
  /** Cursor keyset de la página siguiente ('' = directorio completo cargado). */
  nextCursor?: string;
  /** false = Sytue ID no contestó y esto es el seed de demostración. */
  live?: boolean;
  /** Idioma ya negociado en el servidor; llega por props. */
  locale: Locale;
}

function Inner({
  users,
  tenants,
  nextCursor = '',
  live = true,
  locale,
  t,
}: UsersPanelProps & { t: T }) {
  const toast = useToast();
  const [items, setItems] = useState<DirectoryUser[]>(users);
  const [query, setQuery] = useState('');
  const [tenant, setTenant] = useState('all');
  const [selected, setSelected] = useState<DirectoryUser | null>(null);
  const [cursor, setCursor] = useState(nextCursor);
  const [loadingMore, setLoadingMore] = useState(false);
  const [buscando, setBuscando] = useState(false);
  // Selección para acciones en lote. Se guardan IDS y no objetos: las filas se
  // reemplazan en cada cambio optimista, y una selección de objetos viejos
  // acabaría aplicando la acción sobre datos que ya no son los de la pantalla.
  const [marcados, setMarcados] = useState<Set<string>>(new Set());
  const [enLote, setEnLote] = useState(false);
  // Reset de contraseña por admin (panel inline dentro de la ficha).
  const [resetting, setResetting] = useState(false);
  const [newPw, setNewPw] = useState('');
  const [confirmPw, setConfirmPw] = useState('');
  const [savingPw, setSavingPw] = useState(false);
  const [sendingLink, setSendingLink] = useState(false);
  // Restablecer MFA (destructivo: confirmación inline dentro de la ficha).
  const [confirmingMfa, setConfirmingMfa] = useState(false);
  const [resettingMfa, setResettingMfa] = useState(false);
  // Cambiar rol / quitar miembro dentro de la ficha.
  const [savingRole, setSavingRole] = useState(false);
  const [confirmingRemove, setConfirmingRemove] = useState(false);
  // Qué pasa de verdad si se toca a este usuario. Se pide al abrir la ficha, no
  // al pulsar: si llegara después del clic, la confirmación aparecería primero
  // vacía y luego cambiaría de texto bajo el dedo.
  const [impacto, setImpacto] = useState<UserImpact | null>(null);
  // Departamento / puesto / ubicación en edición. Lleva el id de la persona
  // dentro para no necesitar un efecto que lo reinicie al cambiar de ficha: un
  // borrador de otra persona se descarta al comparar, y un cambio optimista de
  // rol o de estado no pisa lo que se está tecleando.
  const [borrador, setBorrador] = useState<{
    id: string;
    department: string;
    title: string;
    location: string;
  } | null>(null);
  const [savingAttrs, setSavingAttrs] = useState(false);
  const [confirmandoSuspender, setConfirmandoSuspender] = useState(false);
  const [removing, setRemoving] = useState(false);
  // Invitar usuario (modal aparte).
  const [inviteOpen, setInviteOpen] = useState(false);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState<SytueRole>('member');
  const [inviting, setInviting] = useState(false);
  // Dirección de correo del invitado. Vacía = patrón nombre.apellidos, que es lo
  // normal; el campo existe para las excepciones (`ventas@`, un homónimo). El
  // dominio se pide una vez y se enseña pegado al campo, para que quien escribe
  // vea la dirección entera que va a crear y no tenga que imaginársela.
  const [inviteMailbox, setInviteMailbox] = useState('');
  // Contraseña inicial OPCIONAL. Existe porque el correo se cae, y entonces una
  // invitación deja a alguien esperando un enlace que no llega mientras quien la
  // envió cree que ya está dentro. Vacía = invitación normal, que sigue siendo lo
  // recomendado: así la contraseña la elige quien la va a usar.
  const [invitePassword, setInvitePassword] = useState('');
  const [mailDomain, setMailDomain] = useState('');
  // Invitaciones emitidas y sin aceptar. Se sabían enviar y no se podían ver: una
  // enviada a la dirección equivocada no había forma de quitarla, solo esperar a
  // que caducara sola. `null` = todavía no ha llegado la lista; [] = no hay
  // ninguna, y entonces la sección no se pinta.
  const [pendientes, setPendientes] = useState<PendingInvitation[] | null>(null);
  const [revocando, setRevocando] = useState('');

  function closeReset() {
    setResetting(false);
    setNewPw('');
    setConfirmPw('');
  }

  // Depende del ID y no del objeto: `selected` se reemplaza en cada cambio
  // optimista (suspender, cambiar rol), y con él en las dependencias el impacto
  // se volvería a pedir justo después de cada acción, sin que nada haya cambiado.
  const seleccionadoID = selected?.id ?? '';
  useEffect(() => {
    if (!seleccionadoID) {
      setImpacto(null);
      return;
    }
    let vigente = true;
    void (async () => {
      try {
        const res = await fetch(`/api/admin/users/${encodeURIComponent(seleccionadoID)}/impact`, {
          headers: { accept: 'application/json' },
        });
        if (!res.ok || !vigente) return;
        setImpacto((await res.json()) as UserImpact);
      } catch {
        // Sin impacto la confirmación sigue saliendo, solo que sin números: es
        // información de apoyo, no un permiso.
      }
    })();
    return () => {
      vigente = false;
    };
  }, [seleccionadoID]);

  function closeModal() {
    setSelected(null);
    setConfirmandoSuspender(false);
    closeReset();
    setConfirmingRemove(false);
    setConfirmingMfa(false);
    setBorrador(null);
  }

  /**
   * Guarda departamento, puesto y ubicación. Sytue ID los aplica y REEVALÚA los
   * grupos dinámicos de esta persona en la misma transacción, así que quien
   * cumpla ahora la regla de su departamento entra en el grupo al guardar.
   *
   * Son los tres atributos con los que se escriben esas reglas y no había ningún
   * sitio donde ponerlos: se podía crear «departamento = Ventas» y la vista
   * previa devolvía cero personas para siempre.
   */
  async function saveAttrs(
    u: DirectoryUser,
    d: { department: string; title: string; location: string },
  ) {
    if (savingAttrs) return;
    setSavingAttrs(true);
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/profile`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(d),
      });
      if (!res.ok) throw new Error(String(res.status));
      // Se toma lo que devuelve el servicio, no lo tecleado: Sytue ID normaliza
      // (recorta, acota), y quedarse con el texto de la caja enseñaría un valor
      // que no es el que se ha guardado.
      const saved = (await res.json()) as {
        department?: string;
        title?: string;
        location?: string;
      };
      const next = {
        department: saved.department ?? '',
        jobTitle: saved.title ?? '',
        location: saved.location ?? '',
      };
      setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, ...next } : it)));
      setSelected((s) => (s && s.id === u.id ? { ...s, ...next } : s));
      setBorrador(null);
      toast.notify({
        tone: 'success',
        title: t('users.attrs.saved'),
        description: t('users.attrs.savedHint', { email: u.email }),
      });
    } catch (err) {
      actionError(err, t('common.saveFailed'));
    } finally {
      setSavingAttrs(false);
    }
  }

  // Página siguiente por cursor keyset vía el BFF (el token nunca está aquí).
  // Lleva el término vigente: sin él, la segunda página de una búsqueda traería
  // el directorio entero y los resultados se mezclarían con quien no encaja.
  async function loadMore() {
    if (!cursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const res = await fetch(`/api/admin/users?${paginaQuery(aplicada.current, cursor)}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const page = (await res.json()) as { items?: DirectoryUser[]; nextCursor?: string };
      setItems((prev) => [...prev, ...(page.items ?? [])]);
      setCursor(typeof page.nextCursor === 'string' ? page.nextCursor : '');
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('users.toast.loadMoreFailed'),
        description: t('users.toast.loadMoreFailedHint'),
      });
    } finally {
      setLoadingMore(false);
    }
  }

  // Término que produjo lo que hay en pantalla, y último pedido al servidor. El
  // segundo descarta respuestas que llegan tarde: con dos búsquedas en vuelo, la
  // primera en volver no tiene por qué ser la que el admin está escribiendo.
  const aplicada = useRef('');
  const pedido = useRef('');

  // La búsqueda la resuelve el SERVIDOR. Filtrar aquí solo miraba las filas ya
  // Las invitaciones pendientes se piden AL CARGAR la pantalla y no al abrir un
  // modal: son parte de lo que hay que saber del directorio —quién tiene una
  // puerta abierta sin haber entrado— y no un dato de apoyo de otra acción.
  async function cargarPendientes(): Promise<void> {
    try {
      const res = await fetch('/api/admin/invitations', {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) return;
      const data = (await res.json()) as { items?: PendingInvitation[] };
      setPendientes(Array.isArray(data.items) ? data.items : []);
    } catch {
      // Sin la lista, la pantalla es la de antes: el directorio y el botón de
      // invitar. No se avisa de nada porque no hay nada que arreglar desde aquí.
    }
  }

  useEffect(() => {
    void cargarPendientes();
    // Solo al montar: la lista se refresca sola al invitar y al revocar, que son
    // las dos cosas que la cambian desde esta pantalla.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** Revoca la invitación de un correo. El servicio es idempotente: `revoked:false`
   *  significa que ya no estaba —aceptada o caducada—, y eso no es un error. */
  async function revocarInvitacion(email: string): Promise<void> {
    if (revocando) return;
    setRevocando(email);
    try {
      const res = await fetch('/api/admin/invitations', {
        method: 'DELETE',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { revoked?: boolean };
      toast.notify({
        tone: data.revoked ? 'success' : 'warning',
        title: data.revoked ? t('users.pending.revoked') : t('users.pending.gone'),
        description: data.revoked
          ? t('users.pending.revokedHint', { email })
          : t('users.pending.goneHint'),
      });
      await cargarPendientes();
    } catch {
      toast.notify({ tone: 'danger', title: t('users.pending.revokeFailed') });
    } finally {
      setRevocando('');
    }
  }

  // Dominio de correo de la organización, para enseñarlo pegado al campo de la
  // dirección. Se pide UNA vez al abrir el modal de invitación y no al cargar la
  // pantalla: es un dato que solo se usa ahí, y quien viene a mirar el directorio
  // no tiene por qué pagar una petición que casi nunca va a necesitar.
  useEffect(() => {
    if (!inviteOpen || mailDomain) return;
    void (async () => {
      try {
        const res = await fetch('/api/admin/mailboxes', {
          headers: { accept: 'application/json' },
        });
        if (!res.ok) return;
        const data = (await res.json()) as { mail_domain?: string };
        if (typeof data.mail_domain === 'string') setMailDomain(data.mail_domain);
      } catch {
        // Sin dominio el campo sigue sirviendo: Sytue ID lo completa igual. No se
        // avisa de nada porque no hay nada que el administrador pueda arreglar.
      }
    })();
  }, [inviteOpen, mailDomain]);

  // cargadas —cincuenta de golpe—, así que en una organización grande el
  // buscador contestaba «no hay nadie» de casi todo el mundo. Se espera al
  // teclado 250 ms para no lanzar una consulta por pulsación.
  // El cuerpo va DENTRO del efecto: una función declarada fuera se recrea en
  // cada render, así que ponerla en las dependencias relanzaría la búsqueda sin
  // parar y dejarla fuera obliga a silenciar el aviso del linter.
  useEffect(() => {
    const q = query.trim();
    if (q === aplicada.current) return;
    const id = setTimeout(() => {
      void (async () => {
        pedido.current = q;
        setBuscando(true);
        try {
          const res = await fetch(`/api/admin/users?${paginaQuery(q, '')}`, {
            headers: { accept: 'application/json' },
          });
          if (!res.ok) throw new Error(String(res.status));
          const page = (await res.json()) as { items?: DirectoryUser[]; nextCursor?: string };
          if (pedido.current !== q) return; // ya hay otra búsqueda más nueva en vuelo
          aplicada.current = q;
          setItems(page.items ?? []);
          setCursor(typeof page.nextCursor === 'string' ? page.nextCursor : '');
        } catch {
          toast.notify({
            tone: 'danger',
            title: t('users.toast.searchFailed'),
            description: t('common.tryAgain'),
          });
        } finally {
          if (pedido.current === q) setBuscando(false);
        }
      })();
    }, 250);
    return () => clearTimeout(id);
  }, [query, t, toast]);

  function marcar(id: string, on: boolean) {
    setMarcados((prev) => {
      const s = new Set(prev);
      if (on) s.add(id);
      else s.delete(id);
      return s;
    });
  }

  /**
   * Aplica una acción a los marcados. La respuesta es una LISTA —a unos se les
   * aplica y a otros no, por motivos legítimos—, así que el aviso dice cuántos
   * de cuántos en vez de un «hecho» que sería falso para los que fallaron.
   */
  async function enLoteAplicar(action: string, role?: string) {
    if (marcados.size === 0 || enLote) return;
    setEnLote(true);
    try {
      const res = await fetch('/api/admin/users/bulk', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action, user_ids: [...marcados], ...(role ? { role } : {}) }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { applied?: number; results?: { ok?: boolean }[] };
      const total = data.results?.length ?? marcados.size;
      const hechos = data.applied ?? 0;
      toast.notify({
        tone: hechos === total ? 'success' : 'warning',
        title: t('users.bulk.done', { count: hechos }),
        ...(hechos < total
          ? { description: t('users.bulk.partial', { count: total - hechos }) }
          : {}),
      });
      setMarcados(new Set());
      // Se vuelve a pedir la primera página en vez de recargar la ventana: media
      // pantalla ha cambiado de estado y reconstruirla a mano desde la respuesta
      // sería una segunda fuente de verdad, pero un `location.reload()` se lleva
      // por delante el aviso que acaba de salir —justo el «28 de 30», que es lo
      // único que dice qué pasó— y además borra el término de búsqueda con el
      // que se eligió a esas treinta personas.
      const page = await fetch(`/api/admin/users?${paginaQuery(aplicada.current, '')}`, {
        headers: { accept: 'application/json' },
      })
        .then((r) =>
          r.ok ? (r.json() as Promise<{ items?: DirectoryUser[]; nextCursor?: string }>) : null,
        )
        .catch(() => null);
      if (page?.items) {
        setItems(page.items);
        // El cursor vuelve con la lista: quedarse con el de la última página
        // cargada haría que «Cargar más» saltara justo las que se acaban de
        // dejar de pintar.
        setCursor(typeof page.nextCursor === 'string' ? page.nextCursor : '');
      }
    } catch (err) {
      actionError(err, t('users.bulk.failed'));
    } finally {
      setEnLote(false);
    }
  }

  // El filtro por organización se queda en el cliente: el servicio ya acota a la
  // organización activa del token, así que es un refinado de lo que hay, no una
  // consulta distinta.
  const filtered = useMemo(
    () => (tenant === 'all' ? items : items.filter((u) => u.tenant === tenant)),
    [items, tenant],
  );

  // Clave estable por fila: la caché de medición del virtualizador sobrevive a
  // los cambios de filtro sin saltos de scroll.
  const getKey = useMemo(() => (i: number) => filtered[i]?.id ?? i, [filtered]);

  // Denegación por permiso (no puedes tocar a un owner / a ti mismo): el BFF
  // propaga el 403 de Sytue ID; mensaje honesto en vez de un genérico.
  function actionError(err: unknown, title: string) {
    const code = err instanceof Error ? err.message : '';
    toast.notify({
      tone: 'danger',
      title,
      description: code === '403' ? t('users.toast.forbiddenUser') : t('common.tryAgain'),
    });
  }

  // Las sesiones cerradas se anuncian como FRASE APARTE, no pegadas a la anterior:
  // concatenar trozos de frase rompe el orden de las palabras en otras lenguas.
  function withSessions(main: string, revoked?: number): string {
    return revoked ? `${main} ${t('users.sessionsRevoked', { count: revoked })}` : main;
  }

  // Suspender/reactivar la cuenta (real, vía BFF). Optimista: refleja el cambio y
  // revierte si el servicio lo rechaza. Suspender revoca además sus sesiones (el
  // acceso se corta de inmediato, no al caducar el token).
  async function toggleEnabled(u: DirectoryUser) {
    const suspend = u.enabled; // habilitado ahora ⇒ la acción es suspender
    setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, enabled: !suspend } : it)));
    setSelected((s) => (s && s.id === u.id ? { ...s, enabled: !suspend } : s));
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/status`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ status: suspend ? 'suspended' : 'active' }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json().catch(() => ({}))) as { sessions_revoked?: number };
      toast.notify({
        tone: suspend ? 'warning' : 'success',
        title: suspend ? t('users.toast.suspended') : t('users.toast.reactivated'),
        description: suspend
          ? withSessions(t('users.toast.suspendedHint', { email: u.email }), data.sessions_revoked)
          : t('users.toast.reactivatedHint', { email: u.email }),
      });
    } catch (err) {
      setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, enabled: u.enabled } : it)));
      setSelected((s) => (s && s.id === u.id ? { ...s, enabled: u.enabled } : s));
      actionError(err, t('users.toast.statusFailed'));
    }
  }

  // Cierre forzado de todas las sesiones del usuario (cuenta comprometida) sin
  // cambiar su estado: obliga a volver a iniciar sesión en toda la suite.
  async function forceLogout(u: DirectoryUser) {
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/sessions`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json().catch(() => ({}))) as { sessions_revoked?: number };
      toast.notify({
        tone: 'success',
        title: t('users.toast.sessionsClosed'),
        description: t('users.toast.sessionsClosedHint', {
          email: u.email,
          count: data.sessions_revoked ?? 0,
        }),
      });
    } catch (err) {
      actionError(err, t('users.toast.sessionsFailed'));
    }
  }

  /*
    Aquí había un botón «Re-aprovisionar» que decía «SCIM volverá a sincronizar a
    esta persona en los servicios Sytue» y NO llamaba a nada: pintaba el aviso y
    se acababa ahí. Ni existe ese endpoint en Sytue ID ni SCIM está encendido en
    producción, así que cada pulsación dejaba a quien administra creyendo que
    acababa de arreglar un aprovisionamiento que nadie iba a tocar. Es el mismo
    control fingido que el distintivo «SCIM activo» que ya se retiró del menú:
    vuelve cuando haya una reconciliación de verdad que disparar.
  */

  // Reset de contraseña por admin (vía BFF). Fija la contraseña dada y revoca las
  // sesiones del usuario; nunca se registra ni se muestra. La validación real la
  // hace Sytue ID (política de dominio); aquí solo un mínimo espejo para el UX.
  async function resetPassword(u: DirectoryUser) {
    if (newPw.length < 8 || newPw !== confirmPw || savingPw) return;
    setSavingPw(true);
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/password`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ new_password: newPw }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json().catch(() => ({}))) as { sessions_revoked?: number };
      toast.notify({
        tone: 'success',
        title: t('users.toast.pwReset'),
        description: withSessions(
          t('users.toast.pwResetHint', { email: u.email }),
          data.sessions_revoked,
        ),
      });
      closeReset();
    } catch (err) {
      actionError(err, t('users.toast.pwResetFailed'));
    } finally {
      setSavingPw(false);
    }
  }

  // Enviar un enlace de restablecimiento por email: el usuario elige su propia
  // contraseña (el admin no la ve). Alternativa a fijar una contraseña.
  async function sendResetLink(u: DirectoryUser) {
    if (sendingLink) return;
    setSendingLink(true);
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/reset-password`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error(String(res.status));
      toast.notify({
        tone: 'success',
        title: t('users.toast.linkSent'),
        description: t('users.toast.linkSentHint', { email: u.email }),
      });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('users.toast.linkFailed'),
        description:
          code === '409'
            ? t('users.toast.ssoOnly')
            : code === '403'
              ? t('users.toast.forbiddenUser')
              : t('common.tryAgain'),
      });
    } finally {
      setSendingLink(false);
    }
  }

  // Restablecer el MFA del usuario (limpia TOTP + WebAuthn). Destructivo: hasta
  // que vuelva a configurar 2FA su cuenta queda sin segundo factor. Refleja
  // mfaEnabled=false en la lista y la ficha al confirmar el servicio.
  async function resetMfa(u: DirectoryUser) {
    if (resettingMfa) return;
    setResettingMfa(true);
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/reset-mfa`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, mfaEnabled: false } : it)));
      setSelected((s) => (s && s.id === u.id ? { ...s, mfaEnabled: false } : s));
      toast.notify({
        tone: 'success',
        title: t('users.toast.mfaReset'),
        description: t('users.toast.mfaResetHint', { email: u.email }),
      });
      setConfirmingMfa(false);
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('users.toast.mfaResetFailed'),
        description:
          code === '404'
            ? t('users.toast.notInOrg')
            : code === '403'
              ? t('users.toast.forbiddenUser')
              : t('common.tryAgain'),
      });
    } finally {
      setResettingMfa(false);
    }
  }

  // Cambiar el rol del usuario en la organización (optimista con revert). Una
  // degradación revoca sus sesiones (lo hace Sytue ID); el guard del último admin
  // puede rechazar (409).
  async function changeRole(u: DirectoryUser, role: SytueRole) {
    const previous = u.roles[0] ?? 'member';
    if (role === previous || savingRole) return;
    setSavingRole(true);
    setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, roles: [role] } : it)));
    setSelected((s) => (s && s.id === u.id ? { ...s, roles: [role] } : s));
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/role`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ role }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json().catch(() => ({}))) as { sessions_revoked?: number };
      toast.notify({
        tone: 'success',
        title: t('users.toast.roleUpdated'),
        // Se interpola la etiqueta del rol tal cual (en alemán los sustantivos van
        // en mayúscula: pasarla por toLowerCase sería un error de ortografía).
        description: withSessions(
          t('users.toast.roleUpdatedHint', { email: u.email, role: t(roleLabelKey[role]) }),
          data.sessions_revoked,
        ),
      });
    } catch (err) {
      setItems((prev) => prev.map((it) => (it.id === u.id ? { ...it, roles: [previous] } : it)));
      setSelected((s) => (s && s.id === u.id ? { ...s, roles: [previous] } : s));
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('users.toast.roleFailed'),
        description:
          code === '409'
            ? t('users.toast.lastAdmin')
            : code === '403'
              ? t('users.toast.forbiddenUser')
              : t('common.tryAgain'),
      });
    } finally {
      setSavingRole(false);
    }
  }

  // Quitar al usuario de la organización: revoca sus sesiones y lo saca de la
  // lista. El guard del último admin puede rechazar (409).
  async function removeFromOrg(u: DirectoryUser) {
    if (removing) return;
    setRemoving(true);
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(u.id)}/membership`, {
        method: 'DELETE',
      });
      // Quitar a alguien exige autenticación reciente: a confirmarla, y de vuelta aquí.
      if (await pedirReautenticacion(res)) return;
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json().catch(() => ({}))) as { sessions_revoked?: number };
      setItems((prev) => prev.filter((it) => it.id !== u.id));
      toast.notify({
        tone: 'success',
        title: t('users.toast.removed'),
        description: withSessions(
          t('users.toast.removedHint', { email: u.email }),
          data.sessions_revoked,
        ),
      });
      closeModal();
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('users.toast.removeFailed'),
        description:
          code === '409'
            ? t('users.toast.removeLastAdmin')
            : code === '403'
              ? t('users.toast.forbiddenUser')
              : t('common.tryAgain'),
      });
      setConfirmingRemove(false);
    } finally {
      setRemoving(false);
    }
  }

  // Invitar por email a la organización activa: Sytue ID emite el token y envía el
  // enlace de aceptación donde el invitado fija su contraseña.
  async function submitInvite(e: Event) {
    e.preventDefault();
    const email = inviteEmail.trim();
    if (!email.includes('@') || inviting) return;
    setInviting(true);
    try {
      const res = await fetch('/api/admin/invitations', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          email,
          role: inviteRole,
          mailbox_local: inviteMailbox.trim(),
          ...(invitePassword ? { password: invitePassword } : {}),
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      // Con contraseña no se ha enviado ningún correo: decir «le hemos enviado
      // una invitación» sería mentir, y encima haría esperar un mensaje que no
      // existe.
      const conPassword = invitePassword !== '';
      toast.notify({
        tone: 'success',
        title: conPassword ? t('users.toast.created') : t('users.toast.invited'),
        description: conPassword
          ? t('users.toast.createdHint', { email })
          : t('users.toast.invitedHint', {
              email,
              role: t(roleLabelKey[inviteRole]),
            }),
      });
      setInviteOpen(false);
      setInviteEmail('');
      setInviteRole('member');
      setInviteMailbox('');
      setInvitePassword('');
      // Con contraseña la cuenta ya existe y no hay invitación que listar; sin
      // ella acaba de aparecer una pendiente. Se recarga en los dos casos porque
      // la lista es la fuente, no lo que creamos aquí.
      void cargarPendientes();
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      // El 503 NO es culpa de la dirección: la invitación se creó y lo que falló
      // fue el envío. Decir «revisa el correo» mandaba a corregir un dato que
      // estaba bien, y callaba lo único accionable — que la persona invitada
      // existe ya en la lista y que basta con reinvitar cuando el correo vuelva.
      const esEnvio = code === '503' || code === '502';
      toast.notify({
        tone: esEnvio ? 'warning' : 'danger',
        title: esEnvio ? t('users.toast.inviteNotSent') : t('users.toast.inviteFailed'),
        description: esEnvio
          ? t('users.toast.inviteNotSentHint')
          : code === '409'
            ? t('users.toast.alreadyMember')
            : code === '403'
              ? t('users.toast.roleForbidden')
              : t('users.toast.checkEmail'),
      });
      if (esEnvio) {
        // La invitación ESTÁ creada: cerrar el formulario evita que se reintente
        // creyendo que no pasó nada, que es como se acaban acumulando cinco
        // invitaciones a la misma persona.
        setInviteOpen(false);
        setInviteEmail('');
        setInviteMailbox('');
        setInvitePassword('');
      }
    } finally {
      setInviting(false);
    }
  }

  return (
    // h-full + min-h-0: la lista virtualizada necesita altura acotada; el scroll
    // vive DENTRO del VirtualList, no en el main.
    <div class="flex h-full min-h-0 flex-col">
      <SectionHeader
        level={1}
        class="shrink-0"
        title={t('page.users')}
        subtitle={t('users.subtitle')}
      />
      {/*
        Sin Sytue ID esto es el seed de demostración, y se dice: una lista de
        gente inventada se lee igual que la plantilla real, así que quien entra
        cree estar mirando su organización y sale de aquí con una idea falsa de
        quién tiene MFA o quién sigue activo.
      */}
      {!live ? (
        <p class="border-border bg-warning-bg text-warning-text mx-4 mt-3 shrink-0 rounded-md border px-3 py-2 text-sm">
          {t('users.offline')}
        </p>
      ) : null}
      <div class="border-border bg-surface flex shrink-0 flex-wrap items-center gap-2 border-b px-4 py-2.5">
        <Input
          type="search"
          value={query}
          placeholder={t('users.searchPlaceholder')}
          aria-label={t('users.searchLabel')}
          leadingIcon={<Icon name="search" size={16} />}
          class="w-60"
          onInput={(e) => setQuery(e.currentTarget.value)}
        />
        <Select
          aria-label={t('users.filterTenant')}
          value={tenant}
          class="w-48"
          onChange={(e) => setTenant(e.currentTarget.value)}
        >
          <option value="all">{t('users.allTenants')}</option>
          {tenants.map((org) => (
            <option key={org.slug} value={org.slug}>
              {org.name}
            </option>
          ))}
        </Select>
        <Button
          variant="primary"
          class="ml-auto"
          leadingIcon={<Icon name="plus" size={16} />}
          onClick={() => setInviteOpen(true)}
        >
          {t('users.invite')}
        </Button>
      </div>

      {/*
        Barra de lote. Aparece SOLO con alguien marcado y en el sitio donde se
        está mirando: una fila de botones permanentemente encendida invita a
        pulsarlos sin selección, y suspender a nadie no es un no-op inofensivo,
        es un susto. Las acciones destructivas van en `secondary`, no en rojo:
        aquí lo que asusta debe ser el número de la izquierda, no el color.
      */}
      {marcados.size > 0 && (
        <div
          class="border-border bg-surface-sunken flex shrink-0 flex-wrap items-center gap-2 border-b px-4 py-2"
          role="status"
        >
          <strong class="text-text text-sm">
            {t('users.bulk.selected', { count: marcados.size })}
          </strong>
          <Button
            size="sm"
            variant="secondary"
            disabled={enLote}
            onClick={() => void enLoteAplicar('suspend')}
          >
            {t('users.bulk.suspend')}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={enLote}
            onClick={() => void enLoteAplicar('reactivate')}
          >
            {t('users.bulk.reactivate')}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={enLote}
            onClick={() => void enLoteAplicar('revoke_sessions')}
          >
            {t('users.bulk.revokeSessions')}
          </Button>
          <Select
            aria-label={t('users.bulk.setRole')}
            class="w-40"
            value=""
            disabled={enLote}
            onChange={(e) => {
              const rol = e.currentTarget.value;
              e.currentTarget.value = '';
              if (rol) void enLoteAplicar('set_role', rol);
            }}
          >
            <option value="">{t('users.bulk.setRole')}</option>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {t(roleLabelKey[r])}
              </option>
            ))}
          </Select>
          <Button
            size="sm"
            variant="ghost"
            class="ml-auto"
            disabled={enLote}
            onClick={() => setMarcados(new Set())}
          >
            {t('users.bulk.clear')}
          </Button>
        </div>
      )}

      {/* Invitaciones emitidas y sin aceptar. Solo se pinta si hay alguna: en una
          organización asentada lo normal es que no haya ninguna, y una caja vacía
          permanente es ruido que se aprende a no mirar. */}
      {pendientes && pendientes.length > 0 && (
        <section
          aria-labelledby="pendientes-titulo"
          class="border-border bg-surface mx-4 mb-0 rounded-lg border"
        >
          <div class="border-border flex flex-wrap items-baseline gap-x-2 gap-y-1 border-b px-3 py-2.5">
            <h2 id="pendientes-titulo" class="text-text text-sm font-semibold">
              {t('users.pending.title')}
            </h2>
            <span class="text-text-muted text-sm">({pendientes.length})</span>
            <p class="text-text-muted w-full text-sm sm:w-auto sm:flex-1">
              {t('users.pending.hint')}
            </p>
          </div>
          <ul class="divide-border divide-y">
            {pendientes.map((inv) => (
              <li key={inv.email} class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                <span class="text-text min-w-0 flex-1 truncate text-sm">{inv.email}</span>
                <Badge tone="neutral">{t(roleLabelKey[inv.role] ?? 'users.role.member')}</Badge>
                <span class="text-text-muted text-sm">
                  {t('users.pending.expires', {
                    date: formatDate(locale, new Date(inv.expires_at)),
                  })}
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  loading={revocando === inv.email}
                  disabled={revocando !== ''}
                  onClick={() => void revocarInvitacion(inv.email)}
                >
                  {t('users.pending.revoke')}
                </Button>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Caja de tabla: abraza el contenido con pocas filas y, cuando crece,
          se acota al alto disponible (flex-shrink + min-h-0) y desplaza dentro. */}
      <div class="border-border bg-surface m-4 flex min-h-0 flex-col overflow-hidden rounded-lg border">
        <div
          aria-hidden="true"
          class="border-border bg-surface-sunken text-text-muted sytue-overline flex shrink-0 items-center gap-3 border-b px-3 py-2.5"
        >
          <span class="min-w-0 flex-1">{t('users.col.user')}</span>
          <span class="hidden w-44 shrink-0 lg:block">{t('users.col.roles')}</span>
          <span class="hidden w-36 shrink-0 md:block">{t('users.col.tenant')}</span>
          <span class="hidden w-20 shrink-0 sm:block">MFA</span>
          {/* Cabecera larga en alemán/francés: truncada, con el nombre completo en title. */}
          <span class="hidden w-44 shrink-0 truncate xl:block" title={t('users.col.provisioning')}>
            {t('users.col.provisioning')}
          </span>
          <span class="w-24 shrink-0">{t('users.col.status')}</span>
        </div>
        {filtered.length === 0 ? (
          <EmptyState
            title={buscando ? t('common.loading') : t('users.empty.title')}
            // Con búsqueda puesta, «no hay nadie» ya no significa «esta
            // organización está vacía»: lo dice, y así no parece un fallo.
            description={
              buscando
                ? ''
                : aplicada.current
                  ? t('users.empty.noMatch')
                  : t('users.empty.description')
            }
            illustration={<Icon name="user" size={28} />}
            action={
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setQuery('');
                  setTenant('all');
                }}
              >
                {t('users.empty.clearFilters')}
              </Button>
            }
          />
        ) : (
          <VirtualList
            items={filtered}
            estimateSize={57}
            getItemKey={getKey}
            aria-label={t('users.listLabel')}
            class="min-h-0 text-base"
            renderItem={(u) => (
              <UserRow
                user={u}
                onSelect={setSelected}
                marcado={marcados.has(u.id)}
                onMarcar={marcar}
                t={t}
              />
            )}
          />
        )}
        {cursor ? (
          <div class="border-border bg-surface flex shrink-0 items-center justify-center border-t px-3 py-2">
            <Button
              variant="secondary"
              size="sm"
              disabled={loadingMore}
              onClick={() => void loadMore()}
            >
              {loadingMore ? t('common.loading') : t('common.loadMore')}
            </Button>
          </div>
        ) : null}
      </div>

      <Modal
        open={Boolean(selected)}
        onClose={closeModal}
        title={selected?.fullName ?? t('users.modalFallbackTitle')}
        size="md"
      >
        {selected ? (
          <div class="flex flex-col gap-4">
            <div class="flex min-w-0 items-center gap-3">
              <Avatar name={selected.fullName} size="lg" />
              <div class="min-w-0">
                <p class="text-text truncate font-semibold" title={selected.fullName}>
                  {selected.fullName}
                </p>
                <p class="text-text-muted truncate text-sm" title={selected.email}>
                  {selected.email}
                </p>
                <p class="sytue-meta truncate font-mono">
                  {selected.username} · {selected.id}
                </p>
              </div>
            </div>

            <dl class="grid grid-cols-2 gap-3 text-sm">
              <div>
                <dt class="text-text-muted">{t('users.col.tenant')}</dt>
                <dd class="text-text mt-0.5 font-mono">org/{selected.tenant}</dd>
              </div>
              <div>
                <dt class="text-text-muted">{t('users.detail.lastLogin')}</dt>
                <dd class="text-text mt-0.5 tabular-nums">
                  {formatWhen(locale, selected.lastLogin)}
                </dd>
              </div>
              <div>
                <dt class="text-text-muted">{t('users.detail.created')}</dt>
                <dd class="text-text mt-0.5 tabular-nums">
                  {formatDateTime(locale, selected.createdAt)}
                </dd>
              </div>
              <div>
                <dt class="text-text-muted">{t('users.detail.emailVerified')}</dt>
                <dd class="text-text mt-0.5">
                  {selected.emailVerified ? t('common.yes') : t('common.no')}
                </dd>
              </div>
            </dl>

            <UserMailboxes userId={selected.id} t={t} />

            {/*
              Los tres atributos que evalúan las reglas de grupo dinámico. Van
              aquí, en la ficha de la persona, y no en la pantalla de grupos:
              quien crea la regla la escribe una vez, y esto se rellena cada vez
              que alguien cambia de departamento.
            */}
            {(() => {
              const d =
                borrador?.id === selected.id
                  ? borrador
                  : {
                      id: selected.id,
                      department: selected.department,
                      title: selected.jobTitle,
                      location: selected.location,
                    };
              const sucio =
                d.department !== selected.department ||
                d.title !== selected.jobTitle ||
                d.location !== selected.location;
              const campos = [
                { key: 'department', label: 'rule.attr.department' },
                { key: 'title', label: 'rule.attr.title' },
                { key: 'location', label: 'rule.attr.location' },
              ] as const;
              return (
                <div>
                  <p class="text-text mb-1 text-sm font-semibold">{t('users.attrs.title')}</p>
                  <div class="grid gap-2 sm:grid-cols-3">
                    {campos.map((c) => (
                      <div key={c.key}>
                        <label class="text-text-muted mb-0.5 block text-xs" for={`attr-${c.key}`}>
                          {t(c.label)}
                        </label>
                        <Input
                          id={`attr-${c.key}`}
                          value={d[c.key]}
                          disabled={savingAttrs}
                          onInput={(e) => setBorrador({ ...d, [c.key]: e.currentTarget.value })}
                        />
                      </div>
                    ))}
                  </div>
                  <p class="text-text-muted mt-1 text-xs">{t('users.attrs.hint')}</p>
                  {/* El botón solo aparece con algo cambiado: un «Guardar» siempre
                      encendido en una ficha que ya se guarda sola por todas partes
                      invita a pulsarlo para comprobar si hacía falta. */}
                  {sucio ? (
                    <div class="mt-2 flex gap-2">
                      <Button
                        variant="primary"
                        size="sm"
                        loading={savingAttrs}
                        onClick={() =>
                          void saveAttrs(selected, {
                            department: d.department,
                            title: d.title,
                            location: d.location,
                          })
                        }
                      >
                        {t('common.save')}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={savingAttrs}
                        onClick={() => setBorrador(null)}
                      >
                        {t('common.cancel')}
                      </Button>
                    </div>
                  ) : null}
                </div>
              );
            })()}

            <div>
              <p class="text-text mb-1 text-sm font-semibold">{t('users.col.roles')}</p>
              <div class="flex flex-wrap gap-1">
                {selected.roles.map((r) => (
                  <Badge key={r} tone={roleTone[r]}>
                    {t(roleLabelKey[r])}
                  </Badge>
                ))}
              </div>
            </div>

            <div>
              <p class="text-text mb-1 text-sm font-semibold">{t('nav.groups')}</p>
              <div class="flex flex-wrap gap-1">
                {selected.groups.map((g) => (
                  <Badge key={g} tone="neutral">
                    {g}
                  </Badge>
                ))}
              </div>
            </div>

            <div>
              <p class="text-text mb-1 text-sm font-semibold">{t('users.detail.provisioning')}</p>
              <ul class="flex flex-col gap-1">
                {SERVICES.map((e) => (
                  <li key={e.key} class="flex items-center justify-between text-base">
                    <span class="text-text">{e.label}</span>
                    <Badge tone={provisionTone[selected.provisioning[e.key]]}>
                      {t(provisionLabelKey[selected.provisioning[e.key]])}
                    </Badge>
                  </li>
                ))}
              </ul>
            </div>

            <div class="bg-bg-subtle flex flex-col gap-2 rounded-md px-3 py-2">
              <div class="flex items-center justify-between gap-3">
                <label class="text-text text-sm font-medium" for="member-role">
                  {t('users.roleInOrg')}
                </label>
                <Select
                  id="member-role"
                  class="w-44"
                  value={selected.roles[0] ?? 'member'}
                  disabled={savingRole}
                  onChange={(e) => void changeRole(selected, e.currentTarget.value as SytueRole)}
                >
                  {ROLES.map((r) => (
                    <option key={r} value={r}>
                      {t(roleLabelKey[r])}
                    </option>
                  ))}
                </Select>
              </div>
              {/*
                Quitarle el rol al único propietario deja la organización sin
                nadie que pueda administrarla. El backend lo rechaza (409), pero
                enterarse ANTES ahorra el intento y el mensaje de error.
              */}
              {impacto?.last_owner ? (
                <p class="text-warning-text text-sm">{t('users.impact.lastOwner')}</p>
              ) : null}
              {/*
                Suspender era un interruptor que se aplicaba al soltarlo: corta
                el acceso a las diez aplicaciones y cierra las sesiones abiertas,
                y no había nada entre el dedo y eso. Reactivar sí sigue siendo
                inmediato — devolver el acceso no es el riesgo.
              */}
              {selected.enabled ? (
                confirmandoSuspender ? (
                  <div class="border-border flex flex-col gap-2 rounded-md border p-3">
                    <p class="text-text text-sm font-medium">
                      {t('users.suspend.question', { email: selected.email })}
                    </p>
                    <ImpactoLista impacto={impacto} t={t} />
                    <div class="flex gap-2">
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => {
                          setConfirmandoSuspender(false);
                          void toggleEnabled(selected);
                        }}
                      >
                        {t('users.suspend.action')}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setConfirmandoSuspender(false)}
                      >
                        {t('common.cancel')}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Switch
                    label={t('users.accessEnabled')}
                    checked
                    onChange={() => setConfirmandoSuspender(true)}
                  />
                )
              ) : (
                <Switch
                  label={t('users.accessEnabled')}
                  checked={false}
                  onChange={() => void toggleEnabled(selected)}
                />
              )}
              <div class="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" onClick={() => void forceLogout(selected)}>
                  {t('users.action.forceLogout')}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  aria-expanded={resetting}
                  aria-controls={resetting ? 'reset-pw-panel' : undefined}
                  onClick={() => (resetting ? closeReset() : setResetting(true))}
                >
                  {t('users.action.resetPassword')}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  loading={sendingLink}
                  onClick={() => void sendResetLink(selected)}
                >
                  {t('users.action.sendResetLink')}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  aria-expanded={confirmingMfa}
                  aria-controls={confirmingMfa ? 'reset-mfa-panel' : undefined}
                  onClick={() => setConfirmingMfa((v) => !v)}
                >
                  {t('users.action.resetMfa')}
                </Button>
              </div>

              {confirmingMfa ? (
                <div
                  id="reset-mfa-panel"
                  class="border-border mt-1 flex flex-col gap-2 border-t pt-3"
                >
                  <p class="text-text-muted text-sm">
                    {t('users.resetMfa.warning', { email: selected.email })}
                  </p>
                  <div class="flex gap-2">
                    <Button
                      variant="danger"
                      size="sm"
                      loading={resettingMfa}
                      onClick={() => void resetMfa(selected)}
                    >
                      {t('users.action.resetMfa')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => setConfirmingMfa(false)}>
                      {t('common.cancel')}
                    </Button>
                  </div>
                </div>
              ) : null}

              {resetting ? (
                <div
                  id="reset-pw-panel"
                  class="border-border mt-1 flex flex-col gap-2 border-t pt-3"
                >
                  <p class="text-text-muted text-sm">
                    {t('users.resetPw.intro', { email: selected.email })}
                  </p>
                  <label class="text-text text-sm font-medium" for="reset-new-pw">
                    {t('users.resetPw.new')}
                  </label>
                  <Input
                    id="reset-new-pw"
                    type="password"
                    autocomplete="new-password"
                    value={newPw}
                    invalid={newPw.length > 0 && newPw.length < 8}
                    onInput={(e) => setNewPw(e.currentTarget.value)}
                  />
                  <label class="text-text text-sm font-medium" for="reset-confirm-pw">
                    {t('users.resetPw.repeat')}
                  </label>
                  <Input
                    id="reset-confirm-pw"
                    type="password"
                    autocomplete="new-password"
                    value={confirmPw}
                    invalid={confirmPw.length > 0 && confirmPw !== newPw}
                    onInput={(e) => setConfirmPw(e.currentTarget.value)}
                  />
                  {newPw.length > 0 && newPw.length < 8 ? (
                    <p class="text-danger-text text-xs">
                      {t('users.resetPw.tooShort', { count: 8 })}
                    </p>
                  ) : confirmPw.length > 0 && confirmPw !== newPw ? (
                    <p class="text-danger-text text-xs">{t('users.resetPw.mismatch')}</p>
                  ) : null}
                  <div class="flex gap-2">
                    <Button
                      variant="primary"
                      size="sm"
                      loading={savingPw}
                      disabled={newPw.length < 8 || newPw !== confirmPw}
                      onClick={() => void resetPassword(selected)}
                    >
                      {t('common.save')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={closeReset}>
                      {t('common.cancel')}
                    </Button>
                  </div>
                </div>
              ) : null}

              <div class="border-border mt-1 border-t pt-3">
                {confirmingRemove ? (
                  <div class="flex flex-col gap-2">
                    <p class="text-text-muted text-sm">
                      {t('users.remove.confirmQuestion', { email: selected.email })}
                    </p>
                    <ImpactoLista impacto={impacto} t={t} />
                    <div class="flex gap-2">
                      <Button
                        variant="danger"
                        size="sm"
                        id="remove-confirm"
                        loading={removing}
                        onClick={() => void removeFromOrg(selected)}
                      >
                        {t('users.remove.action')}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setConfirmingRemove(false);
                          requestAnimationFrame(() =>
                            document.getElementById('remove-trigger')?.focus(),
                          );
                        }}
                      >
                        {t('common.cancel')}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Button
                    variant="ghost"
                    size="sm"
                    class="text-danger-text"
                    id="remove-trigger"
                    aria-expanded={confirmingRemove}
                    onClick={() => {
                      setConfirmingRemove(true);
                      requestAnimationFrame(() =>
                        document.getElementById('remove-confirm')?.focus(),
                      );
                    }}
                  >
                    {t('users.remove.action')}
                  </Button>
                )}
              </div>
            </div>
          </div>
        ) : null}
      </Modal>

      <Modal
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        title={t('users.invite')}
        size="sm"
      >
        <form class="flex flex-col gap-3" onSubmit={(e) => void submitInvite(e)}>
          <p class="text-text-muted text-sm">{t('users.inviteIntro')}</p>
          <div>
            <label class="text-text mb-1 block text-sm font-medium" for="invite-email">
              {t('users.inviteEmail')}
            </label>
            <Input
              id="invite-email"
              type="email"
              autocomplete="off"
              placeholder={t('users.inviteEmailPlaceholder')}
              value={inviteEmail}
              onInput={(e) => setInviteEmail(e.currentTarget.value)}
              required
            />
          </div>
          <div>
            <label class="text-text mb-1 block text-sm font-medium" for="invite-mailbox">
              {t('users.inviteMailbox')}
            </label>
            <div class="flex items-center gap-1.5">
              <Input
                id="invite-mailbox"
                autocomplete="off"
                placeholder={t('users.inviteMailboxPlaceholder')}
                value={inviteMailbox}
                onInput={(e) => setInviteMailbox(e.currentTarget.value)}
              />
              {mailDomain ? (
                <span class="text-text-muted shrink-0 font-mono text-sm">@{mailDomain}</span>
              ) : null}
            </div>
            <p class="text-text-muted mt-1 text-xs">{t('users.inviteMailboxHint')}</p>
          </div>
          <div>
            <label class="text-text mb-1 block text-sm font-medium" for="invite-password">
              {t('users.invitePassword')}
            </label>
            <Input
              id="invite-password"
              type="password"
              autocomplete="new-password"
              placeholder={t('users.invitePasswordPlaceholder')}
              value={invitePassword}
              onInput={(e) => setInvitePassword(e.currentTarget.value)}
            />
            <p class="text-text-muted mt-1 text-xs">{t('users.invitePasswordHint')}</p>
          </div>
          <div>
            <label class="text-text mb-1 block text-sm font-medium" for="invite-role">
              {t('users.inviteRoleLabel')}
            </label>
            <Select
              id="invite-role"
              value={inviteRole}
              onChange={(e) => setInviteRole(e.currentTarget.value as SytueRole)}
            >
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {t(roleLabelKey[r])}
                </option>
              ))}
            </Select>
          </div>
          <div class="mt-1 flex justify-end gap-2">
            <Button variant="ghost" size="sm" type="button" onClick={() => setInviteOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              type="submit"
              loading={inviting}
              disabled={!inviteEmail.includes('@')}
            >
              {t('users.inviteSubmit')}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

export default function UsersPanel({ users, tenants, nextCursor, live, locale }: UsersPanelProps) {
  return (
    <ToastProvider>
      <Inner
        users={users}
        tenants={tenants}
        nextCursor={nextCursor ?? ''}
        live={live ?? true}
        locale={locale}
        t={useT(locale)}
      />
    </ToastProvider>
  );
}
