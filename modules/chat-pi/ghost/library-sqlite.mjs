// better-sqlite3's trusted constructor surface over Bun's bundled SQLite.
// Kept in a separate module so the real Cindy DB worker, not the shared Host,
// owns synchronous queries. Plugins never receive this constructor or a path.
import {Database} from 'bun:sqlite';

// Bun prepare compiles only the first statement, whereas better-sqlite3 rejects
// a nonempty tail. Preserve that contract before SQLite runs anything. Quoted
// text/comments are opaque; CREATE TRIGGER's BEGIN/END body is one statement.
function assertSingleStatement(sql) {
  const words=[];let body=false,cases=0,ended=false;
  for(let i=0;i<sql.length;) {
    const ch=sql[i];
    if(/\s/.test(ch)){i++;continue;}
    if(sql.startsWith('--',i)){const end=sql.indexOf('\n',i+2);i=end<0?sql.length:end+1;continue;}
    if(sql.startsWith('/*',i)){const end=sql.indexOf('*/',i+2);i=end<0?sql.length:end+2;continue;}
    if(ch===';') {if(!body)ended=true;i++;continue;}
    if(ended)throw Error('SQLite accepts only one statement');
    if(['\"',"'",'`','['].includes(ch)) {
      const close=ch==='['?']':ch;i++;
      while(i<sql.length){if(sql[i++]!==close)continue;if(close!==']'&&sql[i]===close){i++;continue;}break;}
      continue;
    }
    const match=/^[A-Za-z_][A-Za-z_0-9]*/.exec(sql.slice(i));
    if(!match){i++;continue;}
    const word=match[0].toUpperCase();i+=match[0].length;words.push(word);
    const trigger=words[0]==='CREATE'&&(words[1]==='TRIGGER'||(['TEMP','TEMPORARY'].includes(words[1])&&words[2]==='TRIGGER'));
    if(trigger){if(word==='BEGIN')body=true;else if(body&&word==='CASE')cases++;else if(body&&word==='END'){if(cases)cases--;else body=false;}}
  }
}
export class BunSqliteDatabase {
  constructor(file,options={}) {this.database=new Database(file,{readonly:options.readonly===true,create:options.readonly!==true,strict:true});}
  prepare(sql) {
    assertSingleStatement(sql);
    const statement=this.database.prepare(sql);
    return {reader:statement.columnNames.length>0,run:(...params)=>{const result=statement.run(...params);return {changes:result.changes,lastInsertRowid:result.lastInsertRowid};},get:(...params)=>statement.get(...params)??undefined,all:(...params)=>statement.all(...params),iterate:(...params)=>statement.iterate(...params)};
  }
  pragma(source) {return this.database.query('PRAGMA '+source).all();}
  exec(sql) {this.database.run(sql);}
  close() {this.database.close();}
  async backup(destination) {this.database.prepare('VACUUM INTO ?').run(destination);}
  transaction(fn) {return this.database.transaction(fn);}
}
export default BunSqliteDatabase;
