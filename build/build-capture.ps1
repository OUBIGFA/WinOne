param(
    [Parameter(Mandatory)][string]$ShareXSourceDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$DotnetExe='dotnet',
    [string]$OcrRuntimeDirectory=$env:WINONE_OCR_RUNTIME_DIRECTORY
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=(Resolve-Path -LiteralPath $ShareXSourceDirectory).Path
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'build-output.probe')
if($LASTEXITCODE -ne 0){throw 'Capture output must be Git-ignored'}
if($source -eq (Join-Path $root 'dist/resources/sources/ShareX-21.0.0')){throw 'Use the isolated ShareX source'}
. (Join-Path $PSScriptRoot 'sharex-native-host.ps1')
Initialize-ShareXNativeHost $source
& $DotnetExe publish (Join-Path $root 'modules/capture/WinOne.Capture.csproj') -c Release -r win-x64 --self-contained true "-p:ShareXSourceDirectory=$source" -o $output --nologo -v:q
if($LASTEXITCODE -ne 0){throw 'Capture worker build failed'}
if($OcrRuntimeDirectory){& (Join-Path $PSScriptRoot 'copy-ocr.ps1') -RuntimeDirectory $OcrRuntimeDirectory -CaptureDirectory $output}
& (Join-Path $PSScriptRoot 'build-capture-regions.ps1') -OutputDirectory $output
if($LASTEXITCODE -ne 0){throw 'Capture regions build failed'}
