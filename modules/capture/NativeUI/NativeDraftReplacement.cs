using System.Security.Cryptography;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal sealed partial class NativeCaptureDraftStore
{
    // Encode outside the profile lock. The old draft remains authoritative
    // until the new image is complete and the expected revision is rechecked.
    public NativeCaptureDraft ReplaceScroll(string expectedId, long revision, Bitmap image, CaptureSettings settings, WinOneCaptureDocument document, string? notice = null, CancellationToken cancellation = default)
    {
        cancellation.ThrowIfCancellationRequested();
        SettingsStore.Validate(settings); ValidateSize(image.Width,image.Height); ValidateNotice("scroll",notice);
        using var encoded=JsonDocument.Parse(NativeCaptureDocumentCodec.Encode(document,image.Size));
        using(var lease=Lock()) {
            // Any editable capture can enter scrolling mode; the old draft stays valid until commit.
            _ = Current(expectedId,revision);
            if(ReadPending() is not null) throw new InvalidOperationException("截图正在保存，请稍后重试");
        }
        var id=Guid.NewGuid().ToString("N");
        var committed=false;
        using var encoding=EncodingLease(id);
        try {
            var path=ImagePath(id); writeImage(image,path);
            string hash; using(var file=File.OpenRead(path)) hash=Convert.ToHexString(SHA256.HashData(file));
            cancellation.ThrowIfCancellationRequested();
            using var lease=Lock();
            var current=Current(expectedId,revision);
            var next=new Envelope(1,id,0,"scroll",settings with { ToolbarTools=settings.ToolbarTools?.ToArray() },null,image.Width,image.Height,hash,encoded.RootElement.Clone(),Notice:notice);
            var result=ToDraft(next);
            Write(next); committed=true;
            // Cleanup failure must not report an already committed replacement
            // as failed. Retired records allow later cleanup without resurrection.
            try { RecordRetirement(current.Id,current.Revision); CleanupRetiredCore(); }
            catch(Exception error) when(error is IOException or UnauthorizedAccessException or InvalidDataException) { Console.Error.WriteLine("Replaced draft cleanup: "+error.Message); }
            return result;
        } finally {
            if(!committed) {
                try { DeleteImageFiles(id); }
                catch(Exception error) when(error is IOException or UnauthorizedAccessException) { Console.Error.WriteLine("Uncommitted replacement cleanup: "+error.Message); }
            }
        }
    }
}
