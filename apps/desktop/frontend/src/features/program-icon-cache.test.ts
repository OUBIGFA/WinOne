// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {readProgramIconCache,saveProgramIconCache} from './program-icon-cache';
afterEach(()=>localStorage.clear());
it('restores icons across a new page and ignores empty results',()=>{
 saveProgramIconCache({'example.exe':{name:'Example',icon:'AQID'},'empty.exe':{name:'Empty'}});
 expect(readProgramIconCache()).toEqual({'example.exe':{name:'Example',icon:'AQID'}});
});
it('rejects malformed cached metadata',()=>{localStorage.setItem('winone.program-icons.v1','{');expect(readProgramIconCache()).toEqual({});});
