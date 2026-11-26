/*
 * Contraseñas de aplicación: la credencial con la que un cliente de correo entra
 * por IMAP y SMTP, para cuentas con verificación en dos pasos donde la contraseña
 * normal no sirve.
 *
 * La recién creada se enseña UNA vez. Solo se guarda su hash, así que cerrar esto
 * sin copiarla es perderla —y hay que revocar y crear otra—. La pantalla obliga a
 * confirmar antes de quitarla de en medio, igual que con los códigos de
 * recuperación.
 *
 * Revocar también es irreversible —el cliente que la use deja de entrar al
 * momento—, así que se confirma nombrándola, y si el servicio lo rechaza se dice.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  Card,
  DataFlow,
  EmptyState,
  FieldGroup,
  Icon,
  Input,
  copiarAlPortapapeles,
  useConfirm,
} from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { primeraLectura } from '../lib/primeraLectura.js';
import { FilasCargando } from './FilasCargando.js';

interface AppPassword {
  id: string;
  label: string;
  created_at: string;
  last_used_at?: string | null;
}

export function AppPasswordsCard({ t, locale }: { t: T; locale: string }): JSX.Element {
  const [lista, setLista] = useState<AppPassword[] | null>(null);
  const [etiqueta, setEtiqueta] = useState('');
  const [creada, setCreada] = useState<{ label: string; password: string } | null>(null);
  const [error, setError] = useState(false);
  // Separado de `error` (la CARGA): un alta o una revocación que fallan no son
  // «no se ha podido cargar», y antes lo decían así.
  const [falloAccion, setFalloAccion] = useState(false);
  const [enviando, setEnviando] = useState(false);
  // «Copiar» de la recién creada: es lo que se hace con ella SIEMPRE (pegarla en el
  // cliente de correo), y seleccionar a mano dieciséis caracteres sin espacios es
  // donde se cuela uno de menos.
  const [copia, setCopia] = useState<'nada' | 'hecha' | 'fallo'>('nada');
  const { confirmar, dialogo } = useConfirm();

  // `inicial`: la del montaje se comparte con la revisión de seguridad.
  async function cargar(inicial = false): Promise<void> {
    setError(false);
    try {
      const res = await (inicial
        ? primeraLectura('/api/app-passwords')
        : fetch('/api/app-passwords'));
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { app_passwords?: AppPassword[] };
      setLista(datos.app_passwords ?? []);
      setError(false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar(true);
  }, []);

  async function crear(e: Event): Promise<void> {
    e.preventDefault();
    setEnviando(true);
    setFalloAccion(false);
    try {
      const res = await fetch('/api/app-passwords', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ label: etiqueta.trim() }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const d = (await res.json()) as { label: string; password: string };
      setCreada({ label: d.label, password: d.password });
      setCopia('nada');
      setEtiqueta('');
      await cargar();
    } catch {
      setFalloAccion(true);
    } finally {
      setEnviando(false);
    }
  }

  async function revocar(p: AppPassword): Promise<void> {
    const ok = await confirmar({
      mensaje: t('app_passwords.revoke_confirm', {
        label: p.label || t('app_passwords.unnamed'),
      }),
      confirmLabel: t('app_passwords.revoke'),
      tone: 'danger',
    });
    if (!ok) return;
    setFalloAccion(false);
    try {
      const res = await fetch(`/api/app-passwords/${p.id}`, { method: 'DELETE' });
      if (!res.ok) setFalloAccion(true);
    } catch {
      setFalloAccion(true);
    }
    await cargar();
  }

  const fecha = (iso: string): string => {
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(locale, { dateStyle: 'medium' });
  };

  return (
    <Card id="contrasenas-app">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('app_passwords.title')}</h2>
        <p class="text-text-muted text-xs">{t('app_passwords.hint')}</p>
        {/* El cliente de correo que la use lee el buzón y envía como la persona. */}
        <DataFlow compact incoming={['mail']} outgoing={['mail']} />

        {error && (
          <div role="alert" class="flex items-center justify-between gap-3">
            <p class="text-danger-text text-sm">{t('app_passwords.failed')}</p>
            <Button variant="ghost" onClick={() => void cargar()}>
              {t('common.retry')}
            </Button>
          </div>
        )}
        {falloAccion && (
          <p role="alert" class="text-danger-text text-sm">
            {t('security.failed')}
          </p>
        )}
        {!error && lista === null && (
          <FilasCargando etiqueta={t('common.loading')} accion="boton" />
        )}
        {!error && lista?.length === 0 && !creada && (
          <EmptyState title={t('app_passwords.empty')} />
        )}

        {creada && (
          <div class="border-border flex flex-col gap-2 rounded border p-3">
            <p class="text-sm font-medium">{creada.label || t('app_passwords.unnamed')}</p>
            <p class="text-text-muted text-xs">{t('app_passwords.once')}</p>
            <code class="bg-surface-raised rounded px-2 py-1 font-mono text-base break-all">
              {creada.password}
            </code>
            <div class="flex flex-wrap items-center gap-2">
              <Button
                variant="secondary"
                leadingIcon={<Icon name="copies" size={16} />}
                onClick={() =>
                  void copiarAlPortapapeles(creada.password).then((ok) =>
                    setCopia(ok ? 'hecha' : 'fallo'),
                  )
                }
              >
                {t('common.copy')}
              </Button>
              <Button variant="ghost" onClick={() => setCreada(null)}>
                {t('app_passwords.saved')}
              </Button>
            </div>
            {/* Siempre montada mientras se enseña: una región `status` que nace con
                el texto dentro no la anuncian todos los lectores. */}
            <p
              role="status"
              class={copia === 'fallo' ? 'text-danger-text text-xs' : 'text-success-text text-xs'}
            >
              {copia === 'hecha'
                ? t('common.copied')
                : copia === 'fallo'
                  ? t('common.copy_failed')
                  : ''}
            </p>
          </div>
        )}

        <ul class="flex flex-col gap-2">
          {(lista ?? []).map((p) => (
            <li key={p.id} class="flex items-center justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm">{p.label || t('app_passwords.unnamed')}</p>
                <p class="text-text-muted text-xs">
                  {p.last_used_at
                    ? t('app_passwords.last_used', { date: fecha(p.last_used_at) })
                    : t('app_passwords.never_used')}
                </p>
              </div>
              {/* El texto visible es el verbo; el nombre accesible dice CUÁL. */}
              <Button
                variant="ghost"
                aria-label={t('app_passwords.revoke_named', {
                  label: p.label || t('app_passwords.unnamed'),
                })}
                onClick={() => void revocar(p)}
              >
                {t('app_passwords.revoke')}
              </Button>
            </li>
          ))}
        </ul>

        <form onSubmit={(e) => void crear(e)} class="flex flex-col gap-3">
          <FieldGroup
            label={t('app_passwords.label')}
            htmlFor="ap-etiqueta"
            help={t('app_passwords.label.hint')}
          >
            <Input
              id="ap-etiqueta"
              value={etiqueta}
              onInput={(e: Event) => setEtiqueta((e.target as HTMLInputElement).value)}
              maxLength={60}
            />
          </FieldGroup>
          <div>
            <Button type="submit" disabled={enviando}>
              {t('app_passwords.create')}
            </Button>
          </div>
        </form>
      </div>
      {dialogo}
    </Card>
  );
}

export default AppPasswordsCard;
