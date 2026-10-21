/**
 * Tests de los componentes de IA de Sytue Web: diseñar, traducir, exportar, el estado del
 * trabajo y los ajustes.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test, describe, beforeEach } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useState } from 'preact/hooks';
import type { VNode } from 'preact';
import { AjustesIA } from './AjustesIA.js';
import { DisenarConIA } from './DisenarConIA.js';
import { TraducirConIA } from './TraducirConIA.js';
import { ExportarConIA } from './ExportarConIA.js';
import { EstadoTrabajoIA } from './EstadoTrabajoIA.js';

if (typeof document === 'undefined') {
  GlobalRegistrator.register();
}

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

type Respuesta = { status?: number; body: unknown };
interface Llamada {
  url: string;
  metodo: string;
  cuerpo: unknown;
}

/** fetch simulado por ruta: la primera clave que esté contenida en la URL gana. */
function simular(rutas: Record<string, Respuesta | (() => Respuesta)>): Llamada[] {
  const llamadas: Llamada[] = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    llamadas.push({
      url,
      metodo: init?.method ?? 'GET',
      cuerpo: init?.body ? JSON.parse(String(init.body)) : undefined,
    });
    const clave = Object.keys(rutas).find((k) => url.includes(k));
    const r = clave ? rutas[clave] : { status: 404, body: {} };
    const { status = 200, body } = (typeof r === 'function' ? r() : r) ?? { status: 404, body: {} };
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  return llamadas;
}

async function esperar(condicion: () => boolean, ms = 3000): Promise<void> {
  const fin = Date.now() + ms;
  while (!condicion()) {
    if (Date.now() > fin) throw new Error('la condición no llegó a cumplirse');
    await new Promise((r) => setTimeout(r, 10));
  }
}

const CUPO = { limit: 50, used: 5, month: '2026-10', reset_at: '2026-11-01T00:00:00Z' };
const SIN_CLAVE = { has_key: false, provider: 'mistral', model: 'mistral-large-latest' };
const rutasBase = {
  '/api/ia/quota': { body: CUPO },
  '/api/ia/key': { body: SIN_CLAVE },
};
const trabajo = (extra: object) => ({
  id: 'job-1',
  tenant: 'acme',
  site_id: 's1',
  kind: 'design',
  provider: 'scaleway',
  model: 'mistral-large',
  created_at: '2026-10-03T10:00:00Z',
  ...extra,
});

function escribir(el: HTMLTextAreaElement | HTMLInputElement, valor: string) {
  el.value = valor;
  el.dispatchEvent(new Event('input', { bubbles: true }));
}
function enviar(host: HTMLElement) {
  host
    .querySelector('form')
    ?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
}
const boton = (host: HTMLElement, texto: string) =>
  [...host.querySelectorAll('button, a')].find((b) => b.textContent?.includes(texto)) as
    HTMLButtonElement | undefined;

