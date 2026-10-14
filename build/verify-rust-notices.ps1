param(
    [Parameter(Mandatory)][string]$CargoExe,
    [switch]$Update,
    [string]$WebView2Package,
    [string]$BundleDirectory
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$catalogPath=Join-Path $root 'third_party/rust-notices.json'
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source
$rustc=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
$previousRustc=$env:RUSTC
function Hash([string]$path){(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()}
function Notice([string]$source,[string]$sourceName,[string]$file){
    $target=Join-Path $root ('third_party/'+$file)
    $hash=Hash $source
    if($Update){Copy-Item -LiteralPath $source -Destination $target -Force}
    if((Hash $target) -ne $hash){throw "Rust notice changed: $file"}
    [ordered]@{sourceFile=$sourceName;file=$file;sha256=$hash}
}
try{
    $env:RUSTC=$rustc
    $metadataText=& $cargo metadata --locked --format-version 1 --filter-platform x86_64-pc-windows-msvc --manifest-path (Join-Path $root 'apps/desktop-rust/Cargo.toml')
    if($LASTEXITCODE -ne 0){throw 'Cannot resolve locked Rust dependencies'}
    $metadata=$metadataText|ConvertFrom-Json
    $packages=@()
    foreach($package in $metadata.packages|Where-Object source|Sort-Object name,version){
        $directory=Split-Path $package.manifest_path -Parent
        $licenses=@(Get-ChildItem -LiteralPath $directory -File|Where-Object Name -Match '^(LICENSE|LICENCE|COPYING|NOTICE)([._-].*)?$'|Sort-Object Name)
        $notices=@();$upstream=$null
        if($licenses.Count -eq 0 -and $package.name -like 'webview2-com*'){
            # These published crates omit the workspace license. The copied
            # notice is bound to the exact revision recorded inside the crate.
            $revision=(Get-Content -LiteralPath (Join-Path $directory '.cargo_vcs_info.json') -Encoding UTF8 -Raw|ConvertFrom-Json).git.sha1
            $file='licenses/rust-webview2-'+$revision.Substring(0,7)+'-LICENSE.txt'
            $notices+=[ordered]@{sourceFile='LICENSE';file=$file;sha256=(Hash (Join-Path $root ('third_party/'+$file)))}
            $upstream='https://raw.githubusercontent.com/wravery/webview2-rs/'+$revision+'/LICENSE'
        }else{
            foreach($license in $licenses){
                $filename=('rust-'+$package.name+'-'+$package.version+'-'+$license.Name+'.txt') -replace '[^A-Za-z0-9._-]','-'
                $notices+=Notice $license.FullName $license.Name ('licenses/'+$filename)
            }
        }
        if($notices.Count -eq 0){throw "Missing Rust license source: $($package.name)"}
        $record=[ordered]@{name=$package.name;version=$package.version;license=$package.license;notices=$notices}
        if($upstream){$record.upstream=$upstream}
        $packages+=$record
    }
    $versionText=& $rustc --version
    if($LASTEXITCODE -ne 0 -or $versionText -notmatch '^rustc ([^ ]+) '){throw 'Cannot determine Rust toolchain version'}
    $version=$Matches[1]
    $toolchain=Split-Path (Split-Path $rustc -Parent) -Parent
    $standard=Notice (Join-Path $toolchain 'share/doc/rust/COPYRIGHT-library.html') 'share/doc/rust/COPYRIGHT-library.html' ('licenses/rust-standard-library-'+$version+'-COPYRIGHT.txt')
    $binding=$metadata.packages|Where-Object name -eq 'webview2-com-sys'
    $bindingDirectory=Split-Path $binding.manifest_path -Parent
    $sdkVersion=[Diagnostics.FileVersionInfo]::GetVersionInfo((Join-Path $bindingDirectory 'x64/WebView2Loader.dll')).FileVersion
    $sdkNoticeFile='licenses/microsoft-webview2-sdk-'+$sdkVersion+'-LICENSE.txt'
    $binaries=@()
    if($Update){
        if(-not $WebView2Package){throw 'Updating notices requires the matching Microsoft WebView2 NuGet package'}
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $archive=[IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $WebView2Package).Path)
        try{
            foreach($name in @('WebView2Loader.dll','WebView2LoaderStatic.lib')){
                $entry=$archive.GetEntry('build/native/x64/'+$name)
                if(-not $entry){throw 'NuGet loader binary is missing'}
                $stream=$entry.Open();$sha=[Security.Cryptography.SHA256]::Create()
                try{$expected=([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-','').ToLowerInvariant()}finally{$stream.Dispose();$sha.Dispose()}
                if((Hash (Join-Path $bindingDirectory ('x64/'+$name))) -ne $expected){throw 'Crate loader does not match the Microsoft SDK package'}
            }
            $license=$archive.GetEntry('LICENSE.txt');if(-not $license){throw 'Microsoft SDK license missing'}
            $input=$license.Open();$output=[IO.File]::Create((Join-Path $root ('third_party/'+$sdkNoticeFile)))
            try{$input.CopyTo($output)}finally{$input.Dispose();$output.Dispose()}
        }finally{$archive.Dispose()}
    }
    foreach($name in @('WebView2Loader.dll','WebView2LoaderStatic.lib')){$binaries+=[ordered]@{file=('x64/'+$name);sha256=(Hash (Join-Path $bindingDirectory ('x64/'+$name)))}}
    $sdk=[ordered]@{name='Microsoft.Web.WebView2';version=$sdkVersion;source=('https://api.nuget.org/v3-flatcontainer/microsoft.web.webview2/'+$sdkVersion+'/microsoft.web.webview2.'+$sdkVersion+'.nupkg');binaries=$binaries;notices=@([ordered]@{sourceFile='LICENSE.txt';file=$sdkNoticeFile;sha256=(Hash (Join-Path $root ('third_party/'+$sdkNoticeFile)))})}
    $result=[ordered]@{schemaVersion=1;target='x86_64-pc-windows-msvc';toolchain=[ordered]@{name='Rust';version=$version;notices=@($standard)};packages=$packages;sdk=$sdk}
    $json=$result|ConvertTo-Json -Depth 12
    if($Update){[IO.File]::WriteAllText($catalogPath,$json,[Text.UTF8Encoding]::new($false))}
    else{
        $recorded=Get-Content -LiteralPath $catalogPath -Raw -Encoding UTF8|ConvertFrom-Json
        # Property order is preserved by conversion; compare identities and all
        # notice/binary digests, not just the package count.
        if(($recorded|ConvertTo-Json -Depth 12 -Compress) -ne ($result|ConvertTo-Json -Depth 12 -Compress)){throw 'Rust notice catalog differs from the locked Windows build'}
    }
    if($BundleDirectory){
        foreach($record in @($result.toolchain)+@($packages)+@($sdk)){foreach($notice in $record.notices){
            $destination=Join-Path $BundleDirectory $notice.file
            if((Hash $destination) -ne $notice.sha256){throw "Published Rust notice mismatch: $($notice.file)"}
        }}
        [IO.File]::WriteAllText((Join-Path $BundleDirectory 'licenses/rust-notices.txt'),$json,[Text.UTF8Encoding]::new($false))
    }
    'Rust notices verified: '+$packages.Count+' packages, standard library and matching Microsoft loader'
}finally{$env:RUSTC=$previousRustc}
