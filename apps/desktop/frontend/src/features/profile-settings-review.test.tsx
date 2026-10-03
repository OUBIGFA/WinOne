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
