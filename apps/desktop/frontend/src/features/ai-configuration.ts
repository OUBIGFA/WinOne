// Public AI service preferences contain stable references, never credentials.
export type AIKeySlot={id:string;enabled:boolean;hasKey:boolean};
export type AIKeyPolicy={strategy:"round-robin"|"ordered";failover:boolean;maxAttempts:number;cooldownSeconds:number};
export type AIModelPrice={input?:number;output?:number;cache_read?:number;cache_write?:number;cache_write_1h?:number;tiers?:({above:number}&Omit<AIModelPrice,"tiers">)[]};
export type AIModelMetadata={searches?:boolean;contextWindow?:number;maxTokens?:number;compactionThreshold?:number;compactionPercent?:number;listedContextWindow?:number;maxContextWindow?:number;rate?:number;rateWas?:number;reasoning?:boolean;vision?:boolean;tools?:boolean;reasoningEffort?:string;tier?:string;same?:string;upstreamName?:string;api?:string;levels?:string[];efforts?:string[];tiers?:string[];price?:AIModelPrice};
export type AIProvider={limitRestOff?:boolean;id:string;name:string;authMode:"api-key"|"account";protocol?:string;baseUrl?:string;preset?:string;presetRegion?:string;agent?:string;agentBackend?:"builtin"|"plugin";models:string[];delisted?:string[];availableModels?:string[];modelNames?:Record<string,string>;modelFree?:Record<string,boolean>;modelMetadata?:Record<string,AIModelMetadata>;maxRPM?:number;maxConcurrency?:number;queueLimit?:number;queueWait?:number;hasKey:boolean;icon?:string;avatar?:string;keys?:AIKeySlot[];keyPolicy?:AIKeyPolicy;headers?:Record<string,string>;additionalTools?:boolean};
export type AISelection={provider:string;model:string};
// The Host still returns the old Magpie settings object while upgrading a
// profile. Keep it as an opaque compatibility bag so the save path can
// round-trip unknown legacy fields without exposing their removed UI.
export type AIConfiguration={revision:number;providers:AIProvider[];tasks:Record<string,AISelection>;engine:string;ocrEngine:string;target:string;prompt:string;showReasoning?:boolean;autoRetranslate?:boolean;magpieSettings?:Record<string,unknown>};

export type AIAgent={id:string;name:string;icon?:string;backend?:"builtin"|"plugin";plugin?:string};
