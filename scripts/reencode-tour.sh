#!/usr/bin/env bash
# Re-encode the Playwright video tour at high quality.
# Playwright's built-in VP8 encoder uses 1Mbps bitrate which looks terrible.
# This re-encodes to H.264 at ~8Mbps CRF 18 for a sharp, readable result.

set -euo pipefail

INPUT="tests/e2e/test-results/video-tour-Video-Tour-Full-Application-Walkthrough-chromium/video.webm"
OUTPUT="reports/video-tour.mp4"

if [ ! -f "$INPUT" ]; then
  echo "ERROR: No video found at $INPUT"
  echo "Run the tour first: cd tests/e2e && npx playwright test video-tour.spec.ts"
  exit 1
fi

mkdir -p reports

ffmpeg -y -i "$INPUT" \
  -c:v libx264 -crf 18 -preset slow \
  -pix_fmt yuv420p \
  -movflags +faststart \
  "$OUTPUT"

echo "✓ Re-encoded to $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"
