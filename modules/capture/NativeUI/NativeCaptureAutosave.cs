using System.Threading.Channels;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// A session transfers its acquisition bitmap here after the canvas has copied
// it. Only this worker touches that bitmap; subsequent saves contain document
// data, never additional desktop-sized images or an unbounded edit queue.
internal sealed class NativeCaptureAutosave
{
    private readonly NativeCaptureDraftStore store;
    private sealed record PendingSnapshot(long Sequence, NativeCaptureSnapshot Snapshot);
    private readonly Channel<PendingSnapshot> updates = Channel.CreateBounded<PendingSnapshot>(
        new BoundedChannelOptions(1) { SingleReader = true, SingleWriter = true, FullMode = BoundedChannelFullMode.DropOldest });
    private readonly TaskCompletionSource<string> reserved = new(TaskCreationOptions.RunContinuationsAsynchronously);
    private readonly object gate = new();
    private readonly object updateGate = new();
    private readonly Task worker;
    private Task<NativeDraftCleanup>? finishing;
    private bool closed, retiring, finishingRetire;
    private long submitted, persisted;
    private NativeCaptureDraft? latest;
    private TaskCompletionSource? changed;
    public Task Completion => worker;

    public NativeCaptureAutosave(NativeCaptureDraftStore store, Bitmap original, CaptureSettings settings,
        WinOneCaptureDocument initial, string mode, CapturePlacement? placement = null, string? notice = null)
    {
        this.store = store;
        worker = Task.Run(() => RunAsync(() => {
            using (original) {
                var ticket = store.Reserve(original.Size, settings, initial, mode, placement, notice);
                reserved.TrySetResult(ticket.Id);
                return store.CompleteReservation(ticket.Id, original);
            }
        }));
    }

    public NativeCaptureAutosave(NativeCaptureDraftStore store, NativeCaptureDraft recovered)
    {
        this.store = store;
        reserved.SetResult(recovered.Id);
        worker = Task.Run(() => RunAsync(() => recovered));
    }

    private async Task RunAsync(Func<NativeCaptureDraft> initialize)
    {
        try {
            var draft = initialize();
            Saved(draft, 0);
            await foreach (var pending in updates.Reader.ReadAllAsync().ConfigureAwait(false)) {
                lock (updateGate) {
                    lock (gate) { if (retiring) return; }
                    draft = store.Update(draft.Id, draft.Revision, pending.Snapshot.Document, pending.Snapshot.Text);
                    Saved(draft, pending.Sequence);
                }
            }
        } catch (Exception error) {
            reserved.TrySetException(error);
            throw;
        }
    }

    private void Saved(NativeCaptureDraft draft, long sequence)
    {
        TaskCompletionSource? signal;
        lock (gate) {
            latest = draft; persisted = sequence;
            signal = changed; changed = null;
        }
        signal?.TrySetResult();
    }

    // A retake needs the last accepted revision without closing this writer:
    // acquisition can fail and editing must then continue on the old draft.
    public async Task<NativeCaptureDraft> FlushAsync()
    {
        long target;
        lock (gate) { if (closed || retiring) throw new InvalidOperationException("截图自动保存已结束"); target = submitted; }
        while (true) {
            Task wait;
            lock (gate) {
                if (latest is not null && persisted >= target) return latest;
                wait = (changed ??= new(TaskCreationOptions.RunContinuationsAsynchronously)).Task;
            }
            if (worker.IsCompleted) { await worker.ConfigureAwait(false); throw new InvalidOperationException("截图自动保存未完成"); }
            await Task.WhenAny(wait, worker).ConfigureAwait(false);
        }
    }

