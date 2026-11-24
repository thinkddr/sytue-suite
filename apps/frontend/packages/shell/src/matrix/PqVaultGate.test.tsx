/* SPDX-License-Identifier: Apache-2.0 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import {
  PqVaultDisabledError,
  PqVaultError,
  pqVaultProfile,
  type PqVault,
} from '@sytue/offline/vault';
import { PqVaultGate } from './PqVaultGate.js';
import { MatrixStorageFixture } from './MatrixStorageFixture.js';
import {
  PERFIL_CIFRADO,
  estadoPuertaCifrado,
} from '../../../../../../test/e2e/tests/puerta-cifrado.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const actor = '11111111-1111-4111-8111-111111111111';

async function until(check: () => boolean): Promise<void> {
  for (let i = 0; i < 80; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('timeout waiting for condition');
}

function fakeVault(overrides: Partial<PqVault> = {}): PqVault & { closedCount: number } {
  let closed = false;
  const stub = {
    scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'notes' as const }),
    profile: pqVaultProfile,
    idOrigin: 'https://id.sytue.eu',
    vaultId: '!pq_testvault',
    keyId: 'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789',
    recipient: 'age1pq1testrecipient',
    closedCount: 0,
    get isClosed() {
      return closed;
    },
    close() {
      closed = true;
    },
    async closeAndDrain() {
      closed = true;
      stub.closedCount++;
    },
    ...overrides,
  };
  return stub as unknown as PqVault & { closedCount: number };
}

test('PqVaultGate prepares, activates, mounts children, and drains on unmount', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  const vault = fakeVault();
  const events: string[] = [];
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="es"
          product="notes"
          openVault={async () => vault}
          prepare={async (v) => {
            events.push('prepare:' + v.vaultId);
          }}
          activate={(v) => {
            events.push('activate:' + v.vaultId);
          }}
          detach={() => {
            events.push('detach');
          }}
          close={async () => {
            events.push('close');
          }}
        >
          {(v) => <p data-ready="true">{v.profile}</p>}
        </PqVaultGate>,
        root,
      );
    });
    await until(() => root.querySelector('[data-ready]') !== null);
    expect(root.querySelector('[data-ready]')?.textContent).toBe(pqVaultProfile);
    expect(events).toEqual(['prepare:!pq_testvault', 'activate:!pq_testvault']);
  } finally {
    await act(() => {
      render(null, root);
    });
    await until(() => vault.closedCount === 1);
    expect(events).toEqual(['prepare:!pq_testvault', 'activate:!pq_testvault', 'detach', 'close']);
    root.remove();
  }
});

test('PqVaultGate prompts sign-in on signin error and never mounts children', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="en"
          product="notes"
          openVault={async () => {
            throw new PqVaultError('signin');
          }}
        >
          <p data-secret="true">hidden</p>
        </PqVaultGate>,
        root,
      );
    });
    await until(() => root.querySelector('button') !== null);
    expect(root.querySelector('[data-secret]')).toBeNull();
    expect(root.textContent).toContain(pqVaultProfile);
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});

test('PqVaultGate mounts static children when encryption is disabled or inside MatrixStorageFixture', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  let openCalls = 0;
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="en"
          product="notes"
          openVault={async () => {
            throw new PqVaultDisabledError();
          }}
        >
          <p data-disabled="true">open</p>
        </PqVaultGate>,
        root,
      );
    });
    await until(() => root.querySelector('[data-disabled]') !== null);
    expect(root.querySelector('[data-disabled]')?.textContent).toBe('open');

    await act(() => {
      render(
        <MatrixStorageFixture>
          <PqVaultGate
            actor={actor}
            tenant="acme"
            locale="en"
            product="notes"
            openVault={async () => {
              openCalls++;
              return fakeVault();
            }}
          >
            <p data-fixture="true">bench</p>
          </PqVaultGate>
        </MatrixStorageFixture>,
        root,
      );
    });
    await until(() => root.querySelector('[data-fixture]') !== null);
    expect(openCalls).toBe(0);
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});

test('PqVaultGate on a wrong key offers to sign in again or empty, and empties only after confirming', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  let resets = 0;
  let expected: unknown;
  const findButton = (text: string): HTMLButtonElement | undefined =>
    [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === text);
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="es"
          product="notes"
          openVault={async () => {
            throw new PqVaultError('wrong', undefined, { vault_id: '!pqv_vista', version: 4 });
          }}
          resetVault={async (options) => {
            resets++;
            expected = options.expect;
            return fakeVault();
          }}
        >
          <p data-secret="true">datos</p>
        </PqVaultGate>,
        root,
      );
    });
    await until(() => findButton('Vaciar los datos cifrados') !== undefined);
    expect(root.querySelector('[data-secret]')).toBeNull();
    expect(findButton('Volver a entrar en Sytue ID')).toBeDefined();
    // Nada de «verifica desde otro dispositivo»: en ADR 0086 la clave vive en Sytue ID.
    expect(root.textContent).not.toContain('otro dispositivo');

    await act(() => {
      findButton('Vaciar los datos cifrados')!.click();
    });
    await until(() => document.querySelector('input[type="checkbox"]') !== null);
    const confirm = [...document.querySelectorAll('button')].filter(
      (b) => b.textContent?.trim() === 'Vaciar los datos cifrados',
    );
    const danger = confirm[confirm.length - 1]!;
    expect(danger.disabled).toBe(true);
    expect(resets).toBe(0);

    await act(() => {
      (document.querySelector('input[type="checkbox"]') as HTMLInputElement).click();
    });
    await until(() => !danger.disabled);
    await act(() => {
      danger.click();
    });
    await until(() => root.querySelector('[data-secret]') !== null);
    expect(resets).toBe(1);
    // Vacía solo la bóveda que vio: si alguien publicó después, el servidor no vacía.
    expect(expected).toEqual({ vault_id: '!pqv_vista', version: 4 });
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});

/** Confirma «Vaciar» en la puerta que esté pintada. */
async function confirmarVaciar(): Promise<void> {
  const boton = (text: string): HTMLButtonElement | undefined =>
    [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === text);
  await until(() => boton('Vaciar los datos cifrados') !== undefined);
  await act(() => {
    boton('Vaciar los datos cifrados')!.click();
  });
  await until(() => document.querySelector('input[type="checkbox"]') !== null);
  await act(() => {
    (document.querySelector('input[type="checkbox"]') as HTMLInputElement).click();
  });
  const confirm = [...document.querySelectorAll('button')].filter(
    (b) => b.textContent?.trim() === 'Vaciar los datos cifrados',
  );
  const danger = confirm[confirm.length - 1]!;
  await until(() => !danger.disabled);
  await act(() => {
    danger.click();
  });
}

