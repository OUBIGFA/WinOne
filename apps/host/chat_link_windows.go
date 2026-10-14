//go:build windows

package main

import (
 "encoding/json"
 "errors"
 "net/url"
 "strings"
 "golang.org/x/sys/windows"
 "winone/internal/configjson"
)

func openChatLink(raw json.RawMessage) error {
 var input struct{URL string `json:"url"`};if err:=configjson.DecodeObject(raw,&input);err!=nil{return err}
 address,err:=url.Parse(input.URL);if err!=nil||(address.Scheme!="http"&&address.Scheme!="https")||address.Hostname()==""||address.User!=nil||strings.ContainsAny(input.URL,"\x00\r\n"){return errors.New("只能打开 HTTP 或 HTTPS 网页")}
 target,err:=windows.UTF16PtrFromString(address.String());if err!=nil{return err};verb,_:=windows.UTF16PtrFromString("open");return windows.ShellExecute(0,verb,target,nil,nil,windows.SW_SHOWNORMAL)
}
