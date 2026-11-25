param([Parameter(Mandatory)][string]$CaptureDirectory,[string]$Report,[string]$PythonExe='python')
$ErrorActionPreference='Stop'
$arguments=@((Join-Path $PSScriptRoot 'verify-capture-native.py'),$CaptureDirectory)
if($Report){$arguments+=@('--report',$Report)}
$prior=$env:PYTHONIOENCODING
try{$env:PYTHONIOENCODING='utf-8';& $PythonExe @arguments;if($LASTEXITCODE){throw 'Native capture runtime verification failed'}}finally{$env:PYTHONIOENCODING=$prior}