const epoca = '0d3f5a1e-6b2c-4c1d-9e8f-7a6b5c4d3e2f';

test('Drive: vaciar suelta antes la fuente congelada, y la bóveda nueva puede importar', async () => {
  sessionStorage.clear();
  const root = document.createElement('div');
  document.body.appendChild(root);
  const pasos: string[] = [];
  let mode: 'frozen' | null = 'frozen';
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="es"
          product="drive"
          openVault={async () => {
            throw new PqVaultError('unavailable');
          }}
          resetVault={async () => {
            pasos.push('vaciar:' + (mode ?? 'legacy'));
            return fakeVault({
              scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'drive' }),
            } as Partial<PqVault>);
          }}
          sourceReleaser={{
            frozen: async () => (mode ? { mode, epoch: epoca } : null),
            release: async (epoch) => {
              pasos.push('soltar:' + epoch);
              mode = null;
              return 'released';
            },
          }}
          navigate={() => pasos.push('navegar')}
        >
          <p data-secret="true">datos</p>
        </PqVaultGate>,
        root,
      );
    });
    await confirmarVaciar();
    await until(() => root.querySelector('[data-secret]') !== null);
    // Primero se suelta la fuente; con ella ya en legacy, se vacía la bóveda.
    expect(pasos).toEqual(['soltar:' + epoca, 'vaciar:legacy']);
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});

