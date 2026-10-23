//go:build windows

package systray

import (
 "fmt"
 "image"
 "image/png"
 "os"
 "path/filepath"
 "testing"
 "unsafe"
)

// Compare the completed native popup, not merely our WM_DRAWITEM callback.
// USER32 may paint a submenu arrow after owner drawing has returned.
func verifyPopupArrowPixels(t *testing.T, s *nativeMenuStyle, window uintptr, rows map[uintptr]menuDraw) {
 t.Helper()
 var r menuRect
 u32.NewProc("GetClientRect").Call(window,uintptr(unsafe.Pointer(&r)))
 width,height:=int(r.Right),int(r.Bottom)
 if width<=0||height<=0 { t.Error("invalid popup bounds");return }
 type surface struct { dc,bitmap,old,bits uintptr }
 makeSurface:=func()surface {
  dc,_,_:=pCreateCompatibleDC.Call(0)
  bmi:=bitmapInfo{BmiHeader:bitmapInfoHeader{BiWidth:int32(width),BiHeight:-int32(height),BiPlanes:1,BiBitCount:32}}
  bmi.BmiHeader.BiSize=uint32(unsafe.Sizeof(bmi.BmiHeader))
  var bits uintptr
  bitmap,_,_:=pCreateDIBSection.Call(dc,uintptr(unsafe.Pointer(&bmi)),0,uintptr(unsafe.Pointer(&bits)),0,0)
  old,_,_:=pSelectObject.Call(dc,bitmap)
  return surface{dc,bitmap,old,bits}
 }
 actual,expected:=makeSurface(),makeSurface()
 defer func(){for _,v:=range []surface{actual,expected}{pSelectObject.Call(v.dc,v.old);menuDeleteObject.Call(v.bitmap);pDeleteDC.Call(v.dc)}}()
 if actual.bits==0||expected.bits==0 {t.Error("popup capture allocation failed");return}
 if ok,_,_:=u32.NewProc("PrintWindow").Call(window,actual.dc,1);ok==0{t.Error("popup capture failed");return}
 menuFillRect.Call(expected.dc,uintptr(unsafe.Pointer(&r)),s.backgroundBrush)
 // The print pass updates the captured native item state and coordinates.
 mismatches:=0
 a:=unsafe.Slice((*byte)(unsafe.Pointer(actual.bits)),width*height*4)
 e:=unsafe.Slice((*byte)(unsafe.Pointer(expected.bits)),width*height*4)
 for _,row:=range rows {
  entry,ok:=s.entry(row.Data)
  if !ok||entry.submenu==0 {continue}
  // #32768 exposes its HMENU through MN_GETHMENU. Read only: never alter
  // menu selection or synthesize user input on the private test desktop.
  menu,_,_:=u32.NewProc("SendMessageW").Call(window,0x01e1,0,0)
  if row.Window!=menu {continue}
  row.DC=expected.dc
  if !s.draw(&row){t.Error("expected menu draw failed");return}
  for y:=max(0,int(row.Rect.Top));y<min(height,int(row.Rect.Bottom));y++ {
   for x:=max(0,int(row.Rect.Right-s.px(40)));x<min(width,int(row.Rect.Right));x++ {
    i:=(y*width+x)*4
    if a[i]!=e[i]||a[i+1]!=e[i+1]||a[i+2]!=e[i+2]{mismatches++}
   }
  }
 }
 if output:=os.Getenv("WINONE_MENU_RENDER_OUTPUT");output!=""{
  os.MkdirAll(output,0700)
  for name,pixels:=range map[string][]byte{"actual":a,"expected":e}{
   img:=image.NewNRGBA(image.Rect(0,0,width,height))
   for i:=0;i<len(pixels);i+=4{img.Pix[i]=pixels[i+2];img.Pix[i+1]=pixels[i+1];img.Pix[i+2]=pixels[i];img.Pix[i+3]=255}
   file,err:=os.Create(filepath.Join(output,fmt.Sprintf("popup-%x-%x-%s.png",s.background,window,name)))
   if err!=nil{t.Error(err);continue};if err=png.Encode(file,img);err!=nil{t.Error(err)};file.Close()
  }
 }
 if mismatches>0{t.Errorf("native popup added/changed %d pixels in the submenu arrow gutter",mismatches)}
}
