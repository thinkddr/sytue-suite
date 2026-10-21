// SPDX-License-Identifier: Apache-2.0
import { useEffect } from "preact/hooks";
import { Conformance } from "./Conformance";

interface BrowserTrace {
  observations: { prefix: string; previous: number }[];
  unexpectedCalls: string[];
}
declare global {
  interface Window {
    astroFyneBehaviorTrace?: BrowserTrace;
  }
}
const trace: BrowserTrace = { observations: [], unexpectedCalls: [] };

/** Browser-side test host. Only Conformance.tsx is input to the native compiler. */
export function BrowserConformance({ sourceHash }: { sourceHash: string }) {
  useEffect(() => {
    window.astroFyneBehaviorTrace = trace;
    document.documentElement.dataset.conformanceReady = "true";
    document.documentElement.dataset.conformanceSourceHash = sourceHash;
  }, []);
  return (
    <Conformance
      observe={(prefix, previous) =>
        trace.observations.push({ prefix, previous })
      }
      t={(key) => {
        if (key.startsWith("unexpected-")) trace.unexpectedCalls.push(key);
        return `translated:${key}`;
      }}
      disabledBranch={false}
      retainedText="kept"
      retainedNumber={0}
      nullValue={null}
    />
  );
}
