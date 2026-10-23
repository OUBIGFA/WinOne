import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {DatabaseSync} from 'node:sqlite';
import {NodeSqliteDatabase} from '@earendil-works/chord/sqlite/node';
import {applySqliteMigrations,SQLITE_MIGRATIONS} from '@earendil-works/pi-durable/storage/sqlite';
import {openDurableStorage} from './durable-storage.mjs';

test('Pi 1.4 preserves a readable schema-1 rollback copy including committed WAL pages',async t=>{
 const directory=await fs.mkdtemp(path.join(os.tmpdir(),'winone-pi-migration-'));
 // Bun's node:sqlite compatibility layer releases prepared statements at GC.
 // Finalize fixture handles before Windows removes their database files.
 t.after(()=>{globalThis.Bun?.gc(true);return fs.rm(directory,{recursive:true,force:true});});
 const file=path.join(directory,'session.sqlite');
 const source=new DatabaseSync(file);const adapter=new NodeSqliteDatabase(source);
 source.exec('PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0');
 await applySqliteMigrations(adapter,[SQLITE_MIGRATIONS[0]]);
 source.exec("CREATE TABLE migration_fixture (value TEXT); INSERT INTO migration_fixture VALUES ('committed history')");
 const oldCopy=path.join(directory,'session.sqlite.pre-pi-1.4-existing.sqlite');await fs.writeFile(oldCopy,'keep older rollback');
 const storage=await openDurableStorage(file);await storage.close();
 const backups=(await fs.readdir(directory)).filter(name=>name.startsWith('session.sqlite.pre-pi-1.4-')&&name!==path.basename(oldCopy));
 assert.equal(backups.length,1);
 const backup=new DatabaseSync(path.join(directory,backups[0]),{readOnly:true});
 try{
  assert.equal(backup.prepare('SELECT version FROM durable_schema').get().version,1);
  assert.equal(backup.prepare('SELECT value FROM migration_fixture').get().value,'committed history');
 }finally{backup.close();}
 assert.equal(source.prepare('SELECT version FROM durable_schema').get().version,2);
 assert.equal(source.prepare('SELECT value FROM migration_fixture').get().value,'committed history');
 await adapter.close();
 assert.equal(await fs.readFile(oldCopy,'utf8'),'keep older rollback');
 const reopened=await openDurableStorage(file);await reopened.close();
 assert.deepEqual((await fs.readdir(directory)).filter(name=>name.startsWith('session.sqlite.pre-pi-1.4-')&&name!==path.basename(oldCopy)),backups);
});

test('new Pi storage needs no migration backup',async t=>{
 const directory=await fs.mkdtemp(path.join(os.tmpdir(),'winone-pi-new-'));
 t.after(()=>{globalThis.Bun?.gc(true);return fs.rm(directory,{recursive:true,force:true});});
 const storage=await openDurableStorage(path.join(directory,'new.sqlite'));await storage.close();
 // WAL/SHM companions are SQLite runtime files, not migration backups.
 assert.ok((await fs.readdir(directory)).includes('new.sqlite'));
 assert.ok(!(await fs.readdir(directory)).some(name=>name.includes('.pre-pi-1.4-')));
});
