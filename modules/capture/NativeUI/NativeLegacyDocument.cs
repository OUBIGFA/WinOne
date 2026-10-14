using System.Security.Cryptography;
using System.Text;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal static class NativeLegacyDocument
{
    internal static WinOneCaptureDocument Convert(CaptureDraft draft)
    {
        var image=new Size(draft.Width,draft.Height);
        var selection=AnnotationRenderer.ValidateDocument(image,draft.Document);
        var layers=draft.Document.Marks.Select((mark,index)=>{
            var color=ColorTranslator.FromHtml(mark.Color).ToArgb();
            var points=mark.Points.Select(point=>new PointF(point.X,point.Y)).ToArray();
            var bounds=points.Length==0?new RectangleF(mark.X,mark.Y,mark.Width,mark.Height)
                :RectangleF.FromLTRB(points.Min(point=>point.X),points.Min(point=>point.Y),points.Max(point=>point.X),points.Max(point=>point.Y));
            var textId=new Guid(SHA256.HashData(Encoding.UTF8.GetBytes(draft.Id+":"+index))[..16]);
            return new WinOneCaptureLayer {
                Kind=mark.Kind,Bounds=bounds,StrokeColor=color,FillColor=mark.Kind=="cover"?color:Color.Transparent.ToArgb(),
                StrokeWidth=(int)Math.Round(mark.StrokeWidth),LegacyStrokeWidth=mark.Kind is "rectangle" or "ellipse" or "pen" or "arrow"?mark.StrokeWidth:null,
                LegacyEffect=mark.Kind is "mosaic" or "blur",EffectSize=mark.EffectSize,Points=points,
                ArrowHead=(int)ArrowHeadDirection.End,Text=mark.Text,TextId=textId,FontPixels=mark.FontSize,TextColor=color,
            };
        }).ToArray();
        var result=new WinOneCaptureDocument(2,draft.Width,draft.Height,draft.SelectionPending?null:selection,draft.SelectionPending?"region":"select",layers){OrderedEffects=true};
        result.Validate(image);return result;
    }
}
