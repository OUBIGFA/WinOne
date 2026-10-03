using ShareX.ScreenCaptureLib;

internal static class Program
{
    [STAThread]
    private static int Main(string[] args)
    {
        if (args.Length == 2 && args[0] == "--interactive-output") return InteractiveOutputProbe.Run(args[1]);
        if (args is ["--native-pointer-dispatch"]) return NativePointerDispatchTests.Run();
        if (args.Length == 2 && args[0] == "--native-scroll-progress") return NativeScrollProgressTests.Run(args[1]);
        if (args.Length == 2 && args[0] == "--scrolling-frame") { ScrollingFrameTests.Run(args[1]); return 0; }
        if (args is ["--native-presentation"]) { NativePresentationTests.Run(); return 0; }
        if (args.Length == 2 && args[0] == "--manual-capture") return NativeManualProbe.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-paint-dispatch") return NativePaintDispatchProbe.Run(args[1]);
        if (args.Length == 3 && args[0] == "--production-capture-probe") return ProductionCaptureProbe.Run(args[1], args[2]);
        if (args.Length == 3 && args[0] == "--production-capture-resources") return ProductionCaptureProbe.Run(args[1], args[2], 100);
        if (args is ["--native-viewport"]) return NativeViewportTests.Run();
        if (args.Length == 2 && args[0] == "--native-autosave") return NativeAutosaveTests.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-draft-retirement") return NativeDraftRetirementTests.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-draft-store") return NativeDraftStoreTests.Run(args[1]);
        if (args.Length == 4 && args[0] == "--native-host-roundtrip") return NativeHostRoundTripProbe.Run(args[1], int.Parse(args[2], System.Globalization.CultureInfo.InvariantCulture), args[3]);
        if (args.Length == 2 && args[0] == "--native-configuration-lifetime") return NativeConfigurationLifetimeProbe.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-toolbar-customization") return NativeToolbarCustomizationProbe.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-toolbar-preferences") return NativeToolbarPreferencesProbe.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-workflow-probe") return NativeCaptureWorkflowTests.Run(args[1]);
        if (args.Length == 4 && args[0] == "--component-lifetime") return ComponentLifetimeProbe.Run(args[1], args[2], args[3]);
        if (args.Length == 5 && args[0] == "--worker-probe") return NativeWorkerProbe.Run(args[1], args[2], args[3], args[4]);
        if (args.Length == 2 && args[0] == "--native-text-render-probe") return NativeTextRenderingProbe.Run(args[1]);
        if (args.Length == 2 && args[0] == "--native-toolbar-render-probe") return NativeToolbarRenderingProbe.Run(args[1]);
        if (args.Length == 4 && args[0] == "--show-probe") return ShowProbe(args[1], args[2], args[3]);
        // The initial probe checks the facade without opening a desktop window.
        if (args is not ["--self-test"]) return 2;
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        CaptureCompletionTests.Run();
        NativeHostConfigurationTests.Run();
        SingleSelectionTests.Run();
        KeyboardRoutingTests.Run();
        NativeToolbarTests.Run();
        NativeTextHistoryTests.Run();
        NativeDrawingTests.Run();
        NativeToolStyleTests.Run();
        NativeDocumentTests.Run();
        BackgroundRenderingTests.Run();
        NativePresentationTests.Run();
        NativeWindowRegionsTests.RunWithMessageLoop();
        var negativePoint = NativeTextPlacement.PointFromMessage(new IntPtr(unchecked((int)0xfff4fffb)));
        if (negativePoint != new Point(-5, -12)) throw new Exception("Native mouse coordinates lost their sign");
        if (NativeTextPlacement.NewBounds(new Rectangle(100, 100, 700, 400), new Point(140, 150)) != new Rectangle(140, 150, 400, 50)) throw new Exception("Text placement ignored the clicked point");
        try { NativeTextPlacement.NewBounds(new Rectangle(100, 100, 700, 400), new Point(10, 10)); throw new Exception("Outside click was accepted"); }
        catch (InvalidOperationException error) when (error.Message == "请在截图选区内点击") { }
        using var image = new Bitmap(120, 80);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.White);
        using var capture = new WinOneCaptureCanvas(image);
        capture.AddRectangle(new Rectangle(10, 10, 50, 40));
        if (capture.AnnotationCount != 1 || !capture.CanUndo) throw new Exception("Native annotation/history did not advance");
        using var rendered = capture.Render();
        if (rendered.Size != image.Size) throw new Exception("Native output size changed");
        static int[] Pixels(Bitmap bitmap) => Enumerable.Range(0, bitmap.Height).SelectMany(y => Enumerable.Range(0, bitmap.Width).Select(x => bitmap.GetPixel(x, y).ToArgb())).ToArray();
        var expected = Pixels(rendered);
        if (expected.All(pixel => pixel == Color.White.ToArgb())) throw new Exception("Native annotation produced no pixels");
        capture.Undo();
        if (capture.AnnotationCount != 0 || !capture.CanRedo) throw new Exception("Native undo failed");
        using var undone = capture.Render();
        if (Pixels(undone).Any(pixel => pixel != Color.White.ToArgb())) throw new Exception("Undo did not restore original pixels");
        capture.Redo();
        if (capture.AnnotationCount != 1) throw new Exception("Native redo failed");
        using var redone = capture.Render();
        if (!Pixels(redone).SequenceEqual(expected)) throw new Exception("Redo changed rendered pixels");
        capture.AddText("你好\nWinOne", new Rectangle(10, 5, 100, 70), 20);
        using var text = capture.Render();
        var textPixels = Pixels(text);
        if (capture.AnnotationCount != 2 || textPixels.SequenceEqual(expected)) throw new Exception("Inline text was not rendered");
        var originalText = capture.EditableText ?? throw new Exception("New text was not selectable");
        capture.HideEditedText(originalText.Id, true);
        capture.HideEditedText(originalText.Id, false);
        using (var cancelledEdit = capture.Render())
            if (!Pixels(cancelledEdit).SequenceEqual(textPixels)) throw new Exception("Cancelling text edit changed output");
        capture.UpdateText(originalText.Id, "修改文字", originalText.Bounds);
        if (capture.AnnotationCount != 2 || capture.EditableText?.Bounds != originalText.Bounds) throw new Exception("Text edit added a layer or moved it");
        using var editedText = capture.Render();
        if (Pixels(editedText).SequenceEqual(textPixels)) throw new Exception("Text update was not rendered");
        capture.Undo();using (var revertedEdit = capture.Render())
            if (!Pixels(revertedEdit).SequenceEqual(textPixels)) throw new Exception("Text edit was not one undo step");
        capture.Undo();using var beforeText = capture.Render();
        if (capture.AnnotationCount != 1 || !Pixels(beforeText).SequenceEqual(expected)) throw new Exception("Text undo changed earlier annotations");
        capture.Redo();using var restoredText = capture.Render();
        if (capture.AnnotationCount != 2 || !Pixels(restoredText).SequenceEqual(textPixels)) throw new Exception("Text redo changed output");
        capture.UpdateText(originalText.Id, "", originalText.Bounds);
        if (capture.AnnotationCount != 1) throw new Exception("Clearing text retained an empty annotation");
        capture.Undo();using var restoredClearedText = capture.Render();
        if (capture.AnnotationCount != 2 || !Pixels(restoredClearedText).SequenceEqual(textPixels)) throw new Exception("Cleared text could not be restored");
        Console.WriteLine("PASS: isolated native facade delegates annotations, rendering, undo and redo");
        return 0;
    }

    private static int ShowProbe(string imagePath, string assets, string output)
    {
        var entryTicks = System.Diagnostics.Stopwatch.GetTimestamp();
        double? launchToEntryMs = null;
        var launchValue = Environment.GetEnvironmentVariable("WINONE_PROBE_LAUNCH_TICKS");
        if (!string.IsNullOrEmpty(launchValue)) {
            if (!long.TryParse(launchValue, out var launchTicks) || launchTicks <= 0 || launchTicks > entryTicks)
                throw new InvalidDataException("Invalid monotonic launch timestamp");
            launchToEntryMs = System.Diagnostics.Stopwatch.GetElapsedTime(launchTicks, entryTicks).TotalMilliseconds;
        }
        var clock = System.Diagnostics.Stopwatch.StartNew();
        var startupStages = new Dictionary<string, double>();
        imagePath = Path.GetFullPath(imagePath); assets = Path.GetFullPath(assets); output = Path.GetFullPath(output);
        if (Directory.Exists(output)) throw new IOException("Choose a new prototype output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
        Application.EnableVisualStyles();
        using var image = new Bitmap(imagePath);
        startupStages["sourceOpened"] = clock.Elapsed.TotalMilliseconds;
        if (image.Size != SystemInformation.VirtualScreen.Size) throw new InvalidDataException("Synthetic source must match desktop dimensions");
        using var capture = new WinOneCaptureCanvas(image, stage => startupStages[stage] = clock.Elapsed.TotalMilliseconds);
        // A known scene isolates toolbar/native-engine communication from
        // mouse automation, which is separately subject to user approval.
        capture.SetRegion(new Rectangle(100, 100, 700, 400));
        capture.AddRectangle(new Rectangle(200, 200, 150, 100));
        var copyAttempts = 0;
        void ReceiveCopy(Bitmap bitmap) {
            copyAttempts++;
            if (copyAttempts == 1 && Environment.GetEnvironmentVariable("WINONE_PROBE_COPY_FAIL_ONCE") == "1")
                throw new InvalidOperationException("复制未完成，请重试");
            // Isolated receiver substitutes for the user's clipboard.
            bitmap.Save(Path.Combine(output, "confirmed.png"), System.Drawing.Imaging.ImageFormat.Png);
            File.WriteAllText(Path.Combine(output, "copy-receiver.json"), System.Text.Json.JsonSerializer.Serialize(new { copyAttempts, annotations = capture.AnnotationCount, width = bitmap.Width, height = bitmap.Height, realClipboard = false }));
        }
        Action<Bitmap> copy = ReceiveCopy;
        Action<Bitmap>? save = null;
        Action<Bitmap>? saveAs = null;
        if (Environment.GetEnvironmentVariable("WINONE_PROBE_SHARED_OUTPUT") == "1") {
            var directory = Path.Combine(output, "product-data");
            var settings = new WinOne.Capture.SettingsStore(directory);
            settings.Save(settings.Snapshot() with { OutputDirectory = Path.Combine(output, "product-images"), CopyToClipboard = false });
            var pickerCalls = 0;
            Func<WinOne.Capture.CaptureSettings, string?, string?>? picker = null;
            if (Environment.GetEnvironmentVariable("WINONE_PROBE_SAVE_AS_CASES") == "1") picker = (_, _) => {
                if (++pickerCalls == 1) return null;
                if (pickerCalls == 2) {
                    var blocked = Path.Combine(output, "blocked-folder"); File.WriteAllText(blocked, "fixture");
                    return Path.Combine(blocked, "capture.png");
                }
                return Path.Combine(output, "chosen-copy.png");
            };
            var operations = new WinOne.Capture.CaptureOperations(settings, directory, ReceiveCopy, picker);
            var port = operations.CreateNativeOutput(CancellationToken.None);
            var outputs = new List<WinOne.Capture.CaptureOutput>();
            copy = port.Confirm;
            save = bitmap => {
                outputs.Add(port.Save(bitmap));
                File.WriteAllText(Path.Combine(output, "backend-output.json"), System.Text.Json.JsonSerializer.Serialize(outputs, WinOne.Capture.SettingsStore.Json));
            };
            saveAs = bitmap => {
                var result = port.SaveAs(bitmap);
                File.WriteAllText(Path.Combine(output, "save-as-output.json"), System.Text.Json.JsonSerializer.Serialize(result, WinOne.Capture.SettingsStore.Json));
            };
        }
        using ICaptureProbe toolbar = Environment.GetEnvironmentVariable("WINONE_PROBE_NATIVE_TOOLBAR") == "1"
            ? new NativeSessionProbe(capture, assets, output, clock, copy, launchToEntryMs, startupStages, save, saveAs)
            : new HostedToolbar(capture, assets, output, clock, copy, launchToEntryMs, startupStages, save, saveAs);
        startupStages["toolbarAttached"] = clock.Elapsed.TotalMilliseconds;
        capture.ShowDialog();
        return toolbar.Failed ? 1 : 0;
    }
}
