// Runtime tables are derived from the pinned upstream Drizzle schema. OneBot
// and the plugin receipt ledger share this DDL path so indexes/defaults cannot
// silently diverge from their real services. Call inside the caller's migration.
import {getTableConfig, SQLiteSyncDialect} from 'drizzle-orm/sqlite-core';
const quote=name=>'"'+name.replaceAll('"','""')+'"';
export function createRuntimeTable(sqlite,table,relational=false){
 const dialect=new SQLiteSyncDialect(),config=getTableConfig(table);
 // Drizzle index columns may be SQL expressions (e.g. lower(label)), not
 // named columns. SQLite CREATE INDEX forbids a table qualifier in expressions;
 // remove only this trusted schema table's generated qualifier, not its policy.
 const render=expression=>dialect.sqlToQuery(expression.inlineParams()).sql.replaceAll(quote(config.name)+'.','');
 const columns=config.columns.map(column=>{
  let definition=quote(column.name)+' '+column.getSQLType()+(column.primary?' PRIMARY KEY':'')+(column.notNull?' NOT NULL':'');
  if(column.default!==undefined){
   const value=column.default;
   if(value&&typeof value==='object'&&typeof value.getSQL==='function')definition+=' DEFAULT ('+dialect.sqlToQuery(value.inlineParams()).sql+')';
   else{const stored=column.mapToDriverValue(value);definition+=' DEFAULT '+(typeof stored==='string'?"'"+stored.replaceAll("'","''")+"'":stored===null?'NULL':Number(stored));}
  }
  return definition;
 });
 if(relational){
  for(const key of config.primaryKeys)columns.push('PRIMARY KEY ('+key.columns.map(column=>quote(column.name)).join(',')+')');
  for(const key of config.foreignKeys){
   const reference=key.reference();
   columns.push('FOREIGN KEY ('+reference.columns.map(column=>quote(column.name)).join(',')+') REFERENCES '+quote(getTableConfig(reference.foreignTable).name)
    +' ('+reference.foreignColumns.map(column=>quote(column.name)).join(',')+')'+(key.onDelete?' ON DELETE '+key.onDelete.toUpperCase():''));
  }
 }
 sqlite.exec('CREATE TABLE '+quote(config.name)+' ('+columns.join(',')+')');
 for(const {config:index} of config.indexes){
  const names=index.columns.map(column=>typeof column.name==='string'?quote(column.name):render(column)).join(',');
  const where=index.where?' WHERE '+render(index.where):'';
  sqlite.exec('CREATE '+(index.unique?'UNIQUE ':'')+'INDEX '+quote(index.name)+' ON '+quote(config.name)+' ('+names+')'+where);
 }
}
