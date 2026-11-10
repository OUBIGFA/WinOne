using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Windows.Automation;
using System.Windows.Automation.Text;

internal static class SelectionCompatibilityProbe
{
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool SetCursorPos(int x,int y);
    [DllImport("user32.dll")] static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
    [DllImport("user32.dll")] static extern uint GetClipboardSequenceNumber();
    [DllImport("user32.dll")] static extern bool OpenClipboard(IntPtr window);
    [DllImport("user32.dll")] static extern bool CloseClipboard();
    [DllImport("user32.dll")] static extern IntPtr GetClipboardData(uint format);

    public static int Run(string driver,string reader,string fixture,string output)
    {
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        using var form=new Form{Text="Selection compatibility controller",Width=200,Height=100};
        Exception? failure=null;
        form.Shown+=async (_,_)=>{
            Process? app=null; IDataObject? original=null;
            try {
                // Materialize every user format before the fixture touches the
                // shared Windows clipboard; restore even when an assertion fails.
                original=SelectionPipelineProbe.SnapshotClipboard();
                app=Process.Start(new ProcessStartInfo(fixture){UseShellExecute=false,CreateNoWindow=true})??throw new Exception("Fixture did not start");
                var deadline=DateTime.UtcNow.AddSeconds(8);
                while(app.MainWindowHandle==IntPtr.Zero){if(DateTime.UtcNow>deadline)throw new Exception("Fixture window not found");await Task.Delay(15);app.Refresh();}
                var window=app.MainWindowHandle;
                form.Hide();
                if(!SetForegroundWindow(window)&&GetForegroundWindow()!=window)throw new Exception("Fixture foreground refused");
                var root=AutomationElement.FromHandle(window);
                var input=await Task.Run(()=>root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,"Selection input")))??throw new Exception("Native input missing");
                await Task.Run(()=>input.SetFocus());
                var pattern=(TextPattern)input.GetCurrentPattern(TextPattern.Pattern);
                var range=pattern.DocumentRange;
                range.MoveEndpointByUnit(TextPatternRangeEndpoint.Start,TextUnit.Character,7);
                range.MoveEndpointByUnit(TextPatternRangeEndpoint.End,TextUnit.Character,-6);
                await Task.Run(()=>range.Select());
                var samples=new List<object>();
                async Task<JsonElement> Read(string name,int[]? point=null){
                    var info=new ProcessStartInfo(driver){UseShellExecute=false,CreateNoWindow=true,RedirectStandardInput=true,RedirectStandardOutput=true,RedirectStandardError=true,StandardInputEncoding=new UTF8Encoding(false),StandardOutputEncoding=new UTF8Encoding(false)};
                    info.ArgumentList.Add(reader);info.Environment["WINONE_SELECTION_DIAGNOSTICS"]="1";
                    var clock=Stopwatch.StartNew();using var child=Process.Start(info)??throw new Exception("Driver not started");
                    await child.StandardInput.WriteAsync(JsonSerializer.Serialize(new{window=(ulong)window,pid=app.Id,point}));child.StandardInput.Close();
                    var stdout=child.StandardOutput.ReadToEndAsync();var stderr=child.StandardError.ReadToEndAsync();
                    try{await child.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(3));}catch{child.Kill(true);throw;}
                    File.WriteAllText(Path.Combine(output,name+".log"),await stderr,new UTF8Encoding(false));
                    if(child.ExitCode!=0)throw new Exception(name+": "+await stderr);
                    var result=JsonDocument.Parse(await stdout).RootElement.Clone();samples.Add(new{name,milliseconds=clock.Elapsed.TotalMilliseconds,result});return result;
                }
                // UIA selection is applied on MyGo's next frame; observe it.
                deadline=DateTime.UtcNow.AddSeconds(2);
                while(string.Join("",pattern.GetSelection().Select(x=>x.GetText(-1)))!="WinOneSelection"){if(DateTime.UtcNow>deadline)throw new Exception("Input selection not applied");await Task.Delay(10);}
                var inputValue=await Read("mygo-input");
                if(inputValue.GetProperty("status").GetString()!="selected"||inputValue.GetProperty("text").GetString()!="WinOneSelection")throw new Exception("MyGo input rejected: "+inputValue);
                var paragraph=await Task.Run(()=>root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,"WinOne paragraph selection")))??throw new Exception("Native paragraph missing");
                var bounds=paragraph.Current.BoundingRectangle;
                int x=(int)bounds.Left+15,y=(int)(bounds.Top+bounds.Height/2);
                SetCursorPos(x,y);mouse_event(2,0,0,0,UIntPtr.Zero);mouse_event(4,0,0,0,UIntPtr.Zero);
                await Task.Delay(50);mouse_event(2,0,0,0,UIntPtr.Zero);mouse_event(4,0,0,0,UIntPtr.Zero);
                await Task.Delay(40);
                var saved=new DataObject();saved.SetData(DataFormats.UnicodeText,false,"clipboard sentinel");saved.SetData(DataFormats.Html,false,"<b>clipboard sentinel</b>");Clipboard.SetDataObject(saved,true);
                var paragraphValue=await Read("mygo-paragraph",new[]{x,y});
                if(paragraphValue.GetProperty("status").GetString()!="selected"||paragraphValue.GetProperty("text").GetString()!="WinOne")throw new Exception("MyGo paragraph rejected: "+paragraphValue);
                var restoredText=Clipboard.GetText();var hasHtml=Clipboard.TryGetData<string>(DataFormats.Html,out var html);
                if(restoredText!="clipboard sentinel"||!hasHtml||html!="<b>clipboard sentinel</b>")throw new Exception("Clipboard formats were not restored: text="+restoredText+", html="+html+", hasHtml="+hasHtml+", formats="+string.Join(",",Clipboard.GetDataObject()?.GetFormats()??[]));
                Clipboard.Clear();
                if((await Read("mygo-empty-clipboard",new[]{x,y})).GetProperty("text").GetString()!="WinOne")throw new Exception("Empty clipboard selection failed");
                if(Clipboard.GetDataObject() is { } empty&&empty.GetFormats().Length!=0)throw new Exception("Empty clipboard was not restored");
                // Keep the source alive while OLE can still return its managed
                // data object. Disposing it here tests a dead fixture, not restore.
                using var bitmap=new Bitmap(3,3);using(var pixels=Graphics.FromImage(bitmap))pixels.Clear(Color.Blue);bitmap.SetPixel(1,1,Color.Red);Clipboard.SetImage(bitmap);
                using var imageBefore=Clipboard.GetImage() as Bitmap??throw new Exception("Fixture could not put an image on clipboard");
                // Verify the native payload, not OLE's same-process cached
                // object. External clipboard viewers can replace it meanwhile.
                var imageSequence=GetClipboardSequenceNumber();
                if(!OpenClipboard(IntPtr.Zero))throw new Exception("Image fixture clipboard is busy");
                try{if(GetClipboardData(2)==IntPtr.Zero)throw new Exception("Image fixture has no native bitmap");}finally{CloseClipboard();}
                if(imageSequence!=GetClipboardSequenceNumber())throw new Exception("Image fixture clipboard changed before selection");
                if((await Read("mygo-image-clipboard",new[]{x,y})).GetProperty("text").GetString()!="WinOne")throw new Exception("Image clipboard selection failed");
                using(var restored=Clipboard.GetImage() as Bitmap){if(restored==null||restored.Size!=imageBefore.Size||restored.GetPixel(1,1).ToArgb()!=imageBefore.GetPixel(1,1).ToArgb())throw new Exception("Clipboard image was not restored: before="+imageBefore.Size+"/"+imageBefore.GetPixel(1,1).ToArgb()+", after="+restored?.Size+"/"+restored?.GetPixel(1,1).ToArgb()+", sequence="+GetClipboardSequenceNumber()+", formats="+string.Join(",",Clipboard.GetDataObject()?.GetFormats()??[]));}
                var files=new System.Collections.Specialized.StringCollection();files.Add(fixture);Clipboard.SetFileDropList(files);
                if((await Read("mygo-file-clipboard",new[]{x,y})).GetProperty("text").GetString()!="WinOne"||!Clipboard.GetFileDropList().Cast<string>().SequenceEqual(files.Cast<string>()))throw new Exception("Clipboard file list was not restored");
                SetCursorPos((int)bounds.Right-2,y);mouse_event(2,0,0,0,UIntPtr.Zero);mouse_event(4,0,0,0,UIntPtr.Zero);await Task.Delay(30);
                Clipboard.SetText("no selection sentinel");
                var emptyRead=await Read("mygo-caret",new[]{(int)bounds.Right-2,y});
                if(emptyRead.GetProperty("status").GetString()=="selected"||Clipboard.GetText()!="no selection sentinel")throw new Exception("Caret copied unrelated data or changed clipboard");
                File.WriteAllText(Path.Combine(output,"result.json"),JsonSerializer.Serialize(new{passed=true,nativeInput=true,nativeParagraph=true,clipboardFormatsRestored=true,samples},new JsonSerializerOptions{WriteIndented=true}),new UTF8Encoding(false));
            }catch(Exception error){failure=error;File.WriteAllText(Path.Combine(output,"failure.txt"),error.ToString());}
            finally{if(app!=null){if(!app.HasExited)app.Kill(true);app.Dispose();}if(original!=null)Clipboard.SetDataObject(original,true,30,10);else Clipboard.Clear();form.Close();}
        };
        Application.Run(form);return failure==null?0:1;
    }
}
