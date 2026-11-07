param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [Parameter(Mandatory)][string]$PDFWorker,
 [string]$FFmpegExe='',
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe'
)
# Explicit native decoder combination check. Generates only isolated fixtures,
# builds our media helper, and delegates Go validation to the shared UI entry.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'tests.log')
if($LASTEXITCODE-ne 0){throw 'Native workspace output must be Git-ignored'}
New-Item -ItemType Directory -Force $output,(Join-Path $output 'scratch')|Out-Null
$previous=@{}
foreach($name in @('TEMP','TMP','WINONE_MEDIA_WORKER','WINONE_MEDIA_TEST_VIDEO')){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
try {
 $env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP
 & (Join-Path $PSScriptRoot 'prepare-native-media.ps1') -OutputDirectory (Join-Path $output 'media')
 $env:WINONE_MEDIA_WORKER=Join-Path $output 'media/WinOne.Media.exe'
 $env:WINONE_MEDIA_TEST_VIDEO=''
 if($FFmpegExe){
  $video=Join-Path $output 'silent.mp4'
  & $FFmpegExe -hide_banner -loglevel error -f lavfi -i 'color=c=blue:size=64x48:rate=10' -f lavfi -i 'anullsrc=r=16000:cl=mono' -t 3 -c:v libx264 -pix_fmt yuv420p -c:a aac -movflags '+faststart' -y $video
  if($LASTEXITCODE-ne 0){throw 'Isolated media fixture encoding failed'}
  $env:WINONE_MEDIA_TEST_VIDEO=$video
 }
 & (Join-Path $PSScriptRoot 'test-native-ui.ps1') -Modules desktop-mygo -Packages ./chat/workspace,./chat/pdfium,./webui -PDFWorker $PDFWorker -GoExe $GoExe 2>&1 | ForEach-Object {$_.ToString()} | Out-File -Encoding utf8 (Join-Path $output 'tests.log')
 if($LASTEXITCODE-ne 0){throw 'Native workspace combination checks failed'}
} finally {foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}
