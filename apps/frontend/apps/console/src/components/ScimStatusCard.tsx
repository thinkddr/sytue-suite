/**
 * ScimStatusCard — conectar el directorio corporativo, explicado donde se hace.
 *
 * Aquí hubo dos controles fingidos: un KPI «Provisioning: OK» y un distintivo
 * «SCIM activo», los dos escritos en el JSX y sin consultar nada. Decían que las
 * altas y las BAJAS del directorio se aplicaban solas mientras nadie revocaba
 * ninguna cuenta al salir de la empresa.
 *
 * Y cuando se retiraron quedó el problema de fondo: **no había ningún sitio que
 * explicara el token ni cómo configurarlo**, porque no se podía configurar. El
 * token era una variable de entorno de la plataforma, así que conectar Entra u
 * Okta era pedirle a Sytue que editara el despliegue.
 *
 * Ahora es autoservicio y la pantalla es el manual: tres pasos numerados, los
 * dos valores que hay que pegar en el otro extremo con su botón de copiar, y el
 * estado medido debajo. Nada que buscar en una documentación aparte —el sitio
 * donde se hace la tarea es el sitio donde se explica—.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Input,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { ScimStatus, ScimToken } from '../lib/admin.js';

/** Un valor que hay que llevarse a Entra/Okta, con su botón de copiar. */
function ValorCopiable({ label, valor, t }: { label: string; valor: string; t: T }) {
  const { notify } = useToast();
  return (
    <div class="flex flex-col gap-1">
      <span class="sytue-overline text-text-subtle">{label}</span>
      <div class="flex items-center gap-2">
        <code class="bg-surface-sunken text-text min-w-0 flex-1 truncate rounded-md px-2 py-1.5 text-xs">
          {valor}
        </code>
        <Button
          size="sm"
          variant="secondary"
          onClick={() => {
            void navigator.clipboard
              ?.writeText(valor)
              .then(() => notify({ tone: 'success', title: t('scim.copied') }))
              .catch(() => undefined);
          }}
        >
          {t('scim.copy')}
        </Button>
      </div>
    </div>
  );
}

function Paso({
  n,
  title,
  children,
}: {
  n: number;
  title: string;
  children: preact.ComponentChildren;
}) {
  return (
    <li class="flex gap-3">
      <span class="bg-primary mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold text-[var(--sytue-on-primary)]">
        {n}
      </span>
      <div class="min-w-0 flex-1">
        <p class="text-text text-sm font-semibold">{title}</p>
        <div class="mt-1.5">{children}</div>
      </div>
    </li>
  );
}

