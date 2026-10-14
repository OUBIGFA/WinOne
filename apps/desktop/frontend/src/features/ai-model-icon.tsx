import rules from "../../../../../internal/ai/model-icons.json";
import {AIProviderIcon} from "./ai-provider-icon";

// Share the family inventory with the native result host, including manual IDs.
const matchers=rules.map(rule=>({match:new RegExp(rule.pattern,"i"),icon:rule.icon}));
export function modelIcon(model:string){return matchers.find(rule=>rule.match.test(model.trim()))?.icon??"model";}
export function AIModelIcon({model,name}:{model:string;name?:string}){const icon=modelIcon(model);return <AIProviderIcon icon={icon==="model"&&name?modelIcon(name):icon}/>;}
