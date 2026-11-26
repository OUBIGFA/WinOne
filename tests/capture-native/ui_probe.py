"""Run the production native capture worker on an already isolated desktop."""
import argparse
import ctypes
import json
import os
from pathlib import Path
import struct
import subprocess
import sys
sys.dont_write_bytecode=True
import time
import uuid
from ctypes import wintypes as w

parser = argparse.ArgumentParser()
parser.add_argument('--executable', required=True)
parser.add_argument('--output', required=True)
parser.add_argument('--theme', choices=['light','dark'], default='light')
parser.add_argument('--language', choices=['zh-CN','en-US'], default='zh-CN')
parser.add_argument('--width',type=int,default=0)
parser.add_argument('--scale',type=float,default=1)
parser.add_argument('--recording-runtime')
parser.add_argument('--physical-input',action='store_true')
parser.add_argument('--agent-desktop')
parser.add_argument('--suite', choices=['snap','smoke','annotations','resume','recording-options','toolbar','translation','number','settings','scroll','save-dialog','save-failure','pin','repeat','dialog-owner-exit','recording','parent-exit','recording-parent'], default='smoke')
args = parser.parse_args()
output = Path(args.output).resolve()
root = Path(__file__).resolve().parents[2]
if not output.is_relative_to(root / '_temp'):
    raise ValueError('Private capture test data must be inside repository _temp')
output.mkdir(parents=True, exist_ok=False)
(output/'temp').mkdir();os.environ['TEMP']=os.environ['TMP']=str(output/'temp')
if args.suite in ('recording','recording-parent'):
    if not args.recording_runtime:raise ValueError('Recording control tests require the generated-frame runtime')
    generated=Path(args.recording_runtime).resolve()
    if not generated.is_relative_to(root/'_temp') or generated.name!='WinOne.Recording.SyntheticTest.dll':raise ValueError('Recording fixture runtime must be task-owned and explicitly named')
    os.environ['WINONE_CAPTURE_RECORDING_TEST']=str(generated)

user = ctypes.WinDLL('user32', use_last_error=True)
kernel = ctypes.WinDLL('kernel32', use_last_error=True)
user.GetThreadDesktop.argtypes = [w.DWORD]; user.GetThreadDesktop.restype = w.HANDLE
user.GetUserObjectInformationW.argtypes = [w.HANDLE, ctypes.c_int, ctypes.c_void_p, w.DWORD, ctypes.POINTER(w.DWORD)]
kernel.GetCurrentThreadId.restype = w.DWORD
desktop_name = ctypes.create_unicode_buffer(256); needed = w.DWORD()
desktop = user.GetThreadDesktop(kernel.GetCurrentThreadId())
if not user.GetUserObjectInformationW(desktop, 2, desktop_name, ctypes.sizeof(desktop_name), ctypes.byref(needed)):
    raise ctypes.WinError(ctypes.get_last_error())
if not desktop_name.value.startswith('WinOne-Test-'):
    raise RuntimeError('Capture probe refuses the user desktop')
physical_deadline=time.monotonic()+20
user.OpenInputDesktop.argtypes=[w.DWORD,w.BOOL,w.DWORD];user.OpenInputDesktop.restype=w.HANDLE
user.CloseDesktop.argtypes=[w.HANDLE]
user.ClientToScreen.argtypes=[w.HWND,ctypes.POINTER(w.POINT)]

def physical_click(window,x,y,hold=False):
    if not args.agent_desktop or time.monotonic()>=physical_deadline:raise RuntimeError('Physical input lease expired')
    current=user.OpenInputDesktop(0,False,1)
    if not current:raise ctypes.WinError(ctypes.get_last_error())
    try:
        name=ctypes.create_unicode_buffer(256);size=w.DWORD()
        if not user.GetUserObjectInformationW(current,2,name,ctypes.sizeof(name),ctypes.byref(size)) or name.value!=desktop_name.value:
            raise RuntimeError('Physical input refuses a different input desktop')
    finally:user.CloseDesktop(current)
    point=w.POINT(x,y)
    if not user.ClientToScreen(window,ctypes.byref(point)):raise ctypes.WinError(ctypes.get_last_error())
    result=subprocess.run([args.agent_desktop,'--headed','mouse-move' if hold else 'mouse-click','--xy',f'{point.x},{point.y}'],capture_output=True,timeout=3)
    response=json.loads(result.stdout)
    if result.returncode or not response.get('ok'):raise RuntimeError(f'Physical click failed: {response}')
    if hold:
        # agent-desktop does not implement held mouse input. The test needs
        # button-up to arrive after the modal dialog has attached its queue.
        class Mouse(ctypes.Structure):
            _fields_=[('dx',w.LONG),('dy',w.LONG),('data',w.DWORD),('flags',w.DWORD),('time',w.DWORD),('extra',ctypes.c_size_t)]
        class Input(ctypes.Structure):
            _fields_=[('kind',w.DWORD),('mouse',Mouse)]
        user.SendInput.argtypes=[w.UINT,ctypes.POINTER(Input),ctypes.c_int]
        down=Input(0,Mouse(0,0,0,2,0,0));up=Input(0,Mouse(0,0,0,4,0,0))
        if user.SendInput(1,ctypes.byref(down),ctypes.sizeof(Input))!=1:raise ctypes.WinError(ctypes.get_last_error())
        try:wait_until(file_dialog,timeout=3)
        finally:
            if user.SendInput(1,ctypes.byref(up),ctypes.sizeof(Input))!=1:raise ctypes.WinError(ctypes.get_last_error())

