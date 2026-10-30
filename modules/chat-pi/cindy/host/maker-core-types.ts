// WinOne seam for the @cindy/maker-core types used by vendored scheduler-host
// code (b7e46bc). WinOne runs Pi through its own worker; there is no Maker.
export type AgentKind = 'claude-code' | 'codex' | 'pi';
export type Maker = never;
// Voice calls keep Cindy's call-local dialogue record unchanged.
export type {VoiceCallHandoff, VoiceCallRecord, VoiceCallTurn} from '../voice/voice-call';
export {VoiceCallContext} from '../voice/voice-call';
