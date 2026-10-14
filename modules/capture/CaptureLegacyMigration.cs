using System.Security.Cryptography;
using WinOne.Capture.NativeUI;

namespace WinOne.Capture;

internal sealed partial class CaptureDraftStore
{
    internal string? MigrateToNative(NativeCaptureDraftStore target,CancellationToken cancellation=default)
    {
        lock(gate) {
            cancellation.ThrowIfCancellationRequested();
            var source=Read();if(source is null)return null;
            var metadata=Encode(source);
            // Pin the immutable PNG while publishing its native counterpart;
            // a failed encode or metadata write leaves the legacy draft active.
            using(var file=new FileStream(ImagePath(source.Id),FileMode.Open,FileAccess.Read,FileShare.Read)) {
                using var fingerprint=IncrementalHash.CreateHash(HashAlgorithmName.SHA256);
                fingerprint.AppendData(metadata);fingerprint.AppendData(SHA256.HashData(file));file.Position=0;
                using var decoded=Image.FromStream(file,useEmbeddedColorManagement:false,validateImageData:true);
                using var image=new Bitmap(decoded);
                if(image.Width!=source.Width||image.Height!=source.Height)throw new InvalidDataException("旧截图尺寸与记录不一致");
                target.ImportLegacy(source,image,NativeLegacyDocument.Convert(source),Convert.ToHexString(fingerprint.GetHashAndReset()),()=>{
                    if(!Encode(Current(source.Id,source.Revision)).SequenceEqual(metadata))throw new InvalidOperationException("旧截图已变化，请重新加载");
                },cancellation);
            }
            Complete(source.Id,source.Revision);
            return source.Id;
        }
    }
}

internal sealed partial class CaptureOperations
{
    internal async Task MigrateLegacyAsync(NativeCaptureDraftStore target,CancellationToken cancellation=default)
    {
        var migrated=await Task.Run(()=>drafts.MigrateToNative(target,cancellation),cancellation);
        if(migrated is null)return;
        if(selectionCandidates?.Id==migrated){selectionCandidates.Dispose();selectionCandidates=null;}
        ReleaseCaptureHold(migrated);
    }
}
