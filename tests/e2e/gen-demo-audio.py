#!/usr/bin/env python3
"""
Generate spoken audio clips for the Apex MCD demo video.

Uses Fish Speech S1-mini with rhobotmeat.wav as the reference voice.
Outputs WAV files + a JSON manifest with durations.

Usage:
    cd /home/login/PycharmProjects/chat_reader_zonos
    source .venv/bin/activate
    python /home/login/PycharmProjects/gauntlet/apex/tests/e2e/gen-demo-audio.py
"""

import json
import sys
import os
import wave
from pathlib import Path

# Add chat_reader_zonos to path so we can use FishSpeechBackend
ZONOS_DIR = Path("/home/login/PycharmProjects/chat_reader_zonos")
sys.path.insert(0, str(ZONOS_DIR))

OUTPUT_DIR = Path("/home/login/PycharmProjects/gauntlet/apex/tests/e2e/audio-clips")
VOICE_PATH = str(ZONOS_DIR / "voices" / "rhobotmeat.wav")

# ---------------------------------------------------------------------------
# Caption clips — (clip_id, spoken_dialog)
# The demo script captions are shorter display text; this is the spoken version.
# ---------------------------------------------------------------------------
CLIPS = [
    ("01-dashboard",
     "The dashboard gives you an instant view of everything happening across your investor accounts — "
     "what's been cleared, what needs attention, and any exceptions that have come in."),

    ("02-keyboard",
     "Operators can navigate the entire system without touching the mouse — "
     "designed for high-volume environments."),

    ("03-shortcuts",
     "G then T for Transfers, G then R for the Review Queue, G then E for Settlement — "
     "you're never more than two keystrokes from anywhere you need to be."),

    ("04-search",
     "Control-K opens a command palette that searches any transfer or account — "
     "from anywhere in the application."),

    ("05-submit-check",
     "Here we're submitting a deposit with real check images — front and back — "
     "just as an investor would upload them from a mobile device."),

    ("06-recent",
     "Recent deposits appear below the form instantly, with live status updates as each one processes."),

    ("07-ai-analysis",
     "Every check goes through AI analysis — image quality, the printed dollar amount, "
     "routing number, and account information are all verified automatically on every submission."),

    ("08-one-submit",
     "One submission. Automated analysis. Compliance checks. Accounting entries. All in seconds."),

    ("09-realtime",
     "The status updates in real time — you can watch the deposit move through each stage "
     "without refreshing the page."),

    ("10-funds-posted",
     "Funds posted. The deposit passed all checks and the investor's account has been credited."),

    ("11-compliance",
     "Five compliance checks ran automatically — account eligibility, deposit limits, "
     "contribution type, and duplicate detection. Every one passed."),

    ("12-process-return",
     "If this check bounces later, a return can be initiated in one click — "
     "with automatic fee calculation and accounting."),

    ("13-mismatch",
     "This check shows seven hundred fifty dollars, but the customer declared five hundred. "
     "The system catches that discrepancy automatically and routes it for human review."),

    ("14-flagged",
     "Flagged for review. No manual monitoring required — the system caught it."),

    ("15-review-queue",
     "The review queue shows every deposit waiting for an operator decision — "
     "with how long each item has been waiting."),

    ("16-review-detail",
     "In the review detail, the operator sees the full transfer information "
     "and the check images together."),

    ("17-check-images",
     "The operator can compare the front and back images directly against what the AI reported."),

    ("18-ai-finding",
     "The AI read the printed amount as seven fifty. That discrepancy is what triggered the review."),

    ("19-audit-trail",
     "Every action is recorded — state changes, who took each action, and when. "
     "Full audit trail, built in."),

    ("20-approve",
     "After reviewing the images and the AI's findings, "
     "the operator approves the deposit with a note."),

    ("21-approved",
     "Approved. The deposit clears and the investor's account is credited immediately."),

    ("22-settlement",
     "Settlement packages all cleared deposits into the industry-standard clearing file "
     "format required by the Federal Reserve."),

    ("23-clearing-format",
     "This is the actual format used by US banks to exchange checks — "
     "with the check images embedded, as required."),

    ("24-batch-ready",
     "Settlement file generated and ready for transmission to the clearing network."),

    ("25-acknowledge",
     "Acknowledge confirms that the clearing bank has received and accepted the settlement file."),

    ("26-settled",
     "Settlement complete. All deposits in this batch are now fully settled."),

    ("27-returns",
     "When a check bounces, the return is processed using the bank's standard return reason code."),

    ("28-autocomplete",
     "Type the first few characters of a transfer ID and press Tab to auto-complete it."),

    ("29-nsf",
     "NSF — Non-Sufficient Funds — the most common reason for returned checks."),

    ("30-reversal",
     "The system reverses the original deposit and applies the non-sufficient funds fee — automatically."),

    ("31-returned",
     "Return processed. The original deposit is reversed and the NSF fee is recorded "
     "against the investor's account."),

    ("32-ledger",
     "Every transaction flows through the ledger — investor accounts, clearing accounts, "
     "and fee revenue — all reconciled."),

    ("33-journals",
     "The complete picture: the original deposit, the return reversal, and the NSF fee. "
     "Every debit matched by a credit."),

    ("34-dashboard-final",
     "Four core workflows — from submission to settlement to returns — all in one platform."),

    ("35-state-breakdown",
     "Live breakdown by status. Click any row to filter the transfers view instantly."),
]


