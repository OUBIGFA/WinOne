function Initialize-ShareXNativeHost([string]$SourceDirectory) {
    $project = [IO.Path]::GetFullPath((Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/ShareX.ScreenCaptureLib.csproj'))
    $targets = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../modules/capture/NativeUI/ShareXHost.targets'))
    if (!(Test-Path -LiteralPath $project -PathType Leaf) -or !(Test-Path -LiteralPath $targets -PathType Leaf)) { throw 'Native capture build inputs are missing' }
    # Only ShareX's separate OCR app calls this WPF scaler. WinOne uses its
    # own OCR pipeline; dropping the unused helper keeps the capture graph
    # on the Windows Forms runtime profile without changing rendered pixels.
    $images=Join-Path $SourceDirectory 'ShareX.HelpersLib/Helpers/ImageHelpers.cs'
    $imagesText=[IO.File]::ReadAllText($images)
    if($imagesText.Contains('using System.Windows.Media.Imaging;')) {
        $begin=$imagesText.IndexOf('        public static Bitmap ScaleImageFast(Bitmap bmp, double scale)')
        if($begin -lt 0){throw 'ShareX unused WPF scaler entry changed'}
        $end=$imagesText.IndexOf('        public static Bitmap ResizeImage(Bitmap bmp, Size size, bool allowEnlarge', $begin)
        if($end -le $begin){throw 'ShareX unused WPF scaler boundary changed'}
        $imagesText=$imagesText.Remove($begin,$end-$begin).Replace('using System.Windows.Media.Imaging;','')
        [IO.File]::WriteAllText($images,$imagesText,[Text.UTF8Encoding]::new($false))
    }
    $helpersProject=Join-Path $SourceDirectory 'ShareX.HelpersLib/ShareX.HelpersLib.csproj'
    $helpersText=[IO.File]::ReadAllText($helpersProject)
    [IO.File]::WriteAllText($helpersProject,$helpersText.Replace('<UseWPF>true</UseWPF>','<UseWPF>false</UseWPF>'),[Text.UTF8Encoding]::new($false))
    $formPath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Forms/RegionCaptureForm.cs'
    $formText=[IO.File]::ReadAllText($formPath)
    # Older local builds drew the preview after selection handles. Migrate the
    # cached source to draw pixels before tools, so crop handles stay usable.
    $formText=$formText.Replace("DrawShapes(g);`n            if (WinOnePresentation) DrawWinOneTranslation(g);",'DrawShapes(g);')
    if (-not $formText.Contains('DrawWinOneTranslation(g);')) {
        $anchor='// Draw tools'
        if (($formText.Split(@($anchor),[StringSplitOptions]::None)).Count -ne 2) { throw 'ShareX canvas paint boundary changed' }
        $formText=$formText.Replace($anchor,"if (WinOnePresentation) DrawWinOneTranslation(g);`n`n            "+$anchor)
        [IO.File]::WriteAllText($formPath,$formText,[Text.UTF8Encoding]::new($false))
    }
    $shapePath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Shapes/BaseShape.cs'
    # The canvas already shows the pixelation itself; keep its adjustable value
    # in Style and omit the overlaid "Pixelate [15]" label while drawing.
    $pixelatePath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Shapes/Effect/PixelateEffectShape.cs'
    $pixelateText=[IO.File]::ReadAllText($pixelatePath)
    $pixelateText=$pixelateText.Replace('public override string OverlayText => Resources.Pixelate + $" [{PixelSize}]";','public override string OverlayText => Manager?.Form.WinOnePresentation == true ? string.Empty : Resources.Pixelate + $" [{PixelSize}]";')
    [IO.File]::WriteAllText($pixelatePath,$pixelateText,[Text.UTF8Encoding]::new($false))
    # Every effect (including a pending cache) uses this common overlay. Keep
    # the existing preview/corner feedback; values belong in Style, not on pixels.
    $effectPath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Shapes/Effect/BaseEffectShape.cs'
    $effectText=[IO.File]::ReadAllText($effectPath)
    if(-not $effectText.Contains('if (Manager?.Form.WinOnePresentation == true) return;')) {
        $anchor='g.DrawCornerLines(Rectangle.Offset(1), Pens.White, 25);'
        if(($effectText.Split(@($anchor),[StringSplitOptions]::None)).Count -ne 2){throw 'ShareX effect overlay boundary changed'}
        $effectText=$effectText.Replace($anchor,$anchor+"`n`n            if (Manager?.Form.WinOnePresentation == true) return;")
        [IO.File]::WriteAllText($effectPath,$effectText,[Text.UTF8Encoding]::new($false))
    }
    $shapeText=[IO.File]::ReadAllText($shapePath)
    $shapeText=$shapeText.Replace('public abstract class BaseShape :','public abstract partial class BaseShape :')
    # The Move tool selects annotations. A restored crop may be last in z-order,
    # but must remain adjustable only through the explicit region tool.
    $selectable='public virtual bool IsSelectable => Manager.CurrentTool == ShapeType || Manager.CurrentTool == ShapeType.ToolSelect;'
    $shapeText=$shapeText.Replace($selectable,'public virtual bool IsSelectable => Manager.CurrentTool == ShapeType || Manager.CurrentTool == ShapeType.ToolSelect && (!Manager.Form.WinOnePresentation || ShapeCategory != ShapeCategory.Region);')
    if(-not $shapeText.Contains('Manager?.WinOneOrderedEffects == true')) {
        # Rebuilding bounds from float endpoints changes legacy fractional sizes.
        # Translation must keep the original width/height.
        $anchor='public virtual void Move(float x, float y)'
        $position=$shapeText.IndexOf($anchor)
        if($position -lt 0){throw 'ShareX shape translation anchor changed'}
        $position=$shapeText.IndexOf('{',$position)+1
        $shapeText=$shapeText.Insert($position,"`n            if (Manager?.WinOneOrderedEffects == true) { Rectangle = new RectangleF(Rectangle.X + x, Rectangle.Y + y, Rectangle.Width, Rectangle.Height); return; }`n")
    }
    [IO.File]::WriteAllText($shapePath,$shapeText,[Text.UTF8Encoding]::new($false))
    $managerPath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Shapes/ShapeManager.cs'
    $managerText=[IO.File]::ReadAllText($managerPath)
    if(-not $managerText.Contains('TranslateWinOneCanvas(x, y)')) {
        $position=$managerText.IndexOf('public void MoveAll(float x, float y)')
        if($position -lt 0){throw 'ShareX canvas translation anchor changed'}
        $position=$managerText.IndexOf('{',$position)+1
        $managerText=$managerText.Insert($position,"`n            if (WinOneOrderedEffects) { TranslateWinOneCanvas(x, y); return; }`n")
        [IO.File]::WriteAllText($managerPath,$managerText,[Text.UTF8Encoding]::new($false))
    }
    $penPath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Shapes/Drawing/FreehandDrawingShape.cs'
    $penText=[IO.File]::ReadAllText($penPath)
    if($penText.Contains('if (Manager.IsRenderingOutput)')) {
        # Detached rendering snapshots have no UI manager and always render final quality.
        [IO.File]::WriteAllText($penPath,$penText.Replace('if (Manager.IsRenderingOutput)','if (Manager == null || Manager.IsRenderingOutput)'),[Text.UTF8Encoding]::new($false))
    }
    # Legacy documents keep effect order. Their composed background replaces
    # the default effect-first drawing pass only for that restored document.
    $formPath=Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/Forms/RegionCaptureForm.cs'
    $formText=[IO.File]::ReadAllText($formPath)
    if(-not $formText.Contains('ShapeManager.WinOneComposition()')) {
        $anchor='g.DrawImage(captureBackground, CanvasRectangle);'
        if(-not $formText.Contains($anchor)){throw 'ShareX background anchor changed'}
        $formText=$formText.Replace($anchor,'g.DrawImage(WinOnePresentation && ShapeManager.WinOneOrderedEffects ? ShapeManager.WinOneComposition() : captureBackground, CanvasRectangle);')
        $begin=$formText.IndexOf('            // Draw effect shapes')
        $end=$formText.IndexOf('            // Draw tools',$begin)
        if($begin -lt 0 -or $end -lt 0){throw 'ShareX drawing anchors changed'}
        $body=$formText.Substring($begin,$end-$begin)
        $formText=$formText.Remove($begin,$end-$begin).Insert($begin,"            if (!WinOnePresentation || !ShapeManager.WinOneOrderedEffects) {`n"+$body+"            }`n`n")
        [IO.File]::WriteAllText($formPath,$formText,[Text.UTF8Encoding]::new($false))
    }
    foreach ($type in @('FreehandDrawingShape', 'LineDrawingShape')) {
        $file = Join-Path $SourceDirectory ('ShareX.ScreenCaptureLib/Shapes/Drawing/' + $type + '.cs')
        $text = [IO.File]::ReadAllText($file)
        $original = 'public class ' + $type + ' :'
        $partial = 'public partial class ' + $type + ' :'
        if ($text.Contains($original)) { [IO.File]::WriteAllText($file, $text.Replace($original, $partial), [Text.UTF8Encoding]::new($false)) }
        elseif (!$text.Contains($partial)) { throw ('ShareX native drawing declaration changed: ' + $type) }
    }
    [xml]$document = [IO.File]::ReadAllText($project)
    if ($document.DocumentElement.Name -ne 'Project') { throw 'Unexpected ShareX project format' }
    $imports = @($document.SelectNodes('/Project/Import[@Label="WinOneNativeCapture"]'))
    if ($imports.Count -gt 1) { throw 'Duplicate WinOne native capture imports' }
    $baseUri = [Uri]((Split-Path -Parent $project) + [IO.Path]::DirectorySeparatorChar)
    $relation = $baseUri.MakeRelativeUri([Uri]$targets)
    $relative = if ($relation.IsAbsoluteUri) { $targets } else { [Uri]::UnescapeDataString($relation.ToString()) }
    if ($imports.Count -eq 1 -and $imports[0].GetAttribute('Project') -eq $relative) { return }
    $import = if ($imports.Count -eq 1) { $imports[0] } else { $document.CreateElement('Import') }
    $import.SetAttribute('Project', $relative)
    $import.SetAttribute('Label', 'WinOneNativeCapture')
    if ($imports.Count -eq 0) { [void]$document.DocumentElement.AppendChild($import) }
    # All project-graph visits must compile the same engine. Per-reference
    # global properties can create competing builds that share one output DLL.
    $settings = [Xml.XmlWriterSettings]::new()
    $settings.Indent = $true; $settings.NewLineChars = "`n"; $settings.Encoding = [Text.UTF8Encoding]::new($false)
    $writer = [Xml.XmlWriter]::Create($project, $settings)
    try { $document.Save($writer) } finally { $writer.Dispose() }
}
