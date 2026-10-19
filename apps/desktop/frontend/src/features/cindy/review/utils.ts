export {cn} from 'cn';
export function basename(path:string){return path.replaceAll('\\','/').split('/').pop()||path;}
