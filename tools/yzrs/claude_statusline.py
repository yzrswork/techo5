"""Claude Code status line for the YZRS Echo Show HUD.

Prints model / context / rate-limit usage, and saves the rate-limit windows
(remaining percent and reset time only) for voice_bridge.py to deliver on the
next dashboard refresh. Never fails the status line: errors print what it can.
"""
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import sys
import tempfile

OUT = Path(os.environ.get("LOCALAPPDATA", "")) / "YZRS-TECHO5" / "claude-usage.json"


def window(raw):
    """rate_limits.<window> -> remaining percent, or None when absent or malformed."""
    try:
        used, reset = float(raw["used_percentage"]), int(raw["resets_at"])
    except (KeyError, TypeError, ValueError):
        return None
    if not 0 <= used <= 100:
        return None
    return {"remainingPercent": round(100 - used, 1),
        "resetsAt": datetime.fromtimestamp(reset, timezone.utc).isoformat(timespec="seconds")}


def save(snapshot, path=OUT):
    path.parent.mkdir(parents=True, exist_ok=True)
    # Several sessions may render at once; replace atomically so readers never see a partial file.
    fd, temp = tempfile.mkstemp(dir=path.parent, prefix=".claude-usage-")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(snapshot, f)
        os.replace(temp, path)
    finally:
        if os.path.exists(temp):
            os.remove(temp)


def render(data, path=OUT):
    limits = data.get("rate_limits") if isinstance(data.get("rate_limits"), dict) else {}
    snapshot = {"source": "claude-statusline",
        "capturedAt": datetime.now(timezone.utc).isoformat(timespec="seconds")}
    parts = []
    model = data.get("model") if isinstance(data.get("model"), dict) else {}
    if model.get("display_name"):
        parts.append(str(model["display_name"]))
    context = data.get("context_window") if isinstance(data.get("context_window"), dict) else {}
    if isinstance(context.get("used_percentage"), (int, float)):
        parts.append(f"ctx {context['used_percentage']:.0f}%")
    for key, name, label in (("five_hour", "fiveHour", "5h"), ("seven_day", "sevenDay", "7d")):
        value = window(limits.get(key)) if isinstance(limits.get(key), dict) else None
        if value:
            snapshot[name] = value
            parts.append(f"{label} {value['remainingPercent']:.0f}% left")
    if len(snapshot) > 2:
        try:
            save(snapshot, path)
        except OSError:
            pass  # The HUD keeps its previous value; the status line still renders.
    return " · ".join(parts)


def main():
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
    try:
        data = json.loads(sys.stdin.buffer.read().decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        data = {}
    print(render(data if isinstance(data, dict) else {}))


if __name__ == "__main__":
    main()
