/*
 * Acceso y recuperación (ADR 0040 y 0041).
 *
 * Se entra con cuatro cosas —@usuario, dirección de Sytue, móvil y correo de
 * recuperación— y siempre con la misma contraseña. Aquí se cambian tres de ellas: el
 * @usuario, el móvil (o se añade uno, en las cuentas de antes de que se pidiera) y el
 * correo de recuperación. Todo pide la contraseña actual, que se escribe una vez arriba.
 *
 * Ningún identificador nuevo vale hasta que se demuestra: el móvil, con el código que
 * llega por SMS; el correo, con el enlace que llega a esa dirección. Por eso, tras
 * pedirlo, la tarjeta sigue enseñando el anterior. El @usuario que se deja queda
 * reservado 30 días para quien lo dejó.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, EmptyState, FieldGroup, Input } from '@sytue/ui';
import type { AccountMessageKey, T } from '../i18n/index.js';
import { primeraLectura } from '../lib/primeraLectura.js';
import {
  falloDeMovil,
  falloDeUsuario,
  leerCodigo,
  type FalloMovil,
  type FalloUsuario,
} from '../lib/accesoCuenta.js';

type Estado = 'quieto' | 'enviando' | 'enviado' | 'quitado' | 'error';

/** El aviso de una negativa: la contraseña y el «no se ha podido» son los de la tarjeta. */
function avisoUsuario(f: FalloUsuario): AccountMessageKey {
  return f === 'wrong_password' || f === 'failed' ? `recovery.${f}` : `recovery.username.${f}`;
}

function avisoMovil(f: FalloMovil): AccountMessageKey {
  return f === 'wrong_password' || f === 'failed' ? `recovery.${f}` : `recovery.phone.${f}`;
}