user.CreateWindowExW.argtypes = [w.DWORD,w.LPCWSTR,w.LPCWSTR,w.DWORD,ctypes.c_int,ctypes.c_int,ctypes.c_int,ctypes.c_int,w.HWND,w.HMENU,w.HINSTANCE,ctypes.c_void_p]
user.CreateWindowExW.restype = w.HWND
user.ShowWindow.argtypes = [w.HWND,ctypes.c_int]
user.UpdateWindow.argtypes = [w.HWND]
user.DestroyWindow.argtypes = [w.HWND]
user.SendMessageTimeoutW.argtypes = [w.HWND,w.UINT,w.WPARAM,w.LPARAM,w.UINT,w.UINT,ctypes.POINTER(ctypes.c_size_t)]
user.SendMessageTimeoutW.restype = w.LPARAM
user.PostMessageW.argtypes = [w.HWND,w.UINT,w.WPARAM,w.LPARAM]
user.GetWindowThreadProcessId.argtypes = [w.HWND,ctypes.POINTER(w.DWORD)]
user.GetClassNameW.argtypes = [w.HWND,w.LPWSTR,ctypes.c_int]
user.IsWindowVisible.argtypes = [w.HWND]
user.IsWindowEnabled.argtypes = [w.HWND]
enum_callback = ctypes.WINFUNCTYPE(w.BOOL,w.HWND,w.LPARAM)
user.EnumWindows.argtypes = [enum_callback,w.LPARAM]
user.EnumChildWindows.argtypes = [w.HWND,enum_callback,w.LPARAM]
user.SetWindowTextW.argtypes = [w.HWND,w.LPCWSTR]
user.AttachThreadInput.argtypes = [w.DWORD,w.DWORD,w.BOOL]
user.GetKeyboardState.argtypes = [ctypes.POINTER(ctypes.c_ubyte)]
user.SetKeyboardState.argtypes = [ctypes.POINTER(ctypes.c_ubyte)]
user.SetForegroundWindow.argtypes = [w.HWND]
user.GetWindowTextW.argtypes=[w.HWND,w.LPWSTR,ctypes.c_int]
user.GetDlgItem.argtypes=[w.HWND,ctypes.c_int];user.GetDlgItem.restype=w.HWND
kernel.CreateFileW.argtypes = [w.LPCWSTR,w.DWORD,w.DWORD,ctypes.c_void_p,w.DWORD,w.DWORD,w.HANDLE]
kernel.CreateFileW.restype = w.HANDLE
kernel.WaitNamedPipeW.argtypes = [w.LPCWSTR,w.DWORD]
kernel.CloseHandle.argtypes = [w.HANDLE]
kernel.WriteFile.argtypes = [w.HANDLE,ctypes.c_void_p,w.DWORD,ctypes.POINTER(w.DWORD),ctypes.c_void_p]
kernel.ReadFile.argtypes = [w.HANDLE,ctypes.c_void_p,w.DWORD,ctypes.POINTER(w.DWORD),ctypes.c_void_p]

fixture = user.CreateWindowExW(0, 'STATIC', 'WinOne private capture fixture — 中文 123', 0x10CF0000,
                             100, 100, 800, 600, None, None, None, None)
if not fixture:
    raise ctypes.WinError(ctypes.get_last_error())
user.ShowWindow(fixture, 5); user.UpdateWindow(fixture)
fixture_child=user.CreateWindowExW(0,'BUTTON','Snap child',0x50010000,120,90,240,100,fixture,None,None,None)
user.UpdateWindow(fixture_child)
settings = {'schemaVersion':1, 'revision':0, 'outputDirectory':str(output/'saved'),
            'copyToClipboard':False,'askWhereToSave':False,'openEditor':True,
            'pinAfterCapture':False,'pinTranslation':False,'theme':args.theme,
            'scrollDelay':300,'scrollFromTop':False,'toolbarTools':None,'toolbarVersion':3,'fileNameTemplate':'native-test-{sequence}'}
if args.suite in ('recording','recording-parent'):
    settings.update(gifAskWhereToSave=False,gifOutputDirectory=str(output/'gifs'))
if args.suite=='save-failure':
    (output/'saved').write_text('Owned path conflict fixture','utf-8')
(output/'managed-settings.json').write_text(json.dumps(settings, ensure_ascii=False),'utf-8')
pipe = '\\\\.\\pipe\\WinOne-Capture-Test-' + uuid.uuid4().hex
child = None
owner_process=None
if args.suite in ('parent-exit','recording-parent'):
    owner_process=subprocess.Popen([sys.executable,'-c','import time;time.sleep(120)'],creationflags=0x08000000)
events = []
host_fixture=None
extra_arguments=[]
if args.suite in ('toolbar','translation','pin'):
    from host_fixture import HostFixture
    if args.suite in ('translation','pin'):
        from PIL import Image,ImageDraw,ImageFont
        settings['pinTranslation']=True
        (output/'managed-settings.json').write_text(json.dumps(settings),'utf-8')
        frame=Image.new('RGB',(user.GetSystemMetrics(78),user.GetSystemMetrics(79)),'white')
        font=ImageFont.truetype(str(Path(os.environ['WINDIR'])/'Fonts/msyh.ttc'),26)
        ImageDraw.Draw(frame).text((190,220),'Hello native capture',font=font,fill='black')
        frame.save(output/'fixture.png');os.environ['WINONE_CAPTURE_FIXTURE']=str(output/'fixture.png')
    host_fixture=HostFixture(output,settings)
    extra_arguments=['--host-pipe',host_fixture.name]

def rpc(method, params=None):
    if not kernel.WaitNamedPipeW(pipe, 1000):
        raise ctypes.WinError(ctypes.get_last_error())
    handle = kernel.CreateFileW(pipe, 0xC0000000,0,None,3,0,None)
    if handle == ctypes.c_void_p(-1).value:
        raise ctypes.WinError(ctypes.get_last_error())
    try:
        request_id = uuid.uuid4().hex
        message = json.dumps({'version':1,'id':request_id,'method':method,'params':params or {}}).encode()
        data = struct.pack('<I',len(message))+message
        sent = w.DWORD()
        if not kernel.WriteFile(handle,data,len(data),ctypes.byref(sent),None) or sent.value != len(data):
            raise ctypes.WinError(ctypes.get_last_error())
        def read(length):
            result = bytearray()
            while len(result) < length:
                buffer = ctypes.create_string_buffer(length-len(result)); count=w.DWORD()
                if not kernel.ReadFile(handle,buffer,len(buffer),ctypes.byref(count),None) or not count.value:
                    raise ctypes.WinError(ctypes.get_last_error())
                result.extend(buffer.raw[:count.value])
            return bytes(result)
        size, = struct.unpack('<I', read(4))
        if not 0 < size <= 1048576: raise AssertionError('Invalid native reply size')
        reply = json.loads(read(size))
        if reply['id'] != request_id: raise AssertionError('Native reply identity changed')
        if reply.get('error'): raise AssertionError(reply['error'])
        return reply['result']
    finally:
        kernel.CloseHandle(handle)

def overlay():
    found=[]
    @enum_callback
    def check(window, _):
        pid=w.DWORD();user.GetWindowThreadProcessId(window,ctypes.byref(pid))
        name=ctypes.create_unicode_buffer(256);user.GetClassNameW(window,name,len(name))
        if pid.value == child.pid and name.value == 'Tools3000_CaptureOverlay' and user.IsWindowVisible(window):found.append(window)
        return True
    user.EnumWindows(check,0)
    return found[0] if len(found)==1 else None