    // Documents must be detached snapshots captured on the owning UI thread.
    // Publishing does no disk work and fails visibly after a writer failure.
    public void Publish(WinOneCaptureDocument document) => Publish(new NativeCaptureSnapshot(document));
    public void Publish(NativeCaptureSnapshot snapshot)
    {
        lock (gate) {
            if (closed) throw new InvalidOperationException("截图自动保存已结束");
            if (worker.IsFaulted) worker.GetAwaiter().GetResult();
            if (!updates.Writer.TryWrite(new PendingSnapshot(++submitted, snapshot))) throw new InvalidOperationException("截图自动保存不可用");
        }
    }

    // Normal completion/cancel retires the draft and returns once retirement
    // is durable; the discarded encode drains in the background (Draining).
    // Parent shutdown preserves the draft by persisting the newest edit and
    // returns only after that write. Never wait on it from paint or input.
    public Task<NativeDraftCleanup> FinishAsync(bool retire)
    {
        lock (gate) {
            if (finishing is not null && !finishing.IsFaulted) {
                if (finishingRetire != retire) throw new InvalidOperationException("截图结束方式已确定");
                return finishing;
            }
            if (!retire) { closed = true; updates.Writer.TryComplete(); }
            finishingRetire = retire;
            return finishing = FinishCoreAsync(retire);
        }
    }

    private async Task<NativeDraftCleanup> FinishCoreAsync(bool retire)
    {
        if (!retire) { await worker.ConfigureAwait(false); return new(false, []); }
        string id;
        try { id = await reserved.Task.ConfigureAwait(false); }
        catch {
            // Creation failed before this session acquired a draft identity.
            // Explicit discard can still close; it must not delete other data.
            try { await worker.ConfigureAwait(false); } catch (Exception error) { Console.Error.WriteLine("Native autosave discarded: " + error.Message); }
            lock (gate) { closed = true; retiring = true; updates.Writer.TryComplete(); }
            return new(false, []);
        }
        // This operation must run independently of the encoder. Cancellation
        // records invalidation first; the generation lease defers only deletion.
        var recorded = await Task.Run(() => {
            lock (updateGate) {
                var cleanup = store.CancelReservation(id);
                // A failed retirement leaves the writer accepting edits, so the
                // same visible session can retry after permissions are fixed.
                lock (gate) { closed = true; retiring = true; updates.Writer.TryComplete(); }
                return cleanup;
            }
        }).ConfigureAwait(false);
        // The durable retirement record already hides this generation from
        // recovery and lets the next capture reserve a new one. A full-screen
        // PNG encode can take most of a second on detailed desktops, so the
        // window and the capture job no longer wait for it; only process exit
        // does, through the tracked drain below.
        Track(DrainAsync());
        return recorded;
    }

    private async Task DrainAsync()
    {
        try { await worker.ConfigureAwait(false); }
        catch (OperationCanceledException) { }
        catch (Exception error) { Console.Error.WriteLine("Native autosave discarded: " + error.Message); }
        var cleanup = await Task.Run(store.RetryCleanup).ConfigureAwait(false);
        foreach (var error in cleanup.Errors) Console.Error.WriteLine("Native draft cleanup: " + error);
    }

    private static readonly object drainGate = new();
    private static readonly HashSet<Task> drains = new();

    // Track the removal continuation, not the drain, so WhenDrained and
    // Draining agree: once WhenDrained completes, Draining is already false.
    private static void Track(Task drain)
    {
        lock (drainGate) {
            Task tracked = null!;
            tracked = drain.ContinueWith(_ => { lock (drainGate) drains.Remove(tracked); }, TaskScheduler.Default);
            drains.Add(tracked);
        }
    }

    // Discarded encodes still running in the background. Shutdown waits for
    // them so a half-written file is not left for startup cleanup; a new
    // capture never waits.
    internal static bool Draining { get { lock (drainGate) return drains.Count != 0; } }

    internal static Task WhenDrained()
    {
        Task[] pending;
        lock (drainGate) pending = drains.ToArray();
        // DrainAsync logs and swallows its own failures.
        return Task.WhenAll(pending);
    }
}
