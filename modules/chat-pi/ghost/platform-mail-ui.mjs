// Native-only presentation seam, over Cindy's original mail parser, sanitizer
// and editor functions. These transforms never authorize or send an email.
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {parseGhostCardFormValues,parseMailAttachmentItems,visibleMailAttachments,mailPreviewImages,canInsertMailImage,removeMailAttachment,insertMailImage,ghostCardRecipients} from '../cindy/ghost/packages/maker-shared/src/ghostCardForm.ts';
import {mailInlineDocument,mailReadingDocument,hasMailExternalImages,validMailImageUrl} from '../cindy/ghost/packages/maker-shared/src/mailTextEdit.ts';

const invalid=()=>Object.assign(Error('邮件视图参数无效'),{code:'INVALID_PARAMS'});
let sdk;
function imagesOf(input){
 const images=input??[];
 if(!Array.isArray(images)||images.some(image=>!image||typeof image.cid!=='string'||typeof image.url!=='string'||!validMailImageUrl(image.url)))throw invalid();
 return images;
}
export async function nativeMailUi(input,root){
 if(input.op==='file'){
  const directory=await fs.realpath(path.join(root,'native-mail-ui'));
  const file=await fs.realpath(input.path);
  if(path.dirname(file)!==directory||!file.endsWith('.json'))throw invalid();
  const request=JSON.parse(await fs.readFile(file,'utf8'));
  const result=await nativeMailUi(request,root);
  const output=file+'.result';await fs.writeFile(output,JSON.stringify(result),{flag:'wx',mode:0o600});
  return {path:output};
 }
 if(input.op==='document'){
  if(typeof input.source!=='string'||input.source.length>(input.reading===true?1024*1024:64000)||input.source.includes('\0'))throw invalid();
  const images=imagesOf(input.images);
  sdk??=fs.readFile(process.env.WINONE_GHOST_MAIL_UI_SDK??fileURLToPath(new URL('./ghost-mail-ui-sdk.js',import.meta.url)),'utf8');
  return {html:(input.reading===true?mailReadingDocument:mailInlineDocument)(input.source,images,input.allowExternalImages===true),hasExternalImages:hasMailExternalImages(input.source),script:await sdk};
 }
 let values;
 if(input.reading===true&&input.op==='model'){
  // Reader bodies have the original MailMessage 1 MiB limit; compose uses
  // the smaller form contract. Validate all other fields with that original
  // parser, then restore only these bounded, display-only body fields.
  const {bodyHtml,body,bodyText,...rest}=input.values??{};
  values=parseGhostCardFormValues({...rest,bodyHtml:'',body:'',bodyText:''});
  if(!values||[bodyHtml,body,bodyText].some(value=>value!==undefined&&(typeof value!=='string'||value.length>1024*1024)))throw invalid();
  values={...values,...(bodyHtml!==undefined?{bodyHtml}:{}),...(body!==undefined?{body}:{}),...(bodyText!==undefined?{bodyText}:{})};
 }else values=parseGhostCardFormValues(input.values);
 if(!values)throw invalid();
 if(input.op==='model')return {
  attachments:visibleMailAttachments(values),images:mailPreviewImages(values,imagesOf(input.images)),
  insertable:input.reading===true?[]:(parseMailAttachmentItems(values.attachmentItems)||[]).filter(file=>canInsertMailImage(values,file)).map(file=>file.key),
  recipients:Object.fromEntries(['to','cc','bcc'].map(field=>[field,ghostCardRecipients(values[field]??'')])),
 };
 if(input.reading===true||!['remove','insert'].includes(input.op)||typeof input.field!=='string'||typeof input.key!=='string')throw invalid();
 return {values:(input.op==='remove'?removeMailAttachment:insertMailImage)(values,input.field,input.key)};
}
