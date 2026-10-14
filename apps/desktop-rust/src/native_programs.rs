//! Program settings share the shell's controls, draft and save transaction.
use super::*;
pub(super) const PROGRAM_ICON: usize = 453;

/// The list falls back to the file name only when the editable name is blank;
/// blank names still block saving.
pub(super) fn display_name(program: &Value) -> String {
    if let Some(name) = program["name"].as_str().filter(|name| !name.trim().is_empty()) { return name.trim().into(); }
    program["exePath"].as_str().map(|path| {
        path.rsplit(['\\', '/']).next().unwrap_or("")
            .rsplit_once('.').map(|(stem, _)| stem).unwrap_or_else(|| path.rsplit(['\\', '/']).next().unwrap_or("")).to_string()
    }).unwrap_or_default()
}

pub(super) fn draw_program_item(item: &DRAWITEMSTRUCT, palette: &Palette, icon: Option<HICON>, label: &str) {
    unsafe {
        let selected = item.itemState & ODS_SELECTED != 0;
        let background = if selected { palette.selected } else { palette.background };
        let brush = CreateSolidBrush(background);
        FillRect(item.hDC, &item.rcItem, brush);
        DeleteObject(brush);
        let size = palette.px(32);
        let icon_y = item.rcItem.top + (item.rcItem.bottom - item.rcItem.top - size) / 2;
        if let Some(icon) = icon {
            DrawIconEx(item.hDC, item.rcItem.left + palette.px(12), icon_y, icon, size, size, 0, null_mut(), DI_NORMAL);
        }
        SetBkMode(item.hDC, TRANSPARENT as i32);
        SetTextColor(item.hDC, if item.itemState & ODS_DISABLED != 0 { palette.muted } else if selected {palette.selected_foreground} else { palette.foreground });
        let old_font = SelectObject(item.hDC, palette.font);
        let mut rect = item.rcItem;
        rect.left += palette.px(12) + size + palette.px(12);
        rect.right -= palette.px(8);
        DrawTextW(item.hDC, wide(label).as_ptr(), -1, &mut rect, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_END_ELLIPSIS);
        SelectObject(item.hDC, old_font);
        if item.itemState & ODS_FOCUS != 0 { DrawFocusRect(item.hDC, &item.rcItem); }
    }
}

