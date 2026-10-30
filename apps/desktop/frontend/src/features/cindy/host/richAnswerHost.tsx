// WinOne's local owner namespace and existing Base UI primitives for the
// original rich-answer component. This does not create a Cindy sign-in state.
import {Slider as Primitive} from '@base-ui/react/slider';
// The id names WinOne's single data owner for owner-scoped local storage.
const localOwner={user:{id:'winone-local'},dataOwnerId:'winone-local'};
export function useAuth(){return localOwner;}
// Cindy's Radix-style Slider API over Base UI: the thumb carries the accessible
// name and value text; `onValueCommit` fires when a drag or key change settles.
export function Slider({value,onValueChange,onValueCommit,id,'aria-label':ariaLabel,'aria-describedby':ariaDescribedBy,'aria-valuetext':ariaValueText,...props}:{value:number[];onValueChange:(value:number[])=>void;onValueCommit?:(value:number[])=>void;id?:string;min?:number;max?:number;step?:number;disabled?:boolean;className?:string;'aria-label'?:string;'aria-describedby'?:string;'aria-valuetext'?:string}){
 return <Primitive.Root {...props} value={value} onValueChange={next=>onValueChange(Array.isArray(next)?[...next]:[next])} onValueCommitted={onValueCommit?next=>onValueCommit(Array.isArray(next)?[...next]:[next]):undefined}>
  <Primitive.Control className="relative flex h-6 w-full items-center">
   <Primitive.Track className="h-1 w-full rounded-full bg-[var(--border-default)]"><Primitive.Indicator className="rounded-full bg-[var(--text-secondary)]"/></Primitive.Track>
   {value.map((_,index)=><Primitive.Thumb key={index} index={index} id={index===0?id:undefined} aria-label={ariaLabel} aria-describedby={ariaDescribedBy} getAriaValueText={ariaValueText===undefined?undefined:()=>ariaValueText} className="size-4 rounded-full border border-[var(--text-secondary)] bg-[var(--surface)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)]"/>)}
  </Primitive.Control>
 </Primitive.Root>;
}
