param(
 [Parameter(Mandatory)][string]$Executable,
 [Parameter(Mandatory)][string]$OutputDirectory,
 [ValidateRange(0,120)][int]$IdleSeconds=65
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new probe directory'}
New-Item -ItemType Directory -Path $output | Out-Null
$pipe='WinOne-capture-standby-'+[guid]::NewGuid().ToString('N')
function Call-Capture([string]$Method){
 $client=[IO.Pipes.NamedPipeClientStream]::new('.', $pipe, [IO.Pipes.PipeDirection]::InOut)
 try{
  $client.Connect(3000)
  $bytes=[Text.Encoding]::UTF8.GetBytes((@{version=1;id='probe';method=$Method;params=@{language='zh-CN'}}|ConvertTo-Json -Compress))
  $header=[BitConverter]::GetBytes([uint32]$bytes.Length);$client.Write($header,0,4);$client.Write($bytes,0,$bytes.Length);$client.Flush()
  $offset=0;while($offset-lt 4){$count=$client.Read($header,$offset,4-$offset);if($count-le 0){throw 'Capture response ended'};$offset+=$count}
  $length=[BitConverter]::ToUInt32($header,0);if($length-gt 1MB){throw 'Capture response too large'}
  $response=New-Object byte[] $length;$offset=0;while($offset-lt $length){$count=$client.Read($response,$offset,$length-$offset);if($count-le 0){throw 'Capture response ended'};$offset+=$count}
  $value=[Text.Encoding]::UTF8.GetString($response)|ConvertFrom-Json
  if($value.error){throw $value.error.message};return $value.result
 }finally{$client.Dispose()}
}
$exe=(Resolve-Path -LiteralPath $Executable).Path
$start=[Diagnostics.ProcessStartInfo]::new($exe,'--pipe "'+$pipe+'" --data-dir "'+$output+'" --parent '+$PID+' --host-pipe "unused-isolated-host" --stay-ready')
$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardError=$true
$clock=[Diagnostics.Stopwatch]::StartNew();$process=[Diagnostics.Process]::Start($start);$errors=$process.StandardError.ReadToEndAsync()
try{
 $prepared=Call-Capture 'prepare';if(-not $prepared.prepared){throw 'Capture was not prepared'}
 $process.Refresh();$ready=$clock.Elapsed.TotalMilliseconds;$private=$process.PrivateMemorySize64;$working=$process.WorkingSet64;$cpu=$process.TotalProcessorTime.TotalSeconds
 Write-Output ('Capture ready: {0:n1} ms, private {1:n1} MiB; checking idle lifetime.' -f $ready,($private/1MB))
 $idle=[Diagnostics.Stopwatch]::StartNew();for($second=0;$second-lt $IdleSeconds;$second++){Start-Sleep -Seconds 1;if($process.HasExited){throw 'Standby capture exited during idle'}}
 $snapshot=Call-Capture 'snapshot';$process.Refresh();$idleCpu=$process.TotalProcessorTime.TotalSeconds-$cpu
 if($snapshot.busy -or $snapshot.pins -ne 0 -or $snapshot.lastCapture){throw 'Standby retained active capture state'}
 $null=Call-Capture 'shutdown';if(-not $process.WaitForExit(10000)){throw 'Standby capture did not stop'}
 $report=[ordered]@{ReadyMs=$ready;PrivateBytes=$private;WorkingSetBytes=$working;IdleSeconds=$idle.Elapsed.TotalSeconds;IdleCpuSeconds=$idleCpu;Pins=$snapshot.pins;Busy=$snapshot.busy;ExitCode=$process.ExitCode;ExecutableSHA256=(Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash;Diagnostics=$errors.Result}
 [IO.File]::WriteAllText((Join-Path $output 'standby.json'),($report|ConvertTo-Json -Depth 4),[Text.UTF8Encoding]::new($false));$report|ConvertTo-Json -Depth 4
}finally{if(-not $process.HasExited){$process.Kill();$process.WaitForExit()};$process.Dispose()}
