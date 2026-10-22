/**
 * EnvImport — pegar un `.env` entero en vez de teclear veinte variables.
 *
 * Es la primera fricción real de cualquier proyecto: quien migra desde otro sitio
 * llega con un fichero de veinte líneas y el panel solo sabía darlas de alta de
 * una en una. El control-plane no necesita nada nuevo —`PUT
 * /projects/{id}/env` es por clave—, así que esto es el mismo endpoint en bucle
 * con lo que el formulario de una sola ya hacía.
 *
 * # Se enseña lo que se va a hacer ANTES de hacerlo
 *
 * El texto pegado se analiza en el navegador y se lista clave por clave, marcando
 * cuáles pisan una variable existente. Importar a ciegas y descubrir después que
 * has machacado la contraseña de producción no se arregla con un toast.
 *
 * # En serie, no en paralelo
 *
 * Cada alta pasa por la cuota del plan y por la auditoría del control-plane;
 * veinte peticiones a la vez leerían el mismo recuento. Son segundos, y el
 * progreso se ve.
 *
 * Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Badge,
  Icon,
  Textarea,
  Select,
  FieldGroup,
  Checkbox,
  InlineHelp,
  useToast,
} from '@sytue/ui';
import type { EnvScope as ApiEnvScope } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';

/** Misma regla que el BFF y que el alta de una sola variable. */
const KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

export interface ParsedVar {
  key: string;
  value: string;
}

/**
 * Analiza el contenido de un `.env`.
 *
 * Admite lo que traen los ficheros de verdad: comentarios, líneas en blanco,
 * `export ` delante, y valores entre comillas simples o dobles. Lo que NO hace es
 * expandir variables (`$OTRA`) ni interpretar escapes: el valor viaja tal cual al
 * control-plane, que es lo que la app va a recibir. Interpretarlo aquí sería
 * decidir por el usuario qué significa su fichero.
 */
export function parseDotenv(text: string): { vars: ParsedVar[]; skipped: number } {
  const vars: ParsedVar[] = [];
  let skipped = 0;
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const sinExport = line.startsWith('export ') ? line.slice('export '.length).trim() : line;
    const eq = sinExport.indexOf('=');
    if (eq <= 0) {
      skipped++;
      continue;
    }
    const key = sinExport.slice(0, eq).trim();
    let value = sinExport.slice(eq + 1).trim();
    if (
      (value.startsWith('"') && value.endsWith('"') && value.length >= 2) ||
      (value.startsWith("'") && value.endsWith("'") && value.length >= 2)
    ) {
      value = value.slice(1, -1);
    }
    if (!KEY_RE.test(key) || value === '') {
      skipped++;
      continue;
    }
    // La última repetida gana, como hace cualquier cargador de .env.
    const prev = vars.findIndex((v) => v.key === key);
    if (prev >= 0) vars.splice(prev, 1);
    vars.push({ key, value });
  }
  return { vars, skipped };
}

interface Props {
  project: string;
  t: T;
  /** Claves que YA existen: sirven para avisar de cuáles se van a pisar. */
  existing: string[];
  /** Se llama con las variables importadas para que la tabla las recoja. */
  onImported: (vars: ParsedVar[], secret: boolean, scope: ApiEnvScope) => void;
}

export default function EnvImport({ project, t, existing, onImported }: Props) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState('');
  const [secret, setSecret] = useState(true);
  const [scope, setScope] = useState<ApiEnvScope>('all');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(0);

  const { vars, skipped } = parseDotenv(text);
  const yaExisten = new Set(existing);
  const pisadas = vars.filter((v) => yaExisten.has(v.key)).length;

  async function importAll() {
    if (vars.length === 0) return;
    setBusy(true);
    setDone(0);
    let live = true;
    const hechas: ParsedVar[] = [];
    try {
      for (const v of vars) {
        const res = await postBff<{ ok: boolean; live: boolean }>('/api/env', {
          project,
          key: v.key,
          value: v.value,
          secret,
          scope,
        });
        live = live && res.live;
        hechas.push(v);
        setDone(hechas.length);
      }
      onImported(hechas, secret, scope);
      setText('');
      setOpen(false);
      notifyDemoIfNeeded(t, toast, live);
      toast.notify({ tone: 'success', title: t('env.import.done', { count: hechas.length }) });
    } catch {
      // Lo ya guardado SE QUEDA guardado: se dice cuántas entraron, en vez de
      // dejar al usuario sin saber por dónde se cortó.
      onImported(hechas, secret, scope);
      toast.notify({
        tone: 'danger',
        title: t('env.import.partial', { done: hechas.length, total: vars.length }),
      });
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <Button
        variant="secondary"
        leadingIcon={<Icon name="plus" size={16} />}
        onClick={() => setOpen(true)}
      >
        {t('env.import.open')}
      </Button>
    );
  }

  return (
    <Card class="mb-4 w-full">
      <CardHeader>
        <CardTitle as="h2">{t('env.import.title')}</CardTitle>
        <Button variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
          {t('common.cancel')}
        </Button>
      </CardHeader>
      <CardBody class="flex flex-col gap-3">
        <FieldGroup
          label={t('env.import.paste')}
          htmlFor="env-import-text"
          help={t('env.import.paste.help')}
        >
          <Textarea
            id="env-import-text"
            rows={8}
            value={text}
            class="font-mono text-sm"
            placeholder={'DATABASE_URL=postgres://…\nAPI_KEY="abc123"'}
            onInput={(e) => setText(e.currentTarget.value)}
          />
        </FieldGroup>

        <div class="flex flex-wrap items-end gap-3">
          <FieldGroup label={t('env.scopeField')} htmlFor="env-import-scope" class="w-52">
            <Select
              id="env-import-scope"
              value={scope}
              onChange={(e) => setScope(e.currentTarget.value as ApiEnvScope)}
            >
              <option value="all">{t('env.scopeField.all')}</option>
              <option value="production">{t('env.scopeField.production')}</option>
              <option value="preview">{t('env.scopeField.preview')}</option>
            </Select>
          </FieldGroup>
          <Checkbox
            class="mb-2"
            label={t('env.import.secret')}
            checked={secret}
            disabled={busy}
            onChange={(e) => setSecret(e.currentTarget.checked)}
          />
        </div>

        {/* Lo que va a pasar, antes de que pase. */}
        {vars.length > 0 ? (
          <div>
            <p class="text-text-muted mb-2 text-sm">
              {t('env.import.preview', { count: vars.length, skipped })}
            </p>
            <ul class="flex flex-wrap gap-1.5">
              {vars.map((v) => (
                <li key={v.key}>
                  <Badge tone={yaExisten.has(v.key) ? 'warning' : 'neutral'}>{v.key}</Badge>
                </li>
              ))}
            </ul>
            {pisadas > 0 ? (
              <InlineHelp tone="warning" class="mt-3">
                {t('env.import.overwrite', { count: pisadas })}
              </InlineHelp>
            ) : null}
          </div>
        ) : text.trim() ? (
          <InlineHelp tone="warning">{t('env.import.nothing')}</InlineHelp>
        ) : null}

        <div>
          <Button
            variant="primary"
            loading={busy}
            disabled={vars.length === 0 || busy}
            onClick={() => void importAll()}
          >
            {busy
              ? t('env.import.progress', { done, total: vars.length })
              : t('env.import.action', { count: vars.length })}
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}
