[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$Source)
$ErrorActionPreference='Stop'
$destination=$env:WINONE_UNINSTALLER_EXPORT
if(-not [IO.Path]::IsPathRooted($destination)){throw 'Uninstaller export destination must be absolute'}
$destination=[IO.Path]::GetFullPath($destination)
if(-not [IO.Directory]::Exists([IO.Path]::GetDirectoryName($destination))){throw 'Uninstaller export directory must exist'}
if([IO.File]::Exists($destination) -or [IO.Directory]::Exists($destination)){throw 'Uninstaller export already exists'}
$sourceStream=[IO.File]::Open($Source,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
$partial=$destination+'.partial'
try {
    if($sourceStream.Length -lt 2 -or $sourceStream.Length -gt 16MB -or $sourceStream.ReadByte() -ne 77 -or $sourceStream.ReadByte() -ne 90){throw 'Invalid generated uninstaller'}
    $sourceStream.Position=0
    # CreateNew also protects an earlier failed export. Keep partial output on failure.
    $outputStream=[IO.File]::Open($partial,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    try { $sourceStream.CopyTo($outputStream); $outputStream.Flush($true) } finally { $outputStream.Dispose() }
    $sourceHash=(Get-FileHash -LiteralPath $Source -Algorithm SHA256).Hash
    if((Get-FileHash -LiteralPath $partial -Algorithm SHA256).Hash -ne $sourceHash){throw 'Uninstaller export hash mismatch'}
    [IO.File]::Move($partial,$destination)
} finally { $sourceStream.Dispose() }
