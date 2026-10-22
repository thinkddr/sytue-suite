// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

import { createHash } from "node:crypto";

export interface BehaviorScenario {
  ids: string[];
  actions: string[];
}

export interface BehaviorIdentity {
  sourceHash: string;
  scenarioHash: string;
}

export interface BehaviorFrame {
  action: string;
  // null means absent. Text such as "<absent>" remains distinguishable.
  nodes: Record<string, string | null>;
}

export interface BehaviorTrace extends BehaviorIdentity {
  schema: 1;
  frames: BehaviorFrame[];
  observations: { prefix: string; previous: number }[];
  unexpectedCalls: string[];
}

function record(value: unknown, description: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error(`${description} must be an object`);
  return value as Record<string, unknown>;
}

function exactKeys(
  value: Record<string, unknown>,
  expected: readonly string[],
  description: string,
) {
  const actual = Object.keys(value);
  if (
    actual.length !== expected.length ||
    expected.some((key) => !Object.hasOwn(value, key))
  )
    throw new Error(
      `${description} must contain exactly ${expected.join(", ")}`,
    );
}

function identifiers(value: unknown, description: string): string[] {
  if (!Array.isArray(value) || value.length === 0)
    throw new Error(`${description} must be a nonempty array`);
  return value.map((id: unknown) => {
    if (typeof id !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._:-]*$/.test(id))
      throw new Error(`${description} must contain explicit element IDs`);
    return id;
  });
}

export function validateBehaviorScenario(value: unknown): BehaviorScenario {
  const scenario = record(value, "Behavior scenario");
  exactKeys(scenario, ["ids", "actions"], "Behavior scenario");
  const ids = identifiers(scenario.ids, "Scenario IDs");
  if (new Set(ids).size !== ids.length)
    throw new Error("Scenario IDs must be unique");
  return { ids, actions: identifiers(scenario.actions, "Scenario actions") };
}

/** Canonical source-independent digest, preserving ID and event order. */
export function behaviorScenarioHash(value: BehaviorScenario): string {
  const scenario = validateBehaviorScenario(value);
  return createHash("sha256")
    .update(JSON.stringify({ ids: scenario.ids, actions: scenario.actions }))
    .digest("hex");
}

function hash(value: unknown, description: string): string {
  if (typeof value !== "string" || !/^[a-f0-9]{64}$/.test(value))
    throw new Error(`${description} must be a lowercase SHA-256 digest`);
  return value;
}

/**
 * A trace proves exactly one nonempty scenario for one analyzed source tree.
 * Identity must come from the caller's analysis/scenario, not either input trace.
 * This conformance host records forbidden short-circuit callbacks; even matching
 * browser/native calls to a forbidden branch cannot certify the corpus.
 */
export function validateBehaviorTrace(
  value: unknown,
  scenarioValue: BehaviorScenario,
  expected: BehaviorIdentity,
): BehaviorTrace {
  const scenario = validateBehaviorScenario(scenarioValue);
  const expectedSourceHash = hash(expected.sourceHash, "Expected source hash");
  const expectedScenarioHash = hash(
    expected.scenarioHash,
    "Expected scenario hash",
  );
  if (expectedScenarioHash !== behaviorScenarioHash(scenario))
    throw new Error("Expected scenario hash does not match the scenario");
  const trace = record(value, "Behavior trace");
  exactKeys(
    trace,
    [
      "schema",
      "sourceHash",
      "scenarioHash",
      "frames",
      "observations",
      "unexpectedCalls",
    ],
    "Behavior trace",
  );
  if (trace.schema !== 1) throw new Error("Behavior trace schema must be 1");
  if (hash(trace.sourceHash, "Trace source hash") !== expectedSourceHash)
    throw new Error("Behavior trace belongs to a different source tree");
  if (hash(trace.scenarioHash, "Trace scenario hash") !== expectedScenarioHash)
    throw new Error("Behavior trace belongs to a different scenario");
  const actions = ["initial", ...scenario.actions];
  if (!Array.isArray(trace.frames) || trace.frames.length !== actions.length)
    throw new Error(
      "Behavior frames must contain initial and every scenario action",
    );
  const frames = trace.frames.map((value: unknown, index: number) => {
    const frame = record(value, `Frame ${index}`);
    exactKeys(frame, ["action", "nodes"], `Frame ${index}`);
    if (frame.action !== actions[index])
      throw new Error(`Frame ${index} has a different or reordered action`);
    const nodes = record(frame.nodes, `Frame ${index} nodes`);
    exactKeys(nodes, scenario.ids, `Frame ${index} nodes`);
    const validated: Record<string, string | null> = Object.create(null);
    for (const id of scenario.ids) {
      const text = nodes[id];
      if (text !== null && typeof text !== "string")
        throw new Error(`Frame ${index} node ${id} must be text or null`);
      validated[id] = text;
    }
    return { action: actions[index]!, nodes: validated };
  });
  if (!Array.isArray(trace.observations))
    throw new Error("Behavior observations must be an array");
  const observations = trace.observations.map(
    (value: unknown, index: number) => {
      const observation = record(value, `Observation ${index}`);
      exactKeys(observation, ["prefix", "previous"], `Observation ${index}`);
      if (
        typeof observation.prefix !== "string" ||
        !observation.prefix ||
        typeof observation.previous !== "number" ||
        !Number.isFinite(observation.previous)
      )
        throw new Error(
          `Observation ${index} requires a prefix and finite previous value`,
        );
      return { prefix: observation.prefix, previous: observation.previous };
    },
  );
  if (!Array.isArray(trace.unexpectedCalls))
    throw new Error("Behavior unexpectedCalls must be an array");
  for (const call of trace.unexpectedCalls) {
    if (
      typeof call !== "string" ||
      !["unexpected-and", "unexpected-or", "unexpected-nullish"].includes(call)
    )
      throw new Error("Behavior trace contains an unknown unexpected callback");
  }
  if (trace.unexpectedCalls.length > 0)
    throw new Error(
      "Behavior trace evaluated a forbidden short-circuit callback",
    );
  return {
    schema: 1,
    sourceHash: expectedSourceHash,
    scenarioHash: expectedScenarioHash,
    frames,
    observations,
    unexpectedCalls: [],
  };
}
