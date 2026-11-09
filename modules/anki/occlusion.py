"""Native image-mask editing using Anki's original cloze representation."""
import base64
from google.protobuf.json_format import MessageToDict


def invoke(engine, method, params):
    if method == "winone.image.open":
        if params.get("noteId"):
            response = engine.col.get_image_occlusion_note(int(params["noteId"]))
            if response.WhichOneof("value") != "note":
                raise ValueError(response.error or "This is not an image occlusion note")
            value = MessageToDict(response.note, always_print_fields_with_no_presence=True)
            value["noteId"] = params["noteId"]
            return value
        response = engine.col.get_image_for_occlusion(params["path"])
        return {"imageData": base64.b64encode(response.data).decode(), "path": params["path"],
                "occlusions": [], "header": "", "backExtra": "", "tags": [], "occludeInactive": True}
    if method == "winone.image.save":
        engine.runtime.gui.guard("noteTypes.update", {})
        masks = []
        for group in params["occlusions"]:
            ordinal = int(group["ordinal"])
            if ordinal < 1:
                raise ValueError("Mask group must be a positive number")
            for shape in group["shapes"]:
                kind = shape["shape"]
                if kind not in ("rect", "ellipse", "polygon", "text"):
                    raise ValueError("Unknown image mask shape")
                fields = []
                for item in shape["properties"]:
                    name = item["name"]
                    if name not in ("left", "top", "width", "height", "rx", "ry", "points", "text", "scale", "fs", "fill", "angle", "oi"):
                        raise ValueError("Unknown image mask property")
                    value = str(item["value"]).replace("\\", "\\\\").replace(":", "\\:")
                    if "}}" in value:
                        raise ValueError("Mask property cannot contain a cloze terminator")
                    fields.append(f":{name}={value}")
                if params.get("occludeInactive", True):
                    fields.append(":oi=1")
                masks.append("{{c" + str(ordinal) + "::image-occlusion:" + kind + "".join(fields) + "}}<br>")
        if not masks:
            raise ValueError("Draw at least one mask")
        kwargs = {"occlusions": "".join(masks), "header": params.get("header", ""),
                  "back_extra": params.get("backExtra", ""), "tags": params.get("tags", [])}
        if params.get("noteId"):
            engine.col.update_image_occlusion_note(note_id=int(params["noteId"]), **kwargs)
        else:
            engine.col.add_image_occlusion_notetype()
            model = next(m for m in engine.col.models.all() if m.get("originalStockKind") == 6)
            engine.col.decks.select(int(params["deckId"]))
            engine.col.add_image_occlusion_note(notetype_id=model["id"], image_path=params["path"], **kwargs)
        return {"saved": True}
    raise ValueError("Unknown image operation")