describe('EstadoTrabajoIA', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  test('en curso: estado en texto, barra de progreso y tiempo', async () => {
    simular({ '/api/ia/jobs/': { body: trabajo({ status: 'running' }) } });
    const host = mount(<EstadoTrabajoIA jobId="job-1" titulo="Exportando" locale="es" />);
    expect(host.textContent).toContain('En cola');
    await esperar(() => !!host.textContent?.includes('En curso'));
    expect(host.querySelector('[role="progressbar"]')).not.toBeNull();
    expect(host.textContent).toContain('Tiempo transcurrido: 0:00');
    expect(host.textContent).toContain('Dentro de la UE');
  });

  test('hecho: avisa UNA vez aunque el padre se repinte con otra función', async () => {
    simular({ '/api/ia/jobs/': { body: trabajo({ status: 'done' }) } });
    let avisos = 0;
    function Padre() {
      const [n, setN] = useState(0);
      return (
        <div>
          <button type="button" id="repintar" onClick={() => setN(n + 1)}>
            {n}
          </button>
          <EstadoTrabajoIA
            jobId="job-1"
            titulo="T"
            locale="es"
            intervaloMs={5}
            onCompletado={() => {
              avisos += 1;
            }}
          />
        </div>
      );
    }
    const host = mount(<Padre />);
    await esperar(() => !!host.textContent?.includes('Hecho'));
    (host.querySelector('#repintar') as HTMLButtonElement).click();
    await new Promise((r) => setTimeout(r, 60));
    expect(avisos).toBe(1);
  });

  test('con error: alerta, detalle técnico y reintentar', async () => {
    simular({ '/api/ia/jobs/': { body: trabajo({ status: 'failed', error: 'boom' }) } });
    let reintentos = 0;
    const host = mount(
      <EstadoTrabajoIA
        jobId="job-1"
        titulo="T"
        locale="en"
        onReintentar={() => {
          reintentos += 1;
        }}
      />,
    );
    await esperar(() => !!host.querySelector('[role="alert"]'));
    expect(host.textContent).toContain('The job could not be completed.');
    expect(host.textContent).toContain('Technical detail: boom');
    boton(host, 'Retry')?.click();
    expect(reintentos).toBe(1);
  });

  test('un proveedor fuera del EEE se avisa con texto', async () => {
    simular({
      '/api/ia/jobs/': { body: trabajo({ status: 'running', provider: 'openai', byok: true }) },
    });
    const host = mount(<EstadoTrabajoIA jobId="job-1" titulo="T" locale="es" />);
    await esperar(() => !!host.textContent?.includes('Fuera del EEE'));
    expect(host.textContent).toContain('Clave propia');
  });

  test('tras varios cortes de red seguidos da el trabajo por perdido', async () => {
    simular({}); // todo 404 -> definitivo
    const host = mount(<EstadoTrabajoIA jobId="job-x" titulo="T" locale="es" intervaloMs={5} />);
    await esperar(() => !!host.textContent?.includes('Se ha perdido la conexión'));
    expect(host.querySelector('[role="alert"]')).not.toBeNull();
  });

  test('«dejar de esperar» solo aparece con onCancelar', async () => {
    simular({ '/api/ia/jobs/': { body: trabajo({ status: 'running' }) } });
    const sin = mount(<EstadoTrabajoIA jobId="job-1" titulo="T" locale="es" />);
    expect(boton(sin, 'Dejar de esperar')).toBeUndefined();
    const con = mount(
      <EstadoTrabajoIA jobId="job-1" titulo="T" locale="es" onCancelar={() => {}} />,
    );
    expect(boton(con, 'Dejar de esperar')).toBeDefined();
    expect(con.textContent).toContain('contará en tu cupo');
  });
});

