#!/usr/bin/env bash
# [FORK] Layout engine comparison dashboard
# Usage: ./compare.sh [d2file] [port]
#   d2file: path to .d2 file (default: sample.d2)
#   port:   HTTP server port (default: 8765)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
D2_BIN="${D2_BIN:-/tmp/d2-fork}"
D2_FILE="${1:-$SCRIPT_DIR/sample.d2}"
PORT="${2:-8765}"
OUT_DIR="/tmp/d2-dashboard"
ENGINES=("dagre" "elk" "wueortho")

mkdir -p "$OUT_DIR"

if [ ! -f "$D2_FILE" ]; then
  echo "Error: $D2_FILE not found"
  exit 1
fi

cp "$D2_FILE" "$OUT_DIR/input.d2"
D2_SOURCE=$(cat "$D2_FILE")

echo "=== D2 Layout Comparison Dashboard ==="
echo "Input: $D2_FILE"
echo ""

# Render with each engine, collect metrics
METRICS_JSON="["
FIRST=true

for engine in "${ENGINES[@]}"; do
  echo -n "Rendering with $engine... "
  SVG_OUT="$OUT_DIR/$engine.svg"

  # Time the render
  START_NS=$(date +%s%N)
  if "$D2_BIN" --layout "$engine" "$D2_FILE" "$SVG_OUT" 2>/dev/null; then
    END_NS=$(date +%s%N)
    ELAPSED_MS=$(( (END_NS - START_NS) / 1000000 ))

    # Extract SVG dimensions from viewBox or width/height
    VIEWBOX=$(grep -oP 'viewBox="[^"]*"' "$SVG_OUT" | head -1 || true)
    SVG_WIDTH=$(grep -oP 'width="\K[0-9.]+' "$SVG_OUT" | head -1 || echo "?")
    SVG_HEIGHT=$(grep -oP 'height="\K[0-9.]+' "$SVG_OUT" | head -1 || echo "?")
    FILE_SIZE=$(stat -c%s "$SVG_OUT" 2>/dev/null || stat -f%z "$SVG_OUT" 2>/dev/null || echo "?")
    FILE_SIZE_KB=$(echo "scale=1; $FILE_SIZE / 1024" | bc 2>/dev/null || echo "?")

    # Count path elements (rough edge complexity)
    PATH_COUNT=$(grep -c '<path' "$SVG_OUT" || echo "0")

    echo "OK (${ELAPSED_MS}ms, ${SVG_WIDTH}x${SVG_HEIGHT})"

    if [ "$FIRST" = true ]; then FIRST=false; else METRICS_JSON+=","; fi
    METRICS_JSON+="{\"engine\":\"$engine\",\"ok\":true,\"ms\":$ELAPSED_MS,\"width\":$SVG_WIDTH,\"height\":$SVG_HEIGHT,\"fileKB\":$FILE_SIZE_KB,\"paths\":$PATH_COUNT}"
  else
    END_NS=$(date +%s%N)
    ELAPSED_MS=$(( (END_NS - START_NS) / 1000000 ))
    echo "FAILED (${ELAPSED_MS}ms)"

    if [ "$FIRST" = true ]; then FIRST=false; else METRICS_JSON+=","; fi
    METRICS_JSON+="{\"engine\":\"$engine\",\"ok\":false,\"ms\":$ELAPSED_MS}"
  fi
done

METRICS_JSON+="]"

# Escape D2 source for embedding in HTML
D2_SOURCE_ESCAPED=$(echo "$D2_SOURCE" | python3 -c "import sys,html; print(html.escape(sys.stdin.read()))")

