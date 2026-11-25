#pragma once
#include <d2d1.h>
#include <string_view>
namespace winone::icons {
bool render(ID2D1RenderTarget* target, std::string_view name, bool dark, const D2D1_RECT_F& rectangle, float opacity = 1);
void release();
}
