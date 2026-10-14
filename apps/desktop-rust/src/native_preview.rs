//! Read-only native page previews. The real controls paint their own HWND;
//! synthetic replies stand in for Host I/O and never execute product actions.
use super::*;
use std::{fs,io::Write,path::Path};
use windows_sys::Win32::UI::Input::KeyboardAndMouse::SetFocus;
use windows_sys::Win32::Graphics::Dwm::{DwmGetWindowAttribute,DWMWA_EXTENDED_FRAME_BOUNDS};

#[test]
#[ignore="explicit native layout preview; requires WINONE_RUST_PREVIEW_DIRECTORY"]
fn render_native_pages(){
    let directory=std::env::var("WINONE_RUST_PREVIEW_DIRECTORY").expect("preview output directory");
    let directory=Path::new(&directory);assert!(directory.is_dir());
    let filter=std::env::var("WINONE_RUST_PREVIEW_FILTER").unwrap_or_default();
    unsafe{SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);}
    let _desktop=Desktop::create();
    let cases=[("overview",0,0),("capture",0,0),("capture",1,0),("capture",2,0),("capture",3,0),
        ("always-on-top",0,0),("always-on-top",1,0),("always-on-top",2,0),
        ("wintray",0,0),("wintray",0,1),("wintray",1,0),("wintray",2,0),
        ("settings",0,0),("settings",1,0),("settings",2,0),("settings",3,0),("settings",4,0)];
    let mut records=Vec::new();
    for (language,theme,width) in [("zh-CN","light",1100),("en-US","dark",1100),("en-US","light",760),("zh-CN","dark",760)]{
        for (page,tab,program_tab) in cases{
            let name=format!("{page}-{tab}-{program_tab}-{language}-{theme}-{width}");
            if !filter.is_empty()&&!name.contains(&filter){continue;}
            let fixture=Fixture::new();
            let snapshot=json!({"locale":language,"administrator":false,"settings":{"schemaVersion":1,"revision":1,"theme":theme,"language":language,
                "captureShortcut":"Ctrl+Alt+C","pinShortcut":"Ctrl+Alt+P","startup":{"runAtLogon":false,"startMinimized":true,"preferAdministrator":false}},
                "modules":[{"id":"capture","state":"ready"},{"id":"always-on-top","state":"ready"},{"id":"wintray","state":"ready"}]});
            {
                let mut app=fixture.app.borrow_mut();app.snapshot=snapshot.clone();
                app.pages=build_navigation(&app.catalog,language).unwrap();app.navigation=Navigation::new(page);app.tabs.insert(page.into(),tab);app.program_tab=program_tab;
                app.controls.clear();app.palette=Some(Palette::new(app.window,theme));app.open_page().unwrap();
            }
            settle(&fixture,&snapshot);
            let window=fixture.app.borrow().window;let scale=fixture.app.borrow().palette.as_ref().unwrap().scale;
            unsafe{SetWindowPos(window,null_mut(),10,10,(width as f32*scale)as i32,(860.0*scale)as i32,SWP_NOZORDER|SWP_NOACTIVATE);ShowWindow(window,SW_SHOWNOACTIVATE);SetFocus(window);}
            if unsafe{IsWindowVisible(window)}==0{unsafe{ShowWindow(window,SW_SHOWNOACTIVATE);}}
            drain_messages();
            save_window(window,&directory.join(format!("{name}.bmp")));
            let mut controls=Vec::new();
            for control in &fixture.app.borrow().controls{
                let mut rectangle=RECT::default();unsafe{GetWindowRect(control.window,&mut rectangle);}
                controls.push(json!({"id":control.id,"text":widgets::text(control.window),"left":rectangle.left,"top":rectangle.top,"right":rectangle.right,"bottom":rectangle.bottom}));
            }
            records.push(json!({"name":name,"page":page,"tab":tab,"language":language,"theme":theme,"width":width,"scale":scale,"controls":controls}));
            if matches!((page,tab),("capture",0)|("wintray",0)){
                {let mut app=fixture.app.borrow_mut();if app.program_viewport.is_some(){app.program_scroll=i32::MAX;}else{app.scroll=app.content_height;}app.layout();}
                drain_messages();save_window(window,&directory.join(format!("{name}-bottom.bmp")));
            }
        }
    }
    assert!(!records.is_empty(),"no preview cases matched the filter");
    fs::write(directory.join("pages.json"),serde_json::to_vec_pretty(&records).unwrap()).unwrap();
    println!("Previewed {} page variants",records.len());
}

