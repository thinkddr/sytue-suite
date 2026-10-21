/**
 * «Compartir con una persona», el mismo diálogo en todas las apps que lo tienen
 * (Photos, Notepads): se escribe su correo, se elige el nivel —si el recurso tiene
 * más de uno— y debajo está con quién ya está compartido, con «Revocar».
 *
 * Es la parte de personas del `ShareDialog` de Drive, con sus mismas medidas: el
 * campo de correo `min-w-56 flex-1`, el nivel `w-44` y la lista con divisorias. Habla
 * con el BFF común (`rutasDeCompartir` de @sytue/auth/server):
 *
 *   GET    <endpoint>                         la lista, ya con correo y nombre
 *   POST   <endpoint> {email, permission}     compartir
 *   DELETE <endpoint>&sub=<sub>               revocar
 *
 * `endpoint` es la ruta del BFF de la app con su `?` puesto y su prefijo
 * (`withBase(BASE_URL, '/api/people?album=…')`).
 *
 * Un 404 al compartir es «esa dirección no es de nadie de tu organización», que es
 * información y no un fallo: se queda escrita junto al campo en vez de irse en un aviso.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Button,
  FieldGroup,
  FieldHint,
  Icon,
  Input,
  Modal,
  Select,
  Spinner,
  useToast,
} from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';

/** Los niveles con que se comparte: ver, o ver y editar. */
export type NivelCompartido = 'read' | 'write';

interface Persona {
  subject: string;
  permission: NivelCompartido;
  email?: string;
  name?: string;
}

export interface CompartirConPersonasProps {
  /** Ruta del BFF con su `?` y su prefijo. */
  endpoint: string;
  /** Nombre de lo que se comparte, para el título. */
  nombre: string;
  /** Los niveles que admite el recurso, en orden. Con uno solo no se pregunta. */
  niveles: readonly NivelCompartido[];
  locale: Locale;
  onClose: () => void;
}

