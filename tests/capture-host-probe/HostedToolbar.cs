using System.Diagnostics;
using System.Drawing.Imaging;
using System.Text.Json;
using System.Runtime.InteropServices;
using System.Text;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;
using ShareX.ScreenCaptureLib;

internal sealed class HostedToolbar : ICaptureProbe
{
    [DllImport("user32.dll")] private static extern IntPtr GetAncestor(IntPtr window, uint flags);
    [DllImport("user32.dll")] private static extern int GetWindowRgn(IntPtr window, IntPtr region);
    [DllImport("gdi32.dll")] private static extern IntPtr CreateRectRgn(int left, int top, int right, int bottom);
    [DllImport("gdi32.dll")] private static extern bool PtInRegion(IntPtr region, int x, int y);
    [DllImport("gdi32.dll")] private static extern bool DeleteObject(IntPtr value);
    [DllImport("kernel32.dll")] private static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] private static extern IntPtr GetThreadDesktop(uint thread);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern bool GetUserObjectInformationW(IntPtr handle, int index, StringBuilder value, uint size, out uint needed);
    internal static string CurrentDesktopName()
    {
        var name = new StringBuilder(256);
        if (!GetUserObjectInformationW(GetThreadDesktop(GetCurrentThreadId()), 2, name, 512, out _)) throw new InvalidOperationException("Desktop identity unavailable");
        return name.ToString();
    }
    private const string Origin = "https://capture.winone.test";
    private const string Entry = "/tests/native-toolbar.html";
    private readonly WinOneCaptureCanvas capture;
    private readonly WebView2 view = new() { Visible = false, DefaultBackgroundColor = Color.Transparent };
    private readonly NativeToolbarControl? nativeToolbar;
    private readonly bool nativeDark;
    private readonly HintLabel selectionHint = new() { AutoSize = true, Visible = false, Padding = new Padding(8, 4, 8, 4), TextAlign = ContentAlignment.MiddleCenter };
    private readonly string assets, output, nativeWindow;
    private readonly Stopwatch clock;
    private readonly double? launchToEntryMs;
    private readonly IReadOnlyDictionary<string, double>? startupStages;
    private readonly FirstPaintObserver paint;
    private readonly CaptureCompletion confirmation;
    private readonly CaptureCompletion? pinCompletion;
    private readonly Action<Bitmap> saveImage;
    private readonly Action<Bitmap>? saveAsImage;
    private readonly bool visibleInitialization = Environment.GetEnvironmentVariable("WINONE_PROBE_VISIBLE_INIT") == "1";
    private bool disposed, ready, browserReady, initializationStarted;
    private double? firstPaintMs, initializationStartedMs, environmentReadyMs, controllerReadyMs, domReadyMs, navigationReadyMs, toolbarReadyMs;
    private int saved, savedAs, saveAsAttempts;
    private bool applying;
    private int textSequence;
    private TextDraft? textDraft;
    private NativeInlineText? nativeText;
    private Point? toolbarPosition;
    private bool textTool;
    private sealed record TextDraft(int Id, Rectangle Bounds, int MaxHeight, Guid? Target = null, string Value = "", float FontPixels = 24);
    public bool Failed { get; private set; }
    public bool Confirmed => confirmation.CloseRequested && confirmation.Committed;
    public bool Pinned => pinCompletion?.CloseRequested == true;
    private bool Finalized => confirmation.Committed || pinCompletion?.Committed == true;

    public HostedToolbar(WinOneCaptureCanvas capture, string assets, string output, Stopwatch clock, Action<Bitmap> copy, double? launchToEntryMs = null, IReadOnlyDictionary<string, double>? startupStages = null, Action<Bitmap>? saveImage = null, Action<Bitmap>? saveAsImage = null, bool dark = false, Action<Bitmap>? pinImage = null)
    {
        this.capture = capture; this.assets = assets; this.output = output; this.clock = clock;
        nativeDark = dark;
        this.launchToEntryMs = launchToEntryMs;
        this.startupStages = startupStages;
        this.saveImage = saveImage ?? (bitmap => bitmap.Save(Path.Combine(output, $"capture-{saved + 1:00}.png"), ImageFormat.Png));
        this.saveAsImage = saveAsImage;
        if (Environment.GetEnvironmentVariable("WINONE_PROBE_NATIVE_TOOLBAR") == "1") {
            nativeToolbar = new NativeToolbarControl(dark);
            nativeToolbar.SetSaveAsAvailable(saveAsImage is not null);
            nativeToolbar.SetPinAvailable(pinImage is not null);
            nativeToolbar.Command += NativeCommand;
            nativeToolbar.MoveRequested += screenPoint => { toolbarPosition = capture.Window.PointToClient(screenPoint); Publish(0); };
            capture.Window.Controls.Add(nativeToolbar);
            selectionHint.BackColor = dark ? Color.FromArgb(23, 23, 23) : Color.White;
            selectionHint.ForeColor = dark ? Color.White : Color.FromArgb(23, 23, 23);
            capture.Window.Controls.Add(selectionHint);
            capture.Window.KeyPreview = true;
        }
        confirmation = new CaptureCompletion(capture.RenderSelection, copy, capture.Window.Close);
        if (pinImage is not null) pinCompletion = new CaptureCompletion(capture.RenderSelection, pinImage, capture.Window.Close);
        if (!File.Exists(Path.Combine(assets, "tests", "native-toolbar.html"))) throw new FileNotFoundException("Build the prototype toolbar first");
        nativeWindow = capture.Window.Handle.ToInt64().ToString();
        capture.Window.Controls.Add(view);
        // A/B diagnostic only: compare hidden-controller throttling with a
        // visible, noninteractive child. This is not a product startup policy.
        if (nativeToolbar is null && visibleInitialization && capture.Selection is { } initial) {
            view.Bounds = new Rectangle(initial.Left, initial.Bottom + 8, Math.Min(980, capture.Window.ClientSize.Width - 16), 104);
            view.Enabled = false; view.Visible = true;
        }
        capture.Changed += OnChanged;
        capture.KeyPressed += OnKeyPressed;
        capture.Window.MouseUp += OnMouseUp;
        paint = new FirstPaintObserver(capture.Window, () => {
            firstPaintMs = clock.Elapsed.TotalMilliseconds;
            // Begin browser creation only after the native paint returns. The
            // picture and all image gestures remain owned by the same engine.
            if (nativeToolbar is null) capture.Window.BeginInvoke((Action)(() => _ = Initialize()));
            else { ready = true; toolbarReadyMs = clock.Elapsed.TotalMilliseconds; Publish(0); }
        }, FilterTextInput);
        if (nativeToolbar is not null) Publish(0);
    }

    private async Task Initialize()
    {
        if (initializationStarted) return;
        initializationStarted = true;
        try {
            initializationStartedMs = clock.Elapsed.TotalMilliseconds;
            var arguments = "";
            var debug = Environment.GetEnvironmentVariable("WINONE_PROBE_CDP_PORT");
            if (!string.IsNullOrEmpty(debug)) {
                if (!ushort.TryParse(debug, out var port) || port < 1024) throw new InvalidDataException("Invalid probe port");
                arguments = $"--remote-debugging-port={port} --remote-debugging-address=127.0.0.1";
            }
            // The harness can reuse its own isolated profile to distinguish
            // first-profile setup from a later capture. Never use user data.
            var profile = Environment.GetEnvironmentVariable("WINONE_PROBE_WEBVIEW_PROFILE") ?? Path.Combine(output, "webview");
            var environment = await CoreWebView2Environment.CreateAsync(null, Path.GetFullPath(profile), new CoreWebView2EnvironmentOptions(arguments));
            environmentReadyMs = clock.Elapsed.TotalMilliseconds;
            if (disposed) return;
            await view.EnsureCoreWebView2Async(environment);
            controllerReadyMs = clock.Elapsed.TotalMilliseconds;
            if (disposed) return;
            var core = view.CoreWebView2;
            core.Settings.AreDefaultContextMenusEnabled = false;
            core.Settings.AreBrowserAcceleratorKeysEnabled = false;
            core.Settings.AreDevToolsEnabled = !string.IsNullOrEmpty(debug);
            core.SetVirtualHostNameToFolderMapping("capture.winone.test", assets, CoreWebView2HostResourceAccessKind.DenyCors);
            core.NavigationStarting += (_, e) => { if (e.Uri != Origin + Entry) e.Cancel = true; };
            core.NewWindowRequested += (_, e) => e.Handled = true;
            core.DownloadStarting += (_, e) => e.Cancel = true;
            core.WebMessageReceived += Message;
            core.DOMContentLoaded += (_, _) => domReadyMs = clock.Elapsed.TotalMilliseconds;
            core.NavigationCompleted += (_, _) => navigationReadyMs = clock.Elapsed.TotalMilliseconds;
            core.Navigate(Origin + Entry);
        } catch (Exception error) {
            if (disposed) return;
            Failed = true; File.WriteAllText(Path.Combine(output, "error.txt"), error.ToString());
            capture.Window.Close();
        }
    }

    private void Message(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        if (disposed || e.Source != Origin + Entry) return;
        long id = 0;
        try {
            applying = true;
            using var document = JsonDocument.Parse(e.TryGetWebMessageAsString());
            var request = document.RootElement;
            id = request.GetProperty("id").GetInt64();
            if (id < 0 || id > 9007199254740991) throw new InvalidDataException("Invalid request identity");
            var command = request.GetProperty("command").GetString();
            if (Finalized && command is not ("confirm" or "cancel" or "ready")) throw new InvalidOperationException("截图已复制，请关闭窗口");
            switch (command) {
                case "ready": ready = true; browserReady = true; view.Enabled = true; if (nativeToolbar is null) toolbarReadyMs = clock.Elapsed.TotalMilliseconds; break;
                case "tool": textTool = false; capture.SelectTool(request.GetProperty("tool").GetString() ?? ""); capture.Window.Focus(); break;
                case "undo": capture.Undo(); break;
                case "redo": capture.Redo(); break;
                case "text.begin":
                    textTool = true; capture.SelectTool("select"); capture.Window.Focus();
                    break;
                case "text.resize":
                    if (textDraft is not null && request.GetProperty("session").GetInt32() == textDraft.Id) {
                        var height = Math.Clamp(request.GetProperty("height").GetInt32(), 34, textDraft.MaxHeight);
                        textDraft = textDraft with { Bounds = new Rectangle(textDraft.Bounds.Location, new Size(textDraft.Bounds.Width, height)) };
                    }
                    break;
                case "text.cancel":
                    if (textDraft?.Target is Guid cancelled) capture.HideEditedText(cancelled, false);
                    textDraft = null; capture.SetTextEditing(false); capture.Window.Focus(); break;
                case "text.commit":
                    if (textDraft is null || request.GetProperty("session").GetInt32() != textDraft.Id) throw new InvalidOperationException("Text session changed");
                    var text = request.GetProperty("text").GetString() ?? "";
                    var textHeight = Math.Clamp(request.GetProperty("height").GetInt32(), 34, textDraft.MaxHeight);
                    textDraft = textDraft with { Bounds = new Rectangle(textDraft.Bounds.Location, new Size(textDraft.Bounds.Width, textHeight)) };
                    if (textDraft.Target is Guid target) capture.UpdateText(target, text, textDraft.Bounds);
                    else if (!string.IsNullOrWhiteSpace(text)) capture.AddText(text, textDraft.Bounds, textDraft.FontPixels);
                    textDraft = null; capture.SetTextEditing(false); capture.Window.Focus();
                    if (request.TryGetProperty("after", out var after)) {
                        if (after.GetString() == "save") Save();
                        else if (after.GetString() == "confirm") Confirm();
                    }
                    break;
                case "confirm": Confirm(); break;
                case "save":
                    if (textDraft is not null) throw new InvalidOperationException("Commit text before saving");
                    Save();
                    break;
                case "cancel": capture.Window.Close(); return;
                default: throw new InvalidDataException("Unsupported toolbar command");
            }
            applying = false; Publish(id);
        } catch (Exception error) { applying = false; Publish(id, error.Message); }
        finally { applying = false; }
    }

    private void Save() { capture.FinishGesture(); using var image = capture.RenderSelection(); saveImage(image); saved++; }
    private void SaveAs()
    {
        if (saveAsImage is null) throw new InvalidOperationException("另存为尚未连接");
        saveAsAttempts++;
        capture.FinishGesture();
        using var image = capture.RenderSelection();
        try { saveAsImage(image); savedAs++; }
        catch (OperationCanceledException) { } // Closing the picker keeps the screenshot open.
    }
    private void Confirm()
    {
        if (pinCompletion?.Committed == true) { pinCompletion.Complete(); return; }
        if (textDraft is not null) { RequestTextCommit("confirm"); return; }
        capture.FinishGesture();
        try { confirmation.Complete(); }
        catch { if (confirmation.Committed) capture.SetTextEditing(true); throw; }
    }
    private void BeginTextAt(Point point)
    {
        var selection = capture.Selection ?? throw new InvalidOperationException("请先选择截图区域");
        if (!selection.Contains(point)) throw new InvalidOperationException("请在截图选区内点击");
        var existing = capture.TextAt(point);
        var bounds = existing?.Bounds ?? NativeTextPlacement.NewBounds(selection, point);
        if (!selection.Contains(bounds)) throw new InvalidOperationException("文字超出截图选区");
        textDraft = new TextDraft(++textSequence, bounds, selection.Bottom - bounds.Top, existing?.Id, existing?.Text ?? "", existing?.FontPixels ?? 24);
        if (existing is not null) capture.HideEditedText(existing.Id, true);
        capture.SetTextEditing(true);
        if (nativeToolbar is not null) {
            using var background = capture.Render();
            nativeText = new NativeInlineText(capture.Window, background, textDraft.Bounds, textDraft.Value, textDraft.FontPixels);
            nativeText.ResizeToContent(textDraft.MaxHeight);
            nativeText.TextChanged += (_, _) => {
                if (nativeText is null || textDraft is null) return;
                nativeText.ResizeToContent(textDraft.MaxHeight); Publish(0);
            };
            nativeText.KeyDown += (_, key) => {
                if (nativeText?.IsComposing == true) return;
                if (key.KeyData == Keys.Escape) { key.SuppressKeyPress = true; CancelNativeText(); }
                else if (key.KeyData is (Keys.Control | Keys.Enter) or (Keys.Control | Keys.S) or (Keys.Control | Keys.Shift | Keys.S)) {
                    key.SuppressKeyPress = true;
                    try { RequestTextCommit(key.KeyData == (Keys.Control | Keys.S) ? "save" : key.KeyData == (Keys.Control | Keys.Shift | Keys.S) ? "saveAs" : null); }
                    catch (Exception error) { Publish(0, error.Message); }
                }
            };
            nativeText.MouseUp += (_, mouse) => { if (mouse.Button == MouseButtons.Right) capture.Window.Close(); };
            nativeText.BringToFront(); nativeText.Focus(); nativeText.SelectAll();
        }
    }

    private void RequestTextCommit(string? after = null)
    {
        if (nativeText is not null) {
            var input = nativeText;
            input.RequestCommit(() => CommitNativeText(input, after), error => Publish(0, error.Message)); return;
        }
        if (!browserReady) throw new InvalidOperationException("文字输入正在准备");
        view.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(new { kind = "commit-text", after }));
    }

    private void CommitNativeText(NativeInlineText input, string? after)
    {
        if (nativeText != input || textDraft is null) return;
        if (!input.Fits) throw new InvalidOperationException("文字超出截图范围，请缩短内容");
        var bounds = input.Bounds;
        if (textDraft.Target is Guid target) capture.UpdateText(target, input.Text, bounds, input.GetVisualLines());
        else if (!string.IsNullOrWhiteSpace(input.Text)) capture.AddText(input.Text, bounds, textDraft.FontPixels, input.GetVisualLines());
        nativeText = null; textDraft = null; input.Dispose(); capture.SetTextEditing(false);
        capture.Window.Focus();
        if (after == "save") Save();
        else if (after == "saveAs") SaveAs();
        else if (after == "pin") Pin();
        else if (after == "confirm") Confirm();
        Publish(0);
    }

    private void CancelNativeText()
    {
        if (textDraft?.Target is Guid target) capture.HideEditedText(target, false);
        var input = nativeText; nativeText = null; textDraft = null; input?.Dispose();
        capture.SetTextEditing(false); capture.Window.Focus(); Publish(0);
    }

    private void NativeCommand(string command)
    {
        if (command == "pin") {
            try { Pin(); Publish(0); } catch (Exception error) { Publish(0, error.Message); }
            return;
        }
        switch (command) {
            case "confirm": OnKeyPressed(Keys.Enter); return;
            case "save": OnKeyPressed(Keys.Control | Keys.S); return;
            case "saveAs": OnKeyPressed(Keys.Control | Keys.Shift | Keys.S); return;
            case "undo": OnKeyPressed(Keys.Control | Keys.Z); return;
            case "redo": OnKeyPressed(Keys.Control | Keys.Shift | Keys.Z); return;
            case "text": OnKeyPressed(Keys.T); return;
            case "cancel": capture.Window.Close(); return;
        }
        if (textDraft is not null || Finalized) return;
        textTool = false; capture.SelectTool(command); capture.Window.Focus(); Publish(0);
    }
    private void Pin()
    {
        if (pinCompletion is null) throw new InvalidOperationException("截图置顶尚未连接");
        if (textDraft is not null) { RequestTextCommit("pin"); return; }
        capture.FinishGesture(); pinCompletion.Complete();
    }

    private void OnKeyPressed(Keys key)
    {
        if (!ready || disposed) return;
        try {
            applying = true;
            if (key == Keys.Escape) {
                if (textDraft is not null) {
                    if (nativeText is not null) { CancelNativeText(); return; }
                    if (textDraft.Target is Guid target) capture.HideEditedText(target, false);
                    textDraft = null; capture.SetTextEditing(false);
                } else capture.Window.Close();
            } else if (key is Keys.Enter or (Keys.Control | Keys.C)) Confirm();
            else if (!Finalized) {
                if (key is (Keys.Control | Keys.S) or (Keys.Control | Keys.Shift | Keys.S)) {
                    var saveAs = key == (Keys.Control | Keys.Shift | Keys.S);
                    if (textDraft is not null) RequestTextCommit(saveAs ? "saveAs" : "save");
                    else if (saveAs) SaveAs(); else Save();
                } else if (textDraft is null) {
                    if (key == (Keys.Control | Keys.Z)) capture.Undo();
                    else if (key is (Keys.Control | Keys.Y) or (Keys.Control | Keys.Shift | Keys.Z)) capture.Redo();
                    else if (key == Keys.T) { textTool = true; capture.SelectTool("select"); }
                }
            }
            applying = false; Publish(0);
        } catch (Exception error) { applying = false; Publish(0, error.Message); }
        finally { applying = false; }
    }
    private bool FilterTextInput(Message message)
    {
        if (message.Msg == 0x0205) { capture.Window.Close(); return true; }
        if (message.Msg == 0x0203) {
            try { Confirm(); Publish(0); } catch (Exception error) { Publish(0, error.Message); }
            return true;
        }
        if (Finalized && message.Msg == 0x0100) {
            OnKeyPressed((Keys)message.WParam.ToInt32() | Control.ModifierKeys); return true;
        }
        if (Finalized && message.Msg is >= 0x0100 and <= 0x020e) return true;
        if (textTool && textDraft is null && message.Msg == 0x0201) {
            try { BeginTextAt(NativeTextPlacement.PointFromMessage(message.LParam)); Publish(0); }
            catch (Exception error) { Publish(0, error.Message); }
            return true;
        }
        if (textDraft is null) {
            if (!textTool && message.Msg == 0x0201) capture.BeginCover(NativeTextPlacement.PointFromMessage(message.LParam));
            return false;
        }
        if (message.Msg == 0x0201) {
            try { if (nativeText is not null || browserReady) RequestTextCommit(); }
            catch (Exception error) { Publish(0, error.Message); }
            return true;
        }
        return message.Msg is 0x0202 or 0x0203;
    }
    private void OnChanged() { if (!applying) Publish(0); }
    private void OnMouseUp(object? sender, MouseEventArgs e) => Publish(0);
    private object? ReadInputRegion(Rectangle viewport, Rectangle toolbar)
    {
        if (textDraft is null || !view.IsHandleCreated) return null;
        var region = CreateRectRgn(0, 0, 0, 0);
        if (region == IntPtr.Zero) throw new InvalidOperationException("Could not inspect native control region");
        try {
            if (GetWindowRgn(view.Handle, region) == 0) throw new InvalidOperationException("Native control region is missing");
            var text = textDraft.Bounds;
            var gapY = (text.Bottom + 2 + toolbar.Top) / 2;
            return new {
                textInside = PtInRegion(region, text.Left + 4 - viewport.X, text.Top + 4 - viewport.Y),
                toolbarInside = PtInRegion(region, toolbar.Left + 4 - viewport.X, toolbar.Top + 4 - viewport.Y),
                gapOutside = gapY > text.Bottom + 2 && gapY < toolbar.Top && !PtInRegion(region, text.Left + 4 - viewport.X, gapY - viewport.Y)
            };
        } finally { DeleteObject(region); }
    }
    private void Publish(long requestId, string? error = null)
    {
        if (disposed) return;
        var selection = capture.Selection;
        var toolbar = Rectangle.Empty;
        var viewport = Rectangle.Empty;
        if (nativeToolbar is not null) nativeToolbar.Visible = selection is not null;
        selectionHint.Visible = nativeToolbar is not null && selection is null && error is not null;
        if (selectionHint.Visible) {
            var screen = Screen.FromPoint(Cursor.Position).WorkingArea;
            var area = new Rectangle(capture.Window.PointToClient(screen.Location), screen.Size);
            selectionHint.MaximumSize = new Size(Math.Max(1, area.Width - 32), 0);
            selectionHint.Text = error;
            selectionHint.Location = new Point(area.Left + (area.Width - selectionHint.Width) / 2, area.Bottom - selectionHint.Height - 24);
            selectionHint.BringToFront();
        }
        if (selection is { } selected) {
            var width = Math.Min(980, capture.Window.ClientSize.Width - 16);
            var toolbarHeight = error is null ? 104 : 140;
            toolbar = new Rectangle(Math.Clamp(selected.Left, 8, Math.Max(8, capture.Window.ClientSize.Width - width - 8)),
                Math.Clamp(selected.Bottom + 8, 8, Math.Max(8, capture.Window.ClientSize.Height - toolbarHeight - 8)), width, toolbarHeight);
            if (nativeToolbar is not null) {
                var center = toolbarPosition ?? new Point(selected.Left + selected.Width / 2, selected.Top + selected.Height / 2);
                var screen = Screen.FromPoint(capture.Window.PointToScreen(center)).WorkingArea;
                var viewportBounds = new Rectangle(capture.Window.PointToClient(screen.Location), screen.Size);
                toolbar = ToolbarPlacement.Place(selected, viewportBounds, nativeToolbar.PreferredToolbarSize, toolbarPosition);
                nativeToolbar.Bounds = toolbar;
                nativeToolbar.SetState(capture.CanUndo, capture.CanRedo, textTool ? "text" : capture.SelectedTool, Finalized, textDraft is not null);
                nativeToolbar.Visible = true;
            }
            viewport = nativeToolbar is null ? toolbar : Rectangle.Empty;
            if (textDraft is not null) viewport = nativeToolbar is null ? Rectangle.Union(viewport, Rectangle.Inflate(textDraft.Bounds, 2, 2)) : Rectangle.Inflate(textDraft.Bounds, 2, 2);
            view.Bounds = viewport;
            // Only actual controls participate in painting and hit testing.
            // The space between toolbar and text remains the native canvas.
            var region = new Region(); region.MakeEmpty();
            if (nativeToolbar is null) region.Union(new Rectangle(toolbar.X - viewport.X, toolbar.Y - viewport.Y, toolbar.Width, toolbar.Height));
            if (textDraft is not null) { var textBounds = Rectangle.Inflate(textDraft.Bounds, 2, 2); textBounds.Offset(-viewport.X, -viewport.Y); region.Union(textBounds); }
            var previous = view.Region; view.Region = region; previous?.Dispose();
        }
        var currentWindow = capture.Window.IsHandleCreated ? capture.Window.Handle.ToInt64().ToString() : "0";
        var toolbarControl = (Control?)nativeToolbar ?? view;
        var toolbarRoot = toolbarControl.IsHandleCreated ? GetAncestor(toolbarControl.Handle, 2).ToInt64().ToString() : "0";
        var state = JsonSerializer.Serialize(new {
            kind = "state", requestId, nativeTheme = nativeDark ? "dark" : "light", nativeControls = nativeToolbar is not null, browserReady, nativeProcess = Environment.ProcessId, nativeDesktop = CurrentDesktopName(), nativeWindow = currentWindow, initialNativeWindow = nativeWindow, toolbarRoot, annotations = capture.AnnotationCount,
            canUndo = capture.CanUndo, canRedo = capture.CanRedo, tool = capture.Tool, selectedTool = capture.SelectedTool, textTool, copied = confirmation.Committed, closeRequested = confirmation.CloseRequested || Pinned, pinned = Pinned, error,
            hasSelection = selection is not null, nativeToolbarVisible = nativeToolbar?.Visible,
            windowCandidates = capture.ActiveWindowCandidates,
            selectionHintVisible = selectionHint.Visible,
            nativeForms = Application.OpenForms.Count, inputRegion = ReadInputRegion(viewport, toolbar),
            viewWidth = viewport.Width, viewX = viewport.X, viewY = viewport.Y,
            toolbar = new { x = toolbar.X - viewport.X, y = toolbar.Y - viewport.Y, width = toolbar.Width, height = toolbar.Height },
            text = textDraft is null ? null : new { id = textDraft.Id, x = textDraft.Bounds.X - viewport.X, y = textDraft.Bounds.Y - viewport.Y,
                width = textDraft.Bounds.Width, height = textDraft.Bounds.Height, value = textDraft.Value, fontPixels = textDraft.FontPixels, existing = textDraft.Target.HasValue, imageWidth = capture.Window.ClientSize.Width - viewport.X,
                imageHeight = textDraft.Bounds.Top + textDraft.MaxHeight - viewport.Y },
            firstPaintMs, initializationStartedMs, environmentReadyMs, controllerReadyMs, domReadyMs, navigationReadyMs, toolbarReadyMs, visibleInitialization, saved, savedAs, saveAsAttempts,
            nativeTextWindow = nativeText?.IsHandleCreated == true ? nativeText.Handle.ToInt64().ToString() : null,
            nativeTextValue = nativeText?.Text,
            nativeGrip = nativeToolbar?.DescribeGrip(), nativeButtons = nativeToolbar?.DescribeButtons(), startupStages, launchToEntryMs, launchToFirstPaintMs = launchToEntryMs + firstPaintMs,
            launchToToolbarReadyMs = launchToEntryMs + toolbarReadyMs,
        });
        WinOne.Capture.SettingsStore.AtomicWrite(Path.Combine(output, "state.json"), Encoding.UTF8.GetBytes(state));
        if (!browserReady) return;
        if (selection is not null && (nativeToolbar is null || textDraft is not null)) {
            view.Visible = true; view.BringToFront();
        } else view.Visible = false;
        view.CoreWebView2.PostWebMessageAsJson(state);
    }

    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        capture.Changed -= OnChanged; capture.Window.MouseUp -= OnMouseUp;
        capture.KeyPressed -= OnKeyPressed;
        nativeToolbar?.Dispose();
        nativeText?.Dispose();
        selectionHint.Dispose();
        paint.ReleaseHandle(); view.Dispose();
    }

    private sealed class HintLabel : Label
    {
        // The hint is informative; pointer input must still reach the canvas.
        protected override void WndProc(ref Message message)
        {
            if (message.Msg == 0x0084) { message.Result = new IntPtr(-1); return; }
            base.WndProc(ref message);
        }
    }

    private sealed class FirstPaintObserver : NativeWindow
    {
        private readonly Action callback;
        private readonly Func<Message, bool> input;
        private bool fired;
        public FirstPaintObserver(Form window, Action callback, Func<Message, bool> input) { this.callback = callback; this.input = input; AssignHandle(window.Handle); }
        protected override void WndProc(ref Message message)
        {
            var painting = message.Msg == 0x000f;
            if (input(message)) return;
            base.WndProc(ref message);
            if (painting && !fired) { fired = true; callback(); }
        }
    }
}
