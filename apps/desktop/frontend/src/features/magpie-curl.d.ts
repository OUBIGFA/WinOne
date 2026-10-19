/** Pure parser copied from Magpie; placeholders never become saved secrets. */
export interface CurlImport {url:string;api:string;base:string;urlHole:string;key:string;placeholder:string;model:string}
export function curlOf(text:string):CurlImport|null;
