"""TECHO5 native PTT. HW-16 keyboard queue / CUDA float16 Whisper / clipboard UX.

Audio stays in bounded RAM. Network and hotkeys remain live during GPU transcription.
No imports of keyboard/Whisper until main(), so protocol tests need no device or GPU.
"""
import asyncio
import json
import os
import re
import subprocess
from pathlib import Path
import shutil
import socket
import ssl
import sys
import time
import threading
import uuid

MAX_PCM = 16000 * 2 * 60
# Optional pc.json "stt_model". Unset keeps the accepted HW-16 small model.
STT_MODELS = ("small", "large-v3-turbo")
_stt_lock = threading.Lock()
_dll_handles = []


class StartupError(RuntimeError):
    """Only fixed categories, never exception strings containing private config."""


def startup_step(category, action):
    try:
        return action()
    except Exception:
        raise StartupError(category) from None


class Session:
    def __init__(self):
        self.phase = "IDLE"
        self.id = ""
        self.held = False
        self.pcm = bytearray()

    def key(self, event):
        if event == "down":
            if self.held:
                return None
            self.held = True
            if self.phase != "IDLE":
                return None
            self.id = uuid.uuid4().hex
            self.pcm.clear()
            self.phase = "STARTING"
            return {"op": "start", "id": self.id}
        self.held = False
        if self.phase in ("STARTING", "LISTENING"):
            self.phase = "STOPPING"
            return {"op": "stop", "id": self.id}
        return None

    def receive(self, value):
        if isinstance(value, bytes):
            if self.phase not in ("STARTING", "LISTENING", "STOPPING"):
                raise ValueError("Unexpected PCM")
            if len(value) != 640 or len(self.pcm) + len(value) > MAX_PCM:
                raise ValueError("PCM bound exceeded")
            self.pcm.extend(value)
            return None
        message = json.loads(value)
        if message.get("op") in ("ready", "pong"):
            return None
        if message.get("id") != self.id:
            raise ValueError("Session mismatch")
        op = message.get("op")
        if op == "started" and self.phase == "STARTING":
            self.phase = "LISTENING"
        elif op == "started" and self.phase == "STOPPING":
            pass  # KEY_UP was already sent during START acknowledgement latency.
        elif op == "stopped" and self.phase == "STOPPING":
            self.phase = "TRANSCRIBING"
            pcm = bytes(self.pcm)
            self.pcm.clear()
            return pcm
        elif op in ("aborted", "idle", "rejected"):
            self.phase = "IDLE"
            self.pcm.clear()
        else:
            raise ValueError("Unexpected session transition")
        return None

    def clear(self):
        self.pcm.clear()
        self.phase = "IDLE"
        self.id = ""
        # Preserve held until KEY_UP. Reconnection never resumes a held-key recording.


def transcribe_pcm(model, pcm, prompt=None):
    import numpy as np
    if len(pcm) < 1000:
        return ""  # Same short-recording threshold as the accepted HW-16 client.
    audio = np.frombuffer(pcm, dtype="<i2").astype(np.float32) / 32768.0
    if not np.any(audio):
        return ""  # Hardware/software mute produces zeros; Whisper can hallucinate on silence.
    if not _stt_lock.acquire(blocking=False):
        raise RuntimeError("Previous disconnected transcription still running")
    try:
        # Nonzero microphone noise is not speech. Silero VAD prevents decoding
        # noise-only recordings into plausible Japanese and pasting that text.
        # The vocabulary prompt biases spelling of known terms and makes Whisper
        # emit Japanese punctuation, so pasted text reads as sentences.
        segments, _ = model.transcribe(audio, language="ja", vad_filter=True, initial_prompt=prompt)
        return "".join(segment.text for segment in segments).strip()
    finally:
        _stt_lock.release()


def cuda_environment():
    dlls = ("cublas64_12.dll", "cublasLt64_12.dll", "cudart64_12.dll")
    if all(shutil.which(name) for name in dlls):
        return
    folder = Path(os.environ.get("VOICE_BRIDGE_CUDA_DLL_DIR", str(
        Path(os.environ["LOCALAPPDATA"]) / "Programs/Ollama/lib/ollama/cuda_v12")))
    if not all((folder / name).is_file() for name in dlls):
        raise RuntimeError("CUDA 12 の DLL フォルダーを設定してください。")
    os.environ["PATH"] = str(folder) + os.pathsep + os.environ["PATH"]
    if hasattr(os, "add_dll_directory"):
        _dll_handles.append(os.add_dll_directory(str(folder)))


