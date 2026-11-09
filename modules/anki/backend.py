"""Typed access to the pinned official Anki protobuf surface.

This covers core features not exposed by AnkiConnect, including FSRS tools,
image occlusion, filtered decks, preferences, media and all statistics.
"""
import ast
from pathlib import Path
from google.protobuf.json_format import MessageToDict, ParseDict
import anki._backend_generated as generated


def resolve(node):
    if isinstance(node, ast.Name):
        return vars(generated)[node.id]
    if isinstance(node, ast.Attribute):
        return getattr(resolve(node.value), node.attr)
    raise ValueError("Unexpected upstream protobuf type")


def registry():
    result = {}
    tree = ast.parse(Path(generated.__file__).read_text(encoding="utf-8"))
    cls = next(n for n in tree.body if isinstance(n, ast.ClassDef))
    for fn in cls.body:
        if not isinstance(fn, ast.FunctionDef) or fn.name.startswith("_") or fn.name.endswith("_raw"):
            continue
        if not hasattr(generated.RustBackendGenerated, fn.name + "_raw"):
            continue
        request = next((resolve(n.value.func) for n in fn.body if isinstance(n, ast.Assign)
                        and any(isinstance(t, ast.Name) and t.id == "message" for t in n.targets)
                        and isinstance(n.value, ast.Call)), None)
        if request is None:
            request = next((resolve(a.annotation) for a in fn.args.args if a.arg == "message"), None)
        output = next((resolve(n.value.func) for n in fn.body if isinstance(n, ast.Assign)
                       and any(isinstance(t, ast.Name) and t.id == "output" for t in n.targets)
                       and isinstance(n.value, ast.Call)), None)
        if request and output:
            result[fn.name] = (request, output)
    return result


METHODS = registry()


def field_schema(field, seen):
    from google.protobuf.descriptor import FieldDescriptor as F
    if field.type == F.TYPE_MESSAGE:
        target = field.message_type
        if target.GetOptions().map_entry:
            schema = {"type": "object", "additionalProperties": field_schema(target.fields_by_name["value"], seen)}
        elif target.full_name in seen:
            schema = {"type": "object"}
        else:
            schema = message_schema(target, seen | {target.full_name})
    elif field.type == F.TYPE_ENUM:
        schema = {"type": "string", "enum": [v.name for v in field.enum_type.values]}
    elif field.type == F.TYPE_BOOL:
        schema = {"type": "boolean"}
    elif field.type in (F.TYPE_STRING, F.TYPE_BYTES, F.TYPE_INT64, F.TYPE_UINT64, F.TYPE_SINT64, F.TYPE_FIXED64, F.TYPE_SFIXED64):
        schema = {"type": "string"}
    else:
        schema = {"type": "number"}
    if field.is_repeated and not (field.type == F.TYPE_MESSAGE and field.message_type.GetOptions().map_entry):
        schema = {"type": "array", "items": schema}
    return schema


def message_schema(descriptor, seen=None):
    seen = seen or {descriptor.full_name}
    return {"type": "object", "properties": {f.json_name: field_schema(f, seen) for f in descriptor.fields}}


def catalog():
    return {name: message_schema(types[0].DESCRIPTOR) for name, types in METHODS.items()}


def call(engine, name, params):
    if name not in METHODS:
        raise ValueError("Unknown official Anki operation")
    if not name.startswith(("get_", "find_", "search_", "describe_", "check_", "render_")):
        engine.runtime.gui.guard("backend.write", {})
    # Collection lifetime and network authorization belong to the native host.
    if name in {"open_collection", "close_collection", "import_collection_package", "export_collection_package"}:
        raise ValueError("Use the collection import/export action")
    if name.startswith(("sync_", "full_upload", "ankihub_")) or name in {"add_media_from_url", "fetch_sync_status"}:
        raise ValueError("Network operations require an explicit authorized sync or media action")
    request_type, output_type = METHODS[name]
    message = ParseDict(params, request_type())
    raw = getattr(engine.col._backend, name + "_raw")(message.SerializeToString())
    return MessageToDict(output_type.FromString(raw), always_print_fields_with_no_presence=True)
