export type GesturePoint={x:number;y:number};
export type StrokeGesture={id:string;name:string;points?:GesturePoint[];vectors?:number[][]};
export type StrokeAction={id:string;name:string;gesture:string;code:string;enabled:boolean};
export type StrokePackage={id:string;name:string;actions:StrokeAction[]};
export type StrokeScope={id:string;name:string;kind:"explorer-folders"|"application"|"global";enabled:boolean;processPath?:string;pattern?:string;windowClasses?:string[];packages:string[]};
export type StrokePreferences={version:number;revision:number;defaultPolicy:"disabled";triggerButton:string;pen:{enabled?:boolean;color:string;opacity:number;thickness:number};gestures:StrokeGesture[];actionPackages:StrokePackage[];scopes:StrokeScope[];filters:string[];scriptThreadCount:number;scriptTimeoutSeconds:number;namespaces:string[];assemblies:string[]};
export type StrokeWindow={window:number;processId:number;processPath:string;windowClass:string;title:string};
export const strokeId=()=>crypto.randomUUID().replaceAll("-","");
