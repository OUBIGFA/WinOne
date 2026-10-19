// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {playChatDone,readChatSoundEnabled,unlockChatAudio,writeChatSoundEnabled} from "./chat-sound";
afterEach(()=>{localStorage.clear();});
it('remembers the completion sound preference and stays silent without WebAudio',()=>{
 expect(readChatSoundEnabled()).toBe(true);
 writeChatSoundEnabled(false);expect(readChatSoundEnabled()).toBe(false);
 writeChatSoundEnabled(true);expect(readChatSoundEnabled()).toBe(true);
 expect(()=>{unlockChatAudio();playChatDone();}).not.toThrow();
});
