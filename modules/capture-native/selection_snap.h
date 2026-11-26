#pragma once
#include <windows.h>
#include <vector>
#include <algorithm>
#include <cmath>

namespace winone::capture {
// The candidate rectangles come from Tools3000's cached window/UIA detector.
// Snap only the moving edge and leave the cursor itself untouched. This keeps
// subpixel movement predictable and lets Alt bypass the magnet immediately.
inline LONG nearestEdge(LONG value,const std::vector<LONG>& edges,int threshold) {
    LONG nearest=value; int distance=threshold+1;
    for(const auto edge:edges)if(const int d=std::abs(edge-value);d<distance){distance=d;nearest=edge;}
    return distance<=threshold?nearest:value;
}
inline POINT snapPoint(POINT point,const std::vector<RECT>& candidates,int threshold) {
    std::vector<LONG> x,y;
    for(const auto& rect:candidates) {
        if(point.y>=rect.top-threshold && point.y<=rect.bottom+threshold){x.push_back(rect.left);x.push_back(rect.right);}
        if(point.x>=rect.left-threshold && point.x<=rect.right+threshold){y.push_back(rect.top);y.push_back(rect.bottom);}
    }
    return {nearestEdge(point.x,x,threshold),nearestEdge(point.y,y,threshold)};
}
}
