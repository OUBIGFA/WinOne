import { describe, expect, it } from "vitest";
import { imagePoint, gestureMark, createHistory, restoreHistory, commitMark, commitCrop, replaceMark, removeMark, undo, redo, type AnnotationMark } from "./capture-editor-model";

const mark: AnnotationMark = {kind:"cover", x:10,y:20,width:30,height:40,color:"#000000",strokeWidth:3};

describe("original-image annotation coordinates", () => {
  it("converts a scaled preview and clamps a drag outside its edges", () => {
    const view = {left:100,top:50,width:800,height:400};
    expect(imagePoint(500,250,view,3200,1600)).toEqual({x:1600,y:800});
    expect(imagePoint(-10,900,view,3200,1600)).toEqual({x:0,y:1600});
    expect(imagePoint(100.1,50.2,view,3200,1600)).toEqual({x:0,y:1});
    expect(() => imagePoint(0,0,{...view,width:0},3200,1600)).toThrow();
  });
  it("normalizes reverse drags and ignores zero-size gestures", () => {
    expect(gestureMark("rectangle",[{x:50,y:60},{x:10,y:20}],"#ff0000",3)).toEqual({kind:"rectangle",x:10,y:20,width:40,height:40,color:"#ff0000",strokeWidth:3});
    expect(gestureMark("arrow",[{x:10,y:20},{x:10,y:20}],"#ff0000",3)).toBeNull();
    expect(gestureMark("cover",[{x:10,y:20},{x:10.1,y:20.1}],"#ff0000",3)).toBeNull();
  });
});

describe("annotation history", () => {
  it("restores persisted marks and crop without inventing undo history",()=>{
    const document={version:2 as const,width:100,height:80,marks:[mark],crop:{x:1,y:1,width:80,height:60}};
    const restored=restoreHistory(document);
    expect(restored.document).toEqual(document);expect(restored.past).toEqual([]);
    restored.document.marks[0].color="#ffffff";
    expect(document.marks[0].color).toBe("#000000");
    expect(()=>restoreHistory({...document,width:1})).toThrow();
  });
  it("replaces and deletes in place while preserving order, crop and undo",()=>{
    const first=commitMark(createHistory(100,80),mark);
    const second=commitCrop(commitMark(first,{...mark,color:"#ffffff"}),{x:1,y:1,width:80,height:70});
    const changed=replaceMark(second,0,{...mark,color:"#00ff00"});
    expect(changed.document.marks.map(m=>m.color)).toEqual(["#00ff00","#ffffff"]);
    expect(changed.document.crop).toEqual({x:1,y:1,width:80,height:70});
    expect(undo(changed).document).toEqual(second.document);
    const removed=removeMark(changed,0);
    expect(removed.document.marks.map(m=>m.color)).toEqual(["#ffffff"]);
    expect(undo(removed).document).toEqual(changed.document);
    expect(()=>replaceMark(second,2,mark)).toThrow();
    expect(()=>removeMark(second,-1)).toThrow();
    expect(()=>replaceMark(second,0,{...mark,x:99})).toThrow();
  });
  it("keeps effect strength in history and rejects unsupported values",()=>{
    for(const kind of ["mosaic","blur"] as const) {
      const start=createHistory(100,80);
      const effect={...mark,kind,effectSize:24};
      expect(redo(undo(commitMark(start,effect))).document.marks[0]).toEqual(effect);
      for(const effectSize of [0,1,65,NaN,2.5])expect(()=>commitMark(start,{...effect,effectSize})).toThrow();
    }
  });
  it("undoes crop and annotation changes together without shifting source coordinates",()=>{
    const marked=commitMark(createHistory(3200,1600),mark);
    const cropped=commitCrop(marked,{x:100,y:50,width:800,height:600});
    expect(cropped.document).toEqual({version:2,width:3200,height:1600,marks:[mark],crop:{x:100,y:50,width:800,height:600}});
    expect(undo(cropped).document).toEqual(marked.document);
    expect(redo(undo(cropped)).document).toEqual(cropped.document);
    expect(commitCrop(cropped,undefined).document).toEqual(marked.document);
    expect(commitMark(cropped,{...mark,color:"#ffffff"}).document.crop).toEqual({x:100,y:50,width:800,height:600});
  });
  it("rejects invalid crops and treats the full image as no crop",()=>{
    const start=createHistory(100,80);
    for(const crop of [{x:-1,y:0,width:10,height:10},{x:90,y:0,width:11,height:10},{x:0,y:0,width:0,height:10},{x:0.1,y:0,width:10,height:10}])expect(()=>commitCrop(start,crop)).toThrow();
    expect(commitCrop(start,{x:0,y:0,width:100,height:80})).toBe(start);
  });
  it("preserves multiline text and rejects invalid text parameters", () => {
    const start=createHistory(3200,1600);
    const textMark={...mark,kind:"text" as const,text:"项目进度\nWinOne",fontSize:32};
    const edited=commitMark(start,textMark);
    expect(redo(undo(edited)).document.marks[0]).toEqual(textMark);
    for(const invalid of [{text:""},{text:"a".repeat(4097)},{fontSize:0},{fontSize:NaN},{fontSize:257}]) {
      expect(()=>commitMark(start,{...textMark,...invalid})).toThrow();
    }
  });
  it("keeps original resolution through undo, redo and a new branch", () => {
    const start = createHistory(3200,1600);
    const first = commitMark(start,mark);
    const second = commitMark(first,{...mark,color:"#ffffff"});
    expect(undo(second).document).toEqual({version:1,width:3200,height:1600,marks:[mark]});
    expect(redo(undo(second)).document.marks).toHaveLength(2);
    const branch=commitMark(undo(second),{...mark,color:"#00ff00"});
    expect(redo(branch).document.marks.map(m=>m.color)).toEqual(["#000000","#00ff00"]);
    expect(start.document.marks).toHaveLength(0);
  });
  it("rejects out-of-bounds marks without corrupting history", () => {
    const start=createHistory(100,100);
    expect(()=>commitMark(start,{...mark,x:90})).toThrow();
    expect(()=>commitMark(start,{...mark,width:NaN})).toThrow();
    expect(start.document.marks).toHaveLength(0);
  });
  it("bounds history and detaches caller-owned points", () => {
    const points=[{x:1,y:1},{x:20,y:20}];
    let history=commitMark(createHistory(100,100),{kind:"pen",points,color:"#ff0000",strokeWidth:3});
    points[0].x=90;
    expect(history.document.marks[0].points?.[0].x).toBe(1);
    for(let i=0;i<120;i++)history=commitMark(history,mark);
    expect(history.past.length).toBeLessThanOrEqual(100);
  });
});
