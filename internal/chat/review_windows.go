//go:build windows

package chat

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "unicode/utf8"
)

type ReviewFile struct { Path string `json:"path"`; OldPath string `json:"oldPath,omitempty"`; Status string `json:"status"`; Untracked bool `json:"untracked,omitempty"` }
type ReviewSnapshot struct { Root string `json:"root"`; Branch string `json:"branch"`; Scope string `json:"scope"`; Base string `json:"base"`; Revision string `json:"revision"`; Files []ReviewFile `json:"files"`; Source string `json:"source,omitempty"`; IsReversible bool `json:"isReversible,omitempty"`; WorkspaceState string `json:"workspaceState,omitempty"`; captured map[string]*capturedTurnFile }
type ReviewCommit struct { ID string `json:"id"`; Title string `json:"title"`; Author string `json:"author"`; Date string `json:"date"` }
type ReviewMutation struct { Action string `json:"action"`; Paths []string `json:"paths"`; Message string `json:"message"`; Revision string `json:"revision"`; Hunk *int `json:"hunk,omitempty"`; IncludeUnstaged bool `json:"includeUnstaged"` }
type reviewOutput struct { bytes.Buffer }
func (b *reviewOutput) Write(data []byte)(int,error){if b.Len()+len(data)>16<<20{return 0,errors.New("差异超过 16 MB，请按文件查看")};return b.Buffer.Write(data)}

