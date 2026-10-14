using System.Formats.Nrbf;
using System.Reflection;
Console.OutputEncoding=new System.Text.UTF8Encoding(false);
if(args.Length!=2)throw new ArgumentException("Usage: read-release-defaults <Settings.nrbf> <output.json>");
if(new FileInfo(args[0]).Length>1024*1024 || Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(File.ReadAllBytes(args[0]))).ToLowerInvariant()!="ce44437e8a25f4a99f173f6143fdfb0e88fb9bf88198875e2a8e3949fd8a5446")throw new InvalidDataException("Recheck the pinned release before decoding another payload");
using var input=File.OpenRead(args[0]);
var root=NrbfDecoder.Decode(input,out var records);
Array SafeArray(ArrayRecord record)=>(Array)record.GetType().GetMethod("GetArray",new[]{typeof(bool)})!.Invoke(record,new object[]{true})!;
object? Normalize(object? value,int depth=0){if(depth>40)throw new InvalidDataException("Too deep");if(value is ClassRecord c){if(c.TypeName.FullName.StartsWith("System.Collections.Generic.List`1"))return SafeArray(c.GetArrayRecord("_items")!).Cast<object?>().Take(c.GetInt32("_size")).Select(v=>Normalize(v,depth+1)).ToArray();return c.MemberNames.ToDictionary(n=>n,n=>Normalize(c.GetRawValue(n),depth+1));}if(value is ArrayRecord a)return SafeArray(a).Cast<object?>().Select(v=>Normalize(v,depth+1)).ToArray();if(value is SerializationRecord r){var property=r.GetType().GetProperty("Value");if(property!=null)return property.GetValue(r);throw new InvalidDataException("Unexpected record "+r.TypeName.FullName);}return value;}
var pairs=SafeArray(((ClassRecord)root).GetArrayRecord("KeyValuePairs")!);
var result=pairs.Cast<ClassRecord>().ToDictionary(p=>p.GetString("key")!,p=>Normalize(p.GetRawValue("value")));
File.WriteAllText(args[1],System.Text.Json.JsonSerializer.Serialize(result,new System.Text.Json.JsonSerializerOptions{WriteIndented=true,Encoder=System.Text.Encodings.Web.JavaScriptEncoder.UnsafeRelaxedJsonEscaping}),new System.Text.UTF8Encoding(false));
Console.WriteLine("Decoded record data without instantiating serialized types: "+string.Join(", ",result.Keys));
