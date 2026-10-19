param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$CargoExe='cargo',
    [string]$TargetDirectory='dist/resources/cache/rust-recording',
    [switch]$CheckOnly
)
$ErrorActionPreference='Stop'
# Windows PowerShell treats compiler progress on stderr as NativeCommandError
# when a parent captures the log. Preserve that output and fail on exit code.
function Invoke-RecordingTool([string]$Executable,[string[]]$Arguments,[string]$Failure){
    $previous=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & $Executable @Arguments 2>&1 | ForEach-Object { $_.ToString() }
        $code=$LASTEXITCODE
    } finally { $ErrorActionPreference=$previous }
    if($code -ne 0){throw "$Failure (exit $code)"}
}
$root=Split-Path -Parent $PSScriptRoot
function Absolute([string]$p){if([IO.Path]::IsPathRooted($p)){[IO.Path]::GetFullPath($p)}else{[IO.Path]::GetFullPath((Join-Path $root $p))}}
$output=Absolute $OutputDirectory;$target=Absolute $TargetDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target -ReusableCache
foreach($p in @($output,$target)){& git -C $root check-ignore -q --no-index -- (Join-Path $p 'probe');if($LASTEXITCODE -ne 0){throw 'Recording outputs must be ignored'}}
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source
$priorPath=$env:PATH;$priorLib=$env:LIB;$priorRustc=$env:RUSTC
try {
    $env:RUSTC=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
    $vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    if(Test-Path -LiteralPath $vswhere){
        $installation=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        if($installation){
            $msvc=Get-ChildItem (Join-Path $installation 'VC/Tools/MSVC') -Directory|Sort-Object Name -Descending|Select-Object -First 1
            $sdk=Get-ChildItem (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/Lib') -Directory|Sort-Object Name -Descending|Select-Object -First 1
            if(-not $msvc -or -not $sdk){throw 'Windows C++ tools and SDK are required'}
            $env:PATH=(Join-Path $msvc.FullName 'bin/Hostx64/x64')+';'+$priorPath
            $env:LIB=@((Join-Path $msvc.FullName 'lib/x64'),(Join-Path $sdk.FullName 'um/x64'),(Join-Path $sdk.FullName 'ucrt/x64'),$priorLib)-join ';'
        }
    }
    New-Item -ItemType Directory -Path $output,$target -Force|Out-Null
    $manifest=Join-Path $root 'modules/recording/Cargo.toml'
    if($CheckOnly){
        Invoke-RecordingTool $cargo @('check','--locked','--all-targets','--manifest-path',$manifest,'--target-dir',$target) 'Recording compile check failed'
    }else{
        & (Join-Path $PSScriptRoot 'verify-diagram-notices.ps1') -Runtime recording -CargoExe $cargo
        Invoke-RecordingTool $cargo @('test','--release','--locked','--manifest-path',$manifest,'--target-dir',$target) 'Recording tests failed'
        Invoke-RecordingTool $cargo @('build','--release','--locked','--manifest-path',$manifest,'--target-dir',$target,'--examples') 'Recording probe build failed'
        Invoke-RecordingTool $cargo @('build','--release','--locked','--manifest-path',$manifest,'--target-dir',$target) 'Recording library build failed'
        Copy-Item -LiteralPath (Join-Path $target 'release/winone_recording.dll') -Destination (Join-Path $output 'WinOne.Recording.dll') -Force
        $runtime=@{schemaVersion=1;version='0.1.0';capture='windows-capture 2.0.1';gif='gifski 1.34.0';sha256=(Get-FileHash -LiteralPath (Join-Path $output 'WinOne.Recording.dll') -Algorithm SHA256).Hash.ToLowerInvariant()}
        [IO.File]::WriteAllText((Join-Path $output 'recording-runtime.json'),($runtime|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    }
}finally{$env:PATH=$priorPath;$env:LIB=$priorLib;$env:RUSTC=$priorRustc}
