using System;
using System.Diagnostics;
using System.IO;
using System.Threading;
internal static class FixtureChild
{
    private static void Main(string[] args){File.WriteAllText(args[0],Process.GetCurrentProcess().Id.ToString());Thread.Sleep(60000);}
}
