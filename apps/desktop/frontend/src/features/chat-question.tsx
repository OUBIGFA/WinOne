import {useState} from 'react';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Textarea} from '../components/ui/textarea';
import {Field,FieldLabel} from '../components/ui/field';
import {SendButton} from './cindy/SendButton';
import type {Approval} from './chat-state';

// Reuse the pending-operation surface and existing controls. Choices only
// fill a draft answer; selecting one never sends it or grants a permission.
export function ChatQuestion({request,busy,onAnswer,onCancel,onStop}:{request:Approval;busy:boolean;onAnswer:(answers:string[])=>void;onCancel:()=>void;onStop:()=>void}){
 const {t}=useTranslation(),questions=request.questions??[];
 const [answers,setAnswers]=useState(()=>questions.map(()=>''));
 const complete=questions.length>0&&answers.length===questions.length&&answers.every(value=>value.trim());
 const change=(index:number,answer:string)=>setAnswers(current=>current.map((value,i)=>i===index?answer:value));
 return <section className="chat-approval" aria-label={t('chat.questionTitle')}>
  <div className="chat-approval-heading"><h3>{t('chat.questionTitle')}</h3><span className="cindy-chat-ui"><SendButton disabled={busy} isStreaming onClick={onStop}/></span></div>
  <div className="max-h-[50vh] overflow-y-auto space-y-4">
   {questions.map((question,index)=><Field key={index}>
    <FieldLabel htmlFor={request.id+'-answer-'+index}>{question.question}</FieldLabel>
    {!!question.options?.length&&<div className="flex flex-wrap gap-2" role="group" aria-label={question.question}>{question.options.map((option,i)=><Button key={i} disabled={busy} aria-pressed={answers[index]===option} onClick={()=>change(index,option)}>{option}</Button>)}</div>}
    <Textarea id={request.id+'-answer-'+index} value={answers[index]??''} onChange={event=>change(index,event.target.value)} placeholder={t('chat.questionHint')} maxLength={4000} disabled={busy}/>
   </Field>)}
  </div>
  <div className="chat-approval-actions"><Button disabled={busy} onClick={onCancel}>{t('chat.questionCancel')}</Button><Button variant="primary" disabled={busy||!complete} onClick={()=>onAnswer(answers)}>{t('chat.questionSubmit')}</Button></div>
 </section>;
}
