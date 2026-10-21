/**
 * PlatformPanel — consola de ADMINISTRACIÓN GLOBAL de Sytue.
 *
 * La única vista del producto que ve todas las organizaciones: alta manual,
 * suspensión, plazas gratuitas (gestos comerciales y empleados), fusión y
 * medición de uso. Todo pasa por /api/platform (BFF) → /api/v1/platform (Go),
 * que exige estar en `platform_admins` y haber entrado con segundo factor.
 *
 * Cada acción que escribe PIDE UN MOTIVO. No es burocracia: seis meses después,
 * una organización suspendida o con veinte plazas regaladas sin motivo escrito
 * es indistinguible de un error, y quien lo mire no tendrá a quién preguntar.
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  EmptyState,
  FieldGroup,
  Input,
  Modal,
  Table,
  Tabs,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type ConsoleMessageKey as MessageKey, type T } from '../i18n/index.js';
import LogsPanel from './LogsPanel.js';
import VersionsPanel from './VersionsPanel.js';
import { CrearOrganizacion } from './CrearOrganizacion.js';
import { PlatformMiembros } from './PlatformMiembros.js';
import { r } from '../lib/rutas.js';
import type {
  OrgMerge,
  PlatformActivation,
  PlatformAdmin,
  PlatformApproval,
  PlatformAuditRow,
  PlatformOrg,
  PlatformUsage,
  SeatGrant,
} from '../lib/platform.js';
import { contiene } from '@sytue/i18n';

interface Props {
  locale: Locale;
  usage: PlatformUsage | null;
  orgs: PlatformOrg[];
  merges: OrgMerge[];
  admins: PlatformAdmin[];
  /** Histórico de elevaciones. Vacío mientras no se esté elevado: va tras el gate. */
  activations?: PlatformActivation[];
  /** Primera página del registro de plataforma, y el cursor de la siguiente. */
  audit?: PlatformAuditRow[];
  auditCursor?: string;
  /** Solicitudes de operaciones irreversibles, pendientes e históricas. */
  approvals?: PlatformApproval[];
  /** Cada cuántos días hay que volver a mirar quién tiene alcance global. */
  reviewDays?: number;
  /** La elevación viva de quien mira, si la tiene. */
  activation?: PlatformActivation | null;
  /** Motivo por el que no hay datos (403 sin MFA, 503 sin backend…). */
  problema?: number | undefined;
  /** Código del error, que separa los tres 403 posibles. */
  motivo?: string | undefined;
}

async function call(path: string, method: string, body?: unknown): Promise<Response> {
  const res = await fetch(`/api/platform${path}`, {
    method,
    ...(body
      ? { headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) }
      : {}),
  });
  // 403 con la pantalla abierta = la elevación ha caducado. Se recarga para
  // volver al gate en vez de seguir sondeando en balde (ver LogsPanel, que lo
  // explica entero).
  if (res.status === 403) window.location.reload();
  return res;
}

/**
 * Rehidrata una lista mientras tenga algo EN CURSO, y recarga la página cuando
 * deja de tenerlo.
 *
 * Las tareas de esta consola no terminan cuando responde el botón: una fusión la
 * ejecuta un proceso aparte que va marcando servicio a servicio, y una solicitud
 * de segunda firma la resuelve OTRA persona desde su propia pantalla. El panel
 * pintaba el estado que había al abrirlo y no volvía a preguntar nunca, así que
 * la única forma de enterarse era recargar a mano — y hasta entonces la pantalla
 * decía algo que ya era falso.
 *
 * Sondear es la vía correcta aquí y no un flujo de eventos: son minutos, no
 * milisegundos, y no hay nada que mirar el 99 % del tiempo. Por eso el intervalo
 * SOLO existe mientras queda trabajo vivo y se apaga en cuanto acaba: un panel
 * de administración se deja abierto días enteros, y un sondeo perpetuo son miles
 * de peticiones para no enseñar ningún cambio.
 *
 * Al terminar se recarga UNA vez, en vez de conformarse con la lista sondeada:
 * cuando una fusión acaba también cambian la organización de origen, el aforo y
 * el registro, y refrescar solo esta lista dejaría el resto de la pantalla
 * diciendo lo de antes.
 */
function useTrabajoVivo<T>(inicial: T[], ruta: string, vivo: (x: T) => boolean): T[] {
  const [lista, setLista] = useState(inicial);
  const hayVivos = lista.some(vivo);
  const huboTrabajoRef = useRef(false);

  useEffect(() => {
    if (!hayVivos) {
      // De «había trabajo» a «no queda»: acaba de terminar algo.
      if (huboTrabajoRef.current) window.location.reload();
      return;
    }
    huboTrabajoRef.current = true;
    let cancelado = false;
    const id = setInterval(() => {
      void (async () => {
        const res = await call(ruta, 'GET');
        if (!res.ok || cancelado) return;
        const data = (await res.json()) as { items?: T[] };
        if (Array.isArray(data.items)) setLista(data.items);
      })();
    }, 5000);
    return () => {
      cancelado = true;
      clearInterval(id);
    };
  }, [hayVivos, ruta]);

  return lista;
}

/** Barra de aforo: usuarios activos frente a plazas efectivas. */
function Capacity({ org }: { org: PlatformOrg }) {
  const pct =
    org.seats_effective > 0
      ? Math.min(100, Math.round((org.active_users / org.seats_effective) * 100))
      : 100;
  return (
    <div class="sy-capacity">
      <span class="sy-capacity__label">
        {org.active_users} / {org.seats_effective}
        {org.seats_granted > 0 ? ` (+${org.seats_granted})` : ''}
      </span>
      <span class="sy-capacity__track" aria-hidden="true">
        <span
          class={`sy-capacity__fill ${org.over_capacity ? 'sy-capacity__fill--over' : ''}`}
          style={{ width: `${pct}%` }}
        />
      </span>
    </div>
  );
}

/** Tono del estado de una organización. Fuera del JSX para no anidar ternarios. */
function statusTone(status: PlatformOrg['status']): 'success' | 'warning' | 'neutral' {
  if (status === 'active') return 'success';
  if (status === 'suspended') return 'warning';
  return 'neutral';
}

function mergeTone(status: OrgMerge['status']): 'success' | 'danger' | 'neutral' {
  if (status === 'completed') return 'success';
  if (status === 'failed') return 'danger';
  return 'neutral';
}

// ---------------------------------------------------------------------------
// Elevación temporal del alcance global (PIM, mig 0043). Estar en la allowlist
// solo permite PEDIRLA; lo que autoriza es una activación viva, con motivo y
// ventana, que caduca sola.
// ---------------------------------------------------------------------------

/** Ventanas ofrecidas, en minutos. El backend acepta de 5 a 480. */
const VENTANAS = [15, 30, 60, 120, 240, 480];

function ventanaLabel(min: number, t: T): string {
  return min < 60
    ? t('platform.pim.duration.minutes', { count: min })
    : t('platform.pim.duration.hours', { count: min / 60 });
}

/**
 * Minutos que quedan de elevación. Se recalcula cada 30 s en vez de mostrar el
 * valor con el que se pintó: el panel se deja abierto durante horas, y un «te
 * quedan 60 minutos» congelado es peor que no decir nada. Al llegar a cero
 * recarga, para que sea el servidor quien decida otra vez qué enseñar en lugar
 * de dejar botones que ya solo saben devolver 403.
 */
