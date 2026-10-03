import {expect,it} from "vitest";
import {hitTest,moveMark,markBounds} from "./capture-editor-geometry";
import type {AnnotationMark} from "./capture-editor-model";

const box:AnnotationMark={kind:"rectangle",x:10,y:10,width:30,height:20,color:"#ff0000",strokeWidth:3};
it("selects the topmost matching shape and avoids the empty corners of an ellipse",()=>{
  expect(hitTest([box,{...box,kind:"ellipse"}],{x:25,y:20},0)).toBe(1);
  expect(hitTest([box,{...box,kind:"ellipse"}],{x:10,y:10},0)).toBe(0);
  expect(hitTest([box],{x:90,y:70},2)).toBe(-1);
});
it("uses distance from a stroke instead of its entire bounding box",()=>{
  const arrow:AnnotationMark={kind:"arrow",points:[{x:10,y:10},{x:90,y:70}],color:"#ff0000",strokeWidth:3};
  expect(hitTest([arrow],{x:50,y:40},3)).toBe(0);
  expect(hitTest([arrow],{x:80,y:15},3)).toBe(-1);
  expect(hitTest([{...arrow,points:[{x:0,y:50},{x:100,y:50}],strokeWidth:10}],{x:55,y:65},0)).toBe(0);
});
it("moves shapes and strokes within the source bounds without changing size",()=>{
  expect(moveMark(box,100,-100,100,80)).toMatchObject({x:70,y:0,width:30,height:20});
  const pen:AnnotationMark={kind:"pen",points:[{x:10,y:10},{x:20,y:40},{x:50,y:20}],color:"#ff0000",strokeWidth:3};
  expect(markBounds(pen)).toEqual({x:10,y:10,width:40,height:30});
  expect(moveMark(pen,100,100,100,80).points).toEqual([{x:60,y:50},{x:70,y:80},{x:100,y:60}]);
  expect(pen.points?.[0]).toEqual({x:10,y:10});
});
