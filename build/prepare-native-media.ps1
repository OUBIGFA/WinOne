param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$FrameworkDirectory=(Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319')
)
# Build only the repository-owned media helper. Windows' installed media/WPF
# assemblies are system dependencies, not copied or installed by this script.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory}else{Join-Path $root $OutputDirectory}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$csc=Join-Path $FrameworkDirectory 'csc.exe'
$references=@('WPF/WindowsBase.dll','WPF/PresentationCore.dll','WPF/PresentationFramework.dll','System.Xaml.dll','System.Web.Extensions.dll')
foreach($file in @($csc)+@($references|ForEach-Object{Join-Path $FrameworkDirectory $_})){if(-not(Test-Path -LiteralPath $file)){throw 'Windows media compiler/runtime unavailable; supply FrameworkDirectory without installing globally'}}
New-Item -ItemType Directory -Force -Path $output | Out-Null
$source=Join-Path $root 'apps/desktop-mygo/media-worker/Program.cs'
$arguments=@('/nologo','/target:exe','/optimize+','/platform:x64',('/out:'+(Join-Path $output 'WinOne.Media.exe')))
$arguments+=@($references|ForEach-Object{'/reference:'+(Join-Path $FrameworkDirectory $_)})
$arguments+=$source
& $csc @arguments
if($LASTEXITCODE-ne 0){throw 'Native Windows media helper build failed'}
$record=@{schemaVersion=1;engine='Windows WPF MediaPlayer';sourceSha256=(Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash.ToLowerInvariant();executableSha256=(Get-FileHash -LiteralPath (Join-Path $output 'WinOne.Media.exe') -Algorithm SHA256).Hash.ToLowerInvariant();systemRuntime='Windows .NET Framework 4.8 / operating-system media codecs'}
$record|ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $output 'native-media.json') -Encoding UTF8
Write-Output (Join-Path $output 'WinOne.Media.exe')
