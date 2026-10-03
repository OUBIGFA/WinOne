// Reproducible visual fixture. Vite's production entry does not include this page.
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { ArrowLeft, Clipboard, Pin, Save, ScanLine } from "lucide-react";
import { Button } from "../src/ui/button";
import { Switch } from "../src/ui/switch";
import {usePageNavigation} from "../src/ui/page-navigation";
import "../src/styles/shadcn.css";
import "../src/ui/tokens.css";
import "./ui-preview.css";

function Preview() {
  const [page,setPage]=useState("preview");
  const navigation=usePageNavigation(page,setPage);
  const [dark, setDark] = useState(false);
  const [sound, setSound] = useState(false);
  function theme(value: boolean) {
    setDark(value);
    document.documentElement.dataset.theme = value ? "dark" : "light";
  }
  return <main>
    {navigation.dialog}
    <Button onClick={()=>{navigation.onActivity(true,false);}}>标记未保存</Button>
    <Button onClick={()=>navigation.requestPage("next")}>离开设置</Button>
    <header><div><small>WinOne</small><h1>控件预览</h1></div><div className="theme-setting"><label id="theme-label" htmlFor="theme-switch">深色主题</label><Switch id="theme-switch" aria-labelledby="theme-label" checked={dark} onCheckedChange={theme} /></div></header>
    <section><h2>常用操作</h2><div className="actions">
      <Button variant="primary"><ScanLine />窗口截图</Button>
      <Button><Clipboard />复制</Button><Button><Save />保存</Button><Button><Pin />截图置顶</Button>
      <Button variant="ghost"><ArrowLeft />返回</Button>
    </div></section>
    <section><h2>状态</h2><div className="actions">
      <Button busy>正在保存</Button><Button disabled>暂无结果</Button><Button variant="danger">移除模块</Button>
    </div></section>
    <section><h2>设置</h2>
      <div className="setting"><label id="sound-label" htmlFor="sound-switch">播放提示音</label><Switch id="sound-switch" aria-labelledby="sound-label" checked={sound} onCheckedChange={setSound} /></div>
      <div className="setting"><label id="off-label" htmlFor="off-switch">已关闭的设置</label><Switch id="off-switch" aria-labelledby="off-label" disabled /></div>
      <div className="setting"><label id="on-label" htmlFor="on-switch">已开启的设置</label><Switch id="on-switch" aria-labelledby="on-label" checked disabled /></div>
    </section>
    <footer>用于控件与主题验证，不连接功能引擎。</footer>
  </main>;
}
createRoot(document.getElementById("root")!).render(<Preview />);