test('Drive: si soltar pide identificarse, a la vuelta suelta esa época y vacía sin volver a preguntar', async () => {
  sessionStorage.clear();
  const pasos: string[] = [];
  let mode: 'frozen' | null = 'frozen';
  let reautenticado = false;
  const puerta = (root: HTMLElement) =>
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="drive"
        basePath="/drive"
        openVault={async () => {
          pasos.push('abrir');
          throw new PqVaultError('unavailable');
        }}
        resetVault={async () => {
          pasos.push('vaciar:' + (mode ?? 'legacy'));
          return fakeVault({
            scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'drive' }),
          } as Partial<PqVault>);
        }}
        sourceReleaser={{
          frozen: async () => (mode ? { mode, epoch: epoca } : null),
          release: async (epoch) => {
            if (!reautenticado) return 'reauth';
            pasos.push('soltar:' + epoch);
            mode = null;
            return 'released';
          },
        }}
        navigate={(url) => pasos.push('navegar:' + url)}
      >
        <p data-secret="true">datos</p>
      </PqVaultGate>,
      root,
    );
  const primera = document.createElement('div');
  document.body.appendChild(primera);
  try {
    await act(() => puerta(primera));
    await confirmarVaciar();
    await until(() => pasos.some((p) => p.startsWith('navegar:')));
    const url = pasos.find((p) => p.startsWith('navegar:'))!.slice('navegar:'.length);
    expect(url.startsWith('/drive/auth/login?')).toBe(true);
    expect(new URL(url, 'https://drive.test').searchParams.get('reauth')).toBe('1');
    expect(new URL(url, 'https://drive.test').searchParams.get('unlock')).toBe('1');
    expect(pasos.some((p) => p.startsWith('vaciar'))).toBe(false);
  } finally {
    await act(() => render(null, primera));
    primera.remove();
  }
  // De vuelta de Sytue ID, con la identificación reciente.
  reautenticado = true;
  pasos.length = 0;
  const vuelta = document.createElement('div');
  document.body.appendChild(vuelta);
  try {
    await act(() => puerta(vuelta));
    await until(() => vuelta.querySelector('[data-secret]') !== null);
    expect(pasos).toEqual(['soltar:' + epoca, 'vaciar:legacy']);
    // La marca se gasta: otra vuelta abre sin vaciar.
    expect(sessionStorage.getItem('sytue.vault-empty.drive')).toBeNull();
  } finally {
    await act(() => render(null, vuelta));
    vuelta.remove();
  }
});

test('vaciar que pide identificarse antes de tocar la bóveda vacía a la vuelta, una sola vez', async () => {
  sessionStorage.clear();
  const calendario = () =>
    fakeVault({
      scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'calendar' }),
    } as Partial<PqVault>);
  const pasos: string[] = [];
  let falta = true;
  const puerta = (root: HTMLElement) =>
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="calendar"
        openVault={async () => {
          pasos.push('abrir');
          if (falta) throw new PqVaultError('wrong', undefined, { vault_id: '!pqv_v', version: 1 });
          return calendario();
        }}
        resetVault={async () => {
          pasos.push('vaciar');
          // A este navegador le falta la clave de desbloqueo: no se ha vaciado nada.
          if (falta) throw new PqVaultError('signin');
          return calendario();
        }}
      >
        <p data-secret="true">datos</p>
      </PqVaultGate>,
      root,
    );
  const primera = document.createElement('div');
  document.body.appendChild(primera);
  try {
    await act(() => puerta(primera));
    await confirmarVaciar();
    await until(() =>
      [...document.querySelectorAll('button')].some((b) =>
        (b.textContent ?? '').includes('Iniciar sesión'),
      ),
    );
    expect(pasos).toEqual(['abrir', 'vaciar']);
  } finally {
    await act(() => render(null, primera));
    primera.remove();
  }
  falta = false;
  pasos.length = 0;
  const vuelta = document.createElement('div');
  document.body.appendChild(vuelta);
  try {
    await act(() => puerta(vuelta));
    await until(() => vuelta.querySelector('[data-secret]') !== null);
    // A la vuelta vacía directamente, sin abrir la bóveda de antes.
    expect(pasos).toEqual(['vaciar']);
  } finally {
    await act(() => render(null, vuelta));
    vuelta.remove();
  }
  // Una tercera apertura ya no vacía.
  pasos.length = 0;
  const otra = document.createElement('div');
  document.body.appendChild(otra);
  try {
    await act(() => puerta(otra));
    await until(() => otra.querySelector('[data-secret]') !== null);
    expect(pasos).toEqual(['abrir']);
  } finally {
    await act(() => render(null, otra));
    otra.remove();
  }
});

