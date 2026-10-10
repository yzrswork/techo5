# Same scheduled task and hidden launcher; isolated child processes and deadlines.
param([string]$Base = (Join-Path $env:LOCALAPPDATA 'YZRS\EchoDashboard'),
      [int]$HostTimeoutMs = 30000, [int]$AiTimeoutMs = 65000)
$ErrorActionPreference = 'Stop'
function Invoke-BoundedSender([string]$Name, [int]$Deadline) {
    $process = $null
    try {
        $info = [Diagnostics.ProcessStartInfo]::new()
        $info.FileName = (Get-Process -Id $PID).Path
        $info.UseShellExecute = $false
        $info.CreateNoWindow = $true
        foreach ($arg in @('-NoLogo','-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',(Join-Path $Base $Name))) { $info.ArgumentList.Add($arg) }
        $process = [Diagnostics.Process]::Start($info)
        if (-not $process.WaitForExit($Deadline)) {
            $process.Kill($true)
            $event = 'timeout'
        } else { $event = 'exit-' + $process.ExitCode }
    } catch { $event = 'launch-failed' }
    finally { if ($process) { $process.Dispose() } }
    try { [IO.File]::AppendAllText((Join-Path $Base 'send-operational-metrics.log'), ([DateTimeOffset]::UtcNow.ToString('o') + ' ' + $Name + ' ' + $event + [Environment]::NewLine)) } catch {}
}
Invoke-BoundedSender 'send-host-metrics.ps1' $HostTimeoutMs
Invoke-BoundedSender 'send-ai-usage.ps1' $AiTimeoutMs
