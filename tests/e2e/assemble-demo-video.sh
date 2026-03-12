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

# Map caption sequence to audio clip
# demo-short.spec.ts generates cap-1, cap-2 ... in order
# gen-demo-audio.py generates 01-dashboard, 02-keyboard ... in order
# We need to align them by sequence number

audio_clips = [c for c in manifest if "error" not in c and c.get("duration_ms", 0) > 0]
caption_events = [t for t in timing]  # ordered by sequence

print(f"Captions: {len(caption_events)}, Audio clips: {len(audio_clips)}")

# Match: caption seq → audio clip (by index, skip title cards which have no caption)
# Audio clips correspond to caption() calls in order. Title cards and other events
# don't emit captions so there are more caption events than audio clips — or fewer.
# Use min(len) to be safe.

n = min(len(caption_events), len(audio_clips))
print(f"Pairing {n} clips")

# Get video duration
result = subprocess.run(
    ["ffprobe", "-v", "quiet", "-show_entries", "format=duration",
     "-of", "default=noprint_wrappers=1:nokey=1",
     # Find video
     *([f for f in [
         next((f for f in __import__("glob").glob("test-results/demo-short*/video.webm")), None)
     ] if f])],
    capture_output=True, text=True
)
video_duration_s = float(result.stdout.strip() or "240")

# Build silence-padded audio track using ffmpeg
# Strategy: for each clip, place it at (caption_start + 750ms) in the timeline.
# Gaps between clips are silence.

SAMPLE_RATE = 44100
PAD_MS = 750  # 750ms buffer before voice clip starts

# Find max timeline position
max_t_ms = 0
for i in range(n):
    ev = caption_events[i]
    clip = audio_clips[i]
    end_ms = ev["t"] + PAD_MS + clip["duration_ms"] + PAD_MS
    max_t_ms = max(max_t_ms, end_ms)

# Use ffmpeg to build audio: create silence file + mix all clips in
# Build filter_complex with amix
inputs = []
filter_parts = []
clips_data = []

for i in range(n):
    ev = caption_events[i]
    clip = audio_clips[i]
    start_ms = ev["t"] + PAD_MS
    start_s = start_ms / 1000.0
    if not os.path.exists(clip["path"]):
        print(f"  SKIP: {clip['path']} not found")
        continue
    clips_data.append((start_s, clip["path"], clip["duration_ms"]))
    print(f"  [{i+1:02d}] t={start_s:.1f}s  {clip['id']}  ({clip['duration_ms']}ms)")

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
