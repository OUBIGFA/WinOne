import {expect,it} from "vitest";
import {resizeCrop,moveCrop} from "./capture-crop-resize";

it("moves a selection without resizing it and clamps it to the source",()=>{
 const crop={x:20,y:30,width:100,height:80};
 expect(moveCrop(crop,15,20,300,200)).toEqual({x:35,y:50,width:100,height:80});
 expect(moveCrop(crop,-500,500,300,200)).toEqual({x:0,y:120,width:100,height:80});
 expect(moveCrop({x:0,y:0,width:300,height:200},50,50,300,200)).toEqual({x:0,y:0,width:300,height:200});
});

it("moves each selected edge while preserving the opposite edges",()=>{
 const crop={x:20,y:30,width:100,height:80};
 expect(resizeCrop(crop,"left",-10,20,300,200)).toEqual({x:10,y:30,width:110,height:80});
 expect(resizeCrop(crop,"bottom-right",40,50,300,200)).toEqual({x:20,y:30,width:140,height:130});
 expect(resizeCrop(crop,"top",50,-10,300,200)).toEqual({x:20,y:20,width:100,height:90});
});
it("keeps a positive integer selection inside the image without flipping",()=>{
 const crop={x:20,y:30,width:100,height:80};
 expect(resizeCrop(crop,"top-left",500,500,300,200)).toEqual({x:119,y:109,width:1,height:1});
 expect(resizeCrop(crop,"bottom-right",500,500,300,200)).toEqual({x:20,y:30,width:280,height:170});
 expect(resizeCrop(crop,"left",-500,0,300,200)).toEqual({x:0,y:30,width:120,height:80});
 expect(resizeCrop(crop,"right",1.7,0,300,200).width).toBe(102);
});