def check_vad():
    # Check the bundled ONNX model before registering F8 or connecting to a device.
    import numpy as np
    from faster_whisper.vad import get_speech_timestamps
    get_speech_timestamps(np.zeros(512, dtype=np.float32))


async def _measure_lane(script, timeout, powershell=None):
    """Capture only MeasureOnly JSON; never log subprocess output or exceptions."""
    executable = powershell or os.environ.get("YZRS_DASHBOARD_POWERSHELL") or shutil.which("pwsh")
    if not executable or not script.is_file():
        return None
    process = None
    try:
        options = {"creationflags": subprocess.CREATE_NO_WINDOW} if os.name == "nt" else {}
        process = await asyncio.create_subprocess_exec(executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
            "-File", str(script), "-MeasureOnly", stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.DEVNULL, **options)
        stdout, _ = await asyncio.wait_for(process.communicate(), timeout)
        if process.returncode != 0 or len(stdout) > 16384:
            return None
        value = json.loads(stdout.decode("utf-8-sig"))
        return value if isinstance(value, dict) else None
    except Exception:
        return None
    finally:
        # asyncio cancellation also reaches finally. Reconnection never leaves a collector running.
        if process and process.returncode is None:
            try:
                process.kill()
            except ProcessLookupError:
                pass
            await process.communicate()


def _native_lane(lane, value):
    """Rebuild the allowlist even if an installed collector gains extra properties."""
    if not isinstance(value, dict):
        return None
    try:
        def stamp(value):
            return isinstance(value, str) and re.fullmatch(r"\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:[Zz]|[+-]\d{2}:\d{2})", value)
        if lane == "host":
            if not stamp(value.get("measuredAt")):
                return None
            if value["schemaVersion"] != 1 or value["source"] != "windows-codex-temp":
                return None
            temp = value["codexTemp"]
            status, size = temp["status"], temp["bytes"]
            if status in ("ok", "no-matches"):
                if type(size) is not int or not 0 <= size <= 17592186044416 or (status == "no-matches" and size != 0):
                    return None
            elif status not in ("partial", "access-denied", "timeout", "error") or size is not None:
                return None
            return {"schemaVersion": 1, "source": value["source"], "measuredAt": value["measuredAt"],
                "codexTemp": {"bytes": size, "status": status}, "stale": size is None}
        if lane == "claude":
            if value["source"] != "claude-statusline" or not stamp(value.get("capturedAt")):
                return None
            clean = {"source": value["source"], "capturedAt": value["capturedAt"]}
            for name in ("fiveHour", "sevenDay"):
                window = value.get(name)
                if window is None:
                    continue
                percent = window["remainingPercent"]
                if type(percent) not in (int, float) or not 0 <= percent <= 100 or not stamp(window.get("resetsAt")):
                    return None
                clean[name] = {"remainingPercent": percent, "resetsAt": window["resetsAt"]}
            return clean if len(clean) > 2 else None
        if lane == "times":
            if value["source"] != "yzrs-times" or not stamp(value.get("fetchedAt")):
                return None
            issue, title, outlet = value["issueNo"], value["title"], value["outlet"]
            if type(issue) is not int or issue <= 0 or value["edition"] not in ("morning", "midday"):
                return None
            if not isinstance(value["date"], str) or not re.fullmatch(r"\d{4}-\d{2}-\d{2}", value["date"]):
                return None
            if not isinstance(title, str) or not re.fullmatch(r"[^\x00-\x1f\x7f]{1,80}", title):
                return None
            if not isinstance(outlet, str) or not re.fullmatch(r"[^\x00-\x1f\x7f]{0,40}", outlet):
                return None
            return {name: value[name] for name in ("source", "issueNo", "edition", "date", "fetchedAt", "title", "outlet")}
        if lane != "ai":
            return None
        if value["source"] != "token-monitor" or not stamp(value.get("updatedAt")):
            return None
        codex = value["codex"]
        if type(codex["stale"]) is not bool or type(codex["todayTokens"]) is not int or not 0 <= codex["todayTokens"] <= 9007199254740991:
            return None
        if not isinstance(codex["plan"], str) or not re.fullmatch(r"[^@\r\n]{1,64}", codex["plan"]):
            return None
        if not isinstance(codex["status"], str) or not re.fullmatch(r"[a-zA-Z0-9_-]{1,32}", codex["status"]):
            return None
        clean = {name: codex[name] for name in ("plan", "status", "stale", "todayTokens")}
        for name in ("session", "weekly"):
            window = codex[name]
            percent = window["remainingPercent"]
            if type(percent) not in (int, float) or not 0 <= percent <= 100:
                return None
            if not stamp(window.get("resetsAt")):
                return None
            clean[name] = {"remainingPercent": percent, "resetsAt": window["resetsAt"]}
        return {"source": value["source"], "updatedAt": value["updatedAt"], "codex": clean}
    except (KeyError, TypeError):
        return None