// Arguments are never interpreted by a shell. Clear inherited repository
// bindings, use literal paths, and disable external diff/textconv programs.
func reviewGit(ctx context.Context,directory,input string,args ...string)(string,error){
 ctx,cancel:=context.WithTimeout(ctx,45*time.Second);defer cancel()
 command:=exec.CommandContext(ctx,"git",append([]string{"--literal-pathspecs","-C",directory,"-c","core.quotepath=false","-c","color.ui=false"},args...)...)
 command.Env=append(gitReadEnvironment(os.Environ()),"GIT_TERMINAL_PROMPT=0");command.SysProcAttr=&syscall.SysProcAttr{CreationFlags:windowsNoWindow};command.WaitDelay=time.Second
 command.Stdin=strings.NewReader(input);var out reviewOutput;var diagnostic gitOutput;command.Stdout=&out;command.Stderr=&diagnostic
 if err:=command.Run();err!=nil{if ctx.Err()!=nil{return "",ctx.Err()};return "",fmt.Errorf("Git: %s (%w)",strings.TrimSpace(diagnostic.String()),err)};return out.String(),nil
}
const windowsNoWindow=0x08000000
func reviewRoot(ctx context.Context,directory string)(string,error){value,err:=reviewGit(ctx,directory,"","rev-parse","--show-toplevel");if err!=nil{return "",err};return filepath.Clean(strings.TrimSpace(value)),nil}
func reviewRef(ctx context.Context,root,value string)(string,error){
 if value==""||strings.HasPrefix(value,"-")||strings.ContainsAny(value,"\x00\r\n"){return "",errors.New("请选择有效的分支或提交")}
 commit,err:=reviewGit(ctx,root,"","rev-parse","--verify","--end-of-options",value+"^{commit}");return strings.TrimSpace(commit),err
}
func reviewDiffArgs(ctx context.Context,root,scope,base string)([]string,error){
 args:=[]string{"diff","--no-ext-diff","--no-textconv","--find-renames"}
 switch scope {
 case "working":
 case "staged":args=append(args,"--cached")
 case "branch":ref,err:=reviewRef(ctx,root,base);if err!=nil{return nil,err};merge,err:=reviewGit(ctx,root,"","merge-base",ref,"HEAD");if err!=nil{return nil,err};args=append(args,strings.TrimSpace(merge),"HEAD")
 case "commit":ref,err:=reviewRef(ctx,root,base);if err!=nil{return nil,err};args=[]string{"show","--format=","--no-ext-diff","--no-textconv","--find-renames",ref}
 default:return nil,errors.New("未知审查范围")
 };return args,nil
}
func reviewPath(value string)(string,error){
 clean:=filepath.Clean(filepath.FromSlash(value));if clean=="."||filepath.IsAbs(clean)||clean==".."||strings.HasPrefix(clean,".."+string(filepath.Separator))||strings.ContainsAny(value,"\x00:"){return "",errors.New("文件必须位于当前仓库")};return filepath.ToSlash(clean),nil
}
func ReadReview(ctx context.Context,directory,scope,base string)(ReviewSnapshot,error){
 root,err:=reviewRoot(ctx,directory);if err!=nil{return ReviewSnapshot{},err};args,err:=reviewDiffArgs(ctx,root,scope,base);if err!=nil{return ReviewSnapshot{},err}
 raw,err:=reviewGit(ctx,root,"",append(args,"--name-status","-z","--")...);if err!=nil{return ReviewSnapshot{},err}
 result:=ReviewSnapshot{Root:root,Scope:scope,Base:base,Files:[]ReviewFile{}}
 state,err:=ReadGitState(ctx,root);if err!=nil{return result,err};result.Branch=state.Branch;if state.Detached{result.Branch=state.Commit}
 parts:=strings.Split(raw,"\x00");for i:=0;i+1<len(parts);{status:=strings.TrimSpace(parts[i]);i++;if status==""{continue};file:=ReviewFile{Status:status,Path:parts[i]};i++;if strings.HasPrefix(status,"R")||strings.HasPrefix(status,"C"){if i>=len(parts){return result,errors.New("Git 返回的重命名记录不完整")};file.OldPath=file.Path;file.Path=parts[i];i++};result.Files=append(result.Files,file)}
 if scope=="working"{raw,err=reviewGit(ctx,root,"","ls-files","--others","--exclude-standard","-z");if err!=nil{return result,err};for _,path:=range strings.Split(raw,"\x00"){if path!=""{result.Files=append(result.Files,ReviewFile{Path:path,Status:"?",Untracked:true})}}}
 // Bind mutations to the visible state, including untracked contents. A save
 // from another editor requires refresh instead of staging unseen contents.
 hash:=sha256.New();for _,scope:=range []string{"working","staged"}{a,_:=reviewDiffArgs(ctx,root,scope,"");diff,e:=reviewGit(ctx,root,"",append(a,"--binary","--")...);if e!=nil{return result,e};hash.Write([]byte(diff))}
 for _,file:=range result.Files{hash.Write([]byte(file.Path));if info,e:=os.Lstat(filepath.Join(root,filepath.FromSlash(file.Path)));e==nil{fmt.Fprintf(hash,"%d:%d",info.Size(),info.ModTime().UnixNano())}}
 result.Revision=hex.EncodeToString(hash.Sum(nil));return result,nil
}
func ReadReviewDiff(ctx context.Context,directory,scope,base,path string)(string,error){
 root,err:=reviewRoot(ctx,directory);if err!=nil{return "",err};path,err=reviewPath(path);if err!=nil{return "",err};args,err:=reviewDiffArgs(ctx,root,scope,base);if err!=nil{return "",err}
 patch,err:=reviewGit(ctx,root,"",append(args,"--binary","--",path)...);if err!=nil||patch!=""||scope!="working"{return patch,err}
 tracked,err:=reviewGit(ctx,root,"","ls-files","-z","--",path);if err!=nil||tracked!=""{return "",err}
 absolute:=filepath.Join(root,filepath.FromSlash(path));info,err:=os.Lstat(absolute);if err!=nil{return "",err};if !info.Mode().IsRegular(){return "",errors.New("此文件类型不支持文本差异")};if info.Size()>10<<20{return "",errors.New("文件超过 10 MB，请使用文件预览")};data,err:=os.ReadFile(absolute);if err!=nil{return "",err}
 if !utf8.Valid(data)||bytes.IndexByte(data,0)>=0{return "Binary file: "+path,nil}
 lines:=strings.SplitAfter(string(data),"\n");if len(lines)>0&&lines[len(lines)-1]==""{lines=lines[:len(lines)-1]};var out strings.Builder;fmt.Fprintf(&out,"diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n",path,path,path,len(lines));for _,line:=range lines{out.WriteString("+"+line);if !strings.HasSuffix(line,"\n"){out.WriteString("\n\\ No newline at end of file\n")}};return out.String(),nil
}
func ReviewBranches(ctx context.Context,directory string)([]string,error){root,err:=reviewRoot(ctx,directory);if err!=nil{return nil,err};raw,err:=reviewGit(ctx,root,"","for-each-ref","--format=%(refname:short)","refs/heads/","refs/remotes/");if err!=nil{return nil,err};result:=[]string{};for _,ref:=range strings.Split(strings.TrimSpace(raw),"\n"){if ref!=""&&!strings.HasSuffix(ref,"/HEAD"){result=append(result,ref)}};return result,nil}
func ReviewCommits(ctx context.Context,directory string)([]ReviewCommit,error){root,err:=reviewRoot(ctx,directory);if err!=nil{return nil,err};state,err:=ReadGitState(ctx,root);if err!=nil{return nil,err};result:=[]ReviewCommit{};if state.Unborn{return result,nil};raw,err:=reviewGit(ctx,root,"","log","-100","--format=%H%x00%s%x00%an%x00%aI%x00");if err!=nil{return nil,err};fields:=strings.Split(raw,"\x00");for i:=0;i+3<len(fields);i+=4{result=append(result,ReviewCommit{ID:strings.TrimSpace(fields[i]),Title:fields[i+1],Author:fields[i+2],Date:fields[i+3]})};return result,nil}
func MutateReview(ctx context.Context,directory string,input ReviewMutation)error{
 root,err:=reviewRoot(ctx,directory);if err!=nil{return err};scope:="working";if input.Action=="unstage"{scope="staged"};snapshot,err:=ReadReview(ctx,root,scope,"");if err!=nil{return err};if input.Revision!=""&&input.Revision!=snapshot.Revision{return errors.New("文件已变化，请刷新差异后重试")}
 paths:=[]string{};for _,path:=range input.Paths{value,e:=reviewPath(path);if e!=nil{return e};paths=append(paths,value)}
 if input.Hunk!=nil{
  if len(paths)!=1||!strings.Contains(" stage unstage revert "," "+input.Action+" "){return errors.New("请选择一处可修改的差异")}
  patch,e:=ReadReviewDiff(ctx,root,scope,"",paths[0]);if e!=nil{return e};parts:=strings.Split(patch,"\n@@ ");if *input.Hunk<0||*input.Hunk+1>=len(parts){return errors.New("差异已变化，请刷新")};patch=parts[0]+"\n@@ "+parts[*input.Hunk+1];if !strings.HasSuffix(patch,"\n"){patch+="\n"}
  args:=[]string{"apply","--whitespace=nowarn"};if input.Action!="revert"{args=append(args,"--cached")};if input.Action!="stage"{args=append(args,"--reverse")};_,err=reviewGit(ctx,root,patch,append(args,"-")...);return err
 }
 switch input.Action{
 case "stage":if len(paths)==0{return errors.New("请选择要暂存的文件")};_,err=reviewGit(ctx,root,"",append([]string{"add","--"},paths...)...)
 case "unstage":if len(paths)==0{return errors.New("请选择要取消暂存的文件")};state,e:=ReadGitState(ctx,root);if e!=nil{return e};args:=[]string{"restore","--staged","--"};if state.Unborn{args=[]string{"rm","--cached","--"}};_,err=reviewGit(ctx,root,"",append(args,paths...)...)
 case "revert":if len(paths)==0{return errors.New("请选择要还原的文件")};for _,path:=range paths{tracked,e:=reviewGit(ctx,root,"","ls-files","-z","--",path);if e!=nil{return e};if tracked==""{return errors.New("未跟踪文件请从文件管理器移入回收站")}};_,err=reviewGit(ctx,root,"",append([]string{"restore","--worktree","--"},paths...)...)
 case "commit":if strings.TrimSpace(input.Message)==""{return errors.New("请输入提交说明")};if input.IncludeUnstaged{if _,err=reviewGit(ctx,root,"","add","--all","--",".");err!=nil{return err}};_,err=reviewGit(ctx,root,"","commit","-m",input.Message)
 case "push":_,err=reviewGit(ctx,root,"","push")
 default:return errors.New("未知审查操作")
 };return err
}
