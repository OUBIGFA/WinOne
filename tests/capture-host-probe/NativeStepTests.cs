using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;

internal static class NativeStepTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    private static readonly Color Red = Color.FromArgb(0xe2, 0x3b, 0x30);
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height)
        .SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();
    private static int[] Pixels(Bitmap image, Rectangle area) => Enumerable.Range(area.Top, area.Height)
        .SelectMany(y => Enumerable.Range(area.Left, area.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();
    private static bool Near(Color actual, Color expected) =>
        Math.Abs(actual.R - expected.R) <= 6 && Math.Abs(actual.G - expected.G) <= 6 && Math.Abs(actual.B - expected.B) <= 6;
    private static void Check(bool condition, string message) { if (!condition) throw new Exception(message); }
    private static Bitmap Blank(int width, int height)
    {
        var image = new Bitmap(width, height);
        using var graphics = Graphics.FromImage(image); graphics.Clear(Color.White);
        return image;
    }

    // Numbering, the label bubble, history and the saved document, without a window.
    public static void Run()
    {
        var metrics = WinOneStepMetrics.For(16);
        var half = metrics.Badge / 2;
        Rectangle Badge(int y) => new(40 - half, y - half, metrics.Badge, metrics.Badge);
        using var image = Blank(400, 240);
        using var capture = new WinOneCaptureCanvas(image);
        capture.SetRegion(new Rectangle(0, 0, 400, 240));
        var first = capture.AddStep(new Point(40, 40), 16);
        var second = capture.AddStep(new Point(40, 100), 16);
        var third = capture.AddStep(new Point(40, 160), 16, Color.RoyalBlue);
        Check(first.Number == 1 && second.Number == 2 && third.Number == 3 && capture.StepCount == 3 && capture.AnnotationCount == 3, "Steps were not numbered in order");
        var label = new Rectangle(40 + metrics.TextOffset.X, 40 + metrics.TextOffset.Y, 60, metrics.Line);
        using (var plain = capture.Render()) {
            Check(Near(plain.GetPixel(40 - half + 2, 40), Red) && Near(plain.GetPixel(40 - half + 2, 160), Color.RoyalBlue), "Badges lost their fill color");
            Check(Pixels(plain, Badge(40)).Any(pixel => Near(Color.FromArgb(pixel), Color.White)), "The badge has no white number");
            Check(!Pixels(plain, Badge(40)).SequenceEqual(Pixels(plain, Badge(100))), "Two steps show the same number");
            Check(plain.GetPixel(label.Left + 50, 40).ToArgb() == Color.White.ToArgb(), "A step without a label drew a bubble");
        }
        capture.UpdateStep(first.Id, "说明", label.Size, new[] { new WinOneTextLine("说明", 0, 0) });
        Check(capture.Step(first.Id) is { Text: "说明" } labelled && labelled.TextBounds == label, "The label was not stored beside its badge");
        using (var bubble = capture.Render()) {
            Check(Near(bubble.GetPixel(label.Left + 50, 40), WinOneStepShape.LabelFill), "The label bubble is not dark gray");
            Check(Pixels(bubble, new Rectangle(label.Left, label.Top, 34, label.Height)).Any(pixel => Near(Color.FromArgb(pixel), Color.White)), "The label text is not white");
            Check(Near(bubble.GetPixel(40 - half + 2, 40), Red), "Adding a label moved the badge");
            Check(bubble.GetPixel(label.Left + 110, 40 + metrics.Line).ToArgb() == Color.White.ToArgb(), "A short label drew an oversized bubble");
        }
        // The bubble follows the size of its text, in both directions.
        capture.UpdateStep(first.Id, "说明\r\n第二行更长", new Size(120, 2 * metrics.Line),
            new[] { new WinOneTextLine("说明", 0, 0), new WinOneTextLine("第二行更长", 0, metrics.Line) });
        using var grown = capture.Render(); var grownPixels = Pixels(grown);
        Check(Near(grown.GetPixel(label.Left + 110, 40 + metrics.Line), WinOneStepShape.LabelFill), "The bubble did not grow with its text");
        capture.Undo();
        Check(capture.Step(first.Id).TextBounds == label, "Undo did not restore the previous label size");
        capture.Redo();
        using (var redone = capture.Render()) Check(Pixels(redone).SequenceEqual(grownPixels), "Redo changed the labelled step");
        // The saved document restores the same pixels and numbers.
        var document = capture.ExportDocument();
        Check(document.Layers.Select(layer => layer.Kind).SequenceEqual(new[] { "step", "step", "step" }), "Steps were not saved as step layers");
        var decoded = NativeCaptureDocumentCodec.Decode(NativeCaptureDocumentCodec.Encode(document, image.Size), image.Size);
        using (var restored = new WinOneCaptureCanvas(image)) {
            restored.RestoreDocument(decoded);
            using var actual = restored.Render();
            Check(Pixels(actual).SequenceEqual(grownPixels), "A restored step changed output pixels");
            Check(restored.Step(third.Id).Number == 3 && restored.Step(first.Id).Text == "说明\r\n第二行更长", "A restored step lost its number or label");
        }
        // Background composition numbers its detached copies the same way.
        using (var ordered = new WinOneCaptureCanvas(image)) {
            ordered.RestoreDocument(decoded with { Version = 2, OrderedEffects = true });
            using var actual = ordered.Render();
            // GDI label text is antialiased differently on this path, as for
            // inline text; numbers and bubble geometry must be identical.
            foreach (var y in new[] { 40, 100, 160 })
                Check(Pixels(actual, Badge(y)).SequenceEqual(Pixels(grown, Badge(y))), "Background rendering changed a step number");
            Check(Near(actual.GetPixel(label.Left + 110, 40 + metrics.Line), WinOneStepShape.LabelFill) && actual.GetPixel(label.Left + 50, 100).ToArgb() == Color.White.ToArgb(), "Background rendering changed the label bubble");
            var orderedPixels = Pixels(actual);
            ordered.EditStep(first.Id, new Size(180, 3 * metrics.Line));
            using (var preview = ordered.Render()) {
                Check(Near(preview.GetPixel(label.Left + 170, label.Top + 2 * metrics.Line), WinOneStepShape.LabelFill), "The ordered label preview did not grow with its input");
                Check(Pixels(preview, new Rectangle(label.Left, label.Top, 34, metrics.Line)).All(pixel => Near(Color.FromArgb(pixel), WinOneStepShape.LabelFill)), "The ordered preview retained the committed label while editing");
            }
            Check(ordered.ExportDocument().Layers[0].Bounds == decoded.Layers[0].Bounds && ordered.Step(first.Id).Text == decoded.Layers[0].Text, "Live label sizing changed the saved step");
            ordered.EditStep(first.Id, null);
            using var cancelled = ordered.Render();
            Check(Pixels(cancelled).SequenceEqual(orderedPixels), "Cancelling an ordered label preview changed the committed output");
        }
        // Removing a step renumbers the ones after it.
        using (var renumbered = new WinOneCaptureCanvas(image)) {
            renumbered.RestoreDocument(decoded with { Layers = new[] { decoded.Layers[0], decoded.Layers[2] } });
            Check(renumbered.Step(third.Id).Number == 2, "The remaining steps were not renumbered");
        }
        try { (decoded with { Layers = new[] { decoded.Layers[0] with { FillColor = Color.FromArgb(120, Red).ToArgb() } } }).Validate(image.Size); throw new Exception("A translucent badge was accepted"); }
        catch (InvalidDataException) { }
        // A blank label leaves the badge alone; undo brings the label back.
        capture.UpdateStep(first.Id, "  ", label.Size, null);
        using (var cleared = capture.Render())
            Check(capture.Step(first.Id).Text == "" && cleared.GetPixel(label.Left + 50, 40).ToArgb() == Color.White.ToArgb() && Near(cleared.GetPixel(40 - half + 2, 40), Red), "A blank label kept its bubble or lost its badge");
        capture.Undo();
        using (var relabelled = capture.Render()) Check(Pixels(relabelled).SequenceEqual(grownPixels), "Undo did not restore the cleared label");

        using var canvas = new WinOneCaptureCanvas(image);
        canvas.SetRegion(new Rectangle(0, 0, 400, 240));
        // A new step and its first label are one undo step.
        var fresh = canvas.AddStep(new Point(40, 40), 16);
        canvas.UpdateStep(fresh.Id, "一步", label.Size, new[] { new WinOneTextLine("一步", 0, 0) }, amend: true);
        canvas.Undo();
        Check(canvas.AnnotationCount == 0 && !canvas.CanUndo, "A new labelled step needed two undo steps");
        canvas.Redo();
        Check(canvas.Step(fresh.Id) is { Text: "一步", Number: 1 }, "Redo lost the labelled step");
        canvas.AddStep(new Point(40, 100), 16);
        Check(canvas.AddStep(new Point(40, 160), 16).Number == 3, "Numbering did not continue after undo and redo");
        // Placing a number does not make it the Delete target; Move selects one.
        Check(!canvas.DeleteSelected() && canvas.StepCount == 3, "A step that was only placed was deleted");
        // Near an edge the badge shifts so that its smallest label still fits.
        var area = new Rectangle(10, 10, 300, 200);
        var fitted = metrics.Fit(area, new Point(305, 12)) ?? throw new Exception("A normal crop cannot hold a step");
        Check(area.Contains(new Rectangle(fitted.X - half, fitted.Y + metrics.TextOffset.Y - metrics.PadY, half + metrics.TextOffset.X + metrics.MinText + metrics.PadX, metrics.Line + 2 * metrics.PadY)), "A step near the edge leaves the crop");
        Check(metrics.Fit(area, new Point(100, 100)) == new Point(100, 100), "A step inside the crop moved away from the click");
        Check(metrics.Fit(new Rectangle(0, 0, 30, 12), new Point(5, 5)) is null, "A crop too small for a step was accepted");
        Console.WriteLine("PASS: numbered steps keep order, label bubble size, history, document roundtrip and edge placement");
    }

    // Real window messages on the private desktop: click to place, type to
    // resize, click again for the next number, then delete with Move.
    public static void RunInteraction()
    {
        static IntPtr At(int x, int y) => new((y << 16) | (x & 0xffff));
        using var image = Blank(800, 600);
        using var canvas = new WinOneCaptureCanvas(image);
        using var session = new NativeCaptureSession(canvas, _ => { }, _ => { });
        canvas.SetRegion(new Rectangle(20, 20, 700, 500));
        Exception? failure = null; var scheduled = false; var timedOut = false;
        using var deadline = new System.Windows.Forms.Timer { Interval = 8000 };
        deadline.Tick += (_, _) => { timedOut = true; canvas.Window.Close(); };
        void Click(int x, int y) {
            SendMessageW(canvas.Window.Handle, 0x0200, IntPtr.Zero, At(x, y));
            SendMessageW(canvas.Window.Handle, 0x0201, new IntPtr(1), At(x, y));
            SendMessageW(canvas.Window.Handle, 0x0202, IntPtr.Zero, At(x, y));
        }
        ToolStripButton Button(string name) => session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.OfType<ToolStripButton>()).Single(item => item.Name == name);
        session.Updated += () => {
            if (!session.Ready || scheduled) return; scheduled = true;
            canvas.Window.BeginInvoke((Action)(() => {
                try {
                    Button("step").PerformClick();
                    Check(session.StepTool && Button("step").Checked, "The toolbar did not select the step tool");
                    Button("select").PerformClick();
                    SendMessageW(canvas.Window.Handle, 0x0100, new IntPtr((int)Keys.N), IntPtr.Zero);
                    Check(session.StepTool && Button("step").Checked, "The N shortcut did not select the step tool");
                    Click(120, 100);
                    var input = session.TextInput ?? throw new Exception("A new step did not open its label input");
                    Check(canvas.StepCount == 1 && canvas.StepAt(new Point(120, 100)) is { Number: 1 }, "The click did not place step 1");
                    Check(input.ForeColor.ToArgb() == Color.White.ToArgb(), "The label input is not white on dark gray");
                    var metrics = WinOneStepMetrics.For(session.EditingText!.FontPixels);
                    var bubbleEdge = 120 + metrics.TextOffset.X;
                    input.Text = "第一步"; var compact = input.Size;
                    input.Text = "第一步：打开设置页面并找到截图选项";
                    Check(input.Width > compact.Width && input.Height == compact.Height, "The label input did not widen with its text");
                    var wide = input.Width;
                    using (var live = canvas.Render())
                        Check(Near(live.GetPixel(bubbleEdge + wide + metrics.PadX - 3, 100), WinOneStepShape.LabelFill), "The bubble did not follow the input while typing");
                    input.Text = "第一行\r\n第二行";
                    Check(input.Height == 2 * compact.Height && input.Width < wide, "The label input did not grow a second line");
                    input.Text = "第一步";
                    Check(input.Size == compact, "The label input did not shrink back");
                    // The click that finishes a label places the next number.
                    Click(120, 220);
                    var next = session.TextInput ?? throw new Exception("The next click did not start step 2");
                    Check(!ReferenceEquals(next, input) && canvas.StepCount == 2, "The next click did not add step 2");
                    Check(canvas.StepAt(new Point(120, 100)) is { Text: "第一步", Number: 1 } && canvas.StepAt(new Point(120, 220)) is { Number: 2 }, "Step 1 was not committed before step 2");
                    // Escape drops the label and keeps the number.
                    SendMessageW(next.Handle, 0x0100, new IntPtr((int)Keys.Escape), IntPtr.Zero);
                    Check(session.TextInput is null && canvas.StepCount == 2 && canvas.StepAt(new Point(120, 220)) is { Text: "" }, "Escape did not keep a bare number");
                    // A draft recovered after a restart reopens on its step.
                    var bare = canvas.StepAt(new Point(120, 220))!;
                    session.RestoreText(new NativeTextInputState(bare.Id, "恢复", bare.TextBounds.X, bare.TextBounds.Y, bare.TextBounds.Width, bare.TextBounds.Height, bare.TextBounds.Height, bare.FontPixels, Color.White.ToArgb(), 1, 1));
                    var recovered = session.TextInput ?? throw new Exception("A recovered step label did not reopen");
                    Check(session.StepTool && recovered.Text == "恢复" && recovered.SelectionStart == 1 && session.Snapshot()?.Text?.Target == bare.Id, "A recovered step label lost its text, caret or step");
                    SendMessageW(recovered.Handle, 0x0100, new IntPtr((int)Keys.Escape), IntPtr.Zero);
                    Check(session.TextInput is null && canvas.StepAt(new Point(120, 220)) is { Text: "" }, "Cancelling a recovered label changed the step");
                    // Clicking an existing step edits it instead of adding one.
                    Click(120, 220);
                    var edit = session.TextInput ?? throw new Exception("Clicking a step did not edit its label");
                    Check(canvas.StepCount == 2, "Clicking a step added another one");
                    edit.Text = "第二步";
                    // A click outside the crop only finishes the label.
                    Click(760, 560);
                    Check(session.TextInput is null && canvas.StepCount == 2 && canvas.StepAt(new Point(120, 220)) is { Text: "第二步", Number: 2 }, "A click outside the crop did not just commit the label");
                    using (var output = canvas.RenderSelection())
                        Check(Near(output.GetPixel(100 - metrics.Badge / 2 + 2, 80), Red) && Near(output.GetPixel(bubbleEdge - 20 + compact.Width - 2, 80 + metrics.Line / 2 + metrics.PadY - 2), WinOneStepShape.LabelFill), "The output lacks the red badge or dark label");
                    // Move selects a step; Delete removes it and renumbers the rest.
                    Button("select").PerformClick();
                    Click(120, 100);
                    // The selection outline is an editing aid, never part of the output.
                    using (var selected = canvas.RenderSelection())
                        Check(selected.GetPixel(100 - metrics.Badge / 2, 80 + metrics.TextOffset.Y - metrics.PadY).ToArgb() == Color.White.ToArgb(), "The selection outline was rendered into the output");
                    SendMessageW(canvas.Window.Handle, 0x0100, new IntPtr((int)Keys.Delete), IntPtr.Zero);
                    Check(canvas.StepCount == 1 && canvas.StepAt(new Point(120, 220)) is { Number: 1, Text: "第二步" }, "Delete did not remove step 1 and renumber step 2");
                    Button("undo").PerformClick();
                    Check(canvas.StepCount == 2 && canvas.StepAt(new Point(120, 100)) is { Number: 1 } && canvas.StepAt(new Point(120, 220)) is { Number: 2 }, "Undo did not restore the deleted step and its number");
                } catch (Exception error) { failure = error; }
                finally { canvas.Window.Close(); }
            }));
        };
        deadline.Start(); canvas.ShowDialog(); deadline.Stop();
        if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        if (timedOut) throw new Exception("The step session did not become ready");
        Console.WriteLine("PASS: step tool places numbers by click, resizes its label input, chains steps, edits, deletes and renumbers");
    }
}
