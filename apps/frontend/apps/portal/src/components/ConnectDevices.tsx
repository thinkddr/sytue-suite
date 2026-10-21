/**
 * ConnectDevices — asistente «Conectar tus dispositivos».
 *
 * Tres pasos: eliges qué usas, te damos las instrucciones EXACTAS de eso (y solo
 * de eso), y cierras sabiendo cómo comprobar que ha funcionado. Ningún valor está
 * escrito aquí: todos vienen de `endpoints`, que la página SSR lee de Sytue ID
 * (/.well-known/sytue-clients.json). Cada valor tiene su botón de copiar: el
 * usuario no teclea servidores ni puertos a mano.
 *
 * Honestidad del copy: solo se promete lo que la suite sirve (IMAP/SMTP, CalDAV,
 * CardDAV, WebDAV). NO hay Exchange ni ActiveSync y no se insinúa que lo haya.
 *
 * Los textos salen del catálogo del portal (`devices.*`) y el idioma llega por
 * props desde el `.astro`: nunca se detecta aquí (desajustaría la hidratación).
 */
import type { ComponentChildren, JSX } from 'preact';
import { useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import {
  Button,
  ChoiceCard,
  DataFlow,
  EmptyState,
  FieldHint,
  InlineHelp,
  ReviewSummary,
  SettingsShell,
  SettingsSection,
  ToastProvider,
  useToast,
  Wizard,
  type DataCategory,
  type WizardStep,
} from '@sytue/ui';
import { useT, type PortalMessageKey, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';

/** Coordenadas públicas que publica Sytue ID (mismas que la autoconfiguración). */
export interface ClientEndpoints {
  domain: string;
  imap: { host: string; port: number; tls: string };
  smtp: { host: string; port: number; tls: string };
  caldav_url: string;
  carddav_url: string;
  webdav_url: string;
  apple_profile_url: string;
}

interface Props {
  email: string;
  /** null si Sytue ID no respondió: se avisa en vez de inventar valores. */
  endpoints: ClientEndpoints | null;
  /** Idioma ya negociado en el servidor (middleware). Nunca se detecta aquí. */
  locale: Locale;
}

type Platform = 'iphone' | 'mac' | 'outlook' | 'thunderbird' | 'android' | 'drive' | 'apis';

// Solo los identificadores: título, descripción y nota de cada uno viven en el
// catálogo (`devices.platform.<id>.*`), que es de donde salen traducidos.
const PLATFORMS: Platform[] = [
  'iphone',
  'mac',
  'outlook',
  'thunderbird',
  'android',
  'drive',
  // Herramientas escritas contra Google o Microsoft. Drive, Calendar y Contacts hablan
  // esas APIs desde hace tiempo y no se decía en ninguna parte: una capacidad que nadie
  // conoce es una capacidad que nadie usa.
  'apis',
];

/** Lo que se sincroniza con cada cosa: va y viene entre Sytue One y la app que se configure. */
const SYNCS: Record<Platform, DataCategory[]> = {
  iphone: ['mail', 'events', 'contacts'],
  mac: ['mail', 'events', 'contacts'],
  outlook: ['mail', 'events', 'contacts'],
  thunderbird: ['mail', 'events', 'contacts'],
  android: ['mail', 'events', 'contacts'],
  drive: ['files'],
  apis: ['files', 'events', 'contacts'],
};

/** Los clientes con botón de un clic, y el nombre de su contraseña en Mi cuenta. */
type OneClickPlatform = 'outlook' | 'thunderbird' | 'android';
const ONE_CLICK_LABEL: Record<OneClickPlatform, string> = {
  outlook: 'Outlook',
  thunderbird: 'Thunderbird',
  android: 'Android',
};

interface OneClickState {
  platform: OneClickPlatform;
  busy?: boolean;
  failed?: boolean;
  password?: string;
}

export function ConnectDevices(props: Props): JSX.Element {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function Inner({ email, endpoints, locale }: Props): JSX.Element {
  const t = useT(locale);
  const [step, setStep] = useState(0);
  const [platform, setPlatform] = useState<Platform | null>(null);
  // Lo que deja el botón de un clic: la contraseña se enseña UNA vez y se olvida al
  // cambiar de plataforma o salir de las instrucciones.
  const [client, setClient] = useState<OneClickState | null>(null);
  const toast = useToast();

  /** `what` es la clave del catálogo que dice QUÉ se ha copiado. */
  async function copy(text: string, what: PortalMessageKey) {
    try {
      await navigator.clipboard.writeText(text);
      toast.notify({ tone: 'success', title: t(what) });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('devices.copyFailTitle'),
        description: t('devices.copyFailBody'),
      });
    }
  }
  const onCopy = (what: PortalMessageKey) => (value: string) => void copy(value, what);

  /**
   * «Configurar Thunderbird/Outlook/en Android»: crea una contraseña de aplicación con el
   * nombre del cliente (Mi cuenta la lista y la revoca), la copia y la enseña. La web no
   * puede abrir el programa: el resto son los pasos de debajo.
   */
  async function setUp(p: OneClickPlatform) {
    if (client?.busy) return;
    setClient({ platform: p, busy: true });
    try {
      const res = await fetch(r('/api/app-passwords'), {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ label: ONE_CLICK_LABEL[p] }),
      });
      const { password } = res.ok ? ((await res.json()) as { password?: string }) : {};
      if (!password) throw new Error(String(res.status));
      setClient({ platform: p, password });
      await copy(password, 'devices.copied.password');
    } catch {
      setClient({ platform: p, failed: true });
    }
  }

  function oneClick(p: OneClickPlatform): ComponentChildren {
    const c = client?.platform === p ? client : null;
    return (
      <>
        <div class="flex flex-col gap-2">
          <Button class="self-start" loading={c?.busy ?? false} onClick={() => void setUp(p)}>
            {t(`devices.oneClick.${p}`)}
          </Button>
          <FieldHint>{t('devices.oneClick.body')}</FieldHint>
        </div>
        {c?.failed ? (
          <InlineHelp tone="danger" title={t('devices.oneClick.failTitle')}>
            {t('devices.oneClick.failBody')}
          </InlineHelp>
        ) : null}
        {c?.password ? (
          <>
            <InlineHelp tone="success" title={t('devices.oneClick.onceTitle')}>
              {t('devices.oneClick.onceBody')}
            </InlineHelp>
            <div class="flex flex-col gap-2">
              <CopyRow
                t={t}
                label={t('devices.desktop.emailLabel')}
                value={email}
                onCopy={onCopy('devices.copied.address')}
              />
              <CopyRow
                t={t}
                label={t('devices.oneClick.passwordLabel')}
                value={c.password}
                onCopy={onCopy('devices.copied.password')}
              />
              <p class="text-text-muted text-sm">{t(`devices.oneClick.next.${p}`)}</p>
            </div>
          </>
        ) : null}
      </>
    );
  }

  // Sin coordenadas no hay instrucciones honestas que dar: se dice y punto.
  if (!endpoints) {
    return (
      <SettingsShell title={t('devices.title')} description={t('devices.leadShort')}>
        <SettingsSection title={t('devices.unavailable.section')}>
          <EmptyState
            title={t('devices.unavailable.title')}
            description={t('devices.unavailable.body')}
            action={
              <Button variant="secondary" onClick={() => location.reload()}>
                {t('devices.retry')}
              </Button>
            }
          />
        </SettingsSection>
      </SettingsShell>
    );
  }

  const {
    imap,
    smtp,
    caldav_url: caldavUrl,
    carddav_url: carddavUrl,
    webdav_url: webdavUrl,
  } = endpoints;

  /**
   * Direcciones base de la capa de COMPATIBILIDAD: Drive, Calendar y Contacts sirven las
   * APIs de Google y de Microsoft Graph bajo `/compat/…` sobre su propio host, con el
   * mismo Bearer de Sytue ID y los mismos permisos que la API nativa.
   *
   * Se derivan del host del portal y no de una variable aparte: los productos viven en
   * subdominios del MISMO dominio de la organización, y dos fuentes para el mismo dato
   * acabarían discrepando. `location` solo existe en el navegador; en el render del
   * servidor se cae al dominio de producción, que es lo que verá quien mire la página
   * antes de que hidrate.
   */
  const suiteDomain =
    typeof location !== 'undefined'
      ? location.hostname.split('.').slice(1).join('.') || 'sytue.eu'
      : 'sytue.eu';
  const COMPAT_BASES: { label: string; url: string }[] = [
    // La identidad primero: una app escrita para Google empieza por su OAuth (y el
    // directorio de Workspace vive ahí también).
    { label: t('devices.apis.identityGoogle'), url: `https://id.${suiteDomain}/compat/google` },
    { label: t('devices.apis.driveGoogle'), url: `https://drive.${suiteDomain}/compat/google` },
    { label: t('devices.apis.driveGraph'), url: `https://drive.${suiteDomain}/compat/graph` },
    {
      label: t('devices.apis.calendarGoogle'),
      url: `https://calendar.${suiteDomain}/compat/google`,
    },
    { label: t('devices.apis.calendarGraph'), url: `https://calendar.${suiteDomain}/compat/graph` },
    {
      label: t('devices.apis.contactsGoogle'),
      url: `https://contacts.${suiteDomain}/compat/google`,
    },
    { label: t('devices.apis.contactsGraph'), url: `https://contacts.${suiteDomain}/compat/graph` },
  ];

  // --- Bloques de valores reutilizados por varias plataformas ---

  const mailValues = (
    <div class="flex flex-col gap-2">
      <CopyRow
        t={t}
        label={t('devices.mail.address')}
        value={email}
        onCopy={onCopy('devices.copied.address')}
      />
      <CopyRow
        t={t}
        label={t('devices.mail.imapHost')}
        value={imap.host}
        onCopy={onCopy('devices.copied.imap')}
      />
      {/* El puerto es un identificador, no una cantidad: nunca lleva separador de millares. */}
      <CopyRow
        t={t}
        label={t('devices.mail.imapPort')}
        value={String(imap.port)}
        onCopy={onCopy('devices.copied.port')}
      />
      <CopyRow
        t={t}
        label={t('devices.mail.smtpHost')}
        value={smtp.host}
        onCopy={onCopy('devices.copied.smtp')}
      />
      <CopyRow
        t={t}
        label={t('devices.mail.smtpPort')}
        value={String(smtp.port)}
        onCopy={onCopy('devices.copied.port')}
      />
      <FieldHint>{t('devices.mail.hint')}</FieldHint>
    </div>
  );

  const davValues = (
    <div class="flex flex-col gap-2">
      <CopyRow
        t={t}
        label={t('devices.dav.caldav')}
        value={caldavUrl}
        onCopy={onCopy('devices.copied.url')}
      />
      <CopyRow
        t={t}
        label={t('devices.dav.carddav')}
        value={carddavUrl}
        onCopy={onCopy('devices.copied.url')}
      />
      <CopyRow
        t={t}
        label={t('devices.dav.user')}
        value={email}
        onCopy={onCopy('devices.copied.address')}
      />
    </div>
  );

  // --- Instrucciones por plataforma ---

  function instructions(p: Platform): ComponentChildren {
    switch (p) {
      case 'iphone':
      case 'mac':
        return (
          <div class="flex flex-col gap-5">
            {/* Formulario POST y no enlace: la descarga CREA la contraseña de aplicación
                que va dentro del perfil, y eso no puede dispararlo una precarga. */}
            <form method="post" action={r('/api/apple-profile')} class="self-start">
              <button
                type="submit"
                class="bg-primary text-md sytue-focus-ring-inset rounded-control inline-flex min-h-11 max-w-full items-center justify-center gap-2 self-start px-5 py-2.5 text-center font-semibold text-[var(--sytue-on-primary)] transition-colors duration-150 hover:bg-[var(--sytue-primary-hover)]"
              >
                {t('devices.apple.download')}
              </button>
            </form>
            <ol class="flex flex-col gap-3">
              <Step n={1} title={t('devices.apple.step1.title')}>
                {t('devices.apple.step1.body')}
              </Step>
              {p === 'iphone' ? (
                <Step n={2} title={t('devices.apple.step2.iphone.title')}>
                  {t('devices.apple.step2.iphone.body')}
                </Step>
              ) : (
                <Step n={2} title={t('devices.apple.step2.mac.title')}>
                  {t('devices.apple.step2.mac.body')}
                </Step>
              )}
              <Step n={3} title={t('devices.apple.step3.title')}>
                {t('devices.apple.step3.body')}
              </Step>
              <Step n={4} title={t('devices.apple.step4.title')}>
                {t('devices.apple.step4.body')}
              </Step>
            </ol>
            <InlineHelp tone="info" title={t('devices.apple.manualTitle')}>
              {t('devices.apple.manualBody')}
            </InlineHelp>
            <Details summary={t('devices.manualValues')}>
              <div class="flex flex-col gap-4">
                {mailValues}
                {davValues}
              </div>
            </Details>
          </div>
        );

      case 'outlook':
      case 'thunderbird':
        return (
          <div class="flex flex-col gap-5">
            {oneClick(p)}
            <ol class="flex flex-col gap-3">
              <Step n={1} title={t('devices.desktop.step1.title')}>
                {p === 'outlook'
                  ? t('devices.desktop.step1.outlook')
                  : t('devices.desktop.step1.thunderbird')}
              </Step>
              <Step n={2} title={t('devices.desktop.step2.title')}>
                {p === 'outlook'
                  ? t('devices.desktop.step2.outlook')
                  : t('devices.desktop.step2.thunderbird')}
              </Step>
              <Step n={3} title={t('devices.desktop.step3.title')}>
                {t('devices.desktop.step3.body')}
              </Step>
            </ol>
            <div class="flex flex-col gap-2">
              <CopyRow
                t={t}
                label={t('devices.desktop.emailLabel')}
                value={email}
                onCopy={onCopy('devices.copied.address')}
              />
            </div>
            <InlineHelp tone="warning" title={t('devices.desktop.notFoundTitle')}>
              {t('devices.desktop.notFoundBody')}
            </InlineHelp>
            <Details summary={t('devices.manualValues')}>{mailValues}</Details>
            {p === 'thunderbird' ? (
              <SettingsSection
                title={t('devices.thunderbird.davTitle')}
                description={t('devices.thunderbird.davBody')}
              >
                {davValues}
              </SettingsSection>
            ) : (
              <InlineHelp tone="neutral" title={t('devices.outlook.davTitle')}>
                {t('devices.outlook.davBody')}
              </InlineHelp>
            )}
          </div>
        );

      case 'android':
        return (
          <div class="flex flex-col gap-5">
            {oneClick(p)}
            <SettingsSection
              title={t('devices.android.mailTitle')}
              description={t('devices.android.mailBody')}
            >
              <ol class="mb-3 flex flex-col gap-3">
                <Step n={1} title={t('devices.android.mail1.title')}>
                  {t('devices.android.mail1.body')}
                </Step>
                <Step n={2} title={t('devices.android.mail2.title')}>
                  {t('devices.android.mail2.body')}
                </Step>
              </ol>
              {mailValues}
            </SettingsSection>
            <SettingsSection
              title={t('devices.android.davTitle')}
              description={t('devices.android.davBody')}
            >
              <ol class="mb-3 flex flex-col gap-3">
                <Step n={1} title={t('devices.android.dav1.title')}>
                  {t('devices.android.dav1.body')}
                </Step>
                <Step n={2} title={t('devices.android.dav2.title')}>
                  {t('devices.android.dav2.body')}
                </Step>
                <Step n={3} title={t('devices.android.dav3.title')}>
                  {t('devices.android.dav3.body')}
                </Step>
              </ol>
              {davValues}
            </SettingsSection>
          </div>
        );

      case 'drive':
        return (
          <div class="flex flex-col gap-5">
            <ol class="flex flex-col gap-3">
              <Step n={1} title={t('devices.drive.step1.title')}>
                {t('devices.drive.step1.body')}
              </Step>
              <Step n={2} title={t('devices.drive.step2.title')}>
                {t('devices.drive.step2.body')}
              </Step>
              <Step n={3} title={t('devices.drive.step3.title')}>
                {t('devices.drive.step3.body')}
              </Step>
            </ol>
            <div class="flex flex-col gap-2">
              <CopyRow
                t={t}
                label={t('devices.drive.urlLabel')}
                value={webdavUrl}
                onCopy={onCopy('devices.copied.drive')}
              />
              <CopyRow
                t={t}
                label={t('devices.dav.user')}
                value={email}
                onCopy={onCopy('devices.copied.address')}
              />
            </div>
            <InlineHelp tone="info" title={t('devices.drive.expectTitle')}>
              {t('devices.drive.expectBody')}
            </InlineHelp>
          </div>
        );

      case 'apis':
        return (
          <div class="flex flex-col gap-5">
            <ol class="flex flex-col gap-3">
              <Step n={1} title={t('devices.apis.step1.title')}>
                {t('devices.apis.step1.body')}
              </Step>
              <Step n={2} title={t('devices.apis.step2.title')}>
                {t('devices.apis.step2.body')}
              </Step>
              <Step n={3} title={t('devices.apis.step3.title')}>
                {t('devices.apis.step3.body')}
              </Step>
            </ol>
            {/* Una dirección por producto y dialecto: la herramienta pide UNA base y
                traducirla mal es media integración que falla en silencio. */}
            <div class="flex flex-col gap-2">
              {COMPAT_BASES.map((b) => (
                <CopyRow
                  key={b.label}
                  t={t}
                  label={b.label}
                  value={b.url}
                  onCopy={onCopy('devices.copied.apiBase')}
                />
              ))}
            </div>
            <InlineHelp tone="info" title={t('devices.apis.expectTitle')}>
              {t('devices.apis.expectBody')}
            </InlineHelp>
          </div>
        );
    }
  }

  // --- Cómo comprobar que ha funcionado ---

  function checks(p: Platform): { label: string; value: string }[] {
    const mail = { label: t('devices.check.mail.label'), value: t('devices.check.mail.value') };
    const dav = { label: t('devices.check.dav.label'), value: t('devices.check.dav.value') };
    switch (p) {
      case 'iphone':
      case 'mac':
        return [
          { label: t('devices.check.profile.label'), value: t('devices.check.profile.value') },
          mail,
          dav,
        ];
      case 'thunderbird':
        return [mail, dav];
      case 'outlook':
        return [mail];
      case 'android':
        return [mail, dav];
      case 'drive':
        return [
          { label: t('devices.check.drive.label'), value: t('devices.check.drive.value') },
          { label: t('devices.check.write.label'), value: t('devices.check.write.value') },
        ];
      case 'apis':
        return [{ label: t('devices.check.apis.label'), value: t('devices.check.apis.value') }];
    }
  }

  // --- Pasos del asistente ---

  const steps: WizardStep[] = [
    {
      id: 'que-usas',
      title: t('devices.step1.title'),
      heading: t('devices.step1.heading'),
      description: t('devices.step1.description'),
      canContinue: platform !== null,
      content: (
        <div class="grid max-w-3xl gap-3 sm:grid-cols-2">
          {PLATFORMS.map((p) => (
            <ChoiceCard
              key={p}
              name="plataforma"
              value={p}
              title={t(`devices.platform.${p}.title`)}
              description={t(`devices.platform.${p}.body`)}
              footnote={t(`devices.platform.${p}.note`)}
              selected={platform === p}
              onSelect={() => {
                setPlatform(p);
                setClient(null);
              }}
            />
          ))}
        </div>
      ),
    },
    {
      id: 'como',
      title: t('devices.step2.title'),
      heading: platform
        ? t('devices.step2.heading', { platform: t(`devices.platform.${platform}.title`) })
        : t('devices.step2.headingFallback'),
      description: t('devices.step2.description'),
      nextLabel: t('devices.step2.next'),
      content: (
        <div class="flex max-w-3xl flex-col gap-4">
          {platform ? (
            <>
              <DataFlow compact incoming={SYNCS[platform]} outgoing={SYNCS[platform]} />
              {instructions(platform)}
            </>
          ) : null}
        </div>
      ),
    },
    {
      id: 'comprobar',
      title: t('devices.step3.title'),
      heading: t('devices.step3.heading'),
      description: t('devices.step3.description'),
      done: true,
      content: (
        <div class="flex max-w-3xl flex-col gap-4">
          <ReviewSummary
            items={(platform ? checks(platform) : []).map((c) => ({
              label: c.label,
              value: c.value,
            }))}
          />
          <InlineHelp tone="info" title={t('devices.step3.helpTitle')}>
            {t('devices.step3.helpBody')}
          </InlineHelp>
          <div>
            <Button
              variant="secondary"
              onClick={() => {
                setPlatform(null);
                setClient(null);
                setStep(0);
              }}
            >
              {t('devices.step3.again')}
            </Button>
          </div>
        </div>
      ),
    },
  ];

  return (
    <SettingsShell title={t('devices.title')} description={t('devices.lead')}>
      <Wizard
        steps={steps}
        current={step}
        onCurrentChange={(s) => {
          if (s !== 1) setClient(null);
          setStep(s);
        }}
      />
    </SettingsShell>
  );
}

