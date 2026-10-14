param(
    [string]$CargoExe = 'cargo',
    [string]$GoExe = 'go',
    [string]$OutputDirectory = '_temp/rust-desktop-build/output',
    [string]$TargetDirectory = 'dist/resources/cache/rust-desktop',
    [switch]$TestOnly,
    [switch]$HostIntegration,
    [switch]$NativeIntegration,
    [switch]$ComponentUI=$true,
    [switch]$FrontendAlreadyBuilt,
    [string]$UpdateRepository = ''
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
function Invoke-BuildTool([string]$Executable,[string[]]$Arguments,[string]$Failure) {
    $command=(Get-Command $Executable -ErrorAction Stop).Source
    $previous=$ErrorActionPreference
    try {
        # Native compilers use stderr for progress and warnings. Preserve that
        # output while judging the build by its actual exit code, including
        # when the caller redirects the script's output to a verification log.
        $ErrorActionPreference='Continue';$global:LASTEXITCODE=-1
        & $command @Arguments
        $code=$LASTEXITCODE
    } finally { $ErrorActionPreference=$previous }
    if($code -ne 0){throw $Failure}
}
function Resolve-WorkspacePath([string]$path) {
    if ([IO.Path]::IsPathRooted($path)) { return [IO.Path]::GetFullPath($path) }
    return [IO.Path]::GetFullPath((Join-Path $root $path))
}
$output = Resolve-WorkspacePath $OutputDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$target = Resolve-WorkspacePath $TargetDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target -ReusableCache
foreach ($directory in @($output, $target)) {
    & git -C $root check-ignore -q --no-index -- (Join-Path $directory 'build-output.probe')
    if ($LASTEXITCODE -ne 0) { throw 'Rust build directories must be Git-ignored' }
}
$manifest = Join-Path $root 'apps/desktop-rust/Cargo.toml'
$featureArguments=@()
if($ComponentUI){
    if(-not(Test-Path -LiteralPath (Join-Path $root 'apps/result-ui/assets/provider-bitmaps/manifest.json'))){throw 'Run build/build-provider-icons.ps1 before building settings UI'}
    # Build the existing UI source in place. Rust embeds this exact output; it
    # does not maintain another copy of the pages, styles or component library.
    if(-not $FrontendAlreadyBuilt){
        Invoke-BuildTool 'npm.cmd' @('run','build','--prefix',(Join-Path $root 'apps/desktop/frontend')) 'Existing component UI build failed'
    }
    $featureArguments=@('--features','component-ui')
}else{$featureArguments=@('--no-default-features')}
if ($UpdateRepository -and ($UpdateRepository -notmatch '^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?/[A-Za-z0-9_.-]{1,100}$' -or ($UpdateRepository.Split('/')[1] -in @('.', '..')))) { throw 'UpdateRepository must be an owner/repository name' }
$versionMatch = [regex]::Match([IO.File]::ReadAllText($manifest), '(?m)^version\s*=\s*"([^"]+)"')
$buildVersion = $versionMatch.Groups[1].Value
if ($buildVersion -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Invalid Rust package version' }
if (-not $TestOnly) { $GoExe = (Get-Command $GoExe -ErrorAction Stop).Source }
$savedPath = $env:PATH; $savedLib = $env:LIB; $savedRustc = $env:RUSTC
$savedResource=$env:WINONE_RUST_RESOURCE
try {
    if(-not $TestOnly){
        New-Item -ItemType Directory -Path $target -Force | Out-Null
        Push-Location $root
        try{
            $icon=Join-Path $target 'desktop.ico'
            $resource=Join-Path $target 'desktop-resource.syso'
            & $GoExe run ./build/icon -out $icon
            if($LASTEXITCODE -ne 0){throw 'Desktop icon generation failed'}
            & $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico $icon -o $resource
            if($LASTEXITCODE -ne 0){throw 'Desktop Windows resources failed'}
            $env:WINONE_RUST_RESOURCE=$resource
        }finally{Pop-Location}
    }
    # Keep local toolchain discovery out of the Cargo manifest and restore the caller.
    $cargoPath = (Get-Command $CargoExe -ErrorAction Stop).Source
    $rustc = Join-Path (Split-Path -Parent $cargoPath) 'rustc.exe'
    if (Test-Path -LiteralPath $rustc) { $env:RUSTC = $rustc }
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    if (Test-Path -LiteralPath $vswhere) {
        $installation = & $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        if ($installation) {
            $msvc = Get-ChildItem (Join-Path $installation 'VC/Tools/MSVC') -Directory | Sort-Object Name -Descending | Select-Object -First 1
            $sdk = Get-ChildItem (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/Lib') -Directory | Sort-Object Name -Descending | Select-Object -First 1
            if (-not $msvc -or -not $sdk) { throw 'Windows C++ tools and SDK are required' }
            $env:PATH = (Join-Path $msvc.FullName 'bin/Hostx64/x64') + ';' + $env:PATH
            $env:LIB = @((Join-Path $msvc.FullName 'lib/x64'), (Join-Path $sdk.FullName 'um/x64'), (Join-Path $sdk.FullName 'ucrt/x64'), $savedLib) -join ';'
        }
    }
    Invoke-BuildTool $CargoExe (@('test','--locked','--manifest-path',$manifest,'--target-dir',$target)+$featureArguments) 'Rust desktop tests failed'
    if ($HostIntegration) {
        & $CargoExe test --locked --manifest-path $manifest --target-dir $target @featureArguments --test host_roundtrip -- --ignored
        if ($LASTEXITCODE -ne 0) { throw 'Rust/Host integration failed' }
    }
    if ($NativeIntegration) {
        & $CargoExe test --locked --manifest-path $manifest --target-dir $target @featureArguments --bin winone-desktop native::tests:: -- --ignored --nocapture --test-threads=1 --skip native::tests::preview::
        if ($LASTEXITCODE -ne 0) { throw 'Rust native-control integration failed' }
    }
    if (-not $TestOnly) {
        Invoke-BuildTool $CargoExe (@('build','--release','--locked','--manifest-path',$manifest,'--target-dir',$target)+$featureArguments) 'Rust desktop build failed'
        New-Item -ItemType Directory -Path $output -Force | Out-Null
        # The native entrypoint and its startup helper share a versioned wire
        # contract. Always build the matching Host into this candidate folder.
        Push-Location $root
        try {
            & $GoExe build -trimpath -ldflags "-H windowsgui -X main.updateRepository=$UpdateRepository -X main.updateVersion=$buildVersion" -o (Join-Path $output 'WinOne.Host.exe') ./apps/host
            if ($LASTEXITCODE -ne 0) { throw 'Candidate Host build failed' }
        } finally { Pop-Location }
        Copy-Item -LiteralPath (Join-Path $target 'release/winone-desktop.exe') -Destination (Join-Path $output 'WinOne.exe') -Force
        Get-FileHash -LiteralPath (Join-Path $output 'WinOne.exe'),(Join-Path $output 'WinOne.Host.exe') -Algorithm SHA256
    }
} finally {
    $env:PATH = $savedPath; $env:LIB = $savedLib; $env:RUSTC = $savedRustc;$env:WINONE_RUST_RESOURCE=$savedResource
}
