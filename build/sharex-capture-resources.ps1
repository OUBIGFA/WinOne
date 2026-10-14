function Update-ShareXCaptureResourceOwnership([string]$Path) {
    $text = [IO.File]::ReadAllText($Path)
    $original = $text
    if ($text.Contains('private Bitmap captureBackground;')) {
        # The native host replaces the direct draw after ownership is prepared.
        # Both supported draw paths still use the same owned background fallback.
        $ownedDraw = $text.Contains('g.DrawImage(captureBackground, CanvasRectangle);') -or
            $text.Contains('g.DrawImage(WinOnePresentation && ShapeManager.WinOneOrderedEffects ? ShapeManager.WinOneComposition() : captureBackground, CanvasRectangle);')
        if ($text.Contains('g.DrawImage(backgroundBrush.Image, CanvasRectangle);') -or
            -not $ownedDraw -or
            [regex]::Matches($text, [regex]::Escape('captureBackground?.Dispose();')).Count -ne 2) { throw 'Incomplete background ownership change' }
    } else {
    # TextureBrush.Image materializes an Image. Keep one owned background
    # instead, preserving DrawImage's zoom/interpolation behavior.
    $changes = [ordered]@{
        'private TextureBrush backgroundBrush, backgroundHighlightBrush;' = "private TextureBrush backgroundHighlightBrush;`n        // The form owns this image until background replacement or disposal.`n        private Bitmap captureBackground;"
        'backgroundBrush?.Dispose();' = 'captureBackground?.Dispose();'
        'backgroundBrush = new TextureBrush(background) { WrapMode = WrapMode.Clamp };' = 'captureBackground = (Bitmap)background.Clone();'
        'backgroundBrush.TranslateTransform(CanvasRectangle.X, CanvasRectangle.Y);' = ''
        'backgroundBrush = new TextureBrush(DimmedCanvas) { WrapMode = WrapMode.Clamp };' = 'captureBackground = (Bitmap)DimmedCanvas.Clone();'
        'backgroundBrush = new TextureBrush(Canvas) { WrapMode = WrapMode.Clamp };' = 'captureBackground = (Bitmap)Canvas.Clone();'
        'g.DrawImage(backgroundBrush.Image, CanvasRectangle);' = 'g.DrawImage(captureBackground, CanvasRectangle);'
    }
    foreach ($old in $changes.Keys) {
        if (-not $text.Contains($old)) { throw "ShareX background implementation changed: $old" }
        $text = $text.Replace($old, $changes[$old])
    }
    $pan = '            if \(backgroundBrush != null\)\r?\n            \{\r?\n                backgroundBrush.TranslateTransform\(deltaX, deltaY\);\r?\n            \}'
    if ([regex]::Matches($text, $pan).Count -ne 1) { throw 'ShareX background pan implementation changed' }
    $text = [regex]::Replace($text, $pan, '')
    }
    if (-not $text.Contains('private Icon captureWindowIcon;')) {
        # ShareXResources.Icon returns a clone. Form does not own icons assigned
        # by callers, so release that clone only after its window is destroyed.
        foreach ($anchor in @('private Bitmap captureBackground;', 'Icon = ShareXResources.Icon;', 'base.Dispose(disposing);')) {
            if (-not $text.Contains($anchor)) { throw "ShareX icon lifetime changed: $anchor" }
        }
        $text = $text.Replace('private Bitmap captureBackground;', "private Bitmap captureBackground;`n        private Icon captureWindowIcon;")
        $text = $text.Replace('Icon = ShareXResources.Icon;', 'Icon = captureWindowIcon = ShareXResources.Icon;')
        $text = $text.Replace('base.Dispose(disposing);', 'try { base.Dispose(disposing); } finally { captureWindowIcon?.Dispose(); }')
    } elseif (-not $text.Contains('Icon = captureWindowIcon = ShareXResources.Icon;') -or
        -not $text.Contains('finally { captureWindowIcon?.Dispose(); }')) {
        throw 'Incomplete icon ownership change'
    }
    if ($text -ne $original) { [IO.File]::WriteAllText($Path, $text, [Text.UTF8Encoding]::new($false)) }
}
