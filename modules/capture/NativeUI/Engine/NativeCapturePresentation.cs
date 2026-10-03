using System.Drawing;
using System.Drawing.Drawing2D;
using System.Windows.Forms;
using ShareX.HelpersLib;

namespace ShareX.ScreenCaptureLib
{
    public sealed partial class RegionCaptureForm
    {
        internal bool WinOnePresentation { get; private set; }
        internal void EnableWinOnePresentation()
        {
            WinOnePresentation = true;
            borderPen.Color = SystemColors.Highlight;
            borderPen.Width = 1;
            SetDefaultCursor();
            Shown += (_, _) => { RefreshWinOneInput(); Invalidate(); };
        }
        internal void RefreshWinOneInput()
        {
            if (!WinOnePresentation || IsDisposed || ShapeManager == null) return;
            UpdateCoordinates();
            ShapeManager.UpdateObjects();
            if (ShapeManager.IsPanning) PanToOffset(new Vector2(CanvasCenterOffset.X + InputManager.MouseVelocity.X, CanvasCenterOffset.Y + InputManager.MouseVelocity.Y));
            ShapeManager.Update();
            if (!ShapeManager.IsMoving && !ShapeManager.IsPanning && !ShapeManager.IsCursorOnObject)
                SetDefaultCursor();
        }
        private void SetWinOneDefaultCursor()
        {
            if (SetWinOneResizeCursor()) return;
            // A press alone still permits a double click. Show precision input
            // only after an actual drawing/selection drag has begun.
            var bounds = ShapeManager?.CurrentRectangle ?? RectangleF.Empty;
            Cursor = ShapeManager?.IsCreating == true && (bounds.Width > 1 || bounds.Height > 1)
                ? Cursors.Cross : Cursors.Default;
        }
        private bool SetWinOneResizeCursor()
        {
            if (ShapeManager == null || !ShapeManager.NodesVisible) return false;
            var nodes = ShapeManager.ResizeNodes;
            // The active handle keeps its direction even when a drag takes
            // the pointer outside its hit area. Hover is used only when idle.
            var index = System.Array.FindIndex(nodes, node => node.Visible && node.IsDragging);
            if (index < 0) index = System.Array.FindIndex(nodes, node => node.Visible && node.IsCursorHover);
            var cursor = (NodePosition)index switch {
                NodePosition.Left or NodePosition.Right => Cursors.SizeWE,
                NodePosition.Top or NodePosition.Bottom => Cursors.SizeNS,
                NodePosition.TopLeft or NodePosition.BottomRight => Cursors.SizeNWSE,
                NodePosition.TopRight or NodePosition.BottomLeft => Cursors.SizeNESW,
                _ => null
            };
            if (cursor == null) return false;
            Cursor = cursor; return true;
        }
        protected override void WndProc(ref Message message)
        {
            bool pointer = message.Msg is >= 0x0200 and <= 0x020e;
            bool keyboard = message.Msg is >= 0x0100 and <= 0x0105;
            bool update = WinOnePresentation && (pointer || keyboard);
            if (!update) { base.WndProc(ref message); return; }
            // A queued mouse message describes an earlier point/button state.
            // Polling the current cursor here can collapse a fast drag to zero.
            var point = InputManager.ClientMousePosition;
            var buttons = MouseButtons.None;
            if (pointer) {
                var packed = message.LParam.ToInt64();
                point = new Point((short)(packed & 0xffff), (short)((packed >> 16) & 0xffff));
                if (message.Msg is 0x020a or 0x020e) point = PointToClient(point);
                var flags = message.WParam.ToInt64();
                if ((flags & 1) != 0) buttons |= MouseButtons.Left;
                if ((flags & 2) != 0) buttons |= MouseButtons.Right;
                if ((flags & 16) != 0) buttons |= MouseButtons.Middle;
                if ((flags & 32) != 0) buttons |= MouseButtons.XButton1;
                if ((flags & 64) != 0) buttons |= MouseButtons.XButton2;
            } else {
                foreach (var button in new[] { MouseButtons.Left, MouseButtons.Right, MouseButtons.Middle, MouseButtons.XButton1, MouseButtons.XButton2 })
                    if (InputManager.IsMouseDown(button)) buttons |= button;
            }
            InputManager.BeginWinOneEvent(this, point, buttons);
            try {
                RefreshWinOneInput();
                base.WndProc(ref message);
                if (!IsDisposed) {
                    InputManager.ConsumeWinOneEvent();
                    if (message.Msg is 0x0201 or 0x0202 or 0x0207 or 0x0208) RefreshWinOneInput();
                    // Repaint on changes; a self-invalidating OnPaint starves
                    // native child controls of their first paint.
                    Invalidate();
                }
            } finally {
                InputManager.EndWinOneEvent();
            }
        }
        private void DrawWinOneCandidate(Graphics graphics)
        {
            if (IsEditorMode || backgroundHighlightBrush == null || !ShapeManager.IsCurrentShapeTypeRegion ||
                ShapeManager.ProbeCommittedRegion != null || !ShapeManager.IsCurrentHoverShapeValid) return;
            using var path = new GraphicsPath();
            ShapeManager.CurrentHoverShape.AddShapePath(path);
            var state = graphics.Save();
            try {
                graphics.SetClip(path, CombineMode.Intersect);
                graphics.FillRectangle(backgroundHighlightBrush, ClientArea);
            } finally { graphics.Restore(state); }
        }
    }
}
