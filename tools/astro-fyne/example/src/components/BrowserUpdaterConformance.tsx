// SPDX-License-Identifier: Apache-2.0

import { useEffect } from "preact/hooks";
import { UpdaterConformance } from "./UpdaterConformance";

/** The browser runs the same TSX fixture compiled by the native behavioral tests. */
export function BrowserUpdaterConformance() {
  useEffect(() => {
    document.documentElement.dataset.updaterConformanceReady = "true";
  }, []);
  return <UpdaterConformance />;
}
