#include "toolbar_options.h"
#include <commctrl.h>
#include <array>

namespace winone::capture {
namespace {
const std::array<const char*,13> ids{"crop","select","rectangle","ellipse","arrow","pen","cover","text","step","mosaic","blur","ocr","translate"};
const std::array<const wchar_t*,13> chinese{L"调整选区",L"移动标注",L"矩形",L"椭圆",L"箭头",L"画笔",L"遮盖",L"文字",L"序号说明",L"马赛克",L"模糊",L"识别文字",L"翻译"};
const std::array<const wchar_t*,13> english{L"Adjust selection",L"Move annotation",L"Rectangle",L"Ellipse",L"Arrow",L"Pen",L"Cover",L"Text",L"Numbered step",L"Mosaic",L"Blur",L"Recognize text",L"Translate"};
struct Options { json value; bool english; };
HWND add(HWND parent,const wchar_t*type,const wchar_t*text,DWORD style,int id,int x,int y,int width,int height) {
    RECT bounds{x,y,x+width,y+height};MapDialogRect(parent,&bounds);
    const auto window=CreateWindowExW(0,type,text,WS_CHILD|WS_VISIBLE|style,bounds.left,bounds.top,bounds.right-bounds.left,bounds.bottom-bounds.top,
        parent,reinterpret_cast<HMENU>(static_cast<INT_PTR>(id)),GetModuleHandleW(nullptr),nullptr);
    if(!window)throw systemError("Cannot create toolbar option");
    SendMessageW(window,WM_SETFONT,SendMessageW(parent,WM_GETFONT,0,0),TRUE);
    return window;
}
INT_PTR CALLBACK procedure(HWND dialog,UINT message,WPARAM wp,LPARAM lp) {
    auto* options=reinterpret_cast<Options*>(GetWindowLongPtrW(dialog,DWLP_USER));
    if(message==WM_INITDIALOG) {
        options=reinterpret_cast<Options*>(lp);SetWindowLongPtrW(dialog,DWLP_USER,lp);
        try {
            SetWindowTextW(dialog,options->english?L"Customize capture toolbar":L"自定义截图工具栏");
            const auto& labels=options->english?english:chinese;
            for(size_t i=0;i<ids.size();++i) {
                const auto item=add(dialog,L"BUTTON",labels[i],WS_TABSTOP|BS_AUTOCHECKBOX,static_cast<int>(100+i),14,12+static_cast<int>(i)*18,200,16);
                const bool checked=options->value.is_null()||std::find(options->value.begin(),options->value.end(),ids[i])!=options->value.end();
                SendMessageW(item,BM_SETCHECK,checked?BST_CHECKED:BST_UNCHECKED,0);
            }
            add(dialog,L"BUTTON",options->english?L"Reset":L"恢复默认",WS_TABSTOP|BS_PUSHBUTTON,3,14,254,62,20);
            add(dialog,L"BUTTON",options->english?L"Cancel":L"取消",WS_TABSTOP|BS_PUSHBUTTON,IDCANCEL,84,254,62,20);
            add(dialog,L"BUTTON",options->english?L"Save":L"保存",WS_TABSTOP|BS_DEFPUSHBUTTON,IDOK,154,254,62,20);
            return TRUE;
        } catch(...) {EndDialog(dialog,-1);return TRUE;}
    }
    if(message==WM_COMMAND&&options) {
        if(LOWORD(wp)==IDCANCEL){EndDialog(dialog,IDCANCEL);return TRUE;}
        if(LOWORD(wp)==3){for(size_t i=0;i<ids.size();++i)SendDlgItemMessageW(dialog,100+static_cast<int>(i),BM_SETCHECK,BST_CHECKED,0);return TRUE;}
        if(LOWORD(wp)==IDOK){options->value=json::array();for(size_t i=0;i<ids.size();++i)if(SendDlgItemMessageW(dialog,100+static_cast<int>(i),BM_GETCHECK,0,0)==BST_CHECKED)options->value.push_back(ids[i]);EndDialog(dialog,IDOK);return TRUE;}
    }
    if(message==WM_CLOSE){EndDialog(dialog,IDCANCEL);return TRUE;}
    return FALSE;
}
}
std::optional<json> chooseToolbarOptions(HWND owner,const json& current,bool englishUI) {
    std::vector<WORD> bytes((sizeof(DLGTEMPLATE)+1)/2);
    auto* dialog=reinterpret_cast<DLGTEMPLATE*>(bytes.data());
    dialog->style=WS_POPUP|WS_CAPTION|WS_SYSMENU|DS_MODALFRAME|DS_SETFONT|DS_CENTER;dialog->cx=230;dialog->cy=285;
    bytes.insert(bytes.end(),{0,0,0,9});
    for(auto c:std::wstring_view(L"Segoe UI"))bytes.push_back(c);bytes.push_back(0);
    Options options{current,englishUI};
    const auto result=DialogBoxIndirectParamW(GetModuleHandleW(nullptr),reinterpret_cast<const DLGTEMPLATE*>(bytes.data()),owner,procedure,reinterpret_cast<LPARAM>(&options));
    if(result==-1)throw std::runtime_error("Cannot open toolbar customization");
    return result==IDOK?std::optional<json>(options.value):std::nullopt;
}
}
