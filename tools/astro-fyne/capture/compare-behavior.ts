// SPDX-License-Identifier: Apache-2.0
import { readFile } from "node:fs/promises";
import { isDeepStrictEqual } from "node:util";
import {
  behaviorScenarioHash,
  validateBehaviorScenario,
  validateBehaviorTrace,
} from "../src/behavior-contract.ts";
const [browserPath, nativePath, scenarioPath, sourceHash] =
  process.argv.slice(2);
if (!browserPath || !nativePath || !scenarioPath || !sourceHash)
  throw new Error(
    "Usage: compare-behavior.ts BROWSER.json NATIVE.json SCENARIO.json SOURCE_HASH",
  );
const scenario = validateBehaviorScenario(
  JSON.parse(await readFile(scenarioPath, "utf8")),
);
const identity = { sourceHash, scenarioHash: behaviorScenarioHash(scenario) };
const browser = validateBehaviorTrace(
  JSON.parse(await readFile(browserPath, "utf8")),
  scenario,
  identity,
);
const native = validateBehaviorTrace(
  JSON.parse(await readFile(nativePath, "utf8")),
  scenario,
  identity,
);
if (!isDeepStrictEqual(browser, native)) {
  process.stderr.write(
    `Browser trace:\n${JSON.stringify(browser, null, 2)}\nNative trace:\n${JSON.stringify(native, null, 2)}\n`,
  );
  throw new Error("Browser and generated native behavior differ");
}
process.stdout.write(
  JSON.stringify({ schema: 1, exact: true, frames: browser.frames.length }) +
    "\n",
);
