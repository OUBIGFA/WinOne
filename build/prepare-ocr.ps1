param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$PythonExe='python',
 [string]$Downloader='aria2-next'
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'runtime.probe')
if($LASTEXITCODE -ne 0){throw 'OCR preparation output must be Git-ignored'}
$downloads=Join-Path $output 'downloads';$runtime=Join-Path $output 'Ocr';$python=Join-Path $runtime 'python';$models=Join-Path $runtime 'models'
New-Item -ItemType Directory -Force -Path $downloads,$runtime,$models | Out-Null
$assets=@(
 @('python-3.13.12-embed-amd64.zip','https://www.python.org/ftp/python/3.13.12/python-3.13.12-embed-amd64.zip','76f238f606250c87c6beac75dccd35ee99070a13490555936abb6cb64ecce3d0'),
 @('PP-OCRv6_det_small.onnx','https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/v3.9.2/onnx/PP-OCRv6/det/PP-OCRv6_det_small.onnx','090f04abcd9d9a7498bc4ebf677e4cb9bdce1fe4197ddb7e529f1ef44e1ff94f'),
 @('PP-OCRv6_rec_small.onnx','https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/v3.9.2/onnx/PP-OCRv6/rec/PP-OCRv6_rec_small.onnx','6f327246b50388f3c176ae304bd95767ea6dc0c9ae92153ef8cbe210b3c14884'),
 @('cv2.cp313-win_amd64.pyd','https://raw.githubusercontent.com/tcita/HushSnap/2d43f0e1fd125484bd132ee6e9cad1db65fb7a8c/third_party/cv2.cp313-win_amd64.pyd','2655a5b03331d19f5f43a3d52b262d99d1d3d4c6179783c4544eb9fdc5466282')
)
foreach($asset in $assets){
 $path=Join-Path $downloads $asset[0]
 if(-not(Test-Path -LiteralPath $path)){
  & $Downloader -d $downloads -o $asset[0] --check-certificate=true $asset[1]
  if($LASTEXITCODE -ne 0){throw "OCR asset download failed: $($asset[0])"}
 }
 if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $asset[2]){throw "OCR asset checksum mismatch: $($asset[0])"}
}
if(-not(Test-Path -LiteralPath (Join-Path $python 'python.exe'))){Expand-Archive -LiteralPath (Join-Path $downloads $assets[0][0]) -DestinationPath $python}
foreach($asset in $assets|Select-Object -Skip 1 -First 2){Copy-Item -LiteralPath (Join-Path $downloads $asset[0]) -Destination $models -Force}
Copy-Item -LiteralPath (Join-Path $downloads 'cv2.cp313-win_amd64.pyd') -Destination $runtime -Force
$priorCache=$env:PIP_CACHE_DIR;$priorUtf8=$env:PYTHONUTF8
try{
 $env:PIP_CACHE_DIR=Join-Path $output 'pip-cache';$env:PYTHONUTF8='1'
 # ANTLR publishes a pure-Python source release; build that one wheel with
 # the build Python, then install only pinned Windows/Python 3.13 wheels.
 & $PythonExe -m pip wheel --no-deps 'antlr4-python3-runtime==4.9.3' -w (Join-Path $output 'wheels')
 if($LASTEXITCODE -ne 0){throw 'OCR pure Python dependency build failed'}
 & $PythonExe -m pip install --upgrade --no-compile --target (Join-Path $python 'Lib/site-packages') --python-version 3.13 --platform win_amd64 --implementation cp --only-binary=:all: --find-links (Join-Path $output 'wheels') -r (Join-Path $root 'modules/capture/Ocr/requirements.txt')
 if($LASTEXITCODE -ne 0){throw 'OCR runtime dependency installation failed'}
}finally{$env:PIP_CACHE_DIR=$priorCache;$env:PYTHONUTF8=$priorUtf8}
[IO.File]::WriteAllText((Join-Path $python 'python313._pth'),"python313.zip`n.`nLib/site-packages`n..`nimport site`n",[Text.UTF8Encoding]::new($false))
Get-ChildItem -LiteralPath (Join-Path $root 'modules/capture/Ocr') -File | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination $runtime -Force}
& (Join-Path $python 'python.exe') -B -c "import rapidocr, onnxruntime, cv2, hushsnap_layout; assert hasattr(rapidocr.OCRVersion, 'PPOCRV6')"
if($LASTEXITCODE -ne 0){throw 'Bundled OCR runtime smoke test failed'}
$runtime