/*
 * El barrido de pantallas (`test/e2e/mobile-audit.mjs`) reconoce esta puerta con
 * `estadoPuertaCifrado` para volver a entrar cuando a su contexto le falta la clave
 * de desbloqueo. Sin eso medía la puerta como si fuera la app, y el `/edit?id=` de
 * Docus no se resolvía en ningún perfil salvo el primero (2026-11-19 a 23). Si la
 * puerta cambia de forma, esta prueba avisa antes de que el barrido vuelva a medir
 * el aviso en silencio.
 */
test('PqVaultGate with another device copying into the vault waits, retries or cancels that copy', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  let opens = 0,
    cancels = 0;
  let copying = true;
  const findButton = (text: string): HTMLButtonElement | undefined =>
    [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === text);
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="es"
          product="drive"
          openVault={async () => {
            opens++;
            return fakeVault({
              scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'drive' }),
            });
          }}
          prepare={async () => {
            if (copying) throw new PqVaultError('copy');
          }}
          cancelCopy={async (options) => {
            cancels++;
            expect(options.product).toBe('drive');
            copying = false;
          }}
        >
          <p data-secret="true">datos</p>
        </PqVaultGate>,
        root,
      );
    });
    await until(() => findButton('Cancelar la copia') !== undefined);
    expect(root.textContent).toContain(
      'Otro dispositivo está pasando tu Drive a la bóveda cifrada',
    );
    expect(root.querySelector('[data-secret]')).toBeNull();
    // Ni «vaciar» ni «volver a entrar»: no es un problema de clave.
    expect(findButton('Vaciar los datos cifrados')).toBeUndefined();
    await act(() => {
      findButton('Reintentar')!.click();
    });
    await until(() => opens === 2 && findButton('Cancelar la copia') !== undefined);
    expect(cancels).toBe(0);
    await act(() => {
      findButton('Cancelar la copia')!.click();
    });
    await until(() => root.querySelector('[data-secret]') !== null);
    expect(cancels).toBe(1);
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});

test('el barrido reconoce la puerta: abriendo, cerrada y abierta (también con sus detalles a la vista)', async () => {
  expect(PERFIL_CIFRADO).toBe(pqVaultProfile);
  const estado = (): string => estadoPuertaCifrado(PERFIL_CIFRADO);
  const root = document.createElement('div');
  document.body.appendChild(root);
  const montar = async (openVault: () => Promise<PqVault>): Promise<void> => {
    await act(() => {
      render(
        <PqVaultGate actor={actor} tenant="acme" locale="de" product="docs" openVault={openVault}>
          <p data-app="true">Recientes</p>
        </PqVaultGate>,
        root,
      );
    });
  };
  const desmontar = async (): Promise<void> => {
    await act(() => {
      render(null, root);
    });
  };
  try {
    // Preguntando al puente: todavía no se sabe si hará falta entrar.
    await montar(() => new Promise<PqVault>(() => {}));
    await until(() => root.querySelector('[role="status"]') !== null);
    expect(estado()).toBe('abriendo');
    await desmontar();

    // Sin la clave en este navegador (`signin`) o con una que no abre (`wrong`).
    for (const motivo of ['signin', 'wrong'] as const) {
      await montar(async () => {
        throw new PqVaultError(motivo);
      });
      await until(() => root.querySelector('button') !== null);
      expect(root.querySelector('[data-app]')).toBeNull();
      expect(estado()).toBe('cerrada');
      await desmontar();
    }

    // Abierta: está la app, y el perfil solo sale en el diálogo de detalles.
    await montar(async () =>
      fakeVault({ scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'docs' }) }),
    );
    await until(() => root.querySelector('[data-app]') !== null);
    expect(estado()).toBe('abierta');
    // El único botón de la puerta abierta es el candado de los detalles.
    await act(() => {
      root.querySelector('button')!.click();
    });
    await until(() => document.querySelector('[role="dialog"]') !== null);
    expect(document.querySelector('[role="dialog"]')!.textContent).toContain(pqVaultProfile);
    expect(estado()).toBe('abierta');
  } finally {
    await desmontar();
    root.remove();
  }
});
