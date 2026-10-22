/**
 * MembersPanel — quién tiene acceso a la organización y con qué rol.
 *
 * El backend estaba completo (alta/cambio de rol/baja, con guardia de último
 * owner) y la única «página de equipo» del producto era un enlace a otra app.
 *
 * El rol se cambia AQUÍ, en la fila. `POST /api/members {sub, role}` hace un
 * upsert desde el primer día —la guardia de último owner incluida— y la pantalla
 * solo lo pintaba: para ascender a alguien había que quitarlo y volver a
 * invitarlo por correo, esperando a que aceptara otra vez.
 *
 * Además tiene efecto económico directo: los puestos de suite facturables se
 * DERIVAN de esta lista, así que el panel lo dice en vez de dejar que el coste
 * aparezca por sorpresa en la factura.
 *
 * Sytue ID emite la invitación por correo y Cloud materializa el `sub` real al
 * primer acceso. Una invitación pendiente no cuenta como puesto facturable.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  Input,
  Modal,
  SectionHeader,
  Select,
  ToastProvider,
  useToast,
  useConfirm,
  Radio,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';
import type { Member, MemberInvitation } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { formatEur } from '../lib/format.js';
import { postBff } from '../lib/bff-client.js';
import DemoBanner from './DemoBanner.js';

/** Los cuatro roles con su nombre y lo que puede hacer cada uno, ya traducidos. */
function roles(t: T): { value: Member['role']; label: string; hint: string }[] {
  return [
    { value: 'owner', label: t('members.role.owner'), hint: t('members.role.owner.hint') },
    { value: 'admin', label: t('members.role.admin'), hint: t('members.role.admin.hint') },
    { value: 'member', label: t('members.role.member'), hint: t('members.role.member.hint') },
    { value: 'billing', label: t('members.role.billing'), hint: t('members.role.billing.hint') },
  ];
}

const roleTone = {
  owner: 'primary',
  admin: 'primary',
  member: 'neutral',
  billing: 'neutral',
} as const;

