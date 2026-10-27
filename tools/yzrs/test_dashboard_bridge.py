import asyncio
import json
import os
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
    async def test_delivery_buffer_preserves_real_timestamps(self):
        async def collector(path, timeout):
            return HOST
        with patch('voice_bridge.asyncio.sleep', new_callable=AsyncMock) as settle:
            result = await measure_dashboard('fixture', collector, readers={})
            settle.assert_awaited_once_with(2)
        self.assertEqual(result['host']['measuredAt'], STAMP)

    async def test_lanes_independent_and_deadlines(self):
        calls = []
        async def collector(path, timeout):
            calls.append((path.name, timeout))
            return dict(HOST, privateIdentity='never-forward')
        def broken():
            raise TimeoutError('private diagnostic')
        result = await measure_dashboard('fixture', collector, readers={'times': broken})
        self.assertEqual(set(result), {'host'})
        self.assertEqual(result['host']['codexTemp']['bytes'], 1234)
        self.assertFalse(result['host']['stale'])
        self.assertNotIn('privateIdentity', result['host'])
        # The retired Codex AI collector (send-ai-usage.ps1) is never started.
        self.assertEqual(calls, [('send-host-metrics.ps1', 30)])

    async def test_collector_path_survives_empty_path_and_explicit_override(self):
        class Process:
            returncode = 0
            async def communicate(self):
                return json.dumps(HOST).encode(), b""
        with tempfile.TemporaryDirectory() as folder:
            script = Path(folder) / "sender.ps1"
            script.touch()
            executable = str(Path(folder) / "PowerShell 7" / "pwsh.exe")
            with patch.dict(os.environ, {"PATH": "", "YZRS_DASHBOARD_POWERSHELL": executable}), \
                    patch("voice_bridge.shutil.which", return_value=None) as discover, \
                    patch("voice_bridge.asyncio.create_subprocess_exec", AsyncMock(return_value=Process())) as spawn:
                self.assertEqual(await _measure_lane(script, 1), HOST)
                self.assertEqual(spawn.call_args.args[0], executable)
                discover.assert_not_called()
                self.assertEqual(await _measure_lane(script, 1, "explicit-pwsh"), HOST)
                self.assertEqual(spawn.call_args.args[0], "explicit-pwsh")

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

    def test_retired_ai_lane_never_leaves_pc(self):
        self.assertIsNone(_native_lane('ai', AI))

CLAUDE = {'source': 'claude-statusline', 'capturedAt': STAMP,
          'fiveHour': {'remainingPercent': 77, 'resetsAt': STAMP},
          'sevenDay': {'remainingPercent': 59.5, 'resetsAt': STAMP}}
TIMES = {'source': 'yzrs-times', 'issueNo': 213, 'edition': 'morning', 'date': '2026-10-25',
         'fetchedAt': STAMP, 'title': 'JSがこんなに減る！ HTML・CSS最新技術2026', 'outlet': 'はてなブックマーク テクノロジー'}


class ClaudeTimesLaneTests(unittest.IsolatedAsyncioTestCase):
    def test_allowlists(self):
        self.assertEqual(_native_lane('claude', dict(CLAUDE, accountEmail='never-forward')), CLAUDE)
        self.assertEqual(_native_lane('times', dict(TIMES, summaryJa=['never-forward'])), TIMES)
        only_week = {k: v for k, v in CLAUDE.items() if k != 'fiveHour'}
        self.assertEqual(_native_lane('claude', only_week), only_week)
        for bad in ({'fiveHour': None, 'sevenDay': None}, {'source': 'token-monitor'},
                    {'fiveHour': {'remainingPercent': 101, 'resetsAt': STAMP}},
                    {'sevenDay': {'remainingPercent': True, 'resetsAt': STAMP}}):
            self.assertIsNone(_native_lane('claude', dict(CLAUDE, **bad)))
        for bad in ({'issueNo': True}, {'edition': 'evening'}, {'title': ''}, {'title': 'a\nb'},
                    {'title': 'あ' * 81}, {'outlet': 'x' * 41}, {'date': '10/25'}):
            self.assertIsNone(_native_lane('times', dict(TIMES, **bad)))
        self.assertIsNone(_native_lane('unknown', CLAUDE))

    async def test_local_lanes_independent(self):
        async def collector(path, timeout):
            return None
        def broken():
            raise OSError('private diagnostic')
        with patch('voice_bridge.asyncio.sleep', new_callable=AsyncMock):
            result = await measure_dashboard('fixture', collector, readers={'claude': lambda: CLAUDE, 'times': broken})
        self.assertEqual(result, {'claude': CLAUDE})

    def test_statusline_saves_remaining_and_prints(self):
        from claude_statusline import render
        with tempfile.TemporaryDirectory() as folder:
            out = Path(folder) / 'usage.json'
            line = render({'model': {'display_name': 'Opus'}, 'context_window': {'used_percentage': 23.4},
                'rate_limits': {'five_hour': {'used_percentage': 23, 'resets_at': 1761382800},
                                'seven_day': {'used_percentage': 'bad', 'resets_at': 1}}}, out)
            self.assertEqual(line, 'Opus · ctx 23% · 5h 77% left')
            saved = json.loads(out.read_text(encoding='utf-8'))
            self.assertEqual(saved['fiveHour'], {'remainingPercent': 77.0, 'resetsAt': '2025-10-25T09:00:00+00:00'})
            self.assertNotIn('sevenDay', saved)
            self.assertIsNotNone(_native_lane('claude', saved))
            out.unlink()
            # Before the first API response there are no rate limits: keep the previous file.
            self.assertEqual(render({'model': {'display_name': 'Opus'}}, out), 'Opus')
            self.assertFalse(out.exists())


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
            return {'ai': AI, 'host': HOST, 'times': TIMES, 'privateIdentity': 'never-forward'}
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
        self.assertEqual(set(result[0]), {'op', 'id', 'host', 'times'})
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
