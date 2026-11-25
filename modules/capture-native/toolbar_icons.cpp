#include "toolbar_icons.h"
#include "winone_icon_resources.h"
#include <windows.h>
#include <wincodec.h>
#include <wrl/client.h>
#include <map>
#include <vector>

namespace winone::icons {
using Microsoft::WRL::ComPtr;
namespace { ComPtr<ID2D1RenderTarget> owner; std::map<int,ComPtr<ID2D1Bitmap>> images; }
void release(){images.clear();owner.Reset();}
bool render(ID2D1RenderTarget* target,std::string_view name,bool dark,const D2D1_RECT_F& rectangle,float opacity){
    int id=0;
    for(const auto& icon:resources)if(icon.name==name){id=icon.id+(dark?1:0);break;}
    if(!id||!target)return false;
    if(owner.Get()!=target){release();owner=target;}
    auto found=images.find(id);
    if(found==images.end()){
        const auto resource=FindResourceW(GetModuleHandleW(nullptr),MAKEINTRESOURCEW(id),RT_RCDATA);
        if(!resource)return false;
        const auto data=LoadResource(GetModuleHandleW(nullptr),resource);
        auto* bytes=static_cast<BYTE*>(LockResource(data));
        const DWORD length=SizeofResource(GetModuleHandleW(nullptr),resource);
        ComPtr<IWICImagingFactory> factory;
        if(FAILED(CoCreateInstance(CLSID_WICImagingFactory,nullptr,CLSCTX_INPROC_SERVER,IID_PPV_ARGS(&factory))))return false;
        ComPtr<IWICStream> stream;if(FAILED(factory->CreateStream(&stream))||FAILED(stream->InitializeFromMemory(bytes,length)))return false;
        ComPtr<IWICBitmapDecoder> decoder;if(FAILED(factory->CreateDecoderFromStream(stream.Get(),nullptr,WICDecodeMetadataCacheOnLoad,&decoder)))return false;
        ComPtr<IWICBitmapFrameDecode> frame;if(FAILED(decoder->GetFrame(0,&frame)))return false;
        ComPtr<IWICFormatConverter> converter;if(FAILED(factory->CreateFormatConverter(&converter))||FAILED(converter->Initialize(frame.Get(),GUID_WICPixelFormat32bppPBGRA,WICBitmapDitherTypeNone,nullptr,0,WICBitmapPaletteTypeCustom)))return false;
        ComPtr<ID2D1Bitmap> bitmap;if(FAILED(target->CreateBitmapFromWicBitmap(converter.Get(),nullptr,&bitmap)))return false;
        found=images.emplace(id,std::move(bitmap)).first;
    }
    target->DrawBitmap(found->second.Get(),rectangle,opacity,D2D1_BITMAP_INTERPOLATION_MODE_LINEAR);
    return true;
}
}
