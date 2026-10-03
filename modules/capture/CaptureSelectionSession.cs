using ShareX.HelpersLib;

namespace WinOne.Capture;

internal sealed partial class CaptureOperations
{
    private SelectionCandidates? selectionCandidates;

    // The first visible surface is WinOne's canvas. Acquisition creates no
    // selector, toolbar or temporary window, and never waits for user input.
    private CaptureEditorRequest BeginUnifiedSelection(string mode)
    {
        var targets = mode == "window" ? WindowCaptureTargets.Snapshot() : null;
        using var frame = CaptureDesktopFrame();
        if (targets is not null && !targets.SequenceEqual(WindowCaptureTargets.Snapshot())) throw new InvalidOperationException("窗口布局已变化，请重新截图");
        var draft = PreparePendingSelection(frame, mode, store.Snapshot());
        selectionCandidates?.Dispose();
        selectionCandidates = targets is null ? null : new SelectionCandidates(draft.Id, frame.Bounds, targets);
        return new CaptureEditorRequest(draft.Id);
    }

    internal CaptureDraft PreparePendingSelection(CapturedFrame frame, string mode, CaptureSettings settings)
    {
        if (mode is not ("window" or "region" or "fullscreen")) throw new InvalidDataException("不支持的截图方式");
        using var background = frame.CreatePreview();
        return frame.WithImage(image => drafts.Begin(image, settings, CapturePlacement.From(frame.Bounds, frame.Bounds), background,
            new AnnotationCrop(0, 0, image.Width, image.Height), sourceKind: mode, selectionPending: mode != "fullscreen"));
    }

    public CaptureDraft SelectDraft(string id, long revision, AnnotationCrop selection)
    {
        var draft = drafts.Select(id, revision, selection);
        if (selectionCandidates?.Id == id) { selectionCandidates.Dispose(); selectionCandidates = null; }
        return draft;
    }

    public object ReadSelectionCandidates(string id, int? x, int? y)
    {
        var draft = GetDraft(id)!;
        if (!draft.SelectionPending) throw new InvalidOperationException("截图区域已确定");
        var placement = draft.Placement ?? throw new InvalidDataException("截图坐标无效");
        var pointer = Cursor.Position;
        var local = new Point(x ?? pointer.X - placement.X, y ?? pointer.Y - placement.Y);
        if (local.X < 0 || local.Y < 0 || local.X >= draft.Width || local.Y >= draft.Height) return new { region = (AnnotationCrop?)null, x = local.X, y = local.Y, windows = Array.Empty<AnnotationCrop>() };
        var session = selectionCandidates;
        if (draft.SourceKind == "window" && (session is null || session.Id != id)) throw new InvalidOperationException("窗口识别已结束，请拖动框选或重新截图");
        var region = session?.Hover(new Point(local.X + placement.X, local.Y + placement.Y));
        AnnotationCrop? crop = region is { Width: > 0, Height: > 0 } value ? new(value.X - placement.X, value.Y - placement.Y, value.Width, value.Height) : null;
        return new { region = crop, x = local.X, y = local.Y, windows = session?.TopLevel ?? [] };
    }

    private sealed class SelectionCandidates : IDisposable
    {
        public string Id { get; }
        public AnnotationCrop[] TopLevel { get; }
        private readonly Rectangle bounds;
        private readonly WindowRegionRefinement refinement;
        private IReadOnlyList<Rectangle> regions;
        public SelectionCandidates(string id, Rectangle bounds, IReadOnlyList<WindowCaptureTarget> targets)
        {
            Id = id; this.bounds = bounds;
            regions = targets.Select(target => target.Bounds).ToArray();
            TopLevel = regions.Select(rect => Rectangle.Intersect(rect, bounds)).Where(rect => rect.Width > 0 && rect.Height > 0)
                .Select(rect => new AnnotationCrop(rect.X - bounds.X, rect.Y - bounds.Y, rect.Width, rect.Height)).ToArray();
            var groups = targets.ToDictionary(target => target.Handle, target => (IReadOnlyList<Rectangle>)new[] { target.Bounds });
            refinement = new WindowRegionRefinement(targets, groups, next => regions = next, readNative: target => {
                if (CaptureHelpers.GetWindowRectangle(target.Handle) != target.Bounds) throw new InvalidOperationException("窗口布局已变化，请重新截图");
                return WindowCaptureTargets.Regions([target]);
            });
        }
        public Rectangle Hover(Point point) { refinement.Hover(point); return SelectionGeometry.WindowAtPoint(regions, point, bounds); }
        public void Dispose() => refinement.Dispose();
    }
}
