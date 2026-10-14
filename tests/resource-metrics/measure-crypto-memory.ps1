param(
    [string]$GoExe='go',
    [Parameter(Mandatory)][string]$OutputDirectory,
    [ValidateRange(1000,10000)][int]$SampleAfterMilliseconds=3000,
    [switch]$RunSystemChecks
)
$ErrorActionPreference='Stop'
if(-not $RunSystemChecks){throw 'Use -RunSystemChecks to build and launch the owned memory probe'}
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot '../../build/assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored output directory'}
$go=(Get-Command $GoExe -ErrorAction Stop).Source
New-Item -ItemType Directory -Path $output|Out-Null
$exe=Join-Path $output 'WinOne.CryptoMemory.Probe.exe'
$build=[Diagnostics.ProcessStartInfo]::new($go,('build -trimpath -o "'+$exe+'" "'+(Join-Path $PSScriptRoot 'crypto-memory/main.go')+'"'))
$build.UseShellExecute=$false;$build.CreateNoWindow=$true
$build.RedirectStandardOutput=$true;$build.RedirectStandardError=$true
# This probe compares the selected unmodified standard library, independently
# of the repository's minimum Go version, module graph or local build overlays.
foreach($entry in @{GOTOOLCHAIN='local';GOWORK='off';GO111MODULE='off';GOFLAGS='';GOFIPS140='off';CGO_ENABLED='0'}.GetEnumerator()){$build.EnvironmentVariables[$entry.Key]=$entry.Value}
$compiler=[Diagnostics.Process]::Start($build)
try {
    $stdout=$compiler.StandardOutput.ReadToEndAsync();$stderr=$compiler.StandardError.ReadToEndAsync()
    if(-not $compiler.WaitForExit(120000)){throw 'Diagnostic build timed out'}
    [IO.File]::WriteAllText((Join-Path $output 'build.log'),$stdout.Result+$stderr.Result,[Text.UTF8Encoding]::new($false))
    if($compiler.ExitCode -ne 0){throw 'Diagnostic build failed; see build.log'}
} finally {if(-not $compiler.HasExited){$compiler.Kill();$compiler.WaitForExit()};$compiler.Dispose()}
$rows=[Collections.Generic.List[object]]::new()
foreach($round in 1,2){
    foreach($mode in @('off','on')){
        $start=[Diagnostics.ProcessStartInfo]::new($exe)
        $start.UseShellExecute=$false;$start.CreateNoWindow=$true
        $start.RedirectStandardInput=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
        $start.EnvironmentVariables['GODEBUG']="fips140=$mode"
        $process=[Diagnostics.Process]::Start($start)
        try {
            $ready=$process.StandardOutput.ReadLineAsync();$stderrTask=$process.StandardError.ReadToEndAsync()
            if(-not $ready.Wait(15000) -or -not $ready.Result){throw 'Diagnostic did not report readiness'}
            $state=$ready.Result|ConvertFrom-Json
            if(-not $state.ready -or $state.fipsEnabled -ne ($mode -eq 'on')){throw 'Unexpected FIPS mode'}
            Start-Sleep -Milliseconds $SampleAfterMilliseconds
            $process.Refresh()
            if($process.HasExited){throw 'Diagnostic exited before measurement'}
            $rows.Add([ordered]@{round=$round;fips=$mode;pid=$process.Id;createdUtc=$process.StartTime.ToUniversalTime().ToString('o');privateCommitBytes=$process.PrivateMemorySize64;workingSetBytes=$process.WorkingSet64})
            $process.StandardInput.Close()
            if(-not $process.WaitForExit(15000)){throw 'Diagnostic did not exit on EOF'}
            if($process.ExitCode -ne 0){throw $stderrTask.Result}
        } finally {if(-not $process.HasExited){$process.Kill();$process.WaitForExit()};$process.Dispose()}
    }
}
$report=[ordered]@{schemaVersion=1;toolchain=$state.goVersion;executableSHA256=(Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLowerInvariant();sampleAfterMilliseconds=$SampleAfterMilliseconds;scope='Repeated runs of one newly built executable; not an OS cold-cache benchmark or a cryptographic validation';samples=$rows}
[IO.File]::WriteAllText((Join-Path $output 'result.json'),($report|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
$rows|ForEach-Object {[pscustomobject]@{round=$_.round;fips=$_.fips;privateCommitMiB=$_.privateCommitBytes/1MB;workingSetMiB=$_.workingSetBytes/1MB}}
