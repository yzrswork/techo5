"""Optional local Windows acceptance against the loopback Go fixture.

Uses the real GPU and real hook registration, but injects F8 into only this client's
callback. Never sends F8 to another app or pastes recognized/private speech.
"""
import asyncio
import json
import os
from pathlib import Path
import socket
import ssl
import subprocess
import sys
from types import SimpleNamespace
from unittest.mock import patch

import keyboard
import pyperclip
from voice_bridge import StartupError, cuda_environment, read_config, run


async def exercise(cfg, model, speech=False, expected_error=None, waiting=False):
    original_hook = keyboard.hook
    callback = None
    paste_count = 0
    def register(hook):
        nonlocal callback
        callback = hook
        return original_hook(hook)
    def paste(text):
        nonlocal paste_count
        assert text and any("\u3040" <= c <= "\u9fff" for c in text)
        paste_count += 1
    def shortcut(value):
        assert value == "ctrl+v"
        callback(SimpleNamespace(name="esc", event_type="down"))
    async def keys():
        while callback is None:
            await asyncio.sleep(0.01)
        if waiting:
            await asyncio.sleep(6)
            callback(SimpleNamespace(name="esc", event_type="down"))
        elif speech:
            await asyncio.sleep(0.4)
            callback(SimpleNamespace(name="f8", event_type="down"))
            callback(SimpleNamespace(name="f8", event_type="down"))
            await asyncio.sleep(5.6)
            callback(SimpleNamespace(name="f8", event_type="up"))
    with patch.object(keyboard, "hook", register), patch.object(pyperclip, "copy", paste), patch.object(keyboard, "press_and_release", shortcut):
        driver = asyncio.create_task(keys())
        try:
            if expected_error:
                try:
                    await asyncio.wait_for(run(cfg, model), 12)
                except StartupError as error:
                    assert str(error) == expected_error
                else:
                    raise AssertionError("Invalid identity/auth must not reconnect forever")
            else:
                await asyncio.wait_for(run(cfg, model), 35)
                assert paste_count == (1 if speech else 0)
        finally:
            driver.cancel()
            await asyncio.gather(driver, return_exceptions=True)


async def main(cert):
    private = Path(os.environ["LOCALAPPDATA"]) / "YZRS-TECHO5/trial-20261009"
    cfg = read_config(private / "pc.json")
    show = json.loads((private / "yzrs.json").read_text())
    assert cfg["token"] == show["ptt_token"] == (private / "ptt-token.txt").read_text()
    identity = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    identity.load_cert_chain(private / "ptt-cert.pem", private / "ptt-key.pem")
    decoded = ssl._ssl._test_decode_cert(str(private / "ptt-cert.pem"))
    assert ("IP Address", "192.168.0.222") in decoded["subjectAltName"]
    import time
    assert ssl.cert_time_to_seconds(decoded["notBefore"]) <= time.time() < ssl.cert_time_to_seconds(decoded["notAfter"])
    print("PRIVATE_CONFIG / TLS_KEY_MATCH / SAN / VALIDITY PASS")
    cuda_environment()
    from faster_whisper import WhisperModel
    model = WhisperModel("small", device="cuda", compute_type="float16", local_files_only=True)
    local = dict(url="wss://127.0.0.1:17329/ptt", token="fixture-token-loopback-only-000000", ca_file=cert)
    await exercise(dict(local, token="invalid-test-token-00000000000000"), model, expected_error="WSS_AUTH_OR_ENDPOINT")
    await exercise(dict(local, ca_file=cfg["ca_file"]), model, expected_error="TLS_IDENTITY")
    print("WSS_AUTH_REJECTION / TLS_REJECTION_NO_RETRY PASS")
    await exercise(local, model, speech=True)
    print("PRODUCTION_F8_CALLBACK / GPU_JAPANESE / SINGLE_PASTE_CALLBACK / CLEAN_EXIT PASS")
    await exercise(cfg, model, waiting=True)
    print("PRODUCTION_DEVICE_CONNECTION_WAIT / ESC_CLEAN_EXIT PASS")
    with socket.socket() as lock:
        lock.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        lock.bind(("127.0.0.1", 17328))
        lock.listen(1)
        result = subprocess.run([sys.executable, str(Path(__file__).with_name("voice_bridge.py")), "--config", str(private / "pc.json")], capture_output=True, timeout=5)
        assert result.returncode == 0 and b"small" not in result.stdout and result.stdout
    print("DUPLICATE_PROCESS_BLOCKED_BEFORE_GPU PASS")


async def reconnect(cert):
    from websockets.asyncio.server import serve
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(cert, str(Path(cert).with_name("ptt-key.pem")))
    callback = None
    attempts = 0
    original_hook = keyboard.hook
    def register(hook):
        nonlocal callback
        callback = hook
        return original_hook(hook)
    async def handler(ws):
        nonlocal attempts
        assert ws.request.headers["Authorization"] == "Bearer fixture-token-loopback-only-000000"
        attempts += 1
        await ws.send('{"op":"ready"}')
        if attempts == 1:
            await ws.close(code=1011)
        else:
            callback(SimpleNamespace(name="esc", event_type="down"))
            await ws.wait_closed()
    cfg = dict(url="wss://127.0.0.1:17330/ptt", token="fixture-token-loopback-only-000000", ca_file=cert)
    async with serve(handler, "127.0.0.1", 17330, ssl=tls):
        with patch.object(keyboard, "hook", register):
            await asyncio.wait_for(run(cfg, None), 12)
    assert attempts == 2
    print("PRODUCTION_TLS_DISCONNECT / RECONNECT / ESC_CLEAN_EXIT PASS")


if __name__ == "__main__":
    asyncio.run(reconnect(sys.argv[1]) if "--reconnect" in sys.argv else main(sys.argv[1]))
