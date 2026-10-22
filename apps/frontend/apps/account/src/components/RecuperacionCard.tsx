/*
 * Acceso y recuperación (ADR 0041).
 *
 * Se entra con cuatro cosas —@usuario, dirección de Sytue, móvil y correo de
 * recuperación— y siempre con la misma contraseña. Aquí se ve el móvil, que se
 * verificó por SMS al crear la cuenta y no se cambia desde esta pantalla, y se
 * pone, cambia o quita el correo de recuperación.
 *
 * Tocar el correo de recuperación pide la contraseña, y el nuevo no queda puesto
 * hasta abrir el enlace que llega a esa dirección: por eso, tras enviarlo, la
 * tarjeta sigue enseñando el anterior.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, FieldGroup, Input } from '@sytue/ui';
import type { T } from '../i18n/index.js';

type Estado = 'quieto' | 'enviando' | 'enviado' | 'quitado' | 'error';

export function RecuperacionCard({ t }: { t: T }): JSX.Element {
  const [movil, setMovil] = useState('');
  const [actual, setActual] = useState('');
  const [nuevo, setNuevo] = useState('');
  const [password, setPassword] = useState('');
  const [estado, setEstado] = useState<Estado>('quieto');
  const [fallo, setFallo] = useState<'invalid' | 'wrong_password' | 'failed'>('failed');

  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch('/api/me');
        if (!res.ok) return;
        const d = (await res.json()) as { phone?: string; recovery_email?: string };
        setMovil(d.phone ?? '');
        setActual(d.recovery_email ?? '');
      } catch {
        /* la tarjeta sigue sirviendo para poner uno aunque no se vea el actual */
      }
    })();
  }, []);

  async function guardar(email: string): Promise<void> {
    setEstado('enviando');
    const res = await fetch('/api/recovery-email', {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ email, password }),
    }).catch(() => null);
    if (!res?.ok) {
      setFallo(res?.status === 401 ? 'wrong_password' : res?.status === 400 ? 'invalid' : 'failed');
      setEstado('error');
      return;
    }
    setPassword('');
    if (email) {
      setNuevo('');
      setEstado('enviado');
    } else {
      setActual('');
      setEstado('quitado');
    }
  }

  return (
    <Card class="mb-6 break-inside-avoid">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void guardar(nuevo.trim());
        }}
        class="flex flex-col gap-4 p-4 sm:p-5"
      >
        <h2 class="text-lg font-semibold">{t('recovery.title')}</h2>
        <p class="text-text-muted text-xs">{t('recovery.hint')}</p>

        {movil ? (
          <FieldGroup
            label={t('recovery.phone')}
            htmlFor="rec-movil"
            help={t('recovery.phone.hint')}
          >
            <Input id="rec-movil" value={movil} readOnly />
          </FieldGroup>
        ) : null}

        {actual ? (
          <FieldGroup label={t('recovery.email')} htmlFor="rec-actual">
            <Input id="rec-actual" value={actual} readOnly />
          </FieldGroup>
        ) : null}

        <FieldGroup label={actual ? t('recovery.new') : t('recovery.email')} htmlFor="rec-nuevo">
          <Input
            id="rec-nuevo"
            type="email"
            autocomplete="email"
            value={nuevo}
            onInput={(e: Event) => setNuevo((e.target as HTMLInputElement).value)}
          />
        </FieldGroup>
        <FieldGroup label={t('security.password.current')} htmlFor="rec-password" required>
          <Input
            id="rec-password"
            type="password"
            autocomplete="current-password"
            value={password}
            onInput={(e: Event) => setPassword((e.target as HTMLInputElement).value)}
            required
          />
        </FieldGroup>

        <div class="flex flex-wrap items-center gap-3">
          <Button type="submit" disabled={estado === 'enviando' || !nuevo.trim() || !password}>
            {t('recovery.send')}
          </Button>
          {actual ? (
            <Button
              type="button"
              variant="secondary"
              onClick={() => void guardar('')}
              disabled={estado === 'enviando' || !password}
            >
              {t('recovery.remove')}
            </Button>
          ) : null}
          <span aria-live="polite" class="text-sm">
            {estado === 'enviado' && <span class="text-success">{t('recovery.sent')}</span>}
            {estado === 'quitado' && <span class="text-success">{t('recovery.removed')}</span>}
            {estado === 'error' && <span class="text-danger">{t(`recovery.${fallo}`)}</span>}
          </span>
        </div>
      </form>
    </Card>
  );
}

export default RecuperacionCard;
