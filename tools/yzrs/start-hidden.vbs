' Arguments: full path to pythonw.exe, voice_bridge.py, private config.json.
If WScript.Arguments.Count <> 3 Then WScript.Quit 2
Function Q(value)
  Q = Chr(34) & value & Chr(34)
End Function
CreateObject("WScript.Shell").Run Q(WScript.Arguments(0)) & " " & Q(WScript.Arguments(1)) & " --config " & Q(WScript.Arguments(2)), 0, False
