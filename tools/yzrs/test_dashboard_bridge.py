import asyncio
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import AsyncMock, patch

from voice_bridge import Session, connection, measure_dashboard, _native_lane, _measure_lane
from test_voice_bridge import FakeSocket

STAMP = '2026-10-12T00:00:00.000Z'
HOST = {'schemaVersion': 1, 'source': 'windows-codex-temp', 'measuredAt': STAMP,
        'codexTemp': {'bytes': 1234, 'status': 'ok'}}
AI = {'source': 'token-monitor', 'updatedAt': STAMP, 'codex': {
    'plan': 'Pro', 'status': 'ok', 'stale': False, 'todayTokens': 42,
    'session': {'remainingPercent': 50, 'resetsAt': STAMP},
    'weekly': {'remainingPercent': 60, 'resetsAt': STAMP}}}

class LaneTests(unittest.IsolatedAsyncioTestCase):
    async def test_lanes_independent_and_deadlines(self):
        calls = []
        async def collector(path, timeout):
            calls.append((path.name, timeout))
            if path.name == 'send-ai-usage.ps1':
                raise TimeoutError('private diagnostic')
            return dict(HOST, privateIdentity='never-forward')
        result = await measure_dashboard('fixture', collector)
        self.assertEqual(set(result), {'host'})
        self.assertEqual(result['host']['codexTemp']['bytes'], 1234)
        self.assertFalse(result['host']['stale'])
        self.assertNotIn('privateIdentity', result['host'])
        self.assertCountEqual(calls, [('send-ai-usage.ps1', 20), ('send-host-metrics.ps1', 30)])

    async def test_subprocess_timeout_and_cancel_reap_without_output(self):
        class Process:
            returncode = None
            def __init__(self):
                self.first = True
                self.killed = False
                self.started = asyncio.Event()
            async def communicate(self):
                if self.first:
                    self.first = False
                    self.started.set()
                    await asyncio.Future()
                return b'', b''
            def kill(self):
                self.killed = True
                self.returncode = -1
        with tempfile.TemporaryDirectory() as folder:
            script = Path(folder) / 'sender.ps1'
            script.touch()
            for cancel in (False, True):
                process = Process()
                with patch('voice_bridge.asyncio.create_subprocess_exec', AsyncMock(return_value=process)) as spawn:
                    task = asyncio.create_task(_measure_lane(script, .01, 'fixture-pwsh'))
                    await process.started.wait()
                    if cancel:
                        task.cancel()
                        with self.assertRaises(asyncio.CancelledError):
                            await task
                    else:
                        self.assertIsNone(await task)
                    self.assertTrue(process.killed)
                    self.assertEqual(spawn.call_args.kwargs['stderr'], asyncio.subprocess.DEVNULL)

    def test_host_partial_is_null_and_stale(self):
        result = _native_lane('host', dict(HOST, codexTemp={'bytes': None, 'status': 'timeout'}))
        self.assertIsNone(result['codexTemp']['bytes'])
        self.assertTrue(result['stale'])
        self.assertIsNone(_native_lane('host', dict(HOST, codexTemp={'status': 'ok'})))
        self.assertIsNone(_native_lane('host', dict(HOST, codexTemp={'bytes': True, 'status': 'ok'})))

    def test_native_allowlists_and_invalid_ai(self):
        ai = json.loads(json.dumps(AI))
        ai['accountEmail'] = 'never-forward'
        ai['codex']['accountEmail'] = 'never-forward'
        ai['codex']['session']['accountEmail'] = 'never-forward'
        self.assertEqual(_native_lane('ai', ai), AI)
        ai['codex']['todayTokens'] = None
        self.assertIsNone(_native_lane('ai', ai))
        ai = json.loads(json.dumps(AI))
        ai['codex']['weekly']['resetsAt'] = {'privateIdentity': 'never-forward'}
        self.assertIsNone(_native_lane('ai', ai))

class RefreshTests(unittest.IsolatedAsyncioTestCase):
    async def test_refresh_overlaps_voice_and_duplicate_is_ignored(self):
        events = asyncio.Queue()
        ws = FakeSocket(events)
        session = Session()
        entered, release = asyncio.Event(), asyncio.Event()
        count, pasted = [], []
        async def measure():
            count.append(1)
            entered.set()
            await release.wait()
            return {'ai': AI, 'host': HOST, 'privateIdentity': 'never-forward'}
        def paste(text):
            pasted.append(text)
            release.set()
        original = ws.send
        async def send(raw):
            await original(raw)
            if json.loads(raw)['op'] == 'dashboard_result':
                events.put_nowait('exit')
        ws.send = send
        await ws.incoming.put(json.dumps({'op': 'dashboard_refresh', 'id': 'a' * 32}))
        task = asyncio.create_task(connection(ws, events, session, lambda _: 'fixture', paste, measure))
        await entered.wait()
        await ws.incoming.put(json.dumps({'op': 'dashboard_refresh', 'id': 'b' * 32}))
        events.put_nowait('down')
        self.assertFalse(await asyncio.wait_for(task, 2))
        result = [m for m in ws.sent if m['op'] == 'dashboard_result']
        self.assertEqual(len(count), 1)
        self.assertEqual(pasted, ['fixture'])
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]['id'], 'a' * 32)
        self.assertEqual(set(result[0]), {'op', 'id', 'ai', 'host'})
        self.assertTrue(any(m['op'] == 'ping' for m in ws.sent))

    async def test_disconnect_cancels_measurement_and_never_sends_result(self):
        events = asyncio.Queue()
        ws = FakeSocket(events)
        entered, canceled = asyncio.Event(), asyncio.Event()
        async def measure():
            entered.set()
            try:
                await asyncio.Future()
            finally:
                canceled.set()
        await ws.incoming.put(json.dumps({'op': 'dashboard_refresh', 'id': 'c' * 32}))
        task = asyncio.create_task(connection(ws, events, Session(), None, None, measure))
        await entered.wait()
        await ws.incoming.put(ConnectionError('disconnect'))
        with self.assertRaises(ConnectionError):
            await asyncio.wait_for(task, 2)
        self.assertTrue(canceled.is_set())
        self.assertFalse(any(m['op'] == 'dashboard_result' for m in ws.sent))

    async def test_missing_lanes_return_matching_empty_result(self):
        events = asyncio.Queue()
        ws = FakeSocket(events)
        async def measure():
            return {}
        original = ws.send
        async def send(raw):
            await original(raw)
            if json.loads(raw)['op'] == 'dashboard_result':
                events.put_nowait('exit')
        ws.send = send
        await ws.incoming.put(json.dumps({'op': 'dashboard_refresh', 'id': 'd' * 32}))
        self.assertFalse(await asyncio.wait_for(connection(ws, events, Session(), None, None, measure), 2))
        result = [m for m in ws.sent if m['op'] == 'dashboard_result']
        self.assertEqual(result, [{'op': 'dashboard_result', 'id': 'd' * 32}])

if __name__ == '__main__':
    unittest.main()