CLAUDE_USAGE = Path(os.environ.get("LOCALAPPDATA", "")) / "YZRS-TECHO5" / "claude-usage.json"
TIMES_URL = "https://yzrswork.com/data/latest.json"


def read_claude_usage(path=None):
    """Last rate-limit snapshot written by claude_statusline.py; absent until Claude Code runs."""
    path = Path(path) if path else CLAUDE_USAGE
    if not path.is_file() or path.stat().st_size > 4096:
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def fetch_times(url=TIMES_URL, timeout=10):
    """Lead article of the latest public YZRS Times issue. No credentials are sent."""
    import urllib.parse
    import urllib.request
    from datetime import datetime, timezone
    request = urllib.request.Request(url, headers={"Accept": "application/json", "User-Agent": "yzrs-techo5-ptt"})
    with urllib.request.urlopen(request, timeout=timeout) as response:
        if response.status != 200 or urllib.parse.urlsplit(response.geturl()).hostname != "yzrswork.com":
            return None
        raw = response.read(524289)
    if len(raw) > 524288:
        return None
    issue = json.loads(raw.decode("utf-8"))
    articles = {a.get("id"): a for a in issue.get("articles", []) if isinstance(a, dict)}
    lead = articles.get((issue.get("top") or [None])[0])
    if not isinstance(lead, dict) or not isinstance(lead.get("title"), str):
        return None
    title = " ".join(lead["title"].split())
    outlet = " ".join(str(lead.get("source", "")).split())
    return {"source": "yzrs-times", "issueNo": issue.get("issueNo"), "edition": issue.get("edition"),
        "date": issue.get("date"), "fetchedAt": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "title": title if len(title) <= 80 else title[:79] + "…", "outlet": outlet[:40]}


async def measure_dashboard(base=None, collector=_measure_lane, readers=None):
    """Independent bounded local lanes. Missing evidence never becomes a zero."""
    base = Path(base) if base else Path(os.environ.get("LOCALAPPDATA", "")) / "YZRS/EchoDashboard"
    ai_script = Path(__file__).with_name("send-ai-usage.ps1")
    if readers is None:
        readers = {"claude": read_claude_usage, "times": fetch_times}
    async def lane(name, script, timeout):
        try:
            return name, _native_lane(name, await collector(script, timeout))
        except Exception:
            return name, None
    async def local(name, reader):
        try:
            return name, _native_lane(name, await asyncio.wait_for(asyncio.to_thread(reader), 15))
        except Exception:
            return name, None
    values = await asyncio.wait_for(asyncio.gather(
        lane("ai", ai_script, 20), lane("host", base / "send-host-metrics.ps1", 30),
        *(local(name, reader) for name, reader in readers.items())), 55)
    result = {name: value for name, value in values if value is not None}
    if result:
        # Keep real observation timestamps. A bounded delivery buffer lets the
        # device clock catch up without accepting future data or blocking voice.
        await asyncio.sleep(2)
    return result


