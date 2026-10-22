param(
    [Parameter(Mandatory)][string]$InputFile,
    [Parameter(Mandatory)][string]$ProbeExe,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$FfmpegExe='ffmpeg',
    [string]$FfprobeExe='ffprobe',
    [ValidateRange(1,60)][int]$Fps=30
)
# Test-only sample comparison. FFmpeg decodes the explicitly selected file to
# a binary pipe; the production Rust/MF encoder writes the candidate. No screen
# capture, sequence frames, product FFmpeg dependency or source-file changes.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new Git-ignored sample comparison directory'}
$inputPath=(Resolve-Path -LiteralPath $InputFile).Path
$probe=(Resolve-Path -LiteralPath $ProbeExe).Path
$ffmpeg=(Get-Command $FfmpegExe -ErrorAction Stop).Source
$ffprobe=(Get-Command $FfprobeExe -ErrorAction Stop).Source
$info=(& $ffprobe -v error -select_streams v:0 -show_entries stream=width,height -show_entries format=duration,size -of json $inputPath) -join "`n" | ConvertFrom-Json
if($LASTEXITCODE -ne 0 -or -not $info.streams.Count){throw 'Cannot read the selected sample'}
$width=$info.streams[0].width;$height=$info.streams[0].height
if($width % 2 -or $height % 2){throw 'Comparison requires even dimensions; do not silently crop a sample'}
New-Item -ItemType Directory -Path $output|Out-Null
$target=Join-Path $output 'encoded.mp4'
$frames=(& $ffprobe -v error -select_streams v:0 -show_frames -show_entries frame=best_effort_timestamp_time -of json $inputPath) -join "`n" | ConvertFrom-Json
if($LASTEXITCODE -ne 0 -or -not $frames.frames.Count){throw 'Cannot read source frame timing'}
$first=[double]$frames.frames[0].best_effort_timestamp_time
$times=@{durationSeconds=[double]$info.format.duration;ptsSeconds=@($frames.frames|ForEach-Object {[double]$_.best_effort_timestamp_time-$first})}
$timeline=Join-Path $output 'source-timestamps.json'
[IO.File]::WriteAllText($timeline,($times|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
function Start-SampleProcess([string]$exe,[string]$arguments,[bool]$stdin){
    $start=New-Object Diagnostics.ProcessStartInfo
    $start.FileName=$exe;$start.Arguments=$arguments;$start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true;$start.RedirectStandardInput=$stdin
    $process=New-Object Diagnostics.Process;$process.StartInfo=$start
    if(-not $process.Start()){throw 'Cannot start owned sample process'}
    return $process
}
$decode=$null;$encode=$null
try{
    $encode=Start-SampleProcess $probe ('"'+$target+'" '+$width+' '+$height+' '+$Fps+' "'+$timeline+'"') $true
    $encodedOut=$encode.StandardOutput.ReadToEndAsync();$encodedError=$encode.StandardError.ReadToEndAsync()
    $decode=Start-SampleProcess $ffmpeg ('-v error -nostdin -i "'+$inputPath+'" -an -fps_mode passthrough -pix_fmt bgra -f rawvideo pipe:1') $false
    $decodeError=$decode.StandardError.ReadToEndAsync()
    $copy=$decode.StandardOutput.BaseStream.CopyToAsync($encode.StandardInput.BaseStream)
    if(-not $copy.Wait(180000)){throw 'Sample transcode timed out'}
    $encode.StandardInput.Close()
    if(-not $decode.WaitForExit(10000) -or -not $encode.WaitForExit(60000)){throw 'Sample finalization timed out'}
    [IO.File]::WriteAllText((Join-Path $output 'decode.log'),$decodeError.Result)
    [IO.File]::WriteAllText((Join-Path $output 'encode.log'),$encodedError.Result)
    [IO.File]::WriteAllText((Join-Path $output 'encoder-result.json'),$encodedOut.Result)
    if($decode.ExitCode -ne 0 -or $encode.ExitCode -ne 0){throw 'Sample decode/production encode failed; inspect logs'}
}finally{
    foreach($process in @($decode,$encode)){if($null -ne $process){if(-not $process.HasExited){$process.Kill();$process.WaitForExit()};$process.Dispose()}}
}
$prior=$ErrorActionPreference
try{
    $ErrorActionPreference='Continue'
    $quality=& $ffmpeg -hide_banner -nostdin -i $inputPath -i $target -lavfi "[0:v]setpts=PTS-STARTPTS[reference];[1:v]setpts=PTS-STARTPTS[candidate];[candidate][reference]ssim" -an -f null NUL 2>&1 | ForEach-Object {$_.ToString()}
    $code=$LASTEXITCODE
}finally{$ErrorActionPreference=$prior}
[IO.File]::WriteAllText((Join-Path $output 'ssim.log'),($quality -join "`n"))
if($code -ne 0){throw 'Sample quality comparison failed'}
$match=[regex]::Match(($quality -join "`n"),'All:([0-9.]+)')
if(-not $match.Success){throw 'SSIM result missing'}
$bytes=(Get-Item -LiteralPath $target).Length
$report=[ordered]@{input=$inputPath;inputBytes=[long]$info.format.size;inputSeconds=[double]$info.format.duration;width=$width;height=$height;fps=$Fps;sourceFrames=$frames.frames.Count;output=$target;bytes=$bytes;ssim=[double]$match.Groups[1].Value;relativeBytes=$bytes/[double]$info.format.size}
[IO.File]::WriteAllText((Join-Path $output 'comparison.json'),($report|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$report|ConvertTo-Json
