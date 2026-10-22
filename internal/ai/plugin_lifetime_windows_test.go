//go:build windows

package ai

import (
 "context"
 "encoding/json"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "testing"
 "time"
 "golang.org/x/sys/windows"
)

// The helper belongs exclusively to this test, including its descendant.
func TestPluginJobProbe(t *testing.T) {
 role := os.Getenv("WINONE_PLUGIN_JOB_PROBE")
 if role == "child" { time.Sleep(2*time.Minute); return }
 if role != "parent" { t.Skip("subprocess-only fixture") }
 cmd:=exec.Command(os.Args[0],"-test.run=^TestPluginJobProbe$")
 cmd.Env=append(os.Environ(),"WINONE_PLUGIN_JOB_PROBE=child")
 if err:=cmd.Start();err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(os.Getenv("WINONE_PLUGIN_JOB_PID"),[]byte(strconv.Itoa(cmd.Process.Pid)),0600);err!=nil {t.Fatal(err)}
 cmd.Process.Release()
}

func TestPluginChildLivesUntilHostCancellation(t *testing.T) {
 ctx,cancel:=context.WithCancel(t.Context());defer cancel()
 pidFile:=filepath.Join(t.TempDir(),"child.pid")
 cmd:=exec.Command(os.Args[0],"-test.run=^TestPluginJobProbe$")
 cmd.Env=append(os.Environ(),"WINONE_PLUGIN_JOB_PROBE=parent","WINONE_PLUGIN_JOB_PID="+pidFile)
 p,err:=startOwnedProcess(t.Context(),cmd);if err!=nil {t.Fatal(err)}
 p.pluginLifetime=ctx
 p.input.Close()
 if err=p.finish();err!=nil {t.Fatal(err)}
 raw,err:=os.ReadFile(pidFile);if err!=nil {t.Fatal(err)}
 pid,_:=strconv.Atoi(string(raw))
 handle,err:=windows.OpenProcess(windows.SYNCHRONIZE,false,uint32(pid));if err!=nil {t.Fatal("plugin browser descendant died with worker",err)}
 defer windows.CloseHandle(handle)
 if state,_:=windows.WaitForSingleObject(handle,0);state!=uint32(windows.WAIT_TIMEOUT) {t.Fatal("descendant exited before host")}
 cancel()
 if state,err:=windows.WaitForSingleObject(handle,5000);err!=nil||state!=windows.WAIT_OBJECT_0 {t.Fatalf("host exit left descendant running: %d %v",state,err)}
}

// 65 seconds is intentionally above the old independent one-minute budget.
// Opt-in only: the normal unit suite never downloads or waits for login.
func TestPluginAuthorizationPreparationUsesLoginDeadline(t *testing.T) {
 exe:=os.Getenv("WINONE_AI_PLUGIN_TEST_EXE");if exe=="" {t.Skip("explicit packaged runtime")}
 root:=t.TempDir();source:=filepath.Join(root,"slow-provider")
 if err:=os.MkdirAll(source,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(source,"package.json"),[]byte(`{"name":"winone-slow-provider-fixture","type":"module","main":"index.mjs"}`),0600);err!=nil {t.Fatal(err)}
 code:=`export default {server: async () => ({
  config(c) { c.provider ??= {}; c.provider.slowlogin = {name:'Slow fixture',models:{m:{name:'Fixture'}}}; },
  auth: {provider:'slowlogin',methods:[{
   type:'oauth',label:'Fixture browser',
   async authorize() {
    await new Promise(resolve => setTimeout(resolve,65000));
    return {url:'',method:'auto',callback:async()=>({type:'success',key:'fixture-only',email:'fixture@example.invalid'})};
   }
  }]}
 })}`
 if err:=os.WriteFile(filepath.Join(source,"index.mjs"),[]byte(code),0600);err!=nil {t.Fatal(err)}
 registry:=filepath.Join(root,"third-party","plugins.json");if err:=os.MkdirAll(filepath.Dir(registry),0700);err!=nil {t.Fatal(err)}
 data,_:=json.Marshal(map[string]any{"plugins":[]map[string]string{{"spec":source}}});if err:=os.WriteFile(registry,data,0600);err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(t.Context(),90*time.Second);defer cancel()
 runtime:=Translator{Executable:exe,DataDirectory:root,AccountBackend:"plugin",PluginLifetime:ctx}
 if _,err:=runtime.Accounts(ctx,"accounts.list","slowlogin");err!=nil {t.Fatal("fixture discovery failed",err)}
 start:=time.Now();var ready,done bool
 err:=runtime.SignIn(ctx,"slowlogin",make(chan string),func(raw json.RawMessage){
  var event struct{Error string; SignIn struct{State,Error string}};if err:=json.Unmarshal(raw,&event);err!=nil {t.Error(err)}
  if event.Error!=""||event.SignIn.Error!="" {t.Errorf("login failed: %s",raw)}
  if event.SignIn.State=="starting" {ready=true;if time.Since(start)>10*time.Second {t.Error("preparation was not asynchronous")}}
  if event.SignIn.State=="done" {done=true}
 })
 if err!=nil||!ready||!done {t.Fatalf("login preparation failed: ready=%v done=%v err=%v",ready,done,err)}
}
