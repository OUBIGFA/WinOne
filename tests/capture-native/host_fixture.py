"""Local named-pipe provider fixture; never contacts an account or network."""
import ctypes
from ctypes import wintypes as w
import json
from pathlib import Path
import struct
import threading
import time
import uuid

class HostFixture:
    def __init__(self, directory, settings):
        self.directory=Path(directory);self.settings=dict(settings);self.jobs={};self.requests=[];self.error=None
        self.settings_saved=None
        self.translation_polls=0
        self.ocr_ready=False
        self.ocr_failure=False
        self.name='\\\\.\\pipe\\WinOne-NativeHost-Test-'+uuid.uuid4().hex
        self.k=ctypes.WinDLL('kernel32',use_last_error=True)
        self.k.CreateNamedPipeW.argtypes=[w.LPCWSTR,w.DWORD,w.DWORD,w.DWORD,w.DWORD,w.DWORD,w.DWORD,ctypes.c_void_p];self.k.CreateNamedPipeW.restype=w.HANDLE
        self.k.ConnectNamedPipe.argtypes=[w.HANDLE,ctypes.c_void_p]
        self.k.ReadFile.argtypes=[w.HANDLE,ctypes.c_void_p,w.DWORD,ctypes.POINTER(w.DWORD),ctypes.c_void_p]
        self.k.WriteFile.argtypes=[w.HANDLE,ctypes.c_void_p,w.DWORD,ctypes.POINTER(w.DWORD),ctypes.c_void_p]
        self.k.FlushFileBuffers.argtypes=[w.HANDLE]
        self.k.DisconnectNamedPipe.argtypes=[w.HANDLE]
        self.k.CloseHandle.argtypes=[w.HANDLE]
        self.k.OpenFileMappingW.argtypes=[w.DWORD,w.BOOL,w.LPCWSTR];self.k.OpenFileMappingW.restype=w.HANDLE
        self.k.MapViewOfFile.argtypes=[w.HANDLE,w.DWORD,w.DWORD,w.DWORD,ctypes.c_size_t];self.k.MapViewOfFile.restype=ctypes.c_void_p
        self.k.UnmapViewOfFile.argtypes=[ctypes.c_void_p]
        self.ready=threading.Event();self.stopping=False
        self.thread=threading.Thread(target=self.serve,daemon=True);self.thread.start()
        if not self.ready.wait(5):raise RuntimeError('Local Host fixture did not start')
    def exact(self,handle,count):
        result=bytearray()
        while len(result)<count:
            buffer=ctypes.create_string_buffer(count-len(result));read=w.DWORD()
            if not self.k.ReadFile(handle,buffer,len(buffer),ctypes.byref(read),None) or read.value==0:raise OSError('Fixture client disconnected')
            result.extend(buffer.raw[:read.value])
        return bytes(result)
    def handle(self,request):
        method=request['method'];params=request.get('params',{});self.requests.append(method)
        if method=='capture.settings.get':return self.settings
        if method=='capture.settings.save':
            if params['revision']!=self.settings['revision']:raise ValueError('Revision conflict')
            self.settings=dict(params);self.settings['revision']+=1
            (self.directory/'managed-settings.json').write_text(json.dumps(self.settings),'utf-8')
            if self.settings_saved:self.settings_saved()
            return self.settings
        if method=='ai.settings.get':return {'ocrEngine':'local'}
        if method=='ai.ocr.prepare':return {'queued':True}
        if method=='ai.recognize.start':
            # Read the real shared image before acknowledging its lifetime.
            image=params['image'];mapping=self.k.OpenFileMappingW(4,False,image['mapping'])
            if not mapping:raise ctypes.WinError(ctypes.get_last_error())
            view=self.k.MapViewOfFile(mapping,4,0,0,image['bytes'])
            try:
                if not view:raise ctypes.WinError(ctypes.get_last_error())
                import io
                from PIL import Image
                with Image.open(io.BytesIO(ctypes.string_at(view,image['bytes']))) as frame:
                    if frame.size!=(460,280):raise AssertionError(f'OCR crop differs: {frame.size}')
            finally:
                if view:self.k.UnmapViewOfFile(view)
                self.k.CloseHandle(mapping)
            identifier=uuid.uuid4().hex;self.jobs[identifier]={'id':identifier,'ocr':True}
            return {'id':identifier,'state':'running'}
        if method=='ai.translate.target':return {'target':'zh-CN'}
        if method=='ai.translate.start':
            identifier=uuid.uuid4().hex;self.jobs[identifier]={'id':identifier,'state':'completed','text':'测试译文','remaining':self.translation_polls}
            return {'id':identifier,'state':'running'}
        if method=='ai.jobs.get':
            job=self.jobs[params['id']]
            if job.get('ocr'):
                if not self.ocr_ready:
                    time.sleep(.01)
                    return {'id':job['id'],'state':'running'}
                if self.ocr_failure:return {'id':job['id'],'state':'failed','error':'OCR fixture failure'}
                return {'id':job['id'],'state':'completed','text':'识别并复制 Test 123'}
            if job['remaining']:
                job['remaining']-=1
                return {'id':job['id'],'state':'running'}
            return job
        if method=='ai.jobs.cancel':self.jobs.pop(params['id'],None);return {'cancelled':True}
        raise ValueError('Unexpected local fixture method '+method)
    def serve(self):
        try:
            while not self.stopping:
                pipe=self.k.CreateNamedPipeW(self.name,3,8,1,1048576,1048576,0,None)
                if pipe==ctypes.c_void_p(-1).value:raise ctypes.WinError(ctypes.get_last_error())
                self.ready.set()
                try:
                    if not self.k.ConnectNamedPipe(pipe,None) and ctypes.get_last_error()!=535:raise ctypes.WinError(ctypes.get_last_error())
                    length,=struct.unpack('<I',self.exact(pipe,4))
                    if not 0<length<=1048576:raise ValueError('Host request too large')
                    request=json.loads(self.exact(pipe,length))
                    try:reply={'id':request['id'],'result':self.handle(request)}
                    except Exception as error:reply={'id':request.get('id',''),'error':{'code':'fixture','message':str(error)}}
                    message=json.dumps(reply).encode();data=struct.pack('<I',len(message))+message;written=w.DWORD()
                    if not self.k.WriteFile(pipe,data,len(data),ctypes.byref(written),None) or written.value!=len(data):raise OSError('Fixture response failed')
                    self.k.FlushFileBuffers(pipe)
                except OSError:
                    if not self.stopping:raise
                finally:self.k.DisconnectNamedPipe(pipe);self.k.CloseHandle(pipe)
        except Exception as error:self.error=str(error)
