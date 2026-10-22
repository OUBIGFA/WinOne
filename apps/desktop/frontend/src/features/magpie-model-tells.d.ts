// Complete upstream callback signature; WinOne supplies display labels and
// provider grouping without changing any protocol ID.
export function modelTells<T={id:string;name?:string}>(items:T[],label?:(item:T)=>string,id?:(item:T)=>string,key?:(item:T)=>string):Map<string,string>;