export function RecuperacionCard({ t }: { t: T }): JSX.Element {
  const [password, setPassword] = useState('');

  // @usuario
  const [usuario, setUsuario] = useState('');
  const [usuarioNuevo, setUsuarioNuevo] = useState('');
  const [estadoUsuario, setEstadoUsuario] = useState<'quieto' | 'enviando' | 'hecho' | 'error'>(
    'quieto',
  );
  const [falloUsuario, setFalloUsuario] = useState<FalloUsuario>('failed');

  // Móvil
  const [movil, setMovil] = useState('');
  const [movilNuevo, setMovilNuevo] = useState('');
  const [pendiente, setPendiente] = useState('');
  const [codigo, setCodigo] = useState('');
  const [estadoMovil, setEstadoMovil] = useState<'quieto' | 'enviando' | 'hecho' | 'error'>(
    'quieto',
  );
  const [falloMovil, setFalloMovil] = useState<FalloMovil>('failed');

  // Correo de recuperación
  const [actual, setActual] = useState('');
  const [nuevo, setNuevo] = useState('');
  const [estado, setEstado] = useState<Estado>('quieto');
  const [fallo, setFallo] = useState<'invalid' | 'wrong_password' | 'failed'>('failed');

  useEffect(() => {
    void (async () => {
      try {
        const res = await primeraLectura('/api/me');
        if (!res.ok) return;
        const d = (await res.json()) as {
          username?: string;
          phone?: string;
          recovery_email?: string;
        };
        setUsuario(d.username ?? '');
        setMovil(d.phone ?? '');
        setActual(d.recovery_email ?? '');
      } catch {
        /* la tarjeta sigue sirviendo para cambiarlos aunque no se vean los actuales */
      }
    })();
  }, []);

  async function cambiarUsuario(): Promise<void> {
    setEstadoUsuario('enviando');
    const res = await fetch('/api/username', {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ username: usuarioNuevo.trim(), password }),
    }).catch(() => null);
    if (!res?.ok) {
      setFalloUsuario(falloDeUsuario(res?.status ?? 0, await leerCodigo(res)));
      setEstadoUsuario('error');
      return;
    }
    const d = (await res.json().catch(() => ({}))) as { username?: string };
    setUsuario(d.username ?? usuarioNuevo.trim().replace(/^@/, '').toLowerCase());
    setUsuarioNuevo('');
    setEstadoUsuario('hecho');
  }

  async function pedirCodigo(): Promise<void> {
    setEstadoMovil('enviando');
    const res = await fetch('/api/phone', {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ phone: movilNuevo.trim(), password }),
    }).catch(() => null);
    if (!res?.ok) {
      setFalloMovil(falloDeMovil(res?.status ?? 0, await leerCodigo(res)));
      setEstadoMovil('error');
      return;
    }
    const d = (await res.json().catch(() => ({}))) as { pending?: string };
    setPendiente(d.pending ?? movilNuevo.trim());
    setCodigo('');
    setEstadoMovil('quieto');
  }

  async function confirmarCodigo(): Promise<void> {
    setEstadoMovil('enviando');
    const res = await fetch('/api/phone', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ code: codigo.trim() }),
    }).catch(() => null);
    if (!res?.ok) {
      const f = falloDeMovil(res?.status ?? 0, await leerCodigo(res));
      // Sin código vigente, o el número es de otra cuenta: se vuelve a empezar.
      if (f === 'code_expired' || f === 'has_account') setPendiente('');
      setFalloMovil(f);
      setEstadoMovil('error');
      return;
    }
    const d = (await res.json().catch(() => ({}))) as { phone?: string };
    setMovil(d.phone ?? pendiente);
    setPendiente('');
    setMovilNuevo('');
    setCodigo('');
    setEstadoMovil('hecho');
  }

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
    <Card id="recuperacion">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('recovery.title')}</h2>
        <p class="text-text-muted text-xs">{t('recovery.hint')}</p>

        <FieldGroup
          label={t('security.password.current')}
          htmlFor="rec-password"
          help={t('recovery.password.hint')}
          required
        >
          <Input
            id="rec-password"
            type="password"
            autocomplete="current-password"
            value={password}
            onInput={(e: Event) => setPassword((e.target as HTMLInputElement).value)}
            required
          />
        </FieldGroup>

        {/* @usuario */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void cambiarUsuario();
          }}
          class="border-border flex flex-col gap-3 border-t pt-4"
          data-seccion="usuario"
        >
          {usuario ? (
            <FieldGroup label={t('profile.username')} htmlFor="rec-usuario">
              <Input id="rec-usuario" value={`@${usuario}`} readOnly />
            </FieldGroup>
          ) : null}
          <FieldGroup
            label={t('recovery.username.new')}
            htmlFor="rec-usuario-nuevo"
            help={t('recovery.username.hint')}
          >
            <Input
              id="rec-usuario-nuevo"
              autocomplete="username"
              autocapitalize="none"
              spellcheck={false}
              value={usuarioNuevo}
              onInput={(e: Event) => setUsuarioNuevo((e.target as HTMLInputElement).value)}
            />
          </FieldGroup>
          <div class="flex flex-wrap items-center gap-3">
            <Button
              type="submit"
              variant="secondary"
              disabled={estadoUsuario === 'enviando' || !usuarioNuevo.trim() || !password}
            >
              {t('recovery.username.change')}
            </Button>
            <span role="status" class="text-sm">
              {estadoUsuario === 'hecho' && (
                <span class="text-success-text">{t('recovery.username.changed')}</span>
              )}
              {estadoUsuario === 'error' && (
                <span class="text-danger-text">{t(avisoUsuario(falloUsuario))}</span>
              )}
            </span>
          </div>
        </form>

        {/* Móvil */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void (pendiente ? confirmarCodigo() : pedirCodigo());
          }}
          class="border-border flex flex-col gap-3 border-t pt-4"
          data-seccion="movil"
        >
          {movil ? (
            <FieldGroup
              label={t('recovery.phone')}
              htmlFor="rec-movil"
              help={t('recovery.phone.hint')}
            >
              <Input id="rec-movil" value={movil} readOnly />
            </FieldGroup>
          ) : (
            // Sin móvil todavía: el vacío del sistema de diseño, en compacto.
            <EmptyState compact title={t('recovery.phone.none')} />
          )}
          {pendiente ? (
            <>
              <p role="status" class="text-sm">
                {t('recovery.phone.sent', { phone: pendiente })}
              </p>
              <FieldGroup label={t('recovery.phone.code')} htmlFor="rec-codigo" required>
                <Input
                  id="rec-codigo"
                  inputMode="numeric"
                  autocomplete="one-time-code"
                  maxLength={6}
                  value={codigo}
                  onInput={(e: Event) => setCodigo((e.target as HTMLInputElement).value)}
                  required
                />
              </FieldGroup>
            </>
          ) : (
            <FieldGroup
              label={movil ? t('recovery.phone.new') : t('recovery.phone')}
              htmlFor="rec-movil-nuevo"
              help={t('recovery.phone.format')}
            >
              <Input
                id="rec-movil-nuevo"
                type="tel"
                autocomplete="tel"
                value={movilNuevo}
                onInput={(e: Event) => setMovilNuevo((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
          )}
          <div class="flex flex-wrap items-center gap-3">
            {pendiente ? (
              <Button
                type="submit"
                variant="secondary"
                disabled={estadoMovil === 'enviando' || !/^\d{6}$/.test(codigo.trim())}
              >
                {t('recovery.phone.confirm')}
              </Button>
            ) : (
              <Button
                type="submit"
                variant="secondary"
                disabled={estadoMovil === 'enviando' || !movilNuevo.trim() || !password}
              >
                {t('recovery.phone.send')}
              </Button>
            )}
            <span role="status" class="text-sm">
              {estadoMovil === 'hecho' && (
                <span class="text-success-text">{t('recovery.phone.changed')}</span>
              )}
              {estadoMovil === 'error' && (
                <span class="text-danger-text">{t(avisoMovil(falloMovil))}</span>
              )}
            </span>
          </div>
        </form>

        {/* Correo de recuperación */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void guardar(nuevo.trim());
          }}
          class="border-border flex flex-col gap-3 border-t pt-4"
          data-seccion="correo"
        >
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
            <span role="status" class="text-sm">
              {estado === 'error' && <span class="text-danger-text">{t(`recovery.${fallo}`)}</span>}
              {estado === 'enviado' && <span class="text-success-text">{t('recovery.sent')}</span>}
              {estado === 'quitado' && (
                <span class="text-success-text">{t('recovery.removed')}</span>
              )}
            </span>
          </div>
        </form>
      </div>
    </Card>
  );
}

export default RecuperacionCard;
