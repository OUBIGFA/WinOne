param([Parameter(Mandatory)][string]$OutputDirectory,[Parameter(Mandatory)][string]$CargoExe)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$crate=Join-Path $output 'crate'
New-Item -ItemType Directory -Force -Path (Join-Path $crate 'src')|Out-Null
foreach($name in @('lib.rs','core.rs','session.rs','encoder.rs')){Copy-Item -LiteralPath (Join-Path $root ('modules/recording/src/'+$name)) -Destination (Join-Path $crate ('src/'+$name)) -Force}
Copy-Item -LiteralPath (Join-Path $root 'tests/capture-native/recording_source.rs') -Destination (Join-Path $crate 'src/capture.rs') -Force
$manifest=[IO.File]::ReadAllText((Join-Path $root 'modules/recording/Cargo.toml'))
$manifest=$manifest.Replace('../../third_party/gifski',(Join-Path $root 'third_party/gifski').Replace('\','/'))
[IO.File]::WriteAllText((Join-Path $crate 'Cargo.toml'),$manifest,[Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath (Join-Path $root 'modules/recording/Cargo.lock') -Destination (Join-Path $crate 'Cargo.lock') -Force
# An exported marker lets the package verifier reject the generated source
# binary even if someone accidentally renames it to the production filename.
[IO.File]::AppendAllText((Join-Path $crate 'src/lib.rs'),"`n#[unsafe(no_mangle)]`npub extern `"C`" fn winone_recording_synthetic_test()->bool{true}`n",[Text.UTF8Encoding]::new($false))
$vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vs=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
$msvc=Get-ChildItem -LiteralPath (Join-Path $vs 'VC/Tools/MSVC') -Directory|Sort-Object Name -Descending|Select-Object -First 1
$sdk=Get-ChildItem -LiteralPath (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/Lib') -Directory|Sort-Object Name -Descending|Select-Object -First 1
$previousPath=$env:PATH;$previousLib=$env:LIB;$previousRustc=$env:RUSTC
try{
    $env:PATH=(Join-Path $msvc.FullName 'bin/Hostx64/x64')+';'+$env:PATH
    $env:LIB=@((Join-Path $msvc.FullName 'lib/x64'),(Join-Path $sdk.FullName 'um/x64'),(Join-Path $sdk.FullName 'ucrt/x64'),$previousLib)-join ';'
    $env:RUSTC=Join-Path (Split-Path $CargoExe -Parent) 'rustc.exe'
    & $CargoExe build --release --locked --manifest-path (Join-Path $crate 'Cargo.toml') --target-dir (Join-Path $root 'dist/resources/cache/rust-recording-test')
    if($LASTEXITCODE){throw 'Generated recording source test compilation failed'}
    Copy-Item -LiteralPath (Join-Path $root 'dist/resources/cache/rust-recording-test/release/winone_recording.dll') -Destination (Join-Path $output 'WinOne.Recording.SyntheticTest.dll') -Force
}finally{$env:PATH=$previousPath;$env:LIB=$previousLib;$env:RUSTC=$previousRustc}
