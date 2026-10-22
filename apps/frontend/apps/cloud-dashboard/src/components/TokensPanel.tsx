/**
 * TokensPanel — tokens de API de la organización: la credencial con la que se usan
 * el CLI `sytue-cloud`, el provider de Terraform y cualquier CI.
 *
 * El backend existía completo y no había forma de emitir un token desde el
 * producto, así que dos piezas ya construidas (CLI y Terraform) eran inusables
 * para un cliente real.
 *
 * Dos decisiones de honestidad:
 *  - El secreto se muestra UNA vez, al crearlo, con aviso explícito: el
 *    control-plane guarda solo el hash y no lo vuelve a entregar.
 *  - Sin control-plane NO se inventa ningún token de ejemplo. Un token falso
 *    invita a copiarlo y a que falle en cualquier sitio menos aquí.
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
  ToastProvider,
  useToast,
  useConfirm,
  Radio,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';
import type { ApiToken } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff } from '../lib/bff-client.js';
import DemoBanner from './DemoBanner.js';

/** Qué permite cada scope, en lenguaje llano (espejo de domain.TokenScope). */
function scopes(t: T): { value: ApiToken['scope']; label: string; hint: string }[] {
  return [
    { value: 'read', label: t('tokens.scope.read'), hint: t('tokens.scope.read.hint') },
    { value: 'write', label: t('tokens.scope.write'), hint: t('tokens.scope.write.hint') },
    { value: 'deploy', label: t('tokens.scope.deploy'), hint: t('tokens.scope.deploy.hint') },
  ];
}

const scopeTone = {
  read: 'neutral',
  write: 'primary',
  deploy: 'warning',
  hook: 'warning',
} as const;

interface Props {
  tokens: ApiToken[];
  live: boolean;
  error?: { status: number; code: string } | undefined;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function TokensPanel(props: Props) {
  return (
    <ToastProvider>
      <Panel {...props} />
    </ToastProvider>
  );
}

function Panel({ tokens: initial, live, error, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  // Confirmación del sistema de diseño; ver MembersPanel.
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const SCOPES = scopes(t);
  const [tokens, setTokens] = useState<ApiToken[]>(initial);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [scope, setScope] = useState<ApiToken['scope']>('read');
  const [busy, setBusy] = useState(false);
  /** Secreto recién creado: se enseña una vez y se descarta al cerrar. */
  const [fresh, setFresh] = useState<{ token: ApiToken; plaintext: string } | null>(null);

  async function create(e: Event) {
    e.preventDefault();
    const clean = name.trim();
    if (!clean || busy) return;
    setBusy(true);
    try {
      const res = await postBff<{
        ok: boolean;
        live: boolean;
        token: ApiToken;
        plaintext: string;
      }>('/api/tokens', { name: clean, scope });
      setTokens((prev) => [res.token, ...prev]);
      setFresh({ token: res.token, plaintext: res.plaintext });
      setName('');
      setOpen(false);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('tokens.create.fail'),
        description: t('tokens.create.fail.desc'),
      });
    } finally {
      setBusy(false);
    }
  }

