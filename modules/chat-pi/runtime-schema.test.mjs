// Real SQLite indexes/defaults from the pinned Drizzle schema, including the
// expression/partial indexes that serialize Orca worker leases and labels.
import test from 'node:test';
import assert from 'node:assert/strict';
import {Database} from 'bun:sqlite';
import {createRuntimeTable} from './runtime-schema.mjs';
import {sessions,orcaTeams,orcaWorkers,orcaWorkerCreationReservations} from './cindy/tasks/db/schema.ts';

test('derived runtime schema retains expression and partial unique indexes',()=>{
 const db=new Database(':memory:');
 try{
  db.exec('PRAGMA foreign_keys=ON');
  for(const table of [sessions,orcaTeams,orcaWorkers,orcaWorkerCreationReservations])createRuntimeTable(db,table,table!==sessions);
  db.query('INSERT INTO sessions(id,created_at,updated_at) VALUES (?,?,?)').run('lead',1,1);
  db.query('INSERT INTO sessions(id,created_at,updated_at) VALUES (?,?,?)').run('worker',1,1);
  db.query('INSERT INTO sessions(id,created_at,updated_at) VALUES (?,?,?)').run('other',1,1);
  db.query('INSERT INTO orca_teams(id,lead_session_id,created_at,updated_at) VALUES (?,?,?,?)').run('team','lead',1,1);
  assert.throws(()=>db.query('INSERT INTO orca_teams(id,lead_session_id,created_at,updated_at) VALUES (?,?,?,?)').run('second','lead',1,1),/UNIQUE/);
  db.query('INSERT INTO orca_workers(id,team_id,session_id,label,created_at,updated_at) VALUES (?,?,?,?,?,?)').run('one','team','worker','Report',1,1);
  assert.throws(()=>db.query('INSERT INTO orca_workers(id,team_id,session_id,label,created_at,updated_at) VALUES (?,?,?,?,?,?)').run('two','team','other','report',1,1),/UNIQUE/);
  db.query('INSERT INTO orca_worker_creation_reservations(id,team_id,label,created_at,expires_at) VALUES (?,?,?,?,?)').run('reserve','team','New',1,2);
  assert.throws(()=>db.query('INSERT INTO orca_worker_creation_reservations(id,team_id,label,created_at,expires_at) VALUES (?,?,?,?,?)').run('again','team','new',1,2),/UNIQUE/);
 }finally{db.close(true);}
});