describe('DisenarConIA', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  test('muestra cupo, aviso de la UE y los campos en el idioma pedido', async () => {
    simular(rutasBase);
    const host = mount(<DisenarConIA siteId="s1" locale="es" />);
    await esperar(() => !!host.querySelector('[role="progressbar"]'));
    expect(host.textContent).toContain('Describe tu negocio');
    expect(host.textContent).toContain('5 de 50 trabajos este mes');
    expect(host.textContent).toContain('dentro de la UE');
    expect(host.querySelector('textarea#ai-prompt')).not.toBeNull();
    expect(host.querySelector('select#ai-style')).not.toBeNull();
    const en = mount(<DisenarConIA siteId="s1" locale="en" />);
    expect(en.textContent).toContain('Describe your business');
    expect(en.textContent).not.toContain('Describe tu');
  });

  test('envía solo campos que la API acepta (el estilo va dentro del prompt) y enseña el resultado', async () => {
    const llamadas = simular({
      ...rutasBase,
      '/ia/design': { status: 202, body: { job: trabajo({ status: 'running' }) } },
      '/api/ia/jobs/': {
        body: trabajo({
          status: 'done',
          result: { sections: [{ type: 'hero' }, { type: 'faq' }] },
        }),
      },
    });
    let aplicado: unknown[] = [];
    const host = mount(
      <DisenarConIA siteId="s1" locale="es" idioma="en" onAplicar={(s) => (aplicado = s)} />,
    );
    await esperar(() => !!host.querySelector('[role="progressbar"]'));
    escribir(host.querySelector('textarea') as HTMLTextAreaElement, 'Panadería artesana');
    await esperar(() => !(boton(host, 'Generar propuesta') as HTMLButtonElement).disabled);
    enviar(host);
    await esperar(() => !!host.textContent?.includes('2 secciones listas'));

    const envio = llamadas.find((l) => l.metodo === 'POST');
    if (!envio) throw new Error('No se envió el trabajo de IA');
    expect(envio.url).toBe('/web/api/sites/s1/ia/design');
    expect(Object.keys(envio.cuerpo as object).sort()).toEqual(['language', 'prompt']);
    expect((envio.cuerpo as { prompt: string }).prompt).toContain('Visual style: modern and clean');
    expect((envio.cuerpo as { language: string }).language).toBe('en');

    boton(host, 'Añadir al lienzo')?.click();
    expect(aplicado).toHaveLength(2);
  });

  test('con el cupo agotado y sin clave propia no deja generar y dice cuándo se renueva', async () => {
    simular({
      '/api/ia/quota': { body: { ...CUPO, used: 50 } },
      '/api/ia/key': { body: SIN_CLAVE },
    });
    const host = mount(<DisenarConIA siteId="s1" locale="es" />);
    await esperar(() => !!host.querySelector('[role="alert"]'));
    expect(host.textContent).toContain('Has agotado el cupo mensual de IA');
    escribir(host.querySelector('textarea') as HTMLTextAreaElement, 'x');
    await new Promise((r) => setTimeout(r, 20));
    expect((boton(host, 'Generar propuesta') as HTMLButtonElement).disabled).toBe(true);
  });

  test('con clave propia fuera del EEE enseña el aviso RGPD y no gasta cupo', async () => {
    simular({
      ...rutasBase,
      '/api/ia/key': { body: { has_key: true, provider: 'openai', model: 'gpt-4o' } },
    });
    const host = mount(<DisenarConIA siteId="s1" locale="es" />);
    await esperar(() => !!host.textContent?.includes('sale del Espacio Económico Europeo'));
    expect(host.textContent).toContain('OpenAI (EE. UU.)');
    expect(host.textContent).toContain('no gasta cupo');
    expect(host.querySelector('[role="progressbar"]')).toBeNull();
  });

  test('un error de la API sale traducido por su código, no el texto en inglés', async () => {
    simular({
      ...rutasBase,
      '/ia/design': {
        status: 429,
        body: { error: { code: 'ai_busy', message: 'ai busy' } },
      },
    });
    const host = mount(<DisenarConIA siteId="s1" locale="es" />);
    await esperar(() => !!host.querySelector('[role="progressbar"]'));
    escribir(host.querySelector('textarea') as HTMLTextAreaElement, 'x');
    await new Promise((r) => setTimeout(r, 20));
    enviar(host);
    await esperar(() => !!host.textContent?.includes('Ya hay trabajos de IA en marcha'));
    expect(host.textContent).not.toContain('ai busy');
  });

  test('sin poder consultar el cupo, avisa y deja reintentar', async () => {
    simular({
      '/api/ia/quota': { status: 500, body: {} },
      '/api/ia/key': { status: 403, body: {} },
    });
    const host = mount(<DisenarConIA siteId="s1" locale="es" />);
    await esperar(() => !!host.textContent?.includes('No se pudo consultar el cupo de IA'));
    expect(boton(host, 'Reintentar')).toBeDefined();
  });
});

