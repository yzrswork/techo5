"""TECHO5 native PTT. HW-16 keyboard queue / small CUDA float16 / clipboard UX.

Audio stays in bounded RAM. Network and hotkeys remain live during GPU transcription.
No imports of keyboard/Whisper until main(), so protocol tests need no device or GPU.
"""
import asyncio
import json
import os
from pathlib import Path
import shutil
import socket
import ssl
import time
import threading
import uuid

MAX_PCM = 16000 * 2 * 60
_stt_lock = threading.Lock()


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


def transcribe_pcm(model, pcm):
    import numpy as np
    if len(pcm) < 1000:
        return ""  # Same short-recording threshold as the accepted HW-16 client.
    audio = np.frombuffer(pcm, dtype="<i2").astype(np.float32) / 32768.0
    if not np.any(audio):
        return ""  # Hardware/software mute produces zeros; Whisper can hallucinate on silence.
    if not _stt_lock.acquire(blocking=False):
        raise RuntimeError("Previous disconnected transcription still running")
    try:
        segments, _ = model.transcribe(audio, language="ja", vad_filter=False)
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


async def connection(ws, events, session, transcribe, paste):
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
    try:
        while True:
            waiting = [recv, key, beat] + ([stt] if stt else [])
            completed, _ = await asyncio.wait(waiting, return_when=asyncio.FIRST_COMPLETED)
            if beat in completed:
                beat.result()
                raise ConnectionError("Heartbeat stopped")
            # A disconnect/cancel is processed before any STT result, preventing a stale paste.
            if recv in completed:
                pcm = session.receive(recv.result())
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
            print_state = session.phase
            if getattr(session, "reported", None) != print_state:
                print(print_state, flush=True)
                session.reported = print_state
    finally:
        for task in (recv, key, beat, stt):
            if task:
                task.cancel()
        await asyncio.gather(*(t for t in (recv, key, beat, stt) if t), return_exceptions=True)
        session.clear()


def read_config(path):
    cfg = json.loads(Path(path).read_text(encoding="utf-8-sig"))
    from urllib.parse import urlsplit
    url = urlsplit(cfg["url"])
    if url.scheme != "wss" or not url.hostname or url.path != "/ptt" or url.username or url.query or url.fragment:
        raise ValueError("Invalid PTT URL")
    if len(cfg["token"]) < 32:
        raise ValueError("Invalid PTT token")
    return cfg


async def run(cfg, model):
    import keyboard
    import pyperclip
    from websockets.asyncio.client import connect
    tls = ssl.create_default_context(cafile=cfg["ca_file"])
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
        elif event.name == "esc" and event.event_type == keyboard.KEY_DOWN:
            loop.call_soon_threadsafe(enqueue, "exit")
    handle = keyboard.hook(hook)
    def paste(text):
        pyperclip.copy(text)
        keyboard.press_and_release("ctrl+v")
    try:
        while True:
            try:
                async with connect(cfg["url"], ssl=tls, proxy=None,
                    additional_headers={"Authorization": "Bearer " + cfg["token"]},
                    open_timeout=5, close_timeout=2, max_size=1024, max_queue=8,
                    ping_interval=None) as ws:
                    if not await connection(ws, events, session,
                        lambda pcm: transcribe_pcm(model, pcm), paste):
                        return
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
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
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
        cfg = read_config(args.config)
        cuda_environment()
        from faster_whisper import WhisperModel
        print("small / CUDA / float16 読み込み中", flush=True)
        model = WhisperModel("small", device="cuda", compute_type="float16")
        asyncio.run(run(cfg, model))


if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("PTT 起動失敗。設定・TLS・CUDA 環境を確認してください。", flush=True)
        raise SystemExit(1)  # No raw exception/config/credentials in a startup log.
