//! Opt-in native-control checks on a non-input desktop. Never switch desktops or
//! inject global input; these checks do not certify visible user experience.
use super::*;
use std::time::Instant;
use windows_sys::Win32::{System::{StationsAndDesktops::*, Threading::*}, UI::Input::KeyboardAndMouse::{IsWindowEnabled,GetFocus,SetFocus, VK_CONTROL, VK_DELETE, VK_ESCAPE, VK_F12}};
#[path="native_preview.rs"]
mod preview;

struct Desktop { original: HDESK, isolated: HDESK }

#[test]
#[ignore = "creates controls on a private desktop"]
fn program_details_scroll_independently_and_reflow_without_recreating_fields(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.load_programs();
    {let mut app=fixture.app.borrow_mut();let mut items=app.draft.as_ref().unwrap().value()["managedApps"].clone();
        items[0]["schedule"]["enabled"]=json!(true);items[0]["schedule"]["frequencyEnabled"]=json!(true);
        app.draft.as_mut().unwrap().set("managedApps",items).unwrap();}
    let list_window=fixture.control(450);fixture.click(351);
    assert_eq!(fixture.control(450),list_window,"changing detail tabs recreated the program list");
    let window=fixture.app.borrow().window;
    let field_id=fixture.app.borrow().bindings.iter().find(|b|b.key=="name").unwrap().id;
    let field=fixture.control(field_id);let pane=unsafe{GetParent(field)};
    assert_ne!(pane,window,"details need their own clipped viewport");
    let mut list_before=RECT::default();let mut field_before=RECT::default();
    unsafe{GetWindowRect(fixture.control(450),&mut list_before);GetWindowRect(field,&mut field_before);SendMessageW(pane,WM_VSCROLL,SB_LINEDOWN as usize,0);}
    let mut list_after=RECT::default();let mut field_after=RECT::default();
    unsafe{GetWindowRect(fixture.control(450),&mut list_after);GetWindowRect(field,&mut field_after);}
    assert_eq!((list_before.left,list_before.top,list_before.bottom),(list_after.left,list_after.top,list_after.bottom));
    assert!(field_after.top<field_before.top,"detail scrolling did not move its content");
    unsafe{SetWindowPos(window,null_mut(),0,0,760,860,SWP_NOZORDER|SWP_NOACTIVATE);}
    assert_eq!(fixture.control(field_id),field,"responsive layout recreated the active input");
    let mut detail=RECT::default();let mut list=RECT::default();let mut launch=RECT::default();
    unsafe{GetWindowRect(pane,&mut detail);GetWindowRect(fixture.control(450),&mut list);GetWindowRect(fixture.control(357),&mut launch);}
    assert!(detail.top>list.bottom,"narrow layout must stack the two task areas");
    assert!(launch.bottom<=detail.top,"list actions must remain above details in the narrow layout");
    let last_id=fixture.app.borrow().bindings.iter().find(|b|b.key=="schedule.frequencyRuns").unwrap().id;
    let last=fixture.control(last_id);unsafe{SetFocus(last);}
    let mut last_rect=RECT::default();unsafe{GetWindowRect(last,&mut last_rect);}
    assert!(last_rect.top>=detail.top&&last_rect.bottom<=detail.bottom,"keyboard focus remained outside the viewport");
    assert_eq!(unsafe{GetFocus()},last);
    assert_eq!(fixture.control(450),list_window);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn dark_combobox_uses_the_same_dark_surface(){
    let _desktop=Desktop::create();let fixture=Fixture::new();let window=fixture.app.borrow().window;
    {let mut app=fixture.app.borrow_mut();app.snapshot["settings"]["theme"]=json!("dark");app.controls.clear();app.palette=Some(Palette::new(window,"dark"));app.render().unwrap();}
    let id=fixture.app.borrow().bindings.iter().find(|b|b.key=="theme").unwrap().id;
    let field=fixture.control(id);
    let color=unsafe{
        let source=GetDC(window);let dc=CreateCompatibleDC(source);let bitmap=CreateCompatibleBitmap(source,280,40);ReleaseDC(window,source);let old=SelectObject(dc,bitmap);
        SendMessageW(field,WM_PRINTCLIENT,dc as usize,PRF_CLIENT as isize);let color=GetPixel(dc,180,12);
        SelectObject(dc,old);DeleteObject(bitmap);DeleteDC(dc);color
    };
    assert!(color&255<80&&(color>>8)&255<80&&(color>>16)&255<80,"dark combo has a bright surface: {color:#x}");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn late_program_icon_does_not_recreate_the_active_text_input(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.load_programs();fixture.click(351);
    let id=fixture.app.borrow().bindings.iter().find(|b|b.key=="name").unwrap().id;
    let input=fixture.control(id);widgets::set_text(input,"Edited program name");
    unsafe{SetFocus(input);SendMessageW(input,EM_SETSEL,3,7);}
    fixture.replies.send(worker::Reply{task:Task::ProgramIcon{path:"app.exe".into()},result:Ok(json!({"icon":crate::icon::base64_encode(&red_icon())}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.control(id),input,"icon reply recreated an editing HWND");
    assert_eq!(unsafe{GetFocus()},input);
    assert_eq!(unsafe{SendMessageW(input,EM_GETSEL,0,0)},3|(7<<16));
    assert_eq!(widgets::text(input),"Edited program name");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn shortcut_header_and_body_do_not_overlap_at_narrow_width(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.open_capture_shortcuts();
    let window=fixture.app.borrow().window;
    let input_id=fixture.app.borrow().bindings.iter().find(|b|b.key=="outputDirectory").unwrap().id;
    let input=fixture.control(input_id);widgets::set_text(input,"C:\\Working");unsafe{SetFocus(input);SendMessageW(input,EM_SETSEL,3,7);}
    for width in [1100,760]{
        unsafe{SetWindowPos(window,null_mut(),0,0,width,800,SWP_NOZORDER|SWP_NOACTIVATE);}
        assert_eq!(fixture.control(input_id),input,"resizing must not recreate an editing control");
        let mut hint=RECT::default();let mut tab=RECT::default();let mut save=RECT::default();let mut frame=RECT::default();
        unsafe{GetWindowRect(fixture.control(shortcut_view::SHORTCUT_HINT),&mut hint);GetWindowRect(fixture.control(TAB),&mut tab);GetWindowRect(fixture.control(shortcut_view::SHORTCUT_SAVE),&mut save);GetWindowRect(window,&mut frame);}
        assert!(hint.bottom<=tab.top,"shortcut hint overlaps module tabs at {width}");
        assert!(save.right<=frame.right-16,"shortcut action extends beyond the viewport at {width}");
        let first=fixture.app.borrow().bindings.iter().find(|b|b.key=="copyToClipboard").unwrap().id;
        let mut field=RECT::default();unsafe{GetWindowRect(fixture.control(first),&mut field);}
        assert!(field.top>=tab.bottom,"form overlaps tabs at {width}");
    }
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn dark_checkbox_text_remains_visible_and_native_toggle_semantics_are_kept(){
    let _desktop=Desktop::create();let fixture=Fixture::new();let window=fixture.app.borrow().window;
    {let mut app=fixture.app.borrow_mut();app.snapshot["settings"]["theme"]=json!("dark");app.controls.clear();app.palette=Some(Palette::new(window,"dark"));app.tabs.insert("settings".into(),1);app.render().unwrap();}
    let id=fixture.app.borrow().bindings.iter().find(|b|b.key=="startup.runAtLogon").unwrap().id;let field=fixture.control(id);
    let bright=unsafe{
        let source=GetDC(window);let dc=CreateCompatibleDC(source);let bitmap=CreateCompatibleBitmap(source,700,40);ReleaseDC(window,source);let old=SelectObject(dc,bitmap);
        let brush=CreateSolidBrush(0x181818);FillRect(dc,&RECT{left:0,top:0,right:700,bottom:40},brush);DeleteObject(brush);
        SendMessageW(field,WM_PRINTCLIENT,dc as usize,(PRF_CLIENT|PRF_ERASEBKGND)as isize);
        let mut count=0;for y in 0..34{for x in 24..300{let color=GetPixel(dc,x,y);if color&255>180&&(color>>8)&255>180&&(color>>16)&255>180{count+=1;}}}
        SelectObject(dc,old);DeleteObject(bitmap);DeleteDC(dc);count
    };
    assert!(bright>20,"dark checkbox label was not painted in a readable foreground: {bright}");
    assert_eq!(unsafe{GetWindowLongW(field,GWL_STYLE)} as u32&BS_TYPEMASK as u32,BS_AUTOCHECKBOX as u32);
    unsafe{SendMessageW(field,BM_CLICK,0,0);}
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["startup"]["runAtLogon"],true);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn system_printing_keeps_owner_paint_callbacks_available(){
    let _desktop=Desktop::create();let fixture=Fixture::new();
    let window=fixture.app.borrow().window;
    {let mut app=fixture.app.borrow_mut();app.snapshot["settings"]["theme"]=json!("dark");app.controls.clear();app.palette=Some(Palette::new(window,"dark"));app.render().unwrap();}
    let (background,sidebar)=unsafe{
        let source=GetDC(window);let dc=CreateCompatibleDC(source);let bitmap=CreateCompatibleBitmap(source,1100,800);ReleaseDC(window,source);
        let old=SelectObject(dc,bitmap);let fill=CreateSolidBrush(0xff00ff);let area=RECT{left:0,top:0,right:1100,bottom:800};FillRect(dc,&area,fill);DeleteObject(fill);
        SendMessageW(window,WM_PRINT,dc as usize,(PRF_CLIENT|PRF_CHILDREN|PRF_ERASEBKGND)as isize);
        let values=(GetPixel(dc,900,20),GetPixel(dc,16,360));
        SelectObject(dc,old);DeleteObject(bitmap);DeleteDC(dc);values
    };
    let app=fixture.app.borrow();let palette=app.palette.as_ref().unwrap();
    assert_eq!(background,palette.background,"default printing suppressed the parent background callback");
    assert_eq!(sidebar,palette.sidebar,"default printing suppressed the sidebar callback");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn update_success_must_match_the_displayed_build_version(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.click(TAB+4);
    let info=fixture.requests.try_recv().unwrap();fixture.replies.send(worker::Reply{task:info,result:Ok(json!({"version":"1.0.0","configured":true}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.click(410);let task=fixture.requests.try_recv().unwrap();let Task::UpdateCheck{id}=task.clone()else{panic!("expected check")};
    fixture.replies.send(worker::Reply{task,result:Ok(json!({"id":id,"state":"up_to_date","currentVersion":"2.0.0","latestVersion":"2.0.0"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert!(!fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window)=="当前已是最新版本。"),"a response for another build cannot prove this build is current");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn update_release_is_requested_by_identity_and_invalid_links_never_open(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.click(TAB+4);
    let info=fixture.requests.try_recv().unwrap();fixture.replies.send(worker::Reply{task:info,result:Ok(json!({"version":"1.0.0","configured":true}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.click(410);let start=fixture.requests.try_recv().unwrap();let Task::UpdateCheck{id}=start.clone()else{panic!("expected check")};
    fixture.replies.send(worker::Reply{task:start,result:Ok(json!({"id":id,"state":"available","currentVersion":"1.0.0","latestVersion":"1.1.0","releaseUrl":"https://github.com/example/app/releases/tag/v1.1.0"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.click(412);let release=fixture.requests.try_recv().unwrap();assert!(matches!(&release,Task::UpdateRelease{id:requested} if *requested==id));
    fixture.replies.send(worker::Reply{task:release,result:Ok(json!({"url":"file:///C:/untrusted.exe"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("无法打开发布页")));
    fixture.click(410);let next=fixture.requests.try_recv().unwrap();let Task::UpdateCheck{id:next_id}=next.clone()else{panic!("expected new check")};
    fixture.replies.send(worker::Reply{task:next,result:Ok(json!({"id":next_id,"state":"up_to_date","currentVersion":"1.0.0","latestVersion":"1.0.0"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert!(!fixture.app.borrow().controls.iter().any(|c|c.id==412));
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window)=="当前已是最新版本。"));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn update_cancellation_wins_over_late_results_and_polling_stays_bounded(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.click(TAB+4);
    let info=fixture.requests.try_recv().unwrap();fixture.replies.send(worker::Reply{task:info,result:Ok(json!({"version":"1.0.0","configured":true}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.click(410);let start=fixture.requests.try_recv().unwrap();
    let Task::UpdateCheck{id}=start.clone()else{panic!("expected check")};
    let cancel_control=fixture.control(411);
    fixture.click(410);assert!(fixture.requests.try_recv().is_err());
    fixture.replies.send(worker::Reply{task:start,result:Ok(json!({"id":id,"state":"checking"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.control(411),cancel_control,"polling must keep the existing cancel control and focus");
    let window=fixture.app.borrow().window;
    unsafe{SendMessageW(window,WM_TIMER,updates_view::UPDATE_TIMER,0);SendMessageW(window,WM_TIMER,updates_view::UPDATE_TIMER,0);}
    let poll=fixture.requests.try_recv().unwrap();assert!(fixture.requests.try_recv().is_err(),"only one status read may be pending");
    fixture.click(411);let cancel=fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply{task:cancel,result:Ok(json!({"id":id,"state":"failed","errorCode":"cancelled"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.replies.send(worker::Reply{task:poll,result:Ok(json!({"id":id,"state":"available","currentVersion":"1.0.0","latestVersion":"9.0.0","releaseUrl":"https://github.com/example/app/releases/tag/v9.0.0"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert!(!fixture.app.borrow().controls.iter().any(|c|c.id==412),"a late result must not restore a cancelled link");
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window)=="已取消检查"));
    unsafe{SendMessageW(window,WM_TIMER,updates_view::UPDATE_TIMER,0);}
    assert!(fixture.requests.try_recv().is_err(),"terminal checks must stop polling");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn leaving_updates_cancels_its_request_and_keeps_late_results_off_other_pages(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.click(TAB+4);
    let info=fixture.requests.try_recv().unwrap();fixture.replies.send(worker::Reply{task:info,result:Ok(json!({"version":"1.0.0","configured":true}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.click(410);let start=fixture.requests.try_recv().unwrap();let Task::UpdateCheck{id}=start.clone()else{panic!("expected check")};
    fixture.click(TAB);
    let cancel=fixture.requests.try_recv().unwrap();assert!(matches!(&cancel,Task::UpdateCancel{id:cancelled} if *cancelled==id));
    fixture.replies.send(worker::Reply{task:start,result:Ok(json!({"id":id,"state":"available","currentVersion":"1.0.0","latestVersion":"2.0.0","releaseUrl":"https://github.com/example/app/releases/tag/v2.0.0"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().tab(),0);assert!(!fixture.app.borrow().controls.iter().any(|c|c.id==412));
    fixture.replies.send(worker::Reply{task:cancel,result:Ok(json!({"id":id,"state":"failed","errorCode":"cancelled"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    fixture.click(TAB+4);fixture.click(410);let next=fixture.requests.try_recv().unwrap();let Task::UpdateCheck{id:next_id}=next.clone()else{panic!("expected next check")};assert_ne!(next_id,id);
    fixture.replies.send(worker::Reply{task:next,result:Ok(json!({"id":next_id,"state":"failed","errorCode":"network"}))}).unwrap();fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("检查网络")));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn update_page_reads_build_info_without_starting_a_network_check(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.click(TAB+4);
    let info=fixture.requests.try_recv().expect("update page must read build metadata");
    fixture.replies.send(worker::Reply{task:info,result:Ok(json!({"version":"0.0.1","configured":false}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let check=fixture.app.borrow().controls.iter().find(|c|widgets::text(c.window)=="检查更新").expect("missing update command").window;
    assert_eq!(unsafe{IsWindowEnabled(check)},0);
    assert!(fixture.requests.try_recv().is_err(),"opening updates must not trigger network access");
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("未配置更新来源")));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn permission_restart_guard_preserves_edits_and_only_closes_a_clean_window(){
    let _desktop=Desktop::create();
    for dirty in [true,false]{
        let fixture=Fixture::new();fixture.app.borrow_mut().snapshot["administrator"]=json!(false);fixture.click(TAB+1);
        let restart=fixture.app.borrow().controls.iter().find(|c|widgets::text(c.window)=="以管理员身份重新启动").unwrap().id;
        fixture.click(restart);let task=fixture.requests.try_recv().unwrap();
        let window=fixture.app.borrow().window;
        assert_eq!(unsafe{SendMessageW(window,crate::permissions::GUARD_MESSAGE,0,0)},1);
        {let _dispatch=fixture.app.borrow_mut();assert_eq!(unsafe{SendMessageW(window,crate::permissions::GUARD_MESSAGE,0,0)},2);}
        if dirty{fixture.app.borrow_mut().draft.as_mut().unwrap().set("theme",json!("dark")).unwrap();}
        assert_eq!(unsafe{SendMessageW(window,crate::permissions::GUARD_MESSAGE,0,0)},if dirty{0}else{1});
        fixture.replies.send(worker::Reply{task,result:Ok(json!({"closing":true,"token":"a".repeat(32)}))}).unwrap();
        fixture.app.borrow_mut().replies().unwrap();
        assert_eq!(unsafe{IsWindow(window)}!=0,dirty);
        if dirty{assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["theme"],"dark");assert!(fixture.requests.try_recv().is_err(),"reconciliation must not erase unanticipated edits");}
    }
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn permission_changes_wait_for_confirmation_and_reload_after_failure(){
    let _desktop=Desktop::create();let fixture=Fixture::new();
    {let mut app=fixture.app.borrow_mut();app.snapshot["administrator"]=json!(false);app.snapshot["settings"]["startup"]=json!({"preferAdministrator":false});app.draft=Some(Draft::new(app.snapshot["settings"].clone()).unwrap());}
    fixture.click(TAB+1);
    let remember=fixture.app.borrow().controls.iter().find(|c|widgets::text(c.window)=="始终以管理员身份运行").expect("missing permission control").id;
    unsafe{SendMessageW(fixture.control(remember),BM_SETCHECK,BST_CHECKED as usize,0);}
    fixture.click(remember);
    let task=fixture.requests.try_recv().expect("permission change was not submitted");
    assert_eq!(fixture.app.borrow().snapshot["settings"]["startup"]["preferAdministrator"],false);
    let window=fixture.app.borrow().window;
    unsafe{SendMessageW(window,WM_CLOSE,0,0);}
    assert_ne!(unsafe{IsWindow(window)},0,"pending consent must retain the settings window");
    fixture.replies.send(worker::Reply{task,result:Err(winone_desktop::protocol::RpcError::new("cancelled","Consent cancelled"))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let reload=fixture.requests.try_recv().expect("permission failure must reconcile current settings");
    assert!(matches!(&reload,Task::Load{page,..} if page=="settings"));
    fixture.replies.send(worker::Reply{task:reload,result:Ok(json!({"administrator":false,"locale":"zh-CN","settings":{"revision":1,"theme":"light","language":"zh-CN","startup":{"preferAdministrator":false}}}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("已取消")));
    assert_eq!(unsafe{SendMessageW(fixture.control(remember),BM_GETCHECK,0,0)},BST_UNCHECKED as isize);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn quick_capture_disables_the_unused_save_picker_without_losing_the_preference(){
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.open_capture_shortcuts();
    {let mut app=fixture.app.borrow_mut();let draft=app.draft.as_mut().unwrap();draft.set("openEditor",json!(true)).unwrap();draft.set("askWhereToSave",json!(true)).unwrap();app.render().unwrap();}
    let binding=|key:&str|fixture.app.borrow().bindings.iter().find(|b|b.key==key).unwrap().id;
    let picker=binding("askWhereToSave");let mode=binding("openEditor");
    for position in [1,0]{
        let owner=fixture.app.borrow().window;let control=fixture.control(mode);
        unsafe{SendMessageW(control,CB_SETCURSEL,position,0);SendMessageW(owner,WM_COMMAND,mode|((CBN_SELCHANGE as usize)<<16),control as isize);}
        assert_eq!(unsafe{IsWindowEnabled(fixture.control(picker))}!=0,position==0);
        assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["askWhereToSave"],true);
    }
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn filename_preview_coalesces_input_keeps_the_caret_and_retries_failures() {
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.open_capture_shortcuts();fixture.click(TAB+1);
    let first=fixture.requests.try_recv().expect("filename page must request the existing Host preview");
    let field_id=fixture.app.borrow().bindings.iter().find(|b|b.key=="fileNameTemplate").unwrap().id;
    let input=fixture.control(field_id);
    for i in 0..50 {widgets::set_text(input,&format!("Capture-{i}-{{date}}"));}
    assert!(fixture.requests.try_recv().is_err(),"typing while preview is pending must not enqueue every edit");
    unsafe{SendMessageW(input,EM_SETSEL,4,4);}
    fixture.replies.send(worker::Reply{task:first,result:Ok(json!({"name":"old.png"}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let latest=fixture.requests.try_recv().expect("latest input must be previewed after the previous request");
    assert!(!fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("old.png")));
    fixture.replies.send(worker::Reply{task:latest,result:Err(winone_desktop::protocol::RpcError::new("configuration","template error"))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.control(field_id),input,"preview completion must not recreate the text input");
    assert_eq!(unsafe{SendMessageW(input,EM_GETSEL,0,0)},4|(4<<16));
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("template error")));
    let retry=fixture.app.borrow().controls.iter().find(|c|widgets::text(c.window)=="刷新预览").unwrap().id;
    fixture.click(retry);
    let retry=fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply{task:retry,result:Ok(json!({"name":"Capture-49-20261005.png"}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c|widgets::text(c.window).contains("Capture-49-20261005.png")));
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["fileNameTemplate"],"Capture-49-{date}");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn border_color_picker_preserves_cancel_and_uses_the_settings_save_transaction() {
    let _desktop=Desktop::create(); let fixture=Fixture::new();
    fixture.click(NAV+2);
    let task=fixture.requests.try_recv().unwrap();fixture.drain_shortcut_entry();
    fixture.replies.send(worker::Reply{task,result:Ok(json!({"schemaVersion":1,"revision":4,"frame":true,"useAccent":true,"frameColor":"#808080",
        "frameThickness":3,"frameOpacity":100,"roundCorners":true,"sound":false,"blockInGameMode":true,"excludedApps":[]}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let binding=|key:&str|fixture.app.borrow().bindings.iter().find(|b|b.key==key).expect("missing module setting").id;
    let color=binding("frameColor");
    assert_eq!(unsafe{IsWindowEnabled(fixture.control(color))},0,"accent color disables custom selection");
    let accent=binding("useAccent");
    unsafe{SendMessageW(fixture.control(accent),BM_SETCHECK,BST_UNCHECKED as usize,0);}
    fixture.click(accent);
    assert_ne!(unsafe{IsWindowEnabled(fixture.control(color))},0);
    for accept in [false,true] {
        let _picker=ColorPickerDriver::new(accept);
        fixture.click(color);
        let deadline=Instant::now()+std::time::Duration::from_secs(5);
        while fixture.app.borrow().dialog_open{
            assert!(Instant::now()<deadline,"color dialog did not complete");
            let mut message=MSG::default();
            if unsafe{PeekMessageW(&mut message,null_mut(),0,0,PM_REMOVE)}!=0{unsafe{TranslateMessage(&message);DispatchMessageW(&message);}}
            else{std::thread::yield_now();}
        }
        assert!(COLOR_OWNER_PAINTED.with(|value|value.get()),"the native dialog must keep its owner paint handler responsive");
        assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["frameColor"],if accept{"#123456"}else{"#808080"});
        assert!(fixture.requests.try_recv().is_err(),"color selection must remain an unsaved edit");
    }
    fixture.click(SAVE);
    let Task::Save{page,value}=fixture.requests.try_recv().unwrap() else{panic!("expected module save")};
    assert_eq!(page,"always-on-top");assert_eq!(value["frameColor"],"#123456");assert_eq!(value["revision"],4);
}

thread_local!{
    static ACCEPT_COLOR:std::cell::Cell<bool>=const{std::cell::Cell::new(false)};
    static COLOR_OWNER_PAINTED:std::cell::Cell<bool>=const{std::cell::Cell::new(false)};
}
struct ColorPickerDriver(HHOOK);
impl ColorPickerDriver {
    fn new(accept:bool)->Self{
        ACCEPT_COLOR.with(|value|value.set(accept));
        COLOR_OWNER_PAINTED.with(|value|value.set(false));
        let hook=unsafe{SetWindowsHookExW(WH_CBT,Some(Self::hook),null_mut(),GetCurrentThreadId())};
        assert!(!hook.is_null(),"{}",std::io::Error::last_os_error());Self(hook)
    }
    unsafe extern "system" fn hook(code:i32,w:WPARAM,l:LPARAM)->LRESULT{
        use windows_sys::Win32::UI::Controls::Dialogs::{COLOR_RED,COLOR_GREEN,COLOR_BLUE};
        let dialog=w as HWND;
        if code==HCBT_ACTIVATE as i32 && !unsafe{GetDlgItem(dialog,COLOR_RED as i32)}.is_null(){
            let dc=unsafe{CreateCompatibleDC(null_mut())};
            let painted=unsafe{SendMessageW(GetWindow(dialog,GW_OWNER),WM_ERASEBKGND,dc as usize,0)};
            unsafe{DeleteDC(dc);}
            COLOR_OWNER_PAINTED.with(|value|value.set(painted==1));
            // A close request delivered inside the native dialog must not
            // destroy the owner while its settings operation is still active.
            unsafe{SendMessageW(GetWindow(dialog,GW_OWNER),WM_CLOSE,0,0);}
            for (field,value) in [(COLOR_RED,"18"),(COLOR_GREEN,"52"),(COLOR_BLUE,"86")]{
                unsafe{SendDlgItemMessageW(dialog,field as i32,WM_SETTEXT,0,wide(value).as_ptr() as isize);}
            }
            let command=ACCEPT_COLOR.with(|value|if value.get(){IDOK}else{IDCANCEL});
            unsafe{PostMessageW(dialog,WM_COMMAND,command as usize,0);}
        }
        unsafe{CallNextHookEx(null_mut(),code,w,l)}
    }
}
impl Drop for ColorPickerDriver{fn drop(&mut self){assert_ne!(unsafe{UnhookWindowsHookEx(self.0)},0);}}

#[test]
#[ignore = "creates controls on a private desktop"]
fn capture_actions_submit_once_keep_the_page_and_distinguish_cancellation() {
    let _desktop=Desktop::create();
    let fixture=Fixture::new();
    fixture.open_capture_shortcuts();
    fixture.app.borrow_mut().snapshot["modules"]=json!([{"id":"capture","state":"ready"}]);
    fixture.app.borrow_mut().refresh_buttons();
    fixture.click(capture_view::CAPTURE_LAST);
    let task=fixture.requests.try_recv().unwrap();
    assert!(matches!(&task,Task::Capture {mode,language} if mode=="scroll"&&language=="zh-CN"));
    fixture.click(capture_view::CAPTURE_LAST);
    assert!(fixture.requests.try_recv().is_err(),"duplicate capture clicks must not submit another job");
    fixture.app.borrow_mut().navigate(Destination::Close).unwrap();
    assert_ne!(unsafe {IsWindow(fixture.app.borrow().window)},0);
    fixture.replies.send(worker::Reply {task,result:Err(winone_desktop::protocol::RpcError::new("cancelled","cancelled"))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().navigation.page(),"capture");
    assert_eq!(fixture.control_text(STATUS),"已取消截图");
    let snapshot=fixture.requests.try_recv().unwrap();
    assert!(matches!(snapshot,Task::CaptureSnapshot {..}));
    fixture.replies.send(worker::Reply{task:snapshot,result:Ok(json!({"lastCapture":null,"pins":0}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_ne!(unsafe {IsWindowEnabled(fixture.control(capture_view::CAPTURE_LAST))},0);
    fixture.app.borrow_mut().draft.as_mut().unwrap().set("copyToClipboard",json!(false)).unwrap();
    fixture.click(capture_view::CAPTURE_FIRST);
    assert!(fixture.requests.try_recv().is_err(),"unsaved capture preferences must not be silently ignored");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn capture_mode_selection_preserves_the_boolean_settings_contract() {
    let _desktop=Desktop::create(); let fixture=Fixture::new();
    fixture.open_capture_shortcuts();
    let id=fixture.app.borrow().bindings.iter().find(|b|b.key=="openEditor").unwrap().id;
    let field=fixture.control(id); let window=fixture.app.borrow().window;
    unsafe {SendMessageW(field,CB_SETCURSEL,1,0); SendMessageW(window,WM_COMMAND,id|((CBN_SELCHANGE as usize)<<16),field as isize);}
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["openEditor"],false);
    fixture.click(SAVE);
    let Task::Save {value,..}=fixture.requests.try_recv().unwrap() else {panic!("expected capture settings save")};
    assert_eq!(value["openEditor"],false);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn last_capture_actions_bind_the_displayed_image_and_keep_failed_edits() {
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.open_capture_shortcuts();
    fixture.app.borrow_mut().snapshot["modules"]=json!([{"id":"capture","state":"ready"}]);
    fixture.app.borrow_mut().refresh_buttons();
    assert_eq!(unsafe{IsWindowEnabled(fixture.control(capture_view::COPY_LAST))},0);
    fixture.click(capture_view::REFRESH_LAST);let request=fixture.requests.try_recv().unwrap();
    let image=json!({"id":"a".repeat(32),"path":"C:\\Images\\saved.png","width":240,"height":160});
    fixture.replies.send(worker::Reply{task:request,result:Ok(json!({"lastCapture":image,"pins":1}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.app.borrow_mut().draft.as_mut().unwrap().set("copyToClipboard",json!(false)).unwrap();
    fixture.click(capture_view::COPY_LAST);
    let task=fixture.requests.try_recv().unwrap();
    assert!(matches!(&task,Task::CaptureAction{method,params} if method=="copy.last"&&params["expectedId"]=="a".repeat(32)));
    fixture.click(capture_view::COPY_LAST);assert!(fixture.requests.try_recv().is_err());
    fixture.replies.send(worker::Reply{task,result:Err(winone_desktop::protocol::RpcError::new("stale","capture changed"))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().dirty());
    assert!(fixture.control_text(STATUS).contains("capture changed"));
    assert!(matches!(fixture.requests.try_recv().unwrap(),Task::CaptureSnapshot{..}));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn disabled_capture_keeps_local_history_readable_without_enabling_actions() {
    let _desktop=Desktop::create();let fixture=Fixture::new();fixture.open_capture_shortcuts();
    fixture.app.borrow_mut().snapshot["modules"]=json!([{"id":"capture","state":"disabled"}]);
    fixture.app.borrow_mut().refresh_buttons();
    assert_ne!(unsafe{IsWindowEnabled(fixture.control(capture_view::REFRESH_LAST))},0);
    fixture.click(capture_view::REFRESH_LAST);
    let task=fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply{task,result:Ok(json!({"lastCapture":{"id":"a".repeat(32),"path":"C:\\Images\\saved.png","width":20,"height":10}}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.control_text(376),"C:\\Images\\saved.png");
    assert_eq!(unsafe{IsWindowEnabled(fixture.control(capture_view::COPY_LAST))},0);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn singleton_notifications_keep_the_current_draft_and_release_presence() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.load_programs();
    fixture.app.borrow_mut().draft.as_mut().unwrap().set("startupIntervalSeconds",json!(5)).unwrap();
    let pipe = format!(r"\\.\pipe\WinOne-rust-singleton-{}", unsafe { GetCurrentThreadId() });
    let window = fixture.app.borrow().window;
    let mut instance = crate::lifecycle::Instance::claim(&pipe,false).unwrap().unwrap();
    instance.attach(window).unwrap();
    assert!(crate::lifecycle::Instance::claim(&pipe,true).unwrap().is_none(), "second launch must reuse the existing window");
    let mut message = MSG::default();
    assert_ne!(unsafe { PeekMessageW(&mut message,window,crate::lifecycle::WAKE,crate::lifecycle::WAKE,PM_REMOVE) },0);
    unsafe { DispatchMessageW(&message); }
    assert_eq!(fixture.app.borrow().navigation.page(),"wintray");
    assert!(fixture.app.borrow().navigation.pending().is_some(), "external navigation must protect edits");
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["startupIntervalSeconds"],5);
    let event_name = wide(&crate::lifecycle::names(&pipe).3);
    let event = unsafe { OpenEventW(windows_sys::Win32::Storage::FileSystem::SYNCHRONIZE,0,event_name.as_ptr()) };
    assert!(!event.is_null(), "Host must observe the UI presence");
    unsafe { CloseHandle(event); }
    drop(instance);
    assert!(unsafe { OpenEventW(windows_sys::Win32::Storage::FileSystem::SYNCHRONIZE,0,event_name.as_ptr()) }.is_null(), "normal exit must release presence");
    assert!(crate::lifecycle::Instance::claim(&pipe,false).unwrap().is_some(), "the instance can be reopened after exit");
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn global_diagnostics_stay_available_when_host_settings_fail() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.app.borrow_mut().draft = None;
    fixture.app.borrow_mut().status = "Host unavailable".into();
    fixture.click(TAB+3);
    let logs = fixture.requests.try_recv().unwrap();
    assert!(matches!(&logs,Task::Logs { page, .. } if page == "host"));
    fixture.replies.send(worker::Reply {task:logs,result:Ok(json!({"folderAvailable":true,"entries":[{"name":"process","available":true,"truncated":false,"text":"startup error"}]}))}).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window)=="startup error"));
    fixture.click(331);
    assert!(matches!(fixture.requests.try_recv().unwrap(),Task::Logs { page, .. } if page == "capture"));
    assert_eq!(fixture.app.borrow().tab(),3);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn reloading_settings_requires_discard_and_keeps_the_draft_on_cancel() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.app.borrow_mut().draft.as_mut().unwrap().set("theme", json!("dark")).unwrap();
    fixture.click(RELOAD);
    assert!(fixture.requests.try_recv().is_err(), "reload must wait for the user's discard decision");
    assert!(fixture.app.borrow().navigation.pending().is_some());
    fixture.click(STAY);
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["theme"], "dark");
    fixture.click(RELOAD);
    fixture.click(DISCARD);
    assert!(matches!(fixture.requests.try_recv().unwrap(), Task::Load { page, .. } if page == "settings"));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn module_toggle_waits_for_completion_and_refreshes_after_failure() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV);
    let read = fixture.requests.try_recv().unwrap();
    assert!(matches!(read, Task::ModulesSnapshot { .. }));
    let snapshot = json!({"revision":7,"modules":[{"manifest":{"id":"capture","name":{"zh-CN":"截图","en-US":"Capture"}},"enabled":true,"pendingStop":false,"runtime":{"state":"ready"}}]});
    fixture.replies.send(worker::Reply { task: read, result: Ok(snapshot.clone()) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.click(1000);
    let change = fixture.requests.try_recv().unwrap();
    assert!(matches!(&change, Task::ModulesSetEnabled { id, enabled:false, revision:7 } if id == "capture"));
    fixture.click(1000);
    assert!(fixture.requests.try_recv().is_err(), "repeated clicks must not submit another transaction");
    fixture.app.borrow_mut().navigate(Destination::Close).unwrap();
    assert_ne!(unsafe { IsWindow(fixture.app.borrow().window) }, 0);
    fixture.replies.send(worker::Reply { task: change, result: Err(winone_desktop::protocol::RpcError::new("conflict", "配置已变更")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let refresh = fixture.requests.try_recv().unwrap();
    assert!(matches!(refresh, Task::ModulesSnapshot { .. }));
    fixture.replies.send(worker::Reply { task: refresh, result: Ok(snapshot) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.control_text(STATUS).contains("配置已变更"));
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(1000)) }, 0);
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn profile_import_preserves_review_on_failure_and_commits_only_reviewed_revisions() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(TAB+2);
    let preview = json!({"name":"Portable","sourceHash":"a".repeat(64),"configurationRevision":18,"moduleRevision":6,
        "preferences":{"theme":"dark","language":"en-US","runAtLogon":false,"startMinimized":true,"enabled":{"capture":true},"shortcuts":{}},
        "plan":{"modules":[{"id":"capture","state":"ready","requiredPaths":[],"settings":{"copyToClipboard":true}}],"shortcutIssues":[]},
        "paths":[],"currentShortcuts":{}});
    fixture.replies.send(worker::Reply { task:Task::ProfilePreview { path:"original.winone".into(), bindings:json!({}) }, result:Ok(preview.clone()) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.click(383);
    let apply = fixture.requests.try_recv().unwrap();
    let Task::ProfileApply { params } = &apply else { panic!("expected import"); };
    assert_eq!(params["configurationRevision"], 18);
    assert_eq!(params["moduleRevision"], 6);
    assert_eq!(params["modules"], json!(["capture"]));
    assert_eq!(params["sourceHash"], "a".repeat(64));
    fixture.click(383);
    assert!(fixture.requests.try_recv().is_err(), "import is submitted once");
    fixture.replies.send(worker::Reply { task:apply, result:Err(winone_desktop::protocol::RpcError::new("conflict", "changed")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().profiles.preview, preview, "failure must preserve the reviewed choices");
    assert_eq!(unsafe { IsWindowEnabled(fixture.control(383)) }, 0, "a failed import must be reviewed again");
    fixture.click(382);
    assert!(matches!(fixture.requests.try_recv().unwrap(), Task::ProfilePreview { path, .. } if path == "original.winone"));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn startup_edits_preserve_administrator_authorization_and_unknown_settings() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.app.borrow_mut().draft = Some(Draft::new(json!({"revision":4,"startup":{"runAtLogon":false,"startMinimized":true,"preferAdministrator":true,"future":"keep"}})).unwrap());
    fixture.click(TAB+1);
    let field = fixture.control(FIELD);
    unsafe { SendMessageW(field, BM_SETCHECK, BST_CHECKED as usize, 0); }
    fixture.click(FIELD);
    fixture.click(SAVE);
    let Task::Save { value, .. } = fixture.requests.try_recv().unwrap() else { panic!("expected global save"); };
    assert_eq!(value["revision"], 4);
    assert_eq!(value["startup"], json!({"runAtLogon":true,"startMinimized":true,"preferAdministrator":true,"future":"keep"}));
}

#[test]
#[ignore = "creates controls on a private desktop"]
fn closing_during_recording_waits_for_verified_restoration() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.open_capture_shortcuts();
    fixture.begin_recording("closing-token");
    let window = fixture.app.borrow().window;
    fixture.app.borrow_mut().navigate(Destination::Close).unwrap();
    assert_ne!(unsafe { IsWindow(window) }, 0, "closing must not cancel restoration in flight");
    let stop = fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply { task:stop, result:Ok(json!({"stopped":false})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_ne!(unsafe { IsWindow(window) }, 0, "an unconfirmed restoration cannot approve closing");
}
impl Desktop {
    fn create() -> Self {
        let original = unsafe { GetThreadDesktop(GetCurrentThreadId()) };
        let name = wide(&format!("WinOneRustTest-{}-{}", std::process::id(), unsafe { GetCurrentThreadId() }));
        let isolated = unsafe { CreateDesktopW(name.as_ptr(), null(), null(), 0,
            DESKTOP_CREATEWINDOW | DESKTOP_READOBJECTS | DESKTOP_WRITEOBJECTS | DESKTOP_HOOKCONTROL, null()) };
        assert!(!isolated.is_null(), "{}", std::io::Error::last_os_error());
        assert_ne!(unsafe { SetThreadDesktop(isolated) }, 0);
        Self { original, isolated }
    }
}
impl Drop for Desktop {
    fn drop(&mut self) { unsafe { SetThreadDesktop(self.original); CloseDesktop(self.isolated); } }
}

fn red_icon() -> Vec<u8> {
    let mut pixels = vec![0u8; 32 * 32 * 4];
    for index in (0..pixels.len()).step_by(4) { pixels[index + 2] = 255; pixels[index + 3] = 255; }
    pixels
}

struct Fixture {
    app: Box<RefCell<App>>, class: Vec<u16>,
    requests: std::sync::mpsc::Receiver<Task>, replies: std::sync::mpsc::Sender<worker::Reply>,
}
impl Fixture {
    fn new() -> Self {
        let app = Box::new(RefCell::new(App::new()));
        let (worker, requests, replies) = Worker::test_channels();
        let class = wide(&format!("WinOneRustTestWindow-{}", unsafe { GetCurrentThreadId() }));
        let instance = unsafe { GetModuleHandleW(null()) };
        let registration = WNDCLASSW { lpfnWndProc: Some(window_proc), hInstance: instance,
            lpszClassName: class.as_ptr(), ..Default::default() };
        assert_ne!(unsafe { RegisterClassW(&registration) }, 0);
        let window = unsafe { CreateWindowExW(WS_EX_CONTROLPARENT, class.as_ptr(), w!("WinOne test"),
            WS_OVERLAPPEDWINDOW | WS_CLIPCHILDREN, 0, 0, 1100, 780, null_mut(), null_mut(), instance,
            (&*app as *const RefCell<App>).cast()) };
        assert!(!window.is_null());
        {
            let mut state = app.borrow_mut();
            state.window = window;
            state.worker = Some(worker);
            state.palette = Some(Palette::new(window, "light"));
            state.snapshot = json!({"locale":"zh-CN", "settings":{"revision":0,"theme":"light","language":"zh-CN"}});
            state.catalog = json!([{"id":"capture","name":{"zh-CN":"截图","en-US":"Capture"}},
                {"id":"always-on-top","name":{"zh-CN":"置顶功能","en-US":"Always on Top"}},
                {"id":"wintray","name":{"zh-CN":"程序管理","en-US":"Program management"}}]);
            state.pages = build_navigation(&state.catalog, "zh-CN").unwrap();
            state.navigation = Navigation::new("settings");
            state.loading = false;
            state.draft = Some(Draft::new(state.snapshot["settings"].clone()).unwrap());
            state.render().unwrap();
        }
        Self { app, class, requests, replies }
    }
    fn control(&self, id: usize) -> HWND {
        self.app.borrow().controls.iter().find(|c| c.id == id).unwrap().window
    }
    fn control_text(&self, id: usize) -> String { widgets::text(self.control(id)) }
    fn drain_shortcut_entry(&self) {
        // Entering capture or always-on-top also requests the global settings
        // snapshot and the shortcut registration status.
        for _ in 0..2 {
            match self.requests.try_recv().unwrap() {
                Task::ShortcutSnapshot | Task::ShortcutStatus => {}
                _task => panic!("expected a shortcut entry request, got a different task"),
            }
        }
    }
    fn click(&self, id: usize) {
        let window = self.app.borrow().window;
        let control = self.control(id);
        unsafe { SendMessageW(window, WM_COMMAND, id, control as isize); }
    }
    fn load_programs(&self) {
        self.app.borrow_mut().snapshot["modules"] = json!([{"id":"wintray","state":"ready"}]);
        self.click(NAV + 3);
        let task = self.requests.try_recv().unwrap();
        self.replies.send(worker::Reply { task, result: Ok(json!({
            "revision":"7", "settings":{"managedApps":[{"id":"saved-app","name":"Saved app",
                "exePath":"app.exe","args":"--saved","runOnStartup":false,
                "collectTrayIcon":false,"launchViaLogonTask":false,"launchHiddenInBackground":false,
                "trayBehavior":{"autoMinimizeAndHideOnLaunch":false,"closeDelaySeconds":0},
                "schedule":{"enabled":false,"startDelayMinutes":0,"autoExitMinutes":0,
                    "frequencyEnabled":false,"frequencyDays":1,"frequencyRuns":1}}]}
        })) }).unwrap();
        self.app.borrow_mut().replies().unwrap();
        // Loading the draft requests the program's icon; answer it with a red
        // square so the display path (decode + icon creation) is exercised.
        let icon_task = self.requests.try_recv().unwrap();
        assert!(matches!(&icon_task, Task::ProgramIcon { path } if path == "app.exe"), "loading programs must request their icons");
        self.replies.send(worker::Reply { task: icon_task, result: Ok(json!({"path":"app.exe","icon":crate::icon::base64_encode(&red_icon())})) }).unwrap();
        self.app.borrow_mut().replies().unwrap();
    }
    fn drain_icon_requests(&self) {
        while let Ok(task) = self.requests.try_recv() {
            let Task::ProgramIcon { path } = task else { panic!("expected only program icon requests") };
            self.replies.send(worker::Reply { task: Task::ProgramIcon { path: path.clone() }, result: Ok(json!({"path":path,"icon":null})) }).unwrap();
        }
        self.app.borrow_mut().replies().unwrap();
    }
    fn open_capture_shortcuts(&self) {
        self.click(NAV + 1);
        let load = self.requests.try_recv().unwrap();
        let snapshot = self.requests.try_recv().unwrap();
        let status = self.requests.try_recv().unwrap();
        assert!(matches!(&load, Task::Load { page, .. } if page == "capture"), "capture entry must read module settings");
        assert!(matches!(snapshot, Task::ShortcutSnapshot), "capture entry must refresh the global settings snapshot");
        assert!(matches!(status, Task::ShortcutStatus), "capture entry must read shortcut registrations");
        self.replies.send(worker::Reply { task: snapshot, result: Ok(json!({
            "locale":"zh-CN", "settings":{"revision":9,"captureShortcut":"Ctrl+Alt+C","pinShortcut":"Ctrl+Alt+P"}})) }).unwrap();
        self.replies.send(worker::Reply { task: status, result: Ok(json!({
            "bindings":[{"id":"capture","shortcut":"Ctrl+Alt+C","registered":true}],
            "lastError":"","actionErrors":{}})) }).unwrap();
        self.replies.send(worker::Reply { task: load, result: Ok(json!({"revision":"2","copyToClipboard":true})) }).unwrap();
        self.app.borrow_mut().replies().unwrap();
    }
    fn focus_recorder(&self) {
        let field = self.control(shortcut_view::SHORTCUT_FIELD);
        let window = self.app.borrow().window;
        unsafe { SendMessageW(window, WM_COMMAND, shortcut_view::SHORTCUT_FIELD | ((EN_SETFOCUS as usize) << 16), field as isize); }
    }
    fn begin_recording(&self, token: &str) {
        self.focus_recorder();
        let task = self.requests.try_recv().unwrap();
        assert!(matches!(task, Task::ShortcutRecord), "focusing the recorder must pause Host registrations");
        self.replies.send(worker::Reply { task, result: Ok(json!({"token":token})) }).unwrap();
        self.app.borrow_mut().replies().unwrap();
    }
    fn end_recording(&self, token: &str, status: Value) {
        let stop = self.requests.try_recv().unwrap();
        assert!(matches!(&stop, Task::ShortcutStop { token: expected } if expected == token), "the recorder must restore the paused registrations");
        self.replies.send(worker::Reply { task: stop, result: Ok(json!({"stopped":true})) }).unwrap();
        self.app.borrow_mut().replies().unwrap();
        let refresh = self.requests.try_recv().unwrap();
        assert!(matches!(refresh, Task::ShortcutStatus), "restoring must re-read registrations");
        self.replies.send(worker::Reply { task: refresh, result: Ok(status) }).unwrap();
        self.app.borrow_mut().replies().unwrap();
    }
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn launch_submits_once_protects_navigation_and_preserves_settings_after_failure() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.load_programs();
    let saved = fixture.app.borrow().draft.as_ref().unwrap().value().clone();
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(357)) }, 0);
    fixture.click(357);
    fixture.click(357);
    let task = fixture.requests.try_recv().unwrap();
    assert!(matches!(&task, Task::Launch { id } if id == "saved-app"));
    assert!(fixture.requests.try_recv().is_err(), "a repeated click must not submit another launch");
    assert_eq!(unsafe { IsWindowEnabled(fixture.control(357)) }, 0);
    fixture.click(NAV);
    let window = fixture.app.borrow().window;
    unsafe { SendMessageW(window, WM_CLOSE, 0, 0); }
    {
        let app = fixture.app.borrow();
        assert_eq!(app.navigation.page(), "wintray");
        assert!(app.navigation.pending().is_none());
        assert!(app.status.contains("启动"), "a running launch must not be described as a save: {}", app.status);
        assert_ne!(unsafe { IsWindow(window) }, 0);
    }
    fixture.replies.send(worker::Reply { task, result: Err(winone_desktop::protocol::RpcError::new("operation_failed", "fixture launch denied")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    {
        let app = fixture.app.borrow();
        assert!(!app.saving());
        assert!(!app.dirty());
        assert_eq!(app.draft.as_ref().unwrap().value(), &saved);
        assert!(app.status.contains("fixture launch denied"));
    }
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(357)) }, 0);
    fixture.click(NAV);
    assert_eq!(fixture.app.borrow().navigation.page(), "overview");
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn unsaved_or_disabled_programs_cannot_launch_and_completed_launch_releases_controls() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.load_programs();
    fixture.click(351);
    let name = fixture.app.borrow().bindings.iter().find(|b| b.key == "name").unwrap().id;
    widgets::set_text(fixture.control(name), "Unsaved name");
    assert_eq!(unsafe { IsWindowEnabled(fixture.control(357)) }, 0);
    fixture.click(357);
    assert!(fixture.requests.try_recv().is_err());
    widgets::set_text(fixture.control(name), "Saved app");
    fixture.app.borrow_mut().snapshot["modules"][0]["state"] = json!("disabled");
    fixture.app.borrow().refresh_buttons();
    assert_eq!(unsafe { IsWindowEnabled(fixture.control(357)) }, 0);
    fixture.click(357);
    assert!(fixture.requests.try_recv().is_err());
    fixture.app.borrow_mut().snapshot["modules"][0]["state"] = json!("ready");
    fixture.app.borrow().refresh_buttons();
    fixture.click(357);
    let task = fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply { task, result: Ok(json!({"Managed":true})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(!fixture.app.borrow().saving());
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(357)) }, 0);
    fixture.click(NAV);
    assert_eq!(fixture.app.borrow().navigation.page(), "overview");
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn global_reload_fetches_current_revision_and_keeps_failed_edits() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(RELOAD);
    let task = fixture.requests.try_recv().expect("Reload must request current settings from Host");
    assert!(matches!(&task, Task::Load { page, .. } if page == "settings"));
    let snapshot = json!({"locale":"en-US", "settings":{"revision":8,"theme":"dark","language":"en-US"}});
    fixture.replies.send(worker::Reply { task, result: Ok(snapshot) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["revision"], 8);
    assert_eq!(fixture.app.borrow().pages.last().unwrap().title, "Settings");
    {
        let mut app = fixture.app.borrow_mut();
        app.draft.as_mut().unwrap().set("theme", json!("light")).unwrap();
    }
    fixture.click(SAVE);
    let task = fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply { task, result: Err(winone_desktop::protocol::RpcError::new("conflict", "changed externally")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().dirty());
    assert!(!fixture.app.borrow().saving());
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["theme"], "light");
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn late_module_load_does_not_replace_current_page_or_selected_tab() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV + 1);
    let old = fixture.requests.try_recv().unwrap();
    fixture.drain_shortcut_entry();
    fixture.click(NAV + 2);
    let current = fixture.requests.try_recv().unwrap();
    fixture.drain_shortcut_entry();
    fixture.click(TAB + 2);
    fixture.replies.send(worker::Reply { task: old, result: Ok(json!({"revision":3,"outputDirectory":"old"})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().navigation.page(), "always-on-top");
    assert!(fixture.app.borrow().loading);
    assert!(fixture.app.borrow().draft.is_none());
    fixture.replies.send(worker::Reply { task: current, result: Err(winone_desktop::protocol::RpcError::new("load_failed", "test load failure")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().tab(), 2);
    assert!(!fixture.app.borrow().loading);
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(TAB + 2)) }, 0);
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn diagnostics_remain_readable_when_settings_are_loading_or_fail() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV + 1);
    let settings = fixture.requests.try_recv().unwrap();
    fixture.drain_shortcut_entry();
    fixture.click(TAB + 3);
    let logs = fixture.requests.try_recv().expect("diagnostics must request logs while settings are loading");
    fixture.replies.send(worker::Reply { task: logs, result: Ok(json!({
        "folderAvailable":true,"entries":[{"name":"process","available":true,"truncated":true,"text":"fixture worker failure"}]
    })) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().loading, "log completion must not finish the unrelated settings read");
    fixture.replies.send(worker::Reply { task: settings, result: Err(winone_desktop::protocol::RpcError::new("load_failed", "fixture settings failure")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let app = fixture.app.borrow();
    assert_eq!(app.tab(), 3);
    assert!(app.draft.is_none());
    let log = app.controls.iter().find(|c| widgets::text(c.window) == "fixture worker failure").expect("settings failure must not remove the log viewer");
    let style = unsafe { GetWindowLongW(log.window, GWL_STYLE) } as u32;
    assert_ne!(style & ES_READONLY as u32, 0);
    assert_eq!(style & WS_HSCROLL, 0, "long logs must wrap instead of adding a horizontal scrollbar");
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn diagnostics_ignore_other_pages_and_show_missing_logs_without_losing_edits() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV + 1);
    let _capture_settings = fixture.requests.try_recv().unwrap();
    fixture.drain_shortcut_entry();
    fixture.click(TAB + 3);
    let old_logs = fixture.requests.try_recv().expect("capture log request");
    fixture.click(NAV + 2);
    let settings = fixture.requests.try_recv().unwrap();
    fixture.drain_shortcut_entry();
    fixture.replies.send(worker::Reply { task: settings, result: Ok(json!({"revision":1,"frame":false})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let frame = fixture.app.borrow().bindings.iter().find(|b| b.key == "frame").unwrap().id;
    let control = fixture.control(frame);
    unsafe { SendMessageW(control, BM_SETCHECK, BST_CHECKED as usize, 0); }
    fixture.click(frame);
    fixture.click(TAB + 2);
    let current_logs = fixture.requests.try_recv().expect("pinning log request");
    fixture.replies.send(worker::Reply { task: current_logs, result: Ok(json!({
        "folderAvailable":false,"entries":[{"name":"process","available":false,"truncated":false,"text":""}]
    })) }).unwrap();
    fixture.replies.send(worker::Reply { task: old_logs, result: Ok(json!({
        "folderAvailable":true,"entries":[{"name":"process","available":true,"truncated":false,"text":"stale capture log"}]
    })) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let app = fixture.app.borrow();
    assert_eq!(app.navigation.page(), "always-on-top");
    assert_eq!(app.tab(), 2);
    assert!(app.dirty());
    assert_eq!(app.draft.as_ref().unwrap().value()["frame"], true);
    assert!(app.controls.iter().any(|c| widgets::text(c.window).contains("暂无日志")));
    assert!(!app.controls.iter().any(|c| widgets::text(c.window).contains("stale capture log")));
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn diagnostics_retry_and_folder_failure_keep_long_unicode_logs_readable() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.load_programs();
    fixture.click(TAB + 2);
    let task = fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply { task, result: Err(winone_desktop::protocol::RpcError::new("diagnostics", "fixture read denied")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window).contains("fixture read denied")));
    fixture.click(log_view::REFRESH_LOGS);
    let task = fixture.requests.try_recv().unwrap();
    assert!(matches!(&task, Task::Logs { page, .. } if page == "wintray"));
    // A valid 64 KiB Host log can still exceed the edit control's usual
    // 32K-character input limit. Keep the fixture within that byte contract.
    let log_text = "运行".repeat(4000) + &"x".repeat(40000) + "最后一行";
    fixture.replies.send(worker::Reply { task, result: Ok(json!({
        "folderAvailable":true,"entries":[{"name":"activity","available":true,"truncated":true,"text":log_text}]
    })) }).unwrap();
    let render_started = Instant::now();
    fixture.app.borrow_mut().replies().unwrap();
    let render_elapsed = render_started.elapsed();
    println!("private-desktop 64 KiB log render={render_elapsed:?}");
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window) == log_text), "the native viewer must not silently cut long log text");
    // This catches pathological wrapping during construction, not input-to-DWM
    // latency or the product's final visible-response performance budget.
    assert!(render_elapsed < std::time::Duration::from_millis(100), "log construction stalled the UI: {render_elapsed:?}");
    fixture.click(log_view::OPEN_LOG_FOLDER);
    let task = fixture.requests.try_recv().unwrap();
    assert!(matches!(&task, Task::LogFolder { page, .. } if page == "wintray"));
    fixture.replies.send(worker::Reply { task, result: Err(winone_desktop::protocol::RpcError::new("diagnostics", "fixture folder unavailable")) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window) == log_text));
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window).contains("fixture folder unavailable")));
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(log_view::REFRESH_LOGS)) }, 0);
}

#[test]
#[ignore = "compares native log controls on a private desktop; opt in with -NativeIntegration"]
fn compare_native_log_controls_at_the_same_size_and_content() {
    use windows_sys::Win32::System::LibraryLoader::{LoadLibraryExW, LOAD_LIBRARY_SEARCH_SYSTEM32};
    let _desktop = Desktop::create();
    let library = unsafe { LoadLibraryExW(w!("Msftedit.dll"), null_mut(), LOAD_LIBRARY_SEARCH_SYSTEM32) };
    assert!(!library.is_null());
    {
        let fixture = Fixture::new();
        let text = "运行".repeat(4000) + &"x".repeat(40000) + "最后一行";
        for class in ["EDIT", "RICHEDIT50W"] {
            let started = Instant::now();
            let window = fixture.app.borrow_mut().add(class, &text, 499, WS_TABSTOP | WS_VSCROLL | ES_MULTILINE as u32 | ES_READONLY as u32 | ES_AUTOVSCROLL as u32, Area::Content, [0,246,0,230], false).unwrap();
            println!("private-desktop same-log {class}={:?}", started.elapsed());
            assert_eq!(widgets::text(window), text);
            fixture.app.borrow_mut().controls.pop();
        }
    }
    unsafe { FreeLibrary(library); }
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn program_preferences_keep_revision_envelope_and_confirm_completion() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV + 3);
    let task = fixture.requests.try_recv().unwrap();
    let settings = json!({"schemaVersion":3,"startupIntervalSeconds":3,"closeWindowRetrySeconds":10,
        "exitAfterManagedAppsCompleted":false,"managedApps":[{"id":"kept","name":"Kept"}]});
    fixture.replies.send(worker::Reply { task, result: Ok(json!({"revision":"41","settings":settings})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.click(TAB + 1);
    let field = fixture.app.borrow().bindings.iter().find(|b| b.key == "startupIntervalSeconds").map(|b| b.id)
        .expect("program startup interval must be editable");
    widgets::set_text(fixture.control(field), "8");
    fixture.click(SAVE);
    let task = fixture.requests.try_recv().unwrap();
    let Task::Save { value, .. } = &task else { panic!("expected a save"); };
    assert_eq!(value["revision"], "41");
    assert_eq!(value["settings"]["startupIntervalSeconds"], 8);
    assert_eq!(value["settings"]["managedApps"], settings["managedApps"]);
    fixture.replies.send(worker::Reply { task: task.clone(), result: Ok(json!({"state":"completed","result":{
        "revision":"42", "settings":value["settings"]}})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(!fixture.app.borrow().dirty());
    assert_eq!(fixture.app.borrow().module_data["revision"], "42");
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["startupIntervalSeconds"], 8);
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn program_selection_preserves_edits_in_both_columns() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV + 3);
    let task = fixture.requests.try_recv().unwrap();
    let item = |id: &str| json!({"id":id,"name":id,"exePath":"app.exe","args":"","runOnStartup":false,
        "collectTrayIcon":false,"launchViaLogonTask":false,"launchHiddenInBackground":false,
        "trayBehavior":{"autoMinimizeAndHideOnLaunch":false,"closeDelaySeconds":0},
        "schedule":{"enabled":false,"startDelayMinutes":0,"autoExitMinutes":0,"frequencyEnabled":false,"frequencyDays":1,"frequencyRuns":1}});
    fixture.replies.send(worker::Reply { task, result: Ok(json!({"revision":"1","settings":{"managedApps":[item("one"),item("two")]}})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.click(351); // More settings for the selected program.
    let name_field = || fixture.app.borrow().bindings.iter().find(|b| b.key == "name").map(|b| b.id).expect("program name editor");
    widgets::set_text(fixture.control(name_field()), "   ");
    assert_eq!(unsafe { IsWindowEnabled(fixture.control(SAVE)) }, 0, "blank names must block saving");
    widgets::set_text(fixture.control(name_field()), "Renamed first");
    let window = fixture.app.borrow().window;
    let list = fixture.control(450);
    unsafe { SendMessageW(list, LB_SETCURSEL, 1, 0); SendMessageW(window, WM_COMMAND, 450 | ((LBN_SELCHANGE as usize) << 16), list as isize); }
    fixture.click(351);
    widgets::set_text(fixture.control(name_field()), "Renamed second");
    let state = fixture.app.borrow();
    let programs = &state.draft.as_ref().unwrap().value()["managedApps"];
    assert_eq!(programs[0]["name"],"Renamed first");
    assert_eq!(programs[1]["name"],"Renamed second");
    assert!(state.dirty());
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn add_replace_reorder_remove_programs_stay_in_the_unsaved_draft() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.click(NAV + 3);
    let task = fixture.requests.try_recv().unwrap();
    fixture.replies.send(worker::Reply { task, result: Ok(json!({"revision":"5","settings":{"managedApps":[]}})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.app.borrow_mut().apply_program_paths(&[r"C:\Apps\First.exe".into(),r"C:\Apps\Second.exe".into()],false).unwrap();
    let selected = fixture.app.borrow().selected_program.clone();
    fixture.app.borrow_mut().apply_program_paths(&[r"C:\Scripts\Replacement.ps1".into()],true).unwrap();
    let order = fixture.control(452);
    let window = fixture.app.borrow().window;
    unsafe { SendMessageW(order,CB_SETCURSEL,0,0); SendMessageW(window,WM_COMMAND,452 | ((CBN_SELCHANGE as usize)<<16),order as isize); }
    {
        let app = fixture.app.borrow();
        let entries = &app.draft.as_ref().unwrap().value()["managedApps"];
        assert_eq!(entries[0]["id"],selected);
        assert_eq!(entries[0]["name"],"Replacement");
        assert_eq!(entries[0]["launchHiddenInBackground"],true);
        assert_eq!(entries[1]["name"],"First");
        assert_eq!(app.module_data["settings"]["managedApps"],json!([]));
    }
    fixture.click(354);
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["managedApps"].as_array().unwrap().len(),1);
    assert!(fixture.app.borrow().dirty());
    // Icon extraction reads file resources only; settle those requests before
    // proving that list edits never save or launch programs.
    fixture.drain_icon_requests();
    assert!(fixture.requests.try_recv().is_err(),"local list edits must not save or launch programs");
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let window = self.app.borrow().window;
        unsafe { DestroyWindow(window); }
        self.app.borrow_mut().controls.clear();
        unsafe { UnregisterClassW(self.class.as_ptr(), GetModuleHandleW(null())); }
    }
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn program_list_draws_icons_and_the_detail_header_identifies_the_program() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.load_programs();
    {
        let app = fixture.app.borrow();
        assert!(app.icons.get("app.exe").and_then(|icon| *icon).is_some(), "the icon reply must create a display icon");
    }
    let list = fixture.control(450);
    let style = unsafe { GetWindowLongW(list, GWL_STYLE) } as u32;
    assert_ne!(style & LBS_OWNERDRAWFIXED as u32, 0, "the program list must be owner-drawn");
    assert_eq!(unsafe { SendMessageW(list, LB_GETITEMHEIGHT, 0, 0) }, fixture.app.borrow().palette.as_ref().unwrap().px(44) as isize);
    let header = fixture.control(program_view::PROGRAM_ICON);
    let shown = unsafe { SendMessageW(header, STM_GETICON, 0, 0) };
    assert_eq!(shown, fixture.app.borrow().icons.get("app.exe").and_then(|icon| *icon).unwrap() as isize,
        "the detail header must show the selected program's icon");
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window) == "Saved app"), "the detail header must show the program name");
    assert!(fixture.app.borrow().controls.iter().any(|c| widgets::text(c.window) == "app.exe"), "the detail header must show the program path");
    // Paint one row into a memory DC: the synthetic red icon must land inside it.
    let (icon, label) = fixture.app.borrow().program_list_item(0);
    assert_eq!(label, "Saved app");
    let icon = icon.expect("the row must carry the program icon");
    unsafe {
        // A fresh memory DC carries a monochrome surface; compatible bitmaps must
        // be created from the screen DC or the row would paint in 1bpp.
        let screen = GetDC(std::ptr::null_mut());
        assert!(!screen.is_null());
        let dc = CreateCompatibleDC(screen);
        assert!(!dc.is_null());
        let bitmap = CreateCompatibleBitmap(screen, 300, 44);
        ReleaseDC(std::ptr::null_mut(), screen);
        assert!(!bitmap.is_null());
        let previous = SelectObject(dc, bitmap);
        let item = DRAWITEMSTRUCT { CtlType: ODT_LISTBOX as u32, CtlID: 450, itemID: 0, itemAction: ODA_DRAWENTIRE, itemState: 0,
            hwndItem: list, hDC: dc, rcItem: RECT { left: 0, top: 0, right: 300, bottom: 44 }, itemData: 0 };
        program_view::draw_program_item(&item, fixture.app.borrow().palette.as_ref().unwrap(), Some(icon), &label);
        assert_eq!(GetPixel(dc, 28, 22), 0x000000ff, "the icon must be painted inside the row");
        assert_eq!(GetPixel(dc, 40, 22), 0x000000ff, "the full 32-pixel icon span must be painted");
        SelectObject(dc, previous);
        DeleteObject(bitmap);
        DeleteDC(dc);
    }
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn settled_program_icons_are_not_re_requested_but_new_paths_are() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.load_programs();
    fixture.click(RELOAD);
    let task = fixture.requests.try_recv().unwrap();
    assert!(matches!(&task, Task::Load { page, .. } if page == "wintray"));
    fixture.replies.send(worker::Reply { task, result: Ok(json!({
        "revision":"7", "settings":{"managedApps":[{"id":"saved-app","name":"Saved app","exePath":"app.exe","args":"--saved"}]}
    })) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.requests.try_recv().is_err(), "settled icons must be served without a new request");
    fixture.app.borrow_mut().apply_program_paths(&[r"C:\Apps\Other.exe".into()], true).unwrap();
    let task = fixture.requests.try_recv().unwrap();
    assert!(matches!(&task, Task::ProgramIcon { path } if path == r"C:\Apps\Other.exe"), "a replaced path must be requested");
    fixture.replies.send(worker::Reply { task, result: Ok(json!({"path":r"C:\Apps\Other.exe","icon":null})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert!(fixture.requests.try_recv().is_err(), "icon-less replies must settle the path");
    assert!(fixture.app.borrow().icons.get(r"C:\Apps\Other.exe").is_some_and(|icon| icon.is_none()));
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn native_draft_navigation_and_control_recycling() {
    let _desktop = Desktop::create();
    let started = Instant::now();
    let fixture = Fixture::new();
    let initialized = started.elapsed();
    let choice = fixture.control(FIELD);
    let window = fixture.app.borrow().window;
    unsafe {
        SendMessageW(choice, CB_SETCURSEL, 2, 0);
        SendMessageW(window, WM_COMMAND, FIELD | ((CBN_SELCHANGE as usize) << 16), choice as isize);
    }
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["theme"], "dark");
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(SAVE)) }, 0);
    fixture.click(NAV);
    assert!(fixture.app.borrow().navigation.pending().is_some());
    fixture.click(STAY);
    assert_eq!(fixture.app.borrow().navigation.page(), "settings");
    assert_eq!(fixture.app.borrow().draft.as_ref().unwrap().value()["theme"], "dark");
    let window = fixture.app.borrow().window;
    unsafe { SendMessageW(window, WM_CLOSE, 0, 0); }
    assert_ne!(unsafe { IsWindow(window) }, 0, "dirty close must preserve the window");
    fixture.click(STAY);
    let process = unsafe { GetCurrentProcess() };
    let gdi_before = unsafe { GetGuiResources(process, GR_GDIOBJECTS) };
    let user_before = unsafe { GetGuiResources(process, GR_USEROBJECTS) };
    let warm = Instant::now();
    for _ in 0..100 { fixture.click(NAV); fixture.click(STAY); }
    assert!(unsafe { GetGuiResources(process, GR_GDIOBJECTS) } <= gdi_before + 2);
    assert!(unsafe { GetGuiResources(process, GR_USEROBJECTS) } <= user_before + 2);
    println!("private-desktop setup={initialized:?}; 100 draft navigation cycles={:?}; GDI={gdi_before}; USER={user_before}", warm.elapsed());
    unsafe { SendMessageW(window, WM_CLOSE, 0, 0); }
    fixture.click(DISCARD);
    assert_eq!(unsafe { IsWindow(window) }, 0);
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn shortcut_recording_commits_on_modifier_release_and_restores_registration() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.open_capture_shortcuts();
    assert_eq!(fixture.control_text(shortcut_view::SHORTCUT_FIELD), "Ctrl+Alt+C");
    assert!(fixture.control_text(shortcut_view::SHORTCUT_HINT).contains("当前生效：Ctrl+Alt+C"),
        "the registration status must be visible next to the recorder");
    fixture.begin_recording("tok-1");
    // Preparing swallows keys so they cannot trigger dialog controls early.
    assert!(fixture.app.borrow_mut().record_key(WM_KEYDOWN, 'X' as usize));
    fixture.click(RELOAD);
    assert!(fixture.app.borrow().status.contains("录入中"), "page reloads must pause while recording");
    assert!(fixture.requests.try_recv().is_err());
    // The chord preview updates live; the draft only changes once every
    // modifier is released, so restoring registrations cannot retrigger it.
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, VK_CONTROL as usize);
    assert_eq!(fixture.control_text(shortcut_view::SHORTCUT_FIELD), "Ctrl");
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, 'X' as usize);
    fixture.app.borrow_mut().record_key(WM_KEYUP, 'X' as usize);
    assert_eq!(fixture.app.borrow().shortcut.phase, shortcut_view::RecordPhase::Recording,
        "releasing the key first must keep recording while Ctrl is held");
    fixture.app.borrow_mut().record_key(WM_KEYUP, VK_CONTROL as usize);
    {
        let app = fixture.app.borrow();
        assert_eq!(app.shortcut.draft["captureShortcut"], "Ctrl+X");
        assert!(app.any_dirty());
    }
    fixture.end_recording("tok-1", json!({
        "bindings":[{"id":"capture","shortcut":"Ctrl+Alt+C","registered":true}],
        "lastError":"","actionErrors":{"capture":"boom"}}));
    assert_ne!(unsafe { IsWindowEnabled(fixture.control(shortcut_view::SHORTCUT_SAVE)) }, 0,
        "a recorded shortcut must enable its own save once registration is restored");
    assert!(fixture.control_text(shortcut_view::SHORTCUT_HINT).contains("最近一次快捷键操作失败：boom"),
        "restored registrations must surface action failures");
    // Leaving with unsaved shortcut edits must ask before discarding.
    fixture.click(NAV);
    assert!(fixture.app.borrow().navigation.pending().is_some());
    fixture.click(DISCARD);
    {
        let app = fixture.app.borrow();
        assert_eq!(app.navigation.page(), "overview");
        assert!(!app.any_dirty());
    }
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn shortcut_recording_rejects_invalid_keys_and_saves_the_settings_draft() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.open_capture_shortcuts();
    fixture.begin_recording("tok-1");
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, 'X' as usize);
    assert!(fixture.control_text(shortcut_view::SHORTCUT_HINT).contains("请同时按下 Ctrl、Alt、Shift 或 Win"));
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, VK_CONTROL as usize);
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, VK_F12 as usize);
    assert!(fixture.control_text(shortcut_view::SHORTCUT_HINT).contains("F12 为系统保留键"));
    // Escape cancels without touching the draft.
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, VK_ESCAPE as usize);
    assert_eq!(fixture.app.borrow().shortcut.draft["captureShortcut"], "Ctrl+Alt+C");
    fixture.end_recording("tok-1", json!({
        "bindings":[{"id":"capture","shortcut":"Ctrl+Alt+C","registered":true}],"lastError":"","actionErrors":{}}));
    // A second recording clears the shortcut with the Delete key.
    fixture.begin_recording("tok-2");
    fixture.app.borrow_mut().record_key(WM_KEYDOWN, VK_DELETE as usize);
    assert_eq!(fixture.app.borrow().shortcut.draft["captureShortcut"], "");
    fixture.end_recording("tok-2", json!({"bindings":[],"lastError":"","actionErrors":{}}));
    assert!(fixture.control_text(shortcut_view::SHORTCUT_HINT).contains("未启用"));
    // Saving publishes the global settings object with the cleared shortcut.
    fixture.click(shortcut_view::SHORTCUT_SAVE);
    let task = fixture.requests.try_recv().unwrap();
    let Task::ShortcutSave { value } = &task else { panic!("expected a shortcut settings save") };
    assert_eq!(value["captureShortcut"], "");
    assert_eq!(value["revision"], 9, "the save must carry the Host settings revision");
    fixture.replies.send(worker::Reply { task: task.clone(), result: Ok(json!({
        "revision":10, "captureShortcut":"", "pinShortcut":"Ctrl+Alt+P"})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    {
        let app = fixture.app.borrow();
        assert!(!app.shortcut_dirty());
        assert_eq!(app.snapshot["settings"]["revision"], 10);
        assert!(app.status.contains("已保存"));
    }
    let status = fixture.requests.try_recv().unwrap();
    assert!(matches!(status, Task::ShortcutStatus), "a successful save must refresh registrations");
    fixture.replies.send(worker::Reply { task: status, result: Ok(json!({"bindings":[],"lastError":"","actionErrors":{}})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    fixture.click(shortcut_view::SHORTCUT_CLEAR);
    assert!(fixture.requests.try_recv().is_err(), "clearing an empty shortcut must not record work");
    fixture.click(NAV);
    assert_eq!(fixture.app.borrow().navigation.page(), "overview", "a saved shortcut draft must not ask to discard");
}

#[test]
#[ignore = "creates native controls only on a private desktop; opt in with -NativeIntegration"]
fn leaving_the_page_while_preparing_releases_the_recording_token() {
    let _desktop = Desktop::create();
    let fixture = Fixture::new();
    fixture.open_capture_shortcuts();
    fixture.focus_recorder();
    let record = fixture.requests.try_recv().unwrap();
    assert!(matches!(record, Task::ShortcutRecord));
    // Navigate before the Host answers: no stop can be sent yet.
    fixture.click(NAV);
    assert_eq!(fixture.app.borrow().navigation.page(), "capture", "navigation must wait for shortcut restoration");
    assert!(fixture.requests.try_recv().is_err());
    // The late token must still be released.
    fixture.replies.send(worker::Reply { task: record, result: Ok(json!({"token":"tok-late"})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let stop = fixture.requests.try_recv().unwrap();
    assert!(matches!(&stop, Task::ShortcutStop { token } if token == "tok-late"));
    fixture.replies.send(worker::Reply { task: stop, result: Ok(json!({"stopped":true})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    let status = fixture.requests.try_recv().unwrap();
    assert!(matches!(status, Task::ShortcutStatus));
    assert!(matches!(fixture.requests.try_recv().unwrap(), Task::ModulesSnapshot { .. }));
    assert_eq!(fixture.app.borrow().navigation.page(), "overview");
    fixture.replies.send(worker::Reply { task: status, result: Ok(json!({"bindings":[],"lastError":"","actionErrors":{}})) }).unwrap();
    fixture.app.borrow_mut().replies().unwrap();
    assert_eq!(fixture.app.borrow().shortcut.phase, shortcut_view::RecordPhase::Idle);
}
