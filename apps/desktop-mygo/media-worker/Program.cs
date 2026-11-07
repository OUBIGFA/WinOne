// Native Windows media decoding uses the operating system's WPF MediaPlayer.
// The helper owns no UI chrome, account, settings or shell. Its only input is
// a Host-issued read-only loopback capability; no user file is opened directly.
using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Threading;
using System.Web.Script.Serialization;
using System.Windows;
using System.Windows.Interop;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using System.Windows.Threading;

internal static class Program {
    private static readonly JavaScriptSerializer Json = new JavaScriptSerializer { MaxJsonLength = 32 * 1024 * 1024, RecursionLimit = 32 };
    private static MediaPlayer Player;
    private static DrawingVisual Visual;
    private static HwndSource Surface;
    private static Dispatcher UI;
    private static bool Opened, Playing, Ended, Muted;
    private static double Volume = 1, Rate = 1;
    private static string Error = "";
    private static int LastWidth, LastHeight;
    private static long LastPosition = -1;

    [STAThread]
    private static void Main() {
        UI = Dispatcher.CurrentDispatcher;
        Player = new MediaPlayer { ScrubbingEnabled = true };
        Visual = new DrawingVisual();
        var options = new HwndSourceParameters("WinOne native media decoder") { Width = 1, Height = 1, WindowStyle = unchecked((int)0x80000000), PositionX = -32000, PositionY = -32000 };
        Surface = new HwndSource(options) { RootVisual = Visual };
        Player.MediaOpened += (sender, args) => { Opened = true; Player.SpeedRatio = Rate; Player.Volume = Muted ? 0 : Volume; if (Playing) Player.Play(); else Player.Pause(); };
        Player.MediaEnded += (sender, args) => { Ended = true; Playing = false; };
        Player.MediaFailed += (sender, args) => { Error = "MEDIA_READ_FAILED"; Playing = false; };
        Console.Out.WriteLine("{\"ready\":true,\"engine\":\"Windows.MediaPlayer\"}");
        Console.Out.Flush();
        var reader = new Thread(ReadCommands) { IsBackground = true };
        reader.Start();
        try { Dispatcher.Run(); }
        finally { Player.Close(); Surface.Dispose(); }
    }

    private static void ReadCommands() {
        try {
            string line;
            while ((line = Console.In.ReadLine()) != null) {
                if (line.Length > 65536) throw new InvalidDataException("REQUEST_TOO_LARGE");
                var request = Json.Deserialize<Dictionary<string, object>>(line);
                var reply = UI.Invoke(new Func<Dictionary<string, object>>(() => Handle(request)));
                Console.Out.WriteLine(Json.Serialize(reply));
                Console.Out.Flush();
                if (Text(request, "method") == "close") break;
            }
        } catch { /* Pipe failure ends the owned decoder, never leaves an idle process. */ }
        finally { UI.BeginInvokeShutdown(DispatcherPriority.Background); }
    }

    private static Dictionary<string, object> Handle(Dictionary<string, object> request) {
        var reply = new Dictionary<string, object> { { "id", Number(request, "id", 0) } };
        try {
            var method = Text(request, "method");
            switch (method) {
            case "open":
                var address = new Uri(Text(request, "url"), UriKind.Absolute);
                if (address.Scheme != "http" || address.Host != "127.0.0.1" || !String.IsNullOrEmpty(address.UserInfo)) throw new InvalidDataException("INVALID_MEDIA_CAPABILITY");
                Player.Close(); Opened = Playing = Ended = false; Error = ""; LastPosition = -1;
                Player.Open(address);
                break;
            case "play":
                if (Ended) { Player.Position = TimeSpan.Zero; Ended = false; }
                Playing = true; Player.Play();
                break;
            case "pause": Playing = false; Player.Pause(); break;
            case "seek": Player.Position = TimeSpan.FromSeconds(Math.Max(0, Number(request, "position", 0))); Ended = false; LastPosition = -1; break;
            case "volume": Volume = Math.Max(0, Math.Min(1, Number(request, "volume", 1))); Player.Volume = Muted ? 0 : Volume; break;
            case "mute": Muted = Boolean(request, "muted"); Player.Volume = Muted ? 0 : Volume; break;
            case "rate": Rate = Number(request, "rate", 1); if (Rate < 0.25 || Rate > 4) throw new InvalidDataException("INVALID_PLAYBACK_RATE"); Player.SpeedRatio = Rate; break;
            case "status": break;
            case "close": Player.Close(); Playing = false; break;
            default: throw new InvalidDataException("UNKNOWN_MEDIA_OPERATION");
            }
            var position = Player.Position;
            reply["opened"] = Opened; reply["playing"] = Playing; reply["ended"] = Ended;
            reply["position"] = position.TotalSeconds;
            reply["duration"] = Player.NaturalDuration.HasTimeSpan ? Player.NaturalDuration.TimeSpan.TotalSeconds : 0;
            reply["volume"] = Volume; reply["muted"] = Muted; reply["rate"] = Rate;
            reply["videoWidth"] = Player.NaturalVideoWidth; reply["videoHeight"] = Player.NaturalVideoHeight;
            if (Error != "") reply["error"] = Error;
            if (method == "status" && Boolean(request, "frame") && Opened && Player.NaturalVideoWidth > 0) {
                var width = (int)Math.Max(1, Math.Min(1024, Number(request, "width", 640)));
                var height = (int)Math.Max(1, Math.Min(1024, Number(request, "height", 480)));
                if (position.Ticks != LastPosition || width != LastWidth || height != LastHeight) {
                    using (var drawing = Visual.RenderOpen()) drawing.DrawVideo(Player, new Rect(0, 0, width, height));
                    var bitmap = new RenderTargetBitmap(width, height, 96, 96, PixelFormats.Pbgra32);
                    bitmap.Render(Visual);
                    var png = new PngBitmapEncoder(); png.Frames.Add(BitmapFrame.Create(bitmap));
                    using (var stream = new MemoryStream()) { png.Save(stream); reply["frame"] = Convert.ToBase64String(stream.ToArray()); }
                    LastPosition = position.Ticks; LastWidth = width; LastHeight = height;
                }
            }
        } catch { reply["error"] = "MEDIA_OPERATION_FAILED"; }
        return reply;
    }
    private static string Text(Dictionary<string, object> value, string key) { object result; return value.TryGetValue(key, out result) && result != null ? Convert.ToString(result, CultureInfo.InvariantCulture) : ""; }
    private static double Number(Dictionary<string, object> value, string key, double fallback) { object result; return value.TryGetValue(key, out result) && result != null ? Convert.ToDouble(result, CultureInfo.InvariantCulture) : fallback; }
    private static bool Boolean(Dictionary<string, object> value, string key) { object result; return value.TryGetValue(key, out result) && result is bool && (bool)result; }
}
