using System.Threading.Tasks;
using System.Windows.Forms;
using Stroke;
namespace StrokeLegacyFixture
{
    // Compile against the unmodified official Base.dll and Stroke.exe. The
    // resulting binary must resolve through WinOne's compatibility facades.
    public static class Plugin
    {
        public static uint Press(){if(typeof(Base).Assembly.GetName().Name!="Stroke.Base")throw new System.InvalidOperationException("The original input library must not execute in a fixture");return Task.Run(()=>{Base.PressKey(Keys.F13);return Base.GetWindowProcessId();}).Result;}
    }
}