function useMinutosRestantes(expiresAt: string): number {
  const fin = useMemo(() => new Date(expiresAt).getTime(), [expiresAt]);
  const [min, setMin] = useState(() => Math.ceil((fin - Date.now()) / 60_000));
  useEffect(() => {
    const id = setInterval(() => {
      const v = Math.ceil((fin - Date.now()) / 60_000);
      setMin(v);
      if (v <= 0) window.location.reload();
    }, 30_000);
    return () => clearInterval(id);
  }, [fin]);
  return min;
}

/** Barra permanente mientras se está elevado: qué se pidió, cuánto queda y salida. */
function ElevationBar({
  activation,
  t,
  toast,
}: {
  activation: PlatformActivation;
  t: T;
  toast: Toast;
}) {
  const restan = useMinutosRestantes(activation.expires_at);
  const [saliendo, setSaliendo] = useState(false);

  async function bajar() {
    setSaliendo(true);
    const res = await call('/deactivate', 'POST', {});
    if (res.ok) {
      toast.notify({ title: t('platform.pim.toast.steppedDown'), tone: 'success' });
      window.location.reload();
    } else {
      setSaliendo(false);
      toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
    }
  }

  return (
    <div class="sy-elevation" role="status">
      <Badge tone="warning">{t('platform.pim.active')}</Badge>
      <span class="sy-elevation__reason">{activation.justification}</span>
      <span class="sy-elevation__left">
        {restan >= 1
          ? t('platform.pim.remaining', { count: restan })
          : t('platform.pim.expiringSoon')}
      </span>
      <Button size="sm" variant="secondary" disabled={saliendo} onClick={() => void bajar()}>
        {t('platform.pim.stepDown')}
      </Button>
    </div>
  );
}

