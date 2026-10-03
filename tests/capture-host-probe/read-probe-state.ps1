function Read-SharedProbeState([string]$Path) {
    $directory = Split-Path -Parent $Path
    $descriptor = Join-Path $directory 'state-pipe.json'
    if ((Split-Path -Leaf $Path) -eq 'state.json' -and (Test-Path -LiteralPath $descriptor)) {
        $endpoint = [IO.File]::ReadAllText($descriptor, [Text.Encoding]::UTF8) | ConvertFrom-Json
        if ($endpoint.version -ne 1 -or $endpoint.name -notmatch '^WinOneNativeState-[a-f0-9]{32}$') { throw 'Invalid native state endpoint' }
        $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $endpoint.name, [IO.Pipes.PipeDirection]::InOut)
        try {
            $pipe.Connect(3000)
            $reader = [IO.BinaryReader]::new($pipe)
            $length = $reader.ReadInt32()
            if ($length -lt 0 -or $length -gt 1048576) { throw 'Invalid native state length' }
            $bytes = $reader.ReadBytes($length)
            if ($bytes.Length -ne $length) { throw 'Incomplete native state' }
            $pipe.WriteByte(1); $pipe.Flush()
            return [Text.Encoding]::UTF8.GetString($bytes) | ConvertFrom-Json
        } catch { if (!(Test-Path -LiteralPath (Join-Path $directory 'state-complete.json'))) { throw } }
        finally { $pipe.Dispose() }
    }
    # The producer atomically replaces snapshots while native UI events run.
    # Readers must allow deletion/rename of the old file they are reading.
    $file = [IO.FileStream]::new($Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete)
    try {
        $reader = [IO.StreamReader]::new($file, [Text.Encoding]::UTF8)
        try { return $reader.ReadToEnd() | ConvertFrom-Json } finally { $reader.Dispose() }
    } finally { $file.Dispose() }
}
