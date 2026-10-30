// Durable acceptance before an SDK advances its polling cursor. Payloads and
// completed replies share the private DPAPI-derived AES-GCM credential plane.
import {Database} from 'bun:sqlite';
import {createHash,createCipheriv,createDecipheriv,randomBytes} from 'node:crypto';
import path from 'node:path';
export function createOneBotIMQueue(paths,encodedKey){
 const key=Buffer.from(encodedKey??'','base64');if(key.length!==32)throw Error('IM queue key unavailable');
 const file=path.join(paths.root,'im-inbox.db');paths.assertPath(file);const db=new Database(file,{create:true});
 db.exec('PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; CREATE TABLE IF NOT EXISTS inbox(id TEXT PRIMARY KEY,connection TEXT NOT NULL,body BLOB NOT NULL,result BLOB,status TEXT NOT NULL,created_at INTEGER NOT NULL)');
 const seal=(id,value)=>{const nonce=randomBytes(12),cipher=createCipheriv('aes-256-gcm',key,nonce);cipher.setAAD(Buffer.from(id));const bytes=Buffer.concat([cipher.update(JSON.stringify(value),'utf8'),cipher.final()]);return Buffer.concat([nonce,cipher.getAuthTag(),bytes]);};
 const open=(id,bytes)=>{const cipher=createDecipheriv('aes-256-gcm',key,bytes.subarray(0,12));cipher.setAAD(Buffer.from(id));cipher.setAuthTag(bytes.subarray(12,28));return JSON.parse(Buffer.concat([cipher.update(bytes.subarray(28)),cipher.final()]).toString('utf8'));};
 return {
  accept(connection,message){if(message.protectedContent)return null;const id=createHash('sha256').update(JSON.stringify([connection.id,message.contextId,message.senderId,message.messageId])).digest('hex');db.query('INSERT INTO inbox(id,connection,body,status,created_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING').run(id,connection.id,seal(id,{...message,raw:undefined}),'pending',Date.now());return id;},
  result(id){const row=db.query('SELECT result,status FROM inbox WHERE id=?').get(id);return row?.status==='delivered'?{delivered:true}:row?.result?{value:open(id,Buffer.from(row.result))}:null;},
  saveResult(id,value){db.query('UPDATE inbox SET result=?,status=? WHERE id=?').run(seal(id,value),'delivery-pending',id);},
  delivered(id){db.query('UPDATE inbox SET status=?,body=?,result=NULL WHERE id=?').run('delivered',seal(id,{}),id);},
  pending(connection){return db.query("SELECT id,body FROM inbox WHERE connection=? AND status<>'delivered' ORDER BY created_at,rowid").all(connection).map(row=>({id:row.id,message:open(row.id,Buffer.from(row.body))}));},
  removeConnection(connection){db.query('DELETE FROM inbox WHERE connection=?').run(connection);},
  close(){db.close(true);key.fill(0);},
 };
}
