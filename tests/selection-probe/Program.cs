using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;

// Explicit own-window integration fixture; never selects text in another app.
internal static class Program
{
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("user32.dll")] static extern uint GetClipboardSequenceNumber();
    [STAThread] static int Main(string[] args)
    {
        if(args is ["--pipeline",var pipelineOutput]) return SelectionPipelineProbe.Run(pipelineOutput);
        if (args is not ["--reader", var reader, "--output", var output]) return 2;
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        using var form = new Form { Text = "WinOne selection test", Width = 620, Height = 300, TopMost = true };
        using var text = new TextBox { Multiline = true, Text = "Before 选中的文字 after\r\nSecond line", Dock = DockStyle.Top, Height = 100 };
        using var password = new TextBox { Text = "private-test-value", UseSystemPasswordChar = true, Dock = DockStyle.Bottom };
        form.Controls.Add(text); form.Controls.Add(password);
        Exception? failure = null;
        var results = new List<object>();
        form.Shown += async (_, _) => {
            try {
                var formHandle = form.Handle;
                await Task.Run(() => System.Windows.Automation.AutomationElement.FromHandle(formHandle).SetFocus());
                if (!SetForegroundWindow(formHandle) && GetForegroundWindow()!=formHandle) throw new Exception("Owned selection fixture could not become foreground");
                text.Focus();
                text.Select(7, 6);
                var deadline = DateTime.UtcNow.AddSeconds(3);
                while (GetForegroundWindow() != form.Handle) {
                    if (DateTime.UtcNow > deadline) throw new Exception($"Owned fixture did not receive foreground focus: expected={form.Handle}, actual={GetForegroundWindow()}");
                    await Task.Delay(10);
                }
                var clipboardBefore = GetClipboardSequenceNumber();
                async Task<JsonElement> Read(string name, uint? overridePid = null, int[]? point = null, int[]? origin = null) {
                    var info = new ProcessStartInfo(reader) { UseShellExecute = false, CreateNoWindow = true,
                        RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true,
                        StandardInputEncoding = new UTF8Encoding(false), StandardOutputEncoding = new UTF8Encoding(false) };
                    var clock = Stopwatch.StartNew();
                    info.Environment["WINONE_SELECTION_DIAGNOSTICS"] = "1";
                    using var process = Process.Start(info) ?? throw new Exception("Reader did not start");
                    await process.StandardInput.WriteAsync(JsonSerializer.Serialize(new { window = (ulong)form.Handle, pid = overridePid ?? (uint)Environment.ProcessId, point, origin }));
                    process.StandardInput.Close();
                    var stdout = process.StandardOutput.ReadToEndAsync();
                    var stderr = process.StandardError.ReadToEndAsync();
                    using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(4));
                    try { await process.WaitForExitAsync(timeout.Token); }
                    catch { if (!process.HasExited) process.Kill(true); throw; }
                    if (process.ExitCode != 0) throw new Exception("Reader failed: " + await stderr);
                    File.WriteAllText(Path.Combine(output, name + "-metadata.log"), await stderr);
                    using var document = JsonDocument.Parse(await stdout);
                    var result = document.RootElement.Clone();
                    results.Add(new { name, milliseconds = clock.Elapsed.TotalMilliseconds, result, sourceSelectionStart = text.SelectionStart, sourceSelectionLength = text.SelectionLength, sourceFocused = text.Focused, sourceHandle = text.Handle.ToInt64() });
                    return result;
                }
                for (var i = 0; i < 4; i++) {
                    var selected = await Read("text-" + i);
                    if (selected.GetProperty("status").GetString() != "selected" || selected.GetProperty("text").GetString() != "选中的文字 ")
                        throw new Exception("Exact selected text was not returned: " + selected);
                }
                var inside=text.PointToScreen(text.GetPositionFromCharIndex(9));
                if ((await Read("pointer-in-selection",point:new[]{inside.X+1,inside.Y+4})).GetProperty("status").GetString() != "selected") throw new Exception("Pointer inside selected text was rejected");
                var outside=text.PointToScreen(new Point(text.Width-6,text.Height-6));
                var unrelated=await Read("pointer-outside-selection",point:new[]{outside.X,outside.Y});
                if (unrelated.GetProperty("status").GetString() != "unrelated" || unrelated.TryGetProperty("text",out _)) throw new Exception("Pointer outside selected text exposed the selection");
                text.Select(0, 0);
                if ((await Read("caret-only")).GetProperty("status").GetString() == "selected") throw new Exception("Caret exposed whole document");
                password.Focus(); password.SelectAll();
                var secret = await Read("password");
                if (secret.GetProperty("status").GetString() != "sensitive" || secret.TryGetProperty("text", out _)) throw new Exception("Password selection was not rejected");
                text.Focus(); text.SelectAll();
                if ((await Read("wrong-source", 1)).GetProperty("status").GetString() != "stale") throw new Exception("Wrong source process accepted");
                if (Environment.GetEnvironmentVariable("WINONE_SELECTION_NATIVE_ONLY") != "1") {
                // A separate Chromium process tests embedded-browser ownership,
                // not only the same-process WinForms accessibility provider.
                using var browser = new Microsoft.Web.WebView2.WinForms.WebView2 { Dock = DockStyle.Fill };
                form.Controls.Add(browser); browser.BringToFront();
                var environment = await Microsoft.Web.WebView2.Core.CoreWebView2Environment.CreateAsync(null, Path.GetFullPath(Path.Combine(output, "browser-profile")));
                await browser.EnsureCoreWebView2Async(environment);
                var loaded = new TaskCompletionSource();
                browser.CoreWebView2.NavigationCompleted += (_, e) => { if (e.IsSuccess) loaded.TrySetResult(); else loaded.TrySetException(new Exception("Fixture page failed")); };
                browser.NavigateToString("<html><body><p tabindex='0' id='source'>Browser selected text</p><p id='mixed'>Before <a id='link' href='#local'>linked text</a> after</p><button id='other'>Other focus</button><input id='password' type='password' value='browser-secret'></body></html>");
                await loaded.Task.WaitAsync(TimeSpan.FromSeconds(10));
                browser.Focus();
                var browserSelected = await browser.CoreWebView2.ExecuteScriptAsync("(()=>{const p=document.querySelector('#source');p.focus();const r=document.createRange();r.setStart(p.firstChild,8);r.setEnd(p.firstChild,16);const s=getSelection();s.removeAllRanges();s.addRange(r);return s.toString();})()");
                if (browserSelected != "\"selected\"") throw new Exception("Browser fixture did not establish the intended selection");
                var browserText = await Read("browser-text");
                var browserAgain = await Read("browser-text-again");
                if (browserText.GetProperty("status").GetString() != "selected" || browserText.GetProperty("text").GetString() != "selected") throw new Exception("Browser selection failed: " + browserText);
                if (browserAgain.GetProperty("status").GetString() != "selected" || browserAgain.GetProperty("text").GetString() != "selected") throw new Exception("Repeated browser selection changed");
                // Real Chromium UIA: a focused inline hyperlink must not hide
                // or truncate a document range that crosses the link boundary.
                foreach (var scenario in new[] { "across-link", "inside-link", "pointer-other-focus" }) {
                    var setup = await browser.CoreWebView2.ExecuteScriptAsync("(()=>{const p=document.querySelector('#mixed'),a=document.querySelector('#link');a.focus();const r=document.createRange();" +
                        (scenario == "inside-link" ? "r.setStart(a.firstChild,0);r.setEnd(a.firstChild,6);" : "r.selectNodeContents(p);") +
                        "const s=getSelection();s.removeAllRanges();s.addRange(r);" + (scenario == "pointer-other-focus" ? "document.querySelector('#other').focus();" : "") +
                        "const b=r.getBoundingClientRect(),d=devicePixelRatio;return {text:s.toString(),x:(b.left+3)*d,y:(b.top+b.height/2)*d};})()");
                    using var selectedRange = JsonDocument.Parse(setup);
                    var expected = selectedRange.RootElement.GetProperty("text").GetString();
                    var startPoint = browser.PointToScreen(new Point((int)selectedRange.RootElement.GetProperty("x").GetDouble(), (int)selectedRange.RootElement.GetProperty("y").GetDouble()));
                    var hit = new[] { startPoint.X, startPoint.Y };
                    var value = await Read("browser-" + scenario, point: hit, origin: hit);
                    if (value.GetProperty("status").GetString() != "selected" || value.GetProperty("text").GetString() != expected)
                        throw new Exception("Hyperlink selection failed: " + scenario + " " + value);
                }
                await browser.CoreWebView2.ExecuteScriptAsync("getSelection().removeAllRanges();document.querySelector('#link').focus()");
                if ((await Read("browser-link-without-selection")).GetProperty("status").GetString() == "selected")
                    throw new Exception("Focusing a hyperlink exposed unselected text");
                var browserPassword = await browser.CoreWebView2.ExecuteScriptAsync("(()=>{getSelection().removeAllRanges();const p=document.querySelector('#password');p.focus();p.select();return document.activeElement.id;})()");
                if (browserPassword != "\"password\"") throw new Exception("Browser fixture did not focus the password field");
                if ((await Read("browser-password")).GetProperty("status").GetString() != "sensitive") throw new Exception("Browser password not rejected");
                }
                if (GetClipboardSequenceNumber() != clipboardBefore) throw new Exception("Reader changed clipboard");
                File.WriteAllText(Path.Combine(output, "results.json"), JsonSerializer.Serialize(new { passed = true, browserSkipped = Environment.GetEnvironmentVariable("WINONE_SELECTION_NATIVE_ONLY") == "1", clipboardUnchanged = true, samples = results }, new JsonSerializerOptions { WriteIndented = true }), new UTF8Encoding(false));
            } catch (Exception error) {
                File.WriteAllText(Path.Combine(output, "partial-results.json"), JsonSerializer.Serialize(results, new JsonSerializerOptions { WriteIndented = true }), new UTF8Encoding(false));
                failure = error;
            }
            finally { form.Close(); }
        };
        Application.Run(form);
        if (failure is not null) { File.WriteAllText(Path.Combine(output, "failure.txt"), failure.ToString()); Console.Error.WriteLine(failure); return 1; }
        Console.WriteLine("PASS: exact selected text, empty selection, password, stale source, unchanged clipboard and reader exit");
        return 0;
    }
}
