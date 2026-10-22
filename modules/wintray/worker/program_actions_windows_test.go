//go:build windows

package worker

import (
 "context"
 "encoding/json"
 "path/filepath"
 "testing"
 "winone/modules/wintray/internal/config"
 "winone/modules/wintray/internal/orchestrator"
)

type programLogger struct{}
func (programLogger) Info(string){}
func (programLogger) Warn(string){}
func (programLogger) Error(string){}

func TestProgramActionsResolveCommittedEntryAndRequireConfirmation(t *testing.T){
 settings:=config.DefaultSettings();settings.ManagedApps=[]config.ManagedAppEntry{{ID:"one",Name:"Fixture",ExePath:filepath.Join(t.TempDir(),"absent.exe")}}
 r:=&Runtime{ctx:context.Background(),manager:NewManager(config.NewStore(filepath.Join(t.TempDir(),"settings.json")),settings,&effects{}),orchestrator:orchestrator.NewService(orchestrator.NewWin32WindowEnumerator(),orchestrator.NewWin32WindowManager(),programLogger{})}
 for _,tc:=range []struct{method,raw string}{{"program.state",`{"id":"other"}`},{"program.state",`{"id":"one","path":"C:/unrelated.exe"}`},{"program.window",`{"id":"one"}`},{"program.stop",`{"id":"one"}`}} {
  if _,err:=r.programAction(tc.method,json.RawMessage(tc.raw));err==nil {t.Errorf("accepted invalid/unconfirmed action %s %s",tc.method,tc.raw)}
 }
 value,err:=r.programAction("program.state",json.RawMessage(`{"id":"one"}`));if err!=nil {t.Fatal(err)}
 state:=value.(orchestrator.ProgramState);if state.Running||state.Visible||state.HasWindow {t.Fatal("absent program has fabricated state",state)}
 r.manager.recoveryError="pending settings recovery"
 if _,err:=r.programAction("program.state",json.RawMessage(`{"id":"one"}`));err==nil {t.Fatal("action ignored recovery lock")}
}
