param(
    [Parameter(Mandatory)][string]$CargoExe,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$TargetDirectory='dist/resources/cache/rust-desktop',
    [string]$Filter=''
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new preview directory'}
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'preview.json')
if($LASTEXITCODE -ne 0){throw 'Preview output must be Git-ignored'}
& (Join-Path $PSScriptRoot 'build-desktop-rust.ps1') -CargoExe $CargoExe -TargetDirectory $TargetDirectory -TestOnly
if($LASTEXITCODE -ne 0){throw 'Native preview build failed'}
New-Item -ItemType Directory -Path $output | Out-Null
$savedRustc=$env:RUSTC; $savedPreview=$env:WINONE_RUST_PREVIEW_DIRECTORY
$savedFilter=$env:WINONE_RUST_PREVIEW_FILTER
try{
    $cargoPath=(Get-Command $CargoExe -ErrorAction Stop).Source
    $env:RUSTC=Join-Path (Split-Path -Parent $cargoPath) 'rustc.exe'
    $target=if([IO.Path]::IsPathRooted($TargetDirectory)){$TargetDirectory}else{Join-Path $root $TargetDirectory}
    $artifacts=& $CargoExe test --locked --no-run --manifest-path (Join-Path $root 'apps/desktop-rust/Cargo.toml') --target-dir $target --bin winone-desktop --message-format=json
    if($LASTEXITCODE -ne 0){throw 'Preview executable unavailable'}
    $artifact=$artifacts | ForEach-Object {$_ | ConvertFrom-Json} | Where-Object {$_.reason -eq 'compiler-artifact' -and $_.profile.test -and $_.executable} | Select-Object -Last 1
    if(-not $artifact){throw 'Preview executable missing'}
    $env:WINONE_RUST_PREVIEW_DIRECTORY=$output
    $env:WINONE_RUST_PREVIEW_FILTER=$Filter
    $preview=Start-Process -FilePath $artifact.executable -ArgumentList '--ignored --exact native::tests::preview::render_native_pages --nocapture' -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput (Join-Path $output 'stdout.log') -RedirectStandardError (Join-Path $output 'stderr.log')
    Get-Content -LiteralPath (Join-Path $output 'stdout.log'),(Join-Path $output 'stderr.log') -Encoding UTF8
    if($preview.ExitCode -ne 0){throw 'Native page preview failed'}
}finally{$env:RUSTC=$savedRustc;$env:WINONE_RUST_PREVIEW_DIRECTORY=$savedPreview;$env:WINONE_RUST_PREVIEW_FILTER=$savedFilter}
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName Microsoft.VisualBasic
$items=[Collections.Generic.List[string]]::new()
foreach($bitmapFile in Get-ChildItem -LiteralPath $output -Filter '*.bmp' -File){
    $path=[IO.Path]::GetFullPath($bitmapFile.FullName)
    if(-not $path.StartsWith($output+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Preview file escaped output directory'}
    $image=[Drawing.Bitmap]::new($path)
    $png=[IO.Path]::ChangeExtension($path,'.png')
    try{$image.Save($png,[Drawing.Imaging.ImageFormat]::Png)}finally{$image.Dispose()}
    [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($path,[Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,[Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)
    $label=[Net.WebUtility]::HtmlEncode($bitmapFile.BaseName)
    $name=[Uri]::EscapeDataString([IO.Path]::GetFileName($png))
    $items.Add('<figure><a href="'+$name+'"><img loading="lazy" src="'+$name+'"></a><figcaption>'+$label+'</figcaption></figure>')
}
$html='<!doctype html><meta charset="utf-8"><title>Native layout previews</title><style>body{font:14px system-ui;margin:24px;background:#e2e5e9;color:#202020}main{display:grid;grid-template-columns:repeat(auto-fit,minmax(420px,1fr));gap:24px}figure{margin:0}img{width:100%;height:auto}figcaption{padding:8px}</style><h1>Rust native layout previews</h1><p>Synthetic data, native PrintWindow. Layout evidence; not compositor timing or mixed-DPI acceptance.</p><main>'+($items -join '')+'</main>'
[IO.File]::WriteAllText((Join-Path $output 'index.html'),$html,[Text.UTF8Encoding]::new($false))
'Saved preview gallery: '+(Join-Path $output 'index.html')
