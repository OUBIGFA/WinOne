using System.Diagnostics;
using System.Runtime.InteropServices;

// Own the user's clipboard independently of a child that may time out inside
// an external accessibility provider. The desktop watchdog lives in the caller.
internal static class SelectionTestGuard
{
    [DllImport("user32.dll")] static extern bool OpenClipboard(IntPtr window);
    [DllImport("user32.dll")] static extern bool CloseClipboard();
    [DllImport("user32.dll")] static extern IntPtr GetClipboardData(uint format);
    [DllImport("kernel32.dll")] static extern UIntPtr GlobalSize(IntPtr handle);
    [DllImport("kernel32.dll")] static extern IntPtr GlobalLock(IntPtr handle);
    [DllImport("kernel32.dll")] static extern bool GlobalUnlock(IntPtr handle);
    internal static object ReadOpaqueFormat(string name) {
        var deadline=DateTime.UtcNow.AddMilliseconds(500);
        while(!OpenClipboard(IntPtr.Zero)){if(DateTime.UtcNow>deadline)throw new Exception("Clipboard snapshot is busy");Thread.Sleep(5);}
        try {
            var format=DataFormats.GetFormat(name).Id;
            var handle=GetClipboardData((uint)format);
            if(format==2&&handle!=IntPtr.Zero)return Image.FromHbitmap(handle);
            var size=GlobalSize(handle).ToUInt64();
            if(handle==IntPtr.Zero||size>64*1024*1024)throw new Exception("Cannot preserve clipboard format: "+name);
            var bytes=new byte[checked((int)size)];
            if(size>0){var data=GlobalLock(handle);if(data==IntPtr.Zero)throw new Exception("Cannot read clipboard format: "+name);try{Marshal.Copy(data,bytes,0,bytes.Length);}finally{GlobalUnlock(handle);}}
            return new MemoryStream(bytes);
        } finally { CloseClipboard(); }
    }
    internal static int Run(string[] arguments)
    {
        IDataObject? original;
        try { original = SelectionPipelineProbe.SnapshotClipboard(); }
        catch(Exception error) { File.WriteAllText(arguments[^1]+".guard.log",error.ToString()); return 2; }
        using var form = new Form { ShowInTaskbar = false, Opacity = 0 };
        int exit = 1;
        form.Shown += async (_, _) => {
            try {
                var info = new ProcessStartInfo(Environment.ProcessPath!) { UseShellExecute = false, CreateNoWindow = true };
                info.ArgumentList.Add("--guard-child");
                foreach (var argument in arguments) info.ArgumentList.Add(argument);
                using var child = Process.Start(info) ?? throw new Exception("Selection test did not start");
                try { await child.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(24)); exit = child.ExitCode; }
                catch (TimeoutException) { child.Kill(true); await child.WaitForExitAsync(); }
            }
            catch(Exception error) { File.WriteAllText(arguments[^1]+".guard.log",error.ToString()); }
            finally {
                try { if (original == null) Clipboard.Clear(); else Clipboard.SetDataObject(original, true, 30, 10); }
                finally { form.Close(); }
            }
        };
        Application.Run(form);
        return exit;
    }
}
