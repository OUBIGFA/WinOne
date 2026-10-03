param(
    [Parameter(Mandatory=$true)][string]$Archive,
    [Parameter(Mandatory=$true)][string]$OutputDirectory
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $root 'build/sharex-capture-resources.ps1')
if(Test-Path -LiteralPath $OutputDirectory){throw 'Choose a new output directory'}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip=[IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $Archive).Path)
try {
    $entries=@($zip.Entries | Where-Object FullName -Like '*/ShareX.ScreenCaptureLib/Forms/RegionCaptureForm.cs')
    if($entries.Count -ne 1){throw 'Unexpected archive layout'}
    $reader=[IO.StreamReader]::new($entries[0].Open(),[Text.Encoding]::UTF8)
    try {$original=$reader.ReadToEnd()} finally {$reader.Dispose()}
} finally {$zip.Dispose()}
$path=Join-Path (Resolve-Path -LiteralPath $OutputDirectory).Path 'RegionCaptureForm.cs'
[IO.File]::WriteAllText($path,$original,[Text.UTF8Encoding]::new($false))
Update-ShareXCaptureResourceOwnership $path
$first=(Get-FileHash -LiteralPath $path).Hash
Update-ShareXCaptureResourceOwnership $path
if((Get-FileHash -LiteralPath $path).Hash -ne $first){throw 'Repeated preparation changed source'}
$complete=[IO.File]::ReadAllText($path)
foreach($required in @('captureBackground?.Dispose();','captureWindowIcon?.Dispose();')){
    $broken=$complete.Replace($required,'')
    [IO.File]::WriteAllText($path,$broken,[Text.UTF8Encoding]::new($false))
    $hash=(Get-FileHash -LiteralPath $path).Hash
    $rejected=$false
    try {Update-ShareXCaptureResourceOwnership $path} catch {if($_.Exception.Message -like 'Incomplete * ownership change'){$rejected=$true}else{throw}}
    if(!$rejected -or (Get-FileHash -LiteralPath $path).Hash -ne $hash){throw 'Incomplete adaptation was accepted or modified'}
}
[IO.File]::WriteAllText($path,$complete,[Text.UTF8Encoding]::new($false))
Write-Output 'PASS: clean archive preparation, idempotence and incomplete ownership rejection'
