#!/usr/bin/env bash
# assemble-demo-video.sh — combines Playwright WebM video with generated voiceover audio
#
# Prerequisites:
#   1. Run `npx playwright test demo-short.spec.ts` to produce:
#        - tests/e2e/test-results/demo-short-.../video.webm
#        - tests/e2e/audio-clips/timing.json
#   2. Run `python gen-demo-audio.py` to produce:
#        - tests/e2e/audio-clips/manifest.json
#        - tests/e2e/audio-clips/*.wav
#
# Usage:
#   cd tests/e2e
#   bash assemble-demo-video.sh [--preview]
#   # Output: tests/e2e/demo-final.mp4

set -euo pipefail
cd "$(dirname "$0")"

AUDIO_DIR="audio-clips"
TIMING_JSON="${AUDIO_DIR}/timing.json"
MANIFEST_JSON="${AUDIO_DIR}/manifest.json"
CLIPS_DIR="${AUDIO_DIR}"

# Find the latest demo video
VIDEO_FILE=$(find test-results -name "video.webm" -path "*/demo-short*" | sort | tail -1)
if [[ -z "$VIDEO_FILE" ]]; then
  echo "ERROR: No demo-short video.webm found in test-results/"
  echo "Run: cd tests/e2e && npx playwright test demo-short.spec.ts"
  exit 1
fi
echo "Video: $VIDEO_FILE"

# Check timing + manifest
if [[ ! -f "$TIMING_JSON" ]]; then
  echo "ERROR: $TIMING_JSON not found. Run the Playwright demo test first."
  exit 1
fi
if [[ ! -f "$MANIFEST_JSON" ]]; then
  echo "ERROR: $MANIFEST_JSON not found. Run gen-demo-audio.py first."
  exit 1
fi

# Get video duration
VIDEO_DURATION=$(ffprobe -v quiet -show_entries format=duration \
  -of default=noprint_wrappers=1:nokey=1 "$VIDEO_FILE")
echo "Video duration: ${VIDEO_DURATION}s"

# Build audio track with Python (ffmpeg filter_complex gets complex with many inputs)
python3 - <<'PYEOF'
import json
import subprocess
import os
import wave
import struct
import sys

AUDIO_DIR = "audio-clips"
TIMING_JSON = f"{AUDIO_DIR}/timing.json"
MANIFEST_JSON = f"{AUDIO_DIR}/manifest.json"

# Load data
with open(TIMING_JSON) as f:
    timing = json.load(f)   # [{id, t, duration}]
with open(MANIFEST_JSON) as f:
    manifest = json.load(f)  # [{id, path, duration_ms}]

manifest_by_id = {c["id"]: c for c in manifest}

# Match caption events to audio clips by clipId (set in demo-short.spec.ts).
# Captions with no clipId (NIX entries, visual-only captions) produce silence.

print(f"Timing events: {len(timing)}, Audio clips in manifest: {len(manifest_by_id)}")

# Get video duration
result = subprocess.run(
    ["ffprobe", "-v", "quiet", "-show_entries", "format=duration",
     "-of", "default=noprint_wrappers=1:nokey=1",
     next(iter(__import__("glob").glob("test-results/demo-short*/video.webm")), "")],
    capture_output=True, text=True
)
video_duration_s = float(result.stdout.strip() or "240")

SAMPLE_RATE = 44100
PAD_MS = 750  # delay before voice clip starts within its caption window

clips_data = []
max_t_ms = 0

for ev in timing:
    clip_id = ev.get("clipId")
    if not clip_id:
        continue  # NIX or visual-only caption — silence
    clip = manifest_by_id.get(clip_id)
    if not clip or clip.get("duration_ms", 0) == 0:
        print(f"  SKIP: {clip_id} — not in manifest or zero duration")
        continue
    if not os.path.exists(clip["path"]):
        print(f"  SKIP: {clip['path']} not found")
        continue
    start_ms = ev["t"] + PAD_MS
    clips_data.append((start_ms / 1000.0, clip["path"], clip["duration_ms"]))
    end_ms = start_ms + clip["duration_ms"] + PAD_MS
    max_t_ms = max(max_t_ms, end_ms)
    print(f"  t={start_ms/1000:.1f}s  {clip_id}  ({clip['duration_ms']}ms)")

print(f"Paired {len(clips_data)} clips")

total_s = max(video_duration_s, max_t_ms / 1000.0 + 1.0)

# Write ffmpeg filter_complex script
# Silence base + adelay each clip + amix
silence_cmd = [
    "ffmpeg", "-y",
    "-f", "lavfi", "-i", f"anullsrc=r={SAMPLE_RATE}:cl=mono:d={total_s:.2f}",
]
for _, path, _ in clips_data:
    silence_cmd += ["-i", path]

filter_complex = []
# Resample each input clip to match
for i, (start_s, _, _) in enumerate(clips_data):
    delay_ms = int(start_s * 1000)
    filter_complex.append(
        f"[{i+1}:a]aresample={SAMPLE_RATE},adelay={delay_ms}|{delay_ms}[a{i}]"
    )

all_labels = "[0:a]" + "".join(f"[a{i}]" for i in range(len(clips_data)))
filter_complex.append(f"{all_labels}amix=inputs={len(clips_data)+1}:normalize=0[out]")

silence_cmd += [
    "-filter_complex", ";".join(filter_complex),
    "-map", "[out]",
    "-c:a", "pcm_s16le",
    f"{AUDIO_DIR}/voiceover.wav",
]

print("\nBuilding voiceover audio track...")
subprocess.run(silence_cmd, check=True, stderr=subprocess.DEVNULL)
print(f"Voiceover track written to {AUDIO_DIR}/voiceover.wav")
PYEOF

echo ""
echo "Combining video + voiceover..."

VIDEO_FILE=$(find test-results -name "video.webm" -path "*/demo-short*" | sort | tail -1)
ffmpeg -y \
  -i "$VIDEO_FILE" \
  -i "${AUDIO_DIR}/voiceover.wav" \
  -c:v copy -movflags +faststart \
  -c:a aac -b:a 128k \
  -shortest \
  demo-final.mp4

echo ""
echo "✓ Output: tests/e2e/demo-final.mp4"
du -h demo-final.mp4
echo ""

if [[ "${1:-}" == "--preview" ]]; then
  mpv demo-final.mp4
fi
