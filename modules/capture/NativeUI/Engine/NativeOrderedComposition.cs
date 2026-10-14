using System;
using System.Collections.Generic;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Text;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;

namespace ShareX.ScreenCaptureLib
{
    internal partial class ShapeManager
    {
        internal bool WinOneOrderedEffects;
        internal event Action<Exception> WinOneCompositionFailed;
        private Bitmap winOneComposition;
        private long compositionRevision;
        private long renderedRevision = -1;
        private long failedRevision = -1;
        private Exception compositionError;
        private WinOneCompositionJob compositionJob;
        private bool compositionDisposed;

        // Retain the most recent preview. One detached render runs at a time;
        // subsequent edits collapse into the next snapshot instead of a queue.
        internal void InvalidateWinOneComposition() => compositionRevision++;
        internal void ResetWinOneComposition()
        {
            InvalidateWinOneComposition();
            compositionJob?.Dispose(); compositionJob = null;
            winOneComposition?.Dispose(); winOneComposition = null;
            renderedRevision = failedRevision = -1; compositionError = null;
        }
        internal void DisposeWinOneComposition() { compositionDisposed = true; ResetWinOneComposition(); }

        internal Bitmap WinOneComposition()
        {
            if (IsCreating || IsMoving || IsResizing) InvalidateWinOneComposition();
            UpdateWinOneComposition();
            return winOneComposition ?? Form.Canvas;
        }

        private void UpdateWinOneComposition()
        {
            if (compositionDisposed || !WinOneOrderedEffects) return;
            if (compositionJob is { Completion.IsCompleted: true } completed) {
                compositionJob = null;
                using (completed) {
                    var result = completed.TakeResult();
                    if (result is not null) {
                        winOneComposition?.Dispose(); winOneComposition = result;
                        renderedRevision = completed.Revision;
                    } else if (completed.Error is { } error && completed.Revision == compositionRevision) {
                        compositionError = error; failedRevision = compositionRevision;
                        WinOneCompositionFailed?.Invoke(error);
                    }
                }
            }
            if (compositionJob is not null || renderedRevision == compositionRevision || failedRevision == compositionRevision) return;
            try {
                var document = ExportWinOneDocument("select", validate: false, rendering: true);
                compositionJob = new WinOneCompositionJob(compositionRevision, (Bitmap)Form.Canvas.Clone(), document);
                // The job owns every bitmap until UI adoption or disposal. This
                // callback carries no image, so a lost message cannot leak it.
                _ = compositionJob.Completion.ContinueWith(_ => {
                    try { Form.BeginInvoke((Action)(() => { if (!Form.IsDisposed) Form.Invalidate(); })); }
                    catch (InvalidOperationException) { } // The window may have closed.
                }, CancellationToken.None, TaskContinuationOptions.None, TaskScheduler.Default);
            } catch (Exception error) {
                compositionError = error; failedRevision = compositionRevision;
                WinOneCompositionFailed?.Invoke(error);
            }
        }

        internal async Task PrepareWinOneCompositionAsync(CancellationToken cancellation)
        {
            // Explicit output can retry; painting never loops on a failed revision.
            failedRevision = -1;
            while (WinOneOrderedEffects) {
                cancellation.ThrowIfCancellationRequested();
                if (compositionDisposed) throw new ObjectDisposedException(nameof(ShapeManager));
                UpdateWinOneComposition();
                if (renderedRevision == compositionRevision) return;
                if (failedRevision == compositionRevision) throw new InvalidOperationException("无法生成截图，请重试", compositionError);
                var pending = compositionJob ?? throw new InvalidOperationException("截图合成任务不存在");
                await pending.Completion.WaitAsync(cancellation);
            }
        }

        internal Bitmap RenderWinOneOrdered(Bitmap source)
        {
            if (winOneComposition is not null && renderedRevision == compositionRevision) return (Bitmap)winOneComposition.Clone();
            // Interactive output awaits preparation first. Synchronous consumers
            // still use detached shapes and never move the live edit state.
            var document = ExportWinOneDocument("select", validate: false, rendering: true);
            return WinOneOrderedRenderer.Render((Bitmap)source.Clone(), document, CancellationToken.None);
        }
    }

    internal sealed class WinOneCompositionJob : IDisposable
    {
        private readonly object gate = new();
        private readonly CancellationTokenSource cancellation = new();
        private Bitmap result;
        private bool disposed;
        internal long Revision { get; }
        internal Task Completion { get; }
        internal Exception Error { get; private set; }

        internal WinOneCompositionJob(long revision, Bitmap source, WinOneCaptureDocument document)
        {
            Revision = revision;
            Completion = Task.Run(() => {
                Bitmap rendered = null;
                try { rendered = WinOneOrderedRenderer.Render(source, document, cancellation.Token); }
                catch (OperationCanceledException) when (cancellation.IsCancellationRequested) { }
                catch (Exception error) { Error = error; }
                lock (gate) {
                    if (disposed) rendered?.Dispose(); else result = rendered;
                }
            });
        }
        internal Bitmap TakeResult() { lock (gate) { var value = result; result = null; return value; } }
        public void Dispose()
        {
            lock (gate) {
                if (disposed) return;
                disposed = true; cancellation.Cancel(); result?.Dispose(); result = null;
            }
            _ = Completion.ContinueWith(_ => cancellation.Dispose(), TaskScheduler.Default);
        }
    }

    internal static class WinOneOrderedRenderer
    {
        // Takes ownership of source. No HWND, live shape or shared bitmap may
        // be accessed here; inputs are released even if the UI disappears.
        internal static Bitmap Render(Bitmap source, WinOneCaptureDocument document, CancellationToken cancellation)
        {
            var output = source;
            var shapes = new List<BaseShape>();
            try {
                cancellation.ThrowIfCancellationRequested();
                foreach (var layer in document.Layers) shapes.Add(ShapeManager.CreateWinOneShape(layer, null));
                for (var index = 0; index < shapes.Count;) {
                    cancellation.ThrowIfCancellationRequested();
                    if (shapes[index] is IWinOneLegacyEffect legacy) { legacy.ApplyLegacy(output); index++; continue; }
                    if (shapes[index] is BaseEffectShape effect) { using var graphics = Graphics.FromImage(output); effect.OnDrawFinal(graphics, output); index++; continue; }
                    var begin = index;
                    while (index < shapes.Count && shapes[index] is not BaseEffectShape) index++;
                    var drawings = shapes.Skip(begin).Take(index - begin).OfType<BaseDrawingShape>().ToArray();
                    void Draw(Graphics graphics) {
                        graphics.PageUnit = GraphicsUnit.Pixel; graphics.SmoothingMode = SmoothingMode.AntiAlias;
                        graphics.TextRenderingHint = TextRenderingHint.AntiAliasGridFit;
                        foreach (var shape in drawings) { cancellation.ThrowIfCancellationRequested(); shape.OnDraw(graphics); graphics.Flush(); }
                    }
                    if (drawings.OfType<WinOneInlineTextShape>().Any(shape => shape.NativeLines != null)) {
                        var previous = output; output = NativeDrawingSurface.Draw(previous, Draw); previous.Dispose();
                    } else { using var graphics = Graphics.FromImage(output); Draw(graphics); }
                }
                cancellation.ThrowIfCancellationRequested();
                return output;
            } catch { output.Dispose(); throw; }
            finally { foreach (var shape in shapes) shape.Dispose(); }
        }
    }
}
