# Read-only, bounded Windows logical file-size measurement. No file names leave this process.
Set-StrictMode -Version Latest

function Initialize-CodexTempCollector {
    if ('Yzrs.CodexTempCollector' -as [type]) { return }
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
namespace Yzrs {
 public sealed class TempResult {
  public long? Bytes; public string Status; public int Directories, Denied, Disappeared, Reparse;
 }
 public static class CodexTempCollector {
  const uint Reparse = 0x400, Directory = 0x10;
  [StructLayout(LayoutKind.Sequential)] struct Info {
   public uint Attributes; public System.Runtime.InteropServices.ComTypes.FILETIME Created, Access, Write;
   public uint Volume, SizeHigh, SizeLow, Links, IndexHigh, IndexLow;
  }
  [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
  static extern SafeFileHandle CreateFileW(string path, uint access, uint share, IntPtr sa, uint creation, uint flags, IntPtr template);
  [DllImport("kernel32.dll", SetLastError=true)] static extern bool GetFileInformationByHandle(SafeFileHandle file, out Info info);
  static SafeFileHandle Open(string path, out Info info) {
   // READ_ATTRIBUTES only; omit FILE_SHARE_DELETE. Holding each ancestor handle
   // prevents rename/junction substitution during descent. OPEN_REPARSE_POINT
   // opens the link itself rather than traversing its final component.
   var h = CreateFileW(path, 0x80, 3, IntPtr.Zero, 3, 0x02200000, IntPtr.Zero);
   if (h.IsInvalid) { int e=Marshal.GetLastWin32Error(); h.Dispose(); throw new System.ComponentModel.Win32Exception(e); }
   if (!GetFileInformationByHandle(h, out info)) { int e=Marshal.GetLastWin32Error(); h.Dispose(); throw new System.ComponentModel.Win32Exception(e); }
   return h;
  }
  sealed class Frame : IDisposable {
   public SafeFileHandle Handle; public IEnumerator<string> Entries; public int Depth;
   public void Dispose() { Entries?.Dispose(); Handle?.Dispose(); }
  }
  static void Failure(TempResult r, Exception e) {
   var w=e as System.ComponentModel.Win32Exception;
   if (e is UnauthorizedAccessException || w?.NativeErrorCode == 5) r.Denied++;
   else if (e is FileNotFoundException || e is DirectoryNotFoundException || w?.NativeErrorCode == 2 || w?.NativeErrorCode == 3) r.Disappeared++;
   r.Status = r.Denied > 0 ? "access-denied" : "partial";
  }
  public static TempResult Measure(string root, int timeoutMs, int maxEntries) {
   var r=new TempResult { Status="ok", Bytes=null }; var clock=Stopwatch.StartNew(); long sum=0; int entries=0, dirs=0;
   var stack=new Stack<Frame>();
   try {
    Info rootInfo; var rootHandle=Open(Path.GetFullPath(root), out rootInfo);
    if ((rootInfo.Attributes & Reparse)!=0 || (rootInfo.Attributes & Directory)==0) { rootHandle.Dispose(); r.Status="error"; return r; }
    stack.Push(new Frame { Handle=rootHandle, Entries=System.IO.Directory.EnumerateFileSystemEntries(root).GetEnumerator(), Depth=0 });
    while (stack.Count > 0) {
     if (clock.ElapsedMilliseconds >= timeoutMs) { r.Status="timeout"; break; }
     var f=stack.Peek(); string path;
     try { if (!f.Entries.MoveNext()) { stack.Pop().Dispose(); continue; } path=f.Entries.Current; }
     catch (Exception e) { Failure(r,e); stack.Pop().Dispose(); continue; }
     if (++entries > maxEntries) { r.Status="partial"; break; }
     // The Temp root selects immediate codex-* directories only. Unrelated
     // directories are never opened or recursively scanned.
     if (f.Depth==0 && !Path.GetFileName(path).StartsWith("codex-", StringComparison.OrdinalIgnoreCase)) continue;
     SafeFileHandle child=null;
     try {
      Info info; child=Open(path,out info);
      if ((info.Attributes & Reparse)!=0) { r.Reparse++; r.Status="partial"; continue; }
      if ((info.Attributes & Directory)!=0) {
       if (f.Depth==0) r.Directories++;
       if (++dirs>10000 || f.Depth>=256) { r.Status="partial"; break; }
       var frame=new Frame { Handle=child, Depth=f.Depth+1 };
       child=null; stack.Push(frame);
       frame.Entries=System.IO.Directory.EnumerateFileSystemEntries(path).GetEnumerator();
      } else if (f.Depth>0) {
       long length=((long)info.SizeHigh << 32) | info.SizeLow;
       checked { sum+=length; }
       if (sum>17592186044416L) { r.Status="partial"; break; }
      }
     } catch (Exception e) { Failure(r,e); }
     finally { child?.Dispose(); }
    }
    if (r.Status!="timeout" && r.Denied>0) r.Status="access-denied";
    if (r.Status=="ok") { r.Bytes=sum; if (r.Directories==0) r.Status="no-matches"; }
   } catch (Exception e) { Failure(r,e); if (r.Denied==0) r.Status="error"; }
   finally { while(stack.Count>0) stack.Pop().Dispose(); }
   return r;
  }
 }
}
'@
}

function Get-CodexTempMeasurement {
    param([string]$Root = (Join-Path $env:LOCALAPPDATA 'Temp'), [int]$TimeoutMs = 15000, [int]$MaxEntries = 250000)
    if ($TimeoutMs -lt 1 -or $TimeoutMs -gt 15000 -or $MaxEntries -lt 1 -or $MaxEntries -gt 250000) { throw 'Invalid collection bounds' }
    Initialize-CodexTempCollector
    $result = [Yzrs.CodexTempCollector]::Measure($Root, $TimeoutMs, $MaxEntries)
    return [ordered]@{
        schemaVersion = 1
        source = 'windows-codex-temp'
        measuredAt = [DateTimeOffset]::UtcNow.ToString("yyyy-MM-dd'T'HH:mm:ss.fff'Z'", [Globalization.CultureInfo]::InvariantCulture)
        codexTemp = [ordered]@{ bytes = $result.Bytes; status = $result.Status }
    }
}
