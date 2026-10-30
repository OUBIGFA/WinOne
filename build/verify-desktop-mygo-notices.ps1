param([Parameter(Mandatory)][string]$Executable,[string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',[string]$BundleDirectory='')
# Checks the MyGo window's notices against what it actually links: every
# Go module and the toolchain `go version -m` reports must have a recorded,
# unchanged license in third_party/desktop-mygo-notices.json. With a bundle
# directory, the inventory and its license files are copied there.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=Join-Path $root 'third_party/desktop-mygo-notices.json'
$catalog=Get-Content -LiteralPath $source -Encoding UTF8 -Raw | ConvertFrom-Json
$records=@{}
foreach($component in $catalog.components){
    $key=$component.kind+'|'+$component.name+'|'+$component.version
    if($records.ContainsKey($key) -or @($component.notices).Count -eq 0){throw "Invalid MyGo window notice record: $key"}
    $records[$key]=$component
    foreach($notice in $component.notices){
        $path=Join-Path (Join-Path $root 'third_party') $notice.file
        if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256){throw "MyGo window notice mismatch: $($notice.file)"}
    }
}
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$info=@(& $go version -m $Executable)
if($LASTEXITCODE -ne 0 -or $info.Count -eq 0){throw 'Cannot inspect the MyGo window dependencies'}
$toolchain=($info[0] -split '\s+')[-1]
if(-not $records.ContainsKey('toolchain|Go|'+$toolchain)){throw "Missing toolchain notice: $toolchain"}
foreach($line in $info){
    $parts=$line.Trim() -split '\s+'
    if($parts[0] -eq 'dep' -and $parts[1] -ne 'winone' -and -not $records.ContainsKey('go|'+$parts[1]+'|'+$parts[2])){throw "Missing MyGo window dependency notice: $line"}
}
# The icons come from the locked lucide-react package; its license must be
# the one recorded.
$lucide=Join-Path $root 'apps/desktop/frontend/node_modules/lucide-react/LICENSE'
if(Test-Path -LiteralPath $lucide){
    $recorded=@($catalog.components | Where-Object {$_.kind -eq 'asset' -and $_.name -eq 'lucide-react icons'})[0].notices[0].sha256
    if((Get-FileHash -LiteralPath $lucide -Algorithm SHA256).Hash.ToLowerInvariant() -ne $recorded){throw 'Lucide icon license changed'}
}
if($BundleDirectory){
    $licenses=Join-Path $BundleDirectory 'licenses'
    New-Item -ItemType Directory -Path $licenses -Force | Out-Null
    foreach($component in $catalog.components){foreach($notice in $component.notices){Copy-Item -LiteralPath (Join-Path (Join-Path $root 'third_party') $notice.file) -Destination $licenses -Force}}
    Copy-Item -LiteralPath $source -Destination (Join-Path $licenses 'desktop-mygo-notices.txt') -Force
}
Write-Output "MyGo window dependency notices verified: $($catalog.components.Count) records"