describe('quien no administra la organización', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  test('los paneles de IA consultan el cupo y no piden la vista de la clave', async () => {
    const llamadas = simular({
      ...rutasBase,
      '/api/ia/key': { status: 403, body: { error: { code: 'admin_required' } } },
    });
    const host = mount(<DisenarConIA siteId="s1" locale="es" administra={false} />);
    await esperar(() => llamadas.some((l) => l.url.includes('/api/ia/quota')));
    await esperar(() => !!host.querySelector('textarea, input'));
    expect(llamadas.filter((l) => l.url.includes('/api/ia/key'))).toEqual([]);
  });

  test('quien sí administra la sigue pidiendo', async () => {
    const llamadas = simular(rutasBase);
    mount(<DisenarConIA siteId="s1" locale="es" administra />);
    await esperar(() => llamadas.some((l) => l.url.includes('/api/ia/key')));
  });
});

describe('TraducirConIA', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  test('un fallo al guardar impide crear el trabajo y explica cómo volver a intentarlo', async () => {
    const llamadas = simular(rutasBase);
    let guardados = 0;
    const host = mount(
      <TraducirConIA
        siteId="s1"
        locale="es"
        prepararBorrador={async () => {
          guardados++;
          return false;
        }}
      />,
    );
    enviar(host);
    await esperar(() => !!host.querySelector('[role="alert"]'));
    expect(guardados).toBe(1);
    expect(llamadas.filter((l) => l.metodo === 'POST')).toHaveLength(0);
    expect(host.textContent).toContain('Resuelve el problema de guardado');
    expect((boton(host, 'Traducir') as HTMLButtonElement).disabled).toBe(false);
  });

  test('el idioma de origen no se puede elegir como destino', async () => {
    simular(rutasBase);
    const host = mount(<TraducirConIA siteId="s1" locale="es" />);
    expect(host.textContent).toContain('Traduce los textos del sitio');
    expect(host.querySelector('select#ai-trans-from')).not.toBeNull();
    expect(host.querySelector('select#ai-trans-scope')).not.toBeNull();
    expect(host.textContent).toContain('Inglés');
    const origen = host.querySelector('select#ai-trans-from') as HTMLSelectElement;
    origen.value = 'en';
    origen.dispatchEvent(new Event('change', { bubbles: true }));
    await esperar(() => (host.querySelector('#ai-trans-to-en') as HTMLInputElement).disabled);
    expect((host.querySelector('#ai-trans-to-en') as HTMLInputElement).checked).toBe(false);
    // Sin destino no se puede enviar.
    expect((boton(host, 'Traducir') as HTMLButtonElement).disabled).toBe(true);
  });

  test('no ofrece una página sin identificar ni un catálogo todavía no disponible', () => {
    simular(rutasBase);
    const host = mount(<TraducirConIA siteId="s1" locale="es" />);
    const scope = host.querySelector('#ai-trans-scope') as HTMLSelectElement;
    expect((scope.querySelector('option[value="page"]') as HTMLOptionElement).disabled).toBe(true);
    expect((scope.querySelector('option[value="store"]') as HTMLOptionElement).disabled).toBe(true);
    expect(scope.getAttribute('aria-describedby')).toBe('ai-trans-scope-help');
    expect(host.querySelector('#ai-trans-scope-help')?.textContent).toContain(
      'aún no está disponible',
    );
  });

  test('un sitio solo en inglés parte de ese origen y ofrece añadir español, sin idiomas incompatibles', async () => {
    const llamadas = simular({
      ...rutasBase,
      '/ia/translate': {
        status: 202,
        body: { job: trabajo({ status: 'running', kind: 'translate' }) },
      },
      '/api/ia/jobs/': {
        body: trabajo({ status: 'done', result: { 'home/title': { es: 'Inicio' } } }),
      },
    });
    let destinos: readonly string[] = [];
    const host = mount(
      <TraducirConIA
        siteId="s1"
        idioma="en"
        idiomas={['en']}
        locale="es"
        onAplicar={(_patch, _overwrite, langs) => {
          destinos = langs;
        }}
      />,
    );
    const from = host.querySelector('#ai-trans-from') as HTMLSelectElement;
    expect(from.value).toBe('en');
    expect([...from.options].map((option) => option.value)).toEqual(['en']);
    expect(host.querySelector('#ai-trans-to-fr')).toBeNull();
    expect((host.querySelector('#ai-trans-to-es') as HTMLInputElement).disabled).toBe(false);
    expect(host.textContent).toContain('los nuevos idiomas se añadirán al sitio');
    enviar(host);
    await esperar(() => !!host.textContent?.includes('1 texto traducido.'));
    expect(llamadas.find((l) => l.metodo === 'POST')?.cuerpo).toEqual({
      from: 'en',
      to: ['es'],
      scope: 'site',
      overwrite: false,
    });
    boton(host, 'Aplicar traducciones')?.click();
    expect(destinos).toEqual(['es']);
  });

  test('envía el id de la página actual y mantiene sobrescribir al aplicar el resultado', async () => {
    const llamadas = simular({
      ...rutasBase,
      '/ia/translate': {
        status: 202,
        body: { job: trabajo({ status: 'running', kind: 'translate' }) },
      },
      '/api/ia/jobs/': {
        body: trabajo({ status: 'done', result: { 'home/title': { en: 'Home' } } }),
      },
    });
    let overwrite: boolean | undefined;
    const host = mount(
      <TraducirConIA
        siteId="s1"
        pageId="home"
        locale="es"
        onAplicar={(_patch, value) => {
          overwrite = value;
        }}
      />,
    );
    const scope = host.querySelector('#ai-trans-scope') as HTMLSelectElement;
    expect((scope.querySelector('option[value="page"]') as HTMLOptionElement).disabled).toBe(false);
    await act(() => {
      scope.value = 'page';
      scope.dispatchEvent(new Event('change', { bubbles: true }));
      (host.querySelector('#ai-trans-overwrite') as HTMLInputElement).click();
    });
    enviar(host);
    await esperar(() => !!host.textContent?.includes('1 texto traducido.'));
    expect(llamadas.find((l) => l.metodo === 'POST')?.cuerpo).toEqual({
      from: 'es',
      to: ['en'],
      scope: 'page:home',
      overwrite: true,
    });
    boton(host, 'Aplicar traducciones')?.click();
    expect(overwrite).toBe(true);
  });

  test('envía origen y destinos y aplica el parche; sin textos traducidos lo dice', async () => {
    const llamadas = simular({
      ...rutasBase,
      '/ia/translate': {
        status: 202,
        body: { job: trabajo({ status: 'running', kind: 'translate' }) },
      },
      '/api/ia/jobs/': {
        body: trabajo({
          status: 'done',
          result: { 'a/content': { en: 'Hi' }, 'b/alt': { en: 'x' } },
        }),
      },
    });
    let parche: unknown = null;
    const host = mount(<TraducirConIA siteId="s1" locale="en" onAplicar={(p) => (parche = p)} />);
    enviar(host);
    await esperar(() => !!host.textContent?.includes('2 texts translated.'));
    expect(llamadas.find((l) => l.metodo === 'POST')?.cuerpo).toEqual({
      from: 'es',
      to: ['en'],
      scope: 'site',
      overwrite: false,
    });
    boton(host, 'Apply translations')?.click();
    expect(Object.keys(parche as object)).toHaveLength(2);
  });

  test('un resultado vacío no se presenta como éxito', async () => {
    simular({
      ...rutasBase,
      '/ia/translate': { status: 202, body: { job: trabajo({ status: 'running' }) } },
      '/api/ia/jobs/': { body: trabajo({ status: 'done', result: {} }) },
    });
    const host = mount(<TraducirConIA siteId="s1" locale="es" />);
    enviar(host);
    await esperar(() => !!host.textContent?.includes('no ha devuelto ninguna traducción'));
    expect(boton(host, 'Aplicar traducciones')).toBeUndefined();
  });
});

