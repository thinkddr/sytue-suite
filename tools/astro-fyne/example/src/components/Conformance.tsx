// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

import { useState } from "preact/hooks";

interface UnitProps {
  prefix: string;
  initial: number;
  observe: (prefix: string, previous: number) => void;
}

function StatefulUnit({ prefix, initial, observe }: UnitProps) {
  const seed = initial + 1;
  const [count, setCount] = useState(seed);
  const batch = () => {
    setCount(count + 1);
    setCount((previous) => previous + 10);
    setCount(count + 2);
    setCount((previous) => previous + 3);
    observe(prefix, count);
  };
  return (
    <section id={`${prefix}-unit`}>
      <p id={`${prefix}-value`}>{count}</p>
      <button
        id={`${prefix}-increment`}
        type="button"
        onClick={() => setCount((previous) => previous + 1)}
      >
        Increment
      </button>
      <button id={`${prefix}-batch`} type="button" onClick={batch}>
        Batch and observe previous closure
      </button>
    </section>
  );
}

interface PropertyProps {
  missing?: string;
  explicitNull: null;
}

function DestructuredProperties({ missing, explicitNull }: PropertyProps) {
  return (
    <section id="destructured-properties">
      <p id="destructured-missing-undefined">{String(missing === undefined)}</p>
      <p id="destructured-missing-null">{String(missing === null)}</p>
      <p id="destructured-null-undefined">
        {String(explicitNull === undefined)}
      </p>
      <p id="destructured-null-null">{String(explicitNull === null)}</p>
    </section>
  );
}

function ObjectProperties(props: PropertyProps) {
  return (
    <section id="object-properties">
      <p id="object-missing-undefined">{String(props.missing === undefined)}</p>
      <p id="object-missing-null">{String(props.missing === null)}</p>
      <p id="object-null-undefined">
        {String(props.explicitNull === undefined)}
      </p>
      <p id="object-null-null">{String(props.explicitNull === null)}</p>
    </section>
  );
}

export interface ConformanceProps {
  observe: UnitProps["observe"];
  t: (key: string) => string;
  disabledBranch: boolean;
  retainedText: string | null;
  retainedNumber: number | null;
  nullValue: null;
  missingValue?: string;
}

/**
 * A real TSX input to the compiler. Its generated Go is exercised by native tests,
 * so a parser-only pass cannot conceal an emitter or runtime semantic regression.
 */
export function Conformance({
  observe,
  t,
  disabledBranch,
  retainedText,
  retainedNumber,
  nullValue,
  missingValue,
}: ConformanceProps) {
  const [visible, setVisible] = useState(true);
  const skippedAnd = disabledBranch && t("unexpected-and");
  const skippedOr = retainedText || t("unexpected-or");
  const skippedNullish = retainedNumber ?? t("unexpected-nullish");
  const selectedNullish = nullValue ?? t("selected-null");
  const selectedMissing = missingValue ?? t("selected-missing");
  return (
    <main id="conformance">
      <StatefulUnit prefix="left" initial={2} observe={observe} />
      <StatefulUnit prefix="right" initial={10} observe={observe} />
      <button
        id="toggle-ephemeral"
        type="button"
        onClick={() => setVisible((previous) => !previous)}
      >
        Toggle ephemeral component
      </button>
      {visible && (
        <StatefulUnit prefix="ephemeral" initial={20} observe={observe} />
      )}
      <DestructuredProperties explicitNull={null} />
      <ObjectProperties explicitNull={null} />
      <p id="short-and">{String(skippedAnd)}</p>
      <p id="short-or">{String(skippedOr)}</p>
      <p id="short-nullish">{String(skippedNullish)}</p>
      <p id="selected-null">{selectedNullish}</p>
      <p id="selected-missing">{selectedMissing}</p>
      <p id="entity-conformance">A &amp; B &#x1F642; &copy;{" &amp;"}</p>
    </main>
  );
}
