//go:build windows

package pluginpage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_SDK_NATIVE_PAGE") != "1" {
		os.Exit(m.Run())
	}
	directory := os.Getenv("WINONE_SDK_NATIVE_PAGE_PROFILE")
	if !filepath.IsAbs(directory) || !strings.Contains(filepath.Clean(directory), string(filepath.Separator)+"_temp"+string(filepath.Separator)) {
		fmt.Fprintln(os.Stderr, "原生插件页测试必须使用仓库 _temp 隔离目录")
		os.Exit(1)
	}
	mygo.App.SetName("WinOne SDK Native Page Check")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(directory,"browser-profile"))
	mygo.App.OnWindowAllClosed(func(){})
	code := 1
	mygo.App.WhenReady(func(){go func(){code=m.Run();mygo.App.Quit()}()})
	if err := mygo.App.Run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
	os.Exit(code)
}

// The page and its native focus are real WebView2/MyGo. Only the authenticated
// Host boundary is controlled: navigation requests are recorded, never opened
// in the user's browser. The original URL/confirmation policy has separate
// SDK tests, so this test does not reimplement or simulate that policy.
func TestNativePluginPageUsesEmbeddedFocusAndKeepsTheGuestInItsPrivateScope(t *testing.T){
	if os.Getenv("WINONE_SDK_NATIVE_PAGE")!="1"{t.Skip("explicit private-desktop plugin page check")}
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		w.Header().Set("Content-Type","text/html; charset=utf-8")
		fmt.Fprint(w,`<!doctype html><title>Plugin native focus fixture</title><body><button id="inside">Own plugin button</button><a href="https://accounts.example.com/key">External console</a></body>`)
	}))
	defer server.Close()
	h:=testhost.New(t,func(method string,params json.RawMessage)(any,error){
		switch method{case "chat.ghosts.page-attach","chat.ghosts.page-detach","chat.ghosts.page-navigation":return map[string]bool{"ok":true},nil}
		return nil,fmt.Errorf("unexpected fixture Host method: %s",method)
	})
	var page *Page
	outside:="Native field"
	forceOutside:=1
	window:=mygo.NewWindow(mygo.WindowOptions{Title:"WinOne SDK native plugin page",Width:720,Height:560,Content:ui.View(func(c *ui.Context){
		c.SetTheme(kit.Theme(c.Preferences(),false))
		ui.Column(c).Fill().Gap(8).Children(func(){
			field:=ui.TextInput(c.Key("before"),&outside).FillWidth().Label("Native field")
			if forceOutside==1{field.Focus()}
			other:=ui.TextInput(c.Key("after"),&outside).FillWidth().Label("Other native field")
			if forceOutside==2{other.Focus()}
			if page!=nil{page.View(c)}
		})
	})})
	defer window.Destroy()
	env:=h.Env
	env.Post=func(fn func()){if !window.IsDestroyed(){window.Update(fn)}}
	env.WebViews=window.NewWebView
	address:=server.URL+"/"+strings.Repeat("a",64)+"/__page_settings__"
	mygo.RunOnMain(func(){var err error;page,err=New(env,address,"Plugin content","fixture");if err!=nil{t.Error(err)};window.Show();window.Focus()})
	if page==nil{t.Fatal("native plugin page was not created")}
	defer mygo.RunOnMain(page.Close)
	await:=func(check func()bool,label string){
		t.Helper();deadline:=time.Now().Add(8*time.Second)
		for time.Now().Before(deadline){if check(){return};time.Sleep(20*time.Millisecond)}
		t.Fatalf("%s did not settle (host focus=%v embedded=%v)",label,window.IsFocused(),page.focus.IsFocused())
	}
	await(func()bool{select{case<-page.ready:return true;default:return false}},"native attach")
	if page.attachErr!=nil{t.Fatal(page.attachErr)}
	await(func()bool{value,err:=page.view.Page().Eval(`return document.querySelector('#inside')!==null`);return err==nil&&value==true},"real page load")
	await(func()bool{return window.IsFocused()&&!page.focus.IsFocused()},"native field focus")
	if _,err:=page.view.Page().Eval(`location.href='https://accounts.example.com/not-focused'`);err!=nil{t.Fatal(err)}
	// An ordered JS roundtrip lets WebView2 deliver the cancelled navigation;
	// this is not an arbitrary delay or guest-reported permission fact.
	if _,err:=page.view.Page().Eval(`return location.href`);err!=nil{t.Fatal(err)}
	if len(h.Called("chat.ghosts.page-navigation"))!=0{t.Fatal("window focus was mistaken for embedded page focus")}
	mygo.RunOnMain(func(){window.Update(func(){forceOutside=0});page.view.Focus()})
	await(page.focused,"actual embedded focus")
	if _,err:=page.view.Page().Eval(`location.href='https://accounts.example.com/key'`);err!=nil{t.Fatal(err)}
	await(func()bool{return len(h.Called("chat.ghosts.page-navigation"))==1},"native external-link request")
	var request struct{PageID,URL string;Focused bool}
	if err:=json.Unmarshal(h.Called("chat.ghosts.page-navigation")[0].Params,&request);err!=nil||request.PageID!=page.id||request.URL!="https://accounts.example.com/key"||!request.Focused{t.Fatalf("native request lost its page/focus/URL: %+v %v",request,err)}
	if current:=page.view.Page().URL();current!=address{t.Fatalf("guest left its private page: %s",current)}
	mygo.RunOnMain(func(){window.Update(func(){forceOutside=2})})
	await(func()bool{return !page.focus.IsFocused()},"focus returns to native field")
	if _,err:=page.view.Page().Eval(`location.href='https://accounts.example.com/after-blur'`);err!=nil{t.Fatal(err)}
	if _,err:=page.view.Page().Eval(`return location.href`);err!=nil{t.Fatal(err)}
	if len(h.Called("chat.ghosts.page-navigation"))!=1{t.Fatal("a blurred page reused its old activation")}
	mygo.RunOnMain(page.Close)
	await(func()bool{return len(h.Called("chat.ghosts.page-detach"))==1},"native detach")
	if !page.view.IsDestroyed(){t.Fatal("Close retained the native page")}
}
