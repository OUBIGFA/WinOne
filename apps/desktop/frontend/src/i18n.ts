import i18next from "i18next";
import { initReactI18next } from "react-i18next";
import { captureMessages } from "./locales/capture";
import { alwaysOnTopMessages } from "./locales/always-on-top";
import { shortcutMessages } from "./locales/shortcuts";
import { wintrayMessages } from "./locales/wintray";
import { profileMessages, profileFieldMessages } from "./locales/profiles";
import {diagnosticsMessages} from "./locales/diagnostics";
import {updateMessages} from "./locales/updates";
import {aiMessages} from "./locales/ai";
import {aiRoutingMessages} from "./locales/ai-routing";
import {usageMessages} from "./locales/usage";
import {gatewayMessages} from "./locales/gateway";
import {selectionMessages} from "./locales/selection";
import {sharedPackageMessages} from "./locales/shared-packages";
import {chatMessages} from "./locales/chat";
import {directoryLinksMessages} from './locales/directory-links';
import {mouseGesturesMessages} from './locales/mouse-gestures';
export { useTranslation } from "react-i18next";

const zh={
  ...aiRoutingMessages.zh,
  stroke:mouseGesturesMessages.zh,
  links:directoryLinksMessages.zh,
  chat:chatMessages.zh,
  quickAssistant:{alwaysOnTop:"默认置顶快捷助手",showReasoning:"显示快捷助手的思考过程",loading:"正在读取快捷助手设置",open:"打开快捷助手",toolbarHint:"在 AI 对话框中使用这些功能。默认功能与划词助手共用，排序、显示状态和自定义功能在这里单独保存。"},
  sharedPackages:sharedPackageMessages.zh,
  gateway:gatewayMessages.zh,
  usage:usageMessages.zh,
  selection:selectionMessages.zh,
  ai:aiMessages.zh,
  updates:updateMessages.zh,
  permission:{title:"运行权限",loading:"正在读取权限",administrator:"当前以管理员身份运行",standard:"当前以普通权限运行",remember:"始终以管理员身份运行",consent:"首次启用需要管理员授权。",nextStart:"重新启动 WinOne 后生效。",untilExit:"从托盘退出后，下次启动将使用普通权限。",restart:"以管理员身份重新启动",restarting:"正在重新启动"},
  startup:{title:"启动与界面",logon:"登录 Windows 时启动",logonHint:"登录后自动运行 WinOne，无需管理员权限。",silent:"静默启动到托盘",silentHint:"自动启动时不打开设置窗口。",keepUI:"保持界面后台运行",keepUIHint:"关闭窗口后保留界面，便于快速打开。关闭此项后，关闭窗口会释放界面进程。"},
  dashboard:{enabledCount:"已开启 {{count}} / {{total}}",resizePanels:"调整工具与概览宽度",permission:"运行权限",administrator:"管理员",standard:"普通权限",toolsEnabled:"已开启工具",statusHint:"按需开启，关闭后保留设置。",attention:"{{count}} 个工具需要处理",shortcuts:"当前快捷键",noShortcuts:"暂无已启用的快捷键",manageTools:"工具管理"},
  window:{minimize:"最小化窗口",maximize:"最大化窗口",restore:"还原窗口",close:"关闭窗口"},
  navigation:{scrollToTop:"返回顶部",scrollToBottom:"返回底部",expandSidebar:"展开侧栏",collapseSidebar:"收起侧栏",resizeSidebar:"调整侧栏宽度",closeDescription:"关闭窗口会放弃这些更改。",discardClose:"放弃并关闭",waitCloseDescription:"操作完成后将关闭窗口；失败时保留当前更改。",dirtyTitle:"有未保存的更改",dirtyDescription:"离开页面会放弃这些更改。",keepEditing:"继续编辑",discard:"放弃并离开",waitTitle:"操作尚未完成",waitDescription:"操作完成后将切换页面；失败时保留当前更改。",stay:"留在此页"},
  diagnostics:diagnosticsMessages.zh,
  capture:captureMessages.zh,
  aot:alwaysOnTopMessages.zh,
  shortcuts:shortcutMessages.zh,
  wintray:wintrayMessages.zh,
  profiles:profileMessages.zh,profileFields:profileFieldMessages.zh,
  nav:{overview:"首页",capture:"截图助手",selection:"划词助手","quick-assistant":"快捷助手","always-on-top":"置顶功能",convenience:"便捷功能",wintray:"程序管理",modules:"模块",settings:"设置",label:"功能导航"},
  common:{close:"关闭",checkFields:"请检查：{{fields}}",integerMinimum:"请输入不小于 {{min}} 的整数",numberMinimum:"请输入不小于 {{min}} 的数值",decimalRange:"请输入 {{min}}–{{max}} 的数值",numberRange:"请输入 {{min}}–{{max}} 的整数",refresh:"刷新状态",failed:"操作未完成",reconnect:"重试连接",saved:"已保存",open:"打开",back:"返回概览",waiting:"等待后台连接",retry:"重试"},
  shell:{repositoryExample:"GitHub 仓库（示例）",general:"常规",preview:"开发预览",tagline:"你的 Windows 工具集",intro:"常用工具，一个入口",appearance:"外观",sidebarStartup:"打开界面时的侧栏",sidebarCollapsed:"默认收起",sidebarExpanded:"默认展开",sidebarSaveFailed:"侧栏设置保存失败，请重试。",defaultPage:"打开界面时的默认页",theme:"主题",themeHint:"选择浅色、深色或跟随 Windows",system:"跟随系统",light:"浅色",dark:"深色",language:"语言",languageHint:"选择界面语言",background:"后台",connection:"连接状态",backgroundHint:"关闭设置窗口后，后台继续运行",connected:"已连接",disconnected:"未连接",unavailable:"组件未就绪",unavailableHint:"组件通过验证后，将在此提供设置与操作"},
  module:{enableTool:"启用{{name}}",migrationDescription:"配置格式将从版本 {{from}} 转为 {{to}}；失败时恢复原配置。",migrationFields:"涉及的配置字段",migrationUpdated:"转换后使用新配置，原启用状态保持不变。",migrationInstalled:"转换后保持关闭，可在工具管理中开启。",cleanupPending:"有安装暂存或旧模块文件等待清理。",retryCleanup:"重试清理",cleanupDone:"安装遗留文件已处理",cleanupFailed:"安装文件清理未完成",updatePackage:"更新模块",updateDescription:"核对版本变化后确认更新。",updateRetained:"保留当前配置和启用状态；失败时恢复旧版本。",confirmUpdate:"确认更新",updateFailed:"模块更新未完成",packageUpdated:"模块已更新，配置和启用状态已保留",removePackage:"卸载模块",removeDescription:"卸载程序文件，保留本机配置。再次安装时可恢复。",closeBeforeRemove:"先关闭模块，再卸载",confirmRemove:"确认卸载",removeFailed:"模块卸载未完成",packageRemoved:"模块已卸载，配置已保留",installPackage:"安装模块",installDescription:"核对来源和版本后确认安装。",inspectPackage:"正在检查签名、文件和兼容性…",packageVersion:"版本",packagePublisher:"发布者",packageFile:"文件",signatureVerified:"签名已验证",signatureDetails:"查看校验信息",installRetained:"将恢复保留的配置，安装后保持关闭。",installDisabled:"安装后保持关闭，可在工具管理中开启。",inspectAgain:"重新检查",cancelInstall:"取消",confirmInstall:"确认安装",installFailed:"模块安装未完成",packageInstalled:"模块已安装，尚未开启",packageBlocked:"模块未加载，配置和恢复记录已保留。",failureDetails:"查看原因",retryPackage:"重试加载",restorePackage:"恢复旧版本",packageRecovered:"{{name}}已重新加载",unknownResult:"未能确认模块操作结果",panelSections:"模块页面",panel:{capture:"截图助手","always-on-top":"置顶设置",wintray:"程序"},intro:"选择需要的工具，关闭后保留设置。",loading:"正在读取模块",updated:"正在更新状态",failed:"模块状态未能更新",enabled:"{{name}}已开启",disabled:"{{name}}已关闭",retryClose:"继续关闭",retryRecovery:"重试恢复配置",recovered:"配置已恢复",recoveryFailed:"配置恢复未完成",disabledTitle:"{{name}}已关闭",retained:"设置和数据已保留。",manage:"管理模块"},
  moduleDescription:{"quick-assistant":"AI 对话与快捷指令",capture:"截图、标注与截图置顶",selection:"选中文字后翻译、解释或继续对话","always-on-top":"让选定窗口保持在前面",wintray:"管理程序启动和托盘图标"},
  moduleStatus:{incomplete:"关闭未完成",faulted:"需要处理",unavailable:"组件不可用",working:"正在处理",enabled:"已开启",disabled:"已关闭"},
};
const en={...aiRoutingMessages.en,stroke:mouseGesturesMessages.en,links:directoryLinksMessages.en,
  chat:chatMessages.en,
  quickAssistant:{alwaysOnTop:"Keep Quick assistant on top by default",showReasoning:"Show reasoning in Quick assistant",loading:"Loading quick assistant settings",open:"Open quick assistant",toolbarHint:"Use these actions in the AI chat. Defaults are shared with the selection assistant; order, visibility, and custom actions are saved separately here."},
  sharedPackages:sharedPackageMessages.en,
  gateway:gatewayMessages.en,
  usage:usageMessages.en,
  selection:selectionMessages.en,
  ai:aiMessages.en,
  updates:updateMessages.en,
  permission:{title:"Permissions",loading:"Checking permissions",administrator:"Currently running as administrator",standard:"Currently running with standard permissions",remember:"Always run as administrator",consent:"Administrator approval is required the first time.",nextStart:"Restart WinOne to apply.",untilExit:"This session stays elevated. Exit WinOne from the tray to use standard permissions on the next launch.",restart:"Restart as administrator",restarting:"Restarting"},
  startup:{title:"Startup and interface",logon:"Start when signing in to Windows",logonHint:"Run WinOne automatically at sign-in. No administrator access required.",silent:"Start silently in the tray",silentHint:"Keep the settings window closed during automatic startup.",keepUI:"Keep the interface running in the background",keepUIHint:"Retain the interface when you close its window so it opens quickly. Turn this off to release the interface process when closing the window."},
  dashboard:{enabledCount:"{{count}} of {{total}} enabled",resizePanels:"Resize tools and overview",permission:"Running as",administrator:"Administrator",standard:"Standard user",toolsEnabled:"Enabled tools",statusHint:"Enable what you need. Settings are kept when disabled.",attention_one:"1 tool needs attention",attention_other:"{{count}} tools need attention",shortcuts:"Current shortcuts",noShortcuts:"No active shortcuts",manageTools:"Manage tools"},
  window:{minimize:"Minimize window",maximize:"Maximize window",restore:"Restore window",close:"Close window"},
  navigation:{scrollToTop:"Back to top",scrollToBottom:"Go to bottom",expandSidebar:"Expand sidebar",collapseSidebar:"Collapse sidebar",resizeSidebar:"Resize sidebar",closeDescription:"Closing this window will discard your changes.",discardClose:"Discard and close",waitCloseDescription:"The window will close when the action completes. If it fails, your changes stay.",dirtyTitle:"Unsaved changes",dirtyDescription:"Leaving this page will discard your changes.",keepEditing:"Keep editing",discard:"Discard and leave",waitTitle:"Action in progress",waitDescription:"The page will change when the action completes. If it fails, your changes stay.",stay:"Stay on this page"},
  diagnostics:diagnosticsMessages.en,
  capture:captureMessages.en,
  aot:alwaysOnTopMessages.en,
  shortcuts:shortcutMessages.en,
  wintray:wintrayMessages.en,
  profiles:profileMessages.en,profileFields:profileFieldMessages.en,
  nav:{overview:"Home",capture:"Capture assistant",selection:"Selection assistant","quick-assistant":"Quick assistant","always-on-top":"Always on Top",convenience:"Convenience",wintray:"Program management",modules:"Modules",settings:"Settings",label:"Tools"},
  common:{close:"Close",checkFields:"Check: {{fields}}",integerMinimum:"Enter a whole number of at least {{min}}",numberMinimum:"Enter a number of at least {{min}}",decimalRange:"Enter a number from {{min}} to {{max}}",numberRange:"Enter a whole number from {{min}} to {{max}}",refresh:"Refresh",failed:"Action failed",reconnect:"Reconnect",saved:"Saved",open:"Open",back:"Back to home",waiting:"Waiting for connection",retry:"Retry"},
  shell:{repositoryExample:"GitHub repository (example)",general:"General",preview:"Development preview",tagline:"Your Windows toolkit",intro:"Everyday tools, one place",appearance:"Appearance",sidebarStartup:"Sidebar when opening WinOne",sidebarCollapsed:"Collapsed by default",sidebarExpanded:"Expanded by default",sidebarSaveFailed:"Could not save sidebar preference. Please retry.",defaultPage:"Default page when opening WinOne",theme:"Theme",themeHint:"Use light, dark, or your Windows theme",system:"Use system setting",light:"Light",dark:"Dark",language:"Language",languageHint:"Choose the interface language",background:"Background",connection:"Connection",backgroundHint:"Tools keep running when you close Settings",connected:"Connected",disconnected:"Disconnected",unavailable:"Component unavailable",unavailableHint:"Tools appear here when their components are ready"},
  module:{enableTool:"Enable {{name}}",migrationDescription:"Settings format changes from version {{from}} to {{to}}. Original settings are restored if the update fails.",migrationFields:"Affected settings fields",migrationUpdated:"Use converted settings and keep the previous enabled state.",migrationInstalled:"Converted settings are kept with the module disabled.",cleanupPending:"Installation staging or old module files need cleanup.",retryCleanup:"Retry cleanup",cleanupDone:"Installation leftovers processed",cleanupFailed:"File cleanup did not complete",updatePackage:"Update module",updateDescription:"Review the version change before updating.",updateRetained:"Keep settings and enabled state. Restore the previous version if validation fails.",confirmUpdate:"Confirm update",updateFailed:"Module update did not complete",packageUpdated:"Module updated; settings and enabled state preserved",removePackage:"Uninstall module",removeDescription:"Remove program files and keep local settings for a future reinstall.",closeBeforeRemove:"Disable the module before uninstalling",confirmRemove:"Confirm uninstallation",removeFailed:"Module uninstallation did not complete",packageRemoved:"Module uninstalled; settings preserved",installPackage:"Install module",installDescription:"Review the publisher and version before installing.",inspectPackage:"Checking signature, files and compatibility…",packageVersion:"Version",packagePublisher:"Publisher",packageFile:"File",signatureVerified:"Signature verified",signatureDetails:"Show verification details",installRetained:"Saved settings will be restored. The module stays disabled.",installDisabled:"The module stays disabled. Enable it in tool management when needed.",inspectAgain:"Check again",cancelInstall:"Cancel",confirmInstall:"Confirm installation",installFailed:"Module installation did not complete",packageInstalled:"Module installed and disabled",packageBlocked:"The module is unavailable. Settings and recovery records are preserved.",failureDetails:"Show reason",retryPackage:"Retry loading",restorePackage:"Restore previous version",packageRecovered:"{{name}} reloaded",unknownResult:"Could not confirm the module operation",panelSections:"Module sections",panel:{capture:"Capture assistant","always-on-top":"Pin settings",wintray:"Programs"},intro:"Choose your tools. Settings are kept when a module is disabled.",loading:"Loading modules",updated:"Updating",failed:"Could not update the module",enabled:"{{name}} enabled",disabled:"{{name}} disabled",retryClose:"Finish closing",retryRecovery:"Retry configuration recovery",recovered:"Configuration recovered",recoveryFailed:"Configuration recovery is incomplete",disabledTitle:"{{name}} is disabled",retained:"Your settings and data are kept.",manage:"Manage modules"},
  moduleDescription:{"quick-assistant":"AI chat and quick actions",capture:"Screenshots, annotations, and pinned images",selection:"Translate, explain or discuss selected text","always-on-top":"Keep selected windows above others",wintray:"Manage startup programs and tray icons"},
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