describe('ExportarConIA', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  test('solo Shopify está disponible; el resto, atenuado y «próximamente»', () => {
    simular(rutasBase);
    const host = mount(<ExportarConIA siteId="s1" locale="es" />);
    expect(host.textContent).toContain('Exportar con IA');
    expect(host.textContent).toContain('Shopify Online Store 2.0');
    expect(host.textContent).toContain('React + Tailwind CSS');
    expect(host.textContent).toContain('Próximamente');
    expect(host.textContent).not.toContain('Oleada');
    const radios = [...host.querySelectorAll('input[type="radio"]')] as HTMLInputElement[];
    expect(radios.map((r) => r.disabled)).toEqual([false, true, true, true]);
  });

  test('lee el id del trabajo de {job} y ofrece la descarga con las instrucciones', async () => {
    const llamadas = simular({
      ...rutasBase,
      '/api/exportar-ia/': {
        status: 202,
        body: { job: trabajo({ id: 'job-9', status: 'running' }) },
      },
      '/api/ia/jobs/': { body: trabajo({ id: 'job-9', status: 'done', kind: 'export' }) },
    });
    const host = mount(<ExportarConIA siteId="s1" locale="es" />);
    enviar(host);
    await esperar(() => !!host.textContent?.includes('Paquete listo'));
    expect(llamadas.find((l) => l.metodo === 'POST')?.cuerpo).toEqual({ destination: 'shopify' });
    const zip = boton(host, 'Descargar ZIP') as unknown as HTMLAnchorElement;
    expect(zip.getAttribute('href')).toContain('/api/ia/jobs/job-9/zip');
    expect(host.textContent).toContain('Subir archivo zip');
  });

  test('un miembro sin permisos ve un mensaje claro, no el código HTTP', async () => {
    simular({
      ...rutasBase,
      '/api/exportar-ia/': {
        status: 403,
        body: { error: { code: 'admin_required', message: 'admin' } },
      },
    });
    const host = mount(<ExportarConIA siteId="s1" locale="es" />);
    enviar(host);
    await esperar(() => !!host.querySelector('[role="alert"]'));
    expect(host.querySelector('[role="alert"]')?.textContent).toContain(
      'Solo quienes administran la organización pueden hacer esto.',
    );
    expect(host.textContent).not.toContain('admin_required');
  });
});

