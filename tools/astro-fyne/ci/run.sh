#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

bun install --frozen-lockfile --ignore-scripts
bun run typecheck
bun test src
bunx --no-install prettier --check src capture example astro-fyne.json visual.json image-visual.json conformance.json updater-conformance.json conformance-scenario.json package.json tsconfig.json
bun src/cli.ts generate --config astro-fyne.json
bun src/cli.ts check --config astro-fyne.json
bun src/cli.ts generate --config conformance.json
bun src/cli.ts check --config conformance.json
bun src/cli.ts generate --config updater-conformance.json
bun src/cli.ts check --config updater-conformance.json
bun src/cli.ts analyze --config conformance.json > artifacts-conformance-analysis.json
export ASTRO_FYNE_CONFORMANCE_SOURCE_HASH="$(bun -e 'console.log((await Bun.file("artifacts-conformance-analysis.json").json()).sourceHash)')"
bun src/cli.ts analyze --config visual.json > artifacts-analysis.json
bun src/cli.ts analyze --config image-visual.json --entry ImageGeometry > artifacts-image-analysis.json
task_source_hash="$(bun -e 'console.log((await Bun.file("artifacts-analysis.json").json()).sourceHash)')"
task_image_source_hash="$(bun -e 'console.log((await Bun.file("artifacts-image-analysis.json").json()).sourceHash)')"
bunx --no-install astro build --root example
bunx --no-install playwright install --with-deps chromium
bunx --no-install astro preview --root example --host 127.0.0.1 --port 4321 > /tmp/astro-fyne-preview.log 2>&1 &
task_preview_pid=$!
trap 'kill "$task_preview_pid" 2>/dev/null || true' EXIT
task_preview_ready=false
for attempt in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:4321/geometry >/dev/null; then
    task_preview_ready=true
    break
  fi
  sleep 1
done
if [[ "$task_preview_ready" != true ]]; then
  cat /tmp/astro-fyne-preview.log >&2
  exit 1
fi
bun capture/astro-fyne-capture.ts --url http://127.0.0.1:4321/geometry --out artifacts --width 320 --height 240 --scale 1 --source-hash "$task_source_hash"
bun capture/behavior.ts http://127.0.0.1:4321/conformance conformance-scenario.json artifacts/web-behavior.json "$ASTRO_FYNE_CONFORMANCE_SOURCE_HASH"
bun src/cli.ts generate --config visual.json --entry Geometry --measurements artifacts/measurements.json
bun src/cli.ts check --config visual.json --entry Geometry --measurements artifacts/measurements.json
bun capture/astro-fyne-capture.ts --url http://127.0.0.1:4321/geometry-image --out artifacts/images --width 320 --height 240 --scale 1 --source-hash "$task_image_source_hash" --analysis artifacts-image-analysis.json
bun src/cli.ts generate --config image-visual.json --entry ImageGeometry --measurements artifacts/images/measurements.json
bun src/cli.ts check --config image-visual.json --entry ImageGeometry --measurements artifacts/images/measurements.json
git diff --exit-code HEAD -- native/generated
test -z "$(git ls-files --others --exclude-standard -- native/generated)"

export ASTRO_FYNE_ARTIFACTS="$(pwd)/artifacts"
export ASTRO_FYNE_IMAGE_ARTIFACTS="$(pwd)/artifacts/images"
cd native
go mod tidy
git diff --exit-code HEAD -- go.mod go.sum
go vet ./...
GOMAXPROCS=2 go test -p=2 -race ./...
go run ./visual/cmd/astro-fyne-compare --reference ../artifacts/web.png --native ../artifacts/native.png --out ../artifacts/diff.png | tee ../artifacts/comparison.json
go run ./visual/cmd/astro-fyne-compare --reference ../artifacts/images/web.png --native ../artifacts/images/native.png --out ../artifacts/images/diff.png | tee ../artifacts/images/comparison.json
bun ../capture/compare-behavior.ts ../artifacts/web-behavior.json ../artifacts/native-behavior.json ../conformance-scenario.json "$ASTRO_FYNE_CONFORMANCE_SOURCE_HASH" | tee ../artifacts/behavior-comparison.json
test -z "$(gofmt -l .)"
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