fn settle(fixture:&Fixture,snapshot:&Value){
    for _ in 0..32{
        let Ok(task)=fixture.requests.try_recv()else{return;};
        let result=match &task{
            Task::Load{page,..}=>match page.as_str(){
                "settings"=>snapshot.clone(),
                "capture"=>json!({"schemaVersion":1,"revision":1,"copyToClipboard":true,"askWhereToSave":true,"openEditor":true,"pinAfterCapture":false,
                    "outputDirectory":r"C:\Pictures\WinOne","fileNameTemplate":"WinOne-{timestamp}-OK","scrollDelay":300,"scrollFromTop":false}),
                "always-on-top"=>json!({"schemaVersion":1,"revision":1,"frame":true,"useAccent":false,"frameColor":"#5276b8","frameThickness":3,"frameOpacity":100,"roundCorners":true,"sound":false,"blockInGameMode":true,"excludedApps":["game.exe","presentation.exe"]}),
                "wintray"=>json!({"revision":"7","settings":{"startupIntervalSeconds":3,"closeWindowRetrySeconds":15,"exitAfterManagedAppsCompleted":false,
                    "managedApps":[program("editor","项目文档编辑器 / Project document editor","Editor.exe"),program("sync","文件同步工具 / File synchronization","Sync.exe"),program("notes","笔记 / Notes","Notes.exe")]}}),
                _=>panic!("unexpected preview page"),
            },
            Task::ShortcutSnapshot=>snapshot.clone(),
            Task::ShortcutStatus=>json!({"bindings":[{"id":"capture","shortcut":"Ctrl+Alt+C","registered":true},{"id":"pin","shortcut":"Ctrl+Alt+P","registered":true}],"lastError":"","actionErrors":{}}),
            Task::ProgramIcon{path}=>json!({"path":path,"icon":crate::icon::base64_encode(&red_icon())}),
            Task::Logs{..}=>json!({"folderAvailable":true,"entries":[{"name":"process","available":true,"truncated":false,"text":"Configuration loaded.\r\nReady.\r\n配置已载入，等待操作。"}]}),
            Task::UpdateInfo{..}=>json!({"version":"0.0.1","configured":false}),
            Task::CaptureFileName{..}=>json!({"name":"WinOne-20261005090000-OK.png"}),
            Task::ModulesSnapshot{..}=>json!({"revision":1,"modules":[{"manifest":{"id":"capture"},"enabled":true,"runtime":{"id":"capture","state":"ready"}},
                {"manifest":{"id":"always-on-top"},"enabled":true,"runtime":{"id":"always-on-top","state":"ready"}},{"manifest":{"id":"wintray"},"enabled":true,"runtime":{"id":"wintray","state":"ready"}}]}),
            _=>panic!("preview attempted a product action"),
        };
        fixture.replies.send(worker::Reply{task,result:Ok(result)}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    }
    panic!("preview did not settle");
}
fn program(id:&str,name:&str,file:&str)->Value{
    json!({"id":id,"name":name,"exePath":format!("C:\\Apps\\{file}"),"args":"--workspace documents","runOnStartup":true,"collectTrayIcon":true,
        "launchViaLogonTask":false,"launchHiddenInBackground":false,"trayBehavior":{"autoMinimizeAndHideOnLaunch":true,"closeDelaySeconds":3},
        "schedule":{"enabled":true,"startDelayMinutes":5,"autoExitMinutes":30,"frequencyEnabled":true,"frequencyDays":7,"frequencyRuns":2}})
}
fn drain_messages(){
    let mut message=MSG::default();
    while unsafe{PeekMessageW(&mut message,null_mut(),0,0,PM_REMOVE)}!=0{
        if message.message!=WM_QUIT{unsafe{TranslateMessage(&message);DispatchMessageW(&message);}}
    }
}

fn save_window(window:HWND,path:&Path){
    let mut bounds=RECT::default();assert_ne!(unsafe{GetWindowRect(window,&mut bounds)},0);
    let (width,height)=(bounds.right-bounds.left,bounds.bottom-bounds.top);
    assert!(width>0&&height>0&&width<=4000&&height<=4000);
    unsafe{
        let source=GetDC(window);let dc=CreateCompatibleDC(source);let mut pixels=null_mut();
        let info=BITMAPINFO{bmiHeader:BITMAPINFOHEADER{biSize:40,biWidth:width,biHeight:-height,biPlanes:1,biBitCount:32,biCompression:BI_RGB,..Default::default()},..Default::default()};
        let bitmap=CreateDIBSection(source,&info,DIB_RGB_COLORS,&mut pixels,null_mut(),0);ReleaseDC(window,source);
        assert!(!dc.is_null()&&!bitmap.is_null()&&!pixels.is_null());
        let previous=SelectObject(dc,bitmap);
        // Request current native control painting directly. PrintWindow's full
        // content path can return an unpainted client on a non-input desktop;
        // that compositor result is unsuitable for deterministic layout checks.
        SendMessageW(window,WM_PRINT,dc as usize,(PRF_NONCLIENT|PRF_CLIENT|PRF_ERASEBKGND|PRF_CHILDREN)as isize);
        // DIB memory cannot be read until the calling thread's batched GDI
        // drawing has completed. Returning from WM_PRINT is not that fence.
        GdiFlush();
        let data=std::slice::from_raw_parts(pixels as *const u8,(width*height*4)as usize).to_vec();
        SelectObject(dc,previous);DeleteObject(bitmap);DeleteDC(dc);
        assert!(data.chunks_exact(4).any(|pixel|pixel[..3].iter().any(|v|*v>16)),"window did not paint");
        let mut origin=POINT{x:4,y:4};ClientToScreen(window,&mut origin);
        let (x,y)=((origin.x-bounds.left)as usize,(origin.y-bounds.top)as usize);
        assert!(data[(y*width as usize+x)*4..][..3].iter().any(|v|*v>16),"window client did not paint; a caption alone is not a valid preview");
        // Trim only uniform black invisible resize borders. Keep caption and
        // client pixels; these images are layout evidence, not DWM timing.
        let black=|x:usize,y:usize|data[(y*width as usize+x)*4..][..3].iter().all(|v|*v<4);
        let (mut left,mut top,mut right,mut bottom)=(0,0,width as usize,height as usize);
        let mut frame=RECT::default();
        if DwmGetWindowAttribute(window,DWMWA_EXTENDED_FRAME_BOUNDS as u32,(&mut frame as *mut RECT).cast(),std::mem::size_of::<RECT>()as u32)>=0
            &&frame.left>=bounds.left&&frame.top>=bounds.top&&frame.right<=bounds.right&&frame.bottom<=bounds.bottom&&frame.right>frame.left&&frame.bottom>frame.top{
            left=(frame.left-bounds.left)as usize;top=(frame.top-bounds.top)as usize;right=(frame.right-bounds.left)as usize;bottom=(frame.bottom-bounds.top)as usize;
        }
        while left<16&&(0..height as usize).all(|y|black(left,y)){left+=1;}
        while right>width as usize-16&&(0..height as usize).all(|y|black(right-1,y)){right-=1;}
        while top<16&&(left..right).all(|x|black(x,top)){top+=1;}
        while bottom>height as usize-16&&(left..right).all(|x|black(x,bottom-1)){bottom-=1;}
        let (w,h)=((right-left)as i32,(bottom-top)as i32);let mut file=fs::File::create(path).unwrap();
        file.write_all(b"BM").unwrap();file.write_all(&(54+w*h*4).to_le_bytes()).unwrap();file.write_all(&[0;4]).unwrap();file.write_all(&54u32.to_le_bytes()).unwrap();
        for value in [40,w,-h]{file.write_all(&value.to_le_bytes()).unwrap();}
        file.write_all(&1u16.to_le_bytes()).unwrap();file.write_all(&32u16.to_le_bytes()).unwrap();file.write_all(&[0;24]).unwrap();
        for y in top..bottom{
            let mut row=data[(y*width as usize+left)*4..(y*width as usize+right)*4].to_vec();for pixel in row.chunks_exact_mut(4){pixel[3]=255;}
            file.write_all(&row).unwrap();
        }
    }
}
