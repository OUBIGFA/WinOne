//! Capture actions share the existing worker's native canvas. The settings
//! window only hides for acquisition and restores its own page afterwards.
use super::*;
use windows_sys::Win32::Graphics::Dwm::DwmFlush;

pub(super) const CAPTURE_FIRST: usize = 360;
pub(super) const CAPTURE_LAST: usize = 363;
pub(super) const PICK_OUTPUT: usize = 367;
pub(super) const COPY_LAST: usize = 370;
pub(super) const SAVE_LAST: usize = 371;
pub(super) const EDIT_LAST: usize = 372;
pub(super) const PIN_LAST: usize = 373;
pub(super) const CLOSE_PINS: usize = 374;
pub(super) const REFRESH_LAST: usize = 375;
const HISTORY_PATH:usize=376;
const NAME_PREVIEW:usize=377;
const NAME_REFRESH:usize=378;

#[derive(Default)]
pub(super) struct CaptureState {
    pub(super) busy: bool, pub(super) picking: bool, hidden: bool,
    data:Value, loaded:bool, refreshing:bool,
    filename_pending:Option<String>, filename_settled:Option<String>, filename_name:String, filename_error:String,
}

impl App {
    pub(super) fn render_filename(&mut self)->Result<(),String>{
        self.edit("fileNameTemplate",&self.tr("文件名模板","File name template"),194,FieldKind::Text)?;
        self.add("STATIC",&self.tr("留空使用默认文件名。可用：{timestamp}、{date}、{time}、{sequence}",
            "Leave empty for the default name. Available: {timestamp}, {date}, {time}, {sequence}"),0,0,Area::Content,[0,282,0,62],false)?;
        self.add("STATIC","",NAME_PREVIEW,0,Area::Content,[0,356,0,62],false)?;
        self.button(&self.tr("刷新预览","Refresh preview"),NAME_REFRESH,Area::Content,[0,432,150,36])?;
        self.actions(490)?;self.content_height=620;
        self.update_filename_preview();
        Ok(())
    }
    fn current_filename_template(&self)->String{self.field_value("fileNameTemplate").as_str().unwrap_or("").to_owned()}
    pub(super) fn update_filename_preview(&mut self){
        let template=self.current_filename_template();
        if self.capture.filename_pending.is_none() && self.capture.filename_settled.as_ref()!=Some(&template){
            self.capture.filename_name.clear();self.capture.filename_error.clear();
            match self.submit(Task::CaptureFileName{generation:self.generation,template:template.clone()}){
                Ok(())=>self.capture.filename_pending=Some(template),
                Err(error)=>{self.capture.filename_settled=Some(template);self.capture.filename_error=error;},
            }
        }
        self.refresh_filename_preview();
    }
    fn refresh_filename_preview(&self){
        let message=if self.capture.filename_pending.is_some(){self.tr("正在预览文件名…","Previewing file name…")}
            else if !self.capture.filename_error.is_empty(){self.capture.filename_error.clone()}
            else if !self.capture.filename_name.is_empty(){format!("{}: {}",self.tr("示例","Example"),self.capture.filename_name)}
            else{String::new()};
        if let Some(control)=self.controls.iter().find(|c|c.id==NAME_PREVIEW){widgets::set_text(control.window,&message);}
        if let Some(control)=self.controls.iter().find(|c|c.id==NAME_REFRESH){widgets::enable(control.window,!self.saving()&&self.capture.filename_pending.is_none());}
    }
    fn capture_enabled(&self) -> bool {
        self.snapshot["modules"].as_array().is_some_and(|modules| modules.iter().any(|m|
            m["id"]=="capture" && matches!(m["state"].as_str(),Some("ready"|"running"|"idle"|"stopped"))))
    }
    pub(super) fn render_capture_actions(&mut self) -> Result<(),String> {
        let y=self.content_height-56;
        self.label(&self.tr("截图操作","Capture actions"),y,true)?;
        let names=[self.tr("区域截图","Region"),self.tr("窗口截图","Window"),self.tr("全屏截图","Full screen"),self.tr("滚动截图","Scrolling")];
        for (index,name) in names.iter().enumerate() {
            self.button(name,CAPTURE_FIRST+index,Area::Tabs {index,count:names.len()},[0,y+44,0,36])?;
        }
        let mut next=y+98;
        self.label(&self.tr("最近保存的截图","Last saved capture"),next,false)?;
        next+=42;
        let path=self.capture.data["lastCapture"]["path"].as_str();
        let text=if self.capture.refreshing {self.tr("正在读取…","Loading…")}
            else if let Some(path)=path {path.into()}
            else if self.capture.loaded {self.tr("暂无已保存的截图","No saved capture")}
            else {self.tr("点击刷新查看最近截图","Refresh to view the last capture")};
        self.add("STATIC",&text,HISTORY_PATH,SS_ENDELLIPSIS as u32,Area::Content,[0,next,0,38],false)?;
        next+=44;
        let actions=[(COPY_LAST,self.tr("复制","Copy")),(SAVE_LAST,self.tr("另存为","Save as")),(EDIT_LAST,self.tr("编辑","Edit")),(PIN_LAST,self.tr("贴图","Pin")),(REFRESH_LAST,self.tr("刷新","Refresh"))];
        for (index,(id,label)) in actions.iter().enumerate() {self.button(label,*id,Area::Tabs {index,count:actions.len()},[0,next,0,36])?;}
        self.button(&self.tr("关闭全部贴图","Close pinned images"),CLOSE_PINS,Area::Content,[0,next+48,220,36])?;
        self.content_height=next+170;
        Ok(())
    }
    fn last_capture_id(&self)->Option<&str> {
        self.capture.data["lastCapture"]["id"].as_str().filter(|id|id.len()==32&&id.bytes().all(|c|c.is_ascii_digit()||(b'a'..=b'f').contains(&c)))
    }
    fn refresh_capture_history(&mut self)->Result<(),String> {
        if !self.capture.refreshing {
            self.submit(Task::CaptureSnapshot {generation:self.generation})?;
            self.capture.refreshing=true;
        }
        Ok(())
    }
    pub(super) fn refresh_capture_buttons(&self) {
        self.refresh_filename_preview();
        if self.navigation.page()=="capture" {
            if let Some(binding)=self.bindings.iter().find(|b|b.key=="askWhereToSave") {
                if let Some(control)=self.controls.iter().find(|c|c.id==binding.id){
                    widgets::enable(control.window,!self.saving()&&!self.loading&&self.field_value("openEditor")==true);
                }
            }
        }
        for control in &self.controls {
            if (CAPTURE_FIRST..=CAPTURE_LAST).contains(&control.id) {
                widgets::enable(control.window,!self.saving() && !self.loading && !self.any_dirty() && self.capture_enabled());
            } else if control.id==PICK_OUTPUT { widgets::enable(control.window,!self.saving() && !self.loading); }
            else if (COPY_LAST..=REFRESH_LAST).contains(&control.id) {
                let valid=match control.id {
                    REFRESH_LAST=>!self.capture.refreshing,
                    // This explicit command closes the module's pins; a local
                    // history read does not claim to know another process's UI.
                    CLOSE_PINS=>true,
                    _=>self.last_capture_id().is_some() && (control.id!=EDIT_LAST || !self.any_dirty()),
                };
                widgets::enable(control.window,!self.saving()&&(control.id==REFRESH_LAST||self.capture_enabled())&&valid);
            }
        }
    }
    pub(super) fn capture_command(&mut self,id:usize) -> Result<bool,String> {
        if self.navigation.page()!="capture" { return Ok(false); }
        if id==NAME_REFRESH {
            if !self.saving() && self.capture.filename_pending.is_none(){self.capture.filename_settled=None;self.update_filename_preview();}
            return Ok(true);
        }
        if id==REFRESH_LAST {
            if !self.saving() {self.refresh_capture_history()?;self.render()?;}
            return Ok(true);
        }
        if (COPY_LAST..=CLOSE_PINS).contains(&id) {self.last_capture_action(id)?;return Ok(true);}
        if id==PICK_OUTPUT {
            if self.saving() || self.loading { return Ok(true); }
            self.capture.picking=true; self.refresh_buttons();
            let selected=crate::dialogs::pick_folder(self.window,&self.tr("选择保存文件夹","Choose save folder"));
            self.capture.picking=false;
            match selected {
                Ok(Some(path)) => {
                    if let Some(draft)=&mut self.draft { draft.set("outputDirectory",json!(path)).map_err(|e|e.to_string())?; }
                    self.status=self.tr("有未保存的修改","Unsaved changes");
                }
                Ok(None)=>{},
                Err(error)=>self.status=error,
            }
            self.render()?;
            return Ok(true);
        }
        if !(CAPTURE_FIRST..=CAPTURE_LAST).contains(&id) { return Ok(false); }
        if self.saving() || self.loading || self.any_dirty() || !self.capture_enabled() { return Ok(true); }
        let mode=["region","window","fullscreen","scroll"][id-CAPTURE_FIRST];
        let language=self.snapshot["locale"].as_str().unwrap_or("zh-CN").to_string();
        self.capture.hidden=unsafe { IsWindowVisible(self.window) }!=0;
        if self.capture.hidden {
            unsafe { ShowWindow(self.window,SW_HIDE); }
            // Wait for the compositor's completed frame rather than guessing a
            // sleep duration and accidentally capturing the settings window.
            if unsafe { DwmFlush() }<0 {
                self.restore_after_capture();
                return Err(self.tr("无法准备截图画面，请重试。","Unable to prepare the screen. Retry."));
            }
        }
        if let Err(error)=self.submit(Task::Capture {mode:mode.into(),language}) { self.restore_after_capture(); return Err(error); }
        self.capture.busy=true;
        self.status=self.tr("正在截图…","Capturing…");
        self.refresh_buttons();
        Ok(true)
    }
    fn restore_after_capture(&mut self) {
        if self.capture.hidden { unsafe { ShowWindow(self.window,SW_SHOWNOACTIVATE); } }
        self.capture.hidden=false;
    }
    fn last_capture_action(&mut self,id:usize)->Result<(),String> {
        if self.saving()||!self.capture_enabled() {return Ok(());}
        if id==EDIT_LAST&&self.any_dirty() {return Ok(());}
        let mut params=json!({"language":self.snapshot["locale"].as_str().unwrap_or("zh-CN")});
        if id!=CLOSE_PINS {
            let Some(identity)=self.last_capture_id() else{return Ok(());};
            params["expectedId"]=json!(identity);
        }
        let method=match id {COPY_LAST=>"copy.last",SAVE_LAST=>"save.last",EDIT_LAST=>"edit.last",PIN_LAST=>"pin.last",CLOSE_PINS=>"pins.close",_=>return Ok(())};
        if id==SAVE_LAST {
            let suggested=self.capture.data["lastCapture"]["path"].as_str().and_then(|s|std::path::Path::new(s).file_name()).and_then(|s|s.to_str()).unwrap_or("WinOne.png").to_string();
            self.capture.picking=true;self.refresh_buttons();
            let path=crate::dialogs::save_png(self.window,&self.tr("另存截图","Save capture as"),&suggested);
            self.capture.picking=false;self.refresh_buttons();
            let Some(path)=path? else{return Ok(());};params["path"]=json!(path);
        }
        if id==EDIT_LAST {
            self.capture.hidden=unsafe{IsWindowVisible(self.window)}!=0;
            if self.capture.hidden {unsafe{ShowWindow(self.window,SW_HIDE);}}
        }
        if let Err(error)=self.submit(Task::CaptureAction {method:method.into(),params}) {self.restore_after_capture();return Err(error);}
        self.capture.busy=true;self.status=self.tr("正在处理截图…","Working on the capture…");self.refresh_buttons();
        Ok(())
    }
    pub(super) fn receive_capture(&mut self,reply:&worker::Reply) -> Result<bool,String> {
        if let Task::CaptureFileName{generation,template}=&reply.task {
            if self.navigation.page()!="capture" || *generation!=self.generation{return Ok(true);}
            self.capture.filename_pending=None;
            if *template==self.current_filename_template(){
                self.capture.filename_settled=Some(template.clone());
                match &reply.result{
                    Ok(value) if value["name"].as_str().is_some_and(|name|!name.trim().is_empty())=>{
                        self.capture.filename_name=value["name"].as_str().unwrap().to_owned();self.capture.filename_error.clear();
                    },
                    Ok(_)=>self.capture.filename_error=self.tr("无法预览文件名，请重试。","Cannot preview the file name. Retry."),
                    Err(error)=>self.capture.filename_error=error.to_string(),
                }
            }
            // Complete at most one request, then use the latest draft. Updating
            // just the preview preserves the input HWND, caret and composition.
            self.update_filename_preview();return Ok(true);
        }
        if let Task::CaptureSnapshot {generation}=&reply.task {
            if self.navigation.page()!="capture"||*generation!=self.generation{return Ok(true);}
            self.capture.refreshing=false;
            match &reply.result {
                Ok(value) if value.is_object()=>{self.capture.data=value.clone();self.capture.loaded=true;},
                Ok(_)=>self.status=self.tr("无法读取截图信息，请重试。","Cannot read capture information. Retry."),
                Err(error)=>self.status=error.to_string(),
            }
            self.render()?;return Ok(true);
        }
        if !matches!(reply.task,Task::Capture {..}|Task::CaptureAction {..}) { return Ok(false); }
        self.capture.busy=false;
        self.restore_after_capture();
        self.status=match &reply.result {
            Err(error) if error.code=="cancelled"=>self.tr("已取消截图","Capture cancelled"),
            Err(error)=>error.to_string(),
            Ok(value) if value["path"].is_string()=>self.tr("已保存：{path}","Saved: {path}").replace("{path}",value["path"].as_str().unwrap()),
            Ok(value) if value["copied"]==true=>self.tr("已复制到剪贴板","Copied to clipboard"),
            Ok(value) if value["pinned"]==true=>self.tr("已贴图","Pinned to screen"),
            Ok(value) if value["closed"]==true=>self.tr("贴图已关闭","Pinned images closed"),
            Ok(_)=>self.tr("无法确认截图结果，请检查截图模块。","Cannot confirm the capture result. Check the capture module."),
        };
        if let Err(error)=self.refresh_capture_history() {self.status.push_str(&format!("\n{error}"));}
        self.render()?;
        Ok(true)
    }
}
