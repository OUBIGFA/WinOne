//go:build windows

package chat

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
)

// All Git mutations stay inside this test-owned repository. An inherited Git
// binding must not redirect review or staging into the developer's checkout.
func TestReviewReadsStagesAndUnstagesLiteralFiles(t *testing.T) {
 directory:=t.TempDir()
 git:=func(args ...string){t.Helper();command:=exec.Command("git",append([]string{"-C",directory},args...)...);command.Env=gitReadEnvironment(os.Environ());if out,err:=command.CombinedOutput();err!=nil{t.Fatalf("git %v: %s: %v",args,out,err)}}
 git("init");git("config","user.name","Test");git("config","user.email","test@example.test")
 file:=filepath.Join(directory,"note [one].txt")
 if err:=os.WriteFile(file,[]byte("before\n"),0600);err!=nil{t.Fatal(err)}
 git("add",".");git("commit","-m","initial")
 if err:=os.WriteFile(file,[]byte("after\n"),0600);err!=nil{t.Fatal(err)}
 t.Setenv("GIT_DIR",filepath.Join(directory,"not-a-repository"))
 snapshot,err:=ReadReview(context.Background(),directory,"working","")
 if err!=nil||len(snapshot.Files)!=1{t.Fatalf("snapshot=%+v error=%v",snapshot,err)}
 patch,err:=ReadReviewDiff(context.Background(),directory,"working","","note [one].txt")
 if err!=nil||!strings.Contains(patch,"+after"){t.Fatalf("diff=%s error=%v",patch,err)}
 if err:=MutateReview(context.Background(),directory,ReviewMutation{Action:"stage",Paths:[]string{"note [one].txt"}});err!=nil{t.Fatal(err)}
 staged,err:=ReadReview(context.Background(),directory,"staged","");if err!=nil||len(staged.Files)!=1{t.Fatalf("staged=%+v error=%v",staged,err)}
 if err:=MutateReview(context.Background(),directory,ReviewMutation{Action:"unstage",Paths:[]string{"note [one].txt"}});err!=nil{t.Fatal(err)}
 staged,err=ReadReview(context.Background(),directory,"staged","");if err!=nil||len(staged.Files)!=0{t.Fatalf("staged after unstage=%+v error=%v",staged,err)}
 if err:=MutateReview(context.Background(),directory,ReviewMutation{Action:"stage",Paths:[]string{"../outside.txt"}});err==nil{t.Fatal("accepted path outside repository")}
 if _,err:=ReadReview(context.Background(),directory,"branch","--output=bad");err==nil{t.Fatal("accepted an option as branch")}
}
