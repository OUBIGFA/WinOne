//go:build windows

package ai

import (
 "context"
 "encoding/json"
 "errors"
 "time"
)

type UsageAliasEdit struct { From string `json:"from"`; To string `json:"to"` }

// UsageAliases asks the pinned provider.SetUsageAlias to validate a draft.
// It starts no inference, plugin, privacy model or external account discovery.
func(t Translator) UsageAliases(ctx context.Context,aliases map[string]string,edit *UsageAliasEdit)(map[string]string,error){
 if len(aliases)>4096{return nil,errors.New("用量归并别名过多")}
 ctx,cancel:=context.WithTimeout(ctx,20*time.Second);defer cancel()
 t.PrivacyService=nil;t.TransportOnly=true;t.AccountBackend=""
 raw,runErr:=t.invoke(ctx,map[string]any{"operation":"usage.aliases","usageAliases":aliases,"usageAlias":edit})
 var result struct{UsageAliases map[string]string `json:"usageAliases"`;Error string `json:"error"`}
 if err:=json.Unmarshal(raw,&result);err!=nil{return nil,errors.New("用量归并校验服务响应无效，请检查 AI 服务运行组件")}
 if result.Error!=""{message:=[]rune(result.Error);if len(message)>2048{message=append(message[:2048],[]rune("…")...)};return nil,errors.New("用量归并无效，请修改模型标识或循环关系："+string(message))}
 if runErr!=nil{return nil,errors.New("用量归并校验未完成，请重试")}
 if result.UsageAliases==nil{result.UsageAliases=map[string]string{}}
 return result.UsageAliases,nil
}
