// @vitest-environment jsdom
import {afterEach, expect, it, vi} from "vitest";
import {cleanup, render, screen, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {AppearanceSettings} from "./appearance-settings";

// Local sidebar preferences must not leak from one settings case to another.
afterEach(()=>{cleanup();localStorage.clear();});
it("gives theme and language their own named sections and clickable choices",async()=>{
  const theme=vi.fn(),language=vi.fn();
  render(<AppearanceSettings theme="system" language="zh-CN" disabled={false} onThemeChange={theme} onLanguageChange={language}/>);
  const themeSection=screen.getByRole('region',{name:'主题'}),languageSection=screen.getByRole('region',{name:'语言'});
  expect(within(themeSection).getAllByRole('radio')).toHaveLength(3);
  expect(within(languageSection).getAllByRole('radio')).toHaveLength(3);
  await userEvent.setup().click(within(themeSection).getByText('深色'));
  expect(theme).toHaveBeenCalledExactlyOnceWith('dark');
  expect(language).not.toHaveBeenCalled();
});
it("sends theme and language choices to their own save actions", async () => {
  const onThemeChange=vi.fn(), onLanguageChange=vi.fn();
  render(<AppearanceSettings theme="system" language="zh-CN" disabled={false} onThemeChange={onThemeChange} onLanguageChange={onLanguageChange}/>);
  const user=userEvent.setup();
  const groups=screen.getAllByRole("radiogroup");
  const theme=within(groups[0]).getAllByRole("radio");
  await user.click(theme[1]);
  expect(onThemeChange).toHaveBeenLastCalledWith("light");
  expect(onLanguageChange).not.toHaveBeenCalled();
  await user.click(screen.getByRole("radio", {name:"English"}));
  expect(onLanguageChange).toHaveBeenLastCalledWith("en-US");
});
it("blocks changes while preferences are loading or being saved", async () => {
  const change=vi.fn();
  render(<AppearanceSettings theme="dark" language="en-US" disabled onThemeChange={change} onLanguageChange={change}/>);
  const user=userEvent.setup();
  for(const label of ["简体中文","浅色","显示图标和名称"]){await user.click(screen.getByText(label));}
  expect(change).not.toHaveBeenCalled();
  expect(screen.getByRole("radio",{name:"只显示图标"}).getAttribute("aria-checked")).toBe("true");
});

it("supports arrow-key selection without leaving the preference group",async()=>{
 function Preferences(){
  const [theme,setTheme]=useState<"system"|"light"|"dark">("system");
  return <AppearanceSettings theme={theme} language="zh-CN" disabled={false} onThemeChange={setTheme} onLanguageChange={()=>{}}/>;
 }
 render(<Preferences/>);
 const theme=screen.getByRole("radiogroup",{name:"主题"});
 within(theme).getByRole("radio",{name:"跟随系统"}).focus();
 const user=userEvent.setup();
 await user.keyboard("{ArrowRight}");
 expect(within(theme).getByRole("radio",{name:"浅色"}).getAttribute("aria-checked")).toBe("true");
 await user.keyboard("{ArrowRight}");
 expect(within(theme).getByRole("radio",{name:"深色"}).getAttribute("aria-checked")).toBe("true");
 expect(screen.getByRole("radio",{name:"简体中文"}).getAttribute("aria-checked")).toBe("true");
});

it.each([
 ["主题","跟随系统","system"],["主题","浅色","light"],["主题","深色","dark"],
 ["语言","跟随系统","system"],["语言","简体中文","zh-CN"],["语言","English","en-US"],
])("keeps the %s choice %s connected to its original setting",async(group,label,value)=>{
 const theme=vi.fn(),language=vi.fn();
 render(<AppearanceSettings theme={value==="dark"?"light":"dark"} language={value==="en-US"?"zh-CN":"en-US"} disabled={false} onThemeChange={theme} onLanguageChange={language}/>);
 await userEvent.setup().click(within(screen.getByRole("radiogroup",{name:group})).getByText(label));
 expect(group==="主题"?theme:language).toHaveBeenCalledExactlyOnceWith(value);
 expect(group==="主题"?language:theme).not.toHaveBeenCalled();
});
