using System;

namespace WinOne.StrokeRuntime
{
    // WM_INPUT continues after Windows silently removes a timed-out low-level
    // hook. Repair only when fresh physical input lacks a matching callback;
    // ordinary idle time is not a failure and must never restart the hook.
    public sealed class HookRecovery
    {
        private long repairedInput;
        public bool NeedsRestart(long now, long inputAt, long callbackAt)
        {
            if(inputAt<=repairedInput||inputAt<=0||now-inputAt>1000||now-callbackAt<1500)return false;
            repairedInput=inputAt;
            return true;
        }
    }
}