function Inner({
  status,
  live,
  locale,
  t,
}: {
  status: ScimStatus | null;
  live: boolean;
  locale: Locale;
  t: T;
}) {
  const { notify } = useToast();
  const [tokens, setTokens] = useState<ScimToken[]>(status?.tokens ?? []);
  // El secreto recién creado. Vive SOLO en esta pantalla y solo hasta recargar:
  // no se puede releer, y decirlo es parte del paso.
  const [nuevo, setNuevo] = useState('');
  const [label, setLabel] = useState('');
  const [creando, setCreando] = useState(false);

  // Sin respuesta del servicio no se inventa un estado: se dice que no se sabe.
  // Pintar «no configurado» cuando lo que pasa es que el IdP no contesta es
  // volver al mismo problema por el otro lado.
  const desconocido = !live || status === null;
  const activo = !desconocido && (tokens.length > 0 || status.configured);

  async function crear() {
    if (creando) return;
    setCreando(true);
    try {
      const res = await fetch('/api/admin/scim-tokens', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ label }),
      });
      if (!res.ok) throw new Error('http');
      const data = (await res.json()) as { id: string; token: string };
      setNuevo(data.token);
      setTokens((t0) => [
        { id: data.id, label, created_at: new Date().toISOString(), last_used_at: null },
        ...t0,
      ]);
      setLabel('');
    } catch {
      notify({ tone: 'danger', title: t('scim.tokenFailed') });
    } finally {
      setCreando(false);
    }
  }

  async function revocar(id: string) {
    try {
      const res = await fetch(`/api/admin/scim-tokens?id=${encodeURIComponent(id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('http');
      setTokens((t0) => t0.filter((x) => x.id !== id));
      notify({ tone: 'success', title: t('scim.tokenRevoked') });
    } catch {
      notify({ tone: 'danger', title: t('scim.tokenRevokeFailed') });
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('scim.title')}</CardTitle>
        {desconocido ? (
          <Badge tone="neutral">{t('scim.unknown')}</Badge>
        ) : (
          <Badge tone={activo ? 'success' : 'neutral'}>
            {activo ? t('scim.on') : t('scim.off')}
          </Badge>
        )}
      </CardHeader>
      <CardBody>
        {desconocido ? (
          <p class="text-text-muted text-sm">{t('scim.unknownHint')}</p>
        ) : (
          <div class="flex flex-col gap-5">
            <p class="text-text-muted text-sm">{activo ? t('scim.onHint') : t('scim.offHint')}</p>

            <ol class="flex flex-col gap-4">
              <Paso n={1} title={t('scim.step1')}>
                <ValorCopiable label={t('scim.endpoint')} valor={status.baseUrl} t={t} />
              </Paso>

              <Paso n={2} title={t('scim.step2')}>
                {nuevo ? (
                  <div class="flex flex-col gap-2">
                    <ValorCopiable label={t('scim.token')} valor={nuevo} t={t} />
                    {/* Se dice ANTES de que cierre la pantalla, no después: el
                        secreto no se guarda en ninguna parte legible. */}
                    <p class="text-warning-text text-xs">{t('scim.tokenOnce')}</p>
                  </div>
                ) : (
                  <div class="flex flex-wrap items-end gap-2">
                    <label class="flex flex-col gap-1 text-sm">
                      {t('scim.tokenLabel')}
                      <Input
                        value={label}
                        placeholder={t('scim.tokenLabelHint')}
                        onInput={(e) => setLabel(e.currentTarget.value)}
                      />
                    </label>
                    <Button onClick={() => void crear()} disabled={creando}>
                      {t('scim.generate')}
                    </Button>
                  </div>
                )}
              </Paso>

              <Paso n={3} title={t('scim.step3')}>
                <p class="text-text-muted text-sm">{t('scim.step3Hint')}</p>
              </Paso>
            </ol>

            <dl class="grid gap-3 sm:grid-cols-2">
              <div>
                <dt class="sytue-overline text-text-subtle">{t('scim.provisioned')}</dt>
                <dd class="text-text text-lg font-semibold">{status.usersProvisioned}</dd>
              </div>
              <div>
                <dt class="sytue-overline text-text-subtle">{t('scim.lastEvent')}</dt>
                <dd class="text-text text-sm">
                  {status.lastEventAt
                    ? formatDateTime(locale, status.lastEventAt)
                    : t('scim.lastEventNever')}
                </dd>
              </div>
            </dl>

            {tokens.length > 0 ? (
              <div class="flex flex-col gap-2">
                <span class="sytue-overline text-text-subtle">{t('scim.tokensLive')}</span>
                {tokens.map((k) => (
                  <div key={k.id} class="flex items-center justify-between gap-3">
                    <span class="min-w-0 truncate text-sm">
                      {k.label || t('scim.tokenNoLabel')}
                      <span class="text-text-subtle ml-2 text-xs">
                        {k.last_used_at
                          ? t('scim.tokenUsed', { when: formatDateTime(locale, k.last_used_at) })
                          : t('scim.tokenNeverUsed')}
                      </span>
                    </span>
                    <Button size="sm" variant="danger" onClick={() => void revocar(k.id)}>
                      {t('scim.revoke')}
                    </Button>
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        )}
      </CardBody>
    </Card>
  );
}

export default function ScimStatusCard(props: {
  status: ScimStatus | null;
  live: boolean;
  locale: Locale;
  t: T;
}) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