def get_wav_duration_ms(path: Path) -> int:
    """Return WAV file duration in milliseconds."""
    with wave.open(str(path), 'rb') as wf:
        frames = wf.getnframes()
        rate = wf.getframerate()
        return int(frames / rate * 1000)


def save_wav(audio_np, sample_rate: int, path: Path):
    """Save numpy float32 audio array as 16-bit WAV."""
    import numpy as np
    import wave
    # Clamp and convert to int16
    audio_int16 = (np.clip(audio_np, -1.0, 1.0) * 32767).astype(np.int16)
    with wave.open(str(path), 'wb') as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)  # 16-bit
        wf.setframerate(sample_rate)
        wf.writeframes(audio_int16.tobytes())


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    print(f"Loading Fish Speech backend...")
    from fish_speech_backend import FishSpeechBackend
    backend = FishSpeechBackend(
        checkpoint_path=str(ZONOS_DIR / "checkpoints" / "openaudio-s1-mini"),
        device="cuda",
        compile=False,  # Faster startup, slightly slower inference
    )
    backend.load()
    print(f"Backend loaded. Generating {len(CLIPS)} clips...")

    manifest = []
    for i, (clip_id, dialog) in enumerate(CLIPS):
        out_path = OUTPUT_DIR / f"{clip_id}.wav"
        if out_path.exists():
            duration_ms = get_wav_duration_ms(out_path)
            print(f"  [{i+1}/{len(CLIPS)}] {clip_id}: already exists ({duration_ms}ms)")
            manifest.append({"id": clip_id, "path": str(out_path), "duration_ms": duration_ms, "text": dialog})
            continue

        print(f"  [{i+1}/{len(CLIPS)}] {clip_id}: generating...", end='', flush=True)
        try:
            result = backend.synthesize(dialog, VOICE_PATH)
            save_wav(result.audio, result.sample_rate, out_path)
            duration_ms = get_wav_duration_ms(out_path)
            print(f" {duration_ms}ms")
            manifest.append({"id": clip_id, "path": str(out_path), "duration_ms": duration_ms, "text": dialog})
        except Exception as e:
            print(f" ERROR: {e}")
            manifest.append({"id": clip_id, "path": str(out_path), "duration_ms": 0, "error": str(e), "text": dialog})

    manifest_path = OUTPUT_DIR / "manifest.json"
    with open(manifest_path, 'w') as f:
        json.dump(manifest, f, indent=2)

    print(f"\nDone! {len(manifest)} clips written to {OUTPUT_DIR}")
    print(f"Manifest: {manifest_path}")

    total_ms = sum(c.get("duration_ms", 0) for c in manifest)
    print(f"Total audio duration: {total_ms/1000:.1f}s")


if __name__ == "__main__":
    main()
