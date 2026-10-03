import {expect,it} from "vitest";
import {validCapturePlacement} from "./capture-draft-api";

const placement={x:-1800,y:100,desktopX:-1920,desktopY:0,desktopWidth:3840,desktopHeight:1080};
it("accepts physical coordinates on a monitor left of the primary display",()=>{
 expect(validCapturePlacement(placement,40,30)).toBe(true);
});
it("rejects overflowing, fractional and out-of-desktop placement",()=>{
 for(const value of [{...placement,x:-2000},{...placement,y:1070},{...placement,desktopWidth:0},{...placement,desktopX:2147483647},{...placement,x:0.5}])expect(validCapturePlacement(value,40,30)).toBe(false);
 expect(validCapturePlacement(placement,40.5,30)).toBe(false);
});
