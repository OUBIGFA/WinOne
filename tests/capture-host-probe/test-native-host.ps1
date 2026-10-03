param([Parameter(Mandatory)][string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../../build/sharex-native-host.ps1')
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Choose a new test directory' }
$engine = Join-Path $OutputDirectory 'ShareX.ScreenCaptureLib'
New-Item -ItemType Directory -Path $engine -Force | Out-Null
$drawing = Join-Path $engine 'Shapes/Drawing'
New-Item -ItemType Directory -Path $drawing -Force | Out-Null
foreach ($type in @('FreehandDrawingShape', 'LineDrawingShape')) {
    [IO.File]::WriteAllText((Join-Path $drawing ($type + '.cs')), ('public class ' + $type + ' : BaseDrawingShape {}'), [Text.UTF8Encoding]::new($false))
}
$project = Join-Path $engine 'ShareX.ScreenCaptureLib.csproj'
[IO.File]::WriteAllText($project, '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net9.0-windows</TargetFramework></PropertyGroup></Project>', [Text.UTF8Encoding]::new($false))
Initialize-ShareXNativeHost $OutputDirectory
$before = [IO.File]::ReadAllBytes($project)
Initialize-ShareXNativeHost $OutputDirectory
if ([Convert]::ToBase64String([IO.File]::ReadAllBytes($project)) -ne [Convert]::ToBase64String($before)) { throw 'Repeated preparation rewrote the project' }
[xml]$document = [IO.File]::ReadAllText($project)
$imports = @($document.SelectNodes('/Project/Import[@Label="WinOneNativeCapture"]'))
if ($imports.Count -ne 1 -or $document.Project.PropertyGroup.TargetFramework -ne 'net9.0-windows') { throw 'Engine project contract changed' }
$target = [IO.Path]::GetFullPath((Join-Path $engine $imports[0].GetAttribute('Project')))
if (!(Test-Path -LiteralPath $target -PathType Leaf)) { throw 'Native host import does not resolve' }
[void]$document.Project.AppendChild($imports[0].CloneNode($true)); $document.Save($project)
$rejected = $false
try { Initialize-ShareXNativeHost $OutputDirectory } catch { if ($_.Exception.Message -ne 'Duplicate WinOne native capture imports') { throw }; $rejected = $true }
if (!$rejected) { throw 'Duplicate native imports were accepted' }
Write-Output 'PASS: native host preparation preserves project fields, resolves its import, is idempotent and rejects duplicate imports'
