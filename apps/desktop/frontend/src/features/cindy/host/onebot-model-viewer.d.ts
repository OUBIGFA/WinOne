import type {DetailedHTMLProps,HTMLAttributes} from 'react';
declare module 'react' {
 namespace JSX {
  interface IntrinsicElements {
   'model-viewer':DetailedHTMLProps<HTMLAttributes<HTMLElement>,HTMLElement>&{src?:string;poster?:string;alt?:string;[attribute:string]:unknown};
  }
 }
}
