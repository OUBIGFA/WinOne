"""Measure the actual hidden bundled worker with a disposable local collection."""
import argparse
import ctypes
import json
import statistics
import subprocess
import tempfile
import time
from ctypes import wintypes
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument("--bundle",required=True)
parser.add_argument("--output",required=True)
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
bundle,output=Path(args.bundle).resolve(),Path(args.output).resolve()
if not output.is_relative_to(root/"_temp"):raise ValueError("Use repository task output")
output.mkdir(parents=True,exist_ok=True)
class Counters(ctypes.Structure):
    _fields_=[("cb",wintypes.DWORD),("PageFaultCount",wintypes.DWORD)]+[(name,ctypes.c_size_t) for name in
        ["PeakWorkingSetSize","WorkingSetSize","QuotaPeakPagedPoolUsage","QuotaPagedPoolUsage","QuotaPeakNonPagedPoolUsage","QuotaNonPagedPoolUsage","PagefileUsage","PeakPagefileUsage","PrivateUsage"]]
kernel=ctypes.WinDLL("kernel32",use_last_error=True)
kernel.OpenProcess.argtypes=[wintypes.DWORD,wintypes.BOOL,wintypes.DWORD];kernel.OpenProcess.restype=wintypes.HANDLE
kernel.CloseHandle.argtypes=[wintypes.HANDLE]
query=ctypes.WinDLL("psapi",use_last_error=True).GetProcessMemoryInfo
query.argtypes=[wintypes.HANDLE,ctypes.POINTER(Counters),wintypes.DWORD];query.restype=wintypes.BOOL
def memory(pid):
    handle=kernel.OpenProcess(0x410,False,pid)
    if not handle:raise ctypes.WinError(ctypes.get_last_error())
    try:
        counters=Counters();counters.cb=ctypes.sizeof(counters)
        if not query(handle,ctypes.byref(counters),counters.cb):raise ctypes.WinError(ctypes.get_last_error())
        return {"workingSetBytes":counters.WorkingSetSize,"privateBytes":counters.PrivateUsage}
    finally:kernel.CloseHandle(handle)
samples=[]
for index in range(5):
    with tempfile.TemporaryDirectory(dir=output) as profile:
        started=time.perf_counter()
        process=subprocess.Popen([str(bundle/"capture/Ocr/python/python.exe"),"-B","-u",str(bundle/"shared/anki/worker.py"),
                "--profile",profile,"--library",str(bundle/"shared/anki/lib"),"--source",str(bundle/"shared/anki/connect")],
                stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,encoding="utf-8",creationflags=subprocess.CREATE_NO_WINDOW)
        sequence=0
        def call(method,params):
            global sequence
            sequence+=1;process.stdin.write(json.dumps({"id":sequence,"method":method,"params":params})+"\n");process.stdin.flush()
            line=process.stdout.readline()
            if not line:raise RuntimeError(process.stderr.read())
            response=json.loads(line)
            if response.get("error"):raise RuntimeError(response["error"])
            return response["result"]
        try:
            call("winone.overview",{})
            first=(time.perf_counter()-started)*1000
            latencies=[]
            for _ in range(20):
                begin=time.perf_counter();call("winone.overview",{});latencies.append((time.perf_counter()-begin)*1000)
            record={"sample":index+1,"firstOverviewMs":first,"warmMedianMs":statistics.median(latencies),**memory(process.pid)}
            close=time.perf_counter();process.stdin.close();code=process.wait(timeout=10);record["exitMs"]=(time.perf_counter()-close)*1000
            if code:raise RuntimeError(process.stderr.read())
            record["exitCode"]=code;samples.append(record)
        finally:
            if process.poll() is None:process.kill();process.wait()
report={"scope":"isolated bundled Anki worker only; no UI/Host/system/network/audio test", "samples":samples,
        "median":{key:statistics.median(s[key] for s in samples) for key in ["firstOverviewMs","warmMedianMs","workingSetBytes","privateBytes","exitMs"]}}
(output/"performance.json").write_text(json.dumps(report,indent=2)+"\n",encoding="utf-8")
print(json.dumps(report["median"],indent=2))
