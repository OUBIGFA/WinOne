import type {CropRect,Point} from "./capture-editor-model";

export interface SelectionCandidates {region:CropRect|null;x:number;y:number;windows:CropRect[]}
export interface CaptureSelectionPort {
 mode:"window"|"region";
 saveImmediately?:boolean;
 candidates(point?:Point):Promise<SelectionCandidates>;
 commit(crop:CropRect):Promise<void>;
}
export function selectionAt(windows:CropRect[],point:Point):CropRect|undefined {
 return windows.find(rect=>point.x>=rect.x&&point.y>=rect.y&&point.x<rect.x+rect.width&&point.y<rect.y+rect.height);
}
export function draggedSelection(start:Point,end:Point,width:number,height:number):CropRect|undefined {
 const x=Math.max(0,Math.min(width,Math.floor(Math.min(start.x,end.x)))),y=Math.max(0,Math.min(height,Math.floor(Math.min(start.y,end.y))));
 const right=Math.max(0,Math.min(width,Math.ceil(Math.max(start.x,end.x)))),bottom=Math.max(0,Math.min(height,Math.ceil(Math.max(start.y,end.y))));
 return right>x&&bottom>y?{x,y,width:right-x,height:bottom-y}:undefined;
}
