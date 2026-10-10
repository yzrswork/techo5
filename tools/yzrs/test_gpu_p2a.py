"""Optional PC acceptance using an existing private PCM sample and real local CUDA STT.

The fixture server must use -pcm. This only verifies paste callback, never pastes to the desktop.
"""
import asyncio
import ssl
import sys
import time
from websockets.asyncio.client import connect
from voice_bridge import Session, connection, transcribe_pcm, cuda_environment


async def main(cert, duration):
    cuda_environment()
    from faster_whisper import WhisperModel
    import ctranslate2
    assert ctranslate2.get_cuda_device_count() > 0
    model = WhisperModel("small", device="cuda", compute_type="float16", local_files_only=True)
    # Confirm digital mute/silence cannot inject a hallucinated transcript.
    assert transcribe_pcm(model, bytes(32000)) == ""
    events = asyncio.Queue()
    received = []
    session = Session()
    tls = ssl.create_default_context(cafile=cert)
    async with connect("wss://127.0.0.1:17329/ptt", ssl=tls, proxy=None,
        additional_headers={"Authorization": "Bearer fixture-token-loopback-only-000000"},
        max_size=1024, max_queue=8, ping_interval=None) as ws:
        def infer(pcm):
            assert len(pcm) > 32000 and any(pcm)
            started = time.perf_counter()
            result = transcribe_pcm(model, pcm)
            assert result and any("\u3040" <= c <= "\u9fff" for c in result)
            print(f"CUDA STT {time.perf_counter()-started:.2f}s; transcript characters {len(result)}")
            return result
        def paste_callback(text):
            received.append(len(text))
            events.put_nowait("exit")
        async def keys():
            events.put_nowait("down")
            await asyncio.sleep(duration)
            events.put_nowait("up")
        task = asyncio.create_task(keys())
        await asyncio.wait_for(connection(ws, events, session, infer, paste_callback), duration + 30)
        await task
    assert len(received) == 1 and not session.pcm
    print("P2A existing private PCM -> TLS -> small/CUDA/float16 -> Japanese paste callback PASS")


if __name__ == "__main__":
    asyncio.run(main(sys.argv[1], float(sys.argv[2])))
