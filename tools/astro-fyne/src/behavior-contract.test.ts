// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

import { expect, test } from "bun:test";
import {
  behaviorScenarioHash,
  validateBehaviorScenario,
  validateBehaviorTrace,
  type BehaviorTrace,
} from "./behavior-contract.ts";

const scenario = {
  ids: ["value", "conditional"],
  actions: ["update", "update"],
};
const identity = {
  sourceHash: "a".repeat(64),
  scenarioHash: behaviorScenarioHash(scenario),
};

function validTrace(): BehaviorTrace {
  return {
    schema: 1,
    ...identity,
    frames: [
      { action: "initial", nodes: { value: "0", conditional: "<absent>" } },
      { action: "update", nodes: { value: "1", conditional: null } },
      { action: "update", nodes: { value: "2", conditional: "returned" } },
    ],
    observations: [{ prefix: "left", previous: 0 }],
    unexpectedCalls: [],
  };
}

test("a complete ordered source-bound trace preserves absence and literal text", () => {
  const trace = validateBehaviorTrace(validTrace(), scenario, identity);
  expect(trace.frames[0]!.nodes.conditional).toBe("<absent>");
  expect(trace.frames[1]!.nodes.conditional).toBeNull();
  expect(trace.frames.map((frame) => frame.action)).toEqual([
    "initial",
    "update",
    "update",
  ]);
});

test("empty or ambiguous scenarios cannot certify an empty run", () => {
  for (const invalid of [
    null,
    [],
    { ids: [], actions: ["update"] },
    { ids: ["value"], actions: [] },
    { ids: ["value", "value"], actions: ["update"] },
    { ids: ["value"], actions: [""] },
    { ids: ["value"], actions: ['invalid"selector'] },
    { ...scenario, extra: true },
  ])
    expect(() => validateBehaviorScenario(invalid)).toThrow();
  expect(() =>
    validateBehaviorTrace({ frames: [] }, scenario, identity),
  ).toThrow();
});

test("identity cannot be inferred from a pair of matching stale traces", () => {
  for (const invalid of [
    { ...validTrace(), sourceHash: "b".repeat(64) },
    { ...validTrace(), scenarioHash: "b".repeat(64) },
    { ...validTrace(), sourceHash: undefined },
    { ...validTrace(), sourceHash: "A".repeat(64) },
    { ...validTrace(), schema: 2 },
  ])
    expect(() => validateBehaviorTrace(invalid, scenario, identity)).toThrow();
  expect(() =>
    validateBehaviorTrace(validTrace(), scenario, {
      ...identity,
      scenarioHash: "b".repeat(64),
    }),
  ).toThrow();
  expect(
    behaviorScenarioHash({ ...scenario, ids: [...scenario.ids].reverse() }),
  ).not.toBe(identity.scenarioHash);
});

test("dropping or reordering frames or node columns always fails", () => {
  const original = validTrace();
  const mutations: unknown[] = [
    { ...original, frames: [] },
    { ...original, frames: original.frames.slice(0, -1) },
    { ...original, frames: [...original.frames, original.frames[2]] },
    { ...original, frames: [...original.frames].reverse() },
  ];
  for (const nodes of [
    { value: "0" },
    { value: "0", conditional: null, extra: "x" },
    { value: 0, conditional: null },
    { value: "0", conditional: undefined },
  ])
    mutations.push({
      ...original,
      frames: [{ action: "initial", nodes }, ...original.frames.slice(1)],
    });
  for (const invalid of mutations)
    expect(() => validateBehaviorTrace(invalid, scenario, identity)).toThrow();
});

test("observations are typed finite records and forbidden callbacks always fail", () => {
  for (const observations of [
    null,
    [{ prefix: "left", previous: Infinity }],
    [{ prefix: "left", previous: NaN }],
    [{ prefix: "left", previous: "0" }],
    [{ prefix: null, previous: 0 }],
    [{ prefix: "left", previous: 0, extra: "ignored" }],
  ])
    expect(() =>
      validateBehaviorTrace(
        { ...validTrace(), observations },
        scenario,
        identity,
      ),
    ).toThrow();
  for (const unexpectedCalls of [
    null,
    [4],
    ["unknown"],
    ["unexpected-and"],
    ["unexpected-or"],
    ["unexpected-nullish"],
  ])
    expect(() =>
      validateBehaviorTrace(
        { ...validTrace(), unexpectedCalls },
        scenario,
        identity,
      ),
    ).toThrow();
});

test("host trace fields cannot replace or add captured evidence", () => {
  expect(() =>
    validateBehaviorTrace({ ...validTrace(), extra: true }, scenario, identity),
  ).toThrow();
  expect(() =>
    validateBehaviorTrace({ ...validTrace(), frames: [] }, scenario, identity),
  ).toThrow();
});
