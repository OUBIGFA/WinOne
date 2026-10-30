// WinOne seam for the @cindy/voice-input-core types that shared voice
// conversation contracts reference (Cindy b7e46bc, Apache-2.0). Voice input and
// recognition run in the WinOne Host speech service; the renderer only reads
// these event and trace shapes.
export * from '../../../../../../../third_party/cindy/voice-core/types';
export * from '../../../../../../../third_party/cindy/voice-core/refinementContext';
export * from '../../../../../../../third_party/cindy/voice-core/dictionaryLearningPolicy';
export {hasPcmSound} from '../../../../../../../third_party/cindy/voice-core/pcmActivity';
export {DEFAULT_SPEAKER_GATE_STRICTNESS, isSpeakerGateStrictness, type SpeakerGateStrictness} from '../../../../../../../third_party/cindy/voice-core/SpeakerGate';
export type {ConversationInputEvent} from '../../../../../../../third_party/cindy/voice-core/VoiceConversationInput';
export type {DictationDictionaryAdviceInput, DictationDictionaryLearningAction, DictationDictionaryLearningCandidateState, DictationDictionaryLearningEntryState} from '../../../../../../../third_party/cindy/voice-core/DictationDictionaryAdvisor';