async def connection(ws, events, session, transcribe, paste, measure=measure_dashboard):
    """Injected callbacks make the exact production lifecycle testable with fixture PCM."""
    async def heartbeat():
        while True:
            await ws.send('{"op":"ping"}')
            await asyncio.sleep(2)

    recv = asyncio.create_task(ws.recv())
    key = asyncio.create_task(events.get())
    beat = asyncio.create_task(heartbeat())
    stt = None
    stt_id = None
    dashboard = None
    dashboard_id = None
    async def refresh():
        try:
            return await asyncio.wait_for(measure(), 55)
        except Exception:
            return {}
    try:
        while True:
            waiting = [recv, key, beat] + ([stt] if stt else []) + ([dashboard] if dashboard else [])
            completed, _ = await asyncio.wait(waiting, return_when=asyncio.FIRST_COMPLETED)
            if beat in completed:
                beat.result()
                raise ConnectionError("Heartbeat stopped")
            # A disconnect/cancel is processed before any STT result, preventing a stale paste.
            if recv in completed:
                incoming = recv.result()
                command = json.loads(incoming) if isinstance(incoming, str) else None
                if isinstance(command, dict) and command.get("op") == "dashboard_refresh":
                    request_id = command.get("id")
                    if not isinstance(request_id, str) or not re.fullmatch(r"[0-9a-f]{32}", request_id):
                        raise ValueError("Invalid dashboard request")
                    if dashboard is None:
                        dashboard_id = request_id
                        dashboard = asyncio.create_task(refresh())
                    pcm = None
                else:
                    pcm = session.receive(incoming)
                recv = asyncio.create_task(ws.recv())
                if pcm is not None:
                    stt_id = session.id
                    stt = asyncio.create_task(asyncio.to_thread(transcribe, pcm))
            if key in completed:
                event = key.result()
                key = asyncio.create_task(events.get())
                if event == "exit":
                    if session.id:
                        await ws.send(json.dumps({"op": "cancel", "id": session.id}))
                    return False
                command = session.key(event)
                if command:
                    await ws.send(json.dumps(command))
            if stt and stt in completed:
                try:
                    text = stt.result()
                    if session.phase == "TRANSCRIBING" and session.id == stt_id:
                        # Return remote state to IDLE before paste; no text leaves this PC.
                        await ws.send(json.dumps({"op": "done", "id": session.id}))
                        if text:
                            paste(text)
                except Exception:
                    print("文字起こしに失敗しました。音声は保存されません。", flush=True)
                    if session.phase == "TRANSCRIBING":
                        await ws.send(json.dumps({"op": "cancel", "id": session.id}))
                finally:
                    stt = None
            if dashboard and dashboard in completed:
                result = dashboard.result()
                message = {"op": "dashboard_result", "id": dashboard_id}
                # Defense in depth: only native dashboard lanes can leave this PC.
                for name in ("ai", "host", "claude", "times"):
                    clean = _native_lane(name, result.get(name)) if isinstance(result, dict) else None
                    if clean is not None:
                        message[name] = clean
                await ws.send(json.dumps(message, allow_nan=False))
                dashboard = None
                dashboard_id = None
            print_state = session.phase
            if getattr(session, "reported", None) != print_state:
                print(print_state, flush=True)
                session.reported = print_state
    finally:
        for task in (recv, key, beat, stt, dashboard):
            if task:
                task.cancel()
        await asyncio.gather(*(t for t in (recv, key, beat, stt, dashboard) if t), return_exceptions=True)
        session.clear()


def read_config(path):
    cfg = json.loads(Path(path).read_text(encoding="utf-8-sig"))
    from urllib.parse import urlsplit
    url = urlsplit(cfg["url"])
    if url.scheme != "wss" or not url.hostname or url.path != "/ptt" or url.username or url.password or url.query or url.fragment:
        raise ValueError("Invalid PTT URL")
    if not isinstance(cfg["token"], str) or len(cfg["token"]) < 32 or any(c.isspace() for c in cfg["token"]):
        raise ValueError("Invalid PTT token")
    url.port  # Validate malformed ports before entering a reconnect loop.
    if not Path(cfg["ca_file"]).is_file():
        raise ValueError("Missing PTT certificate")
    cfg.setdefault("stt_model", "small")
    if cfg["stt_model"] not in STT_MODELS:
        raise ValueError("Unsupported STT model")
    words = cfg.setdefault("stt_vocabulary", [])
    if (not isinstance(words, list) or len(words) > 40 or any(
            not isinstance(w, str) or not w.strip() or len(w) > 32 for w in words)):
        raise ValueError("Invalid STT vocabulary")
    # Whisper keeps only the tail of an overlong prompt; bounded terms stay intact.
    cfg["stt_prompt"] = "、".join(w.strip() for w in words) + "。" if words else None
    return cfg


