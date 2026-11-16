/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Traer los datos del programa anterior: Holded, Odoo, Salesforce o Business
 * Central.
 *
 * El servicio ya sabía importar de los cuatro (`/importar/{proveedor}`), pero no
 * había pantalla: hacía falta la API y saber el id de la instancia de la pasarela.
 * Este asistente es esa pantalla, solo para quien administra la organización
 * (el servicio exige owner/admin en esas rutas).
 *
 * Las claves del tercero NUNCA pasan por aquí: viven selladas en el Integration
 * Gateway, que es quien habla con Holded y compañía. El ERP solo guarda qué
 * conexión de la pasarela usar; por eso el único dato que se pide es su
 * identificador, y la prueba sale con el token de quien está delante.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  DataFlow,
  FieldGroup,
  InlineHelp,
  Input,
  Modal,
  Wizard,
} from '@sytue/ui';
import type { DataCategory } from '@sytue/ui';
import { ErrorApi, api, mensajeDeError } from '../lib/api.js';
import type { ErpMessageKey, useT } from '../i18n/index.js';

type Traducir = ReturnType<typeof useT>;

export type ProveedorImportacion = 'holded' | 'odoo' | 'salesforce' | 'business-central';

interface Objeto {
  /** La clave del mapeo en el servicio (`conectores.Mapeo.Clave()`). */
  clave: string;
  rotulo: ErpMessageKey;
}

interface Proveedor {
  id: ProveedorImportacion;
  /** Marca: no se traduce. */
  nombre: string;
  entra: readonly DataCategory[];
  objetos: readonly Objeto[];
}

/**
 * Los objetos de cada proveedor, con las MISMAS claves que los mapeos de
 * `services/syerp/internal/conectores`. Una clave que no existe allí hace que el
 * servicio rechace la importación entera, así que el test las compara con los
 * ficheros Go.
 */
export const PROVEEDORES: readonly Proveedor[] = [
  {
    id: 'holded',
    nombre: 'Holded',
    entra: ['contacts', 'records', 'documents'],
    objetos: [
      { clave: 'contacts', rotulo: 'migrate.obj.contacts' },
      { clave: 'products', rotulo: 'migrate.obj.products' },
      { clave: 'invoices', rotulo: 'migrate.obj.salesInvoices' },
      { clave: 'purchases', rotulo: 'migrate.obj.purchaseInvoices' },
    ],
  },
  {
    id: 'odoo',
    nombre: 'Odoo',
    entra: ['contacts', 'records', 'documents'],
    objetos: [
      { clave: 'res.partner:empresas', rotulo: 'migrate.obj.companies' },
      { clave: 'res.partner:personas', rotulo: 'migrate.obj.people' },
      { clave: 'product.product', rotulo: 'migrate.obj.products' },
      { clave: 'account.move:ventas', rotulo: 'migrate.obj.salesInvoices' },
      { clave: 'crm.lead', rotulo: 'migrate.obj.opportunities' },
    ],
  },
  {
    id: 'salesforce',
    nombre: 'Salesforce',
    entra: ['contacts', 'records'],
    objetos: [
      { clave: 'Account', rotulo: 'migrate.obj.companies' },
      { clave: 'Contact', rotulo: 'migrate.obj.people' },
      { clave: 'Opportunity', rotulo: 'migrate.obj.opportunities' },
    ],
  },
  {
    id: 'business-central',
    nombre: 'Business Central',
    entra: ['contacts', 'records', 'documents'],
    objetos: [
      { clave: 'customers:empresas', rotulo: 'migrate.obj.companies' },
      { clave: 'customers:personas', rotulo: 'migrate.obj.people' },
      { clave: 'vendors', rotulo: 'migrate.obj.suppliers' },
      { clave: 'items:bienes', rotulo: 'migrate.obj.products' },
      { clave: 'items:servicios', rotulo: 'migrate.obj.services' },
      { clave: 'salesInvoices', rotulo: 'migrate.obj.salesInvoices' },
      { clave: 'purchaseInvoices', rotulo: 'migrate.obj.purchaseInvoices' },
    ],
  },
];

/** Lo mismo que valida la pasarela (`uuid.Parse`): mejor decirlo antes de probar. */
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

interface Resultado {
  objeto: string;
  modelo: string;
  creados: number;
  actualizados: number;
  fallos?: string[];
}

