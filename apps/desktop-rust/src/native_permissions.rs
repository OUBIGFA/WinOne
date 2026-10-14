//! Global permission controls share the current settings transaction and close guard.
use super::*;
use crate::permissions::Action;

const REMEMBER:usize=392;
const RESTART:usize=393;
#[derive(Default)]
pub(super) struct PermissionState{action:Option<Action>,error:String}

impl App{
    pub(super) fn permission_busy(&self)->bool{self.permission.action.is_some()}
    pub(super) fn permission_guard(&self)->bool{
        self.permission_busy() && !self.loading && !self.any_dirty() && !self.recording()
            && !self.dialog_open && !self.capture.busy && !self.launching && !self.profiles.busy
    }
    fn can_change_permission(&self)->bool{
        self.navigation.page()=="settings" && self.tab()==1 && !self.loading && !self.saving() && !self.any_dirty()
            && self.snapshot["administrator"].is_boolean()
    }
    pub(super) fn render_permissions(&mut self)->Result<(),String>{
        let administrator=self.snapshot["administrator"].as_bool();
        let remembered=self.snapshot["settings"]["startup"]["preferAdministrator"]==true;
        self.label(&self.tr("运行权限","Permissions"),374,true)?;
        self.label(&match administrator{
            Some(true)=>self.tr("当前以管理员权限运行","Running as administrator"),
            Some(false)=>self.tr("当前以普通权限运行","Running with standard permissions"),
            None=>self.tr("正在读取权限状态…","Loading permission status…"),
        },420,false)?;
        let checkbox=self.add("BUTTON",&self.tr("始终以管理员身份运行","Always run as administrator"),REMEMBER,
            WS_TABSTOP|BS_AUTOCHECKBOX as u32,Area::Content,[0,462,0,34],false)?;
        unsafe{SendMessageW(checkbox,BM_SETCHECK,if remembered{BST_CHECKED}else{BST_UNCHECKED} as usize,0);}
        self.label(&self.tr("首次启用需要 Windows 授权。","Enabling this for the first time requires Windows consent."),506,false)?;
        let hint=if remembered && administrator==Some(false){self.tr("下次启动时使用管理员权限。","Administrator permissions will apply at the next start.")}
            else if !remembered && administrator==Some(true){self.tr("当前管理员权限保留至退出。","Current administrator permissions remain until exit.")}else{String::new()};
        self.label(&hint,550,false)?;
        self.button(&self.tr("以管理员身份重新启动","Restart as administrator"),RESTART,Area::Content,[0,596,292,36])?;
        self.add("STATIC",&self.permission.error.clone(),0,0,Area::Content,[0,648,0,62],false)?;
        self.content_height=794;
        Ok(())
    }
    pub(super) fn refresh_permission_buttons(&self){
        for control in &self.controls{
            if control.id==REMEMBER || control.id==RESTART{
                widgets::enable(control.window,self.can_change_permission() && (control.id!=RESTART || self.snapshot["administrator"]==false));
            }
        }
    }
    pub(super) fn permission_command(&mut self,id:usize,window:HWND)->Result<bool,String>{
        if id!=REMEMBER && id!=RESTART{return Ok(false);}
        if !self.can_change_permission(){return Ok(true);}
        let action=if id==REMEMBER{Action::Preference(unsafe{SendMessageW(window,BM_GETCHECK,0,0)}==BST_CHECKED as isize)}else{
            if self.snapshot["administrator"]!=false{return Ok(true);}Action::Restart
        };
        match self.submit(Task::Permission{action}){
            Ok(())=>{self.permission.action=Some(action);self.permission.error.clear();self.status=self.tr("正在更新运行权限…","Updating permissions…");},
            Err(error)=>self.permission.error=error,
        }
        self.render()?;Ok(true)
    }
    pub(super) fn receive_permission(&mut self,reply:&worker::Reply)->Result<bool,String>{
        let Task::Permission{action}=&reply.task else{return Ok(false);};
        if self.permission.action!=Some(*action){return Ok(true);}
        if self.any_dirty(){
            self.permission.action=None;
            self.permission.error=self.tr("已保留未保存的修改，请保存后刷新权限状态。","Unsaved edits were kept. Save them before refreshing permissions.");
            self.render()?;return Ok(true);
        }
        if let Ok(value)=&reply.result{
            if *action==Action::Restart && value["closing"]==true{
                if self.permission_guard() && value["token"].as_str().is_some_and(|s|s.len()==32&&s.bytes().all(|c|c.is_ascii_hexdigit())){
                    if unsafe{DestroyWindow(self.window)}!=0{return Ok(true);}
                }
                self.permission.error=self.tr("未能关闭当前窗口，重启尚未完成。","The current window could not close. Restart is incomplete.");
            }else if value["closing"]==false && value["settings"].is_object() && value["settings"]["revision"].is_u64()
                && match action {Action::Preference(enabled)=>value["settings"]["startup"]["preferAdministrator"]==*enabled,Action::Restart=>true}{
                self.snapshot["settings"]=value["settings"].clone();
                self.draft=Some(Draft::new(value["settings"].clone()).map_err(|e|e.to_string())?);
                self.permission.error.clear();
            }else{self.permission.error=self.tr("无法确认权限操作结果，请重新读取设置。","Cannot confirm permissions. Reload settings.");}
        }else if let Err(error)=&reply.result{
            self.permission.error=if error.code=="cancelled"{self.tr("已取消权限操作","Permission change cancelled")}else{error.to_string()};
        }
        self.permission.action=None;self.status.clear();
        // Even failed/uncertain consent can race an external settings change.
        // Reconcile the Host snapshot without erasing the operation feedback.
        match self.submit(Task::Load{page:"settings".into(),generation:self.generation}){
            Ok(())=>self.loading=true,
            Err(error)=>self.permission.error.push_str(&format!("\n{error}")),
        }
        self.render()?;Ok(true)
    }
}
