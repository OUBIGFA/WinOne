param([string]$GoExe='go',[string]$BundleDirectory,[ValidateSet('transfer','peer')][string]$Runtime='transfer')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$catalogPath=Join-Path $root ('third_party/interconnect-'+$Runtime+'-notices.json')
$catalog=Get-Content -LiteralPath $catalogPath -Encoding UTF8 -Raw | ConvertFrom-Json
$records=@{}
foreach($component in $catalog.components){
 $key=$component.kind+'|'+$component.name+'|'+$component.version
 if($records.ContainsKey($key) -or @($component.notices).Count -eq 0){throw "Invalid transfer notice record: $key"}
 $records[$key]=$true
 foreach($notice in $component.notices){
  if((Get-FileHash -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256){throw "Transfer notice mismatch: $($notice.file)"}
  if($BundleDirectory -and (Get-FileHash -LiteralPath (Join-Path $BundleDirectory $notice.file) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256){throw "Packaged transfer notice mismatch: $($notice.file)"}
 }
}
$go=(Get-Command $GoExe -ErrorAction Stop).Source
Push-Location (Join-Path $root ('modules/interconnect/'+$Runtime))
try {
 $modules=@(& $go list -mod=readonly -deps -test -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}{{end}}' .)
 if($LASTEXITCODE -ne 0){throw 'Cannot inspect transfer dependencies'}
 foreach($module in ($modules | Sort-Object -Unique)){
  if(-not $module -or $module.StartsWith('winone/interconnect/'+$Runtime+'|')){continue}
  if(-not $records.ContainsKey('go|'+$module)){throw "Missing transfer notice: $module"}
 }
 $version=& $go env GOVERSION
 if($LASTEXITCODE -ne 0 -or -not $records.ContainsKey('toolchain|Go|'+$version)){throw 'Transfer Go toolchain notice mismatch'}
}finally{Pop-Location}
if($BundleDirectory){
 $binary=Join-Path $BundleDirectory ('shared/interconnect/WinOne.Interconnect.'+$Runtime+'.exe')
 $info=@(& $go version -m $binary)
 if($LASTEXITCODE -ne 0){throw 'Cannot inspect transfer executable'}
 foreach($line in $info){$parts=$line.Trim() -split '\s+';if($parts[0] -eq 'dep' -and -not $records.ContainsKey('go|'+$parts[1]+'|'+$parts[2])){throw "Unlicensed transfer binary dependency: $line"}}
 Copy-Item -LiteralPath $catalogPath -Destination (Join-Path $BundleDirectory ('licenses/interconnect-'+$Runtime+'-notices.txt')) -Force
}
Write-Output "Transfer dependencies and notices verified: $($catalog.components.Count) records"