interface Props {
  members: Member[];
  invitations: MemberInvitation[];
  live: boolean;
  error?: { status: number; code: string } | undefined;
  /** Precio por usuario y mes de la suite, del control-plane. */
  seatPriceEur: number;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function MembersPanel(props: Props) {
  return (
    <ToastProvider>
      <Panel {...props} />
    </ToastProvider>
  );
}

function Panel({
  members: initial,
  invitations: initialInvitations,
  live,
  error,
  seatPriceEur,
  locale,
}: Props) {
  const t = useT(locale);
  const toast = useToast();
  // Confirmaciones del sistema de diseño y no `confirm()` del navegador: aquel
  // sale con los botones en el idioma del sistema operativo y sin la estética de
  // la casa, y en algunos navegadores bloquea la pestaña entera.
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const ROLES = roles(t);
  const roleLabel = (r: Member['role']): string => ROLES.find((x) => x.value === r)?.label ?? r;
  const [members, setMembers] = useState<Member[]>(initial);
  const [invitations, setInvitations] = useState<MemberInvitation[]>(initialInvitations);
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [emailTouched, setEmailTouched] = useState(false);
  const [role, setRole] = useState<Member['role']>('member');
  const [busy, setBusy] = useState(false);
  /** `sub` cuyo rol se está guardando: bloquea SOLO esa fila, no la lista entera. */
  const [savingRole, setSavingRole] = useState<string | null>(null);

  const monthly = members.length * seatPriceEur;
  const emailInvalid = emailTouched && !validEmail(email);

  async function submit(e: Event) {
    e.preventDefault();
    const clean = email.trim().toLowerCase();
    if (!validEmail(clean)) {
      setEmailTouched(true);
      return;
    }
    if (busy) return;
    setBusy(true);
    try {
      const res = await postBff<{
        ok: boolean;
        live: boolean;
        member?: Member;
        invitation?: MemberInvitation;
      }>('/api/invitations', {
        email: clean,
        role,
      });
      if (res.member) {
        setMembers((prev) => [...prev.filter((m) => m.sub !== res.member?.sub), res.member!]);
        setInvitations((prev) => prev.filter((inv) => inv.email !== clean));
      } else if (res.invitation) {
        setInvitations((prev) => [res.invitation!, ...prev.filter((inv) => inv.email !== clean)]);
      }
      setEmail('');
      setEmailTouched(false);
      setOpen(false);
      toast.notify({
        tone: 'success',
        title: res.member ? t('members.added') : t('members.invited'),
        description: res.member ? t('members.added.desc') : t('members.invited.desc'),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('members.add.fail'),
        description: t('members.add.fail.desc'),
      });
    } finally {
      setBusy(false);
    }
  }

  async function resend(inv: MemberInvitation) {
    if (busy) return;
    setBusy(true);
    try {
      const res = await postBff<{
        ok: boolean;
        live: boolean;
        member?: Member;
        invitation?: MemberInvitation;
      }>('/api/invitations', { email: inv.email, role: inv.role });
      if (res.member) {
        setMembers((prev) => [...prev.filter((m) => m.sub !== res.member?.sub), res.member!]);
        setInvitations((prev) => prev.filter((item) => item.id !== inv.id));
      } else if (res.invitation) {
        setInvitations((prev) => [res.invitation!, ...prev.filter((item) => item.id !== inv.id)]);
      }
      toast.notify({ tone: 'success', title: t('members.invitation.resent') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('members.add.fail'),
        description: t('members.add.fail.desc'),
      });
    } finally {
      setBusy(false);
    }
  }

  async function revoke(inv: MemberInvitation) {
    const ok = await confirmar({
      titulo: t('members.invitation.revoke.title'),
      mensaje: t('members.invitation.revoke.confirm', { email: inv.email }),
      confirmLabel: t('members.invitation.revoke'),
      tone: 'danger',
    });
    if (!ok) return;
    setBusy(true);
    try {
      await postBff<{ ok: boolean; live: boolean }>('/api/invitations', {
        id: inv.id,
        delete: true,
      });
      setInvitations((prev) => prev.filter((item) => item.id !== inv.id));
      toast.notify({ tone: 'success', title: t('members.invitation.revoked') });
    } catch {
      toast.notify({ tone: 'danger', title: t('members.invitation.revoke.fail') });
    } finally {
      setBusy(false);
    }
  }

  /**
   * Cambia el rol de un colaborador. Sin diálogo de confirmación: es reversible
   * desde este mismo desplegable, y el único cambio que NO lo sería —dejar la
   * organización sin ningún propietario— lo rechaza el control-plane.
   *
   * Se pinta antes de que responda porque un desplegable que no se mueve al
   * elegir parece roto; si el control-plane dice que no, la fila vuelve al rol
   * anterior y el toast explica por qué.
   */
  async function changeRole(m: Member, next: Member['role']) {
    if (next === m.role || savingRole) return;
    setSavingRole(m.sub);
    setMembers((prev) => prev.map((x) => (x.sub === m.sub ? { ...x, role: next } : x)));
    try {
      await postBff<{ ok: boolean; live: boolean }>('/api/members', { sub: m.sub, role: next });
      toast.notify({
        tone: 'success',
        title: t('members.role.updated', { role: roleLabel(next) }),
      });
    } catch {
      setMembers((prev) => prev.map((x) => (x.sub === m.sub ? { ...x, role: m.role } : x)));
      toast.notify({
        tone: 'danger',
        title: t('members.role.updateFail'),
        description: t('members.remove.fail.desc'),
      });
    } finally {
      setSavingRole(null);
    }
  }

  async function remove(m: Member) {
    const ok = await confirmar({
      titulo: t('members.remove.title'),
      mensaje: t('members.remove.confirm', { sub: m.sub }),
      confirmLabel: t('members.remove'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await postBff<{ ok: boolean; live: boolean }>('/api/members', { sub: m.sub, delete: true });
      setMembers((prev) => prev.filter((x) => x.sub !== m.sub));
      toast.notify({ tone: 'success', title: t('members.removed') });
    } catch {
      // El caso típico: es el único propietario y el control-plane lo impide.
      toast.notify({
        tone: 'danger',
        title: t('members.remove.fail'),
        description: t('members.remove.fail.desc'),
      });
    }
  }

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        icon={<Icon name="users" size={20} />}
        title={t('nav.team')}
        subtitle={t('members.subtitle')}
        actions={
          <Button variant="primary" size="sm" onClick={() => setOpen(true)}>
            {t('members.add')}
          </Button>
        }
        level={1}
      />

      <div class="cloud-content">
        {/* El coste se dice ANTES de la factura, no después. La frase entera es
            una sola clave con forma plural: partirla para poner el importe en
            negrita obligaría a concatenar, y el orden de las palabras cambia
            entre idiomas. */}
        {members.length > 0 ? (
          <p class="text-text-muted mb-3 text-sm">
            {t('members.summary', {
              count: members.length,
              amount: formatEur(locale, monthly),
            })}
          </p>
        ) : null}

        {members.length === 0 ? (
          <EmptyState
            illustration={<Icon name="users" size={24} />}
            title={t('members.empty.title')}
            description={t('members.empty.desc')}
          />
        ) : (
          <Card>
            <CardBody class="p-0">
              <ul class="divide-border divide-y">
                {/* `flex-wrap` + un mínimo en la identidad: en un móvil el
                    desplegable de rol y el botón bajan a una segunda línea en vez
                    de estrujar el identificador hasta dejarlo en «a1b2…». */}
                {members.map((m) => (
                  <li key={m.sub} class="flex flex-wrap items-center gap-3 px-4 py-3">
                    <div class="min-w-[12rem] flex-1">
                      <p class="text-text truncate font-mono text-sm">{m.sub}</p>
                      <p class="sytue-meta">
                        {t('members.since', { date: formatDate(locale, m.createdAt) })}
                      </p>
                    </div>
                    <div class="ml-auto flex shrink-0 items-center gap-2">
                      {/* El rol es editable en la fila, no un rótulo: es la única
                          forma de ascender o degradar a alguien sin darle de baja
                          y volver a invitarlo por correo. */}
                      <Select
                        class="w-44"
                        value={m.role}
                        disabled={savingRole !== null}
                        aria-label={t('members.role.change', { sub: m.sub })}
                        onChange={(e) =>
                          void changeRole(m, e.currentTarget.value as Member['role'])
                        }
                      >
                        {ROLES.map((r) => (
                          <option key={r.value} value={r.value}>
                            {r.label}
                          </option>
                        ))}
                      </Select>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={savingRole !== null}
                        onClick={() => void remove(m)}
                      >
                        {t('members.remove')}
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>
        )}

        {invitations.length > 0 ? (
          <section class="mt-6" aria-labelledby="pending-invitations-title" aria-live="polite">
            <h2 id="pending-invitations-title" class="text-text mb-2 text-sm font-semibold">
              {t('members.invitations.pending', { count: invitations.length })}
            </h2>
            <p class="sytue-meta mb-3">{t('members.invitations.billingNote')}</p>
            <Card>
              <CardBody class="p-0">
                <ul class="divide-border divide-y">
                  {invitations.map((inv) => (
                    <li key={inv.id} class="flex items-center justify-between gap-3 px-4 py-3">
                      <div class="min-w-0">
                        <p class="text-text truncate text-sm">{inv.email}</p>
                        <p class="sytue-meta">
                          {t('members.invitation.expires', {
                            date: formatDate(locale, inv.expiresAt),
                          })}
                        </p>
                      </div>
                      <div class="flex shrink-0 items-center gap-2">
                        <Badge tone={roleTone[inv.role]}>{roleLabel(inv.role)}</Badge>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={busy}
                          onClick={() => void resend(inv)}
                        >
                          {t('members.invitation.resend')}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={busy}
                          onClick={() => void revoke(inv)}
                        >
                          {t('members.invitation.revoke')}
                        </Button>
                      </div>
                    </li>
                  ))}
                </ul>
              </CardBody>
            </Card>
          </section>
        ) : null}
      </div>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t('members.add')}
        footer={
          <>
            <Button variant="ghost" type="button" onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" type="submit" form="miembro-invitar" disabled={busy}>
              {t('members.sendInvitation')}
            </Button>
          </>
        }
      >
        <form id="miembro-invitar" class="flex flex-col gap-3" onSubmit={(e) => void submit(e)}>
          <label class="flex flex-col gap-1">
            <span class="text-text text-sm">{t('members.field.email')}</span>
            <Input
              id="member-invite-email"
              name="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              placeholder={t('members.field.email.placeholder')}
              invalid={emailInvalid}
              aria-describedby={
                emailInvalid ? 'member-invite-email-error' : 'member-invite-email-help'
              }
              aria-errormessage={emailInvalid ? 'member-invite-email-error' : undefined}
              onBlur={() => setEmailTouched(true)}
              onInput={(e) => {
                setEmail((e.target as HTMLInputElement).value);
                setEmailTouched(false);
              }}
            />
            {emailInvalid ? (
              <span id="member-invite-email-error" class="text-danger-text text-xs" role="alert">
                {t('members.field.email.error')}
              </span>
            ) : (
              <span id="member-invite-email-help" class="sytue-meta">
                {t('members.field.email.help')}
              </span>
            )}
          </label>
          <fieldset class="flex flex-col gap-1">
            <legend class="text-text text-sm">{t('members.field.role')}</legend>
            {ROLES.map((r) => (
              <label
                key={r.value}
                class="flex cursor-pointer items-start gap-2 py-0.5 pointer-coarse:min-h-[max(44px,2.75rem)]"
              >
                <Radio
                  class="accent-primary"
                  name="role"
                  checked={role === r.value}
                  onChange={() => setRole(r.value)}
                />
                {/* El nombre del rol va DIRECTO dentro del span: anidarlo un nivel
                    más lo deja fuera del alcance con que jsx-a11y busca el texto
                    accesible del <label> (profundidad 2). */}
                <span class="text-text text-sm">
                  {r.label}
                  <span class="sytue-meta block">{r.hint}</span>
                </span>
              </label>
            ))}
          </fieldset>
          <p class="sytue-meta">
            {t('members.inviteSeatNote', { amount: formatEur(locale, seatPriceEur) })}
          </p>
        </form>
      </Modal>
      {dialogoConfirmar}
    </>
  );
}

function validEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}
