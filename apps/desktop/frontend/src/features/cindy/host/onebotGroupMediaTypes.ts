// Type-only boundary of Cindy's generic image viewer. Group messages render
// user attachments; they do not load the unrelated agent process-row renderer.
export interface ToolMediaModelFileCindy {provider: 'cindy'; url: string; format: string}
export type ToolMediaModelFile = ToolMediaModelFileCindy;
