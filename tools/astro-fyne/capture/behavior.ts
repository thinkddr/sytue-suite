// SPDX-License-Identifier: Apache-2.0
import { chromium } from "playwright";
import { readFile, writeFile } from "node:fs/promises";
import {
  behaviorScenarioHash,
  validateBehaviorScenario,
  validateBehaviorTrace,
} from "../src/behavior-contract.ts";

const args = process.argv.slice(2);
if (args.length !== 4)
  throw new Error(
    "Usage: behavior.ts URL SCENARIO.json OUTPUT.json SOURCE_HASH",
  );
const [url, scenarioPath, outputPath, sourceHash] = args as [
  string,
  string,
  string,
  string,
];
const scenario = validateBehaviorScenario(
  JSON.parse(await readFile(scenarioPath, "utf8")),
);
const identity = { sourceHash, scenarioHash: behaviorScenarioHash(scenario) };
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const response = await page.goto(url);
  if (!response?.ok())
    throw new Error(`Browser scenario returned ${response?.status()}`);
  await page.locator('html[data-conformance-ready="true"]').waitFor();
  if (
    (await page
      .locator("html")
      .getAttribute("data-conformance-source-hash")) !== sourceHash
  )
    throw new Error("Browser host was built from another analyzed source tree");
  const frames: { action: string; nodes: Record<string, string | null> }[] = [];
  async function snapshot(action: string) {
    const nodes = await page.evaluate(
      (ids) =>
        Object.fromEntries(
          ids.map((id) => {
            const element = document.getElementById(id);
            return [id, element ? (element.textContent ?? "") : null];
          }),
        ),
      scenario.ids,
    );
    frames.push({ action, nodes });
  }
  await snapshot("initial");
  for (const action of scenario.actions) {
    await page.locator(`[id="${action}"]`).click();
    await page.evaluate(
      () => new Promise<void>((done) => requestAnimationFrame(() => done())),
    );
    await snapshot(action);
  }
  if (errors.length) throw new Error(errors.join("\n"));
  const trace = await page.evaluate(() => {
    const value = (
      window as unknown as {
        astroFyneBehaviorTrace?: {
          observations: unknown[];
          unexpectedCalls: string[];
        };
      }
    ).astroFyneBehaviorTrace;
    if (!value) throw new Error("Browser host trace is missing");
    return value;
  });
  await writeFile(
    outputPath,
    JSON.stringify(
      validateBehaviorTrace(
        {
          schema: 1,
          ...identity,
          frames,
          observations: trace.observations,
          unexpectedCalls: trace.unexpectedCalls,
        },
        scenario,
        identity,
      ),
      null,
      2,
    ) + "\n",
  );
} finally {
  await browser.close();
}
