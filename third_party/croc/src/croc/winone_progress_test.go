package croc

import (
 "encoding/json"
 "io"
 "testing"
 "github.com/schollz/progressbar/v3"
)

func TestWinOneProgressSnapshot(t *testing.T) {
 c:= &Client{}
 before:=c.WinOneProgress()
 if before.Progress != nil {t.Fatal("invented progress before a file exists")}
 bar:=progressbar.NewOptions64(200,progressbar.OptionSetWriter(io.Discard),progressbar.OptionSetDescription("file.bin"))
 c.setProgressBar(bar)
 if err:=bar.Add64(40);err!=nil {t.Fatal(err)}
 after:=c.WinOneProgress()
 if after.Progress == nil || after.Progress.CurrentNum != 40 || after.Progress.Max != 200 || after.Progress.Description != "file.bin" {t.Fatalf("bad progress: %+v",after)}
 c.setProgressBar(progressbar.NewOptions64(0,progressbar.OptionSetWriter(io.Discard)))
 if _,err:=json.Marshal(c.WinOneProgress());err!=nil{t.Fatalf("empty-file progress cannot cross the bridge: %v",err)}
}
