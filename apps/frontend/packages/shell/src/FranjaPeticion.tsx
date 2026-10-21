/**
 * La franja de solo lectura CON su petición, para cualquier app: la pieza visual es
 * `ReadOnlyBar` de @sytue/ui y aquí está el contrato común de platform/pkg/peticiones,
 * que es igual en todos los servicios:
 *
 *   POST   <endpoint>              pedir acceso de edición
 *   GET    <endpoint>              las peticiones pendientes (quien puede conceder)
 *   POST   <endpoint>&grant=<sub>  conceder
 *   DELETE <endpoint>&sub=<sub>    rechazar
 *
 * `endpoint` es la ruta del BFF de la app con su `?` ya puesto (`/api/edit-requests?id=…`).
 *
 * Tres caras:
 * - (b) sin permiso: «Pedir acceso de edición» y, enviada, «Petición enviada».
 * - quien puede conceder: «Ana pide acceso de edición» con «Dar acceso» y «Rechazar».
 * - (a) el bloqueo es de la persona: no hace falta esto, basta `ReadOnlyBar` con «Editar».
 *
 * EN VIVO. Si la app no tiene canal propio, pasa `concedido` (el tipo del aviso del Hub,
 * `calendar.access.granted`…) y `recurso`: al llegar ese aviso por el flujo de la
 * campana (`sytue:aviso`), se llama a `alConceder` y la app vuelve a leer su permiso.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { ReadOnlyBar, useToast } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import { alLlegarAviso, esConcesion } from './avisos.js';

interface Peticion {
  sub: string;
  name?: string;
  email?: string;
}

export interface FranjaPeticionProps {
  /** Ruta del BFF con su `?` (`/api/edit-requests?id=…`). */
  endpoint: string;
  /** No puede editar: se ofrece pedirlo. */
  soloLectura: boolean;
  /** Puede conceder: se le enseñan las peticiones pendientes. */
  puedeConceder: boolean;
  /** Ya lo pidió (lo dice el servicio al leer el recurso). */
  pedida?: boolean;
  /** Por qué es de solo lectura, ya traducido. */
  mensaje: string;
  locale: Locale;
  /** Tipo del aviso del Hub que desbloquea en vivo, y el recurso que lo identifica. */
  concedido?: string;
  recurso?: string;
  alConceder?: () => void;
}

export function FranjaPeticion(p: FranjaPeticionProps): JSX.Element | null {
  const toast = useToast();
  const tc = (k: Parameters<typeof translate>[1], v?: Record<string, string>) =>
    translate(p.locale, k, v);
  const [pedida, setPedida] = useState(p.pedida ?? false);
  const [enviando, setEnviando] = useState(false);
  const [peticiones, setPeticiones] = useState<Peticion[]>([]);
  const fallo = tc('readOnly.requestFailed');
  const { endpoint, puedeConceder, concedido, recurso, alConceder } = p;

  useEffect(() => setPedida(p.pedida ?? false), [p.pedida]);

  useEffect(() => {
    if (!puedeConceder) return;
    let vivo = true;
    void fetch(endpoint)
      .then((r) => (r.ok ? (r.json() as Promise<{ requests?: Peticion[] }>) : { requests: [] }))
      .then((d) => vivo && setPeticiones(d.requests ?? []))
      .catch(() => undefined);
    return () => {
      vivo = false;
    };
  }, [puedeConceder, endpoint]);

  useEffect(() => {
    if (!concedido || !alConceder) return;
    return alLlegarAviso((n) => {
      if (esConcesion(n, concedido, recurso)) alConceder();
    });
  }, [concedido, recurso, alConceder]);

  const pedir = useCallback(async () => {
    setEnviando(true);
    try {
      const r = await fetch(endpoint, { method: 'POST' });
      if (!r.ok) throw new Error(String(r.status));
      setPedida(true);
    } catch {
      toast.notify({ tone: 'danger', title: fallo });
    } finally {
      setEnviando(false);
    }
  }, [endpoint, toast, fallo]);

  const resolver = useCallback(
    async (sub: string, concede: boolean) => {
      setEnviando(true);
      const s = encodeURIComponent(sub);
      const r = await fetch(concede ? `${endpoint}&grant=${s}` : `${endpoint}&sub=${s}`, {
        method: concede ? 'POST' : 'DELETE',
      }).catch(() => null);
      setEnviando(false);
      // 404: otra pestaña ya la resolvió. En los dos casos, fuera de la lista.
      if (r && (r.ok || r.status === 404)) setPeticiones((ps) => ps.filter((x) => x.sub !== sub));
    },
    [endpoint],
  );

  if (p.soloLectura) {
    return (
      <ReadOnlyBar
        message={p.mensaje}
        action={{
          label: tc('readOnly.requestEdit'),
          onClick: () => void pedir(),
          loading: enviando,
        }}
        {...(pedida ? { note: tc('readOnly.requestSent') } : {})}
      />
    );
  }
  const primera = peticiones[0];
  if (!primera) return null;
  return (
    <ReadOnlyBar
      message={tc('readOnly.editRequest', { name: primera.name || primera.email || primera.sub })}
      action={{
        label: tc('readOnly.grant'),
        onClick: () => void resolver(primera.sub, true),
        loading: enviando,
      }}
      secondaryAction={{
        label: tc('readOnly.decline'),
        onClick: () => void resolver(primera.sub, false),
      }}
    />
  );
}
