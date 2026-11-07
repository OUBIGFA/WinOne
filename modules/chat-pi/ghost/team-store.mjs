// Orca's pinned tables over the same owner-local receipt SQLite database.
// This adapter stores links and native input receipts only, never transcripts
// or a second Agent runtime; all work executes in ordinary Host-owned sessions.
import {randomUUID} from 'node:crypto';

export function createGhostTeamStore(ledger){
 const db=ledger.sqlite;
 const team=id=>db.query('SELECT * FROM orca_teams WHERE id=?').get(id);
 const active=lead=>db.query("SELECT * FROM orca_teams WHERE lead_session_id=? AND status='active'").get(lead);
 const workers=lead=>db.query('SELECT w.*,t.lead_session_id AS leadSessionId,t.status AS teamStatus FROM orca_workers w JOIN orca_teams t ON t.id=w.team_id WHERE t.lead_session_id=? ORDER BY w.created_at,w.id').all(lead);
 const worker=id=>db.query('SELECT w.*,t.lead_session_id AS leadSessionId,t.status AS teamStatus FROM orca_workers w JOIN orca_teams t ON t.id=w.team_id WHERE w.id=?').get(id);
 const session=id=>db.query('SELECT w.*,t.lead_session_id AS leadSessionId,t.status AS teamStatus FROM orca_workers w JOIN orca_teams t ON t.id=w.team_id WHERE w.session_id=?').get(id);
 return {team,active,workers,worker,session,
  create(lead){
   ledger.created(lead);const old=active(lead);if(old)return old;
   const now=Date.now(),id=randomUUID();db.query('INSERT INTO orca_teams(id,lead_session_id,created_at,updated_at) VALUES (?,?,?,?)').run(id,lead,now,now);return team(id);
  },
  end(id,status){const now=Date.now();db.query('UPDATE orca_teams SET status=?,completed_at=?,updated_at=? WHERE id=?').run(status,now,now,id);},
  put(input){
   ledger.created(input.sessionId);const now=Date.now();
   db.query('INSERT INTO orca_workers(id,team_id,session_id,status,label,role,focused,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,label=excluded.label,role=excluded.role,focused=excluded.focused,updated_at=excluded.updated_at')
    .run(input.id,input.teamId,input.sessionId,input.status,input.label,input.role,Number(input.focused),now,now);
  },
  report(id,value,status){db.query('UPDATE orca_workers SET pending_remote_report=?,status=COALESCE(?,status),updated_at=? WHERE id=?').run(JSON.stringify(value),status??null,Date.now(),id);},
  status(id,status){db.query('UPDATE orca_workers SET status=?,idle_since=?,updated_at=? WHERE id=?').run(status,status==='idle'?Date.now():null,Date.now(),id);},
  focus(id){const row=worker(id);if(!row)return false;db.transaction(()=>{db.query('UPDATE orca_workers SET focused=0 WHERE team_id=?').run(row.team_id);db.query('UPDATE orca_workers SET focused=1,updated_at=? WHERE id=?').run(Date.now(),id);})();return true;},
  identity(id,role,label){db.query('UPDATE orca_workers SET role=?,label=?,updated_at=? WHERE id=?').run(role,label,Date.now(),id);},
  remove(id){db.query('DELETE FROM orca_workers WHERE id=?').run(id);},
  reserve({reservationId,teamId,label,hardLimit,leaseMs}){
   return db.transaction(()=>{
    const now=Date.now();db.query('DELETE FROM orca_worker_creation_reservations WHERE expires_at<=?').run(now);
    if(db.query('SELECT id FROM orca_workers WHERE team_id=? AND lower(label)=lower(?)').get(teamId,label))return {ok:false,errorCode:'DUPLICATE_LABEL'};
    if(db.query('SELECT id FROM orca_worker_creation_reservations WHERE team_id=? AND lower(label)=lower(?)').get(teamId,label))return {ok:false,errorCode:'WORKER_CREATION_IN_PROGRESS'};
    const workers=db.query("SELECT count(*) AS n FROM orca_workers WHERE team_id=? AND status IN ('idle','running')").get(teamId).n;
    const reservations=db.query('SELECT count(*) AS n FROM orca_worker_creation_reservations WHERE team_id=?').get(teamId).n;
    const occupiedSlotsBefore=workers+reservations;
    if(occupiedSlotsBefore>=hardLimit)return {ok:false,errorCode:'WORKER_LIMIT_HARD_EXCEEDED'};
    db.query('INSERT INTO orca_worker_creation_reservations(id,team_id,label,created_at,expires_at) VALUES (?,?,?,?,?)').run(reservationId,teamId,label.toLowerCase(),now,now+leaseMs);
    return {ok:true,occupiedSlotsBefore};
   })();
  },
  renew(id,leaseMs){return db.query('UPDATE orca_worker_creation_reservations SET expires_at=? WHERE id=? AND expires_at>?').run(Date.now()+leaseMs,id,Date.now()).changes===1;},
  release(id){db.query('DELETE FROM orca_worker_creation_reservations WHERE id=?').run(id);},
  orphaned(id){const row=team(id);return !!row&&Date.now()-row.created_at>300_000&&!db.query('SELECT id FROM orca_workers WHERE team_id=? LIMIT 1').get(id)&&!db.query('SELECT id FROM orca_worker_creation_reservations WHERE team_id=? AND expires_at>? LIMIT 1').get(id,Date.now());},
 };
}
