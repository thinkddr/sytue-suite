// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

import { useState } from "preact/hooks";

function CrossHookUnit({ prefix }: { prefix: string }) {
  const [a, setA] = useState(0);
  const [b, setB] = useState(1);
  const update = () => {
    setB(2);
    setA((prev) => prev + b);
  };
  return (
    <section id={`${prefix}-unit`}>
      <p id={`${prefix}-a`}>{a}</p>
      <p id={`${prefix}-b`}>{b}</p>
      <button id={`${prefix}-update`} type="button" onClick={update}>
        Update with the other hook's render closure
      </button>
    </section>
  );
}

function SelfClosureUnit() {
  const [a, setA] = useState(0);
  const update = () => {
    setA(1);
    setA((prev) => prev + a);
  };
  return (
    <section id="self-unit">
      <p id="self-a">{a}</p>
      <button id="self-update" type="button" onClick={update}>
        Combine pending state with the render closure
      </button>
    </section>
  );
}

function SequentialUnit() {
  const [count, setCount] = useState(0);
  const update = () => {
    setCount((previous) => previous + 1);
    setCount((current) => current + 2);
  };
  return (
    <section id="sequential-unit">
      <p id="sequential-count">{count}</p>
      <button id="sequential-update" type="button" onClick={update}>
        Compose pending functional updates
      </button>
    </section>
  );
}

function ShadowedParameterUnit() {
  const [prev, setPrev] = useState(2);
  const [count, setCount] = useState(0);
  const update = () => {
    setPrev(9);
    setCount((prev) => prev + 1);
    setCount((current) => current + prev);
  };
  return (
    <section id="shadow-unit">
      <p id="shadow-prev">{prev}</p>
      <p id="shadow-count">{count}</p>
      <button id="shadow-update" type="button" onClick={update}>
        Keep updater parameters distinct from captured names
      </button>
    </section>
  );
}

function EventArgumentUnit() {
  const [count, setCount] = useState(0);
  return (
    <section id="event-unit">
      <p id="event-count">{count}</p>
      <input
        id="event-input"
        value=""
        onInput={(count) => setCount((previous) => previous + 1)}
      />
    </section>
  );
}

function MappedUnit() {
  const items = [10, 20];
  const [count, setCount] = useState(0);
  return (
    <section id="mapped-unit">
      <p id="mapped-count">{count}</p>
      {items.map((count, index) => (
        <button
          id={`mapped-${index}`}
          type="button"
          onClick={() => setCount((prev) => prev + count)}
        >
          Add captured row value
        </button>
      ))}
    </section>
  );
}

function NormalMappedUnit() {
  const items = [10, 20];
  const [total, setTotal] = useState(0);
  return (
    <section id="normal-mapped-unit">
      <p id="normal-mapped-total">{total}</p>
      {items.map((item, index) => (
        <button
          id={`normal-mapped-${index}`}
          type="button"
          onClick={() => setTotal((previous) => previous + item)}
        >
          Add row value to the owning component
        </button>
      ))}
    </section>
  );
}

/** One TSX source exercised by native behavioral tests and the browser host. */
export function UpdaterConformance() {
  return (
    <main id="updater-conformance">
      <CrossHookUnit prefix="left" />
      <CrossHookUnit prefix="right" />
      <SelfClosureUnit />
      <SequentialUnit />
      <ShadowedParameterUnit />
      <EventArgumentUnit />
      <MappedUnit />
      <NormalMappedUnit />
    </main>
  );
}
