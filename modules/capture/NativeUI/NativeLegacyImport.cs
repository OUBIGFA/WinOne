using System.Security.Cryptography;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal sealed partial class NativeCaptureDraftStore
{
    private static void ValidateLegacySource(string? fingerprint)
    {
        if (fingerprint is not null && (fingerprint.Length != 64 || !fingerprint.All(Uri.IsHexDigit)))
            throw new InvalidDataException("旧截图校验标识无效");
    }
    // Keep the original identity so an in-flight --capture-draft launch still
    // names the same capture. A committed fingerprint makes interruption between
    // native publication and legacy retirement retryable without reconversion.
    internal NativeCaptureDraft ImportLegacy(CaptureDraft source,Bitmap image,WinOneCaptureDocument document,string fingerprint,Action validateSource,CancellationToken cancellation=default)
    {
        cancellation.ThrowIfCancellationRequested();
        ValidateLegacySource(fingerprint);
        var mode=source.SourceKind??"fullscreen";
        var placement=source.Placement??(mode=="scroll"?null:CapturePlacement.From(new Rectangle(Point.Empty,image.Size),new Rectangle(Point.Empty,image.Size)));
        SettingsStore.Validate(source.Settings);ValidateSize(image.Width,image.Height);ValidateNotice(mode,source.Notice);
        using var encoded=JsonDocument.Parse(NativeCaptureDocumentCodec.Encode(document,image.Size));
        using var lease=Lock();
        var current=ReadEnvelope();
        if(current is not null) {
            if(current.Id==source.Id&&current.LegacySource==fingerprint)return ToDraft(current);
            throw new InvalidOperationException("已有另一张原生截图草稿，请先处理");
        }
        if(ReadPending() is not null)throw new InvalidOperationException("原生截图正在保存，请稍后重试");
        if(ReadRetired().Any(entry=>entry.Id==source.Id))throw new InvalidOperationException("此原生草稿已结束，请检查旧格式记录");
        var temporaryId=Guid.NewGuid().ToString("N");
        var temporary=ImagePath(temporaryId);var destination=ImagePath(source.Id);
        var moved=false;var committed=false;
        try {
            Directory.CreateDirectory(Path.GetDirectoryName(temporary)!);
            writeImage(image,temporary);
            string hash;using(var file=File.OpenRead(temporary))hash=Convert.ToHexString(SHA256.HashData(file));
            validateSource();
            cancellation.ThrowIfCancellationRequested();
            var record=new Envelope(1,source.Id,0,mode,source.Settings,placement,image.Width,image.Height,hash,encoded.RootElement.Clone(),Notice:source.Notice,LegacySource:fingerprint);
            var result=ToDraft(record);
            File.Move(temporary,destination,overwrite:true);moved=true;
            Write(record);committed=true;return result;
        } finally {
            try {if(File.Exists(temporary))File.Delete(temporary);if(moved&&!committed)File.Delete(destination);}
            catch(Exception error) when(error is IOException or UnauthorizedAccessException){Console.Error.WriteLine("Legacy import cleanup: "+error.Message);}
        }
    }
}
