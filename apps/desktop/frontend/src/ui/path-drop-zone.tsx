import type {ComponentProps,ReactNode} from "react";
import {usePathDrop,type PathDropOptions} from "./path-drop";

/** Preserve the existing input/div markup while sharing path validation and
 * native drop ownership in repeated directory and command fields. */
export function PathDropInput({onPaths,onError,kind,multiple,disabled,...props}:Omit<ComponentProps<"input">,"onError"|"multiple">&PathDropOptions){
 const drop=usePathDrop({onPaths,onError,kind,multiple,disabled});
 return <input {...props} {...drop} disabled={disabled}/>;
}
export function PathDropArea({children,className,...options}:PathDropOptions&{children:ReactNode;className?:string}){
 const drop=usePathDrop(options);
 return <div {...drop} className={className}>{children}</div>;
}
