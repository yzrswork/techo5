"""Optional offline CUDA regression with a caller-owned, known Japanese PCM fixture.

No device connection, recording, clipboard writes, or key injection. Raw input and
recognized text are never logged. Supply 16 kHz mono PCM16LE and its reference text.
"""
import argparse
from difflib import SequenceMatcher
import json
from pathlib import Path
import re
import time

from voice_bridge import cuda_environment, transcribe_pcm


def normalized(text):
    return re.sub(r"[^\w]", "", text)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pcm", type=Path, required=True)
    parser.add_argument("--expected", required=True)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    import numpy as np
    from faster_whisper import WhisperModel
    from faster_whisper.vad import get_speech_timestamps
    cuda_environment()
    model = WhisperModel("small", device="cuda", compute_type="float16", local_files_only=True)
    raw = args.pcm.read_bytes()
    assert len(raw) >= 32000 and len(raw) % 2 == 0, "Invalid PCM16 fixture"
    reference = normalized(args.expected)
    assert reference and any("\u3040" <= c <= "\u9fff" for c in reference), "Japanese reference required"
    source = np.frombuffer(raw, dtype="<i2").astype(np.float32)
    peak = float(np.max(np.abs(source)))
    assert peak > 0, "Silent reference fixture"
    results = []

    def check(name, samples, speech):
        pcm = np.rint(samples).astype("<i2")
        audio = pcm.astype(np.float32) / 32768.0
        start = time.perf_counter()
        text = transcribe_pcm(model, pcm.tobytes())
        similarity = SequenceMatcher(None, reference, normalized(text)).ratio() if speech else None
        # Small-model transcription can vary in kanji/punctuation; measure against
        # the supplied ground truth, never accept merely "some Japanese".
        if speech:
            assert similarity >= 0.85, name
        else:
            assert text == "", name
        rms = float(np.sqrt(np.mean(audio**2)))
        results.append({"case": name, "pass": True, "seconds": round(time.perf_counter()-start, 3),
            "samples": len(pcm), "duration_seconds": len(pcm)/16000,
            "peak": int(np.max(np.abs(pcm.astype(np.int32)))),
            "rms_dbfs": float(20*np.log10(rms)) if rms else None,
            "exact_zero_fraction": float(np.mean(pcm == 0)),
            "clipping_fraction": float(np.mean((pcm == 32767) | (pcm == -32768))),
            "vad_seconds": sum(t["end"]-t["start"] for t in get_speech_timestamps(audio))/16000,
            "characters": len(text), "reference_similarity": similarity})

    check("known_japanese", source, True)
    for level in (715, 200):
        check(f"known_japanese_peak{level}", source*(level/peak), True)
    check("known_japanese_with_leading_trailing_silence", np.pad(source, (16000*3, 16000*3)), True)
    check("digital_mute", np.zeros(102720), False)
    check("short_recording", np.zeros(320), False)
    for seed in (0, 1, 2):
        noise = np.random.default_rng(seed).normal(size=102720)
        spectrum = np.fft.rfft(noise)
        spectrum /= np.sqrt(np.maximum(np.fft.rfftfreq(len(noise)), 1/len(noise)))
        noise = np.fft.irfft(spectrum, n=len(noise))
        check(f"non_speech_pink_noise_seed{seed}", noise*(715/np.max(np.abs(noise))), False)
    check("non_speech_hum", 715*np.sin(2*np.pi*60*np.arange(102720)/16000), False)
    report = {"model": "small", "device": "cuda", "compute_type": "float16",
              "format": "16000 Hz / mono / signed PCM16 little-endian", "cases": results}
    if args.report:
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"LOCAL CUDA STT: {len(results)} cases PASS; no clipboard or device actions")


if __name__ == "__main__":
    main()
