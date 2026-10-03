param([string]$Downloader)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'sharex-capture-resources.ps1')
. (Join-Path $PSScriptRoot 'sharex-native-host.ps1')
. (Join-Path $PSScriptRoot 'sharex-native-presentation.ps1')
$source = (Get-Content -LiteralPath (Join-Path $root 'third_party/sources.json') -Raw -Encoding UTF8 | ConvertFrom-Json).sharex
$cache = Join-Path $root '_temp/dependencies'
$archive = Join-Path $cache 'ShareX-v21.0.0.zip'
$checkout = Join-Path $cache 'ShareX-21.0.0'
$marker = Join-Path $checkout '.winone-source-sha256'
function ApplyLocalEditorOptions([string]$directory) {
    Initialize-ShareXNativeHost $directory
    $form = Join-Path $directory 'ShareX.ScreenCaptureLib/Forms/RegionCaptureForm.cs'
    $formText = [IO.File]::ReadAllText($form)
    $updatedForm = $formText.Replace('title.AppendFormat("ShareX - {0}",', 'title.AppendFormat(ShareXResources.Name + " - {0}",')
    if (-not $updatedForm.Contains('TryCopyImageRequested')) {
        if (-not $updatedForm.Contains('public event Action<Bitmap> CopyImageRequested;')) { throw 'ShareX copy callback changed' }
        $updatedForm = $updatedForm.Replace('public event Action<Bitmap> CopyImageRequested;', "public event Action<Bitmap> CopyImageRequested;`n        public event Func<Bitmap, bool> TryCopyImageRequested;")
        $updatedForm = $updatedForm.Replace('if (CopyImageRequested != null)', 'if (CopyImageRequested != null || TryCopyImageRequested != null)')
        $updatedForm = $updatedForm.Replace('if (CopyImageRequested != null && bmp != null)', 'if ((CopyImageRequested != null || TryCopyImageRequested != null) && bmp != null)')
        $updatedForm = [regex]::Replace($updatedForm, '(?<![A-Za-z0-9_])CopyImageRequested\(bmp\);', 'if (TryCopyImageRequested != null) { if (!TryCopyImageRequested(bmp)) return; } else CopyImageRequested(bmp);')
    }
    if ($updatedForm -ne $formText) { [IO.File]::WriteAllText($form, $updatedForm, [Text.UTF8Encoding]::new($false)) }
    Update-ShareXCaptureResourceOwnership $form
    Update-ShareXNativePresentation $form
    Update-ShareXNativePointer $form
    Update-ShareXNativeResizePointer $form
    Update-ShareXNativeEventInput $directory
    Update-ShareXNativeSelectionOutline $form
    Update-ShareXNativeEdgeSnap (Join-Path $directory 'ShareX.ScreenCaptureLib/Shapes/ShapeManager.cs')
    $manager = Join-Path $directory 'ShareX.ScreenCaptureLib/ScrollingCaptureManager.cs'
    $managerText = [IO.File]::ReadAllText($manager)
    if ($managerText.Contains('internal class ScrollingCaptureManager')) {
        [IO.File]::WriteAllText($manager, $managerText.Replace('internal class ScrollingCaptureManager', 'public class ScrollingCaptureManager'), [Text.UTF8Encoding]::new($false))
    } elseif (-not $managerText.Contains('public class ScrollingCaptureManager')) { throw 'ShareX scrolling manager declaration changed' }
    $managerText = [IO.File]::ReadAllText($manager)
    if (-not $managerText.Contains('public void SelectTarget(')) {
        $anchor = 'public bool SelectWindow()'
        if (-not $managerText.Contains($anchor)) { throw 'ShareX scrolling selection entry changed' }
        $selection = @'
// WinOne supplies its own selection UI. Validate before replacing a target,
        // preserving the previous selection if a retry supplies invalid bounds.
        public void SelectTarget(WindowInfo window, Rectangle rectangle)
        {
            if (IsCapturing) throw new InvalidOperationException("Capture is running");
            if (window == null || window.Handle == IntPtr.Zero || !window.IsVisible || window.IsMinimized ||
                rectangle.Width <= 0 || rectangle.Height <= 0 || !CaptureHelpers.GetWindowRectangle(window.Handle).Contains(rectangle))
                throw new ArgumentException("Invalid scrolling target");
            selectedWindow = window;
            selectedRectangle = rectangle;
        }

        public bool SelectWindow()
'@
        [IO.File]::WriteAllText($manager, $managerText.Replace($anchor, $selection), [Text.UTF8Encoding]::new($false))
    }
    $managerText = [IO.File]::ReadAllText($manager)
    if (-not $managerText.Contains('public Bitmap TakeResult()')) {
        $anchor = 'public void Dispose()'
        if (-not $managerText.Contains($anchor)) { throw 'ShareX scrolling disposal entry changed' }
        $transfer = @'
// Transfer the completed bitmap without another full-size allocation.
        // The caller owns it; retry and manager disposal no longer release it.
        public Bitmap TakeResult()
        {
            if (IsCapturing) throw new InvalidOperationException("Capture is running");
            Bitmap result = Result;
            Result = null;
            return result;
        }

        public void Dispose()
'@
        [IO.File]::WriteAllText($manager, $managerText.Replace($anchor, $transfer), [Text.UTF8Encoding]::new($false))
    }
    $managerText = [IO.File]::ReadAllText($manager)
    if (-not $managerText.Contains('private void ValidateWinOneTarget()')) {
        foreach ($anchor in @('private Rectangle selectedRectangle;', 'selectedRectangle = rectangle;', 'await Task.Delay(Options.StartDelay);', 'lastScreenshot = screenshot.CaptureRectangle(selectedRectangle);', 'switch (Options.ScrollMethod)', 'public void StopCapture()')) {
            if (-not $managerText.Contains($anchor)) { throw "ShareX scrolling guard anchor changed: $anchor" }
        }
        $guard = @'
// Only explicitly selected WinOne targets opt into this guard. Stop before
        // reading old screen coordinates or sending input to a different window.
        private void ValidateWinOneTarget()
        {
            if (winOneTargetBounds.IsEmpty) return;
            if (selectedWindow == null || !selectedWindow.IsVisible || selectedWindow.IsMinimized ||
                CaptureHelpers.GetWindowRectangle(selectedWindow.Handle) != winOneTargetBounds ||
                !winOneTargetBounds.Contains(selectedRectangle) || (Options.ScrollMethod != ScrollMethod.ScrollMessage && NativeMethods.GetForegroundWindow() != selectedWindow.Handle))
                throw new InvalidOperationException("WinOne.ScrollingTargetChanged");
        }

        public void StopCapture()
'@
        $managerText = $managerText.Replace('private Rectangle selectedRectangle;', "private Rectangle selectedRectangle;`n        private Rectangle winOneTargetBounds;")
        $managerText = $managerText.Replace('selectedRectangle = rectangle;', "selectedRectangle = rectangle;`n            winOneTargetBounds = CaptureHelpers.GetWindowRectangle(window.Handle);")
        $managerText = $managerText.Replace('await Task.Delay(Options.StartDelay);', "await Task.Delay(Options.StartDelay);`n                    if (stopRequested) return status;`n                    ValidateWinOneTarget();")
        $managerText = $managerText.Replace('lastScreenshot = screenshot.CaptureRectangle(selectedRectangle);', "ValidateWinOneTarget();`n                        lastScreenshot = screenshot.CaptureRectangle(selectedRectangle);")
        $managerText = $managerText.Replace('switch (Options.ScrollMethod)', "ValidateWinOneTarget();`n                        switch (Options.ScrollMethod)")
        $managerText = $managerText.Replace('public void StopCapture()', $guard)
        [IO.File]::WriteAllText($manager, $managerText, [Text.UTF8Encoding]::new($false))
    }
    # Message-based scrolling addresses an HWND; only global mouse/key input
    # requires foreground ownership. Upgrade previously prepared local sources.
    $managerText = [IO.File]::ReadAllText($manager)
    $oldFocusGuard = '!winOneTargetBounds.Contains(selectedRectangle) || NativeMethods.GetForegroundWindow() != selectedWindow.Handle)'
    $newFocusGuard = '!winOneTargetBounds.Contains(selectedRectangle) || (Options.ScrollMethod != ScrollMethod.ScrollMessage && NativeMethods.GetForegroundWindow() != selectedWindow.Handle))'
    if ($managerText.Contains($oldFocusGuard)) { [IO.File]::WriteAllText($manager, $managerText.Replace($oldFocusGuard,$newFocusGuard), [Text.UTF8Encoding]::new($false)) }
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