describe('Ajustes de IA', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  test('AjustesIA renderiza el panel de configuración tras cargar', async () => {
    globalThis.fetch = (async (url: string) => {
      if (url.includes('/key')) {
        return new Response(
          JSON.stringify({ has_key: false, provider: 'mistral', model: 'mistral-large-latest' }),
          {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          },
        );
      }
      if (url.includes('/quota')) {
        return new Response(
          JSON.stringify({
            limit: 50,
            used: 5,
            month: '2026-09',
            reset_at: '2026-10-01T00:00:00Z',
          }),
          {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          },
        );
      }
      return new Response(JSON.stringify({}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }) as unknown as typeof fetch;

    const host = mount(<AjustesIA locale="es" administra />);
    await esperar(() => (host.textContent ?? '').includes('Configurar clave propia (BYOK)'));

    expect(host.textContent).toContain('Configurar clave propia (BYOK)');
    expect(host.textContent).toContain('Proveedor de IA');
    expect(host.querySelector('select#ai-provider')).not.toBeNull();
    expect(host.querySelector('input#ai-key')).not.toBeNull();
  });

  test('AjustesIA sale en el idioma pedido y avisa si no puede cargar', async () => {
    globalThis.fetch = (async () => new Response('{}', { status: 500 })) as unknown as typeof fetch;
    const host = mount(<AjustesIA locale="en" administra />);
    await esperar(() => (host.textContent ?? '').includes('AI settings could not be loaded.'));
    expect(host.textContent).toContain('AI settings could not be loaded.');
    expect(host.textContent).not.toContain('No se pudieron');
  });

  test('un fallo parcial no inventa el cupo ni permite modificar la configuración', async () => {
    simular({
      ...rutasBase,
      '/api/ia/quota': { status: 503, body: { error: { code: 'unavailable' } } },
    });
    const host = mount(<AjustesIA locale="es" administra />);
    await esperar(() => !!host.textContent?.includes('No se pudieron cargar los ajustes de IA.'));
    expect(host.querySelector('form')).toBeNull();
    expect(host.textContent).not.toContain('0 / 50');
    expect(host.textContent).not.toContain('Sin clave propia configurada');
  });

  test('a quien no administra la organización se le explica, y NO se pide la clave ni el cupo', async () => {
    // El servicio contesta 403 a GET /ia/key si no eres `owner` ni `admin`. Pedirla para
    // enseñar el 403 dejaba un «No se pudieron cargar los ajustes» con un «Reintentar» que
    // fallaba siempre, y un 403 en la red de cada visita a los ajustes.
    const llamadas = simular({
      ...rutasBase,
      '/api/ia/key': { status: 403, body: { error: { code: 'admin_required' } } },
    });
    const host = mount(<AjustesIA locale="es" administra={false} />);
    await esperar(() => !!host.textContent?.includes('Solo quienes administran la organización'));
    await new Promise((r) => setTimeout(r, 30));
    expect(llamadas).toEqual([]);
    expect(host.textContent).toContain('Configurar clave propia (BYOK)');
    expect(host.textContent).not.toContain('No se pudieron cargar los ajustes de IA.');
    expect(host.querySelector('form')).toBeNull();
    expect(host.querySelector('button')).toBeNull();
  });

  test('sin permiso para leer la clave explica el requisito y no presenta un formulario inútil', async () => {
    simular({
      ...rutasBase,
      '/api/ia/key': { status: 403, body: { error: { code: 'admin_required' } } },
    });
    const host = mount(<AjustesIA locale="es" administra />);
    await esperar(() => !!host.textContent?.includes('Solo quienes administran la organización'));
    expect(host.querySelector('form')).toBeNull();
    expect(host.textContent).not.toContain('Sin clave propia configurada');
  });

  test('Probar clave usa el contrato estricto del endpoint, sin enviar model', async () => {
    const llamadas = simular({
      '/api/ia/key:check': { body: { ok: true } },
      ...rutasBase,
    });
    const host = mount(<AjustesIA locale="es" administra />);
    await esperar(() => !!host.querySelector('input#ai-key'));
    escribir(host.querySelector('input#ai-key') as HTMLInputElement, '  prueba-key  ');
    await esperar(() => !boton(host, 'Probar clave')?.disabled);
    boton(host, 'Probar clave')?.click();
    await esperar(() => !!host.textContent?.includes('Clave comprobada con éxito'));
    expect(llamadas.find((l) => l.url.includes('/key:check'))?.cuerpo).toEqual({
      provider: 'mistral',
      key: 'prueba-key',
    });
  });

  test('ZAI exige aceptar su propio aviso antes de guardar y traduce el rechazo de la clave', async () => {
    simular({
      '/api/ia/key:check': { status: 400, body: { error: { code: 'key_invalid' } } },
      ...rutasBase,
    });
    const host = mount(<AjustesIA locale="es" administra />);
    await esperar(() => !!host.querySelector('select#ai-provider'));
    const selector = host.querySelector('select#ai-provider') as HTMLSelectElement;
    selector.value = 'zai';
    selector.dispatchEvent(new Event('change', { bubbles: true }));
    await esperar(() => !!host.querySelector('input#notice-eu'));
    escribir(host.querySelector('input#ai-key') as HTMLInputElement, 'prueba-key');
    await esperar(() => !boton(host, 'Probar clave')?.disabled);
    expect(boton(host, 'Guardar y activar')?.disabled).toBe(true);
    boton(host, 'Probar clave')?.click();
    await esperar(() => !!host.textContent?.includes('El proveedor ha rechazado la clave.'));
    expect(host.textContent).not.toContain('key_invalid');
  });
});
