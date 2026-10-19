import type {ReviewJumpTarget,ReviewSourceDescriptor} from './shared/reviewSource';
import type {DiffViewMode} from './DiffViewer/diffRows';

// Cindy's persisted review state; the surrounding tab remains a WinOne tab.
export interface ReviewState {
 descriptor:ReviewSourceDescriptor;messageSnapshot:Extract<ReviewSourceDescriptor,{kind:'turn-set'}>|null;jumpTarget:ReviewJumpTarget|null;
 diffsExpanded:boolean;diffViewMode:DiffViewMode;fileTreeVisible:boolean;wordWrap:boolean;wordDiff:boolean;hideWhitespace:boolean;richMarkdownPreview:boolean;branchBaseRef:string|null;
}
export interface TabKindHostContext {
 tabId:string;sessionId:string;workdir:string;remoteHostId:null;deviceLinkDeviceId:null;patchState:(patch:Partial<ReviewState>)=>void;onVisibilityChange:(visible:boolean)=>void;setCloseInterceptor:(interceptor:(()=>Promise<boolean>)|null)=>(()=>void);
}
export const defaultReviewState:ReviewState={descriptor:{kind:'unstaged'},messageSnapshot:null,jumpTarget:null,diffsExpanded:true,diffViewMode:'unified',fileTreeVisible:false,wordWrap:false,wordDiff:false,hideWhitespace:false,richMarkdownPreview:true,branchBaseRef:null};
