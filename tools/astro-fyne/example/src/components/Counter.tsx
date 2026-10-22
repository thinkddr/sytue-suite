// SPDX-License-Identifier: Apache-2.0
import { useState } from "preact/hooks";

export function Counter() {
  const [count, setCount] = useState(0);
  const [name, setName] = useState("Astro");
  return (
    <section id="counter" className="flex flex-col gap-2 p-4">
      <h1 id="title">Astro + Preact → Fyne</h1>
      <p id="count">{`Cuenta: ${count}`}</p>
      <button
        id="increment"
        type="button"
        onClick={() => setCount((current) => current + 1)}
      >
        Incrementar
      </button>
      <label id="name-label" for="name">
        Nombre
      </label>
      <input
        id="name"
        value={name}
        onInput={(event) => setName(event.currentTarget.value)}
      />
      <p id="greeting">{`Hola, ${name}`}</p>
      {count > 0 && (
        <p id="changed">El mismo estado funciona en las dos interfaces.</p>
      )}
    </section>
  );
}