async def run(cfg, model, check_startup=False, exit_key=None):
    # No global exit key in production: Esc pressed in any app used to stop the
    # client silently. Stop with stop-voice.ps1. Tests pass exit_key to end a run.
    import keyboard
    import pyperclip
    from websockets.asyncio.client import connect
    from websockets.exceptions import InvalidStatus, InvalidHandshake
    tls = startup_step("TLS", lambda: ssl.create_default_context(cafile=cfg["ca_file"]))
    loop = asyncio.get_running_loop()
    events = asyncio.Queue(maxsize=32)
    session = Session()
    def enqueue(value):
        if events.full():
            # Never drop KEY_UP silently: close this client, so the lease cuts capture.
            while not events.empty():
                events.get_nowait()
            events.put_nowait("exit")
        else:
            events.put_nowait(value)
    def hook(event):
        if event.name == "f8":
            loop.call_soon_threadsafe(enqueue, event.event_type)
        elif exit_key and event.name == exit_key and event.event_type == keyboard.KEY_DOWN:
            loop.call_soon_threadsafe(enqueue, "exit")
    handle = startup_step("HOTKEY", lambda: keyboard.hook(hook))
    def paste(text):
        pyperclip.copy(text)
        keyboard.press_and_release("ctrl+v")
    try:
        print("PTT READY / F8 DOWN-UP / 終了は stop-voice.ps1", flush=True)
        if check_startup:
            return
        while True:
            try:
                async with connect(cfg["url"], ssl=tls, proxy=None,
                    additional_headers={"Authorization": "Bearer " + cfg["token"]},
                    open_timeout=5, close_timeout=2, max_size=1024, max_queue=8,
                    ping_interval=None) as ws:
                    if not await connection(ws, events, session,
                        lambda pcm: transcribe_pcm(model, pcm, cfg["stt_prompt"]), paste):
                        return
            except ssl.SSLCertVerificationError:
                raise StartupError("TLS_IDENTITY") from None
            except ssl.SSLError:
                raise StartupError("TLS_PROTOCOL") from None
            except InvalidStatus as error:
                status = error.response.status_code
                if status in (401, 403, 404):
                    raise StartupError("WSS_AUTH_OR_ENDPOINT") from None
                print("接続待ち / WSS サーバー応答待ち。", flush=True)
            except InvalidHandshake:
                raise StartupError("WSS_HANDSHAKE") from None
            except (OSError, asyncio.TimeoutError):
                print("接続待ち / NETWORK。", flush=True)
            except Exception:
                # Exception text may contain URLs/headers. Log a fixed, credential-free message.
                print("接続待ち / 音声セッションを破棄しました。", flush=True)
            session.clear()
            until = time.monotonic() + 2
            while time.monotonic() < until:
                try:
                    event = await asyncio.wait_for(events.get(), max(0.01, until-time.monotonic()))
                    if event == "exit":
                        return
                    session.held = event == "down"
                except asyncio.TimeoutError:
                    break
    finally:
        keyboard.unhook(handle)


def main():
    if sys.stdout is not None and hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
    parser.add_argument("--check-startup", action="store_true",
        help="Validate config/TLS/GPU/hotkeys, then exit without connecting or recording")
    args = parser.parse_args()
    # Cross-process singleton before loading the GPU model, even when Deck is pressed repeatedly.
    with socket.socket() as singleton:
        if hasattr(socket, "SO_EXCLUSIVEADDRUSE"):
            singleton.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        try:
            singleton.bind(("127.0.0.1", 17328))
            singleton.listen(1)
        except OSError:
            print("PTT クライアントは既に起動しています。", flush=True)
            return
        cfg = startup_step("CONFIG", lambda: read_config(args.config))
        startup_step("TLS", lambda: ssl.create_default_context(cafile=cfg["ca_file"]))
        startup_step("CUDA", cuda_environment)
        def load_dependencies():
            import keyboard, pyperclip, numpy
            from websockets.asyncio.client import connect
            from faster_whisper import WhisperModel
            return WhisperModel
        WhisperModel = startup_step("DEPENDENCIES", load_dependencies)
        startup_step("VAD", check_vad)
        print(cfg["stt_model"] + " / CUDA / float16 読み込み中", flush=True)
        model = startup_step("MODEL_CUDA", lambda: WhisperModel(cfg["stt_model"], device="cuda", compute_type="float16"))
        asyncio.run(run(cfg, model, args.check_startup))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        category = str(error) if isinstance(error, StartupError) else "RUNTIME"
        print("PTT 起動失敗 / " + category, flush=True)
        raise SystemExit(1)  # No raw exception/config/credentials in a startup log.