/** Paso numerado de una instrucción. */
function Step({
  n,
  title,
  children,
}: {
  n: number;
  title: string;
  children: ComponentChildren;
}): JSX.Element {
  return (
    <li class="flex gap-3">
      <span
        aria-hidden="true"
        class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold"
      >
        {n}
      </span>
      <div class="min-w-0">
        <p class="text-text text-sm font-medium">{title}</p>
        <p class="text-text-muted mt-0.5 text-sm leading-snug">{children}</p>
      </div>
    </li>
  );
}

/** Valor con botón de copiar: el usuario nunca teclea un servidor a mano. */
function CopyRow({
  t,
  label,
  value,
  onCopy,
}: {
  /** `t` ya atada al idioma negociado en el servidor; llega desde la isla raíz. */
  t: T;
  label: string;
  value: string;
  onCopy: (value: string) => void;
}): JSX.Element {
  return (
    <div class="border-border bg-surface flex flex-wrap items-center gap-2 rounded-md border px-3 py-2">
      {/* `min-w-0` + `break-words`: en alemán la etiqueta es ~30 % más larga y no
          puede empujar al valor ni al botón fuera de la fila. */}
      <span class="sytue-meta w-full shrink-0 break-words sm:w-52 sm:min-w-0">{label}</span>
      <code class="text-text min-w-0 flex-1 font-mono text-xs break-all">{value || '—'}</code>
      <Button variant="ghost" size="sm" disabled={!value} onClick={() => onCopy(value)}>
        {t('devices.copy')}
      </Button>
    </div>
  );
}

/** Detalle plegado nativo: lo secundario no ocupa pantalla hasta que hace falta. */
function Details({
  summary,
  children,
}: {
  summary: string;
  children: ComponentChildren;
}): JSX.Element {
  return (
    <details class="border-border bg-bg-subtle rounded-md border">
      <summary class="text-text cursor-pointer px-3 py-2 text-sm font-medium">{summary}</summary>
      <div class="px-3 pt-1 pb-3">{children}</div>
    </details>
  );
}
