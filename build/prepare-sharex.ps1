param([string]$Downloader)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$source = (Get-Content -LiteralPath (Join-Path $root 'third_party/sources.json') -Raw -Encoding UTF8 | ConvertFrom-Json).sharex
$cache = Join-Path $root '_temp/dependencies'
$archive = Join-Path $cache 'ShareX-v21.0.0.zip'
$checkout = Join-Path $cache 'ShareX-21.0.0'
$marker = Join-Path $checkout '.winone-source-sha256'
function ApplyLocalEditorOptions([string]$directory) {
    $form = Join-Path $directory 'ShareX.ScreenCaptureLib/Forms/RegionCaptureForm.cs'
    $formText = [IO.File]::ReadAllText($form)
    $updatedForm = $formText.Replace('title.AppendFormat("ShareX - {0}",', 'title.AppendFormat(ShareXResources.Name + " - {0}",')
    if ($updatedForm -ne $formText) { [IO.File]::WriteAllText($form, $updatedForm, [Text.UTF8Encoding]::new($false)) }
    $manager = Join-Path $directory 'ShareX.ScreenCaptureLib/ScrollingCaptureManager.cs'
    $managerText = [IO.File]::ReadAllText($manager)
    if ($managerText.Contains('internal class ScrollingCaptureManager')) {
        [IO.File]::WriteAllText($manager, $managerText.Replace('internal class ScrollingCaptureManager', 'public class ScrollingCaptureManager'), [Text.UTF8Encoding]::new($false))
    } elseif (-not $managerText.Contains('public class ScrollingCaptureManager')) { throw 'ShareX scrolling manager declaration changed' }
    $options = Join-Path $directory 'ShareX.ScreenCaptureLib/RegionCaptureOptions.cs'
    $menu = Join-Path $directory 'ShareX.ScreenCaptureLib/Shapes/ShapeManagerMenu.cs'
    $text = [IO.File]::ReadAllText($options)
    if (-not $text.Contains('public bool EnableSharing')) {
        if (-not $text.Contains('public bool EditorAutoCopyImage = false;')) { throw 'ShareX editor options changed' }
        $text = $text.Replace('public bool EditorAutoCopyImage = false;', "public bool EditorAutoCopyImage = false;`n        public bool EnableSharing = true;")
        [IO.File]::WriteAllText($options, $text, [Text.UTF8Encoding]::new($false))
    }
    $text = [IO.File]::ReadAllText($menu)
    if (-not $text.Contains('if (Options.EnableSharing)')) {
        if (-not $text.Contains('ToolStripButton tsbUploadImage =') -or -not $text.Contains('tsMain.Items.Add(tsbPrintImage);')) { throw 'ShareX editor toolbar changed' }
        $text = $text.Replace('ToolStripButton tsbUploadImage =', "if (Options.EnableSharing)`n                {`n                ToolStripButton tsbUploadImage =")
        $text = $text.Replace('tsMain.Items.Add(tsbPrintImage);', "tsMain.Items.Add(tsbPrintImage);`n                }")
        [IO.File]::WriteAllText($menu, $text, [Text.UTF8Encoding]::new($false))
    }
}
if ((Test-Path -LiteralPath $marker) -and ((Get-Content -LiteralPath $marker -Raw).Trim() -eq $source.archiveSha256)) { ApplyLocalEditorOptions $checkout; return $checkout }
New-Item -ItemType Directory -Path $cache -Force | Out-Null
if (-not (Test-Path -LiteralPath $archive)) {
    if (-not $Downloader) { $command = Get-Command aria2-next -ErrorAction SilentlyContinue; if ($command) { $Downloader = $command.Source } }
    if ($Downloader) {
        & $Downloader -d $cache -o 'ShareX-v21.0.0.zip' --auto-file-renaming=false --allow-overwrite=true $source.archive | Out-Host
        if ($LASTEXITCODE -ne 0) { throw 'ShareX download failed' }
    } else {
        Add-Type -AssemblyName System.Net.Http
        $client = [Net.Http.HttpClient]::new()
        try { [IO.File]::WriteAllBytes($archive, $client.GetByteArrayAsync($source.archive).GetAwaiter().GetResult()) } finally { $client.Dispose() }
    }
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $source.archiveSha256) { throw 'ShareX archive checksum mismatch; preserve and inspect the download' }
if (Test-Path -LiteralPath $checkout) { throw 'Unverified ShareX checkout already exists; inspect it before replacing' }
Add-Type -AssemblyName System.IO.Compression.FileSystem
[IO.Compression.ZipFile]::ExtractToDirectory($archive, $cache)
[IO.File]::WriteAllText($marker, $source.archiveSha256, [Text.UTF8Encoding]::new($false))
ApplyLocalEditorOptions $checkout
return $checkout
