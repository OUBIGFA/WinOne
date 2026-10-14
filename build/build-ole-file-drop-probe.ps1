param([Parameter(Mandatory)][string]$OutputDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'WinOne.OleDropProbe.exe')
if($LASTEXITCODE -ne 0){throw 'OLE probe output must be Git-ignored'}
New-Item -ItemType Directory -Force -Path $output | Out-Null
$executable=Join-Path $output 'WinOne.OleDropProbe.exe'
if(Test-Path -LiteralPath $executable){throw 'Choose a new OLE probe output directory'}
Add-Type -Path (Join-Path $PSScriptRoot 'ole-file-drop-probe.cs') -ReferencedAssemblies @('System.dll','System.Core.dll','System.Drawing.dll','System.Windows.Forms.dll','System.Web.Extensions.dll') -OutputAssembly $executable -OutputType WindowsApplication
if(-not (Test-Path -LiteralPath $executable)){throw 'OLE probe compilation did not produce its executable'}
$executable
