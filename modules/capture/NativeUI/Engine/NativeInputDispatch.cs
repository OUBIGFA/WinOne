using System.Drawing;
using System.Windows.Forms;

namespace ShareX.ScreenCaptureLib
{
    public partial struct MouseState
    {
        internal static MouseState FromWinOneEvent(Control control, Point point, MouseButtons buttons) =>
            new MouseState { ClientPosition = point, Position = control.PointToScreen(point), Buttons = buttons };
    }

    public partial class InputManager
    {
        internal bool WinOneEventActive { get; private set; }
        internal void BeginWinOneEvent(Control control, Point point, MouseButtons buttons)
        {
            WinOneEventActive = true;
            oldMouseState = mouseState;
            mouseState = MouseState.FromWinOneEvent(control, point, buttons);
        }
        internal void ConsumeWinOneEvent() => oldMouseState = mouseState;
        internal void EndWinOneEvent() { ConsumeWinOneEvent(); WinOneEventActive = false; }
    }
}
