// Durable plan/todo and structured questions. Inspired by oh-my-pi's todo/ask
// surfaces and Pi Durable's official plan-mode example. All state is committed
// before UI updates, so reopening never resets a plan or fabricates an answer.
import {Type} from '@earendil-works/pi-ai';
import {defineDoc,defineTool,defineExtension,section} from '@earendil-works/pi-durable';

// Keep branch-rewindable plan changes as deltas under Durable 1.4.
export const WorkflowDoc=defineDoc({kind:'winone.workflow',version:1,scope:'conversation',history:'rewindable',fork:'asOf',initial:()=>({items:[],explanation:''}),checkpointWhen:()=>false});
const reply=value=>({output:[{type:'text',text:JSON.stringify(value)}],details:{workflow:value}});
const validStatus=['pending','in_progress','completed'];

export function createWorkflow({question,settings,onChange=()=>{}}){
 const todo=defineTool({name:'update_plan',description:'Keep a durable ordered plan. Supply the complete current plan and statuses. Preserve completed steps; only one step may be in progress.',parameters:Type.Object({explanation:Type.Optional(Type.String({maxLength:2000})),plan:Type.Array(Type.Object({step:Type.String({minLength:1,maxLength:1000}),status:Type.Union(validStatus.map(value=>Type.Literal(value)))}),{maxItems:100})}),replay:'safe',execute:async(args,api,ctx)=>{
  if(args.plan.filter(item=>item.status==='in_progress').length>1)throw Error('Only one plan step may be in progress');
  const value={items:args.plan,explanation:args.explanation??''};
  await api.commit(async tx=>{const document=await tx.doc(WorkflowDoc,api.conversationId);document.items=value.items;document.explanation=value.explanation;},ctx);
  onChange();return reply(value);
 }});
 const ask=defineTool({name:'ask_user_question',description:'Ask the user up to three concrete questions and await their answers. Give clear choices where useful. Cancellation is not an answer and never authorizes an action.',parameters:Type.Object({questions:Type.Array(Type.Object({question:Type.String({minLength:1,maxLength:512}),options:Type.Optional(Type.Array(Type.String({minLength:1,maxLength:256}),{minItems:2,maxItems:6}))}),{minItems:1,maxItems:3})}),replay:'unsafe',execute:async(args,api,ctx)=>{
  const request={id:api.callId,questions:args.questions};await api.details({question:request},ctx);
  const answers=await question(request,ctx.abortSignal);
  if(!Array.isArray(answers)||answers.length!==args.questions.length||answers.some(answer=>typeof answer!=='string'||!answer.trim()||answer.length>4000))throw Error('The user did not provide valid answers');
  return {output:[{type:'text',text:JSON.stringify(args.questions.map((q,i)=>({question:q.question,answer:answers[i]})))}],details:{questions:args.questions,answers}};
 }});
 const extension=defineExtension({name:'winone.workflow',tools:settings.planning===false?[ask]:[todo,ask],sections:[section('work_mode',()=>settings.toolPreset==='read-only'?'Plan and investigate only. Do not modify files or execute commands. Use update_plan to preserve concrete steps.':undefined)]});
 return {extension};
}
