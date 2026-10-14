using System;
using System.ComponentModel;
using System.IO;
using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;
using System.Threading;
using System.Web.Script.Serialization;
using Microsoft.Win32.SafeHandles;

namespace WinOne.StrokeRuntime
{
    internal sealed class Request {public int version{get;set;}public string id{get;set;}public string method{get;set;}public object @params{get;set;}}
    internal static class Json
    {
        internal static string Encode(object value){return new JavaScriptSerializer{MaxJsonLength=1024*1024,RecursionLimit=48}.Serialize(value);}
        internal static T Decode<T>(string value){return new JavaScriptSerializer{MaxJsonLength=1024*1024,RecursionLimit=48}.Deserialize<T>(value);}
        internal static T Params<T>(object value){return Decode<T>(Encode(value));}
    }
    // Adapted from WinOne.Capture LocalPipe: current-user ACL, first-instance
    // protection and network-client rejection. One bounded frame per request.
    internal sealed class LocalServer:IDisposable
    {
        [StructLayout(LayoutKind.Sequential)]private struct SecurityAttributes{public int Length;public IntPtr Descriptor;public int Inherit;}
        [DllImport("advapi32.dll",CharSet=CharSet.Unicode,SetLastError=true)]private static extern bool ConvertStringSecurityDescriptorToSecurityDescriptor(string text,uint revision,out IntPtr descriptor,out uint size);
        [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]private static extern SafePipeHandle CreateNamedPipe(string name,uint mode,uint pipeMode,uint instances,uint output,uint input,uint timeout,ref SecurityAttributes security);
        [DllImport("kernel32.dll")]private static extern IntPtr LocalFree(IntPtr memory);
        [DllImport("kernel32.dll",SetLastError=true)]private static extern bool GetNamedPipeClientProcessId(SafePipeHandle handle,out uint pid);
        private readonly string name;private readonly int parent;private readonly Func<Request,object> handler;private Thread thread;private NamedPipeServerStream pending;private volatile bool closed;private readonly object gate=new object();
        internal System.Action<string> AfterReply;
        internal System.Action Faulted;
        internal LocalServer(string name,int parent,Func<Request,object> handler){this.name=name.StartsWith(@"\\.\pipe\")?name:@"\\.\pipe\"+name;this.parent=parent;this.handler=handler;}
        private NamedPipeServerStream Create(bool first)
        {
            string sid;using(var identity=WindowsIdentity.GetCurrent())sid=identity.User.Value;IntPtr descriptor;uint size;
            if(!ConvertStringSecurityDescriptorToSecurityDescriptor("D:P(A;;GA;;;"+sid+")",1,out descriptor,out size))throw new Win32Exception(Marshal.GetLastWin32Error());
            try{var security=new SecurityAttributes{Length=Marshal.SizeOf(typeof(SecurityAttributes)),Descriptor=descriptor};var handle=CreateNamedPipe(name,3|0x40000000|(first?0x00080000u:0),8,2,65536,65536,0,ref security);if(handle.IsInvalid)throw new Win32Exception(Marshal.GetLastWin32Error());return new NamedPipeServerStream(PipeDirection.InOut,true,false,handle);}finally{LocalFree(descriptor);}
        }
        internal void Start(){pending=Create(true);thread=new Thread(Serve){IsBackground=true,Name="Stroke local protocol"};thread.Start();}
        private void Serve()
        {
            while(!closed){NamedPipeServerStream pipe;lock(gate)pipe=pending;
                try{pipe.WaitForConnection();lock(gate){if(closed)return;pending=Create(false);}uint pid;if(!GetNamedPipeClientProcessId(pipe.SafePipeHandle,out pid)||pid!=parent)throw new UnauthorizedAccessException("Unexpected pipe client");
                    using(var deadline=new Timer(_=>{try{pipe.Dispose();}catch{}},null,5000,Timeout.Infinite)){
                        string text=Encoding.UTF8.GetString(ReadFrame(pipe));Request request=Json.Decode<Request>(text);object response;
                        try{if(request==null||request.version!=1||String.IsNullOrEmpty(request.id))throw new InvalidDataException("Invalid protocol request");response=new{id=request.id,result=handler(request)};}
                        catch(Exception error){response=new{id=request==null?"":request.id,error=new{code="stroke",message=error.Message}};}
                        WriteFrame(pipe,Encoding.UTF8.GetBytes(Json.Encode(response)));pipe.Flush();if(AfterReply!=null&&request!=null)AfterReply(request.method);
                    }
                }catch(Exception error){if(!closed)Console.Error.WriteLine("stroke.protocol: "+error.Message);}
                finally{try{pipe.Dispose();}catch{}}
                lock(gate){if(closed)return;if(Object.ReferenceEquals(pending,pipe)){closed=true;if(Faulted!=null)Faulted();return;}}
            }
        }
        internal static byte[] ReadFrame(Stream stream){var header=Read(stream,4);int length=BitConverter.ToInt32(header,0);if(length<1||length>1024*1024)throw new InvalidDataException("Invalid frame size");return Read(stream,length);}
        private static byte[] Read(Stream stream,int length){var bytes=new byte[length];int at=0;while(at<length){int count=stream.Read(bytes,at,length-at);if(count==0)throw new EndOfStreamException();at+=count;}return bytes;}
        internal static void WriteFrame(Stream stream,byte[] bytes){if(bytes.Length<1||bytes.Length>1024*1024)throw new InvalidDataException("Invalid frame size");byte[] length=BitConverter.GetBytes(bytes.Length);stream.Write(length,0,4);stream.Write(bytes,0,bytes.Length);}
        public void Dispose(){closed=true;lock(gate){if(pending!=null)pending.Dispose();}if(thread!=null&&Thread.CurrentThread!=thread)thread.Join(1000);}
    }
}
