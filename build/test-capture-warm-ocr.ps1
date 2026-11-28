param([Parameter(Mandatory)][string]$CaptureDirectory,[Parameter(Mandatory)][string]$OutputDirectory,[string]$PythonExe='python')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory}else{Join-Path $root $OutputDirectory}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$env:PYTHONIOENCODING='utf-8'
& $PythonExe -B (Join-Path $root 'tests/ocr/native_warm_protocol.py') --capture (Resolve-Path -LiteralPath $CaptureDirectory).Path --output $output
if($LASTEXITCODE){throw 'Native warm OCR protocol failed'}
