using System;
using System.Collections.Generic;
using System.Drawing;
using System.Windows.Forms;

namespace ShareX.ScreenCaptureLib
{
    internal partial class ShapeManager
    {
        internal void SnapWinOneSelectionEdge(BaseShape shape)
        {
            // Only resize handles snap. Free selection and annotation gestures
            // retain their native geometry, including proportional resizing.
            if (!Form.WinOnePresentation || shape.ShapeCategory != ShapeCategory.Region ||
                IsProportionalResizing || IsCornerMoving || IsPanning) return;
            var node = Array.FindIndex(ResizeNodes, item => item.IsDragging);
            if (node < 0 || node >= 8) return;
            var position = (NodePosition)node;
            var bounds = shape.Rectangle;
            var distance = 6f * Form.DeviceDpi / 96f / Form.ZoomFactor;
            var candidates = new List<RectangleF> { Form.CanvasRectangle };
            if (!Form.IsEditorMode) {
                if (Windows != null) foreach (var window in Windows)
                    candidates.Add(RectangleF.Intersect(Form.CanvasRectangle, Form.RectangleToClient(window.Rectangle)));
                foreach (var screen in Screen.AllScreens)
                    candidates.Add(RectangleF.Intersect(Form.CanvasRectangle, Form.RectangleToClient(screen.Bounds)));
            }
            float Closest(float value, bool vertical)
            {
                var best = value; var delta = distance;
                foreach (var target in candidates) {
                    if (target.Width <= 0 || target.Height <= 0) continue;
                    if (vertical ? target.Bottom <= bounds.Top || target.Top >= bounds.Bottom
                        : target.Right <= bounds.Left || target.Left >= bounds.Right) continue;
                    foreach (var edge in vertical ? new[] { target.Left, target.Right } : new[] { target.Top, target.Bottom }) {
                        var next = Math.Abs(edge - value);
                        if (next <= delta) { delta = next; best = edge; }
                    }
                }
                return best;
            }
            var left = bounds.Left; var top = bounds.Top; var right = bounds.Right; var bottom = bounds.Bottom;
            if (position is NodePosition.Left or NodePosition.TopLeft or NodePosition.BottomLeft) left = Closest(left, true);
            if (position is NodePosition.Right or NodePosition.TopRight or NodePosition.BottomRight) right = Closest(right, true);
            if (position is NodePosition.Top or NodePosition.TopLeft or NodePosition.TopRight) top = Closest(top, false);
            if (position is NodePosition.Bottom or NodePosition.BottomLeft or NodePosition.BottomRight) bottom = Closest(bottom, false);
            if (right - left >= 1 && bottom - top >= 1) shape.Rectangle = RectangleF.FromLTRB(left, top, right, bottom);
        }
    }
}