def send(window, message, wp=0, lp=0):
    result=ctypes.c_size_t()
    if not user.SendMessageTimeoutW(window,message,wp,lp,2,3000,ctypes.byref(result)):
        raise ctypes.WinError(ctypes.get_last_error())

def key(window, value, control=False, shift=False, modal=False):
    target=user.GetWindowThreadProcessId(window,None)
    current=kernel.GetCurrentThreadId()
    if not user.AttachThreadInput(current,target,True):raise ctypes.WinError(ctypes.get_last_error())
    old=(ctypes.c_ubyte*256)()
    try:
        if not user.GetKeyboardState(old):raise ctypes.WinError(ctypes.get_last_error())
        state=(ctypes.c_ubyte*256)(*old)
        state[0x11]=state[0xA2]=0x80 if control else 0
        state[0x10]=state[0xA0]=0x80 if shift else 0
        if not user.SetKeyboardState(state):raise ctypes.WinError(ctypes.get_last_error())
        if modal:
            if not user.PostMessageW(window,0x100,value,0):raise ctypes.WinError(ctypes.get_last_error())
            wait_until(file_dialog)
        else:send(window,0x100,value,0)
    finally:
        user.SetKeyboardState(old)
        user.AttachThreadInput(current,target,False)

def number_editor(canvas):
    found=[]
    @enum_callback
    def check(window,_):
        name=ctypes.create_unicode_buffer(256);user.GetClassNameW(window,name,len(name))
        if name.value.lower()=='edit' and user.IsWindowVisible(window):found.append(window)
        return True
    user.EnumChildWindows(canvas,check,0)
    return found[0] if len(found)==1 else None

def titled_window(title):
    found=[]
    @enum_callback
    def check(window,_):
        pid=w.DWORD();user.GetWindowThreadProcessId(window,ctypes.byref(pid))
        text=ctypes.create_unicode_buffer(256);user.GetWindowTextW(window,text,len(text))
        if pid.value==child.pid and text.value==title and user.IsWindowVisible(window):found.append(window)
        return True
    user.EnumWindows(check,0)
    return found[0] if len(found)==1 else None

def file_dialog(title=None):
    # The test desktop contains only our fixture, capture worker and its modal
    # file-dialog child. Never enumerate the interactive user desktop.
    found=[]
    @enum_callback
    def check(window,_):
        name=ctypes.create_unicode_buffer(128);user.GetClassNameW(window,name,len(name))
        text=ctypes.create_unicode_buffer(128);user.GetWindowTextW(window,text,len(text))
        if name.value=='#32770' and user.IsWindowVisible(window) and (title is None or text.value==title):found.append(window)
        return True
    user.EnumWindows(check,0)
    return found[0] if len(found)==1 else None

def choose_recording_file(dialog, destination):
    # Target the filename field by its suggested extension, avoiding the
    # unrelated search box and keeping every accepted path in this fixture.
    fields=[]
    @enum_callback
    def find(window,_):
        name=ctypes.create_unicode_buffer(128);user.GetClassNameW(window,name,len(name))
        text=ctypes.create_unicode_buffer(32768)
        if name.value=='Edit':send(window,0x0D,len(text),ctypes.addressof(text))
        if name.value=='Edit' and text.value.lower().endswith(('.gif','.mp4')):fields.append(window)
        return True
    user.EnumChildWindows(dialog,find,0)
    if len(fields)!=1:raise AssertionError(f'Recording filename field is ambiguous: {len(fields)}')
    destination.parent.mkdir(parents=True,exist_ok=True)
    if args.physical_input:physical_click(fields[0],10,10)
    else:send(dialog,0x28,fields[0],1)
    def accept():
        if args.physical_input:
            button=user.GetDlgItem(dialog,1)
            if not button:raise AssertionError('Save button was not found')
            physical_click(button,15,10)
        else:user.PostMessageW(dialog,0x111,1,0)
    def type_path(path):
        send(fields[0],0xB1,0,-1)
        for character in str(path):send(fields[0],0x102,ord(character),0)
    # Navigate through a real local folder before accepting the filename; the
    # former cancel-only probe never exercised the Shell's browsing path.
    type_path(str(destination.parent)+'\\')
    accept()
    time.sleep(.5)
    if not file_dialog():raise AssertionError('Folder navigation unexpectedly closed save dialog')
    rpc('snapshot')
    type_path(destination)
    accept()
    wait_until(lambda:not file_dialog())

def click_point(canvas,x,y):
    point=(y<<16)|x
    send(canvas,0x201,1,point);send(canvas,0x202,0,point)

def menu_item(canvas,label):
    item=wait_until(lambda:next((item for item in rpc('probe.state')['menu'] if item['label']==label and item['y']>0),None))
    click_point(canvas,round(item['x']),round(item['y']))

def wait_until(predicate, timeout=10):
    limit=time.monotonic()+timeout
    while time.monotonic()<limit:
        value=predicate()
        if value:return value
        if child.poll() is not None: raise AssertionError(f'Native capture exited early: {child.returncode}')
        time.sleep(.025)
    raise TimeoutError('Native capture condition did not become true')

def click_button(canvas,label):
    def find():
        choices=[b for b in rpc('probe.state')['buttons'] if b['label']==label and not b['secondary']]
        return choices[0] if len(choices)==1 else None
    button=wait_until(find)
    if args.physical_input:
        physical_click(canvas,round(button['x']),round(button['y']),hold=label in ('GIF','录制视频'))
        return
    point=(round(button['y'])<<16)|round(button['x'])
    send(canvas,0x200,0,point)
    if not user.PostMessageW(canvas,0x201,1,point) or not user.PostMessageW(canvas,0x202,0,point):
        raise ctypes.WinError(ctypes.get_last_error())

