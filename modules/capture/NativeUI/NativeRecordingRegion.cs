using System.Drawing.Drawing2D;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// Reuse ShareX's transparent region window and its outside-pixel geometry.
// Only adapt the border to WinOne's grayscale palette, non-activation and
// WDA_EXCLUDEFROMCAPTURE. Painting follows ScrollingCaptureRegionForm (GPL,
// ShareX Team 2007–2026); it adds no timer or separate resident process.
internal sealed class NativeRecordingRegion : ScrollingCaptureRegionForm
{
    internal NativeRecordingRegion(Rectangle region) : base(region)
    {
        // Selection coordinates are already physical. The upstream designer's
        // font autoscaling must not resize the outline at non-default DPI.
        AutoScaleMode = AutoScaleMode.None;
        Text = "WinOne";
        Bounds = Rectangle.Inflate(region, 1, 1);
        // A full-size color-keyed layered window can make WGC exclude the
        // surface beneath its transparent interior as well. Give the HWND a
        // real hole: only the outside border exists in DWM/hit-test geometry.
        // This also avoids covering any recorded pixel with a layered surface.
        TransparencyKey = Color.Empty;
        var outline = new Region(ClientRectangle);
        outline.Exclude(new Rectangle(1, 1, region.Width, region.Height));
        Region = outline;
    }
    protected override bool ShowWithoutActivation => true;
    protected override CreateParams CreateParams {
        get { var value = base.CreateParams; value.ExStyle |= 0x08000000; return value; }
    }
    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        RecordingWindowProtection.Exclude(Handle);
    }
    protected override void OnPaint(PaintEventArgs e)
    {
        using var dark = new Pen(Color.FromArgb(32, 32, 32)) { DashPattern = [5, 5] };
        using var light = new Pen(Color.White) { DashPattern = [5, 5], DashOffset = 5 };
        e.Graphics.DrawRectangleProper(dark, ClientRectangle);
        e.Graphics.DrawRectangleProper(light, ClientRectangle);
    }
}
