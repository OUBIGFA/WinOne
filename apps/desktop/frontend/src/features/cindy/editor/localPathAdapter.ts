// Cindy's lossless path carrier; FileEditor resolves it through WinOne before
// assigning an image URL, so the WebView never gains filesystem access.
export {resolveLocalPath} from '../localPathResolver';
export function toLocalFileUrl(absPath:string,revision?:string):string{
 const url=`xdt-file://local/?path=${encodeURIComponent(absPath)}`;
 return revision?`${url}&v=${encodeURIComponent(revision)}`:url;
}
