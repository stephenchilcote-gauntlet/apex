#!/usr/bin/env python3
"""
Generate spoken audio clips for the Apex MCD demo video.

Uses Fish Speech S1-mini with rhobotmeat.wav as the reference voice.
Reads narration script from demo-script.json (edit that file to change what the voice says).
Outputs WAV files + a JSON manifest with durations.

Skips clips that already exist AND whose text hasn't changed.
Automatically regenerates clips when the spoken text is edited in demo-script.json.

Usage:
    cd /home/login/PycharmProjects/chat_reader_zonos
    source .venv/bin/activate
    python /home/login/PycharmProjects/gauntlet/apex/tests/e2e/gen-demo-audio.py

    # Force-regenerate all clips even if unchanged:
    python /home/login/PycharmProjects/gauntlet/apex/tests/e2e/gen-demo-audio.py --force
"""

import json
import sys
import os
import wave
import argparse
from pathlib import Path

# Add chat_reader_zonos to path so we can use FishSpeechBackend
ZONOS_DIR = Path("/home/login/PycharmProjects/chat_reader_zonos")
sys.path.insert(0, str(ZONOS_DIR))

SCRIPT_DIR = Path(__file__).parent
OUTPUT_DIR = SCRIPT_DIR / "audio-clips"
SCRIPT_JSON = SCRIPT_DIR / "demo-script.json"
VOICE_PATH = str(ZONOS_DIR / "voices" / "rhobotmeat.wav")


def get_wav_duration_ms(path: Path) -> int:
    """Return WAV file duration in milliseconds."""
    with wave.open(str(path), 'rb') as wf:
        frames = wf.getnframes()
        rate = wf.getframerate()
        return int(frames / rate * 1000)


def save_wav(audio_np, sample_rate: int, path: Path):
    """Save numpy float32 audio array as 16-bit WAV."""
    import numpy as np
    audio_int16 = (np.clip(audio_np, -1.0, 1.0) * 32767).astype(np.int16)
    with wave.open(str(path), 'wb') as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)  # 16-bit
        wf.setframerate(sample_rate)
        wf.writeframes(audio_int16.tobytes())


def load_manifest_texts(manifest_path: Path) -> dict:
    """Return {clip_id: spoken_text} from an existing manifest.json, or {} if missing."""
    if not manifest_path.exists():
        return {}
    with open(manifest_path) as f:
        data = json.load(f)
    return {c["id"]: c.get("text", "") for c in data}


def main():
    parser = argparse.ArgumentParser(description="Generate TTS audio clips for the demo video.")
    parser.add_argument("--force", action="store_true",
                        help="Regenerate all clips even if they already exist and text is unchanged.")
    args = parser.parse_args()

    # Load narration script
    with open(SCRIPT_JSON) as f:
        script = json.load(f)  # [{id, spoken}]

    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    manifest_path = OUTPUT_DIR / "manifest.json"
    existing_texts = load_manifest_texts(manifest_path)

    print(f"Script: {SCRIPT_JSON} ({len(script)} clips)")
    print(f"Loading Fish Speech backend...")
    from fish_speech_backend import FishSpeechBackend
    backend = FishSpeechBackend(
        checkpoint_path=str(ZONOS_DIR / "checkpoints" / "openaudio-s1-mini"),
        device="cuda",
        compile=False,
    )
    backend.load()
    print(f"Backend loaded. Processing {len(script)} clips...\n")

    manifest = []
    regenerated = 0
    skipped = 0

    for i, entry in enumerate(script):
        clip_id = entry["id"]
        dialog = entry["spoken"]
        out_path = OUTPUT_DIR / f"{clip_id}.wav"

        # Skip NIX entries — no audio for these captions
        if dialog.strip().upper().startswith("NIX"):
            print(f"  [{i+1:02d}/{len(script)}] {clip_id}: skip (NIX)")
            if out_path.exists():
                out_path.unlink()
            continue

        # Determine if we need to (re)generate
        text_changed = existing_texts.get(clip_id, "") != dialog
        needs_gen = args.force or not out_path.exists() or text_changed

        reason = ""
        if args.force:
            reason = "forced"
        elif not out_path.exists():
            reason = "new"
        elif text_changed:
            reason = "text changed"

        if not needs_gen:
            duration_ms = get_wav_duration_ms(out_path)
            print(f"  [{i+1:02d}/{len(script)}] {clip_id}: skip ({duration_ms}ms)")
            manifest.append({"id": clip_id, "path": str(out_path), "duration_ms": duration_ms, "text": dialog})
            skipped += 1
            continue

        # Delete stale file before regenerating
        if out_path.exists():
            out_path.unlink()

        print(f"  [{i+1:02d}/{len(script)}] {clip_id}: generating ({reason})...", end='', flush=True)
        try:
            result = backend.synthesize(dialog, VOICE_PATH)
            save_wav(result.audio, result.sample_rate, out_path)
            duration_ms = get_wav_duration_ms(out_path)
            print(f" {duration_ms}ms")
            manifest.append({"id": clip_id, "path": str(out_path), "duration_ms": duration_ms, "text": dialog})
            regenerated += 1
        except Exception as e:
            print(f" ERROR: {e}")
            manifest.append({"id": clip_id, "path": str(out_path), "duration_ms": 0, "error": str(e), "text": dialog})

    with open(manifest_path, 'w') as f:
        json.dump(manifest, f, indent=2)

    total_ms = sum(c.get("duration_ms", 0) for c in manifest)
    print(f"\nDone: {regenerated} generated, {skipped} unchanged")
    print(f"Total audio duration: {total_ms/1000:.1f}s")
    print(f"Manifest: {manifest_path}")


if __name__ == "__main__":
    main()
