// Pi 1.4 migrates SQLite to schema 2, which 1.3 cannot reopen. Preserve a
// consistent pre-migration database, including committed WAL pages, first.
import fs from 'node:fs/promises';
import {randomUUID} from 'node:crypto';
import {DatabaseSync} from 'node:sqlite';
import {openNodeSqliteStorage} from '@earendil-works/pi-durable/storage/sqlite/node';

export async function openDurableStorage(file){
 let existing;
 try{existing=await fs.lstat(file);}catch(error){if(error.code!=='ENOENT')throw error;}
 if(existing){
  if(!existing.isFile()||existing.isSymbolicLink())throw Error('Durable storage must be a regular file');
  const database=new DatabaseSync(file,{readOnly:true});
  try{
   const table=database.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name='durable_schema'").get();
   const version=table?database.prepare('SELECT version FROM durable_schema WHERE singleton=1').get()?.version:0;
   if(version===1){
    // VACUUM INTO refuses to overwrite a nonempty file. A unique destination
    // also preserves earlier rollback copies after a failed migration retry.
    const backup=file+'.pre-pi-1.4-'+randomUUID()+'.sqlite';
    database.prepare('VACUUM INTO ?').run(backup);
    const copy=new DatabaseSync(backup,{readOnly:true});
    try{
     if(copy.prepare('PRAGMA quick_check').get()?.quick_check!=='ok'||copy.prepare('SELECT version FROM durable_schema WHERE singleton=1').get()?.version!==1)throw Error('Durable migration backup verification failed');
    }finally{copy.close();}
   }
  }finally{database.close();}
 }
 return openNodeSqliteStorage(file);
}
