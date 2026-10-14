export interface PackageReference {kind:'module'|'runtime';id:string;version:string;manifestSha256:string}
interface PackageIdentity {id:string;name:{'zh-CN':string;'en-US':string}}
// Legacy module metadata omits kind. Runtime packages deliberately have no
// module name, switch, settings namespace or worker lifecycle.
export type PackageMetadata = {version:string;dependencies?:PackageReference[]} & (
 {kind?:'module';module:PackageIdentity} | {kind:'runtime';runtime:PackageIdentity}
);
export function packageName(metadata:PackageMetadata,language:'zh-CN'|'en-US'){
 return (metadata.kind==='runtime'?metadata.runtime:metadata.module).name[language];
}
export interface SharedRuntime extends PackageReference {
 name:PackageIdentity['name'];requiredBy:PackageReference[]|null;canRemove:boolean;error?:string;
}
