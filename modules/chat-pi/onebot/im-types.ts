// Local host side of Cindy's IM lane contract; protocol fields and semantics
// match apps/desktop/src/main/im/shared/types.ts at the pinned Cindy revision.
export type ImCompanionLane =
 | {kind:'owner-dm'}
 | {kind:'group';chatId:string;threadId:string}
 | {kind:'guest-dm';userId:string};
