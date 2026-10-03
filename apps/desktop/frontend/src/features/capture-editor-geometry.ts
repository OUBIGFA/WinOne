import type {AnnotationMark,CropRect,Point} from "./capture-editor-model";

export function markBounds(mark:AnnotationMark):CropRect {
  if(mark.kind!=="pen"&&mark.kind!=="arrow")return {x:mark.x!,y:mark.y!,width:mark.width!,height:mark.height!};
  const points=mark.points!;
  const x=Math.min(...points.map(p=>p.x)),y=Math.min(...points.map(p=>p.y));
  return {x,y,width:Math.max(...points.map(p=>p.x))-x,height:Math.max(...points.map(p=>p.y))-y};
}

function segmentDistance(point:Point,a:Point,b:Point):number {
  const dx=b.x-a.x,dy=b.y-a.y,length=dx*dx+dy*dy;
  const ratio=length?Math.max(0,Math.min(1,((point.x-a.x)*dx+(point.y-a.y)*dy)/length)):0;
  return Math.hypot(point.x-a.x-ratio*dx,point.y-a.y-ratio*dy);
}

// Test topmost-first in original pixels. Stroke tolerance is supplied by the
// view so selecting a thin line stays practical at every preview zoom level.
export function hitTest(marks:AnnotationMark[],point:Point,tolerance:number):number {
  for(let index=marks.length-1;index>=0;index--) {
    const mark=marks[index];
    if(mark.kind==="pen"||mark.kind==="arrow") {
      const points=mark.points!;
      if(points.slice(1).some((p,i)=>segmentDistance(point,points[i],p)<=tolerance+mark.strokeWidth/2))return index;
      if(mark.kind==="arrow") {
        const [start,end]=points,dx=end.x-start.x,dy=end.y-start.y,length=Math.hypot(dx,dy);
        const along=-((point.x-end.x)*dx+(point.y-end.y)*dy)/length;
        const across=Math.abs((point.x-end.x)*dy-(point.y-end.y)*dx)/length;
        if(along>=-tolerance&&along<=mark.strokeWidth*5+tolerance&&across<=Math.max(0,along)*0.4+tolerance)return index;
      }
      continue;
    }
    const bounds=markBounds(mark);
    if(mark.kind==="ellipse") {
      const rx=bounds.width/2+tolerance,ry=bounds.height/2+tolerance;
      if(((point.x-bounds.x-bounds.width/2)/rx)**2+((point.y-bounds.y-bounds.height/2)/ry)**2<=1)return index;
    } else if(point.x>=bounds.x-tolerance&&point.y>=bounds.y-tolerance&&point.x<=bounds.x+bounds.width+tolerance&&point.y<=bounds.y+bounds.height+tolerance)return index;
  }
  return -1;
}

export function moveMark(mark:AnnotationMark,dx:number,dy:number,width:number,height:number):AnnotationMark {
  if(![dx,dy,width,height].every(Number.isFinite))throw new Error("Invalid movement");
  const bounds=markBounds(mark);
  const x=Math.max(-bounds.x,Math.min(width-bounds.x-bounds.width,Math.round(dx)));
  const y=Math.max(-bounds.y,Math.min(height-bounds.y-bounds.height,Math.round(dy)));
  return mark.kind==="pen"||mark.kind==="arrow"?{...mark,points:mark.points!.map(p=>({x:p.x+x,y:p.y+y}))}:{...mark,x:mark.x!+x,y:mark.y!+y};
}
