"""Real TLS/WebSocket transport + production lifecycle, synthetic microphone and STT.

Run against cmd/yzrs-ptt-fixture. No keyboard hooks or focused-field mutation.
"""
import asyncio
import ssl
import sys
from websockets.asyncio.client import connect
from voice_bridge import Session, connection


async def main(cert):
    tls = ssl.create_default_context(cafile=cert)
    events = asyncio.Queue()
    pasted = []
    calls = []
    session = Session()
    async with connect("wss://127.0.0.1:17329/ptt", ssl=tls, proxy=None,
        additional_headers={"Authorization": "Bearer fixture-token-loopback-only-000000"},
        max_size=1024, max_queue=8, ping_interval=None) as ws:
        def transcribe(pcm):
            assert len(pcm) >= 1000 and len(pcm) <= 32000, len(pcm)
            assert set(pcm) == {0}
            calls.append(len(pcm))
            return "日本語の音声入力テスト"
        def paste(text):
            pasted.append(text)
            events.put_nowait("exit")
        async def keys():
            await events.put("down")
            await asyncio.sleep(0.25)
            await events.put("up")
        task = asyncio.create_task(keys())
        await asyncio.wait_for(connection(ws, events, session, transcribe, paste), 5)
        await task
    assert pasted == ["日本語の音声入力テスト"] and len(calls) == 1
    assert not session.pcm and session.phase == "IDLE"
    print("P2A TLS / PCM / KEY_DOWN-UP / single transcription / paste callback / cleanup PASS")


if __name__ == "__main__":
    asyncio.run(main(sys.argv[1]))