  // El parámetro se llama `tok`, no `t`: `t` es la función de traducción y
  // ensombrecerla aquí dejaría este bloque en inglés pasara lo que pasara.
  async function revoke(tok: ApiToken) {
    const ok = await confirmar({
      titulo: t('tokens.revoke.title'),
      mensaje: t('tokens.revoke.confirm', { name: tok.name }),
      confirmLabel: t('tokens.revoke'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await postBff<{ ok: boolean; live: boolean }>('/api/tokens', { id: tok.id, delete: true });
      setTokens((prev) => prev.filter((x) => x.id !== tok.id));
      toast.notify({ tone: 'success', title: t('tokens.revoked') });
    } catch {
      toast.notify({ tone: 'danger', title: t('tokens.revoke.fail') });
    }
  }

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        icon={<Icon name="lock" size={20} />}
        title={t('nav.tokens')}
        subtitle={t('tokens.subtitle')}
        actions={
          <Button variant="primary" size="sm" onClick={() => setOpen(true)}>
            {t('tokens.create')}
          </Button>
        }
        level={1}
      />

      <div class="cloud-content">
        {tokens.length === 0 ? (
          <EmptyState
            illustration={<Icon name="lock" size={24} />}
            title={t('tokens.empty.title')}
            description={t('tokens.empty.desc')}
          />
        ) : (
          <Card>
            <CardBody class="p-0">
              <ul class="divide-border divide-y">
                {/* `tok`, no `t`: ver el comentario de `revoke`. */}
                {tokens.map((tok) => (
                  <li key={tok.id} class="flex items-center justify-between gap-3 px-4 py-3">
                    <div class="min-w-0">
                      <p class="text-text truncate text-sm font-medium">{tok.name}</p>
                      {/* Dos hechos independientes separados por «·», no una frase
                          partida: cada mitad se traduce entera por su cuenta. */}
                      <p class="sytue-meta">
                        {tok.lastUsedAt
                          ? t('tokens.lastUsed', { date: formatDate(locale, tok.lastUsedAt) })
                          : t('tokens.neverUsed')}
                        {' · '}
                        {tok.projectId ? t('tokens.scopedProject') : t('tokens.scopedOrg')}
                      </p>
                    </div>
                    <div class="flex shrink-0 items-center gap-2">
                      <Badge tone={scopeTone[tok.scope]}>
                        {SCOPES.find((s) => s.value === tok.scope)?.label ?? tok.scope}
                      </Badge>
                      <Button variant="ghost" size="sm" onClick={() => void revoke(tok)}>
                        {t('tokens.revoke')}
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>
        )}
      </div>

      {/* Alta: nombre + alcance, nada más. El resto son valores por defecto sensatos. */}
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t('tokens.modal.title')}
        footer={
          <>
            <Button variant="ghost" type="button" onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="token-nuevo"
              disabled={busy || name.trim() === ''}
            >
              {t('common.create')}
            </Button>
          </>
        }
      >
        <form id="token-nuevo" class="flex flex-col gap-3" onSubmit={(e) => void create(e)}>
          <label class="flex flex-col gap-1">
            <span class="text-text text-sm">{t('common.name')}</span>
            <Input
              value={name}
              placeholder={t('tokens.field.name.placeholder')}
              onInput={(e) => setName((e.target as HTMLInputElement).value)}
            />
            <span class="sytue-meta">{t('tokens.field.name.help')}</span>
          </label>
          <fieldset class="flex flex-col gap-1">
            <legend class="text-text text-sm">{t('tokens.field.scope')}</legend>
            {SCOPES.map((s) => (
              <label
                key={s.value}
                class="flex cursor-pointer items-start gap-2 py-0.5 pointer-coarse:min-h-[max(44px,2.75rem)]"
              >
                <Radio
                  class="accent-primary"
                  name="scope"
                  checked={scope === s.value}
                  onChange={() => setScope(s.value)}
                />
                {/* El nombre del permiso va DIRECTO dentro del span: anidarlo un
                    nivel más lo deja fuera del alcance con que jsx-a11y busca el
                    texto accesible del <label> (profundidad 2) y rompe el lint. */}
                <span class="text-text text-sm">
                  {s.label}
                  <span class="sytue-meta block">{s.hint}</span>
                </span>
              </label>
            ))}
          </fieldset>
        </form>
      </Modal>

      {/* El secreto, una sola vez. Cerrar equivale a perderlo. */}
      <Modal
        open={fresh !== null}
        onClose={() => setFresh(null)}
        title={t('tokens.fresh.title')}
        footer={
          fresh ? (
            <>
              <Button
                variant="ghost"
                onClick={() => {
                  void navigator.clipboard?.writeText(fresh.plaintext);
                  toast.notify({ tone: 'success', title: t('tokens.copied') });
                }}
              >
                {t('copy.copy')}
              </Button>
              <Button variant="primary" onClick={() => setFresh(null)}>
                {t('tokens.saved')}
              </Button>
            </>
          ) : null
        }
      >
        {fresh ? (
          <div class="flex flex-col gap-3">
            <p class="text-text text-sm">{t('tokens.fresh.desc')}</p>
            {/* Entero y partido donde haga falta: con desplazamiento horizontal, en el
                teléfono no se veía el final del token que hay que guardar. */}
            <code class="border-border bg-surface-sunken text-text block rounded-md border px-3 py-2 font-mono text-sm break-all">
              {fresh.plaintext}
            </code>
          </div>
        ) : null}
      </Modal>
      {dialogoConfirmar}
    </>
  );
}
