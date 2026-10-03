import type {AnnotationDocument} from "./capture-editor-model";
import {i18n} from "../i18n";

interface DraftPort {
 update:(revision:number,document:AnnotationDocument)=>Promise<{draftId:string;revision:number}>;
 read:()=>Promise<{revision:number;document:AnnotationDocument}>;
}
// Native DTOs include defaults for every tool. Compare rendering properties,
// not incidental fields, when reconciling an acknowledgement lost in transit.
export const documentKey=(document:AnnotationDocument)=>JSON.stringify({version:document.version,width:document.width,height:document.height,crop:document.crop??undefined,marks:document.marks.map(mark=>({
 kind:mark.kind,color:mark.color,strokeWidth:mark.strokeWidth,
 ...(mark.kind==="pen"||mark.kind==="arrow"?{points:mark.points}:{x:mark.x,y:mark.y,width:mark.width,height:mark.height}),
 ...(mark.kind==="text"?{text:mark.text??"",fontSize:mark.fontSize??24}:{}),
 ...(mark.kind==="mosaic"||mark.kind==="blur"?{effectSize:mark.effectSize??12}:{}),
}))});

export class DraftSession {
 private tail:Promise<void>=Promise.resolve();
 private pending?:AnnotationDocument;
 private confirmed:string;
 private attempted?:string;
 private fault:unknown;
 constructor(readonly id:string,public revision:number,document:AnnotationDocument,private port:DraftPort){this.confirmed=documentKey(document);}
 get failed(){return this.fault!==undefined;}
 save(document:AnnotationDocument):Promise<void> {
  this.pending=document;
  this.tail=this.tail.then(async()=>{
   const next=this.pending;if(!next)return;this.pending=undefined;
   const signature=documentKey(next);if(signature===this.confirmed)return;
   this.attempted=signature;
   try {
    const acknowledgement=await this.port.update(this.revision,next);
    if(acknowledgement.draftId!==this.id||acknowledgement.revision!==this.revision+1)throw Error("Invalid draft acknowledgement");
    this.revision=acknowledgement.revision;this.confirmed=signature;this.attempted=undefined;
   }catch(error){this.pending??=next;this.fault=error;throw error;}
  });
  return this.tail;
 }
 async settle(){let observed:Promise<void>;do{observed=this.tail;await observed;}while(observed!==this.tail);}
 async recover() {
  try{await this.tail;}catch{/* Reconcile explicitly; never retry an uncertain write blindly. */}
  if(!this.failed)return;
  const remote=await this.port.read();
  const signature=documentKey(remote.document);
  if(!Number.isSafeInteger(remote.revision)||remote.revision<this.revision||signature!==this.confirmed&&signature!==this.attempted)throw Error(i18n.t("capture.draftConflict"));
  this.revision=remote.revision;this.confirmed=signature;this.attempted=undefined;this.fault=undefined;this.tail=Promise.resolve();
 }
}
