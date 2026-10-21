/*
 * Contraseñas de aplicación: la credencial con la que un cliente de correo entra
 * por IMAP y SMTP, para cuentas con verificación en dos pasos donde la contraseña
 * normal no sirve.
 *
 * La recién creada se enseña UNA vez. Solo se guarda su hash, así que cerrar esto
 * sin copiarla es perderla —y hay que revocar y crear otra—. La pantalla obliga a
 * confirmar antes de quitarla de en medio, igual que con los códigos de
 * recuperación.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, DataFlow, EmptyState, FieldGroup, Input } from '@sytue/ui';
import type { T } from '../i18n/index.js';

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
  const [enviando, setEnviando] = useState(false);

  async function cargar(): Promise<void> {
    try {
      const res = await fetch('/api/app-passwords');
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { app_passwords?: AppPassword[] };
      setLista(datos.app_passwords ?? []);
      setError(false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar();
  }, []);

  async function crear(e: Event): Promise<void> {
    e.preventDefault();
    setEnviando(true);
    try {
      const res = await fetch('/api/app-passwords', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ label: etiqueta.trim() }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const d = (await res.json()) as { label: string; password: string };
      setCreada({ label: d.label, password: d.password });
      setEtiqueta('');
      await cargar();
    } catch {
      setError(true);
    } finally {
      setEnviando(false);
    }
  }

  async function revocar(id: string): Promise<void> {
    await fetch(`/api/app-passwords/${id}`, { method: 'DELETE' });
    await cargar();
  }

  const fecha = (iso: string): string => {
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(locale, { dateStyle: 'medium' });
  };

  return (
    <Card class="mb-6 break-inside-avoid">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('app_passwords.title')}</h2>
        <p class="text-text-muted text-xs">{t('app_passwords.hint')}</p>
        {/* El cliente de correo que la use lee el buzón y envía como la persona. */}
        <DataFlow compact incoming={['mail']} outgoing={['mail']} />

        {error && <p class="text-danger text-sm">{t('app_passwords.failed')}</p>}
        {!error && lista === null && <p class="text-text-muted text-sm">{t('common.loading')}</p>}
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
            <div>
              <Button variant="ghost" onClick={() => setCreada(null)}>
                {t('app_passwords.saved')}
              </Button>
            </div>
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
              <Button variant="ghost" onClick={() => void revocar(p.id)}>
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
    </Card>
  );
}

export default AppPasswordsCard;
