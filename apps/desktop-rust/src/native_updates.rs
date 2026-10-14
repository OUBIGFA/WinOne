//! Update checks have their own cancellable state and never save settings.
use super::*;
pub(super) const UPDATE_TIMER:usize=901;
const CHECK:usize=410;
const CANCEL:usize=411;
const RELEASE:usize=412;
const REFRESH:usize=413;

#[derive(Default)]
pub(super) struct UpdateState{
    info:Option<Value>,info_requested:bool,info_loading:bool,
    id:Option<String>,checking:bool,polling:bool,cancelling:bool,opening:bool,
    result:Option<Value>,error:String,cancel_pending:Option<String>,
}
impl App{
    pub(super) fn showing_updates(&self)->bool{self.navigation.page()=="settings"&&self.tab()==4}
    fn load_update_info(&mut self){
        if self.updates.info_loading{return;}
        self.updates.info_requested=true;self.updates.error.clear();
        match self.submit(Task::UpdateInfo{generation:self.generation}){
            Ok(())=>self.updates.info_loading=true,
            Err(error)=>self.updates.error=error,
        }
    }
    pub(super) fn render_updates(&mut self)->Result<(),String>{
        if !self.updates.info_requested{self.load_update_info();}
        let version=self.updates.info.as_ref().and_then(|v|v["version"].as_str()).unwrap_or(env!("CARGO_PKG_VERSION"));
        let version=if version=="development"{self.tr("开发版本","Development build")}else{version.to_owned()};
        self.label(&format!("{}: {version}",self.tr("当前版本","Current version")),194,false)?;
        self.button(&self.tr("检查更新","Check for updates"),CHECK,Area::Content,[0,248,158,36])?;
        self.button(&self.tr("取消检查","Cancel check"),CANCEL,Area::Content,[170,248,140,36])?;
        self.button(&self.tr("重新读取","Reload"),REFRESH,Area::Content,[322,248,130,36])?;
        let message=if !self.updates.error.is_empty(){self.updates.error.clone()}
            else if self.updates.checking{self.tr(if self.updates.cancelling{"正在取消检查…"}else{"正在检查更新…"},if self.updates.cancelling{"Cancelling update check…"}else{"Checking for updates…"})}
            else if self.updates.info_loading{self.tr("正在读取更新信息…","Loading update information…")}
            else if let Some(result)=&self.updates.result{match result["state"].as_str(){
                Some("available")=>format!("{} {}",self.tr("发现新版本","New version available:"),result["latestVersion"].as_str().unwrap_or("")),
                Some("up_to_date")=>self.tr("当前已是最新版本。","You are up to date."),
                Some("not_configured")=>self.tr("此版本未配置更新来源。","This build has no update source configured."),
                _=>String::new(),
            }}else if self.updates.info.as_ref().is_some_and(|v|v["configured"]==false){self.tr("此版本未配置更新来源。","This build has no update source configured.")}
            else{String::new()};
        self.add("STATIC",&message,0,0,Area::Content,[0,308,0,78],false)?;
        if self.updates.result.as_ref().is_some_and(|v|v["state"]=="available"){
            self.button(&self.tr("打开发布页","Open release page"),RELEASE,Area::Content,[0,398,220,36])?;
        }
        self.content_height=560;Ok(())
    }
    pub(super) fn refresh_update_buttons(&self){
        for control in &self.controls{
            let enabled=match control.id{
                CHECK=>!self.saving()&&!self.updates.checking&&!self.updates.info_loading&&self.updates.cancel_pending.is_none()&&self.updates.info.as_ref().is_some_and(|v|v["configured"]==true),
                CANCEL=>self.updates.checking&&!self.updates.cancelling,
                RELEASE=>!self.updates.checking&&!self.updates.cancelling&&!self.updates.opening&&self.updates.id.is_some(),
                REFRESH=>!self.saving()&&!self.updates.info_loading&&!self.updates.checking,
                _=>continue,
            };widgets::enable(control.window,enabled);
        }
    }
    pub(super) fn flush_update_cancel(&mut self){
        if let Some(id)=self.updates.cancel_pending.clone(){
            if self.submit(Task::UpdateCancel{id}).is_ok(){self.updates.cancel_pending=None;}
        }
    }
    pub(super) fn leave_updates(&mut self){
        unsafe{KillTimer(self.window,UPDATE_TIMER);}
        if self.updates.checking{self.updates.cancel_pending=self.updates.id.take();}
        self.updates.id=None;self.updates.checking=false;self.updates.polling=false;self.updates.cancelling=false;self.updates.opening=false;
        self.updates.result=None;self.updates.error.clear();
        self.updates.info_loading=false;
        if self.updates.info.is_none(){self.updates.info_requested=false;}
        self.flush_update_cancel();
    }
    fn update_failed(&mut self,error:String){
        if self.updates.checking{self.updates.cancel_pending=self.updates.id.take();}
        self.updates.id=None;self.updates.checking=false;self.updates.polling=false;self.updates.cancelling=false;self.updates.opening=false;
        self.updates.result=None;self.updates.error=error;
        unsafe{KillTimer(self.window,UPDATE_TIMER);}
        self.flush_update_cancel();
    }
    pub(super) fn update_tick(&mut self){
        self.flush_update_cancel();
        if !self.showing_updates()||!self.updates.checking||self.updates.polling{return;}
        let Some(id)=self.updates.id.clone()else{return;};
        if self.submit(Task::UpdateStatus{id}).is_ok(){self.updates.polling=true;}
    }
    pub(super) fn updates_command(&mut self,id:usize)->Result<bool,String>{
        if !self.showing_updates()||!matches!(id,CHECK|CANCEL|RELEASE|REFRESH){return Ok(false);}
        match id{
            CHECK=>{
                if self.saving()||self.updates.checking||self.updates.info_loading||self.updates.cancel_pending.is_some()||!self.updates.info.as_ref().is_some_and(|v|v["configured"]==true){return Ok(true);}
                let id=crate::dialogs::new_program_id()?;
                self.submit(Task::UpdateCheck{id:id.clone()})?;
                self.updates.id=Some(id);self.updates.checking=true;self.updates.cancelling=false;self.updates.result=None;self.updates.error.clear();
                if unsafe{SetTimer(self.window,UPDATE_TIMER,250,None)}==0{self.update_failed(self.tr("无法等待更新检查结果，请重试。","Cannot wait for the update result. Retry."));}
            },
            CANCEL=>{
                if self.updates.checking&&!self.updates.cancelling{
                    let id=self.updates.id.clone().ok_or("更新检查标识无效")?;
                    match self.submit(Task::UpdateCancel{id}){
                        Ok(())=>{self.updates.cancelling=true;self.updates.result=None;self.updates.error.clear();},
                        Err(error)=>self.updates.error=error,
                    }
                }
            },
            RELEASE=>{
                if self.updates.opening||self.updates.checking||self.updates.cancelling||!self.updates.result.as_ref().is_some_and(|v|v["state"]=="available"){return Ok(true);}
                let id=self.updates.id.clone().ok_or("更新检查标识无效")?;
                self.submit(Task::UpdateRelease{id})?;self.updates.opening=true;self.updates.error.clear();
            },
            REFRESH=>{if !self.saving()&&!self.updates.checking{self.load_update_info();}},
            _=>{},
        }
        self.render()?;Ok(true)
    }
    pub(super) fn receive_updates(&mut self,reply:&worker::Reply)->Result<bool,String>{
        if let Task::UpdateInfo{generation}=&reply.task{
            if *generation!=self.generation{return Ok(true);}
            self.updates.info_requested=true;
            self.updates.info_loading=false;
            match &reply.result{
                Ok(value) if value["version"].as_str().is_some_and(|v|!v.is_empty())&&value["configured"].is_boolean()=>self.updates.info=Some(value.clone()),
                _=>self.updates.error=self.update_error("unavailable"),
            }
            if self.showing_updates(){self.render()?;}return Ok(true);
        }
        let id=match &reply.task{
            Task::UpdateCheck{id}|Task::UpdateStatus{id}|Task::UpdateCancel{id}|Task::UpdateRelease{id}=>id,
            _=>return Ok(false),
        };
        if self.updates.id.as_ref()!=Some(id){return Ok(true);}
        // A terminal result is final for this request. A queued status reply
        // cannot resurrect a cancelled/failed check or its release action.
        if !self.updates.checking && !matches!(reply.task,Task::UpdateRelease{..}){return Ok(true);}
        if matches!(reply.task,Task::UpdateStatus{..}){self.updates.polling=false;}
        if matches!(reply.task,Task::UpdateRelease{..}){
            self.updates.opening=false;
            match &reply.result{
                Ok(value) if self.showing_updates()=>{
                    let checked=self.updates.result.as_ref().and_then(|result|result["releaseUrl"].as_str());
                    let result=value["url"].as_str().filter(|url|Some(*url)==checked).ok_or_else(||self.update_error("invalid_response"))
                        .and_then(|url|crate::dialogs::open_update_release(self.window,url));
                    if result.is_err(){self.updates.error=self.update_error("open_failed");}
                },
                _=>self.updates.error=self.update_error("open_failed"),
            }
        }else{
            match &reply.result{
                Ok(value) if value["id"].as_str()==Some(id)=>match value["state"].as_str(){
                    Some("checking")=>{self.refresh_update_buttons();return Ok(true);},
                    Some("failed")=>{
                        let error=self.update_error(value["errorCode"].as_str().unwrap_or("unavailable"));
                        self.updates.checking=false;self.updates.cancelling=false;self.updates.result=None;self.updates.error=error;
                        unsafe{KillTimer(self.window,UPDATE_TIMER);}
                    },
                    Some("available"|"up_to_date"|"not_configured") if !self.updates.cancelling=>{
                        let version=self.updates.info.as_ref().and_then(|info|info["version"].as_str());
                        let valid_version=version.is_some()&&value["currentVersion"].as_str()==version;
                        let valid_latest=value["state"]=="not_configured"||value["latestVersion"].as_str().is_some_and(|v|!v.is_empty());
                        let valid_link=value["state"]!="available"||value["releaseUrl"].as_str().is_some_and(crate::dialogs::valid_release_url);
                        if !valid_version||!valid_latest||!valid_link{
                            self.update_failed(self.update_error("invalid_response"));
                        }else{
                            self.updates.checking=false;self.updates.result=Some(value.clone());self.updates.error.clear();
                            unsafe{KillTimer(self.window,UPDATE_TIMER);}
                        }
                    },
                    Some("available"|"up_to_date"|"not_configured")=>{}, // Cancellation is still awaiting its authoritative reply.
                    _=>self.update_failed(self.update_error("invalid_response")),
                },
                _=>self.update_failed(self.update_error("unavailable")),
            }
        }
        if self.showing_updates(){self.render()?;}Ok(true)
    }
    fn update_error(&self,code:&str)->String{
        match code{
            "cancelled"=>self.tr("已取消检查","Update check cancelled"),
            "network"=>self.tr("无法连接更新服务，请检查网络后重试。","Cannot reach the update service. Check your connection and retry."),
            "timeout"=>self.tr("更新检查超时，请重试。","The update check timed out. Retry."),
            "rate_limited"=>self.tr("检查过于频繁，请稍后重试。","Too many checks. Try again later."),
            "not_found"=>self.tr("暂未找到已发布的版本。","No published version was found."),
            "invalid_response"=>self.tr("更新信息无效，请稍后重试。","Invalid update information. Try again later."),
            "invalid_version"=>self.tr("当前版本信息无效，请修复安装。","Invalid current version. Repair the installation."),
            "invalid_source"=>self.tr("更新来源无效，请修复安装。","Invalid update source. Repair the installation."),
            "open_failed"=>self.tr("无法打开发布页，请重试。","Cannot open the release page. Retry."),
            _=>self.tr("更新检查暂不可用，请重试。","Update checks are unavailable. Retry."),
        }
    }
}