export function CompartirConPersonas(p: CompartirConPersonasProps): JSX.Element {
  const toast = useToast();
  const t = (k: Parameters<typeof translate>[1], v?: Record<string, string>) =>
    translate(p.locale, k, v);
  const [personas, setPersonas] = useState<Persona[] | null>(null);
  const [falloLista, setFalloLista] = useState(false);
  const [correo, setCorreo] = useState('');
  const [nivel, setNivel] = useState<NivelCompartido>(p.niveles[0] ?? 'read');
  const [enviando, setEnviando] = useState(false);
  const [errorCorreo, setErrorCorreo] = useState<string | null>(null);
  const [quitando, setQuitando] = useState<string | null>(null);
  const { endpoint } = p;

  const cargar = useCallback(async () => {
    setPersonas(null);
    setFalloLista(false);
    try {
      const r = await fetch(endpoint, { credentials: 'include' });
      if (!r.ok) throw new Error(String(r.status));
      setPersonas(((await r.json()) as { people?: Persona[] }).people ?? []);
    } catch {
      setFalloLista(true);
    }
  }, [endpoint]);

  useEffect(() => {
    void cargar();
  }, [cargar]);

  const compartir = async () => {
    const email = correo.trim();
    if (!email || enviando) return;
    setEnviando(true);
    setErrorCorreo(null);
    try {
      const r = await fetch(endpoint, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email, permission: nivel }),
      });
      if (r.status === 404) {
        setErrorCorreo(t('sharePeople.unknown'));
        return;
      }
      if (!r.ok) throw new Error(String(r.status));
      const { person } = (await r.json()) as { person: Persona };
      setPersonas((ps) => [...(ps ?? []).filter((x) => x.subject !== person.subject), person]);
      setCorreo('');
      toast.notify({
        tone: 'success',
        title: t('sharePeople.added', { email: person.email ?? email }),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('sharePeople.failed') });
    } finally {
      setEnviando(false);
    }
  };

  const quitar = async (persona: Persona) => {
    setQuitando(persona.subject);
    try {
      const r = await fetch(`${endpoint}&sub=${encodeURIComponent(persona.subject)}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      // 404: ya no estaba (otra pestaña). En los dos casos, fuera de la lista.
      if (!r.ok && r.status !== 404) throw new Error(String(r.status));
      setPersonas((ps) => (ps ?? []).filter((x) => x.subject !== persona.subject));
      toast.notify({
        tone: 'success',
        title: t('sharePeople.removed', {
          name: persona.name ?? persona.email ?? t('sharePeople.someone'),
        }),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('sharePeople.removeFailed') });
    } finally {
      setQuitando(null);
    }
  };

  const nivelDe = (n: NivelCompartido) =>
    t(n === 'write' ? 'sharePeople.write' : 'sharePeople.read');

  return (
    <Modal
      open
      onClose={p.onClose}
      title={t('sharePeople.title', { name: p.nombre })}
      size="lg"
      footer={<Button onClick={p.onClose}>{t('sharePeople.done')}</Button>}
    >
      <fieldset class="min-w-0">
        <legend class="text-text mb-2 text-base font-semibold">{t('sharePeople.legend')}</legend>
        <div class="flex flex-wrap items-start gap-2">
          <FieldGroup
            label={t('sharePeople.email')}
            htmlFor="compartir-correo"
            class="min-w-56 flex-1"
          >
            <Input
              id="compartir-correo"
              type="email"
              inputMode="email"
              autocomplete="off"
              placeholder={t('sharePeople.placeholder')}
              value={correo}
              aria-describedby="compartir-correo-pista"
              onInput={(e) => {
                setCorreo(e.currentTarget.value);
                if (errorCorreo) setErrorCorreo(null);
              }}
              onKeyDown={(e: KeyboardEvent) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void compartir();
                }
              }}
            />
            <FieldHint id="compartir-correo-pista" tone={errorCorreo ? 'danger' : 'neutral'}>
              {errorCorreo ?? t('sharePeople.hint')}
            </FieldHint>
          </FieldGroup>
          {p.niveles.length > 1 ? (
            <FieldGroup label={t('sharePeople.access')} htmlFor="compartir-nivel" class="w-44">
              <Select
                id="compartir-nivel"
                value={nivel}
                onChange={(e) => setNivel(e.currentTarget.value === 'write' ? 'write' : 'read')}
              >
                {p.niveles.map((n) => (
                  <option key={n} value={n}>
                    {nivelDe(n)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          ) : null}
          <Button
            class="mt-6"
            loading={enviando}
            disabled={!correo.trim()}
            onClick={() => void compartir()}
          >
            {t('sharePeople.add')}
          </Button>
        </div>
      </fieldset>

      <h3 class="sytue-overline text-text-subtle mt-5 mb-2 font-semibold">
        {t('sharePeople.heading')}
      </h3>
      {personas === null && !falloLista ? (
        <div class="border-border grid h-16 place-items-center rounded-md border" aria-busy="true">
          <Spinner size={18} />
        </div>
      ) : falloLista ? (
        <div class="border-border bg-bg-subtle flex items-center justify-between gap-2 rounded-md border px-3 py-2">
          <p class="text-text-muted text-sm">{t('sharePeople.loadFailed')}</p>
          <Button variant="secondary" size="sm" onClick={() => void cargar()}>
            {t('action.retry')}
          </Button>
        </div>
      ) : personas && personas.length === 0 ? (
        <p class="text-text-muted text-sm">{t('sharePeople.empty')}</p>
      ) : (
        <ul class="divide-border/60 border-border divide-y rounded-md border">
          {(personas ?? []).map((x) => (
            <li key={x.subject} class="flex items-center gap-3 px-3 py-2">
              <Icon name="user" size={16} class="text-text-muted shrink-0" />
              <span class="min-w-0 flex-1">
                <span class="text-text block truncate font-medium">
                  {x.name ?? x.email ?? t('sharePeople.someone')}
                </span>
                <span class="sytue-meta block">
                  {nivelDe(x.permission)}
                  {x.name && x.email ? ` · ${x.email}` : ''}
                </span>
              </span>
              <Button
                variant="ghost"
                size="sm"
                class="text-danger-text"
                loading={quitando === x.subject}
                disabled={quitando !== null && quitando !== x.subject}
                onClick={() => void quitar(x)}
              >
                {t('sharePeople.remove')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
