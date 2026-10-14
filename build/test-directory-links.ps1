param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='go',
    [switch]$RunNative,
    [switch]$RequireSymbolicLinks
)
$ErrorActionPreference='Stop'
$OutputEncoding=[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'native-links.test.exe')
if($LASTEXITCODE -ne 0){throw 'Choose an ignored repository test directory'}
New-Item -ItemType Directory -Path $output -Force | Out-Null
$test=Join-Path $output 'native-links.test.exe'
Push-Location $root
try {
    & $GoExe test -c -o $test ./internal/directorylinks
    if($LASTEXITCODE -ne 0){throw 'Link integration tests failed to compile'}
    if(-not $RunNative){Write-Output $test;return}
    # All input data and history stay inside this invocation. Recycling moves
    # only these owned fixtures into the user's normal Recycle Bin.
    $start=[Diagnostics.ProcessStartInfo]::new($test,'-test.v -test.timeout=120s')
    $start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    $start.StandardOutputEncoding=[Text.UTF8Encoding]::new($false)
    $start.StandardErrorEncoding=[Text.UTF8Encoding]::new($false)
    $start.WorkingDirectory=$root
    $start.EnvironmentVariables['TEMP']=$output;$start.EnvironmentVariables['TMP']=$output
    $start.EnvironmentVariables['WINONE_RECYCLE_INTEGRATION']='1'
    $start.EnvironmentVariables['WINONE_LINKS_REQUIRE_SYMLINK_TEST']=if($RequireSymbolicLinks){'1'}else{'0'}
    $process=[Diagnostics.Process]::Start($start)
    $stdout=$process.StandardOutput.ReadToEndAsync();$stderr=$process.StandardError.ReadToEndAsync()
    while(-not $process.WaitForExit(1000)){}
    $code=$process.ExitCode
    [IO.File]::WriteAllText((Join-Path $output 'native-links.out.log'),$stdout.Result,[Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText((Join-Path $output 'native-links.err.log'),$stderr.Result,[Text.UTF8Encoding]::new($false))
    $stdout.Result
    if($code -ne 0){$stderr.Result;throw "Native link verification failed: $code"}
} finally {Pop-Location}
