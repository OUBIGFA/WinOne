import i18next from "i18next";
import { initReactI18next } from "react-i18next";
import { captureMessages } from "./locales/capture";
import { alwaysOnTopMessages } from "./locales/always-on-top";
import { shortcutMessages } from "./locales/shortcuts";
import { wintrayMessages } from "./locales/wintray";
import { profileMessages, profileFieldMessages } from "./locales/profiles";
import {diagnosticsMessages} from "./locales/diagnostics";
import {updateMessages} from "./locales/updates";
export { useTranslation } from "react-i18next";

const zh={
  updates:updateMessages.zh,
  permission:{title:"运行权限",loading:"正在读取权限",administrator:"当前以管理员身份运行",standard:"当前以普通权限运行",remember:"始终以管理员身份运行",consent:"首次启用需要管理员授权。",nextStart:"重新启动 WinOne 后生效。",untilExit:"从托盘退出后，下次启动将使用普通权限。",restart:"以管理员身份重新启动",restarting:"正在重新启动"},
  startup:{title:"启动",logon:"登录 Windows 时启动",logonHint:"登录后自动运行 WinOne，无需管理员权限。",silent:"静默启动到托盘",silentHint:"自动启动时不打开设置窗口。"},
  dashboard:{resizePanels:"调整工具与概览宽度",permission:"运行权限",administrator:"管理员",standard:"普通权限",toolsEnabled:"已开启工具",statusHint:"按需开启，关闭后保留设置。",attention:"{{count}} 个工具需要处理",shortcuts:"当前快捷键",noShortcuts:"暂无已启用的快捷键",manageTools:"工具管理"},
  window:{minimize:"最小化窗口",maximize:"最大化窗口",restore:"还原窗口",close:"关闭窗口"},
  navigation:{closeDescription:"关闭窗口会放弃这些更改。",discardClose:"放弃并关闭",waitCloseDescription:"操作完成后将关闭窗口；失败时保留当前更改。",dirtyTitle:"有未保存的更改",dirtyDescription:"离开页面会放弃这些更改。",keepEditing:"继续编辑",discard:"放弃并离开",waitTitle:"操作尚未完成",waitDescription:"操作完成后将切换页面；失败时保留当前更改。",stay:"留在此页"},
  diagnostics:diagnosticsMessages.zh,
  capture:captureMessages.zh,
  aot:alwaysOnTopMessages.zh,
  shortcuts:shortcutMessages.zh,
  wintray:wintrayMessages.zh,
  profiles:profileMessages.zh,profileFields:profileFieldMessages.zh,
  nav:{overview:"首页",capture:"截图","always-on-top":"置顶功能",wintray:"程序管理",modules:"模块",settings:"设置",label:"功能导航"},
  common:{checkFields:"请检查：{{fields}}",integerMinimum:"请输入不小于 {{min}} 的整数",numberMinimum:"请输入不小于 {{min}} 的数值",decimalRange:"请输入 {{min}}–{{max}} 的数值",numberRange:"请输入 {{min}}–{{max}} 的整数",refresh:"刷新状态",failed:"操作未完成",reconnect:"重试连接",saved:"已保存",open:"打开",back:"返回概览",waiting:"等待后台连接",retry:"重试"},
  shell:{repositoryExample:"GitHub 仓库（示例）",general:"常规",preview:"开发预览",tagline:"你的 Windows 工具集",intro:"常用工具，一个入口",appearance:"外观",theme:"主题",themeHint:"选择浅色、深色或跟随 Windows",system:"跟随系统",light:"浅色",dark:"深色",language:"语言",languageHint:"选择界面语言",background:"后台",connection:"连接状态",backgroundHint:"关闭设置窗口后，后台继续运行",connected:"已连接",disconnected:"未连接",unavailable:"组件未就绪",unavailableHint:"组件通过验证后，将在此提供设置与操作"},
  module:{unknownResult:"未能确认模块操作结果",panelSections:"模块页面",panel:{capture:"截图","always-on-top":"置顶设置",wintray:"程序"},intro:"选择需要的工具，关闭后保留设置。",loading:"正在读取模块",updated:"正在更新状态",failed:"模块状态未能更新",enabled:"{{name}}已开启",disabled:"{{name}}已关闭",retryClose:"继续关闭",retryRecovery:"重试恢复配置",recovered:"配置已恢复",recoveryFailed:"配置恢复未完成",disabledTitle:"{{name}}已关闭",retained:"设置和数据已保留。",manage:"管理模块"},
  moduleDescription:{capture:"截图、标注与截图置顶","always-on-top":"让选定窗口保持在前面",wintray:"管理程序启动和托盘图标"},
  moduleStatus:{incomplete:"关闭未完成",faulted:"需要处理",unavailable:"组件不可用",working:"正在处理",enabled:"已开启",disabled:"已关闭"},
};
const en={
  updates:updateMessages.en,
  permission:{title:"Permissions",loading:"Checking permissions",administrator:"Currently running as administrator",standard:"Currently running with standard permissions",remember:"Always run as administrator",consent:"Administrator approval is required the first time.",nextStart:"Restart WinOne to apply.",untilExit:"This session stays elevated. Exit WinOne from the tray to use standard permissions on the next launch.",restart:"Restart as administrator",restarting:"Restarting"},
  startup:{title:"Startup",logon:"Start when signing in to Windows",logonHint:"Run WinOne automatically at sign-in. No administrator access required.",silent:"Start silently in the tray",silentHint:"Keep the settings window closed during automatic startup."},
  dashboard:{resizePanels:"Resize tools and overview",permission:"Running as",administrator:"Administrator",standard:"Standard user",toolsEnabled:"Enabled tools",statusHint:"Enable what you need. Settings are kept when disabled.",attention_one:"1 tool needs attention",attention_other:"{{count}} tools need attention",shortcuts:"Current shortcuts",noShortcuts:"No active shortcuts",manageTools:"Manage tools"},
  window:{minimize:"Minimize window",maximize:"Maximize window",restore:"Restore window",close:"Close window"},
  navigation:{closeDescription:"Closing this window will discard your changes.",discardClose:"Discard and close",waitCloseDescription:"The window will close when the action completes. If it fails, your changes stay.",dirtyTitle:"Unsaved changes",dirtyDescription:"Leaving this page will discard your changes.",keepEditing:"Keep editing",discard:"Discard and leave",waitTitle:"Action in progress",waitDescription:"The page will change when the action completes. If it fails, your changes stay.",stay:"Stay on this page"},
  diagnostics:diagnosticsMessages.en,
  capture:captureMessages.en,
  aot:alwaysOnTopMessages.en,
  shortcuts:shortcutMessages.en,
  wintray:wintrayMessages.en,
  profiles:profileMessages.en,profileFields:profileFieldMessages.en,
  nav:{overview:"Home",capture:"Capture","always-on-top":"Always on Top",wintray:"Program management",modules:"Modules",settings:"Settings",label:"Tools"},
  common:{checkFields:"Check: {{fields}}",integerMinimum:"Enter a whole number of at least {{min}}",numberMinimum:"Enter a number of at least {{min}}",decimalRange:"Enter a number from {{min}} to {{max}}",numberRange:"Enter a whole number from {{min}} to {{max}}",refresh:"Refresh",failed:"Action failed",reconnect:"Reconnect",saved:"Saved",open:"Open",back:"Back to home",waiting:"Waiting for connection",retry:"Retry"},
  shell:{repositoryExample:"GitHub repository (example)",general:"General",preview:"Development preview",tagline:"Your Windows toolkit",intro:"Everyday tools, one place",appearance:"Appearance",theme:"Theme",themeHint:"Use light, dark, or your Windows theme",system:"Use system setting",light:"Light",dark:"Dark",language:"Language",languageHint:"Choose the interface language",background:"Background",connection:"Connection",backgroundHint:"Tools keep running when you close Settings",connected:"Connected",disconnected:"Disconnected",unavailable:"Component unavailable",unavailableHint:"Tools appear here when their components are ready"},
  module:{unknownResult:"Could not confirm the module operation",panelSections:"Module sections",panel:{capture:"Capture","always-on-top":"Pin settings",wintray:"Programs"},intro:"Choose your tools. Settings are kept when a module is disabled.",loading:"Loading modules",updated:"Updating",failed:"Could not update the module",enabled:"{{name}} enabled",disabled:"{{name}} disabled",retryClose:"Finish closing",retryRecovery:"Retry configuration recovery",recovered:"Configuration recovered",recoveryFailed:"Configuration recovery is incomplete",disabledTitle:"{{name}} is disabled",retained:"Your settings and data are kept.",manage:"Manage modules"},
  moduleDescription:{capture:"Screenshots, annotations, and pinned images","always-on-top":"Keep selected windows above others",wintray:"Manage startup programs and tray icons"},
  moduleStatus:{incomplete:"Closing needs recovery",faulted:"Needs attention",unavailable:"Unavailable",working:"Working",enabled:"Enabled",disabled:"Disabled"},
};

export const i18n=i18next.createInstance();
// Bundled dictionaries need no network backend, cookie, storage or external service.
void i18n.use(initReactI18next).init({resources:{"zh-CN":{translation:zh},"en-US":{translation:en}},lng:"zh-CN",fallbackLng:"en-US",supportedLngs:["zh-CN","en-US"],initAsync:false,interpolation:{escapeValue:false},react:{useSuspense:false}});

export async function setApplicationLanguage(preference:string,hostLocale?:string) {
  const locale=preference==="system"?(hostLocale||"en-US"):(preference||"zh-CN");
  await i18n.changeLanguage(locale);
  if(typeof document!=="undefined")document.documentElement.lang=locale;
}
