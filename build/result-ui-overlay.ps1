param([string]$GoExe='go')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root 'apps/result-ui')
try {
 $source=& $GoExe list -m -f '{{.Dir}}' github.com/egoist/mygo
 if($LASTEXITCODE -ne 0){throw 'Cannot locate the pinned MyGo source'}
 $version=& $GoExe list -m -f '{{.Version}}' github.com/egoist/mygo
 if($LASTEXITCODE -ne 0 -or $version -ne 'v0.2.13-0.20261006044557-3ad015a7f887'){throw 'Recheck the selected-text adapter before updating MyGo'}
 if([string]::IsNullOrWhiteSpace($source)){
  & $GoExe mod download "github.com/egoist/mygo@$version"
  if($LASTEXITCODE -ne 0){throw 'Cannot download the pinned MyGo source'}
  $source=& $GoExe list -m -f '{{.Dir}}' github.com/egoist/mygo
  if($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($source)){throw 'Pinned MyGo source is unavailable'}
 }
 # Go forbids overlays inside GOMODCACHE. Copy the pinned sources into an
 # ignored build tree and replace only this build's module, leaving the cache intact.
 $directory=Join-Path $root '_temp/result-ui-overlay'
 New-Item -ItemType Directory -Path $directory -Force|Out-Null
 $local=Join-Path $directory 'mygo'
 if(-not (Test-Path -LiteralPath $local)){Copy-Item -LiteralPath $source -Destination $local -Recurse}
 $patch=Join-Path $local 'ui/winone_selected_text.go'
 if(Test-Path -LiteralPath $patch){(Get-Item -LiteralPath $patch).IsReadOnly=$false}
 Copy-Item -LiteralPath (Join-Path $root 'third_party/mygo-patches/selected_text.go.txt') -Destination $patch -Force
 Copy-Item -LiteralPath (Join-Path $root 'third_party/mygo-patches/fast_selection_test.go.txt') -Destination (Join-Path $local 'ui/winone_fast_selection_test.go') -Force
 $inputPath=Join-Path $local 'ui/input.go'
 $inputSource=[IO.File]::ReadAllText((Join-Path $source 'ui/input.go'))
 $needle='s.editor.release()'
 if(([regex]::Matches($inputSource,[regex]::Escape($needle))).Count -ne 1){throw 'MyGo pointer release no longer matches the verified patch'}
 $replacement='// WinOne: commit the release coordinate even between two paints.'+[Environment]::NewLine+'        if s.editor.dragging { s.editor.drag(rt.pointerX-s.x, rt.pointerY-s.y) }'+[Environment]::NewLine+'        s.editor.release()'
 (Get-Item -LiteralPath $inputPath).IsReadOnly=$false
 [IO.File]::WriteAllText($inputPath,$inputSource.Replace($needle,$replacement),[Text.UTF8Encoding]::new($false))
 $mod=Join-Path $directory 'result.mod'
 Copy-Item -LiteralPath 'go.mod' -Destination $mod -Force
 Copy-Item -LiteralPath 'go.sum' -Destination (Join-Path $directory 'result.sum') -Force
 & $GoExe mod edit "-modfile=$mod" '-replace=github.com/egoist/mygo=../../_temp/result-ui-overlay/mygo'
 if($LASTEXITCODE -ne 0){throw 'Cannot prepare the isolated result UI module'}
 # ICO stores 256 as a zero dimension byte. Fyne's mask loops otherwise
 # produce a fully transparent 256px BMP icon; patch only this build copy.
 $icoVersion=& $GoExe list -m -f '{{.Version}}' github.com/fyne-io/image
 if($LASTEXITCODE -ne 0 -or $icoVersion -ne 'v0.1.1'){throw 'Recheck the ICO dimension adapter before updating Fyne image'}
 $icoSource=& $GoExe list -m -f '{{.Dir}}' github.com/fyne-io/image
 if([string]::IsNullOrWhiteSpace($icoSource)){
  & $GoExe mod download "github.com/fyne-io/image@$icoVersion"
  if($LASTEXITCODE -ne 0){throw 'Cannot download the pinned ICO decoder'}
  $icoSource=& $GoExe list -m -f '{{.Dir}}' github.com/fyne-io/image
 }
 if($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($icoSource)){throw 'Pinned ICO decoder source is unavailable'}
 $icoLocal=Join-Path $directory 'fyne-image'
 if(-not (Test-Path -LiteralPath $icoLocal)){Copy-Item -LiteralPath $icoSource -Destination $icoLocal -Recurse}
 $icoReader=[IO.File]::ReadAllText((Join-Path $icoSource 'ico/reader.go'))
 foreach($dimension in @('Width','Height')){
  $needle="int(e.$dimension)"
  if(([regex]::Matches($icoReader,[regex]::Escape($needle))).Count -ne 4){throw 'Upstream ICO dimensions no longer match the verified adapter'}
  $icoReader=$icoReader.Replace($needle,"winOneICODimension(e.$dimension)")
 }
 $icoReader+="`n// WinOne: ICO's zero dimension byte represents 256 pixels.`nfunc winOneICODimension(value byte) int { if value == 0 { return 256 }; return int(value) }`n"
 $icoReaderPath=Join-Path $icoLocal 'ico/reader.go'
 (Get-Item -LiteralPath $icoReaderPath).IsReadOnly=$false
 [IO.File]::WriteAllText($icoReaderPath,$icoReader,[Text.UTF8Encoding]::new($false))
 & $GoExe mod edit "-modfile=$mod" '-replace=github.com/fyne-io/image=../../_temp/result-ui-overlay/fyne-image'
 if($LASTEXITCODE -ne 0){throw 'Cannot prepare the isolated ICO decoder'}
 Write-Output $mod
} finally {Pop-Location}