impl App {
    pub(super) fn layout_program_workspace(&mut self) {
        let (Some(palette),Some(pane))=(&self.palette,&self.program_viewport) else{return;};
        // The shell never scrolls this workspace. Each task area owns its native
        // scroll range, keeping navigation, save and list actions stationary.
        self.scroll=0;
        let info=SCROLLINFO{cbSize:std::mem::size_of::<SCROLLINFO>()as u32,fMask:SIF_RANGE|SIF_PAGE|SIF_POS,nPage:1,..Default::default()};
        unsafe{SetScrollInfo(self.window,SB_VERT,&info,1);}
        let mut client=RECT::default();unsafe{GetClientRect(self.window,&mut client);}
        let width=(client.right as f32/palette.scale)as i32;
        let height=(client.bottom as f32/palette.scale)as i32;
        let left=widgets::SIDEBAR_WIDTH+32;let available=(width-left-32).max(1);
        let top=250+self.current_body_offset;let bottom=(height-80).max(top+180);
        let stacked=available<660;
        let list_width=if stacked{available}else{(available/3).clamp(260,300)};
        let list_bottom=if stacked{top+132}else{bottom-88};
        let detail=if stacked{RECT{left,top:list_bottom+112,right:left+available,bottom}}
            else{RECT{left:left+list_width+24,top,right:left+available,bottom}};
        pane.place(palette,detail);
        for control in &self.controls{
            match control.area{
                Area::ProgramDetail=>{},
                Area::ProgramList=>control.place_rect(palette,RECT{left,top,right:left+list_width,bottom:list_bottom}),
                Area::ProgramFooter(slot)=>{
                    let y=list_bottom+12+if slot>=2{40}else{0};
                    let (x,w)=match slot{0=>(0,128),1=>(136,80),_=>{let w=(list_width-16)/3;((slot as i32-2)*(w+8),w)}};
                    // A combo's requested height includes its dropdown, unlike
                    // its visible collapsed rectangle used for scroll extents.
                    control.place_rect(palette,RECT{left:left+x,top:y,right:left+x+w,bottom:y+control.height});
                },
                _ if control.id==STATUS=>control.place_rect(palette,RECT{left,top:height-64,right:left+available,bottom:height-8}),
                _=>control.place(palette,client,0),
            }
        }
        self.layout_program_details();
    }
    fn layout_program_details(&mut self){
        let (Some(palette),Some(pane))=(&self.palette,&self.program_viewport)else{return;};
        let mut client=RECT::default();unsafe{GetClientRect(pane.window,&mut client);}
        let page=(client.bottom as f32/palette.scale)as i32;
        let height=self.controls.iter().filter(|c|matches!(c.area,Area::ProgramDetail)).map(|c|{
            let mut rect=RECT::default();unsafe{GetWindowRect(c.window,&mut rect);}
            c.y+((rect.bottom-rect.top)as f32/palette.scale).ceil()as i32
        }).max().unwrap_or(0)+12;
        self.program_scroll=self.program_scroll.clamp(0,(height-page).max(0));
        pane.range(height,page,self.program_scroll);
        unsafe{GetClientRect(pane.window,&mut client);}
        let width=(client.right as f32/palette.scale)as i32;
        for control in self.controls.iter().filter(|c|matches!(c.area,Area::ProgramDetail)){
            control.place_local(palette,width,self.program_scroll);
        }
        unsafe{InvalidateRect(pane.window,null(),1);}
    }
    pub(super) fn program_scroll_message(&mut self,message:u32,w:WPARAM,window:HWND){
        if !self.program_viewport.as_ref().is_some_and(|p|p.window==window){return;}
        let mut info=SCROLLINFO{cbSize:std::mem::size_of::<SCROLLINFO>()as u32,fMask:SIF_ALL,..Default::default()};
        unsafe{GetScrollInfo(window,SB_VERT,&mut info);}
        if message==crate::viewport::WHEEL{self.program_scroll-=((w>>16)as u16 as i16 as i32/120)*72;}
        else{match (w&0xffff)as i32{
            SB_LINEUP=>self.program_scroll-=36,SB_LINEDOWN=>self.program_scroll+=36,
            SB_PAGEUP=>self.program_scroll-=info.nPage as i32,SB_PAGEDOWN=>self.program_scroll+=info.nPage as i32,
            SB_TOP=>self.program_scroll=0,SB_BOTTOM=>self.program_scroll=info.nMax,
            SB_THUMBTRACK|SB_THUMBPOSITION=>self.program_scroll=info.nTrackPos,_=>return,
        }}
        self.layout_program_details();
    }
    pub(super) fn reveal_program_control(&mut self,window:HWND){
        let Some(pane)=&self.program_viewport else{return;};
        if unsafe{GetParent(window)}!=pane.window{return;}
        let Some(palette)=&self.palette else{return;};
        let mut field=RECT::default();let mut viewport=RECT::default();
        unsafe{GetWindowRect(window,&mut field);GetClientRect(pane.window,&mut viewport);MapWindowPoints(pane.window,null_mut(),(&mut viewport as *mut RECT).cast(),2);}
        let offset=if field.top<viewport.top{field.top-viewport.top}else if field.bottom>viewport.bottom{field.bottom-viewport.bottom}else{return;};
        self.program_scroll+=(offset as f32/palette.scale).round()as i32;
        self.layout_program_details();
    }
    /// Rebuild only the selected program's fields. List selection, scroll and
    /// keyboard position survive tab/conditional-field changes.
    pub(super) fn refresh_program_details(&mut self,reset_scroll:bool)->Result<(),String>{
        let focused=unsafe{windows_sys::Win32::UI::Input::KeyboardAndMouse::GetFocus()};
        let focus_id=self.controls.iter().find(|c|c.window==focused&&matches!(c.area,Area::ProgramDetail)).map(|c|c.id);
        self.controls.retain(|c|!matches!(c.area,Area::ProgramDetail));
        self.bindings.retain(|b|b.program.is_none());
        if reset_scroll{self.program_scroll=0;}
        self.field_program=Some(self.selected_program.clone());
        let result=self.render_program_fields();self.field_program=None;result?;
        if let Some(order)=self.controls.iter().find(|c|c.id==452){
            let index=self.draft.as_ref().and_then(|d|d.value()["managedApps"].as_array()).and_then(|items|items.iter().position(|p|p["id"]==self.selected_program));
            if let Some(index)=index{unsafe{SendMessageW(order.window,CB_SETCURSEL,index,0);}}
        }
        self.refresh_buttons();self.layout();
        if let Some(window)=focus_id.and_then(|id|self.controls.iter().find(|c|c.id==id)).map(|c|c.window){
            unsafe{windows_sys::Win32::UI::Input::KeyboardAndMouse::SetFocus(window);}
            self.reveal_program_control(window);
        }
        Ok(())
    }
    pub(super) fn refresh_program_icons(&self){
        let selected=self.draft.as_ref().and_then(|draft|draft.value()["managedApps"].as_array())
            .and_then(|programs|programs.iter().find(|program|program["id"]==self.selected_program));
        let icon=selected.and_then(|program|program["exePath"].as_str()).and_then(|path|self.icons.get(path)).and_then(|icon|*icon);
        for control in &self.controls{
            if control.id==450{unsafe{InvalidateRect(control.window,null(),0);}}
            else if control.id==PROGRAM_ICON{unsafe{SendMessageW(control.window,STM_SETICON,icon.unwrap_or(null_mut())as usize,0);}}
        }
    }
    pub(super) fn program_list_item(&self, index: usize) -> (Option<HICON>, String) {
        let Some(program) = self.draft.as_ref().and_then(|draft| draft.value()["managedApps"].as_array())
            .and_then(|items| items.get(index)) else { return (None, String::new()); };
        let icon = program["exePath"].as_str().and_then(|path| self.icons.get(path).and_then(|icon| *icon));
        (icon, display_name(program))
    }
    pub(super) fn program_module_enabled(&self) -> bool {
        self.snapshot["modules"].as_array().is_some_and(|items| items.iter().any(|m| m["id"] == "wintray"
            && matches!(m["state"].as_str(), Some("ready" | "running" | "idle" | "stopped"))))
    }
    pub(super) fn field_value(&self, key: &str) -> Value {
        let Some(draft) = &self.draft else { return Value::Null; };
        if let Some(id) = &self.field_program { winone_desktop::programs::field(draft.value(), id, key) }
        else if let Some(key) = key.strip_prefix("startup.") { draft.value()["startup"][key].clone() }
        else { draft.value()[key].clone() }
    }
    pub(super) fn render_program_editor(&mut self) -> Result<(), String> {
        let programs = self.draft.as_ref().and_then(|d| d.value()["managedApps"].as_array()).cloned().unwrap_or_default();
        if !programs.iter().any(|p| p["id"] == self.selected_program) {
            self.selected_program = programs.first().and_then(|p| p["id"].as_str()).unwrap_or("").into();
        }
        // Owner-draw keeps the native listbox a11y and keyboard behavior while
        // each row shows the program's file icon next to its display name.
        let list = self.add("LISTBOX", "", 450, WS_TABSTOP | WS_VSCROLL | LBS_NOTIFY as u32
            | LBS_NOINTEGRALHEIGHT as u32 | LBS_OWNERDRAWFIXED as u32 | LBS_HASSTRINGS as u32,
            Area::ProgramList, [0,250,0,350], false)?;
        for program in &programs {
            unsafe { SendMessageW(list, LB_ADDSTRING, 0, wide(&display_name(program)).as_ptr() as isize); }
        }
        if let Some(index) = programs.iter().position(|p| p["id"] == self.selected_program) {
            unsafe { SendMessageW(list, LB_SETCURSEL, index, 0); }
            self.add("STATIC", &self.tr("排序序号", "Sort position"), 0, 0, Area::ProgramFooter(0), [0,0,128,30], false)?;
            let order = self.add("COMBOBOX", "", 452, WS_TABSTOP | CBS_DROPDOWNLIST as u32 | WS_VSCROLL,
                Area::ProgramFooter(1), [0,0,80,220], false)?;
            for position in 1..=programs.len() {
                unsafe { SendMessageW(order, CB_ADDSTRING, 0, wide(&position.to_string()).as_ptr() as isize); }
            }
            unsafe { SendMessageW(order, CB_SETCURSEL, index, 0); }
            widgets::enable(order, !self.saving());
        }
        self.actions(194)?;
        self.button(&self.tr("添加程序", "Add programs"), 356, Area::Content, [296,194,132,36])?;
        if !programs.is_empty(){
            self.button(&self.tr("更换", "Replace"), 355, Area::ProgramFooter(2), [0,0,0,36])?;
            self.button(&self.tr("移除", "Remove"), 354, Area::ProgramFooter(3), [0,0,0,36])?;
            self.button(&self.tr("启动", "Start"), 357, Area::ProgramFooter(4), [0,0,0,36])?;
        }
        self.program_viewport=Some(crate::viewport::Viewport::new(self.window,&self.tr("程序详情","Program details"))?);
        self.layout_program_workspace();
        if programs.is_empty(){
            self.add("STATIC",&self.tr("尚未添加程序","No programs added"),0,0,Area::ProgramDetail,[0,0,0,38],false)?;
            return Ok(());
        }
        self.field_program = Some(self.selected_program.clone());
        // Scope ends even when a control fails to create, so later pages cannot
        // accidentally bind global controls to the previously selected program.
        let result = self.render_program_fields();
        self.field_program = None;
        result
    }
    pub(super) fn apply_program_paths(&mut self, paths: &[String], replace: bool) -> Result<(),String> {
        if paths.is_empty() || self.saving() { return Ok(()); }
        let draft = self.draft.as_mut().ok_or("请先读取程序设置")?;
        let mut items = draft.value()["managedApps"].as_array().cloned().ok_or("程序列表格式无效")?;
        if replace {
            let program = items.iter_mut().find(|p| p["id"] == self.selected_program).ok_or("请先选择程序")?;
            *program = winone_desktop::programs::replaced(program,&paths[0]).map_err(|e| e.to_string())?;
        } else {
            for path in paths {
                let id = crate::dialogs::new_program_id()?;
                items.push(winone_desktop::programs::created(&id,path).map_err(|e| e.to_string())?);
                self.selected_program = id;
            }
        }
        draft.set("managedApps",Value::Array(items)).map_err(|e| e.to_string())?;
        self.request_program_icons();
        self.status = self.tr("有未保存的修改", "Unsaved changes");
        self.render()
    }
    pub(super) fn remove_selected_program(&mut self) -> Result<(),String> {
        if self.saving() { return Ok(()); }
        let draft = self.draft.as_mut().ok_or("请先读取程序设置")?;
        let mut items = draft.value()["managedApps"].as_array().cloned().ok_or("程序列表格式无效")?;
        items.retain(|p| p["id"] != self.selected_program);
        draft.set("managedApps",Value::Array(items)).map_err(|e| e.to_string())?;
        let prefix = format!("{}/",self.selected_program);
        self.invalid.retain(|key,_| !key.starts_with(&prefix));
        self.status = self.tr("有未保存的修改", "Unsaved changes");
        self.render()
    }
    fn render_program_fields(&mut self) -> Result<(), String> {
        // The detail column identifies the selected program above its settings.
        let path = self.field_value("exePath").as_str().unwrap_or("").to_string();
        let icon = self.icons.get(&path).and_then(|icon| *icon);
        let header = self.add("STATIC", "", PROGRAM_ICON, SS_ICON as u32, Area::ProgramDetail, [0,0,32,32], false)?;
        if let Some(icon) = icon { unsafe { SendMessageW(header, STM_SETICON, icon as usize, 0); } }
        let name = self.draft.as_ref().and_then(|draft| draft.value()["managedApps"].as_array())
            .and_then(|items| items.iter().find(|program| program["id"] == self.selected_program))
            .map(display_name).unwrap_or_default();
        self.add("STATIC", &name, 0, SS_LEFT as u32, Area::ProgramDetail, [44,0,0,36], false)?;
        self.add("STATIC", &path, 0, SS_ENDELLIPSIS as u32, Area::ProgramDetail, [44,42,0,30], false)?;
        self.button(&self.tr("常用设置", "General"), 350, Area::Content, [0,96,130,36])?;
        self.button(&self.tr("更多设置", "More settings"), 351, Area::Content, [136,96,150,36])?;
        if self.program_tab == 0 {
            self.check("runOnStartup", &self.tr("开机启动此程序", "Start this program at sign-in"), 148)?;
            self.check("collectTrayIcon", &self.tr("托盘收纳", "Collect tray icon"), 192)?;
            let executable = self.field_value("exePath").as_str().is_some_and(|p| p.to_ascii_lowercase().ends_with(".exe"));
            let modes = if executable { vec!["tray","hidden","task","normal"] } else { vec!["hidden"] };
            let labels = modes.iter().map(|m| match *m {
                "tray" => self.tr("收进托盘", "Minimize to tray"), "hidden" => self.tr("后台启动", "Start in background"),
                "task" => self.tr("开机即启动", "Start immediately at sign-in"), _ => self.tr("正常启动", "Start normally") }).collect::<Vec<_>>();
            self.choice("mode", &self.tr("启动方式", "Startup behavior"), 240, &modes, &labels)?;
            let mut y = 328;
            if self.field_value("mode") == "tray" {
                self.edit("trayBehavior.closeDelaySeconds", &self.tr("收起前等待（0–600 秒）", "Wait before minimizing (0–600 seconds)"), y, FieldKind::Number(0,600))?; y += 88;
            }
            self.check("schedule.enabled", &self.tr("自定义时间", "Custom schedule"), y)?; y += 54;
            if self.field_value("schedule.enabled") == true {
                self.edit("schedule.startDelayMinutes", &self.tr("延后启动（0–1440 分钟）", "Start delay (0–1440 minutes)"), y, FieldKind::Number(0,1440))?; y += 88;
                self.edit("schedule.autoExitMinutes", &self.tr("自动退出（分钟，0 为不退出）", "Stop after (minutes; 0 means never)"), y, FieldKind::Number(0,1440))?;
            }
        } else {
            self.edit("name", &self.tr("名称（必填）", "Name (required)"), 148, FieldKind::Text)?;
            self.edit("args", &self.tr("启动参数（可选）", "Startup arguments (optional)"), 236, FieldKind::Text)?;
            self.check("schedule.frequencyEnabled", &self.tr("限制自动运行次数", "Limit automatic launches"), 330)?;
            if self.field_value("schedule.enabled") == true && self.field_value("schedule.frequencyEnabled") == true {
                self.edit("schedule.frequencyDays", &self.tr("最近天数（1–365）", "Lookback period (1–365 days)"), 384, FieldKind::Number(1,365))?;
                self.edit("schedule.frequencyRuns", &self.tr("最多运行次数（1–1000）", "Maximum launches (1–1000)"), 472, FieldKind::Number(1,1000))?;
            }
        }
        Ok(())
    }
}
