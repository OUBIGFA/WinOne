// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import {ProfileSettingsReview} from "./profile-settings-review";
import {setApplicationLanguage} from "../i18n";

afterEach(async()=>{cleanup();await setApplicationLanguage("zh-CN");});
it.each([
 ["zh-CN","完成截图后","截完保存","默认名称","工具栏中的工具"],
 ["en-US","After selecting a capture","Save immediately","Default filename","Toolbar tools"],
])("explains capture defaults in %s",async(language,workflow,mode,filename,toolbar)=>{
 await setApplicationLanguage(language);
 render(<ProfileSettingsReview values={{openEditor:false,fileNameTemplate:"",toolbarTools:null}} paths={[]}/>);
 expect(screen.getByText(workflow)).toBeTruthy();expect(screen.getByText(mode)).toBeTruthy();
 expect(screen.getByText(filename)).toBeTruthy();expect(screen.getByText(toolbar)).toBeTruthy();
});
it("shows editing mode and preserves the user's filename template",()=>{
 render(<ProfileSettingsReview values={{openEditor:true,fileNameTemplate:"工作-{date}",toolbarTools:["arrow","text"]}} paths={[]}/>);
 expect(screen.getByText("截图后编辑")).toBeTruthy();expect(screen.getByText("工作-{date}")).toBeTruthy();
 expect(screen.getByText("箭头、文字")).toBeTruthy();
});
it("renders every action script literally and resolves DLL and program mappings",()=>{
 const source='Base.Activate();\n// <script>kept as text</script>\nBase.PressKeys("([s])");';
 const {container}=render(<ProfileSettingsReview values={{actionPackages:[{name:"自定义动作",actions:[{name:"保存",code:source},{name:"第二个动作",code:"Base.Run(\"fixture\");"}]}],assemblies:[{pathRef:"stroke.dll"}],scopes:[{processPathRef:"stroke.app"}]}} paths={[{id:"stroke.dll",kind:"file",label:"DLL",value:"D:\\Chosen.dll",state:"ready"},{id:"stroke.app",kind:"program",label:"程序",value:"D:\\Chosen.exe",state:"ready"}]}/>);
 expect(Array.from(container.querySelectorAll('pre')).map(node=>node.textContent)).toEqual([source,'Base.Run("fixture");']);
 expect(container.querySelector('script')).toBeNull();expect(screen.getByText('D:\\Chosen.dll')).toBeTruthy();expect(screen.getByText('D:\\Chosen.exe')).toBeTruthy();
});
