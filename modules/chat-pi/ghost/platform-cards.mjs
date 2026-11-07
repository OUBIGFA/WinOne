// Pure native presentation over Cindy's sanitized persisted cards. The original
// HTML remains the authority for forms/actions; the window never invents fields.
import {parseFragment} from 'parse5';
import {readGhostCardForm,readGhostCardFormChange,omitGhostCardFormPreview,readMailCard} from '../cindy/ghost/packages/maker-shared/src/ghostCardForm.ts';
import {ghostCardRootCallId} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';

export function nativeCard(card){
 if(!card)return null;
 const fields=readGhostCardForm(card.html);
 return {...card,rootCallId:ghostCardRootCallId(card.callId),viewHtml:fields?omitGhostCardFormPreview(card.html):card.html,
  fields:fields??[],formChange:readGhostCardFormChange(card.html)??'',mail:readMailCard(card.html)};
}
export function cardAttribute(html,attribute,matchAttribute,matchValue){
 const visit=node=>{
  if(node.attrs?.some(item=>item.name===matchAttribute&&item.value===matchValue)){
   const value=node.attrs.find(item=>item.name===attribute);if(value)return value.value;
  }
  for(const child of node.childNodes??[]){const found=visit(child);if(found!==undefined)return found;}
 };
 return visit(parseFragment(html));
}