/** Petición de elevación: motivo obligatorio y ventana. */
function ElevateModal({ t, onClose, toast }: { t: T; onClose: () => void; toast: Toast }) {
  const [motivo, setMotivo] = useState('');
  const [minutos, setMinutos] = useState(60);
  const [saving, setSaving] = useState(false);
  const invalid = motivo.trim() === '';

  async function elevar() {
    if (invalid || saving) return;
    setSaving(true);
    const res = await call('/activate', 'POST', {
      justification: motivo.trim(),
      duration_minutes: minutos,
    });
    if (res.ok) {
      toast.notify({ title: t('platform.pim.toast.elevated'), tone: 'success' });
      window.location.reload();
      return;
    }
    setSaving(false);
    // El mensaje del backend responde a lo que se acaba de escribir («la ventana
    // va de 5 minutos a 8 horas»); un «no se pudo» genérico deja probando a ciegas.
    const cuerpo = (await res.json().catch(() => ({}))) as { message?: string };
    toast.notify({ title: cuerpo.message || t('platform.toast.failed'), tone: 'danger' });
  }

  return (
    <Modal open title={t('platform.pim.modalTitle')} onClose={onClose}>
      <div class="sy-form">
        <label>
          {t('platform.pim.justification')}
          <Input value={motivo} onInput={(e) => setMotivo((e.target as HTMLInputElement).value)} />
          <small class="sy-hint">{t('platform.pim.justificationHint')}</small>
        </label>
        <label>
          {t('platform.pim.duration')}
          <select
            class="sy-select"
            value={String(minutos)}
            onChange={(e) => setMinutos(Number((e.target as HTMLSelectElement).value))}
          >
            {VENTANAS.map((m) => (
              <option key={m} value={m}>
                {ventanaLabel(m, t)}
              </option>
            ))}
          </select>
        </label>
        <div class="sy-form__actions">
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={invalid || saving} onClick={() => void elevar()}>
            {t('platform.pim.elevate')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/** Puerta de quien PUEDE entrar pero no está elevado: la única con salida. */
function ElevationGate({ t, toast }: { t: T; toast: Toast }) {
  const [abierto, setAbierto] = useState(false);
  return (
    <>
      <EmptyState
        title={t('platform.pim.gateTitle')}
        description={t('platform.pim.gateBody')}
        action={<Button onClick={() => setAbierto(true)}>{t('platform.pim.elevate')}</Button>}
      />
      {abierto && <ElevateModal t={t} toast={toast} onClose={() => setAbierto(false)} />}
    </>
  );
}

function activationTone(s: PlatformActivation['effective_status']): 'warning' | 'neutral' {
  return s === 'active' ? 'warning' : 'neutral';
}

/** Histórico: quién estuvo elevado, para qué y cuándo. */
function ActivationHistory({
  activations,
  locale,
  t,
}: {
  activations: PlatformActivation[];
  locale: Locale;
  t: T;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('platform.pim.history')}</CardTitle>
      </CardHeader>
      <CardBody>
        {activations.length === 0 ? (
          <EmptyState title={t('platform.pim.history.empty')} />
        ) : (
          <Table<PlatformActivation>
            rowKey={(a) => a.id}
            rows={activations}
            caption={t('platform.pim.history')}
            columns={[
              {
                key: 'who',
                header: t('platform.pim.col.who'),
                cell: (a) => a.user_email || a.user_id,
              },
              {
                key: 'reason',
                header: t('platform.col.reason'),
                cell: (a) => a.justification,
              },
              {
                key: 'when',
                header: t('platform.pim.col.when'),
                cell: (a) => new Date(a.activated_at).toLocaleString(locale),
              },
              {
                key: 'window',
                header: t('platform.pim.col.window'),
                cell: (a) => ventanaLabel(a.duration_minutes, t),
              },
              {
                key: 'state',
                header: t('platform.col.status'),
                cell: (a) => (
                  <Badge tone={activationTone(a.effective_status)}>
                    {t(`platform.pim.state.${a.effective_status}`)}
                  </Badge>
                ),
              },
            ]}
          />
        )}
      </CardBody>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Revisión periódica del alcance global (mig 0047). El PIM hizo que el alcance
// no valga sin activarlo; esto pone un momento en el que alguien vuelve a mirar
// si la ELEGIBILIDAD sigue teniendo sentido.
// ---------------------------------------------------------------------------

/** Quién tiene alcance global, cuándo se revisó y quién puede confirmarlo. */
function AdminsTable({
  admins,
  reviewDays,
  locale,
  t,
  toast,
}: {
  admins: PlatformAdmin[];
  reviewDays: number;
  locale: Locale;
  t: T;
  toast: Toast;
}) {
  const [ocupado, setOcupado] = useState('');
  // Alta de administrador global. El endpoint existía desde el principio
  // (`POST /platform/admins`) y NO tenía puerta en la interfaz: conceder el
  // permiso más amplio del sistema solo se podía hacer por API, así que en la
  // práctica no se podía hacer. Exige correo y motivo, y —si hay más de un
  // administrador— la firma de una segunda persona.
  const [alta, setAlta] = useState(false);
  const [email, setEmail] = useState('');
  const [motivo, setMotivo] = useState('');
  const [concediendo, setConcediendo] = useState(false);

  async function conceder() {
    if (!email.trim() || !motivo.trim() || concediendo) return;
    setConcediendo(true);
    const res = await call('/admins', 'POST', { email: email.trim(), reason: motivo.trim() });
    setConcediendo(false);
    if (res.ok) {
      toast.notify({ title: t('platform.admins.granted'), tone: 'success' });
      window.location.reload();
      return;
    }
    // 409 = esperando la segunda firma. No es un fallo y decirlo como tal haría
    // que alguien reintentara pensando que no llegó.
    if (res.status === 409) {
      toast.notify({ title: t('platform.approval.pending'), tone: 'info' });
      setAlta(false);
      return;
    }
    const cuerpo = (await res.json().catch(() => ({}))) as { message?: string };
    toast.notify({ title: cuerpo.message ?? t('platform.toast.failed'), tone: 'danger' });
  }

  async function revisar(a: PlatformAdmin) {
    setOcupado(a.user_id);
    const res = await call(`/admins/${a.user_id}/review`, 'POST', {});
    if (res.ok) {
      toast.notify({ title: t('platform.review.done'), tone: 'success' });
      window.location.reload();
      return;
    }
    setOcupado('');
    const cuerpo = (await res.json().catch(() => ({}))) as { code?: string };
    toast.notify({
      title:
        cuerpo.code === 'same_person'
          ? t('platform.review.notYourself')
          : t('platform.toast.failed'),
      tone: 'danger',
    });
  }

  const [adminARevocar, setAdminARevocar] = useState<PlatformAdmin | null>(null);
  const [revocando, setRevocando] = useState(false);

  async function revocar(a: PlatformAdmin) {
    setRevocando(true);
    const res = await call(`/admins/${a.user_id}`, 'DELETE');
    setRevocando(false);
    setAdminARevocar(null);
    if (res.ok) {
      toast.notify({ title: t('platform.toast.adminRevoked'), tone: 'success' });
      window.location.reload();
      return;
    }
    const cuerpo = (await res.json().catch(() => ({}))) as { message?: string };
    toast.notify({ title: cuerpo.message ?? t('platform.toast.failed'), tone: 'danger' });
  }

  return (
    <>
      <div class="mb-3 flex flex-wrap items-end gap-2">
        {alta ? (
          <>
            <label class="flex flex-col gap-1 text-sm">
              {t('platform.admins.email')}
              <Input
                type="email"
                value={email}
                placeholder="persona@sytue.com"
                onInput={(e) => setEmail(e.currentTarget.value)}
              />
            </label>
            <label class="flex min-w-[16rem] flex-1 flex-col gap-1 text-sm">
              {t('platform.admins.reason')}
              <Input
                value={motivo}
                placeholder={t('platform.admins.reasonHint')}
                onInput={(e) => setMotivo(e.currentTarget.value)}
              />
            </label>
            <Button
              onClick={() => void conceder()}
              disabled={concediendo || !email.trim() || !motivo.trim()}
            >
              {t('platform.admins.grant')}
            </Button>
            <Button variant="secondary" onClick={() => setAlta(false)}>
              {t('platform.action.cancel')}
            </Button>
          </>
        ) : (
          <Button onClick={() => setAlta(true)}>{t('platform.admins.add')}</Button>
        )}
      </div>
      <Table<PlatformAdmin>
        rowKey={(a) => a.user_id}
        rows={admins}
        caption={t('platform.admins.title')}
        columns={[
          { key: 'email', header: t('platform.col.email'), cell: (a) => a.email },
          { key: 'reason', header: t('platform.col.reason'), cell: (a) => a.reason || '—' },
          {
            key: 'since',
            header: t('platform.col.since'),
            cell: (a) => new Date(a.created_at).toLocaleDateString(locale),
          },
          {
            key: 'reviewed',
            header: t('platform.review.col.reviewed'),
            // Vencida se marca en ámbar, no en rojo: nadie ha hecho nada malo, es
            // que toca mirarlo. El acceso sigue funcionando.
            cell: (a) =>
              a.review_due ? (
                <Badge tone="warning">{t('platform.review.overdue', { count: reviewDays })}</Badge>
              ) : (
                new Date(a.reviewed_at).toLocaleDateString(locale)
              ),
          },
          {
            key: 'actions',
            header: t('platform.col.actions'),
            align: 'end',
            cell: (a) => (
              <div class="sy-row-actions">
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={ocupado === a.user_id}
                  onClick={() => void revisar(a)}
                >
                  {t('platform.review.confirm')}
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={ocupado === a.user_id}
                  onClick={() => setAdminARevocar(a)}
                >
                  {t('platform.admins.revoke')}
                </Button>
              </div>
            ),
          },
        ]}
      />
      <ConfirmDialog
        open={adminARevocar !== null}
        onClose={() => setAdminARevocar(null)}
        onConfirm={() => {
          if (adminARevocar) void revocar(adminARevocar);
        }}
        title={t('platform.prompt.revokeAdminTitle')}
        confirmLabel={t('platform.admins.revoke')}
        tone="danger"
        confirmDisabled={revocando}
      >
        <p class="sy-hint">{t('platform.prompt.revokeAdminHint')}</p>
        {adminARevocar && <p class="text-sm font-semibold">{adminARevocar.email}</p>}
      </ConfirmDialog>
    </>
  );
}

// ---------------------------------------------------------------------------
// Segunda persona para lo irreversible (mig 0046). Pedir una suspensión o una
// fusión ya no la ejecuta: la deja esperando a que la firme otro admin global.
// ---------------------------------------------------------------------------

/**
 * Lee el 409 `approval_required` de una operación que ha quedado esperando y
 * avisa. Devuelve true si era eso: quien llama no debe recargar ni tratarlo como
 * un fallo, porque la operación no ha fallado — está en cola.
 */
async function quedoEsperando(res: Response, t: T, toast: Toast): Promise<boolean> {
  if (res.status !== 409) return false;
  const cuerpo = (await res.json().catch(() => ({}))) as { code?: string };
  if (cuerpo.code !== 'approval_required') return false;
  toast.notify({
    title: t('platform.approval.queued'),
    description: t('platform.approval.queuedHint'),
    tone: 'success',
  });
  return true;
}

function approvalTone(s: PlatformApproval['effective_status']): 'warning' | 'success' | 'neutral' {
  if (s === 'pending') return 'warning';
  if (s === 'executed') return 'success';
  return 'neutral';
}

/** Qué se pidió, en una línea legible. El payload cambia según la acción. */
function approvalQue(a: PlatformApproval, orgs: PlatformOrg[], t: T): string {
  const slug = (id?: string) => orgs.find((o) => o.id === id)?.slug ?? id ?? '—';
  if (a.action === 'org.merge') {
    return `${slug(a.payload?.source_org_id)} → ${slug(a.payload?.target_org_id)}`;
  }
  if (a.action === 'platform.admin.grant') {
    return (
      (a.payload?.email as string) || (a.payload?.user_id as string) || t('platform.tab.admins')
    );
  }
  return slug(a.payload?.org_id) || t('platform.audit.noOrg');
}

/** Solicitudes pendientes de una segunda firma, y su histórico. */
function ApprovalsPanel({
  approvals,
  orgs,
  locale,
  t,
  toast,
}: {
  approvals: PlatformApproval[];
  orgs: PlatformOrg[];
  locale: Locale;
  t: T;
  toast: Toast;
}) {
  const [ocupada, setOcupada] = useState('');

  async function decidir(a: PlatformApproval, decision: 'approve' | 'reject') {
    setOcupada(a.id);
    const res = await call(`/approvals/${a.id}/${decision}`, 'POST', {});
    if (res.ok) {
      window.location.reload();
      return;
    }
    setOcupada('');
    // El 403 de «no puedes aprobar lo tuyo» es la regla, no una avería: se dice
    // tal cual en vez de un «no se pudo» que parece un error del sistema.
    const cuerpo = (await res.json().catch(() => ({}))) as { code?: string; message?: string };
    toast.notify({
      title:
        cuerpo.code === 'same_person'
          ? t('platform.approval.samePerson')
          : t('platform.toast.failed'),
      ...(cuerpo.code === 'same_person' ? {} : { description: cuerpo.message ?? '' }),
      tone: 'danger',
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('platform.approval.title')}</CardTitle>
      </CardHeader>
      <CardBody>
        <p class="sy-hint">{t('platform.approval.intro')}</p>
        {approvals.length === 0 ? (
          <EmptyState title={t('platform.approval.empty')} />
        ) : (
          <Table<PlatformApproval>
            rowKey={(a) => a.id}
            rows={approvals}
            caption={t('platform.approval.title')}
            columns={[
              {
                key: 'action',
                header: t('platform.audit.col.action'),
                cell: (a) => t(`platform.approval.action.${a.action}`),
              },
              {
                key: 'what',
                header: t('platform.col.org'),
                cell: (a) => approvalQue(a, orgs, t),
              },
              { key: 'reason', header: t('platform.col.reason'), cell: (a) => a.reason || '—' },
              {
                key: 'who',
                header: t('platform.approval.col.requester'),
                cell: (a) => a.requester_email || a.requested_by,
              },
              {
                key: 'when',
                header: t('platform.pim.col.when'),
                cell: (a) => new Date(a.created_at).toLocaleString(locale),
              },
              {
                key: 'status',
                header: t('platform.col.status'),
                cell: (a) => (
                  <Badge tone={approvalTone(a.effective_status)}>
                    {t(`platform.approval.state.${a.effective_status}`)}
                  </Badge>
                ),
              },
              {
                key: 'actions',
                header: t('platform.col.actions'),
                align: 'end',
                cell: (a) =>
                  a.effective_status === 'pending' ? (
                    <div class="sy-row-actions">
                      <Button
                        size="sm"
                        variant="secondary"
                        disabled={ocupada === a.id}
                        onClick={() => void decidir(a, 'reject')}
                      >
                        {t('platform.approval.reject')}
                      </Button>
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={ocupada === a.id}
                        onClick={() => void decidir(a, 'approve')}
                      >
                        {t('platform.approval.approve')}
                      </Button>
                    </div>
                  ) : null,
              },
            ]}
          />
        )}
      </CardBody>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Registro de plataforma: lo que hacen los admin globales, de todas las
// organizaciones a la vez.
// ---------------------------------------------------------------------------

/** Familias de acción. El prefijo lo entiende el filtro `action` de la API. */
const FAMILIAS = [
  '',
  'platform.org.',
  'platform.seats.',
  'platform.merge.',
  'platform.admin.',
  'platform.pim.',
  'platform.approval.',
  'platform.user.',
];

/**
 * Registro de plataforma con «cargar más» por cursor. La acción se enseña en
 * crudo (`platform.org.status`): es un identificador estable que se busca en el
 * SIEM y en el código, y traducirlo lo rompería como pista.
 */
function PlatformAudit({
  rows,
  cursor,
  locale,
  t,
  toast,
}: {
  rows: PlatformAuditRow[];
  cursor: string;
  locale: Locale;
  t: T;
  toast: Toast;
}) {
  const [items, setItems] = useState(rows);
  const [next, setNext] = useState(cursor);
  const [familia, setFamilia] = useState('');
  const [cargando, setCargando] = useState(false);

  async function pedir(accion: string, desde: string) {
    setCargando(true);
    const p = new URLSearchParams();
    if (accion) p.set('action', accion);
    if (desde) p.set('cursor', desde);
    const res = await call(`/audit?${p.toString()}`, 'GET');
    setCargando(false);
    if (!res.ok) {
      toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
      return;
    }
    const page = (await res.json()) as { items?: PlatformAuditRow[]; nextCursor?: string };
    // Cambiar de familia REEMPLAZA; «cargar más» añade. Sin la distinción, el
    // primer filtro dejaba las filas anteriores mezcladas con las nuevas.
    setItems((prev) => (desde ? [...prev, ...(page.items ?? [])] : (page.items ?? [])));
    setNext(typeof page.nextCursor === 'string' ? page.nextCursor : '');
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('platform.audit.title')}</CardTitle>
        <select
          class="sy-select"
          aria-label={t('platform.audit.filter')}
          value={familia}
          onChange={(e) => {
            const v = (e.target as HTMLSelectElement).value;
            setFamilia(v);
            void pedir(v, '');
          }}
        >
          {FAMILIAS.map((f) => (
            <option key={f} value={f}>
              {t(`platform.audit.family.${f || 'all'}` as MessageKey)}
            </option>
          ))}
        </select>
      </CardHeader>
      <CardBody>
        {items.length === 0 ? (
          <EmptyState title={t('platform.audit.empty')} />
        ) : (
          <>
            <Table<PlatformAuditRow>
              rowKey={(a) => a.id}
              rows={items}
              caption={t('platform.audit.title')}
              columns={[
                {
                  key: 'when',
                  header: t('platform.pim.col.when'),
                  cell: (a) => new Date(a.occurred_at).toLocaleString(locale),
                },
                {
                  key: 'who',
                  header: t('platform.pim.col.who'),
                  cell: (a) => a.actor_email || a.actor_id || '—',
                },
                {
                  key: 'action',
                  header: t('platform.audit.col.action'),
                  cell: (a) => <code>{a.action}</code>,
                },
                {
                  key: 'org',
                  // Una acción sin organización (conceder alcance, elevarse) no
                  // es un dato que falte: es que no fue sobre ninguna.
                  header: t('platform.col.org'),
                  cell: (a) => a.org_slug || t('platform.audit.noOrg'),
                },
                {
                  key: 'target',
                  header: t('platform.audit.col.detail'),
                  cell: (a) => a.target || '—',
                },
              ]}
            />
            {next && (
              <div class="sy-form__actions">
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={cargando}
                  onClick={() => void pedir(familia, next)}
                >
                  {cargando ? t('common.loading') : t('common.loadMore')}
                </Button>
              </div>
            )}
          </>
        )}
      </CardBody>
    </Card>
  );
}

function StatCard({ label, value }: { label: string; value: number | string }) {
  return (
    <Card>
      <CardBody>
        <p class="sy-stat__label">{label}</p>
        <p class="sy-stat__value">{value}</p>
      </CardBody>
    </Card>
  );
}

function Inner({
  locale,
  usage,
  orgs,
  merges,
  admins,
  activations = [],
  audit = [],
  auditCursor = '',
  approvals = [],
  reviewDays = 90,
  activation = null,
  problema,
  motivo,
  t,
}: Props & { t: T }) {
  const toast = useToast();
  const [filtro, setFiltro] = useState('');
  // Estos dos van AQUÍ, con el resto, y no junto a `cambiarEstado`, que es donde
  // se usan: más abajo hay un `if (problema) return …` y un hook declarado
  // después de un retorno temprano no se ejecuta en todos los renders. React
  // empareja los hooks POR ORDEN, así que en cuanto la pantalla mostraba un 403
  // el estado se descolocaba y a la vuelta cada `useState` leía el valor del
  // vecino. Lo cazó el gate de lint (rules-of-hooks); el síntoma habría sido un
  // panel que se comporta de forma errática justo después de un error.
  // Tres acciones y no un booleano: al añadir «eliminar», `suspender: false` ya
  // no significaba «reactivar» sino «cualquiera de las otras dos», y el diálogo
  // habría acabado adivinando cuál por el estado de la organización.
  const [cambioEstado, setCambioEstado] = useState<{
    org: PlatformOrg;
    accion: 'suspend' | 'reactivate' | 'delete';
  } | null>(null);
  const [motivoEstado, setMotivoEstado] = useState('');
  const [seatsOrg, setSeatsOrg] = useState<PlatformOrg | null>(null);
  const [detalleOrg, setDetalleOrg] = useState<PlatformOrg | null>(null);
  const [mergeOrg, setMergeOrg] = useState<PlatformOrg | null>(null);

  // Las dos listas que cambian SIN que nadie toque esta pantalla: una fusión la
  // ejecuta un proceso aparte, y una solicitud de segunda firma la resuelve otra
  // persona. Se rehidratan solas mientras quede algo vivo en ellas.
  const mergesVivos = useTrabajoVivo(
    merges,
    '/merges',
    (m: OrgMerge) => m.status === 'pending' || m.status === 'running',
  );
  const approvalsVivas = useTrabajoVivo(
    approvals,
    '/approvals',
    (a: PlatformApproval) => a.effective_status === 'pending',
  );

  // Lo que espera una firma. Se calcula aquí y no en el servidor: la lista ya
  // viene entera y filtrarla dos veces solo podría discrepar.
  const pendientes = useMemo(
    () => approvalsVivas.filter((a) => a.effective_status === 'pending'),
    [approvalsVivas],
  );

  // Accesos globales que tocaba revisar. El servidor ya marca cada fila; aquí
  // solo se cuentan, para no tener dos criterios de «vencido».
  const porRevisar = useMemo(() => admins.filter((a) => a.review_due).length, [admins]);

  const visibles = useMemo(() => {
    const q = filtro.trim().toLowerCase();
    if (!q) return orgs;
    return orgs.filter((o) => contiene(o.slug, q) || contiene(o.name, q));
  }, [orgs, filtro]);

  if (problema) {
    // TRES 403 distintos, tres pantallas distintas. Aplanarlos al mismo texto
    // dejaba a un admin global sin elevar leyendo «no puedes acceder», que es
    // falso y no tiene salida; y a quien de verdad no está en la allowlist
    // buscando un segundo factor que no le habría servido de nada.
    if (problema === 403) {
      // «Puedes, pero no estás elevado»: tiene salida aquí mismo. No se exige
      // además `elegible` —que sale de otra llamada— porque si esa segunda
      // llamada falla por red se perdería la única salida de la pantalla; el
      // botón responderá 403 si de verdad no puede, y eso se lee mejor que una
      // pared.
      if (motivo === 'activation_required') {
        return <ElevationGate t={t} toast={toast} />;
      }
      // Falta el segundo factor: se arregla saliendo y volviendo a entrar.
      if (motivo === 'mfa_required') {
        return (
          <EmptyState
            title={t('platform.error.mfa')}
            description={t('platform.error.mfaHint')}
            action={
              <Button onClick={() => (window.location.href = r('/auth/logout'))}>
                {t('platform.error.signOut')}
              </Button>
            }
          />
        );
      }
      // Y el que no tiene arreglo desde aquí: no estás en la allowlist. Se dice
      // tal cual, sin mandar a nadie a repetir un MFA que no cambia nada.
      return (
        <EmptyState
          title={t('platform.error.forbidden')}
          description={t('platform.error.notAllowlisted')}
        />
      );
    }
    return (
      <EmptyState
        title={t('platform.error.unavailable')}
        description={t('platform.error.unavailableHint')}
      />
    );
  }

  async function reanudar(id: string) {
    const res = await call(`/merges/${id}/resume`, 'POST', {});
    if (res.ok) window.location.reload();
    else toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
  }

  async function cambiarEstado(
    org: PlatformOrg,
    accion: 'suspend' | 'reactivate' | 'delete',
    reason: string,
  ) {
    setCambioEstado(null);
    setMotivoEstado('');
    // Eliminar es un ESTADO, no un borrado de filas: la organización deja de
    // servir y desaparece de la operación, pero su rastro se conserva y se puede
    // deshacer reactivándola. El porqué está en handlePlatformSetOrgStatus.
    const estado = accion === 'suspend' ? 'suspended' : accion === 'delete' ? 'deleted' : 'active';
    const res = await call(`/orgs/${org.id}/status`, 'POST', { status: estado, reason });
    if (res.ok) {
      toast.notify({ title: t('platform.toast.statusSaved'), tone: 'success' });
      window.location.reload();
      return;
    }
    // Suspender y eliminar no se aplican al pedirlos: quedan esperando otra firma.
    if (await quedoEsperando(res, t, toast)) {
      window.location.reload();
      return;
    }
    toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
  }

  return (
    // `sy-platform` no existía en ningún CSS, así que el panel entero se pintaba
    // pegado a los bordes: ni AdminShell ni ConsoleLayout ponen gutter. Los paneles
    // hermanos van a 16px.
    <div class="p-4">
      {activation && <ElevationBar activation={activation} t={t} toast={toast} />}
      {/*
        Las solicitudes esperando firma van ARRIBA y no solo en su pestaña: una
        suspensión pedida hace veinte horas caduca sin que nadie la vea si hay
        que acordarse de ir a buscarla.
      */}
      {pendientes.length > 0 && (
        <div class="sy-elevation" role="status">
          <Badge tone="warning">
            {t('platform.approval.waiting', { count: pendientes.length })}
          </Badge>
          <span class="sy-elevation__reason">{t('platform.approval.waitingHint')}</span>
        </div>
      )}
      {/*
        Accesos globales que llevan sin revisarse más del periodo. Se avisa, no
        se corta: un acceso que caduca solo suena bien hasta el día en que caduca
        el de todos a la vez y no queda nadie que pueda devolverlo.
      */}
      {porRevisar > 0 && (
        <div class="sy-elevation" role="status">
          <Badge tone="warning">{t('platform.review.pending', { count: porRevisar })}</Badge>
          <span class="sy-elevation__reason">{t('platform.review.pendingHint')}</span>
        </div>
      )}
      <Tabs
        items={[
          {
            id: 'orgs',
            label: t('platform.tab.orgs'),
            content: (
              <Card>
                <CardHeader>
                  <CardTitle>{t('platform.orgs.title')}</CardTitle>
                  <Input
                    type="search"
                    value={filtro}
                    placeholder={t('platform.orgs.filter')}
                    onInput={(e) => setFiltro((e.target as HTMLInputElement).value)}
                  />
                  <CrearOrganizacion t={t} activation={activation} />
                </CardHeader>
                <CardBody>
                  {visibles.length === 0 ? (
                    <EmptyState title={t('platform.orgs.empty')} />
                  ) : (
                    <Table<PlatformOrg>
                      rowKey={(o) => o.id}
                      rows={visibles}
                      caption={t('platform.orgs.title')}
                      columns={[
                        {
                          key: 'org',
                          header: t('platform.col.org'),
                          // La fila entera no es pulsable: al lado hay botones
                          // que suspenden y fusionan, y un clic ancho junto a
                          // ellos invita a errores caros.
                          cell: (o) => (
                            <button
                              type="button"
                              class="sy-linkish sytue-focus-ring"
                              onClick={() => setDetalleOrg(o)}
                            >
                              <strong>{o.name}</strong>
                              {o.education && (
                                <>
                                  {' '}
                                  <Badge tone="info">{t('platform.education.badge')}</Badge>
                                </>
                              )}
                              <br />
                              <code>{o.slug}</code>
                            </button>
                          ),
                        },
                        {
                          key: 'status',
                          header: t('platform.col.status'),
                          cell: (o) => (
                            <Badge tone={statusTone(o.status)}>
                              {t(`platform.status.${o.status}`)}
                            </Badge>
                          ),
                        },
                        {
                          key: 'capacity',
                          header: t('platform.col.capacity'),
                          cell: (o) => <Capacity org={o} />,
                        },
                        {
                          key: 'owner',
                          header: t('platform.col.owner'),
                          cell: (o) => o.owner_emails.join(', ') || '—',
                        },
                        {
                          key: 'actions',
                          header: t('platform.col.actions'),
                          align: 'end',
                          cell: (o) => (
                            <div class="sy-row-actions">
                              <Button size="sm" variant="secondary" onClick={() => setSeatsOrg(o)}>
                                {t('platform.action.grantSeats')}
                              </Button>
                              {/* La organización que OPERA la plataforma no se
                                  suspende, ni se fusiona, ni se elimina: sus
                                  miembros son los administradores globales y
                                  `createSession` deniega el login a quien
                                  pertenece a una que no está activa, así que un
                                  clic dejaría a todo el mundo fuera —incluido
                                  quien tendría que deshacerlo—. El servidor lo
                                  deniega igual; esto evita ofrecer el botón. */}
                              {o.is_platform ? (
                                <span class="text-text-subtle text-xs">
                                  {t('platform.org.protected')}
                                </span>
                              ) : null}
                              {/* Suspender/reactivar NO se ofrece en una
                                  fusionada: reactivarla pondría dos
                                  organizaciones sirviendo los mismos datos
                                  migrados, y el servidor lo rechaza. */}
                              {!o.is_platform && o.status !== 'merged' && (
                                <Button
                                  size="sm"
                                  variant="secondary"
                                  onClick={() =>
                                    setCambioEstado({
                                      org: o,
                                      accion: o.status === 'active' ? 'suspend' : 'reactivate',
                                    })
                                  }
                                >
                                  {o.status === 'active'
                                    ? t('platform.action.suspend')
                                    : t('platform.action.reactivate')}
                                </Button>
                              )}
                              {!o.is_platform && o.status === 'active' && (
                                <Button size="sm" variant="danger" onClick={() => setMergeOrg(o)}>
                                  {t('platform.action.merge')}
                                </Button>
                              )}
                              {/* Eliminar se ofrece en CUALQUIER estado, la
                                  fusionada incluida. Antes exigía suspender
                                  primero, y en la práctica eso no protegía de
                                  nada: el diálogo ya pide motivo escrito, la
                                  acción queda auditada y —si hay más de un
                                  administrador global— espera una segunda firma.
                                  Lo único que conseguía era que retirar una
                                  organización de prueba fuera imposible de
                                  encontrar. Y la fusionada era el caso peor: su
                                  fila se queda para siempre —fusionar no borra
                                  nada— sin ninguna forma de limpiarla. */}
                              {!o.is_platform && (
                                <Button
                                  size="sm"
                                  variant="danger"
                                  onClick={() => setCambioEstado({ org: o, accion: 'delete' })}
                                >
                                  {t('platform.action.delete')}
                                </Button>
                              )}
                            </div>
                          ),
                        },
                      ]}
                    />
                  )}
                </CardBody>
              </Card>
            ),
          },
          {
            id: 'usage',
            label: t('platform.tab.usage'),
            content: usage ? (
              <div class="sy-stats">
                <StatCard label={t('platform.usage.orgs')} value={usage.orgs} />
                <StatCard label={t('platform.usage.orgsActive')} value={usage.orgs_active} />
                <StatCard label={t('platform.usage.orgsSuspended')} value={usage.orgs_suspended} />
                <StatCard label={t('platform.usage.users')} value={usage.users_active} />
                <StatCard label={t('platform.usage.users30d')} value={usage.users_active_30d} />
                <StatCard label={t('platform.usage.seatsSold')} value={usage.seats_purchased} />
                <StatCard label={t('platform.usage.seatsGranted')} value={usage.seats_granted} />
                <StatCard label={t('platform.usage.subs')} value={usage.subscriptions_active} />
              </div>
            ) : (
              <EmptyState title={t('platform.error.unavailable')} />
            ),
          },
          {
            id: 'versions',
            label: t('platform.tab.versions'),
            // Se trae sus propios datos y se refresca solo, como los logs: es la
            // pantalla que se mira MIENTRAS se despliega, y un dato del SSR
            // nacería viejo justo cuando más importa que no lo esté.
            content: <VersionsPanel t={t} />,
          },
          {
            id: 'logs',
            label: t('platform.tab.logs'),
            // El panel se trae sus propios datos y se refresca solo: son los
            // únicos de esta pantalla que cambian sin que nadie haga nada, y
            // pasarlos por el SSR los dejaría viejos en cuanto se pintan.
            content: <LogsPanel locale={locale} t={t} />,
          },
          {
            id: 'merges',
            label: t('platform.tab.merges'),
            content: (
              <Card>
                <CardHeader>
                  <CardTitle>{t('platform.merges.title')}</CardTitle>
                </CardHeader>
                <CardBody>
                  {mergesVivos.length === 0 ? (
                    <EmptyState title={t('platform.merges.empty')} />
                  ) : (
                    <Table<OrgMerge>
                      rowKey={(m) => m.id}
                      rows={mergesVivos}
                      caption={t('platform.merges.title')}
                      columns={[
                        {
                          key: 'orgs',
                          header: t('platform.col.merge'),
                          cell: (m) => {
                            const src = orgs.find((o) => o.id === m.source_org_id);
                            const dst = orgs.find((o) => o.id === m.target_org_id);
                            return `${src?.slug ?? m.source_org_id} → ${dst?.slug ?? m.target_org_id}`;
                          },
                        },
                        {
                          key: 'status',
                          header: t('platform.col.status'),
                          cell: (m) => (
                            <Badge tone={mergeTone(m.status)}>
                              {t(`platform.merge.state.${m.status}`)}
                            </Badge>
                          ),
                        },
                        {
                          key: 'progress',
                          header: t('platform.col.progress'),
                          // El progreso por servicio ES el diagnóstico: dice en cuál se
                          // quedó y, por tanto, qué mirar antes de reanudar.
                          cell: (m) => (
                            <span class="sy-merge-progress">
                              {Object.entries(m.services)
                                .map(([svc, st]) => `${svc}:${st}`)
                                .join(' · ') || '—'}
                              {m.error ? <em class="sy-merge-error"> {m.error}</em> : null}
                            </span>
                          ),
                        },
                        {
                          key: 'actions',
                          header: t('platform.col.actions'),
                          align: 'end',
                          cell: (m) =>
                            m.status === 'failed' ? (
                              <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => void reanudar(m.id)}
                              >
                                {t('platform.action.resume')}
                              </Button>
                            ) : null,
                        },
                      ]}
                    />
                  )}
                </CardBody>
              </Card>
            ),
          },
          {
            id: 'admins',
            label: t('platform.tab.admins'),
            content: (
              <>
                <Card>
                  <CardHeader>
                    <CardTitle>{t('platform.admins.title')}</CardTitle>
                  </CardHeader>
                  <CardBody>
                    <AdminsTable
                      admins={admins}
                      reviewDays={reviewDays}
                      locale={locale}
                      t={t}
                      toast={toast}
                    />
                  </CardBody>
                </Card>
                {/*
                El histórico va junto a la allowlist y no en una pestaña propia:
                son las dos mitades de la misma pregunta —quién puede y quién ha
                usado el alcance global—, y separarlas obliga a mirar en dos
                sitios para responderla.
              */}
                <div class="mt-6">
                  <ActivationHistory activations={activations} locale={locale} t={t} />
                </div>
              </>
            ),
          },
          {
            id: 'approvals',
            label: t('platform.tab.approvals'),
            content: (
              <ApprovalsPanel
                approvals={approvalsVivas}
                orgs={orgs}
                locale={locale}
                t={t}
                toast={toast}
              />
            ),
          },
          {
            id: 'audit',
            label: t('platform.tab.audit'),
            content: (
              <PlatformAudit
                rows={audit}
                cursor={auditCursor}
                locale={locale}
                t={t}
                toast={toast}
              />
            ),
          },
        ]}
      />

      {detalleOrg && (
        <OrgDetailModal
          org={detalleOrg}
          t={t}
          locale={locale}
          onClose={() => setDetalleOrg(null)}
          toast={toast}
        />
      )}
      {seatsOrg && (
        <SeatGrantModal org={seatsOrg} t={t} onClose={() => setSeatsOrg(null)} toast={toast} />
      )}
      {mergeOrg && (
        <MergeModal
          org={mergeOrg}
          orgs={orgs.filter((o) => o.id !== mergeOrg.id && o.status === 'active')}
          t={t}
          onClose={() => setMergeOrg(null)}
          toast={toast}
        />
      )}
      <ConfirmDialog
        open={cambioEstado !== null}
        onClose={() => {
          setCambioEstado(null);
          setMotivoEstado('');
        }}
        onConfirm={() => {
          if (cambioEstado) void cambiarEstado(cambioEstado.org, cambioEstado.accion, motivoEstado);
        }}
        title={
          cambioEstado?.accion === 'delete'
            ? t('platform.prompt.deleteTitle')
            : t('platform.prompt.statusReason')
        }
        confirmLabel={t(`platform.action.${cambioEstado?.accion ?? 'suspend'}`)}
        tone={cambioEstado?.accion === 'reactivate' ? 'primary' : 'danger'}
        confirmDisabled={motivoEstado.trim() === ''}
      >
        {cambioEstado?.accion === 'delete' && (
          <p class="sy-hint">{t('platform.prompt.deleteHint')}</p>
        )}
        <FieldGroup label={t('platform.prompt.statusReason')} htmlFor="motivo-estado" required>
          <Input
            id="motivo-estado"
            value={motivoEstado}
            onInput={(e) => setMotivoEstado(e.currentTarget.value)}
          />
        </FieldGroup>
      </ConfirmDialog>
    </div>
  );
}

type Toast = ReturnType<typeof useToast>;

/**
 * Detalle de una organización, con las plazas que se le han regalado.
 *
 * Las plazas se podían dar desde la consola y NO se podían ver ni retirar desde
 * ninguna parte: la API para hacerlo existe desde el principio, así que un gesto
 * comercial de hace dos años seguía sumando aforo y nadie tenía forma de
 * enterarse. Las concesiones se cargan al abrir y no con la lista: son una por
 * concesión y solo interesan cuando se mira UNA organización.
 */
function OrgDetailModal({
  org,
  t,
  locale,
  onClose,
  toast,
}: {
  org: PlatformOrg;
  t: T;
  locale: Locale;
  onClose: () => void;
  toast: Toast;
}) {
  const [grants, setGrants] = useState<SeatGrant[] | null>(null);
  const [fallo, setFallo] = useState(false);
  const [retirando, setRetirando] = useState('');
  const [modificarEducacion, setModificarEducacion] = useState(false);
  const [motivoEducacion, setMotivoEducacion] = useState('');
  const [guardandoEducacion, setGuardandoEducacion] = useState(false);

  useEffect(() => {
    let vigente = true;
    void (async () => {
      const res = await call(`/orgs/${org.id}`, 'GET');
      if (!vigente) return;
      if (!res.ok) {
        setFallo(true);
        return;
      }
      const data = (await res.json()) as { seat_grants?: SeatGrant[] };
      setGrants(data.seat_grants ?? []);
    })();
    return () => {
      vigente = false;
    };
  }, [org.id]);

  async function retirar(g: SeatGrant) {
    setRetirando(g.id);
    const res = await call(`/seat-grants/${g.id}`, 'DELETE');
    if (res.ok) {
      toast.notify({ title: t('platform.seats.revoked'), tone: 'success' });
      window.location.reload();
      return;
    }
    setRetirando('');
    toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
  }

  async function cambiarEducacion() {
    if (!motivoEducacion.trim() || guardandoEducacion) return;
    setGuardandoEducacion(true);
    const res = await call(`/orgs/${org.id}/education`, 'POST', {
      education: !org.education,
      reason: motivoEducacion.trim(),
    });
    setGuardandoEducacion(false);
    setModificarEducacion(false);
    if (res.ok) {
      toast.notify({ title: t('platform.toast.educationSaved'), tone: 'success' });
      window.location.reload();
      return;
    }
    const cuerpo = (await res.json().catch(() => ({}))) as { message?: string };
    toast.notify({ title: cuerpo.message ?? t('platform.toast.failed'), tone: 'danger' });
  }

  // Una concesión retirada o caducada ya no suma aforo. Se siguen enseñando —el
  // histórico es medio motivo por el que existe esta pantalla— pero apagadas y
  // sin botón.
  const vigente = (g: SeatGrant) =>
    !g.revoked_at && (!g.expires_at || new Date(g.expires_at) > new Date());

  return (
    <>
      <Modal open size="lg" title={`${org.name} (${org.slug})`} onClose={onClose}>
        <div class="sy-form">
          <div class="sy-version">
            <div class="sy-version__row">
              <span class="sy-version__app">{t('platform.col.capacity')}</span>
              <span class="sy-version__tag">
                {org.active_users} / {org.seats_effective}
              </span>
            </div>
            <div class="sy-version__row">
              <span class="sy-version__app">{t('platform.col.owner')}</span>
              <span class="sy-version__tag">{org.owner_emails.join(', ') || '—'}</span>
            </div>
            <div class="sy-version__row">
              <span class="sy-version__app">{t('platform.detail.namespace')}</span>
              <code class="sy-version__commit">{org.namespace}</code>
            </div>
            <div class="sy-version__row">
              <span class="sy-version__app">{t('platform.education.toggle')}</span>
              <span class="sy-version__tag flex items-center gap-2">
                <Badge tone={org.education ? 'info' : 'neutral'}>
                  {org.education ? t('common.yes') : t('common.no')}
                </Badge>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    setMotivoEducacion('');
                    setModificarEducacion(true);
                  }}
                >
                  {org.education ? t('platform.education.disable') : t('platform.education.enable')}
                </Button>
              </span>
            </div>
          </div>

          <h4 class="sy-stat__label">{t('platform.detail.grants')}</h4>
          {fallo ? (
            <EmptyState title={t('platform.error.unavailable')} />
          ) : grants === null ? (
            <p class="sy-hint">{t('common.loading')}</p>
          ) : grants.length === 0 ? (
            <p class="sy-hint">{t('platform.detail.noGrants')}</p>
          ) : (
            <Table<SeatGrant>
              rowKey={(g) => g.id}
              rows={grants}
              caption={t('platform.detail.grants')}
              columns={[
                { key: 'seats', header: t('platform.seats.count'), cell: (g) => g.seats },
                {
                  key: 'kind',
                  header: t('platform.seats.kind'),
                  cell: (g) => t(`platform.seats.kind.${g.kind}`),
                },
                { key: 'reason', header: t('platform.col.reason'), cell: (g) => g.reason || '—' },
                {
                  key: 'state',
                  header: t('platform.col.status'),
                  cell: (g) => (
                    <Badge tone={vigente(g) ? 'success' : 'neutral'}>
                      {g.revoked_at
                        ? t('platform.detail.grantRevoked')
                        : !vigente(g)
                          ? t('platform.detail.grantExpired')
                          : g.expires_at
                            ? new Date(g.expires_at).toLocaleDateString(locale)
                            : t('platform.detail.grantForever')}
                    </Badge>
                  ),
                },
                {
                  key: 'actions',
                  header: t('platform.col.actions'),
                  align: 'end',
                  cell: (g) =>
                    vigente(g) ? (
                      <Button
                        size="sm"
                        variant="secondary"
                        disabled={retirando === g.id}
                        onClick={() => void retirar(g)}
                      >
                        {t('platform.detail.revokeGrant')}
                      </Button>
                    ) : null,
                },
              ]}
            />
          )}
          {/* Propietarios y cuentas: la organización vista por personas. */}
          <PlatformMiembros org={org} t={t} toast={toast} />
          <div class="sy-form__actions">
            <Button variant="secondary" onClick={onClose}>
              {t('common.close')}
            </Button>
          </div>
        </div>
      </Modal>
      <ConfirmDialog
        open={modificarEducacion}
        onClose={() => {
          setModificarEducacion(false);
          setMotivoEducacion('');
        }}
        onConfirm={() => void cambiarEducacion()}
        title={org.education ? t('platform.education.disable') : t('platform.education.enable')}
        confirmLabel={t('common.save')}
        tone={org.education ? 'danger' : 'primary'}
        confirmDisabled={motivoEducacion.trim() === '' || guardandoEducacion}
      >
        <FieldGroup
          label={t('platform.prompt.educationReason')}
          htmlFor="motivo-educacion"
          required
        >
          <Input
            id="motivo-educacion"
            value={motivoEducacion}
            onInput={(e) => setMotivoEducacion(e.currentTarget.value)}
          />
        </FieldGroup>
      </ConfirmDialog>
    </>
  );
}

/** Concesión de plazas: cantidad, tipo, motivo obligatorio y caducidad. */
function SeatGrantModal({
  org,
  t,
  onClose,
  toast,
}: {
  org: PlatformOrg;
  t: T;
  onClose: () => void;
  toast: Toast;
}) {
  const [seats, setSeats] = useState(1);
  const [kind, setKind] = useState<'goodwill' | 'employee' | 'trial' | 'migration'>('goodwill');
  const [reason, setReason] = useState('');
  const [expires, setExpires] = useState('');
  const [saving, setSaving] = useState(false);
  const invalid = seats < 1 || seats > 10000 || reason.trim() === '';

  async function guardar() {
    if (invalid || saving) return;
    setSaving(true);
    const res = await call(`/orgs/${org.id}/seat-grants`, 'POST', {
      seats,
      kind,
      reason: reason.trim(),
      // <input type="date"> da AAAA-MM-DD; la API espera RFC3339.
      ...(expires ? { expires_at: `${expires}T00:00:00Z` } : {}),
    });
    setSaving(false);
    if (res.ok) {
      toast.notify({ title: t('platform.toast.seatsGranted'), tone: 'success' });
      window.location.reload();
    } else {
      toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
    }
  }

  return (
    <Modal open title={`${t('platform.action.grantSeats')} — ${org.name}`} onClose={onClose}>
      <div class="sy-form">
        <label>
          {t('platform.seats.count')}
          <Input
            type="number"
            min="1"
            max="10000"
            value={String(seats)}
            onInput={(e) => setSeats(Number((e.target as HTMLInputElement).value))}
          />
        </label>
        <label>
          {t('platform.seats.kind')}
          <select
            class="sy-select"
            value={kind}
            onChange={(e) => setKind((e.target as HTMLSelectElement).value as typeof kind)}
          >
            <option value="goodwill">{t('platform.seats.kind.goodwill')}</option>
            <option value="employee">{t('platform.seats.kind.employee')}</option>
            <option value="trial">{t('platform.seats.kind.trial')}</option>
            <option value="migration">{t('platform.seats.kind.migration')}</option>
          </select>
        </label>
        <label>
          {t('platform.seats.reason')}
          <Input value={reason} onInput={(e) => setReason((e.target as HTMLInputElement).value)} />
        </label>
        <label>
          {t('platform.seats.expires')}
          <Input
            type="date"
            value={expires}
            onInput={(e) => setExpires((e.target as HTMLInputElement).value)}
          />
        </label>
        <div class="sy-form__actions">
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={invalid || saving} onClick={() => void guardar()}>
            {t('common.save')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/**
 * Fusión: exige elegir destino y REESCRIBIR el identificador del origen. Es la
 * única operación irreversible de la consola; un desplegable mal pulsado no
 * puede bastar para fusionar al cliente equivocado.
 */
function MergeModal({
  org,
  orgs,
  t,
  onClose,
  toast,
}: {
  org: PlatformOrg;
  orgs: PlatformOrg[];
  t: T;
  onClose: () => void;
  toast: Toast;
}) {
  const [target, setTarget] = useState('');
  const [confirm, setConfirm] = useState('');
  const [saving, setSaving] = useState(false);
  const invalid = target === '' || confirm !== org.slug;

  async function fusionar() {
    if (invalid || saving) return;
    setSaving(true);
    const res = await call('/merges', 'POST', {
      source_org_id: org.id,
      target_org_id: target,
      confirm_source_slug: confirm,
    });
    setSaving(false);
    if (res.ok) {
      toast.notify({ title: t('platform.toast.mergeStarted'), tone: 'success' });
      window.location.reload();
      return;
    }
    // Fusionar ya no arranca al pedirlo: espera la firma de otro admin global.
    if (await quedoEsperando(res, t, toast)) {
      window.location.reload();
      return;
    }
    toast.notify({ title: t('platform.toast.failed'), tone: 'danger' });
  }

  return (
    <Modal open title={`${t('platform.action.merge')} — ${org.name}`} onClose={onClose}>
      <div class="sy-form">
        <p class="sy-warning">{t('platform.merge.warning')}</p>
        <label>
          {t('platform.merge.target')}
          <select
            class="sy-select"
            value={target}
            onChange={(e) => setTarget((e.target as HTMLSelectElement).value)}
          >
            <option value="">—</option>
            {orgs.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name} ({o.slug})
              </option>
            ))}
          </select>
        </label>
        <label>
          {t('platform.merge.confirm').replace('{slug}', org.slug)}
          <Input
            value={confirm}
            onInput={(e) => setConfirm((e.target as HTMLInputElement).value)}
          />
        </label>
        <div class="sy-form__actions">
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" disabled={invalid || saving} onClick={() => void fusionar()}>
            {t('platform.action.merge')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

export default function PlatformPanel(props: Props) {
  const t = useT(props.locale);
  return (
    <ToastProvider>
      <Inner {...props} t={t} />
    </ToastProvider>
  );
}
