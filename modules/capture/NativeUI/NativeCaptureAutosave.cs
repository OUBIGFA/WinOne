using System.Threading.Channels;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// A session transfers its acquisition bitmap here after the canvas has copied
// it. Only this worker touches that bitmap; subsequent saves contain document
// data, never additional desktop-sized images or an unbounded edit queue.
internal sealed class NativeCaptureAutosave
{
    private readonly NativeCaptureDraftStore store;
    private readonly Channel<NativeCaptureSnapshot> updates = Channel.CreateBounded<NativeCaptureSnapshot>(
        new BoundedChannelOptions(1) { SingleReader = true, SingleWriter = true, FullMode = BoundedChannelFullMode.DropOldest });
    private readonly TaskCompletionSource<string> reserved = new(TaskCreationOptions.RunContinuationsAsynchronously);
    private readonly object gate = new();
    private readonly object updateGate = new();
    private readonly Task worker;
    private Task<NativeDraftCleanup>? finishing;
    private bool closed, retiring, finishingRetire;
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
            await foreach (var snapshot in updates.Reader.ReadAllAsync().ConfigureAwait(false)) {
                lock (updateGate) {
                    lock (gate) { if (retiring) return; }
                    draft = store.Update(draft.Id, draft.Revision, snapshot.Document, snapshot.Text);
                }
            }
        } catch (Exception error) {
            reserved.TrySetException(error);
            throw;
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
            if (!updates.Writer.TryWrite(snapshot)) throw new InvalidOperationException("截图自动保存不可用");
        }
    }

    // Normal completion/cancel retires the draft. Parent shutdown preserves it
    // by draining the newest edit instead. Callers retain Worker lifetime until
    // this task ends, but do not wait on it from paint or input handlers.
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
        await Task.Run(() => {
            lock (updateGate) {
                var cleanup = store.CancelReservation(id);
                // A failed retirement leaves the writer accepting edits, so the
                // same visible session can retry after permissions are fixed.
                lock (gate) { closed = true; retiring = true; updates.Writer.TryComplete(); }
                return cleanup;
            }
        }).ConfigureAwait(false);
        try { await worker.ConfigureAwait(false); }
        catch (OperationCanceledException) { }
        catch (Exception error) { Console.Error.WriteLine("Native autosave discarded: " + error.Message); }
        return await Task.Run(store.RetryCleanup).ConfigureAwait(false);
    }
}
