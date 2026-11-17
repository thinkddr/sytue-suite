/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { JSX } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import { Button, Icon } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { correoDeDestino, enlazarTexto } from '../lib/enlaces.js';
import { separarCita } from './utils.js';

const PARRAFO = 'text-base leading-relaxed [overflow-wrap:anywhere] whitespace-pre-wrap';

/**
 * El cuerpo de texto de un mensaje del hilo, con el historial que arrastra plegado.
 *
 * En una conversación cada respuesta repite todas las anteriores, y leer el hilo era
 * leerlo entero tantas veces como mensajes tuviera. Como el «•••» de Gmail, lo nuevo
 * se ve y la cita espera tras un botón; desplegarla la pone DEBAJO del botón, que no
 * se mueve, para que plegarla otra vez no obligue a buscarlo. Un mensaje que es solo
 * cita no se pliega (lo decide `separarCita`).
 *
 * Solo para la rama de texto de `MessageBodyView`: en el HTML la cita va dentro del
 * iframe aislado, y plegarla allí es otra cosa.
 */
export function TextoConCita({
  t,
  texto,
  alEscribirA,
}: {
  t: T;
  texto: string;
  /** Escribe a la dirección de un `mailto:` desde Sytue (ver `TextoEnlazado`). */
  alEscribirA?: ((direccion: string) => void) | undefined;
}): JSX.Element {
  const { cuerpo, cita } = useMemo(() => separarCita(texto), [texto]);
  const [abierta, setAbierta] = useState(false);
  if (!cita)
    return (
      <p class={`text-text ${PARRAFO}`}>
        <TextoEnlazado texto={texto} alEscribirA={alEscribirA} />
      </p>
    );
  return (
    <div>
      <p class={`text-text ${PARRAFO}`}>
        <TextoEnlazado texto={cuerpo} alEscribirA={alEscribirA} />
      </p>
      <div class="mt-2">
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={abierta}
          onClick={() => setAbierta(!abierta)}
          trailingIcon={<Icon name={abierta ? 'chevronUp' : 'chevronDown'} />}
        >
          {t(abierta ? 'reader.hideQuoted' : 'reader.showQuoted')}
        </Button>
      </div>
      {abierta ? (
        <p class={`text-text-muted mt-2 ${PARRAFO}`}>
          <TextoEnlazado texto={cita} alEscribirA={alEscribirA} />
        </p>
      ) : null}
    </div>
  );
}

const ENLACE = 'text-primary-text sytue-focus-ring rounded-sm underline underline-offset-2';

/**
 * El texto plano de un correo con sus direcciones convertidas en enlaces
 * (`enlazarTexto`): una web, un «www.» o una dirección de correo en un mensaje de
 * texto se pulsaban y no hacían nada.
 *
 * Es una superficie de phishing, así que: solo `http(s)` y `mailto` (lo decide
 * `enlazarTexto`); la web sale en otra pestaña sin `opener` ni `Referer`, que no le
 * dice al remitente desde qué correo se abrió; y la pista (`title`) es el destino REAL,
 * con el `https://` que se le pone a un «www.», no el texto que escribió el remitente.
 *
 * Un `mailto:` se escribe desde Sytue con `alEscribirA` —que es donde está la cuenta—;
 * con una tecla modificadora, o sin `alEscribirA`, lo abre el navegador como siempre.
 */
export function TextoEnlazado({
  texto,
  alEscribirA,
}: {
  texto: string;
  alEscribirA?: ((direccion: string) => void) | undefined;
}): JSX.Element {
  const tramos = useMemo(() => enlazarTexto(texto), [texto]);
  return (
    <>
      {tramos.map((tr, i) => {
        if (!tr.destino) return tr.texto;
        const correo = correoDeDestino(tr.destino);
        if (correo !== null)
          return (
            <a
              key={i}
              href={tr.destino}
              title={correo}
              class={ENLACE}
              onClick={(e) => {
                if (!alEscribirA || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
                e.preventDefault();
                alEscribirA(correo);
              }}
            >
              {tr.texto}
            </a>
          );
        return (
          <a
            key={i}
            href={tr.destino}
            title={tr.destino}
            target="_blank"
            rel="noopener noreferrer"
            class={ENLACE}
          >
            {tr.texto}
          </a>
        );
      })}
    </>
  );
}