export function AsistenteImportarDesde({
  onCerrar,
  onImportado,
  t,
}: {
  onCerrar: () => void;
  /** Tras importar: la lista abierta y los contadores se vuelven a pedir. */
  onImportado: () => void;
  t: Traducir;
}): JSX.Element {
  const [paso, setPaso] = useState(0);
  const [proveedor, setProveedor] = useState<Proveedor | null>(null);
  const [guardadas, setGuardadas] = useState<Record<string, string>>({});
  const [instancia, setInstancia] = useState('');
  const [probada, setProbada] = useState(false);
  const [probando, setProbando] = useState(false);
  const [errorConexion, setErrorConexion] = useState('');
  const [elegidos, setElegidos] = useState<Set<string>>(new Set());
  const [ocupado, setOcupado] = useState(false);
  const [error, setError] = useState('');
  const [resultado, setResultado] = useState<Resultado[] | null>(null);

  // Las conexiones ya guardadas rellenan el identificador: volver a importar
  // no obliga a buscarlo otra vez en la consola.
  useEffect(() => {
    let vivo = true;
    api<{ conexiones: { proveedor: string; instancia?: string }[] | null }>('conexiones')
      .then((r) => {
        if (!vivo) return;
        const mapa: Record<string, string> = {};
        for (const c of r.conexiones ?? []) if (c.instancia) mapa[c.proveedor] = c.instancia;
        setGuardadas(mapa);
      })
      .catch(() => undefined);
    return () => {
      vivo = false;
    };
  }, []);

  // Si la lista llega después de elegir el programa, rellena lo que siga vacío.
  useEffect(() => {
    if (proveedor && !instancia && guardadas[proveedor.id]) setInstancia(guardadas[proveedor.id]!);
    // `instancia` fuera a propósito: borrar el campo a mano no lo vuelve a rellenar.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [guardadas, proveedor]);

  function elegir(p: Proveedor): void {
    setProveedor(p);
    setInstancia(guardadas[p.id] ?? '');
    setProbada(false);
    setErrorConexion('');
    setElegidos(new Set(p.objetos.map((o) => o.clave)));
  }

  const instanciaValida = UUID.test(instancia.trim());

  async function probar(): Promise<void> {
    if (!proveedor) return;
    if (!instanciaValida) {
      setErrorConexion(t('migrate.invalidId'));
      return;
    }
    setProbando(true);
    setErrorConexion('');
    try {
      await api(`conexiones/${proveedor.id}/probar`, {
        method: 'POST',
        body: JSON.stringify({ instancia: instancia.trim() }),
      });
      setProbada(true);
    } catch (err) {
      setProbada(false);
      setErrorConexion(
        err instanceof ErrorApi && err.estado === 422
          ? t('migrate.testFailed')
          : mensajeDeError(err, t),
      );
    } finally {
      setProbando(false);
    }
  }

  /** Se guarda solo lo que ya ha conectado: nunca una instancia que no responde. */
  async function guardarConexion(): Promise<void> {
    if (!proveedor) return;
    setOcupado(true);
    setError('');
    try {
      await api(`conexiones/${proveedor.id}`, {
        method: 'POST',
        body: JSON.stringify({ instancia: instancia.trim(), activo: true }),
      });
      setGuardadas((g) => ({ ...g, [proveedor.id]: instancia.trim() }));
      setPaso(2);
    } catch (err) {
      setError(mensajeDeError(err, t));
    } finally {
      setOcupado(false);
    }
  }

  async function importar(): Promise<void> {
    if (!proveedor) return;
    setOcupado(true);
    setError('');
    try {
      const r = await api<{ importado: Resultado[] | null }>(`importar/${proveedor.id}`, {
        method: 'POST',
        body: JSON.stringify({
          objetos: proveedor.objetos.map((o) => o.clave).filter((c) => elegidos.has(c)),
        }),
      });
      setResultado(r.importado ?? []);
      onImportado();
      setPaso(3);
    } catch (err) {
      setError(mensajeDeError(err, t));
    } finally {
      setOcupado(false);
    }
  }

  const rotuloDe = (clave: string): string => {
    const o = proveedor?.objetos.find((x) => x.clave === clave);
    return o ? t(o.rotulo) : clave;
  };
  const nombre = proveedor?.nombre ?? '';

  return (
    <Modal
      open
      onClose={onCerrar}
      dismissable={!ocupado && !probando}
      title={t('migrate.title')}
      size="md"
    >
      {error ? (
        <InlineHelp tone="danger" class="mb-4">
          {error}
        </InlineHelp>
      ) : null}
      <Wizard
        current={paso}
        busy={ocupado}
        {...(resultado ? {} : { onCancel: onCerrar })}
        onCurrentChange={(n) => {
          if (!resultado) {
            setError('');
            setPaso(n);
          }
        }}
        steps={[
          {
            id: 'programa',
            title: t('migrate.stepSource'),
            heading: t('migrate.sourceHeading'),
            canContinue: proveedor !== null,
            content: (
              <div class="space-y-3">
                <div
                  role="radiogroup"
                  aria-label={t('migrate.sourceHeading')}
                  class="grid grid-cols-1 gap-2 sm:grid-cols-2"
                >
                  {PROVEEDORES.map((p) => {
                    const activo = proveedor?.id === p.id;
                    return (
                      <button
                        key={p.id}
                        type="button"
                        role="radio"
                        aria-checked={activo}
                        onClick={() => elegir(p)}
                        class={
                          'sytue-focus-ring rounded-control min-h-[44px] border px-3 py-2.5 text-left text-sm font-semibold transition-colors ' +
                          (activo
                            ? 'border-primary bg-selected text-text'
                            : 'border-border text-text hover:bg-bg-subtle')
                        }
                      >
                        {p.nombre}
                      </button>
                    );
                  })}
                </div>
                {proveedor ? (
                  <DataFlow partner={proveedor.nombre} incoming={proveedor.entra} />
                ) : null}
              </div>
            ),
          },
          {
            id: 'conexion',
            title: t('migrate.stepConnect'),
            heading: t('migrate.connectHeading', { name: nombre }),
            description: t('migrate.connectHint', { name: nombre }),
            canContinue: probada,
            onNext: guardarConexion,
            content: (
              <div class="space-y-3">
                <FieldGroup
                  label={t('migrate.instanceLabel')}
                  htmlFor="erp-importar-instancia"
                  {...(errorConexion ? { error: errorConexion } : {})}
                >
                  <Input
                    id="erp-importar-instancia"
                    autocomplete="off"
                    spellcheck={false}
                    value={instancia}
                    invalid={Boolean(errorConexion)}
                    disabled={probando || ocupado}
                    onInput={(e) => {
                      setInstancia(e.currentTarget.value);
                      setProbada(false);
                      setErrorConexion('');
                    }}
                  />
                </FieldGroup>
                <div class="flex flex-wrap items-center gap-3">
                  <Button
                    variant="secondary"
                    disabled={instancia.trim() === '' || probando || ocupado}
                    onClick={() => void probar()}
                  >
                    {t('migrate.test')}
                  </Button>
                  {probada ? (
                    <p role="status" class="text-success-text text-sm">
                      {t('migrate.testOk', { name: nombre })}
                    </p>
                  ) : null}
                </div>
              </div>
            ),
          },
          {
            id: 'que',
            title: t('migrate.stepWhat'),
            heading: t('migrate.whatHeading'),
            description: t('migrate.whatHint'),
            nextLabel: t('migrate.run'),
            canContinue: elegidos.size > 0,
            onNext: importar,
            content: (
              <fieldset class="space-y-2">
                <legend class="sr-only">{t('migrate.whatHeading')}</legend>
                {(proveedor?.objetos ?? []).map((o) => (
                  <Checkbox
                    key={o.clave}
                    label={t(o.rotulo)}
                    checked={elegidos.has(o.clave)}
                    disabled={ocupado}
                    onChange={(e) => {
                      const marcado = e.currentTarget.checked;
                      setElegidos((s) => {
                        const n = new Set(s);
                        if (marcado) n.add(o.clave);
                        else n.delete(o.clave);
                        return n;
                      });
                    }}
                  />
                ))}
              </fieldset>
            ),
          },
          {
            id: 'listo',
            title: t('migrate.stepDone'),
            heading: t('migrate.doneHeading'),
            done: true,
            content: (
              <div class="space-y-4">
                <table class="w-full text-sm">
                  <thead>
                    <tr class="text-text-muted border-border border-b text-left text-xs">
                      <th scope="col" class="py-1.5 font-medium">
                        {t('migrate.stepWhat')}
                      </th>
                      <th scope="col" class="py-1.5 text-right font-medium">
                        {t('migrate.created')}
                      </th>
                      <th scope="col" class="py-1.5 text-right font-medium">
                        {t('migrate.updated')}
                      </th>
                      <th scope="col" class="py-1.5 text-right font-medium">
                        {t('migrate.failed')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {(resultado ?? []).map((r) => (
                      <tr key={r.objeto} class="border-border-subtle border-b">
                        <th scope="row" class="text-text py-1.5 text-left font-normal">
                          {rotuloDe(r.objeto)}
                        </th>
                        <td class="py-1.5 text-right tabular-nums">{r.creados}</td>
                        <td class="py-1.5 text-right tabular-nums">{r.actualizados}</td>
                        <td
                          class={
                            'py-1.5 text-right tabular-nums ' +
                            ((r.fallos?.length ?? 0) > 0 ? 'text-danger-text' : '')
                          }
                        >
                          {r.fallos?.length ?? 0}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <Button onClick={onCerrar}>{t('action.close')}</Button>
              </div>
            ),
          },
        ]}
      />
    </Modal>
  );
}
