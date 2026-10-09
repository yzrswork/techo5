import asyncio
import json
import unittest
import tempfile
from pathlib import Path

from voice_bridge import MAX_PCM, Session, connection, read_config, startup_step, StartupError


class StartupTests(unittest.TestCase):
    def test_missing_configuration_and_certificate(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "pc.json"
            with self.assertRaises(StartupError) as error:
                startup_step("CONFIG", lambda: read_config(path))
            self.assertEqual(str(error.exception), "CONFIG")
            path.write_text(json.dumps({"url": "wss://127.0.0.1:17327/ptt",
                "token": "x" * 64, "ca_file": str(Path(folder) / "absent.pem")}))
            with self.assertRaises(ValueError):
                read_config(path)

    def test_safe_error_category_and_invalid_config(self):
        def private_error():
            raise ValueError("Bearer must-never-appear")
        with self.assertRaises(StartupError) as error:
            startup_step("TLS", private_error)
        self.assertEqual(str(error.exception), "TLS")
        with tempfile.TemporaryDirectory() as folder:
            cert = Path(folder) / "cert.pem"
            cert.touch()
            cfg = {"url": "wss://127.0.0.1:bad/ptt", "token": "x" * 64, "ca_file": str(cert)}
            path = Path(folder) / "pc.json"
            path.write_text(json.dumps(cfg))
            with self.assertRaises(ValueError):
                read_config(path)
            cfg.update(url="wss://127.0.0.1:17327/ptt", token="x" * 32 + "\n")
            path.write_text(json.dumps(cfg))
            with self.assertRaises(ValueError):
                read_config(path)


class SessionTests(unittest.TestCase):
    def test_fast_key_up_and_repeat(self):
        s = Session()
        start = s.key("down")
        self.assertIsNone(s.key("down"))
        stop = s.key("up")
        self.assertEqual(start["id"], stop["id"])
        s.receive(json.dumps({"op": "started", "id": s.id}))
        s.receive(bytes(640))
        pcm = s.receive(json.dumps({"op": "stopped", "id": s.id}))
        self.assertEqual(len(pcm), 640)
        self.assertEqual(s.phase, "TRANSCRIBING")
        self.assertEqual(len(s.pcm), 0)
        with self.assertRaises(ValueError):
            s.receive(json.dumps({"op": "stopped", "id": s.id}))

    def test_bounds_and_reconnect_while_held(self):
        s = Session()
        s.key("down")
        s.pcm.extend(bytes(MAX_PCM))
        with self.assertRaises(ValueError):
            s.receive(bytes(640))
        s.clear()
        self.assertEqual(len(s.pcm), 0)
        self.assertIsNone(s.key("down"))
        s.key("up")
        self.assertIsNotNone(s.key("down"))


class FakeSocket:
    def __init__(self, events):
        self.incoming = asyncio.Queue()
        self.sent = []
        self.events = events

    async def recv(self):
        value = await self.incoming.get()
        if isinstance(value, Exception):
            raise value
        return value

    async def send(self, raw):
        msg = json.loads(raw)
        self.sent.append(msg)
        op, sid = msg["op"], msg.get("id", "")
        if op == "start":
            await self.incoming.put(json.dumps({"op": "started", "id": sid}))
            await self.incoming.put(bytes(640))
            await self.incoming.put(bytes(640))
            # Actual production loop sees key release while transport is asynchronous.
            await asyncio.sleep(0.01)
            await self.events.put("up")
        elif op == "stop":
            await self.incoming.put(json.dumps({"op": "stopped", "id": sid}))
        elif op == "done":
            await self.incoming.put(json.dumps({"op": "idle", "id": sid}))


class ConnectionTests(unittest.IsolatedAsyncioTestCase):
    async def test_fixture_transcription_single_paste_and_heartbeat(self):
        events = asyncio.Queue()
        ws = FakeSocket(events)
        s = Session()
        pasted, recordings = [], []
        def transcribe(pcm):
            recordings.append(pcm)
            return "日本語の音声入力テスト"
        def paste(text):
            pasted.append(text)
            events.put_nowait("exit")
        events.put_nowait("down")
        self.assertFalse(await asyncio.wait_for(connection(ws, events, s, transcribe, paste), 2))
        self.assertEqual(pasted, ["日本語の音声入力テスト"])
        self.assertEqual(len(recordings), 1)
        self.assertEqual(len(recordings[0]), 1280)
        self.assertEqual(len(s.pcm), 0)
        self.assertTrue(any(m["op"] == "ping" for m in ws.sent))
        self.assertEqual(sum(m["op"] == "stop" for m in ws.sent), 1)

    async def test_disconnect_during_stt_does_not_paste(self):
        events = asyncio.Queue()
        ws = FakeSocket(events)
        s = Session()
        pasted = []
        def transcribe(_):
            import time
            time.sleep(0.1)
            return "discard"
        original_send = ws.send
        async def send(raw):
            await original_send(raw)
            if json.loads(raw)["op"] == "stop":
                await ws.incoming.put(ConnectionError("disconnected"))
        ws.send = send
        events.put_nowait("down")
        with self.assertRaises(ConnectionError):
            await asyncio.wait_for(connection(ws, events, s, transcribe, pasted.append), 2)
        self.assertEqual(pasted, [])
        self.assertEqual(s.phase, "IDLE")


if __name__ == "__main__":
    unittest.main()
