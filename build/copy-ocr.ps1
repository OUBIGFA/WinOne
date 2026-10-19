param([Parameter(Mandatory)][string]$RuntimeDirectory,[Parameter(Mandatory)][string]$CaptureDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$runtime=(Resolve-Path -LiteralPath $RuntimeDirectory).Path
$destination=Join-Path ([IO.Path]::GetFullPath($CaptureDirectory)) 'Ocr'
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $destination
if(-not(Test-Path -LiteralPath (Join-Path $runtime 'python/python.exe'))){throw 'Prepared OCR runtime is missing'}
foreach($model in @(@('PP-OCRv6_det_small.onnx','090f04abcd9d9a7498bc4ebf677e4cb9bdce1fe4197ddb7e529f1ef44e1ff94f'),@('PP-OCRv6_rec_small.onnx','6f327246b50388f3c176ae304bd95767ea6dc0c9ae92153ef8cbe210b3c14884'))){
 if((Get-FileHash -LiteralPath (Join-Path $runtime ('models/'+$model[0])) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $model[1]){throw 'OCR model checksum mismatch'}
}
New-Item -ItemType Directory -Force -Path $destination | Out-Null
# Ship HushSnap's pinned minimal OpenCV build, not the wheel's video stack.
# RapidOCR 3.10 loads sessions lazily. Classification is disabled for screen
# text, so only the two explicitly selected PP-OCRv6 models belong in the bundle.
$minimal=Join-Path $runtime 'cv2.cp313-win_amd64.pyd'
if((Get-FileHash -LiteralPath $minimal -Algorithm SHA256).Hash.ToLowerInvariant() -ne '2655a5b03331d19f5f43a3d52b262d99d1d3d4c6179783c4544eb9fdc5466282'){throw 'Minimal OpenCV checksum mismatch'}
foreach($file in Get-ChildItem -LiteralPath $runtime -File -Recurse){
 $relative=$file.FullName.Substring($runtime.Length+1).Replace('\','/')
 if($relative -notmatch '^(python|models)/'){continue}
 if($relative -match '(^|/)(__pycache__|tests?|benchmarks)(/|$)' -or $relative -match '\.py[coi]$'){continue}
 if($relative -match '(^|/)(_examples|examples|test-examples)(/|$)' -or $relative -match '^python/Lib/site-packages/(bin|share)/'){continue}
 if($relative -match '\.(h|lib|c|pxd|pyx|sh|js|1|pc|build|template)$'){continue}
 if($relative -match '/cv2/(cv2[^/]*\.pyd|opencv_videoio[^/]*\.dll)$'){continue}
 if($relative -match '/rapidocr/models/'){continue}
 $target=Join-Path $destination $relative
 New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
 Copy-Item -LiteralPath $file.FullName -Destination $target -Force
}
Copy-Item -LiteralPath $minimal -Destination (Join-Path $destination 'python/Lib/site-packages/cv2/cv2.pyd') -Force
# Copy source files afresh; runtime preparation must not freeze an old adapter.
Get-ChildItem -LiteralPath (Join-Path $root 'modules/capture/Ocr') -File | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination $destination -Force}
& (Join-Path $destination 'python/python.exe') -B -c "import rapidocr, onnxruntime, cv2, hushsnap_layout"
if($LASTEXITCODE -ne 0){throw 'Packaged OCR import failed'}