# Generate HTML dashboard
cat > "$OUT_DIR/index.html" <<'HTMLEOF'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>D2 Layout Comparison</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: system-ui, -apple-system, sans-serif; background: #0d1117; color: #c9d1d9; }
  .header { padding: 16px 24px; background: #161b22; border-bottom: 1px solid #30363d; display: flex; align-items: center; gap: 16px; }
  .header h1 { font-size: 18px; font-weight: 600; }
  .header .src-toggle { background: #21262d; border: 1px solid #30363d; color: #c9d1d9; padding: 4px 12px; border-radius: 6px; cursor: pointer; font-size: 13px; }
  .header .src-toggle:hover { background: #30363d; }
  .source-panel { display: none; padding: 16px 24px; background: #161b22; border-bottom: 1px solid #30363d; }
  .source-panel.visible { display: block; }
  .source-panel pre { background: #0d1117; padding: 12px; border-radius: 6px; border: 1px solid #30363d; font-size: 13px; line-height: 1.5; overflow-x: auto; color: #e6edf3; }
  .grid { display: flex; gap: 0; flex-wrap: wrap; }
  .card { flex: 1; min-width: 33.33%; border-right: 1px solid #30363d; }
  .card:last-child { border-right: none; }
  .card-header { padding: 10px 16px; background: #161b22; border-bottom: 1px solid #30363d; display: flex; justify-content: space-between; align-items: center; }
  .card-header h2 { font-size: 15px; font-weight: 600; }
  .metrics { display: flex; gap: 12px; font-size: 12px; color: #8b949e; }
  .metrics .metric { display: flex; align-items: center; gap: 4px; }
  .metrics .value { color: #58a6ff; font-weight: 600; font-variant-numeric: tabular-nums; }
  .metrics .best { color: #3fb950; }
  .metrics .worst { color: #f85149; }
  .svg-container { padding: 8px; background: #fff; min-height: 200px; overflow: auto; display: flex; align-items: flex-start; justify-content: center; }
  .svg-container img { max-width: 100%; height: auto; }
  .svg-container.failed { display: flex; align-items: center; justify-content: center; color: #f85149; background: #161b22; }
  .controls { padding: 8px 16px; background: #161b22; border-top: 1px solid #30363d; display: flex; gap: 8px; align-items: center; font-size: 12px; color: #8b949e; }
  .controls input[type=range] { width: 100px; }
  .controls label { display: flex; align-items: center; gap: 4px; }
  .summary { padding: 12px 24px; background: #161b22; border-top: 1px solid #30363d; font-size: 13px; display: flex; gap: 24px; }
  .summary .item { display: flex; gap: 6px; }
  .summary .label { color: #8b949e; }
</style>
</head>
<body>

<div class="header">
  <h1>D2 Layout Comparison</h1>
  <button class="src-toggle" onclick="document.querySelector('.source-panel').classList.toggle('visible')">Show Source</button>
</div>

<div class="source-panel">
  <pre id="d2source"></pre>
</div>

<div class="grid" id="cards"></div>
<div class="summary" id="summary"></div>

<script>
const METRICS = /*METRICS_JSON*/;
const D2_SOURCE = /*D2_SOURCE_JSON*/;

document.getElementById('d2source').textContent = D2_SOURCE;

const grid = document.getElementById('cards');
const summary = document.getElementById('summary');

// Find best/worst for highlighting
const ok = METRICS.filter(m => m.ok);
const minMs = Math.min(...ok.map(m => m.ms));
const maxMs = Math.max(...ok.map(m => m.ms));
const areas = ok.map(m => m.width * m.height);
const minArea = Math.min(...areas);
const maxArea = Math.max(...areas);
const minFile = Math.min(...ok.map(m => m.fileKB));
const maxFile = Math.max(...ok.map(m => m.fileKB));

METRICS.forEach(m => {
  const card = document.createElement('div');
  card.className = 'card';

  const area = m.ok ? m.width * m.height : 0;
  const msClass = m.ms === minMs ? 'best' : m.ms === maxMs ? 'worst' : '';
  const areaClass = area === minArea ? 'best' : area === maxArea ? 'worst' : '';
  const fileClass = m.fileKB === minFile ? 'best' : m.fileKB === maxFile ? 'worst' : '';

  card.innerHTML = `
    <div class="card-header">
      <h2>${m.engine}</h2>
      ${m.ok ? `<div class="metrics">
        <div class="metric"><span>Time:</span><span class="value ${msClass}">${m.ms}ms</span></div>
        <div class="metric"><span>Size:</span><span class="value ${areaClass}">${m.width}×${m.height}</span></div>
        <div class="metric"><span>Area:</span><span class="value ${areaClass}">${(area/1000).toFixed(0)}k</span></div>
        <div class="metric"><span>File:</span><span class="value ${fileClass}">${m.fileKB}KB</span></div>
        <div class="metric"><span>Paths:</span><span class="value">${m.paths}</span></div>
      </div>` : '<span style="color:#f85149">FAILED</span>'}
    </div>
    <div class="svg-container${m.ok ? '' : ' failed'}">
      ${m.ok ? `<img src="${m.engine}.svg" alt="${m.engine}">` : 'Render failed'}
    </div>
  `;
  grid.appendChild(card);
});

// Summary
const best = ok.reduce((a, b) => (a.width * a.height < b.width * b.height ? a : b));
const fastest = ok.reduce((a, b) => (a.ms < b.ms ? a : b));
summary.innerHTML = `
  <div class="item"><span class="label">Most compact:</span><span style="color:#3fb950">${best.engine} (${best.width}×${best.height}, area ${(best.width*best.height/1000).toFixed(0)}k)</span></div>
  <div class="item"><span class="label">Fastest:</span><span style="color:#3fb950">${fastest.engine} (${fastest.ms}ms)</span></div>
  <div class="item"><span class="label">Engines:</span><span>${ok.length}/${METRICS.length} OK</span></div>
`;
</script>
</body>
</html>
HTMLEOF

# Inject metrics and source into HTML
python3 -c "
import json, sys

metrics = json.loads(sys.argv[1])
source = sys.argv[2]

with open('$OUT_DIR/index.html', 'r') as f:
    html = f.read()

html = html.replace('/*METRICS_JSON*/', json.dumps(metrics))
html = html.replace('/*D2_SOURCE_JSON*/', json.dumps(source))

with open('$OUT_DIR/index.html', 'w') as f:
    f.write(html)
" "$METRICS_JSON" "$D2_SOURCE"

echo ""
echo "Dashboard ready at: $OUT_DIR/index.html"
echo "Serving at: http://localhost:$PORT"
echo ""

# Kill existing server on this port if any
fuser -k "$PORT/tcp" 2>/dev/null || true
sleep 0.2

cd "$OUT_DIR"
exec python3 -m http.server "$PORT" --bind 127.0.0.1