try:
    with (output/'stdout.log').open('wb') as stdout, (output/'stderr.log').open('wb') as stderr:
        child=subprocess.Popen([args.executable,'--pipe',pipe,'--data-dir',str(output),'--parent',str(owner_process.pid if owner_process else os.getpid())]+extra_arguments,
            stdout=stdout,stderr=stderr,creationflags=0x08000000)
        def ready():
            try:return rpc('snapshot')
            except OSError:return None
        snapshot=wait_until(ready)
        if host_fixture:host_fixture.settings_saved=lambda:rpc('settings.reload')
        if snapshot['processId'] != child.pid: raise AssertionError('Snapshot process differs')
        events.append({'snapshot':snapshot})
        job=rpc('capture.unified',{'mode':'region','language':args.language})
        canvas=wait_until(overlay)
        user.SetForegroundWindow(canvas)
        if args.suite=='snap':
            # Real window/control detector; no synthesized candidate rectangle.
            send(canvas,0x200,0,(230<<16)|350)
            detected=wait_until(lambda:(s if (s:=rpc('probe.state'))['detectionCandidates'] else None))
            if detected['shortcutHints']:raise AssertionError('Shortcut overlay remained enabled')
            events.append({'windowAndControlDetection':detected['detected'],'candidates':detected['detectionCandidates']})
            target=detected['detected']
            send(canvas,0x201,1,((target[1]+20)<<16)|(target[0]+20))
            send(canvas,0x200,1,((target[3]-3)<<16)|(target[2]-3))
            send(canvas,0x202,0,((target[3]-3)<<16)|(target[2]-3))
            selected=rpc('probe.state')['selection']
            if selected[2]!=target[2] or selected[3]!=target[3]:raise AssertionError(f'Edges did not snap: {selected} to {target}')
            events.append({'draggedEdgesSnapped':selected})
            key(canvas,0x1B)
            wait_until(lambda:rpc('jobs.get',{'id':job['id']})['state']=='completed')
            job=rpc('capture.unified',{'mode':'region','language':args.language});canvas=wait_until(overlay)
        # Input goes to the owned HWND on this private desktop only.
        send(canvas,0x201,1,(180<<16)|160)
        send(canvas,0x200,1,(460<<16)|620)
        send(canvas,0x202,0,(460<<16)|620)
        events.append({'selected':True})
        if args.suite=='parent-exit':
            key(canvas,ord('N'));click_point(canvas,260,260)
            edit=wait_until(lambda:number_editor(canvas))
            for character in 'Host exit draft':send(edit,0x102,ord(character),0)
            expected=wait_until(lambda:rpc('draft.get')['draft'])['id']
            owner_process.terminate();owner_process.wait(timeout=5)
            child.wait(timeout=15)
            if child.returncode:raise AssertionError('Capture failed while preserving Host-exit draft')
            child=subprocess.Popen([args.executable,'--pipe',pipe,'--data-dir',str(output),'--parent',str(os.getpid())],stdout=stdout,stderr=stderr,creationflags=0x08000000)
            wait_until(ready)
            if rpc('draft.get')['draft']['id']!=expected:raise AssertionError('Host-exit draft identity changed')
            job=rpc('draft.resume',{'expectedId':expected,'language':'zh-CN'});canvas=wait_until(overlay)
            if not any(e['text']=='Host exit draft' for e in rpc('probe.state')['elements']):raise AssertionError('Host exit lost unfinished annotation')
            events.append({'hostExitPreservedAndRestoredText':True})
        if args.suite in ('recording','recording-parent'):
            from PIL import Image
            def begin_recording(format,auto=False):
                click_button(canvas,'GIF' if format=='gif' else '录制视频')
                ready=wait_until(lambda:(s if (s:=rpc('probe.state'))['recordingMode'] else None))
                if not ready['recordingDestination']:raise AssertionError('Ready toolbar has no confirmed save destination')
                if ready['recording'] and ready['recording']['phase']=='recording':raise AssertionError('Recording started before Start')
                shot='recording-ready-'+format+('-auto' if auto else '')+'.png'
                rpc('probe.render',{'name':shot});wait_until(lambda:(output/shot).is_file())
                if auto:
                    click_button(canvas,'120 秒');menu_item(canvas,'3 秒')
                click_button(canvas,'开始录制')
                wait_until(lambda:(s if (s:=rpc('probe.state'))['recording'] and s['recording']['phase']=='recording' else None))
                return wait_until(lambda:(u if (u:=rpc('probe.state')['recordingUI'])['hwnd'] and u['buttons'][2][0]>0 else None))
            def recording_button(index):
                ui=rpc('probe.state')['recordingUI'];x,y=ui['buttons'][index];click_point(ui['hwnd'],x,y)
            scenarios=[('gif','owner-exit')] if args.suite=='recording-parent' else [('gif','stop'),('mp4','unpin'),('gif','cancel'),('gif','auto')]
            for format,action in scenarios:
                ui=begin_recording(format,action=='auto')
                wait_until(lambda:rpc('probe.state')['recording']['encodedFrames']>=3)
                shot='recording-active-'+format+'-'+action+'.png'
                rpc('probe.render',{'name':shot});wait_until(lambda:(output/shot).is_file())
                recording_button(1)
                paused=wait_until(lambda:(s if (s:=rpc('probe.state')['recording'])['phase']=='paused' else None))
                time.sleep(.25)
                if rpc('probe.state')['recording']['durationMs']!=paused['durationMs']:raise AssertionError('Paused recording clock advanced')
                recording_button(1)
                wait_until(lambda:rpc('probe.state')['recording']['phase']=='recording')
                wait_until(lambda:rpc('probe.state')['recording']['durationMs']>paused['durationMs']+300)
                if action=='stop':recording_button(2)
                elif action=='unpin':
                    recording_button(0)
                    wait_until(lambda:not rpc('probe.state')['recordingUI']['pinned'])
                    send(ui['hwnd'],0x6,0,fixture)
                elif action=='cancel':
                    rpc('probe.discardRecording')
                    confirmation=wait_until(lambda:titled_window('WinOne'))
                    user.PostMessageW(confirmation,0x111,6,0)
                elif action=='owner-exit':
                    owner_process.terminate();owner_process.wait(timeout=5)
                    child.wait(timeout=20)
                    if child.returncode:raise AssertionError('Host exit did not gracefully save recording')
                    files=list((output/'gifs').glob('*.gif'))
                    if len(files)!=1:raise AssertionError('Host exit lost the recording output')
                    with Image.open(files[0]) as image:
                        if image.size!=(460,280) or image.n_frames<2:raise AssertionError('Host-exit recording was not finalized')
                    events.append({'hostExitFinalizedRecording':True,'path':str(files[0])})
                    child=subprocess.Popen([args.executable,'--pipe',pipe,'--data-dir',str(output),'--parent',str(os.getpid())],stdout=stdout,stderr=stderr,creationflags=0x08000000)
                    wait_until(ready);job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
                    break
                completed=wait_until(lambda:(j if (j:=rpc('jobs.get',{'id':job['id'],'waitMs':100}))['state']!='running' else None),timeout=30)
                if completed['state']!='completed':raise AssertionError(f'Recording failed: {completed}')
                recording=completed['result']['recording']
                if action=='cancel':
                    if recording['phase']!='cancelled' or recording.get('path'):raise AssertionError('Cancelled recording was published')
                else:
                    if action=='auto' and recording['reason']!='gifLimit':raise AssertionError('GIF limit did not finish through engine auto-stop')
                    path=Path(recording['path'])
                    if recording['phase']!='completed' or not path.is_file() or path.stat().st_size<100:raise AssertionError('Recording output was not finalized')
                    if format=='gif':
                        with Image.open(path) as image:
                            if image.size!=(460,280) or image.n_frames<2:raise AssertionError('Recorded GIF lost selected dimensions or frames')
                events.append({'nativeRecordingControls':action,'format':format,'recording':recording})
                job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
                send(canvas,0x201,1,(180<<16)|160);send(canvas,0x200,1,(460<<16)|620);send(canvas,0x202,0,(460<<16)|620)
        if args.suite=='dialog-owner-exit':
            wait_until(lambda:rpc('draft.get')['draft'])
            expected=rpc('draft.get')['draft']['id']
            click_button(canvas,'另存为')
            dialog=wait_until(file_dialog)
            worker_pid=w.DWORD();user.GetWindowThreadProcessId(dialog,ctypes.byref(worker_pid))
            if worker_pid.value==child.pid:raise AssertionError('File chooser did not use its owned worker')
            kernel.OpenProcess.argtypes=[w.DWORD,w.BOOL,w.DWORD];kernel.OpenProcess.restype=w.HANDLE
            kernel.WaitForSingleObject.argtypes=[w.HANDLE,w.DWORD]
            worker=kernel.OpenProcess(0x100000,False,worker_pid.value)
            if not worker:raise ctypes.WinError(ctypes.get_last_error())
            try:
                child.kill();child.wait(timeout=5)
                if kernel.WaitForSingleObject(worker,5000)!=0:raise AssertionError('File-dialog child survived its owner')
            finally:kernel.CloseHandle(worker)
            child=subprocess.Popen([args.executable,'--pipe',pipe,'--data-dir',str(output),'--parent',str(os.getpid())],stdout=stdout,stderr=stderr,creationflags=0x08000000)
            wait_until(ready)
            if rpc('draft.get')['draft']['id']!=expected:raise AssertionError('Owner exit discarded unfinished screenshot')
            job=rpc('draft.resume',{'expectedId':expected,'language':'zh-CN'});canvas=wait_until(overlay)
            events.append({'fileDialogChildReapedAndDraftRecovered':True})
        if args.suite=='repeat':
            from concurrent.futures import ThreadPoolExecutor
            with ThreadPoolExecutor(max_workers=4) as pool:
                concurrent=list(pool.map(lambda _:rpc('capture.unified',{'mode':'region','language':'zh-CN'}),range(4)))
            if any(x['id']!=job['id'] for x in concurrent):raise AssertionError('Repeated capture requests opened another job')
            for iteration in range(8):
                wait_until(lambda:rpc('draft.get')['draft'])
                key(canvas,0x1B)
                wait_until(lambda:rpc('jobs.get',{'id':job['id']})['state']=='completed')
                if rpc('draft.get')['draft'] is not None:raise AssertionError('Cancelled generation reappeared during background cleanup')
                job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
                send(canvas,0x201,1,(180<<16)|160);send(canvas,0x200,1,(460<<16)|620);send(canvas,0x202,0,(460<<16)|620)
            events.append({'concurrentReactivationAndRepeatedCancellation':8})
        if args.suite=='save-dialog':
            click_button(canvas,'另存为')
            dialog=wait_until(file_dialog)
            user.PostMessageW(dialog,0x111,2,0)
            wait_until(lambda:not file_dialog())
            if not overlay() or rpc('jobs.get',{'id':job['id']})['state']!='running':raise AssertionError('Cancelling Save As discarded screenshot')
            events.append({'saveDialogCancelRetainedScreenshot':True})
        if args.suite=='save-failure':
            click_button(canvas,'保存')
            wait_until(lambda:rpc('probe.state')['notice'])
            if not overlay() or rpc('jobs.get',{'id':job['id']})['state']!='running':raise AssertionError('Failed save discarded screenshot')
            if (output/'saved').read_text('utf-8')!='Owned path conflict fixture':raise AssertionError('Save failure overwrote existing content')
            events.append({'saveFailureRetainedPixelsAndExistingFile':True})
        if args.suite=='pin':
            click_button(canvas,'贴图')
            result=wait_until(lambda:(x if (x:=rpc('jobs.get',{'id':job['id']}))['state']!='running' else None))
            if result['state']!='completed' or not result['result']['pinned'] or overlay():raise AssertionError('Pin did not complete the capture')
            if rpc('snapshot')['pins']!=1:raise AssertionError('Community pin window did not open')
            pin=rpc('probe.state')['pin']['hwnd']
            key(pin,ord('S'),control=True,modal=True)
            dialog=wait_until(file_dialog);user.PostMessageW(dialog,0x111,2,0)
            wait_until(lambda:not file_dialog())
            if rpc('snapshot')['pins']!=1:raise AssertionError('Cancelling pin save lost the pin')
            key(pin,ord('E'))
            canvas=wait_until(overlay)
            selection=rpc('probe.state')['selection'];x=min(selection[0],selection[2])+100;y=min(selection[1],selection[3])+100
            click_button(canvas,'矩形');rpc('probe.state')
            send(canvas,0x201,1,(y<<16)|x);send(canvas,0x200,1,((y+60)<<16)|(x+70));send(canvas,0x202,0,((y+60)<<16)|(x+70))
            click_button(canvas,'确认并复制')
            wait_until(lambda:not overlay())
            if len(rpc('probe.state')['pin']['elements'])!=1:raise AssertionError('Pin edit completion bypassed community pin state')
            key(pin,ord('T'),control=True,shift=True)
            wait_until(lambda:rpc('probe.state')['pin']['translated'],timeout=30)
            if 'ai.translate.start' not in host_fixture.requests:raise AssertionError('Pin translation bypassed WinOne Host')
            key(pin,0x09)
            if rpc('probe.state')['pin']['translated']:raise AssertionError('Pin Tab did not restore original')
            key(pin,0x09)
            if not rpc('probe.state')['pin']['translated']:raise AssertionError('Pin Tab did not restore translation')
            host_fixture.settings['pinTranslation']=False
            (output/'managed-settings.json').write_text(json.dumps(host_fixture.settings),'utf-8');rpc('settings.reload')
            if rpc('probe.state')['pin']['translated']:raise AssertionError('Disabling translation did not restore pin original')
            closing=rpc('pins.close')
            wait_until(lambda:rpc('jobs.get',{'id':closing['id']})['state']=='completed')
            if rpc('snapshot')['pins']:raise AssertionError('Pin close did not release the window')
            events.append({'communityPinCreateAndClose':True})
            job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
        if args.suite in ('smoke','toolbar'):
            if args.width:
                rpc('probe.layout',{'width':args.width,'height':900,'scale':args.scale})
            wait_until(lambda:len(rpc('probe.state')['buttons'])>0)
            rpc('probe.render')
            wait_until(lambda:(output/'rendered.png').is_file())
            if rpc('probe.state')['shortcutHints']:raise AssertionError('Canvas shortcut hints are still enabled')
            tip=next(b for b in rpc('probe.state')['buttons'] if b['label']==('保存' if args.language=='zh-CN' else 'Save'))
            send(canvas,0x200,0,(round(tip['y'])<<16)|round(tip['x']))
            rpc('probe.render',{'name':'tooltip.png'});wait_until(lambda:(output/'tooltip.png').is_file())
            from PIL import Image
            with Image.open(output/'rendered.png') as image:
                if max(hi-lo for lo,hi in image.convert('RGB').getextrema())<20:raise AssertionError('Native render evidence has no visible content')
            if args.width:
                buttons=[b for b in rpc('probe.state')['buttons'] if not b['secondary']]
                for b in buttons:
                    x1,y1,x2,y2=b['rect']
                    if min(x1,y1)<0 or x2>args.width or y2>900:raise AssertionError('Toolbar button left the narrow screen')
                for i,a in enumerate(buttons):
                    for b in buttons[i+1:]:
                        if min(a['rect'][2],b['rect'][2])-max(a['rect'][0],b['rect'][0])>1 and min(a['rect'][3],b['rect'][3])-max(a['rect'][1],b['rect'][1])>1:raise AssertionError('Toolbar buttons overlap')
                events.append({'layoutWidth':args.width,'scale':args.scale,'theme':args.theme,'language':args.language})
        if args.suite=='scroll':
            original=rpc('probe.state')['selection']
            click_button(canvas,'滚动截图')
            ready=wait_until(lambda:rpc('probe.state')['scrollReady'])
            if rpc('probe.state')['scrolling']:raise AssertionError('Scroll acquisition began before Start')
            click_button(canvas,'开始滚动截图')
            wait_until(lambda:(s:=rpc('probe.state'))['scrolling'] and s['scrollFrames']>=3)
            controls=rpc('probe.state')['scrollUI'];x,y=controls['buttons'][3]
            click_point(controls['hwnd'],round(x),round(y))
            canvas=wait_until(overlay)
            if rpc('probe.state')['selection']!=original or rpc('jobs.get',{'id':job['id']})['state']!='running':raise AssertionError('Scrolling cancellation lost the original selection')
            # Recovered screenshots normally do not expose live screen actions;
            # this is a cancelled live acquisition and must still allow retry.
            click_button(canvas,'滚动截图')
            wait_until(lambda:rpc('probe.state')['scrollReady'])
            click_button(canvas,'开始滚动截图')
            wait_until(lambda:(s:=rpc('probe.state'))['scrolling'] and s['scrollFrames']>=3)
            rpc('scroll.stop')
            canvas=wait_until(overlay)
            state=wait_until(lambda:(s if (s:=rpc('probe.state'))['longImage'] else None))
            rpc('probe.render',{'name':'scroll-minimap.png'});wait_until(lambda:(output/'scroll-minimap.png').is_file())
            if state['imageSize'][0]!=460 or state['imageSize'][1]<=280:raise AssertionError('Community scroll stitching did not retain the selected width and extend the page')
            send(canvas,0x205,0,(300<<16)|350)
            send(canvas,0x203,1,(300<<16)|350)
            if not overlay() or rpc('jobs.get',{'id':job['id']})['state']!='running':raise AssertionError('Long-image click implicitly finished editing')
            before=rpc('probe.state')['zoom']
            send(canvas,0x20A,120<<16,(300<<16)|350)
            after=rpc('probe.state')['zoom']
            if before==after:raise AssertionError('Long-image wheel did not zoom')
            events.append({'communityScrollStitchAndExplicitExit':True,'imageSize':state['imageSize'],'zoomBefore':before,'zoomAfter':after})
            click_button(canvas,'重新滚动截图')
            wait_until(lambda:not rpc('probe.state')['longImage'])
            canvas=wait_until(overlay);key(canvas,0x1B)
            wait_until(lambda:rpc('probe.state')['longImage'])
            canvas=wait_until(overlay)
            if rpc('probe.state')['imageSize']!=state['imageSize']:raise AssertionError('Cancelled retake lost original long image')
            events.append({'retakeCancelRestoredLongImage':True})
        if args.suite=='toolbar':
            original=[b['label'] for b in rpc('probe.state')['buttons'] if not b['secondary']]
            click_button(canvas,'自定义工具栏')
            wait_until(lambda:rpc('probe.state')['menu'])
            rpc('probe.render',{'name':'toolbar-menu.png'});wait_until(lambda:(output/'toolbar-menu.png').is_file())
            click_point(canvas,80,80)
            if host_fixture.requests.count('capture.settings.save'):raise AssertionError('Cancelling toolbar saved preferences')
            if original!=[b['label'] for b in rpc('probe.state')['buttons'] if not b['secondary']]:raise AssertionError('Cancelling toolbar changed tools')
            click_button(canvas,'自定义工具栏')
            menu_item(canvas,'矩形')
            wait_until(lambda:host_fixture.settings['toolbarTools'] is not None and 'rectangle' not in host_fixture.settings['toolbarTools'])
            wait_until(lambda:'矩形' not in [b['label'] for b in rpc('probe.state')['buttons']])
            click_button(canvas,'自定义工具栏')
            menu_item(canvas,'恢复默认')
            wait_until(lambda:'矩形' in [b['label'] for b in rpc('probe.state')['buttons']])
            if host_fixture.requests.count('capture.settings.save')!=2:raise AssertionError('Toolbar save did not use Host twice')
            events.append({'toolbarCancelSaveReset':True})
        if args.suite=='settings':
            invalid=dict(settings,theme='unsupported')
            (output/'managed-settings.json').write_text(json.dumps(invalid),'utf-8')
            try:rpc('settings.reload')
            except AssertionError:pass
            else:raise AssertionError('Invalid settings reload was accepted')
            if rpc('snapshot')['settings']['theme']!='light':raise AssertionError('Invalid reload replaced accepted settings')
            updated=dict(settings,outputDirectory=str(output/'changed'),fileNameTemplate='changed-{sequence}',theme='dark',revision=1)
            (output/'managed-settings.json').write_text(json.dumps(updated),'utf-8')
            rpc('settings.reload')
            click_button(canvas,'保存')
            finished=wait_until(lambda:(x if (x:=rpc('jobs.get',{'id':job['id'],'waitMs':100}))['state']!='running' else None))
            if finished['state']!='completed':raise AssertionError(f'Frozen settings save failed: {finished}')
            destination=Path(finished['result']['path'])
            if destination.parent!=output/'saved' or not destination.name.startswith('native-test-'):raise AssertionError('Active screenshot output changed after settings reload')
            job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
            click_point(canvas,160,180)
            send(canvas,0x201,1,(180<<16)|160);send(canvas,0x200,1,(460<<16)|620);send(canvas,0x202,0,(460<<16)|620)
            click_button(canvas,'保存')
            finished=wait_until(lambda:(x if (x:=rpc('jobs.get',{'id':job['id'],'waitMs':100}))['state']!='running' else None))
            if finished['state']!='completed' or Path(finished['result']['path']).parent!=output/'changed':raise AssertionError('Next screenshot did not receive new settings')
            events.append({'settingsFrozenDuringCapture':True})
            job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
        if args.suite=='number':
            key(canvas,ord('N'));click_point(canvas,260,260)
            edit=wait_until(lambda:number_editor(canvas))
            rpc('probe.render',{'name':'number-empty.png'});wait_until(lambda:(output/'number-empty.png').is_file())
            for character in '第一条':send(edit,0x102,ord(character),0)
            click_point(canvas,400,360)
            edit2=wait_until(lambda:number_editor(canvas))
            values=rpc('probe.state')['elements']
            if len(values)!=2 or values[0]['text']!='第一条':raise AssertionError('Next click did not commit and create the next number')
            for character in '放弃':send(edit2,0x102,ord(character),0)
            key(edit2,0x1B)
            values=rpc('probe.state')['elements']
            if len(values)!=2 or values[1]['text']:raise AssertionError('Escape removed number or retained abandoned text')
            click_point(canvas,260,260)
            edit=wait_until(lambda:number_editor(canvas))
            for character in '修订说明':send(edit,0x102,ord(character),0)
            key(edit,0x0D,control=True)
            rpc('probe.render',{'name':'number-caption.png'});wait_until(lambda:(output/'number-caption.png').is_file())
            if number_editor(canvas):raise AssertionError('Control Enter did not commit number text')
            click_button(canvas,'移动标注')
            wait_until(lambda:rpc('probe.state')['moveTool'])
            events.append({'numberMoveToolSelected':True})
            send(canvas,0x201,1,(260<<16)|260);send(canvas,0x200,1,(280<<16)|280);send(canvas,0x202,0,(280<<16)|280)
            events.append({'afterNumberMove':rpc('probe.state')['elements']})
            key(canvas,0x2E)
            values=rpc('probe.state')['elements']
            if len(values)!=1 or values[0]['numberValue']!=1:raise AssertionError('Deleting moved number did not renumber')
            events.append({'numberDeleted':True})
            key(canvas,ord('Z'),control=True)
            if len(rpc('probe.state')['elements'])!=2:raise AssertionError('Number deletion could not be undone')
            events.append({'numberContinuousEditCancelMoveDeleteUndo':True})
        if args.suite=='translation':
            click_button(canvas,'翻译')
            events.append({'translationClicked':True})
            result=wait_until(lambda:(x if (x:=rpc('probe.state'))['translated'] else None),timeout=30)
            if not host_fixture.requests.count('ai.translate.start'):raise AssertionError('Translation bypassed the shared Host')
            key(canvas,0x09)
            if rpc('probe.state')['translated']:raise AssertionError('Tab did not show the original image')
            key(canvas,0x09)
            if not rpc('probe.state')['translated']:raise AssertionError('Tab did not restore translated image')
            click_button(canvas,'矩形');rpc('probe.state')
            send(canvas,0x201,1,(310<<16)|300);send(canvas,0x200,1,(380<<16)|390);send(canvas,0x202,0,(380<<16)|390)
            wait_until(lambda:not rpc('probe.state')['translated'])
            if not rpc('probe.state')['elements']:raise AssertionError('Translation invalidation did not follow an annotation edit')
            host_fixture.translation_polls=100
            click_button(canvas,'翻译')
            cancel_count=host_fixture.requests.count('ai.jobs.cancel')
            wait_until(lambda:rpc('probe.state')['textBusy'] and host_fixture.requests.count('ai.translate.start')>=2)
            click_button(canvas,'翻译')
            wait_until(lambda:not rpc('probe.state')['textBusy'])
            wait_until(lambda:host_fixture.requests.count('ai.jobs.cancel')>cancel_count)
            if rpc('probe.state')['translated']:raise AssertionError('Cancelled translation replaced current pixels')
            host_fixture.translation_polls=0
            click_button(canvas,'翻译')
            wait_until(lambda:rpc('probe.state')['translated'],timeout=30)
            host_fixture.settings['pinTranslation']=False
            (output/'managed-settings.json').write_text(json.dumps(host_fixture.settings),'utf-8')
            rpc('settings.reload')
            if rpc('probe.state')['translated']:raise AssertionError('Turning translation off did not restore original image')
            events.append({'realOcrAndTranslationRender':True,'hostRequests':host_fixture.requests})
        if args.suite=='recording-options':
            for label in ('GIF','录制视频'):
                click_button(canvas,label)
                dialog=wait_until(lambda:file_dialog('保存录制'))
                if args.physical_input and label=='GIF':
                    auxiliary=rpc('probe.modalInput',{'create':True})['hwnd']
                    physical_click(auxiliary,30,15)
                    typed=subprocess.run([args.agent_desktop,'--headed','press','1'],capture_output=True,timeout=3)
                    if typed.returncode:raise RuntimeError('Physical keyboard input failed')
                    time.sleep(.2)
                    actual=rpc('probe.modalInput')['text']
                    if actual!='1':raise AssertionError(f'Modal dialog starved real input on capture UI thread: {actual!r}')
                    events.append({'modalDialogDispatchesPhysicalInput':True})
                # A user can spend longer than the Host's 20-second watchdog
                # threshold choosing a folder. The real dialog must keep both
                # UI commands and lifecycle timers responsive throughout.
                # Leave it idle first: polling UI commands must not mask an
                # otherwise stalled lifecycle timer in the modal message loop.
                time.sleep(0 if args.physical_input else (21 if label=='GIF' else 1))
                deadline=time.monotonic()+(0.5 if args.physical_input else 2)
                while time.monotonic()<deadline:
                    if rpc('snapshot')['processId']!=child.pid:raise AssertionError('Destination dialog replaced capture worker')
                    if rpc('jobs.get',{'id':job['id']})['state']!='running':raise AssertionError('Destination dialog ended capture')
                    time.sleep(.25)
                events.append({'recordingDestinationRemainedResponsive':label})
                user.PostMessageW(dialog,0x111,2,0)
                wait_until(lambda:not file_dialog())
                wait_until(lambda:user.IsWindowEnabled(canvas))
                state=rpc('probe.state')
                if not overlay() or state['recordingMode'] or state['recording'] is not None:raise AssertionError('Cancelling recording destination started or discarded capture')
                events.append({'recordingDestinationCancelRetainedScreenshot':label})
                click_button(canvas,label)
                dialog=wait_until(lambda:file_dialog('保存录制'))
                destination=output/'saved'/('chosen.gif' if label=='GIF' else 'chosen.mp4')
                choose_recording_file(dialog,destination)
                state=wait_until(lambda:(s if (s:=rpc('probe.state'))['recordingMode'] else None))
                if Path(state['recordingDestination'])!=destination or destination.exists():raise AssertionError('Choosing a destination failed or started recording')
                events.append({'recordingDestinationAccepted':label})
                if not args.physical_input:
                    click_button(canvas,'保存地址')
                    dialog=wait_until(lambda:file_dialog('保存录制'))
                    user.PostMessageW(dialog,0x111,2,0)
                    wait_until(lambda:not file_dialog())
                    state=rpc('probe.state')
                    if Path(state['recordingDestination'])!=destination or not state['recordingMode']:
                        raise AssertionError('Cancelling destination change lost the confirmed recording path')
                    events.append({'cancelledDestinationChangeRetainedPath':label})
                key(canvas,0x1B)
                wait_until(lambda:rpc('jobs.get',{'id':job['id']})['state']=='completed')
                job=rpc('capture.unified',{'mode':'region','language':'zh-CN'});canvas=wait_until(overlay)
                send(canvas,0x201,1,(180<<16)|160);send(canvas,0x200,1,(460<<16)|620);send(canvas,0x202,0,(460<<16)|620)
        if args.suite in ('annotations','resume'):
            key(canvas,ord('N'))
            send(canvas,0x201,1,(260<<16)|260)
            send(canvas,0x202,0,(260<<16)|260)
            edit=wait_until(lambda:number_editor(canvas))
            events.append({'numberEditor':int(edit)})
            for character in '测试说明 Native caption':
                send(edit,0x102,ord(character),0)
            events.append({'numberTextEntered':True})
            send(edit,0x8,canvas,0)
            metadata=output/'native-capture-cpp/current.json'
            def saved_caption():
                try:
                    value=json.loads(metadata.read_text('utf-8'))
                    return value if any(e.get('text')=='测试说明 Native caption' for e in value.get('elements',[])) else None
                except (OSError,json.JSONDecodeError):return None
            saved=wait_until(saved_caption)
            events.append({'numberCaptionSaved':True,'draftId':saved['id']})
            if args.suite=='annotations':
                key(canvas,ord('S'),control=True)
                finished=wait_until(lambda:(x if (x:=rpc('jobs.get',{'id':job['id'],'waitMs':100}))['state']!='running' else None))
                if finished['state']!='completed':raise AssertionError(f'Annotation save failed: {finished}')
                destination=Path(finished['result']['path'])
                if not destination.is_file():raise AssertionError('Native image was not saved')
                from PIL import Image
                with Image.open(destination) as image:
                    if image.size!=(460,280):raise AssertionError(f'Selected pixels changed: {image.size}')
                events.append({'annotationSaved':True,'path':str(destination)})
                job=rpc('capture.unified',{'mode':'region','language':'zh-CN'})
                canvas=wait_until(overlay)
            else:
                # Crash only this owned candidate after its metadata is committed.
                child.kill();child.wait(timeout=5)
                child=subprocess.Popen([args.executable,'--pipe',pipe,'--data-dir',str(output),'--parent',str(os.getpid())],
                    stdout=stdout,stderr=stderr,creationflags=0x08000000)
                wait_until(ready)
                recovered=rpc('draft.get')['draft']
                if recovered['id']!=saved['id']:raise AssertionError('Saved draft was not offered after restart')
                job=rpc('draft.resume',{'expectedId':saved['id'],'language':'zh-CN'})
                canvas=wait_until(overlay)
                events.append({'draftResumed':True})
        send(canvas,0x100,0x1B,0)
        result=wait_until(lambda: (x if (x:=rpc('jobs.get',{'id':job['id'],'waitMs':100}))['state']!='running' else None))
        if result['state']!='completed' or not result['result'].get('cancelled'):
            raise AssertionError(f'Cancel did not complete: {result}')
        if overlay():raise AssertionError('Capture window remained after cancel')
        events.append({'cancel':result})
        rpc('shutdown')
        child.wait(timeout=15)
        if child.returncode:raise AssertionError('Native worker failed at shutdown')
        events.append({'exitCode':child.returncode})
    (output/'result.json').write_text(json.dumps({'passed':True,'desktop':desktop_name.value,'events':events},ensure_ascii=False,indent=2),'utf-8')
except Exception as error:
    import traceback
    events.append({'traceback':traceback.format_exc()})
    if child and child.poll() is None:
        try:events.append({'diagnosticState':rpc('probe.state')})
        except Exception:pass
    if host_fixture:events.append({'hostRequests':host_fixture.requests,'hostError':host_fixture.error})
    (output/'result.json').write_text(json.dumps({'passed':False,'error':str(error),'events':events},ensure_ascii=False,indent=2),'utf-8')
    raise
finally:
    if child and child.poll() is None:child.kill();child.wait()
    if host_fixture:host_fixture.stopping=True
    if owner_process and owner_process.poll() is None:owner_process.terminate();owner_process.wait(timeout=5)
    user.DestroyWindow(fixture)
