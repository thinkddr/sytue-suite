/**
 * InviteUserWizard — alta guiada de una persona en la organización activa.
 *
 * Antes era un modal de cuatro campos con el rol en un desplegable: se elegía
 * «Administrador» sin leer qué suponía, las pistas no estaban enlazadas a sus
 * campos y un error de la API salía en un aviso flotante que no decía qué campo
 * corregir. Ahora son cuatro pasos —a quién, qué podrá hacer, repaso y hecho—
 * y cada error de Sytue ID vuelve al paso que lo causa y se pinta allí.
 *
 * Usa el BFF de siempre (POST /api/admin/invitations); quién puede conceder qué
 * lo decide Sytue ID: aquí solo se esconde «Propietario» a quien no lo es, igual
 * que hace `handleAdminInviteUser` en el servidor.
 *
 * «Varias personas» invita hasta 25 de golpe con el mismo rol: el alta inicial
 * lo permitía y, si te pasabas, te mandaba aquí, donde solo se podía de una en
 * una. Los mismos cuatro pasos; el último enseña qué pasó con CADA dirección,
 * con la misma forma que el alta (`InviteResults` del portal).
 */
import { useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  ReviewSummary,
  Textarea,
  Wizard,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { SytueRole } from '../lib/types.js';
import type { InviteBatchResult, InviteBatchStatus } from '../lib/admin.js';
import { roleLabelKey } from '../lib/format.js';

const PASO_QUIEN = 0;
const PASO_ROL = 1;
const PASO_REPASO = 2;
const PASO_HECHO = 3;

const ROLES: SytueRole[] = ['member', 'admin', 'billing', 'owner'];

/** Tope de una tanda; el mismo que aplica Sytue ID (maxBatchInvites). */
export const MAX_TANDA = 25;

type Modo = 'una' | 'varias';

/**
 * Separa lo pegado en direcciones: comas, punto y coma, espacios o saltos de
 * línea, como `domain.ParseInviteEmails`. Lo que no lleva @ se descarta y se
 * cuenta aparte: al pegar un CSV llegan la cabecera y los nombres, y tratarlos
 * como «no es un correo» llenaría el resultado de falsos errores. Lo que lleva @
 * pero está mal escrito SÍ se envía, para que Sytue ID lo diga por dirección.
 */
export function separarDirecciones(texto: string): { direcciones: string[]; ignorados: number } {
  const vistas = new Set<string>();
  const direcciones: string[] = [];
  let ignorados = 0;
  for (const bruto of texto.split(/[\s,;]+/)) {
    const trozo = bruto.replace(/^["'<]+|["'>]+$/g, '');
    if (!trozo) continue;
    if (!trozo.includes('@')) {
      ignorados++;
      continue;
    }
    const clave = trozo.toLowerCase();
    if (vistas.has(clave)) continue;
    vistas.add(clave);
    direcciones.push(trozo);
  }
  return { direcciones, ignorados };
}

const tonoEstado = {
  sent: 'success',
  already_member: 'warning',
  invalid: 'warning',
  not_sent: 'danger',
  failed: 'danger',
} as const satisfies Record<InviteBatchStatus, 'success' | 'warning' | 'danger'>;

const etiquetaEstado = {
  sent: 'users.inviteWizard.many.status.sent',
  already_member: 'users.inviteWizard.many.status.already_member',
  invalid: 'users.inviteWizard.many.status.invalid',
  not_sent: 'users.inviteWizard.many.status.not_sent',
  failed: 'users.inviteWizard.many.status.failed',
} as const satisfies Record<InviteBatchStatus, string>;

const cuerpoEstado = {
  already_member: 'users.inviteWizard.many.body.already_member',
  invalid: 'users.inviteWizard.many.body.invalid',
  not_sent: 'users.inviteWizard.many.body.not_sent',
  failed: 'users.inviteWizard.many.body.failed',
} as const satisfies Record<Exclude<InviteBatchStatus, 'sent'>, string>;

const consecuenciaRol = {
  owner: 'users.inviteWizard.role.ownerBody',
  admin: 'users.inviteWizard.role.adminBody',
  member: 'users.inviteWizard.role.memberBody',
  billing: 'users.inviteWizard.role.billingBody',
} as const satisfies Record<SytueRole, string>;

/** Error de la API, atado al paso donde se corrige. */
interface ErrorPaso {
  paso: number;
  mensaje: string;
}

/** Resultado de una tanda, para el paso final. */
interface ResultadoTanda {
  enviadas: number;
  filas: InviteBatchResult[];
}

/** Resultado de la invitación, para el paso final. */
interface Resultado {
  email: string;
  /** '' = alta directa con contraseña (no hay invitación que caduque). */
  caduca: string;
  conPassword: boolean;
  /** La invitación existe pero el correo no salió (502/503). */
  sinEnvio: boolean;
}

/**
 * Lleva el código de la API al paso que lo corrige. El 400 es de la dirección o
 * del buzón (lo único que valida Sytue ID antes de crear), el 409 de la persona
 * (ya es miembro) y el 403 del rol (un admin no concede propietario). Lo demás
 * no tiene un campo culpable y se queda en el repaso para reintentar.
 */
export function pasoDelError(code: string): number {
  if (code === '400' || code === '409') return PASO_QUIEN;
  if (code === '403') return PASO_ROL;
  return PASO_REPASO;
}

export interface InviteUserWizardProps {
  t: T;
  locale: Locale;
  /** Dominio de correo de la organización ('' = todavía no se sabe). */
  mailDomain: string;
  /** Quien invita es propietario: solo entonces puede conceder «Propietario». */
  canGrantOwner: boolean;
  /** La lista de pendientes ha cambiado (se emitió o creó algo). */
  onInvited: () => void;
  onClose: () => void;
  /** Pie del diálogo donde va la botonera (alcanzable sobre el teclado del móvil). */
  footerTarget?: Element | null;
}

export default function InviteUserWizard({
  t,
  locale,
  mailDomain,
  canGrantOwner,
  onInvited,
  onClose,
  footerTarget,
}: InviteUserWizardProps) {
  const [step, setStep] = useState(PASO_QUIEN);
  const [modo, setModo] = useState<Modo>('una');
  const [lista, setLista] = useState('');
  const [tanda, setTanda] = useState<ResultadoTanda | null>(null);
  const [busy, setBusy] = useState(false);
  const [email, setEmail] = useState('');
  const [mailbox, setMailbox] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<SytueRole>('member');
  const [error, setError] = useState<ErrorPaso | null>(null);
  const [resultado, setResultado] = useState<Resultado | null>(null);

  const correo = email.trim();
  const buzon = mailbox.trim();
  const { direcciones, ignorados } = separarDirecciones(lista);
  const varias = modo === 'varias';
  const demasiadas = direcciones.length > MAX_TANDA;
  const listaValida = direcciones.length > 0 && !demasiadas;
  const correoValido = varias ? listaValida : /^[^\s@]+@[^\s@]+$/.test(correo);
  // Propietario no se reparte en tanda (Sytue ID lo rechaza): es una decisión
  // por persona y se toma de una en una.
  const roles = ROLES.filter((r) => r !== 'owner' || (canGrantOwner && !varias));
  const errorEn = (paso: number) => (error?.paso === paso ? error.mensaje : '');

  function reiniciar() {
    setEmail('');
    setMailbox('');
    setPassword('');
    setRole('member');
    setError(null);
    setResultado(null);
    setLista('');
    setTanda(null);
    setStep(PASO_QUIEN);
  }

  function cambiarModo(m: Modo) {
    setModo(m);
    setError(null);
    if (m === 'varias' && role === 'owner') setRole('member');
  }

  async function enviarVarias() {
    if (busy || !listaValida) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch('/api/admin/invitations', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ emails: direcciones, role }),
      });
      if (!res.ok) {
        const code = String(res.status);
        const paso = res.status === 429 ? PASO_REPASO : pasoDelError(code);
        setError({
          paso,
          mensaje:
            res.status === 429
              ? t('users.inviteWizard.many.rateLimited')
              : code === '403'
                ? t('users.toast.roleForbidden')
                : code === '400'
                  ? t('users.inviteWizard.many.invalid')
                  : t('common.tryAgain'),
        });
        setStep(paso);
        return;
      }
      const body = (await res.json().catch(() => ({}))) as {
        sent?: number;
        results?: InviteBatchResult[];
      };
      const filas = Array.isArray(body.results) ? body.results : [];
      const enviadas = body.sent ?? filas.filter((f) => f.status === 'sent').length;
      setTanda({ enviadas, filas });
      if (enviadas > 0) onInvited();
      setStep(PASO_HECHO);
    } catch {
      setError({ paso: PASO_REPASO, mensaje: t('common.tryAgain') });
    } finally {
      setBusy(false);
    }
  }

  async function enviar() {
    if (busy || !correoValido) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch('/api/admin/invitations', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          email: correo,
          role,
          mailbox_local: buzon,
          ...(password ? { password } : {}),
        }),
      });
      const code = String(res.status);
      if (res.status === 502 || res.status === 503) {
        // La invitación ESTÁ creada y lo que falló es el envío: no es un error que
        // se corrija en ningún paso, así que se cierra el asistente para que nadie
        // reintente y acabe con cinco invitaciones a la misma persona.
        setResultado({ email: correo, caduca: '', conPassword: false, sinEnvio: true });
        onInvited();
        setStep(PASO_HECHO);
        return;
      }
      if (!res.ok) {
        const paso = pasoDelError(code);
        // Con contraseña inicial Sytue ID hace un alta directa y sus errores dicen
        // otra cosa: el 400 puede ser la política de contraseñas, y el 409 es que
        // la cuenta YA existe (aunque no sea de esta organización), no que sea
        // miembro. Decir «ya está en la organización» mandaba a buscarla en la
        // lista sin encontrarla.
        const conPassword = password !== '';
        setError({
          paso,
          mensaje:
            code === '409'
              ? conPassword
                ? t('users.inviteWizard.who.accountExists')
                : t('users.toast.alreadyMember')
              : code === '403'
                ? t('users.toast.roleForbidden')
                : code === '400'
                  ? conPassword
                    ? t('users.inviteWizard.who.invalidWithPassword')
                    : t('users.inviteWizard.who.invalid')
                  : t('common.tryAgain'),
        });
        setStep(paso);
        return;
      }
      const body = (await res.json().catch(() => ({}))) as { expires_at?: string };
      setResultado({
        email: correo,
        caduca: body.expires_at ?? '',
        conPassword: password !== '',
        sinEnvio: false,
      });
      onInvited();
      setStep(PASO_HECHO);
    } catch {
      setError({ paso: PASO_REPASO, mensaje: t('common.tryAgain') });
    } finally {
      setBusy(false);
    }
  }

  const errQuien = errorEn(PASO_QUIEN);
  const stepWho: WizardStep = {
    id: 'who',
    title: t('users.inviteWizard.step.who'),
    heading: t('users.inviteWizard.who.heading'),
    description: t('users.inviteIntro'),
    canContinue: correoValido,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <div
          role="radiogroup"
          aria-label={t('users.inviteWizard.mode.label')}
          class="grid gap-2 sm:grid-cols-2"
        >
          <ChoiceCard
            name="invite-mode"
            value="una"
            dense
            icon="user"
            title={t('users.inviteWizard.mode.one')}
            description={t('users.inviteWizard.mode.oneBody')}
            selected={!varias}
            onSelect={() => cambiarModo('una')}
          />
          <ChoiceCard
            name="invite-mode"
            value="varias"
            dense
            icon="users"
            title={t('users.inviteWizard.mode.many')}
            description={t('users.inviteWizard.mode.manyBody')}
            selected={varias}
            onSelect={() => cambiarModo('varias')}
          />
        </div>
        {varias ? camposVarias() : camposUna()}
      </div>
    ),
  };

  function camposVarias() {
    const aviso =
      errQuien ||
      (demasiadas ? t('users.inviteWizard.many.tooMany', { count: direcciones.length }) : '');
    return (
      <FieldGroup label={t('users.inviteWizard.many.label')} htmlFor="invite-emails" required>
        <Textarea
          id="invite-emails"
          rows={5}
          spellcheck={false}
          autocomplete="off"
          placeholder={t('users.inviteWizard.many.placeholder')}
          value={lista}
          invalid={Boolean(aviso)}
          aria-describedby="invite-emails-hint invite-emails-count"
          onInput={(e) => {
            setLista(e.currentTarget.value);
            setError(null);
          }}
        />
        <FieldHint id="invite-emails-hint" tone={aviso ? 'danger' : 'neutral'}>
          {aviso || t('users.inviteWizard.many.hint')}
        </FieldHint>
        <p id="invite-emails-count" class="sytue-meta" aria-live="polite">
          {t('users.inviteWizard.many.count', { count: direcciones.length })}
          {ignorados > 0 ? ` · ${t('users.inviteWizard.many.ignored', { count: ignorados })}` : ''}
        </p>
      </FieldGroup>
    );
  }

  function camposUna() {
    return (
      <>
        <FieldGroup label={t('users.inviteEmail')} htmlFor="invite-email" required>
          <Input
            id="invite-email"
            type="email"
            autocomplete="off"
            placeholder={t('users.inviteEmailPlaceholder')}
            value={email}
            invalid={Boolean(errQuien)}
            aria-describedby="invite-email-hint"
            onInput={(e) => {
              setEmail(e.currentTarget.value);
              setError(null);
            }}
          />
          <FieldHint id="invite-email-hint" tone={errQuien ? 'danger' : 'neutral'}>
            {errQuien || t('users.inviteWizard.who.emailHint')}
          </FieldHint>
        </FieldGroup>
        <FieldGroup label={t('users.inviteMailbox')} htmlFor="invite-mailbox">
          <div class="flex items-center gap-1.5">
            <Input
              id="invite-mailbox"
              autocomplete="off"
              placeholder={t('users.inviteMailboxPlaceholder')}
              value={mailbox}
              aria-describedby="invite-mailbox-hint"
              onInput={(e) => {
                setMailbox(e.currentTarget.value);
                setError(null);
              }}
            />
            {mailDomain ? (
              <span class="text-text-muted shrink-0 font-mono text-sm">@{mailDomain}</span>
            ) : null}
          </div>
          <FieldHint id="invite-mailbox-hint">{t('users.inviteMailboxHint')}</FieldHint>
        </FieldGroup>
        <FieldGroup label={t('users.invitePassword')} htmlFor="invite-password">
          <Input
            id="invite-password"
            type="password"
            autocomplete="new-password"
            placeholder={t('users.invitePasswordPlaceholder')}
            value={password}
            aria-describedby="invite-password-hint"
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
          <FieldHint id="invite-password-hint">{t('users.invitePasswordHint')}</FieldHint>
        </FieldGroup>
      </>
    );
  }

  const errRol = errorEn(PASO_ROL);
  const stepRole: WizardStep = {
    id: 'role',
    title: t('users.inviteWizard.step.role'),
    heading: t('users.inviteWizard.role.heading'),
    description: t('users.inviteWizard.role.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-3">
        <div role="radiogroup" aria-label={t('users.inviteRoleLabel')} class="flex flex-col gap-2">
          {roles.map((r) => (
            <ChoiceCard
              key={r}
              name="invite-role"
              value={r}
              title={t(roleLabelKey[r])}
              description={t(consecuenciaRol[r])}
              selected={role === r}
              onSelect={() => {
                setRole(r);
                setError(null);
              }}
            />
          ))}
        </div>
        {errRol ? (
          <InlineHelp tone="danger" title={t('users.toast.inviteFailed')}>
            {errRol}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const direccion = buzon
    ? mailDomain
      ? `${buzon}@${mailDomain}`
      : buzon
    : t('users.inviteWizard.review.mailboxAuto');
  const filaRol: ReviewItem = {
    label: t('users.inviteRoleLabel'),
    value: t(roleLabelKey[role]),
    onEdit: () => setStep(PASO_ROL),
  };
  // En tanda no hay buzón elegido ni contraseña: todas van por enlace y con la
  // dirección automática, y el repaso lo dice para que nadie lo espere distinto.
  const summary: ReviewItem[] = [
    varias
      ? {
          label: t('users.inviteWizard.many.people'),
          value: (
            <>
              <span class="block">
                {t('users.inviteWizard.many.count', { count: direcciones.length })}
              </span>
              <span class="text-text-muted block font-mono text-xs break-all">
                {direcciones.join(', ')}
              </span>
            </>
          ),
          onEdit: () => setStep(PASO_QUIEN),
        }
      : { label: t('users.inviteEmail'), value: correo || '—', onEdit: () => setStep(PASO_QUIEN) },
    {
      label: t('users.inviteMailbox'),
      value: varias ? t('users.inviteWizard.review.mailboxAuto') : direccion,
      mono: !varias && Boolean(buzon),
      onEdit: () => setStep(PASO_QUIEN),
    },
    {
      label: t('users.inviteWizard.review.access'),
      value:
        password && !varias
          ? t('users.inviteWizard.review.accessPassword')
          : t('users.inviteWizard.review.accessLink'),
      onEdit: () => setStep(PASO_QUIEN),
    },
    filaRol,
  ];

  const errRepaso = errorEn(PASO_REPASO);
  const stepReview: WizardStep = {
    id: 'review',
    title: t('users.inviteWizard.step.review'),
    heading: t('users.inviteWizard.review.heading'),
    description: t('users.inviteWizard.review.description'),
    nextLabel: varias
      ? t('users.inviteWizard.many.submit', { count: direcciones.length })
      : password
        ? t('users.inviteWizard.review.create')
        : t('users.inviteSubmit'),
    canContinue: correoValido,
    onNext: () => void (varias ? enviarVarias() : enviar()),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={summary} />
        {errRepaso ? (
          <InlineHelp tone="danger" title={t('users.toast.inviteFailed')}>
            {errRepaso}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  let hecho: { tone: 'success' | 'warning'; title: string; body: string } = {
    tone: 'success',
    title: t('users.toast.invited'),
    body: '',
  };
  if (resultado?.sinEnvio) {
    hecho = {
      tone: 'warning',
      title: t('users.toast.inviteNotSent'),
      body: t('users.toast.inviteNotSentHint'),
    };
  } else if (resultado?.conPassword) {
    hecho = {
      tone: 'success',
      title: t('users.toast.created'),
      body: t('users.toast.createdHint', { email: resultado.email }),
    };
  } else if (resultado) {
    hecho.body = resultado.caduca
      ? t('users.inviteWizard.done.expires', {
          email: resultado.email,
          date: formatDateTime(locale, new Date(resultado.caduca)),
        })
      : t('users.toast.invitedHint', { email: resultado.email, role: t(roleLabelKey[role]) });
  }

  if (tanda) {
    const todas = tanda.filas.length > 0 && tanda.enviadas === tanda.filas.length;
    hecho = {
      tone: todas ? 'success' : 'warning',
      title: t('users.inviteWizard.many.doneTitle', {
        sent: tanda.enviadas,
        total: tanda.filas.length,
      }),
      body: todas ? t('users.inviteWizard.many.doneAll') : t('users.inviteWizard.many.doneSome'),
    };
  }

  const stepDone: WizardStep = {
    id: 'done',
    title: t('users.inviteWizard.step.done'),
    heading: t('users.inviteWizard.done.heading'),
    done: true,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <InlineHelp tone={hecho.tone} title={hecho.title}>
          {hecho.body}
        </InlineHelp>
        {tanda && tanda.filas.length > 0 ? (
          <ul class="flex flex-col gap-2" aria-label={t('users.inviteWizard.many.people')}>
            {tanda.filas.map((f) => (
              <li key={f.email}>
                <InlineHelp
                  tone={tonoEstado[f.status] ?? 'danger'}
                  title={`${f.email} — ${t(etiquetaEstado[f.status] ?? etiquetaEstado.failed)}`}
                >
                  {f.status === 'sent'
                    ? f.expires_at
                      ? t('users.inviteWizard.many.body.sent', {
                          date: formatDateTime(locale, new Date(f.expires_at)),
                        })
                      : ''
                    : t(cuerpoEstado[f.status] ?? cuerpoEstado.failed)}
                </InlineHelp>
              </li>
            ))}
          </ul>
        ) : null}
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={reiniciar}>
            {t('users.inviteWizard.done.another')}
          </Button>
          <Button variant="ghost" onClick={onClose}>
            {t('common.close')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepWho, stepRole, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onClose}
      footerTarget={footerTarget ?? null}
    />
  );
}
